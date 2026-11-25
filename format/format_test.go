package format

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)

func sample(t testing.TB) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := NewWriter(&buf)
	for i := 0; i < 5; i++ {
		if err := w.WriteEvent(Event{Time: t0.Add(time.Duration(i) * time.Second), Kind: KindLog, Level: LevelInfo, Message: "step", Attrs: map[string]any{"i": i}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.WriteEvent(Event{Time: t0.Add(6 * time.Second), Kind: KindError, Level: LevelError, Message: "boom", Error: &ErrorInfo{Type: "*errors.errorString", Message: "boom"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.AddAttachment(Attachment{ID: "shot1", MediaType: "image/png", Role: RoleScreenshot}, strings.NewReader("\x89PNG fake")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Close(Manifest{Title: "Save button does nothing", Producer: Producer{Name: "test", Version: "0"}, CreatedAt: t0,
		Annotations: []Annotation{{ID: "n1", Text: "here", AttachmentID: "shot1"}}}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func open(t testing.TB, b []byte) (*Report, error) {
	t.Helper()
	return NewReader(bytes.NewReader(b), int64(len(b)), Limits{})
}

func TestRoundTrip(t *testing.T) {
	b := sample(t)
	r, err := open(t, b)
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
	if len(ev) != 6 || ev[5].Error.Message != "boom" || ev[0].Seq != 1 {
		t.Fatalf("unexpected events %+v", ev)
	}
	if r.Manifest.Events.Count != 6 || r.Manifest.Events.First == nil || !r.Manifest.Events.First.Equal(t0) {
		t.Fatalf("bad events info %+v", r.Manifest.Events)
	}
	data, err := r.ReadAttachment("shot1", 1<<20)
	if err != nil || string(data) != "\x89PNG fake" {
		t.Fatalf("attachment: %q %v", data, err)
	}
	if r.Manifest.Attachments[0].Path != "attachments/shot1.png" {
		t.Fatal(r.Manifest.Attachments[0].Path)
	}
}

// rebuild copies an archive, letting mutate change names/content.
func rebuild(t testing.TB, b []byte, mutate func(name string, data []byte) (string, []byte, bool)) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, f := range zr.File {
		rc, _ := f.Open()
		data, _ := io.ReadAll(rc)
		rc.Close()
		name, nd, keep := mutate(f.Name, data)
		if !keep {
			continue
		}
		w, _ := zw.Create(name)
		w.Write(nd)
	}
	zw.Close()
	return out.Bytes()
}

func addEntry(t testing.TB, b []byte, name string, data []byte) []byte {
	zr, _ := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, f := range zr.File {
		zw.Copy(f)
	}
	w, err := zw.CreateRaw(&zip.FileHeader{Name: name, Method: zip.Store, CompressedSize64: uint64(len(data)), UncompressedSize64: uint64(len(data))})
	if err != nil {
		t.Fatal(err)
	}
	w.Write(data)
	zw.Close()
	return out.Bytes()
}

func expectInvalid(t *testing.T, b []byte, want string) {
	t.Helper()
	r, err := open(t, b)
	if err == nil {
		err = r.Verify()
	}
	if err == nil {
		t.Fatalf("expected failure containing %q", want)
	}
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("error does not wrap ErrInvalid: %v", err)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not mention %q", err, want)
	}
}

func TestMaliciousArchives(t *testing.T) {
	base := sample(t)
	cases := map[string]struct {
		b    []byte
		want string
	}{
		"zip slip":       {addEntry(t, base, "../../etc/passwd", []byte("x")), "'..' segment"},
		"absolute":       {addEntry(t, base, "/tmp/x", []byte("x")), "absolute"},
		"backslash":      {addEntry(t, base, `attachments\..\x`, []byte("x")), "forbidden character"},
		"nested dir":     {addEntry(t, base, "attachments/a/b.png", []byte("x")), "outside the allowed layout"},
		"unlisted entry": {addEntry(t, base, "attachments/extra.png", []byte("x")), "not listed"},
		"duplicate":      {addEntry(t, base, "events.jsonl", []byte("x")), "duplicate"},
		"control char":   {addEntry(t, base, "attachments/a\x01.png", []byte("x")), "control character"},
		"script entry":   {addEntry(t, base, "run.sh", []byte("rm -rf /")), "outside the allowed layout"},
		"tampered events": {rebuild(t, base, func(n string, d []byte) (string, []byte, bool) {
			if n == EventsPath {
				d = bytes.Replace(d, []byte("boom"), []byte("bang"), 1)
			}
			return n, d, true
		}), "SHA-256 mismatch"},
		"tampered attachment": {rebuild(t, base, func(n string, d []byte) (string, []byte, bool) {
			if strings.HasPrefix(n, AttachmentsDir) {
				d = []byte("\x89PNG fakf")
			}
			return n, d, true
		}), "SHA-256 mismatch"},
		"size mismatch": {rebuild(t, base, func(n string, d []byte) (string, []byte, bool) {
			if strings.HasPrefix(n, AttachmentsDir) {
				d = append(d, 'x')
			}
			return n, d, true
		}), "does not match manifest"},
		"missing manifest": {rebuild(t, base, func(n string, d []byte) (string, []byte, bool) { return n, d, n != ManifestPath }), "missing"},
		"missing events":   {rebuild(t, base, func(n string, d []byte) (string, []byte, bool) { return n, d, n != EventsPath }), "missing from archive"},
		"future major": {rebuild(t, base, func(n string, d []byte) (string, []byte, bool) {
			if n == ManifestPath {
				d = bytes.Replace(d, []byte(`"spec_version": "1.0"`), []byte(`"spec_version": "2.0"`), 1)
			}
			return n, d, true
		}), "major version 2"},
		"not json": {rebuild(t, base, func(n string, d []byte) (string, []byte, bool) {
			if n == ManifestPath {
				d = []byte("{nope")
			}
			return n, d, true
		}), "malformed JSON"},
		"not zip": {[]byte("PK\x03\x04 definitely not"), "not a ZIP"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) { expectInvalid(t, c.b, c.want) })
	}
}

func TestDecompressionBomb(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	w.WriteEvent(Event{Time: t0, Message: "x"})
	if _, err := w.AddAttachment(Attachment{ID: "bomb", MediaType: "text/plain", Role: RoleLog}, bytes.NewReader(make([]byte, 64<<20))); err != nil {
		t.Fatal(err)
	}
	w.Close(Manifest{Title: "bomb", Producer: Producer{Name: "t"}})
	expectInvalid(t, buf.Bytes(), "compression ratio")
}

func TestLimitsTotal(t *testing.T) {
	b := sample(t)
	_, err := NewReader(bytes.NewReader(b), int64(len(b)), Limits{MaxArchiveBytes: 10})
	if err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatal(err)
	}
}

func TestEventOrderingRejected(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	if err := w.WriteEvent(Event{Seq: 5, Time: t0}); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteEvent(Event{Seq: 5, Time: t0}); err == nil {
		t.Fatal("expected seq error")
	}
	if err := w.WriteEvent(Event{Seq: 6}); err == nil {
		t.Fatal("expected timestamp error")
	}
}

func TestWriterRejectsBadIDs(t *testing.T) {
	w := NewWriter(io.Discard)
	for _, id := range []string{"", "../x", "a/b", strings.Repeat("a", 65), "ünï"} {
		if _, err := w.AddAttachment(Attachment{ID: id}, strings.NewReader("")); err == nil {
			t.Fatalf("id %q accepted", id)
		}
	}
}

func TestEmptyReport(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	m, err := w.Close(Manifest{Title: "empty", Producer: Producer{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := open(t, buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Verify(); err != nil || m.Events.Count != 0 {
		t.Fatal(err)
	}
}

func TestNewIDIsV7(t *testing.T) {
	id := NewID()
	if len(id) != 36 || id[14] != '7' {
		t.Fatal(id)
	}
	if NewID() == id {
		t.Fatal("not unique")
	}
}

func FuzzReader(f *testing.F) {
	f.Add(sample(f))
	f.Add([]byte("PK\x05\x06" + strings.Repeat("\x00", 18)))
	f.Fuzz(func(t *testing.T, b []byte) {
		r, err := NewReader(bytes.NewReader(b), int64(len(b)), Limits{MaxTotalBytes: 8 << 20, MaxEventsBytes: 4 << 20, MaxAttachmentBytes: 4 << 20})
		if err != nil {
			return
		}
		_ = r.Verify()
		_, _ = r.ReadEvents()
	})
}

func FuzzEventLine(f *testing.F) {
	f.Add([]byte(`{"seq":1,"ts":"2026-11-25T10:00:00Z","kind":"log","msg":"hi"}`))
	f.Add([]byte(`{"seq":1,"ts":"2026-11-25T10:00:00Z","kind":"error","error":{"message":"x","cause":{"message":"y"}}}`))
	f.Fuzz(func(t *testing.T, line []byte) {
		b := buildRaw(t, line)
		r, err := NewReader(bytes.NewReader(b), int64(len(b)), Limits{})
		if err != nil {
			return
		}
		_, _ = r.ReadEvents()
	})
}
