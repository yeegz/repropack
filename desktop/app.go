package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	wr "github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/yeegz/repropack/format"
	"github.com/yeegz/repropack/ingest"
	"github.com/yeegz/repropack/redact"
	"github.com/yeegz/repropack/viewer"
)

const appVersion = "0.1.0"

// App is bound to the frontend. Every method is an explicit user action;
// nothing runs in the background except a watch the person started.
type App struct {
	ctx   context.Context
	store *Store

	mu        sync.Mutex
	rec       *recording
	recDraft  string
	watchStop context.CancelFunc
	watchPID  int
	reports   map[string]*openReport // token -> opened report
}

type openReport struct {
	r    *format.Report
	path string
	sum  *viewer.Summary
}

func NewApp(store *Store) *App { return &App{store: store, reports: map[string]*openReport{}} }

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	days := a.retentionDays()
	if n, err := a.store.Cleanup(time.Duration(days) * 24 * time.Hour); err == nil && n > 0 {
		wr.LogInfof(ctx, "retention: removed %d drafts older than %d days", n, days)
	}
}

func (a *App) shutdown(ctx context.Context) {
	a.StopWatching()
	a.mu.Lock()
	rec := a.rec
	a.mu.Unlock()
	if rec != nil {
		rec.stop()
	}
	for _, o := range a.reports {
		o.r.Close()
	}
	a.store.Close()
}

// ---- environment ----

type Environment struct {
	Capabilities Capabilities `json:"capabilities"`
	Version      string       `json:"version"`
	OSName       string       `json:"osName"`
	OSVersion    string       `json:"osVersion"`
	Arch         string       `json:"arch"`
	Model        string       `json:"model"`
	CPU          string       `json:"cpu"`
	MemoryBytes  uint64       `json:"memoryBytes"`
	DataDir      string       `json:"dataDir"`
	Retention    int          `json:"retentionDays"`
	IssueRepo    string       `json:"issueRepo"`
	IssueHost    string       `json:"issueHost"`
}

func (a *App) Environment() Environment {
	ctx, cancel := context.WithTimeout(a.ctx, 2*time.Second)
	defer cancel()
	v, _, model, cpu, mem := systemInfo(ctx)
	return Environment{
		Capabilities: capabilities(), Version: appVersion, OSName: osName(runtime.GOOS), OSVersion: v, Arch: runtime.GOARCH,
		Model: model, CPU: cpu, MemoryBytes: mem, DataDir: a.store.dir, Retention: a.retentionDays(),
		IssueRepo: a.store.Setting("issue_repo", ""), IssueHost: a.store.Setting("issue_host", "github"),
	}
}

func osName(g string) string {
	switch g {
	case "darwin":
		return "macOS"
	case "windows":
		return "Windows"
	case "linux":
		return "Linux"
	}
	return g
}

func (a *App) retentionDays() int {
	var d int
	fmt.Sscanf(a.store.Setting("retention_days", "30"), "%d", &d)
	return d
}

func (a *App) SetRetentionDays(days int) error {
	if days < 0 || days > 3650 {
		return errors.New("retention must be between 0 (keep forever) and 3650 days")
	}
	return a.store.SetSetting("retention_days", fmt.Sprint(days))
}

// ---- drafts ----

type DraftDetail struct {
	Draft       Draft             `json:"draft"`
	Attachments []DraftAttachment `json:"attachments"`
	Notes       []Note            `json:"notes"`
	Events      []format.Event    `json:"events"`
	Watching    bool              `json:"watching"`
	Recording   bool              `json:"recording"`
}

func (a *App) ListDrafts() ([]Draft, error) { return a.store.ListDrafts() }

func (a *App) NewDraft() (*Draft, error) {
	d, err := a.store.CreateDraft()
	if err != nil {
		return nil, err
	}
	a.store.AddEvent(d.ID, format.Event{Kind: format.KindMarker, Level: format.LevelInfo, Source: "repropack", Message: "Report started"})
	return d, nil
}

func (a *App) GetDraft(id string) (*DraftDetail, error) {
	d, err := a.store.GetDraft(id)
	if err != nil {
		return nil, err
	}
	atts, err := a.store.Attachments(id)
	if err != nil {
		return nil, err
	}
	notes, err := a.store.Notes(id)
	if err != nil {
		return nil, err
	}
	ev, err := a.store.Events(id)
	if err != nil {
		return nil, err
	}
	if len(ev) > 2000 {
		ev = ev[len(ev)-2000:]
	}
	a.mu.Lock()
	det := &DraftDetail{Draft: *d, Attachments: atts, Notes: notes, Events: ev, Watching: a.watchStop != nil && d.TargetPID == a.watchPID, Recording: a.rec != nil && a.recDraft == id}
	a.mu.Unlock()
	return det, nil
}

func (a *App) SaveDraftText(id string, t DraftText) error { return a.store.UpdateDraft(id, t) }

func (a *App) DeleteDraft(id string) error {
	a.mu.Lock()
	busy := a.rec != nil && a.recDraft == id
	a.mu.Unlock()
	if busy {
		return errors.New("stop the recording before deleting this report")
	}
	return a.store.DeleteDraft(id)
}

func (a *App) AddNote(id, text string) (*Note, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, errors.New("write something first")
	}
	return a.store.AddNote(id, text)
}

func (a *App) DeleteNote(id, noteID string) error { return a.store.DeleteNote(id, noteID) }

// MarkMoment records "it happened now" on the timeline.
func (a *App) MarkMoment(id, label string) error {
	if label == "" {
		label = "The problem happened here"
	}
	return a.store.AddEvent(id, format.Event{Kind: format.KindMarker, Level: format.LevelWarn, Source: "you", Message: label})
}

// ---- capture ----

func (a *App) TakeScreenshot(id, mode string) (*DraftAttachment, error) {
	if _, err := a.store.GetDraft(id); err != nil {
		return nil, err
	}
	attID := format.ShortID()
	file := attID + ".png"
	path := filepath.Join(a.store.DraftDir(id), file)
	ctx, cancel := context.WithTimeout(a.ctx, 5*time.Minute)
	defer cancel()
	// Hide our window so it is not in the way of the picker; always restore it.
	wr.WindowHide(a.ctx)
	err := screenshot(ctx, mode, path)
	wr.WindowShow(a.ctx)
	if err != nil {
		return nil, err
	}
	os.Chmod(path, 0o600)
	w, h := imageSize(path)
	title := map[string]string{ModeWindow: "Window screenshot", ModeArea: "Area screenshot"}[mode]
	if title == "" {
		title = "Screenshot"
	}
	att := DraftAttachment{ID: attID, File: file, MediaType: "image/png", Role: format.RoleScreenshot, Title: title, CreatedAt: time.Now(), Width: w, Height: h}
	if err := a.store.AddAttachment(id, att); err != nil {
		return nil, err
	}
	a.store.AddEvent(id, format.Event{Kind: format.KindMarker, Level: format.LevelInfo, Source: "repropack", Message: "Screenshot captured", Attrs: map[string]any{"attachment": attID}})
	return a.store.Attachment(id, attID)
}

func imageSize(path string) (int, int) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	c, _, err := image.DecodeConfig(io.LimitReader(f, 64<<20))
	if err != nil {
		return 0, 0
	}
	return c.Width, c.Height
}

func (a *App) StartRecording(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.rec != nil {
		return errors.New("a recording is already running")
	}
	if _, err := a.store.GetDraft(id); err != nil {
		return err
	}
	attID := format.ShortID()
	path := filepath.Join(a.store.DraftDir(id), attID+".mov")
	r, err := startRecording(path, 300)
	if err != nil {
		return err
	}
	a.rec, a.recDraft = r, id
	go func() {
		// If the person stops from the macOS menu bar, finish the same way.
		err := <-r.done
		r.done <- err
		a.mu.Lock()
		still := a.rec == r
		a.mu.Unlock()
		if still {
			a.finishRecording(id, attID, r)
		}
	}()
	a.store.AddEvent(id, format.Event{Kind: format.KindMarker, Level: format.LevelInfo, Source: "repropack", Message: "Screen recording started"})
	wr.EventsEmit(a.ctx, "recording", map[string]any{"draft": id, "state": "started", "at": time.Now()})
	return nil
}

func (a *App) StopRecording() (*DraftAttachment, error) {
	a.mu.Lock()
	r, id := a.rec, a.recDraft
	a.mu.Unlock()
	if r == nil {
		return nil, errors.New("nothing is recording")
	}
	return a.finishRecording(id, strings.TrimSuffix(filepath.Base(r.path), ".mov"), r)
}

func (a *App) finishRecording(id, attID string, r *recording) (*DraftAttachment, error) {
	d, err := r.stop()
	a.mu.Lock()
	if a.rec == r {
		a.rec, a.recDraft = nil, ""
	}
	a.mu.Unlock()
	wr.EventsEmit(a.ctx, "recording", map[string]any{"draft": id, "state": "stopped"})
	if err != nil {
		a.store.AddEvent(id, format.Event{Kind: format.KindMarker, Level: format.LevelInfo, Source: "repropack", Message: "Screen recording cancelled"})
		return nil, err
	}
	if existing, err := a.store.Attachment(id, attID); err == nil {
		return existing, nil
	}
	os.Chmod(r.path, 0o600)
	att := DraftAttachment{ID: attID, File: filepath.Base(r.path), MediaType: "video/quicktime", Role: format.RoleRecording, Title: "Screen recording", CreatedAt: r.started, DurationMS: d.Milliseconds()}
	if err := a.store.AddAttachment(id, att); err != nil {
		return nil, err
	}
	a.store.AddEvent(id, format.Event{Kind: format.KindMarker, Level: format.LevelInfo, Source: "repropack", Message: "Screen recording saved", Attrs: map[string]any{"attachment": attID, "seconds": d.Seconds()}})
	return a.store.Attachment(id, attID)
}

var addable = map[string]string{
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif", ".webp": "image/webp",
	".mov": "video/quicktime", ".mp4": "video/mp4", ".webm": "video/webm",
	".log": "text/plain", ".txt": "text/plain", ".json": "application/json", ".jsonl": "application/x-ndjson",
}

// AddFiles lets the person choose existing evidence. Images and videos are
// copied as attachments; log files are parsed into timeline events (last
// 4 MB) and also attached so the original lines are preserved.
func (a *App) AddFiles(id string) ([]DraftAttachment, error) {
	paths, err := wr.OpenMultipleFilesDialog(a.ctx, wr.OpenDialogOptions{
		Title: "Add screenshots, recordings or log files",
		Filters: []wr.FileFilter{
			{DisplayName: "Evidence (images, videos, logs)", Pattern: "*.png;*.jpg;*.jpeg;*.gif;*.webp;*.mov;*.mp4;*.webm;*.log;*.txt;*.json;*.jsonl"},
		},
	})
	if err != nil || len(paths) == 0 {
		return nil, err
	}
	var out []DraftAttachment
	for _, p := range paths {
		att, err := a.addFile(id, p)
		if err != nil {
			return out, fmt.Errorf("%s: %w", filepath.Base(p), err)
		}
		out = append(out, *att)
	}
	return out, nil
}

const maxAddBytes = 512 << 20
const logTail = 4 << 20

func (a *App) addFile(id, p string) (*DraftAttachment, error) {
	st, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	ext := strings.ToLower(filepath.Ext(p))
	mt, ok := addable[ext]
	if !ok {
		return nil, errors.New("this file type is not supported")
	}
	if st.Size() > maxAddBytes {
		return nil, fmt.Errorf("files larger than %s are not supported", viewer.HumanBytes(maxAddBytes))
	}
	attID := format.ShortID()
	file := attID + ext
	dst := filepath.Join(a.store.DraftDir(id), file)
	role := format.RoleOther
	switch {
	case strings.HasPrefix(mt, "image/"):
		role = format.RoleScreenshot
	case strings.HasPrefix(mt, "video/"):
		role = format.RoleRecording
	default:
		role = format.RoleLog
	}
	if role == format.RoleLog {
		var evs []format.Event
		if err := ingest.File(p, logTail, func(e format.Event) { evs = append(evs, e) }); err != nil {
			return nil, err
		}
		if len(evs) > 0 {
			if err := a.store.AddEvents(id, evs); err != nil {
				return nil, err
			}
		}
		if err := copyTail(p, dst, logTail); err != nil {
			return nil, err
		}
	} else if err := copyFile(p, dst); err != nil {
		return nil, err
	}
	att := DraftAttachment{ID: attID, File: file, MediaType: mt, Role: role, Title: filepath.Base(p), CreatedAt: st.ModTime()}
	if role == format.RoleScreenshot {
		att.Width, att.Height = imageSize(dst)
	}
	if err := a.store.AddAttachment(id, att); err != nil {
		return nil, err
	}
	return a.store.Attachment(id, attID)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, io.LimitReader(in, maxAddBytes)); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	return out.Close()
}

func copyTail(src, dst string, n int64) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if st, _ := in.Stat(); st.Size() > n {
		in.Seek(st.Size()-n, io.SeekStart)
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, io.LimitReader(in, n))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}

func (a *App) RemoveAttachment(id, attID string) error { return a.store.DeleteAttachment(id, attID) }

func (a *App) SetAttachmentNote(id, attID, note string) error {
	if len(note) > 2000 {
		note = note[:2000]
	}
	return a.store.UpdateAttachment(id, attID, map[string]any{"note": note})
}

func (a *App) SetReviewed(id, attID string, reviewed bool) error {
	return a.store.UpdateAttachment(id, attID, map[string]any{"reviewed": reviewed})
}

// SaveMarkup replaces a screenshot with the marked-up version from the
// editor. Redaction boxes are burned into the pixels and the original file
// is deleted, so hidden content cannot be recovered from the report.
func (a *App) SaveMarkup(id, attID, pngBase64 string, redactions int) (*DraftAttachment, error) {
	att, err := a.store.Attachment(id, attID)
	if err != nil {
		return nil, err
	}
	if att.Role != format.RoleScreenshot {
		return nil, errors.New("only screenshots can be marked up")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(pngBase64, "data:image/png;base64,"))
	if err != nil || len(raw) > 64<<20 {
		return nil, errors.New("the edited image could not be read")
	}
	img, err := png.Decode(bytes.NewReader(raw)) // re-encode: guarantees a clean PNG with no metadata
	if err != nil {
		return nil, errors.New("the edited image is not a PNG")
	}
	newFile := format.ShortID() + ".png"
	dst := filepath.Join(a.store.DraftDir(id), newFile)
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		os.Remove(dst)
		return nil, err
	}
	f.Close()
	old := filepath.Join(a.store.DraftDir(id), att.File)
	b := img.Bounds()
	if err := a.store.UpdateAttachment(id, attID, map[string]any{"file": newFile, "annotated": true, "reviewed": true, "media_type": "image/png", "width": b.Dx(), "height": b.Dy()}); err != nil {
		os.Remove(dst)
		return nil, err
	}
	os.Remove(old)
	if redactions > 0 {
		a.store.AddManual(id, redactions)
	}
	return a.store.Attachment(id, attID)
}

// ---- watch an app ----

func (a *App) ListProcesses() ([]Process, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 3*time.Second)
	defer cancel()
	return listProcesses(ctx)
}

// WatchProcess samples CPU and memory of the chosen process once a second
// until StopWatching, the process exits, or 30 minutes pass.
func (a *App) WatchProcess(id string, pid int, name string) error {
	if pid <= 0 {
		return errors.New("choose an app to watch")
	}
	a.StopWatching()
	if err := a.store.SetTarget(id, pid, name); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Minute)
	a.mu.Lock()
	a.watchStop, a.watchPID = cancel, pid
	a.mu.Unlock()
	a.store.AddEvent(id, format.Event{Kind: format.KindMarker, Level: format.LevelInfo, Source: "repropack", Message: "Started watching " + name, Attrs: map[string]any{"pid": pid}})
	go func() {
		defer cancel()
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			sctx, scancel := context.WithTimeout(ctx, 900*time.Millisecond)
			cpu, rss, err := sample(sctx, pid)
			scancel()
			if err != nil {
				if ctx.Err() == nil {
					a.store.AddEvent(id, format.Event{Kind: format.KindLog, Level: format.LevelWarn, Source: "repropack", Message: name + " is no longer running"})
					wr.EventsEmit(a.ctx, "watch", map[string]any{"draft": id, "state": "exited"})
				}
				break
			}
			now := time.Now()
			a.store.AddEvents(id, []format.Event{
				{Time: now, Kind: format.KindMetric, Source: name, Metric: &format.Metric{Name: "cpu", Value: cpu, Unit: "%"}},
				{Time: now, Kind: format.KindMetric, Source: name, Metric: &format.Metric{Name: "memory", Value: rss, Unit: "By"}},
			})
			wr.EventsEmit(a.ctx, "sample", map[string]any{"draft": id, "cpu": cpu, "rss": rss, "at": now})
			select {
			case <-ctx.Done():
				goto out
			case <-t.C:
			}
		}
	out:
		a.mu.Lock()
		if a.watchPID == pid {
			a.watchStop, a.watchPID = nil, 0
		}
		a.mu.Unlock()
		a.store.AddEvent(id, format.Event{Kind: format.KindMarker, Level: format.LevelInfo, Source: "repropack", Message: "Stopped watching " + name})
	}()
	return nil
}

func (a *App) StopWatching() {
	a.mu.Lock()
	stop := a.watchStop
	a.watchStop, a.watchPID = nil, 0
	a.mu.Unlock()
	if stop != nil {
		stop()
	}
}

// ---- review ----

// ReviewFinding is a sensitive value found in the draft's text.
type ReviewFinding struct {
	Key     string `json:"key"` // field + "#" + index within that field's scan
	Field   string `json:"field"`
	Label   string `json:"label"`
	RuleID  string `json:"ruleId"`
	Rule    string `json:"rule"`
	Preview string `json:"preview"`
	Context string `json:"context"` // surrounding text with the value masked
}

type Review struct {
	Findings   []ReviewFinding   `json:"findings"`
	Media      []DraftAttachment `json:"media"`
	Unreviewed int               `json:"unreviewed"`
	Policy     string            `json:"policy"`
}

type textField struct {
	field, label, text string
}

func (a *App) draftFields(id string) ([]textField, *Draft, []format.Event, []Note, []DraftAttachment, error) {
	d, err := a.store.GetDraft(id)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	ev, err := a.store.Events(id)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	notes, _ := a.store.Notes(id)
	atts, _ := a.store.Attachments(id)
	fs := []textField{{"title", "Title", d.Title}, {"description", "Description", d.Description}, {"expected", "Expected", d.Expected}, {"actual", "What happened", d.Actual}}
	for i, s := range d.Steps {
		fs = append(fs, textField{fmt.Sprintf("steps[%d]", i), fmt.Sprintf("Step %d", i+1), s})
	}
	for i, n := range notes {
		fs = append(fs, textField{fmt.Sprintf("notes[%d]", i), "Note", n.Text})
	}
	for i, at := range atts {
		fs = append(fs, textField{fmt.Sprintf("attnote[%d]", i), "Note on " + at.Title, at.Note})
		fs = append(fs, textField{fmt.Sprintf("atttitle[%d]", i), "Attachment name", at.Title})
	}
	for _, e := range ev {
		fs = append(fs, textField{fmt.Sprintf("events[%d].msg", e.Seq), "Event message", e.Message})
	}
	return fs, d, ev, notes, atts, nil
}

func (a *App) Review(id string) (*Review, error) {
	p := redact.Default()
	fields, _, ev, _, atts, err := a.draftFields(id)
	if err != nil {
		return nil, err
	}
	desc := map[string]string{}
	for _, r := range p.Rules {
		desc[r.ID] = r.Description
	}
	rv := &Review{Policy: p.Name, Findings: []ReviewFinding{}, Media: []DraftAttachment{}}
	for _, f := range fields {
		for i, fd := range p.Scan(f.field, f.text) {
			rv.Findings = append(rv.Findings, ReviewFinding{Key: fmt.Sprintf("%s#%d", f.field, i), Field: f.field, Label: f.label, RuleID: fd.RuleID, Rule: desc[fd.RuleID], Preview: fd.Preview, Context: contextAround(f.text, fd)})
		}
	}
	// Attribute values and text attachments are always redacted; list them
	// so the person knows, but they cannot be kept.
	for _, e := range ev {
		if e.Attrs != nil {
			c := redact.Counts{}
			p.Value("attrs", e.Attrs, c)
			for rule, n := range c {
				for k := 0; k < n; k++ {
					rv.Findings = append(rv.Findings, ReviewFinding{Key: fmt.Sprintf("events[%d].attrs#%s%d", e.Seq, rule, k), Field: "attrs", Label: "Event details", RuleID: rule, Rule: desc[rule], Preview: "always removed"})
				}
			}
		}
	}
	for _, at := range atts {
		if redact.IsText(at.MediaType) {
			b, err := os.ReadFile(filepath.Join(a.store.DraftDir(id), at.File))
			if err == nil {
				for i, fd := range p.Scan("log", string(b)) {
					rv.Findings = append(rv.Findings, ReviewFinding{Key: fmt.Sprintf("att:%s#%d", at.ID, i), Field: "attachment", Label: at.Title, RuleID: fd.RuleID, Rule: desc[fd.RuleID], Preview: fd.Preview, Context: contextAround(string(b), fd)})
				}
			}
		}
		if strings.HasPrefix(at.MediaType, "image/") || strings.HasPrefix(at.MediaType, "video/") {
			rv.Media = append(rv.Media, at)
			if !at.Reviewed {
				rv.Unreviewed++
			}
		}
	}
	return rv, nil
}

func contextAround(text string, f redact.Finding) string {
	s, e := max(0, f.Start-32), min(len(text), f.End+32)
	for s > 0 && s < len(text) && !utf8Start(text[s]) {
		s--
	}
	for e < len(text) && !utf8Start(text[e]) {
		e++
	}
	out := text[s:f.Start] + "⟦" + f.Preview + "⟧" + text[f.End:e]
	out = strings.Join(strings.Fields(out), " ")
	if s > 0 {
		out = "…" + out
	}
	if e < len(text) {
		out += "…"
	}
	return out
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }

// ---- export ----

type ExportResult struct {
	Path       string `json:"path"`
	HTMLPath   string `json:"htmlPath,omitempty"`
	Size       int64  `json:"size"`
	Digest     string `json:"digest"`
	Removed    int    `json:"removed"`
	Kept       int    `json:"kept"`
	Verified   bool   `json:"verified"`
	Events     int    `json:"events"`
	Attachment int    `json:"attachments"`
	IssueURL   string `json:"issueUrl,omitempty"`
}

// Export writes the draft to a .repropack file the person chooses. keep
// lists finding keys the person decided to keep; everything else matched by
// the policy is removed. The written file is re-opened, verified and
// re-scanned before success is reported.
func (a *App) Export(id string, keep []string, withHTML bool) (*ExportResult, error) {
	rv, err := a.Review(id)
	if err != nil {
		return nil, err
	}
	if rv.Unreviewed > 0 {
		return nil, fmt.Errorf("check the %d remaining %s before exporting", rv.Unreviewed, plural(rv.Unreviewed, "image or recording", "images or recordings"))
	}
	d, err := a.store.GetDraft(id)
	if err != nil {
		return nil, err
	}
	name := safeFileName(d.Title)
	path, err := wr.SaveFileDialog(a.ctx, wr.SaveDialogOptions{
		Title: "Save bug report", DefaultFilename: name + format.FileExtension, CanCreateDirectories: true,
		Filters: []wr.FileFilter{{DisplayName: "ReproPack report", Pattern: "*.repropack"}},
	})
	if err != nil || path == "" {
		return nil, err
	}
	if !strings.HasSuffix(path, format.FileExtension) {
		path += format.FileExtension
	}
	res, err := a.exportTo(id, path, keep)
	if err != nil {
		return nil, err
	}
	if withHTML {
		hp := strings.TrimSuffix(path, format.FileExtension) + ".html"
		if err := writeHTML(path, hp); err == nil {
			res.HTMLPath = hp
		}
	}
	a.store.MarkExported(id, path)
	return res, nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

var unsafeName = regexp.MustCompile(`[^\p{L}\p{N} _.-]+`)

func safeFileName(t string) string {
	t = strings.TrimSpace(unsafeName.ReplaceAllString(t, ""))
	if len(t) > 60 {
		t = t[:60]
	}
	if t == "" {
		t = "bug-report"
	}
	return t
}

func (a *App) exportTo(id, path string, keepKeys []string) (*ExportResult, error) {
	p := redact.Default()
	_, d, ev, notes, atts, err := a.draftFields(id)
	if err != nil {
		return nil, err
	}
	keep := map[string]map[int]bool{} // field -> scan indices to keep
	for _, k := range keepKeys {
		f, idx, ok := strings.Cut(k, "#")
		var i int
		if ok && !strings.HasPrefix(f, "att:") && !strings.Contains(f, ".attrs") {
			if _, err := fmt.Sscanf(idx, "%d", &i); err == nil {
				if keep[f] == nil {
					keep[f] = map[int]bool{}
				}
				keep[f][i] = true
			}
		}
	}
	counts := redact.Counts{}
	kept := 0
	red := func(field, s string) string {
		kept += len(keep[field])
		return p.String(field, s, counts, keep[field])
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".repropack-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	w := format.NewWriter(tmp)
	sort.SliceStable(ev, func(i, j int) bool { return ev[i].Time.Before(ev[j].Time) })
	var seq uint64
	for _, e := range ev {
		orig := e.Seq
		e.Message = red(fmt.Sprintf("events[%d].msg", orig), e.Message)
		msg := e.Message
		e = p.Event(format.Event{Time: e.Time, Kind: e.Kind, Level: e.Level, Source: e.Source, Message: "", Attrs: e.Attrs, TraceID: e.TraceID, SpanID: e.SpanID, Error: e.Error, Metric: e.Metric, Span: e.Span}, counts)
		e.Message = msg
		seq++
		e.Seq = seq
		if e.Kind == format.KindError || e.Kind == format.KindCrash {
			if e.Error == nil {
				e.Error = &format.ErrorInfo{Message: e.Message}
			}
		}
		if err := w.WriteEvent(e); err != nil {
			tmp.Close()
			return nil, err
		}
	}
	var reviewed []string
	for i, at := range atts {
		f, err := os.Open(filepath.Join(a.store.DraftDir(id), at.File))
		if err != nil {
			tmp.Close()
			return nil, err
		}
		var src io.Reader = f
		if redact.IsText(at.MediaType) {
			b, _ := io.ReadAll(io.LimitReader(f, redact.MaxTextAttachment))
			src = strings.NewReader(p.String("att", string(b), counts, nil))
		}
		meta := format.Attachment{ID: at.ID, MediaType: at.MediaType, Role: at.Role, Title: red(fmt.Sprintf("atttitle[%d]", i), at.Title), CreatedAt: at.CreatedAt.UTC(),
			Width: at.Width, Height: at.Height, DurationMS: at.DurationMS, Annotated: at.Annotated}
		_, err = w.AddAttachment(meta, src)
		f.Close()
		if err != nil {
			tmp.Close()
			return nil, err
		}
		if at.Reviewed {
			reviewed = append(reviewed, at.ID)
		}
	}
	var ann []format.Annotation
	for i, n := range notes {
		at := n.At.UTC()
		ann = append(ann, format.Annotation{ID: n.ID, Text: red(fmt.Sprintf("notes[%d]", i), n.Text), At: &at})
	}
	for i, at := range atts {
		if strings.TrimSpace(at.Note) != "" {
			ann = append(ann, format.Annotation{ID: "att-" + at.ID, Text: red(fmt.Sprintf("attnote[%d]", i), at.Note), AttachmentID: at.ID})
		}
	}
	steps := make([]string, 0, len(d.Steps))
	for i, s := range d.Steps {
		if strings.TrimSpace(s) != "" {
			steps = append(steps, red(fmt.Sprintf("steps[%d]", i), s))
		}
	}
	ctx, cancel := context.WithTimeout(a.ctx, 2*time.Second)
	ver, build, model, cpu, mem := systemInfo(ctx)
	cancel()
	m := format.Manifest{
		ReportID:    d.ID,
		CreatedAt:   time.Now().UTC(),
		Title:       red("title", orDefault(d.Title, "Untitled bug report")),
		Description: red("description", d.Description),
		Producer:    format.Producer{Name: "ReproPack", Version: appVersion},
		OS:          &format.OSContext{Name: runtime.GOOS, Version: ver, Build: build, Arch: runtime.GOARCH, Timezone: time.Local.String()},
		Device:      &format.DeviceInfo{Model: model, CPU: cpu, CPUCores: runtime.NumCPU(), MemoryBytes: mem},
		Annotations: ann,
	}
	if d.TargetName != "" {
		m.App = &format.AppContext{Name: d.TargetName, PID: d.TargetPID}
	}
	if len(steps) > 0 || d.Expected != "" || d.Actual != "" || d.Frequency != "" {
		m.Reproduction = &format.Reproduction{Steps: steps, Expected: red("expected", d.Expected), Actual: red("actual", d.Actual), Frequency: d.Frequency}
	}
	removed := 0
	info := &format.RedactionInfo{Policy: p.Name, Manual: d.Manual, ReviewedAttachments: reviewed}
	desc := map[string]string{}
	for _, r := range p.Rules {
		desc[r.ID] = r.Description
	}
	for id, n := range counts {
		info.Rules = append(info.Rules, format.RedactionRule{ID: id, Description: desc[id], Matches: n})
		removed += n
	}
	sort.Slice(info.Rules, func(i, j int) bool { return info.Rules[i].ID < info.Rules[j].ID })
	m.Redaction = info
	if kept == 0 {
		now := time.Now().UTC()
		info.Verified, info.VerifiedAt = true, &now
	}
	written, err := w.Close(m)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, err
	}
	// Verify what is on disk before telling the person it worked.
	r, err := format.Open(tmp.Name(), format.Limits{})
	if err != nil {
		return nil, fmt.Errorf("the report did not pass its own check: %w", err)
	}
	if err := r.Verify(); err != nil {
		r.Close()
		return nil, fmt.Errorf("the report did not pass its own check: %w", err)
	}
	left, err := redact.ScanReport(r, p)
	r.Close()
	if err != nil {
		return nil, err
	}
	if len(left) > kept {
		return nil, fmt.Errorf("redaction check failed: %d sensitive values remain that you did not choose to keep", len(left)-kept)
	}
	// When values were kept on purpose the manifest says "not verified".
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return nil, err
	}
	st, _ := os.Stat(path)
	return &ExportResult{Path: path, Size: st.Size(), Digest: written.ContentDigest, Removed: removed, Kept: kept, Verified: info.Verified, Events: written.Events.Count, Attachment: len(written.Attachments)}, nil
}

func orDefault(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return s
}

func writeHTML(src, dst string) error {
	r, err := format.Open(src, format.Limits{})
	if err != nil {
		return err
	}
	defer r.Close()
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return viewer.WriteHTML(f, r, viewer.HTMLOptions{})
}

// RevealInFinder shows an exported file in the system file manager.
func (a *App) RevealFile(path string) error {
	if !strings.HasSuffix(path, format.FileExtension) && !strings.HasSuffix(path, ".html") {
		return errors.New("only reports can be revealed")
	}
	return revealFile(path)
}

// ---- issue trackers ----

var repoRe = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}/[A-Za-z0-9_.-]{1,100}$`)

// IssueLink builds a "new issue" URL prefilled with the (already redacted)
// report summary. Nothing is uploaded: the person reviews the text in the
// browser and attaches the .repropack file themselves.
func (a *App) IssueLink(reportPath, host, repo string) (string, error) {
	repo = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(repo, "https://github.com/"), "https://gitlab.com/"))
	repo = strings.TrimSuffix(repo, "/")
	if !repoRe.MatchString(repo) {
		return "", errors.New("enter the repository as owner/name")
	}
	r, err := format.Open(reportPath, format.Limits{})
	if err != nil {
		return "", err
	}
	defer r.Close()
	s, err := viewer.Summarize(r, viewer.Options{MaxEvents: 1})
	if err != nil {
		return "", err
	}
	a.store.SetSetting("issue_repo", repo)
	a.store.SetSetting("issue_host", host)
	body := issueBody(s, filepath.Base(reportPath))
	title := s.Manifest.Title
	switch host {
	case "gitlab":
		return "https://gitlab.com/" + repo + "/-/issues/new?" + url.Values{"issue[title]": {title}, "issue[description]": {body}}.Encode(), nil
	default:
		return "https://github.com/" + repo + "/issues/new?" + url.Values{"title": {title}, "body": {body}}.Encode(), nil
	}
}

func issueBody(s *viewer.Summary, file string) string {
	m := s.Manifest
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", s.Headline)
	if m.Description != "" {
		fmt.Fprintf(&b, "%s\n\n", m.Description)
	}
	if r := m.Reproduction; r != nil {
		if len(r.Steps) > 0 {
			b.WriteString("### Steps to reproduce\n")
			for i, st := range r.Steps {
				fmt.Fprintf(&b, "%d. %s\n", i+1, st)
			}
			b.WriteString("\n")
		}
		if r.Expected != "" {
			fmt.Fprintf(&b, "**Expected:** %s\n\n", r.Expected)
		}
		if r.Actual != "" {
			fmt.Fprintf(&b, "**Actual:** %s\n\n", r.Actual)
		}
	}
	b.WriteString("### Environment\n")
	if m.App != nil {
		fmt.Fprintf(&b, "- App: %s %s\n", m.App.Name, m.App.Version)
	}
	if m.OS != nil {
		fmt.Fprintf(&b, "- OS: %s %s (%s)\n", osName(m.OS.Name), m.OS.Version, m.OS.Arch)
	}
	fmt.Fprintf(&b, "- Events: %d (%d errors) over %s\n\n", s.TotalEvents, s.Errors, viewer.HumanDuration(s.Duration))
	fmt.Fprintf(&b, "### Diagnostic report\nDrag **%s** into this box to attach it. It is a ReproPack report (ZIP with JSON and screenshots) that can be opened with the ReproPack app or `repropack inspect`.\n\n`%s`\n", file, m.ContentDigest)
	return b.String()
}

func (a *App) OpenURL(u string) error {
	if !strings.HasPrefix(u, "https://github.com/") && !strings.HasPrefix(u, "https://gitlab.com/") {
		return errors.New("only issue tracker links can be opened")
	}
	wr.BrowserOpenURL(a.ctx, u)
	return nil
}

// ---- viewer ----

type ReportView struct {
	Token     string          `json:"token"`
	Path      string          `json:"path"`
	FileSize  int64           `json:"fileSize"`
	Summary   *viewer.Summary `json:"summary"`
	MediaBase string          `json:"mediaBase"`
}

// OpenReportDialog asks for a .repropack file and opens it.
func (a *App) OpenReportDialog() (*ReportView, error) {
	p, err := wr.OpenFileDialog(a.ctx, wr.OpenDialogOptions{Title: "Open a bug report", Filters: []wr.FileFilter{{DisplayName: "ReproPack report", Pattern: "*.repropack"}}})
	if err != nil || p == "" {
		return nil, err
	}
	return a.OpenReport(p)
}

// OpenReport parses an untrusted report with the hardened reader. Nothing
// in it is executed; media is served only after integrity verification.
func (a *App) OpenReport(path string) (*ReportView, error) {
	r, err := format.Open(path, format.Limits{})
	if err != nil {
		return nil, err
	}
	s, err := viewer.Summarize(r, viewer.Options{MaxEvents: 50000})
	if err != nil {
		r.Close()
		return nil, err
	}
	st, _ := os.Stat(path)
	tok := format.ShortID()
	a.mu.Lock()
	for k, o := range a.reports { // keep at most 4 open
		if len(a.reports) < 4 {
			break
		}
		o.r.Close()
		delete(a.reports, k)
	}
	a.reports[tok] = &openReport{r: r, path: path, sum: s}
	a.mu.Unlock()
	a.store.TouchRecent(path, s.Manifest.Title)
	return &ReportView{Token: tok, Path: path, FileSize: st.Size(), Summary: s, MediaBase: "/media/report/" + tok + "/"}, nil
}

func (a *App) CloseReport(token string) {
	a.mu.Lock()
	if o, ok := a.reports[token]; ok {
		o.r.Close()
		delete(a.reports, token)
	}
	a.mu.Unlock()
}

func (a *App) Recents() ([]Recent, error)     { return a.store.Recents() }
func (a *App) ForgetRecent(path string) error { return a.store.ForgetRecent(path) }

// ReadTextAttachment returns a text attachment from an opened report.
func (a *App) ReadTextAttachment(token, attID string) (string, error) {
	a.mu.Lock()
	o := a.reports[token]
	a.mu.Unlock()
	if o == nil {
		return "", errors.New("report is closed")
	}
	at, ok := o.r.Attachment(attID)
	if !ok || !redact.IsText(at.MediaType) {
		return "", errors.New("not a text attachment")
	}
	b, err := o.r.ReadAttachment(attID, 2<<20)
	if err != nil {
		return "", err
	}
	return strings.ToValidUTF8(string(b), "�"), nil
}

// ExportReportHTML writes an HTML summary next to an opened report.
func (a *App) ExportReportHTML(token string) (string, error) {
	a.mu.Lock()
	o := a.reports[token]
	a.mu.Unlock()
	if o == nil {
		return "", errors.New("report is closed")
	}
	dst, err := wr.SaveFileDialog(a.ctx, wr.SaveDialogOptions{Title: "Save HTML summary", DefaultFilename: strings.TrimSuffix(filepath.Base(o.path), format.FileExtension) + ".html",
		Filters: []wr.FileFilter{{DisplayName: "Web page", Pattern: "*.html"}}})
	if err != nil || dst == "" {
		return "", err
	}
	return dst, writeHTML(o.path, dst)
}

// mediaType returns a safe Content-Type for serving.
func mediaType(mt string) (string, bool) {
	mt = strings.TrimSpace(strings.SplitN(mt, ";", 2)[0])
	switch mt {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "video/mp4", "video/quicktime", "video/webm":
		return mt, true
	case "text/plain", "application/json", "application/x-ndjson":
		return "text/plain; charset=utf-8", true
	}
	return mime.TypeByExtension(".bin"), false
}
