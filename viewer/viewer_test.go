package viewer

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	stdhtml "html"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/yeegz/repropack/format"
)

func build(t *testing.T, fn func(w *format.Writer)) *format.Report {
	t.Helper()
	var buf bytes.Buffer
	w := format.NewWriter(&buf)
	fn(w)
	if _, err := w.Close(format.Manifest{Title: `<img src=x onerror=alert(1)>`, Producer: format.Producer{Name: "t"},
		Context: map[string]any{"k": `"><script>alert(2)</script>`}}); err != nil {
		t.Fatal(err)
	}
	r, err := format.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()), format.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func render(t *testing.T, r *format.Report) string {
	var out bytes.Buffer
	if err := WriteHTML(&out, r, HTMLOptions{Now: func() time.Time { return time.Unix(0, 0) }}); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func TestHTMLEscapesUntrustedContent(t *testing.T) {
	now := time.Now()
	r := build(t, func(w *format.Writer) {
		w.WriteEvent(format.Event{Time: now, Message: `</script><script>alert(3)</script>`, Attrs: map[string]any{`<b>k</b>`: `javascript:alert(4)`}})
		w.WriteEvent(format.Event{Time: now, Kind: "crash", Level: "fatal", Error: &format.ErrorInfo{Message: `<iframe src=//evil>`, Raw: "<svg onload=alert(5)>"}})
		w.AddAttachment(format.Attachment{ID: "svg", MediaType: "image/svg+xml", Role: "screenshot"}, strings.NewReader(`<svg onload="alert(6)"/>`))
		w.AddAttachment(format.Attachment{ID: "fakepng", MediaType: "image/png", Role: "screenshot"}, strings.NewReader(`<html><script>alert(7)</script>`))
		w.AddAttachment(format.Attachment{ID: "page", MediaType: "text/html", Role: "other"}, strings.NewReader(`<script>alert(8)</script>`))
	})
	html := render(t, r)
	// Only one script element: ours.
	if n := strings.Count(html, "<script"); n != 1 {
		t.Fatalf("expected exactly one <script>, found %d", n)
	}
	for _, bad := range []string{"<img src=x", "<iframe", "<svg onload", "alert(6)", "alert(7)", "alert(8)", `href="javascript`} {
		if strings.Contains(html, bad) {
			t.Errorf("unescaped content %q in output", bad)
		}
	}
	if !strings.Contains(html, "content does not match its declared type") {
		t.Error("sniff mismatch not reported")
	}
	if !strings.Contains(html, "not displayed in summaries") {
		t.Error("svg/html should not be embedded")
	}
}

func TestCSPHashMatchesScript(t *testing.T) {
	r := build(t, func(w *format.Writer) { w.WriteEvent(format.Event{Time: time.Now(), Message: "x"}) })
	html := render(t, r)
	m := regexp.MustCompile(`(?s)<script>(.*)</script>`).FindStringSubmatch(html)
	if m == nil {
		t.Fatal("no script")
	}
	sum := sha256.Sum256([]byte(m[1]))
	want := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
	if !strings.Contains(stdhtml.UnescapeString(html), want) {
		t.Fatalf("CSP does not allow the inline script (want %s)", want)
	}
	if !strings.Contains(stdhtml.UnescapeString(html), "default-src 'none'") {
		t.Fatal("CSP missing default-src none")
	}
}

func TestTamperedReportHidesMedia(t *testing.T) {
	r := build(t, func(w *format.Writer) {
		w.WriteEvent(format.Event{Time: time.Now(), Message: "x"})
	})
	r.Manifest.Events.SHA256 = strings.Repeat("0", 64) // simulate tamper detected at read time
	s, _ := Summarize(r, Options{})
	if s.Verified {
		t.Fatal("should not verify")
	}
	if !strings.Contains(render(t, r), "may have been altered") {
		t.Fatal("no tamper banner")
	}
}

func TestSummaryHeadlineAndLanes(t *testing.T) {
	t0 := time.Now()
	r := build(t, func(w *format.Writer) {
		w.WriteEvent(format.Event{Time: t0, Message: "a"})
		w.WriteEvent(format.Event{Time: t0.Add(time.Second), Kind: "error", Level: "error", Error: &format.ErrorInfo{Message: "disk full"}})
		w.WriteEvent(format.Event{Time: t0.Add(2 * time.Second), Kind: "metric", Metric: &format.Metric{Name: "m", Value: 3}})
	})
	s, _ := Summarize(r, Options{})
	if !strings.Contains(s.Headline, "disk full") || s.Errors != 1 || len(s.Metrics) != 1 || len(s.Lanes) != 2 {
		t.Fatalf("%+v", s)
	}
}

func TestRelative(t *testing.T) {
	t0 := time.Unix(0, 0)
	for d, want := range map[time.Duration]string{0: "0:00.000", 1500 * time.Millisecond: "0:01.500", 61 * time.Minute: "1:01:00.000", -time.Second: "−0:01.000"} {
		if got := Relative(t0, t0.Add(d)); got != want {
			t.Errorf("%v: %q want %q", d, got, want)
		}
	}
}
