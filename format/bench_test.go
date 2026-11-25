package format

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"
)

func buildN(b *testing.B, n int, attach int) []byte {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	t := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		w.WriteEvent(Event{Time: t.Add(time.Duration(i) * time.Millisecond), Kind: KindLog, Level: LevelInfo, Source: "api",
			Message: "GET /api/items 200", Attrs: map[string]any{"route": "/api/items", "status": 200, "ms": 12.5}})
	}
	if attach > 0 {
		w.AddAttachment(Attachment{ID: "shot", MediaType: "image/png", Role: RoleScreenshot}, bytes.NewReader(bytes.Repeat([]byte{0x89, 'P', 'N', 'G'}, attach/4)))
	}
	w.Close(Manifest{Title: "bench", Producer: Producer{Name: "bench"}})
	return buf.Bytes()
}

func BenchmarkWrite10kEvents(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buildN(b, 10_000, 0)
	}
}

func BenchmarkOpenVerify10kEvents2MB(b *testing.B) {
	data := buildN(b, 10_000, 2<<20)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r, err := NewReader(bytes.NewReader(data), int64(len(data)), Limits{})
		if err != nil {
			b.Fatal(err)
		}
		if err := r.Verify(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReadEvents10k(b *testing.B) {
	data := buildN(b, 10_000, 0)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r, _ := NewReader(bytes.NewReader(data), int64(len(data)), Limits{})
		if _, err := r.ReadEvents(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRejectZipSlip(b *testing.B) {
	data := addEntry(b, buildN(b, 100, 0), "../../evil", []byte("x"))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := NewReader(bytes.NewReader(data), int64(len(data)), Limits{}); err == nil {
			b.Fatal("accepted")
		}
	}
}

var _ = io.Discard
var _ = strings.Repeat
