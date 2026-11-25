// Basic: record a few events and write a report.
package main

import (
	"errors"
	"fmt"
	"log"

	"github.com/yeegz/repropack/format"
	"github.com/yeegz/repropack/sdk"
)

func main() {
	rec := sdk.New(sdk.Options{App: format.AppContext{Name: "notes", Version: "2.3.0"}})
	rec.Breadcrumb("opened note", map[string]any{"note_id": "n_42"})
	rec.Log(format.LevelInfo, "sync started", map[string]any{"items": 18})
	rec.Metric("sync.duration", 812, "ms")
	rec.Error(fmt.Errorf("save note: %w", errors.New("database is locked")), "", nil)

	m, err := rec.WriteFile("basic.repropack", sdk.ReportOptions{
		Title: "Note does not save after sync",
		Reproduction: &format.Reproduction{
			Steps:    []string{"Open any note", "Wait for sync", "Edit and press ⌘S"},
			Expected: "The note saves",
			Actual:   "Nothing happens; the edit is lost on restart",
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote basic.repropack: %d events, digest %s\n", m.Events.Count, m.ContentDigest)
}
