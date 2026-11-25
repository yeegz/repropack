package sdk

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/yeegz/repropack/format"
)

func BenchmarkLogNoAttrs(b *testing.B) {
	rec := New(Options{})
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		rec.Log(format.LevelInfo, "request handled", nil)
	}
}

func BenchmarkLogNoAttrsNoRedaction(b *testing.B) {
	rec := New(Options{Policy: NoRedaction})
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		rec.Log(format.LevelInfo, "request handled", nil)
	}
}

func BenchmarkLog4Attrs(b *testing.B) {
	rec := New(Options{})
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		rec.Log(format.LevelInfo, "request handled", map[string]any{"route": "/api/items", "status": 200, "ms": 12.5, "user": "u_123"})
	}
}

func BenchmarkLogFilteredByLevel(b *testing.B) {
	rec := New(Options{MinLevel: format.LevelInfo})
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		rec.Log(format.LevelDebug, "noise", nil)
	}
}

func BenchmarkLogParallel(b *testing.B) {
	rec := New(Options{})
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			rec.Log(format.LevelInfo, "request handled", nil)
		}
	})
}

func BenchmarkSlogBaseline(b *testing.B) {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		log.Info("request handled", "route", "/api/items", "status", 200)
	}
}

func BenchmarkSlogTee(b *testing.B) {
	rec := New(Options{})
	log := slog.New(rec.Handler(slog.NewJSONHandler(io.Discard, nil)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		log.Info("request handled", "route", "/api/items", "status", 200)
	}
}

func BenchmarkWriteReport2000Events(b *testing.B) {
	rec := New(Options{MaxEvents: 2000})
	for i := 0; i < 2000; i++ {
		rec.Log(format.LevelInfo, "request handled", map[string]any{"route": "/api/items", "status": 200})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := rec.WriteReport(context.Background(), io.Discard, ReportOptions{Title: "bench"}); err != nil {
			b.Fatal(err)
		}
	}
}
