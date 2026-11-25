package format

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
)

// buildRaw wraps arbitrary events.jsonl bytes in an archive with a correct
// integrity table, so fuzzers reach the event parser.
func buildRaw(t testing.TB, events []byte) []byte {
	sum := sha256.Sum256(events)
	m := Manifest{Format: FormatName, SpecVersion: SpecVersion, ReportID: "r", CreatedAt: t0, Title: "fuzz",
		Producer: Producer{Name: "fuzz"}, Events: EventsInfo{Path: EventsPath, SHA256: hex.EncodeToString(sum[:]), Size: int64(len(events)), Count: 1}}
	m.ContentDigest = ComputeContentDigest(&m)
	mb, _ := json.Marshal(m)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create(EventsPath)
	w.Write(events)
	w, _ = zw.Create(ManifestPath)
	w.Write(mb)
	zw.Close()
	return buf.Bytes()
}
