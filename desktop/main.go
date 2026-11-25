package main

import (
	"embed"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	wr "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	base, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(err)
	}
	store, err := OpenStore(filepath.Join(base, "ReproPack"))
	if err != nil {
		log.Fatal(err)
	}
	app := NewApp(store)

	appMenu := menu.NewMenu()
	if runtime.GOOS == "darwin" {
		appMenu.Append(menu.AppMenu())
	}
	file := appMenu.AddSubmenu("File")
	file.AddText("New Report", keys.CmdOrCtrl("n"), func(*menu.CallbackData) { wr.EventsEmit(app.ctx, "menu", "new") })
	file.AddText("Open Report…", keys.CmdOrCtrl("o"), func(*menu.CallbackData) { wr.EventsEmit(app.ctx, "menu", "open") })
	if runtime.GOOS == "darwin" {
		appMenu.Append(menu.EditMenu())
		appMenu.Append(menu.WindowMenu())
	}

	err = wails.Run(&options.App{
		Title:     "ReproPack",
		Width:     1240,
		Height:    820,
		MinWidth:  860,
		MinHeight: 600,
		Menu:      appMenu,
		AssetServer: &assetserver.Options{
			Assets:     assets,
			Handler:    &mediaHandler{app: app},
			Middleware: securityHeaders,
		},
		BackgroundColour:         &options.RGBA{R: 246, G: 244, B: 239, A: 255},
		OnStartup:                app.startup,
		OnShutdown:               app.shutdown,
		Bind:                     []interface{}{app},
		EnableDefaultContextMenu: false,
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarHiddenInset(),
			Appearance:           mac.DefaultAppearance,
			WebviewIsTransparent: false,
			About:                &mac.AboutInfo{Title: "ReproPack", Message: "Portable, local-first bug reports.\nNothing leaves your computer unless you share it."},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}

// pageCSP applies to the app's own pages. Wails injects small inline
// bootstrap scripts, so inline scripts are allowed; report content is never
// rendered as HTML (Svelte escapes all text) and no network origin is allowed.
const pageCSP = "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; media-src 'self' blob:; font-src 'self' data:; connect-src 'self'; object-src 'none'; frame-src 'none'; base-uri 'none'; form-action 'none'"

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/media/") {
			w.Header().Set("Content-Security-Policy", pageCSP)
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Referrer-Policy", "no-referrer")
		}
		next.ServeHTTP(w, r)
	})
}

// mediaHandler serves attachment bytes to the webview from drafts and from
// opened, integrity-verified reports. Only allowlisted raster image, video
// and plain-text types are served, with nosniff and a sandboxing CSP.
type mediaHandler struct{ app *App }

func (h *mediaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/media/"), "/")
	if !strings.HasPrefix(r.URL.Path, "/media/") || len(parts) != 3 {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Cache-Control", "no-store")
	switch parts[0] {
	case "draft":
		att, err := h.app.store.Attachment(parts[1], parts[2])
		if err != nil || !validDraftID(parts[1]) {
			http.NotFound(w, r)
			return
		}
		ct, ok := mediaType(att.MediaType)
		if !ok {
			http.Error(w, "unsupported", http.StatusUnsupportedMediaType)
			return
		}
		f, err := os.Open(filepath.Join(h.app.store.DraftDir(parts[1]), filepath.Base(att.File)))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		st, _ := f.Stat()
		w.Header().Set("Content-Type", ct)
		http.ServeContent(w, r, "", st.ModTime(), f)
	case "report":
		h.app.mu.Lock()
		o := h.app.reports[parts[1]]
		h.app.mu.Unlock()
		if o == nil || !o.sum.Verified {
			http.Error(w, "report failed its integrity check; media is not shown", http.StatusForbidden)
			return
		}
		at, ok := o.r.Attachment(parts[2])
		if !ok {
			http.NotFound(w, r)
			return
		}
		ct, ok := mediaType(at.MediaType)
		if !ok {
			http.Error(w, "unsupported", http.StatusUnsupportedMediaType)
			return
		}
		rc, err := o.r.OpenAttachment(at.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rc.Close()
		// Buffer to support range requests for video and to verify the hash
		// before any byte reaches the webview.
		b, err := io.ReadAll(io.LimitReader(rc, at.Size+1))
		if err != nil {
			http.Error(w, "attachment failed verification", http.StatusUnprocessableEntity)
			return
		}
		w.Header().Set("Content-Type", ct)
		http.ServeContent(w, r, "", o.r.Manifest.CreatedAt, &readSeeker{b: b})
	default:
		http.NotFound(w, r)
	}
}

type readSeeker struct {
	b   []byte
	off int64
}

func (r *readSeeker) Read(p []byte) (int, error) {
	if r.off >= int64(len(r.b)) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.off:])
	r.off += int64(n)
	return n, nil
}

func (r *readSeeker) Seek(off int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		off += r.off
	case io.SeekEnd:
		off += int64(len(r.b))
	}
	if off < 0 {
		return 0, errors.New("negative offset")
	}
	r.off = off
	return off, nil
}

func revealFile(path string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", "-R", path).Run()
	case "windows":
		return exec.Command("explorer", "/select,", path).Run()
	}
	return exec.Command("xdg-open", filepath.Dir(path)).Run()
}
