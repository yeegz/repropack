// Existing slog: keep your logger exactly as it is and tee into ReproPack.
package main

import (
	"log/slog"
	"os"

	"github.com/yeegz/repropack/format"
	"github.com/yeegz/repropack/sdk"
)

func main() {
	rec := sdk.New(sdk.Options{
		App:       format.AppContext{Name: "worker", Version: "0.9.1"},
		MaxEvents: 500,               // bounded: oldest events are dropped first
		MinLevel:  format.LevelDebug, // record debug even though stderr shows info+
	})
	// Before: logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	logger := slog.New(rec.Handler(slog.NewJSONHandler(os.Stderr, nil)))
	slog.SetDefault(logger)

	slog.Debug("cache warm", "keys", 1200) // only in the report
	slog.Info("job started", "job", "resize", "user_email", "ana@example.com")
	slog.Warn("retrying upload", "attempt", 2, "url", "https://s3.example.com/b?signature=abcd1234efgh")

	if _, err := rec.WriteFile("worker.repropack", sdk.ReportOptions{Title: "Uploads retry forever"}); err != nil {
		slog.Error("could not write report", "error", err)
	}
	// The email and the signed URL secret are redacted in the report; your
	// stderr output is unchanged.
}
