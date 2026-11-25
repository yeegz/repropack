package sdk

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yeegz/repropack/format"
	"github.com/yeegz/repropack/redact"
)

func readBack(t *testing.T, b []byte) (*format.Report, []format.Event) {
	t.Helper()
	r, err := format.NewReader(bytes.NewReader(b), int64(len(b)), format.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Verify(); err != nil {
		t.Fatal(err)
	}
	ev, err := r.ReadEvents()
	if err != nil {
		t.Fatal(err)
	}
	return r, ev
}

func TestRingBufferBoundsByCount(t *testing.T) {
	rec := New(Options{MaxEvents: 10})
	for i := 0; i < 25; i++ {
		rec.Log(format.LevelInfo, fmt.Sprint("e", i), nil)
	}
	s := rec.Snapshot()
	if len(s) != 10 || s[0].Message != "e15" || s[9].Message != "e24" {
		t.Fatalf("got %d, first %q", len(s), s[0].Message)
	}
	if st := rec.Stats(); st.Dropped != 15 || st.Recorded != 25 {
		t.Fatalf("%+v", st)
	}
}

func TestRingBufferBoundsByBytes(t *testing.T) {
	rec := New(Options{MaxEvents: 1000, MaxBytes: 4096})
	big := strings.Repeat("x", 1000)
	for i := 0; i < 50; i++ {
		rec.Log(format.LevelInfo, big, nil)
	}
	if st := rec.Stats(); st.Bytes > 4096 || st.Buffered == 0 || st.Buffered > 4 {
		t.Fatalf("%+v", st)
	}
	rec.Log(format.LevelInfo, strings.Repeat("y", 10000), nil) // larger than the whole buffer
	if st := rec.Stats(); st.Bytes > 4096 {
		t.Fatalf("%+v", st)
	}
}

func TestLevelAndFilters(t *testing.T) {
	rec := New(Options{MinLevel: format.LevelInfo, Filters: []Filter{func(e *format.Event) bool { return e.Source != "noisy" }}})
	rec.Log(format.LevelDebug, "hidden", nil)
	rec.Record(format.Event{Level: format.LevelInfo, Source: "noisy", Message: "hidden"})
	rec.Log(format.LevelWarn, "kept", nil)
	if s := rec.Snapshot(); len(s) != 1 || s[0].Message != "kept" {
		t.Fatal(s)
	}
	if rec.Stats().Filtered != 2 {
		t.Fatal(rec.Stats())
	}
}

func TestRedactsAtRecordTime(t *testing.T) {
	rec := New(Options{})
	rec.Log(format.LevelInfo, "user bob@example.com signed in", map[string]any{"authorization": "Bearer abc", "path": "/Users/bob/x"})
	e := rec.Snapshot()[0]
	if strings.Contains(e.Message, "bob@") || e.Attrs["authorization"] != "[REDACTED:sensitive-key]" || e.Attrs["path"] != "/Users/[user]/x" {
		t.Fatalf("%+v", e)
	}
	var buf bytes.Buffer
	rec.SetContext("account_email", "bob@example.com")
	m, err := rec.WriteReport(context.Background(), &buf, ReportOptions{Title: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "bob@example.com") {
		t.Fatal("email leaked into archive")
	}
	if m.Redaction == nil || len(m.Redaction.Rules) == 0 {
		t.Fatal(m.Redaction)
	}
	r, _ := readBack(t, buf.Bytes())
	if fs, _ := redact.ScanReport(r, redact.Default()); len(fs) != 0 {
		t.Fatalf("remaining findings %+v", fs)
	}
}

func TestNoRedactionOptOut(t *testing.T) {
	rec := New(Options{Policy: NoRedaction})
	rec.Log(format.LevelInfo, "bob@example.com", nil)
	if rec.Snapshot()[0].Message != "bob@example.com" {
		t.Fatal("should not redact")
	}
}

func TestSlogTee(t *testing.T) {
	rec := New(Options{})
	var out bytes.Buffer
	log := slog.New(rec.Handler(slog.NewJSONHandler(&out, nil)))
	log = log.With("svc", "api").WithGroup("req").With("id", 7)
	log.Info("handled", "status", 200, slog.Group("user", "plan", "pro"))
	log.Error("failed", "error", fmt.Errorf("wrap: %w", os.ErrNotExist))
	log.Debug("dropped by next but kept by recorder")
	if !strings.Contains(out.String(), `"req":{"id":7,"status":200`) {
		t.Fatalf("next handler output changed: %s", out.String())
	}
	if strings.Contains(out.String(), "dropped by next") {
		t.Fatal("debug should not reach default JSON handler")
	}
	s := rec.Snapshot()
	if len(s) != 3 {
		t.Fatalf("%d events", len(s))
	}
	a := s[0].Attrs
	if a["svc"] != "api" || a["req.id"] != int64(7) || a["req.status"] != int64(200) || a["req.user.plan"] != "pro" {
		t.Fatalf("attrs %+v", a)
	}
	if s[1].Error == nil || s[1].Error.Cause == nil || s[1].Level != "error" {
		t.Fatalf("%+v", s[1])
	}
}

func TestErrorChainAndStack(t *testing.T) {
	rec := New(Options{})
	base := errors.New("disk full")
	rec.Error(fmt.Errorf("save invoice: %w", base), "", nil)
	e := rec.Snapshot()[0]
	if e.Error.Cause == nil || e.Error.Cause.Message != "disk full" {
		t.Fatalf("%+v", e.Error)
	}
	if len(e.Error.Stack) == 0 || !strings.Contains(e.Error.Stack[0].Function, "TestErrorChainAndStack") {
		t.Fatalf("stack %+v", e.Error.Stack)
	}
}

func TestConcurrentRecording(t *testing.T) {
	rec := New(Options{MaxEvents: 500})
	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				rec.Log(format.LevelInfo, "x", map[string]any{"g": g})
				if i%250 == 0 {
					var buf bytes.Buffer
					if _, err := rec.WriteReport(context.Background(), &buf, ReportOptions{}); err != nil {
						t.Error(err)
					}
				}
			}
		}(g)
	}
	wg.Wait()
	st := rec.Stats()
	if st.Buffered != 500 || st.Recorded < 16000 {
		t.Fatalf("%+v", st)
	}
	s := rec.Snapshot()
	for i := 1; i < len(s); i++ {
		if s[i].Seq <= s[i-1].Seq {
			t.Fatal("seq not increasing")
		}
	}
}

type slowProvider struct{}

func (slowProvider) Name() string { return "slow" }
func (slowProvider) Collect(ctx context.Context) (ProviderData, error) {
	<-ctx.Done()
	return ProviderData{}, ctx.Err()
}

func TestProvidersTimeoutPanicAndError(t *testing.T) {
	rec := New(Options{Providers: []Provider{
		RuntimeProvider(),
		slowProvider{},
		ProviderFunc{ID: "bad", Fn: func(context.Context) (ProviderData, error) { return ProviderData{}, errors.New("nope") }},
		ProviderFunc{ID: "boom", Fn: func(context.Context) (ProviderData, error) { panic("x") }},
	}})
	var buf bytes.Buffer
	start := time.Now()
	m, err := rec.WriteReport(context.Background(), &buf, ReportOptions{Timeout: 200 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("timeout not respected")
	}
	if m.Context["go.goroutines"] == nil {
		t.Fatalf("runtime context missing: %+v", m.Context)
	}
	_, ev := readBack(t, buf.Bytes())
	var msgs []string
	for _, e := range ev {
		msgs = append(msgs, e.Message)
	}
	joined := strings.Join(msgs, "|")
	for _, want := range []string{"timed out", "provider bad failed", "provider boom failed"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %s", want, joined)
		}
	}
}

func TestAttachmentsAreRedacted(t *testing.T) {
	rec := New(Options{})
	var buf bytes.Buffer
	_, err := rec.WriteReport(context.Background(), &buf, ReportOptions{Attachments: []Attachment{
		BytesAttachment("app.log", "text/plain", format.RoleLog, []byte("token=supersecretvalue\n")),
	}})
	if err != nil {
		t.Fatal(err)
	}
	r, _ := readBack(t, buf.Bytes())
	b, _ := r.ReadAttachment(r.Manifest.Attachments[0].ID, 1<<20)
	if strings.Contains(string(b), "supersecret") {
		t.Fatal(string(b))
	}
}

func TestWriteFileAtomicAndPrivate(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "r.repropack")
	rec := New(Options{})
	if _, err := rec.WriteFile(p, ReportOptions{}); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(p)
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatal(err, st.Mode())
	}
	ents, _ := os.ReadDir(dir)
	if len(ents) != 1 {
		t.Fatal("temp file left behind")
	}
}

// TestCapturePanic runs a child process that panics and checks the crash report.
func TestCapturePanic(t *testing.T) {
	if os.Getenv("RP_PANIC_CHILD") != "" {
		rec := New(Options{App: format.AppContext{Name: "child"}})
		rec.Log(format.LevelInfo, "about to fail", nil)
		defer rec.CapturePanic(os.Getenv("RP_PANIC_CHILD"))
		var m map[string]int
		m["x"] = 1 // nil map write
		return
	}
	out := filepath.Join(t.TempDir(), "crash.repropack")
	cmd := exec.Command(os.Args[0], "-test.run=^TestCapturePanic$")
	cmd.Env = append(os.Environ(), "RP_PANIC_CHILD="+out)
	b, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "assignment to entry in nil map") {
		t.Fatalf("child should re-panic: %v\n%s", err, b)
	}
	r, err := format.Open(out, format.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	ev, _ := r.ReadEvents()
	last := ev[len(ev)-1]
	if last.Kind != format.KindCrash || last.Error == nil || *last.Error.Handled {
		t.Fatalf("%+v", last)
	}
	if len(last.Error.Stack) == 0 || !strings.Contains(last.Error.Stack[0].Function, "TestCapturePanic") {
		t.Fatalf("crash stack %+v", last.Error.Stack)
	}
	if !strings.HasPrefix(r.Manifest.Title, "Crash: ") || len(r.Manifest.Attachments) != 1 {
		t.Fatalf("%+v", r.Manifest)
	}
}
