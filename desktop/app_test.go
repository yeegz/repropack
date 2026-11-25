package main

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yeegz/repropack/format"
	"github.com/yeegz/repropack/redact"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	a := NewApp(st)
	a.ctx = context.Background()
	return a
}

func writePNG(t *testing.T, path string) {
	var b bytes.Buffer
	png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 4, 3)))
	os.WriteFile(path, b.Bytes(), 0o600)
}

// seed builds a draft containing every kind of evidence the app records.
func seed(t *testing.T, a *App) string {
	d, err := a.store.CreateDraft()
	if err != nil {
		t.Fatal(err)
	}
	if err := a.store.UpdateDraft(d.ID, DraftText{Title: "Sync fails for jane@example.com", Steps: []string{"Open /Users/jane/Notes", "Press sync"}, Actual: "Error", Expected: "Synced"}); err != nil {
		t.Fatal(err)
	}
	a.store.AddEvent(d.ID, format.Event{Kind: "log", Level: "error", Message: "token=abcd1234secret rejected", Attrs: map[string]any{"authorization": "Bearer xyz"}})
	a.store.AddEvent(d.ID, format.Event{Kind: "metric", Metric: &format.Metric{Name: "cpu", Value: 50, Unit: "%"}})
	a.store.AddNote(d.ID, "Happens after lunch")
	dir := a.store.DraftDir(d.ID)
	writePNG(t, filepath.Join(dir, "shot.png"))
	a.store.AddAttachment(d.ID, DraftAttachment{ID: "shot", File: "shot.png", MediaType: "image/png", Role: format.RoleScreenshot, Title: "Window", CreatedAt: time.Now()})
	os.WriteFile(filepath.Join(dir, "app.log"), []byte("password=hunter22\n"), 0o600)
	a.store.AddAttachment(d.ID, DraftAttachment{ID: "log", File: "app.log", MediaType: "text/plain", Role: format.RoleLog, Title: "app.log", CreatedAt: time.Now()})
	return d.ID
}

func TestReviewRequiresImageCheck(t *testing.T) {
	a := newTestApp(t)
	id := seed(t, a)
	rv, err := a.Review(id)
	if err != nil {
		t.Fatal(err)
	}
	if rv.Unreviewed != 1 {
		t.Fatalf("unreviewed = %d", rv.Unreviewed)
	}
	rules := map[string]bool{}
	for _, f := range rv.Findings {
		rules[f.RuleID] = true
		if strings.Contains(f.Context, "jane@example.com") || strings.Contains(f.Context, "hunter22") {
			t.Fatalf("review context leaks the value: %q", f.Context)
		}
	}
	for _, want := range []string{"email", "home-path", "assignment-secret", "sensitive-key"} {
		if !rules[want] {
			t.Errorf("missing finding for %s: %+v", want, rv.Findings)
		}
	}
	if _, err := a.Export(id, nil, false); err == nil || !strings.Contains(err.Error(), "check the 1 remaining") {
		t.Fatalf("export should refuse unreviewed media, got %v", err)
	}
}

func TestExportRedactsAndVerifies(t *testing.T) {
	a := newTestApp(t)
	id := seed(t, a)
	a.SetReviewed(id, "shot", true)
	out := filepath.Join(t.TempDir(), "r.repropack")
	res, err := a.exportTo(id, out, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Verified || res.Removed < 5 || res.Kept != 0 {
		t.Fatalf("%+v", res)
	}
	raw, _ := os.ReadFile(out)
	r, err := format.Open(out, format.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Verify(); err != nil {
		t.Fatal(err)
	}
	if fs, _ := redact.ScanReport(r, redact.Default()); len(fs) != 0 {
		t.Fatalf("findings remain: %+v", fs)
	}
	_ = raw
	logb, _ := r.ReadAttachment("log", 1<<20)
	if strings.Contains(string(logb), "hunter22") {
		t.Fatal("log attachment not redacted")
	}
	if r.Manifest.Redaction == nil || !r.Manifest.Redaction.Verified || len(r.Manifest.Redaction.ReviewedAttachments) != 1 {
		t.Fatalf("%+v", r.Manifest.Redaction)
	}
	if r.Manifest.Reproduction == nil || len(r.Manifest.Reproduction.Steps) != 2 || len(r.Manifest.Annotations) != 1 {
		t.Fatalf("%+v", r.Manifest)
	}
	st, _ := os.Stat(out)
	if st.Mode().Perm() != 0o600 {
		t.Fatal(st.Mode())
	}
}

func TestExportKeepsChosenValuesHonestly(t *testing.T) {
	a := newTestApp(t)
	id := seed(t, a)
	a.SetReviewed(id, "shot", true)
	out := filepath.Join(t.TempDir(), "k.repropack")
	res, err := a.exportTo(id, out, []string{"title#0"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Kept != 1 || res.Verified {
		t.Fatalf("kept values must mark the report unverified: %+v", res)
	}
	r, _ := format.Open(out, format.Limits{})
	defer r.Close()
	if !strings.Contains(r.Manifest.Title, "jane@example.com") {
		t.Fatal("kept value missing")
	}
	if strings.Contains(r.Manifest.Reproduction.Steps[0], "jane") {
		t.Fatal("unkept value leaked")
	}
}

func TestSaveMarkupReplacesOriginal(t *testing.T) {
	a := newTestApp(t)
	id := seed(t, a)
	var b bytes.Buffer
	png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 8, 6)))
	att, err := a.SaveMarkup(id, "shot", "data:image/png;base64,"+b64(b.Bytes()), 2)
	if err != nil {
		t.Fatal(err)
	}
	if !att.Annotated || !att.Reviewed || att.Width != 8 {
		t.Fatalf("%+v", att)
	}
	if _, err := os.Stat(filepath.Join(a.store.DraftDir(id), "shot.png")); !os.IsNotExist(err) {
		t.Fatal("original screenshot must be deleted")
	}
	d, _ := a.store.GetDraft(id)
	if d.Manual != 2 {
		t.Fatal(d.Manual)
	}
	if _, err := a.SaveMarkup(id, "shot", "bm90IGEgcG5n", 0); err == nil {
		t.Fatal("non-PNG accepted")
	}
	if _, err := a.SaveMarkup(id, "log", "", 0); err == nil {
		t.Fatal("markup on a log accepted")
	}
}

func TestRetentionCleanup(t *testing.T) {
	a := newTestApp(t)
	old := seed(t, a)
	fresh := seed(t, a)
	a.store.db.Exec(`UPDATE drafts SET updated_at=? WHERE id=?`, time.Now().Add(-40*24*time.Hour).UnixMilli(), old)
	os.MkdirAll(filepath.Join(a.store.dir, "drafts", "00000000-0000-7000-8000-000000000000"), 0o700) // orphan
	n, err := a.store.Cleanup(30 * 24 * time.Hour)
	if err != nil || n != 1 {
		t.Fatal(n, err)
	}
	if _, err := a.store.GetDraft(old); err == nil {
		t.Fatal("old draft kept")
	}
	if _, err := os.Stat(a.store.DraftDir(old)); !os.IsNotExist(err) {
		t.Fatal("old files kept")
	}
	if _, err := a.store.GetDraft(fresh); err != nil {
		t.Fatal("fresh draft removed")
	}
	if _, err := os.Stat(filepath.Join(a.store.dir, "drafts", "00000000-0000-7000-8000-000000000000")); !os.IsNotExist(err) {
		t.Fatal("orphan dir kept")
	}
}

func TestMediaHandler(t *testing.T) {
	a := newTestApp(t)
	id := seed(t, a)
	h := &mediaHandler{app: a}
	get := func(p string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", p, nil))
		return w
	}
	w := get("/media/draft/" + id + "/shot")
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("%d %v", w.Code, w.Header())
	}
	for _, p := range []string{"/media/draft/../../etc/passwd", "/media/draft/" + id + "/nope", "/media/report/x/y", "/media/draft/" + id + "/shot/extra"} {
		if c := get(p).Code; c == 200 {
			t.Errorf("%s served", p)
		}
	}
	// Opened reports: verified media is served; a tampered report's is refused.
	a.SetReviewed(id, "shot", true)
	out := filepath.Join(t.TempDir(), "r.repropack")
	if _, err := a.exportTo(id, out, nil); err != nil {
		t.Fatal(err)
	}
	v, err := a.OpenReport(out)
	if err != nil {
		t.Fatal(err)
	}
	if c := get(v.MediaBase + "shot").Code; c != 200 {
		t.Fatal(c)
	}
	a.reports[v.Token].sum.Verified = false
	if c := get(v.MediaBase + "shot").Code; c != http.StatusForbidden {
		t.Fatal(c)
	}
}

func TestIssueLink(t *testing.T) {
	a := newTestApp(t)
	id := seed(t, a)
	a.SetReviewed(id, "shot", true)
	out := filepath.Join(t.TempDir(), "r.repropack")
	a.exportTo(id, out, nil)
	u, err := a.IssueLink(out, "github", "https://github.com/acme/app/")
	if err != nil || !strings.HasPrefix(u, "https://github.com/acme/app/issues/new?") || strings.Contains(u, "jane%40example.com") {
		t.Fatal(u, err)
	}
	if _, err := a.IssueLink(out, "github", "evil.com/x?y"); err == nil {
		t.Fatal("bad repo accepted")
	}
	if err := a.OpenURL("javascript:alert(1)"); err == nil {
		t.Fatal("non-tracker URL accepted")
	}
}

func b64(b []byte) string { return base64Std(b) }
