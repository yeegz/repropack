package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var bin string

func TestMain(m *testing.M) {
	dir, _ := os.MkdirTemp("", "rpcli")
	bin = filepath.Join(dir, "repropack")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		panic(string(out))
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func run(t *testing.T, wantCode int, args ...string) string {
	t.Helper()
	cmd := exec.Command(bin, args...)
	out, err := cmd.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	if code != wantCode {
		t.Fatalf("repropack %v: exit %d want %d\n%s", args, code, wantCode, out)
	}
	return string(out)
}

func TestEndToEnd(t *testing.T) {
	d := t.TempDir()
	logf := filepath.Join(d, "app.log")
	os.WriteFile(logf, []byte(strings.Join([]string{
		`2026-11-25T10:00:00.120Z INFO server started on :8080`,
		`2026-11-25T10:00:03.400Z WARN slow query took 1200ms user=ana@example.com`,
		`{"time":"2026-11-25T10:00:04Z","level":"ERROR","msg":"payment failed","error":"gateway 502","token":"abc123secret"}`,
		`time=2026-11-25T10:00:05Z level=debug msg="cache miss" key=k1`,
		`2026-11-25T10:00:06Z ERROR panic: boom`,
		`goroutine 1 [running]:`,
		`	/Users/ana/src/app/main.go:12 +0x1d`,
	}, "\n")), 0o600)
	png := filepath.Join(d, "shot.png")
	os.WriteFile(png, []byte("\x89PNG\r\n\x1a\nfake"), 0o600)

	report := filepath.Join(d, "r.repropack")
	out := run(t, 0, "create", "-o", report, "--title", "Payment fails for ana@example.com", "--log", logf, "--attach", png,
		"--step", "Open checkout", "--step", "Press Pay", "--expected", "Receipt", "--actual", "Error", "--note", "since 4.2", "--app", "shop", "--app-version", "4.2.1")
	if !strings.Contains(out, "5 events") {
		t.Fatalf("create: %s", out)
	}
	run(t, 0, "validate", report)
	js := run(t, 0, "validate", "--json", report)
	if !strings.Contains(js, `"valid":true`) {
		t.Fatal(js)
	}
	sum := run(t, 0, "inspect", report)
	for _, want := range []string{"Payment fails for [REDACTED:email]", "shop 4.2.1", "Integrity    verified", "1. Open checkout"} {
		if !strings.Contains(sum, want) {
			t.Errorf("inspect missing %q:\n%s", want, sum)
		}
	}
	ev := run(t, 0, "inspect", "--events", "--level", "error", report)
	if strings.Count(ev, "\n") != 2 || strings.Contains(ev, "ana@") || strings.Contains(ev, "abc123secret") {
		t.Fatalf("events: %q", ev)
	}
	full := run(t, 0, "inspect", "--events", "--json", report)
	if strings.Contains(full, "/Users/ana") {
		t.Fatal("home path leaked")
	}
	// scan finds nothing in an already-redacted report except the unreviewed image hint.
	sc := run(t, 0, "scan", report)
	if !strings.Contains(sc, "No sensitive values") || !strings.Contains(sc, "need a human check") {
		t.Fatal(sc)
	}
	// An unredacted report: scan fails, redact fixes and verifies.
	raw := filepath.Join(d, "raw.repropack")
	run(t, 0, "create", "-o", raw, "--title", "t", "--log", logf, "--no-redact")
	run(t, 1, "scan", raw)
	clean := filepath.Join(d, "clean.repropack")
	rd := run(t, 0, "redact", raw, "-o", clean)
	if !strings.Contains(rd, "verified clean") {
		t.Fatal(rd)
	}
	run(t, 0, "scan", clean)
	run(t, 2, "redact", raw, "-o", raw)

	html := filepath.Join(d, "r.html")
	run(t, 0, "html", report, "-o", html)
	b, _ := os.ReadFile(html)
	if !strings.Contains(string(b), "Content-Security-Policy") || !strings.Contains(string(b), "data:image/png;base64") {
		t.Fatal("html incomplete")
	}

	x := filepath.Join(d, "x")
	run(t, 0, "extract", report, "-d", x)
	for _, f := range []string{"manifest.json", "events.jsonl"} {
		if _, err := os.Stat(filepath.Join(x, f)); err != nil {
			t.Fatal(err)
		}
	}
	run(t, 1, "extract", report, "-d", x) // not empty

	otl := filepath.Join(d, "otlp.json")
	run(t, 0, "export", "--otlp", report, "-o", otl)
	back := filepath.Join(d, "back.repropack")
	if o := run(t, 0, "import", "--otlp", otl, "-o", back); !strings.Contains(o, "5 events") {
		t.Fatal(o)
	}
	run(t, 0, "validate", back)

	if s := run(t, 0, "schema", "event"); !strings.Contains(s, `"$schema"`) {
		t.Fatal(s)
	}
	run(t, 2, "nope")
	run(t, 2, "validate")
}

func TestInvalidExamples(t *testing.T) {
	files, _ := filepath.Glob("../../spec/examples/invalid/*.repropack")
	if len(files) < 4 {
		t.Fatal("missing invalid examples")
	}
	for _, f := range files {
		run(t, 1, "validate", f)
	}
	for _, f := range []string{"../../spec/examples/minimal.repropack", "../../spec/examples/checkout-crash.repropack"} {
		run(t, 0, "validate", f)
	}
}

func TestTerminalEscapesStripped(t *testing.T) {
	if got := sanitize("ok\x1b[31mred\x07"); strings.ContainsAny(got, "\x1b\x07") {
		t.Fatal(got)
	}
}
