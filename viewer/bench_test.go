package viewer

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/yeegz/repropack/format"
)

func big(b *testing.B, n int) *format.Report {
	var buf bytes.Buffer
	w := format.NewWriter(&buf)
	t := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		e := format.Event{Time: t.Add(time.Duration(i) * 10 * time.Millisecond), Level: "info", Message: "GET /api/items 200", Attrs: map[string]any{"ms": 12}}
		if i%100 == 0 {
			e.Kind, e.Level, e.Error = "error", "error", &format.ErrorInfo{Message: "timeout"}
		}
		if i%10 == 0 {
			e = format.Event{Time: e.Time, Kind: "metric", Metric: &format.Metric{Name: "cpu", Value: float64(i % 100), Unit: "%"}}
		}
		w.WriteEvent(e)
	}
	w.Close(format.Manifest{Title: "big", Producer: format.Producer{Name: "b"}})
	r, err := format.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()), format.Limits{})
	if err != nil {
		b.Fatal(err)
	}
	return r
}

func BenchmarkSummarize50k(b *testing.B) {
	r := big(b, 50_000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Summarize(r, Options{MaxEvents: 50_000}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWriteHTML50k(b *testing.B) {
	r := big(b, 50_000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := WriteHTML(io.Discard, r, HTMLOptions{}); err != nil {
			b.Fatal(err)
		}
	}
}
