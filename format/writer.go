package format

import (
	"archive/zip"
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"sort"
	"strings"
	"time"
)

// Writer streams a .repropack archive. Usage:
//
//	w := format.NewWriter(out)
//	for _, e := range events { w.WriteEvent(e) }
//	w.AddAttachment(att, reader)
//	w.Close(manifest)
//
// Events must be written before attachments. The manifest is written last,
// after every hash is known; readers locate it via the ZIP central directory.
type Writer struct {
	zw *zip.Writer

	events     io.Writer
	eventsHash hash.Hash
	eventsCnt  *countWriter
	eventsBuf  *bufio.Writer
	nEvents    int
	lastSeq    uint64
	first      *time.Time
	last       *time.Time
	eventsDone bool

	attachments []Attachment
	ids         map[string]bool
	closed      bool
	enc         *json.Encoder
}

type countWriter struct {
	w io.Writer
	n int64
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// NewWriter starts a new archive on out.
func NewWriter(out io.Writer) *Writer {
	return &Writer{zw: zip.NewWriter(out), ids: map[string]bool{}}
}

func (w *Writer) openEvents() error {
	if w.events != nil {
		return nil
	}
	f, err := w.zw.CreateHeader(&zip.FileHeader{Name: EventsPath, Method: zip.Deflate, Modified: time.Unix(0, 0).UTC()})
	if err != nil {
		return err
	}
	w.eventsHash = sha256.New()
	w.eventsCnt = &countWriter{w: io.MultiWriter(f, w.eventsHash)}
	w.eventsBuf = bufio.NewWriterSize(w.eventsCnt, 64<<10)
	w.events = w.eventsBuf
	w.enc = json.NewEncoder(w.eventsBuf)
	w.enc.SetEscapeHTML(false)
	return nil
}

// WriteEvent appends an event. If e.Seq is zero it is assigned; otherwise it
// must be strictly greater than the previous event's Seq.
func (w *Writer) WriteEvent(e Event) error {
	if w.closed || w.eventsDone {
		return errors.New("repropack: events are closed")
	}
	if err := w.openEvents(); err != nil {
		return err
	}
	if e.Seq == 0 {
		e.Seq = w.lastSeq + 1
	} else if e.Seq <= w.lastSeq {
		return fmt.Errorf("repropack: event seq %d not increasing (previous %d)", e.Seq, w.lastSeq)
	}
	if e.Kind == "" {
		e.Kind = KindLog
	}
	if e.Time.IsZero() {
		return fmt.Errorf("repropack: event %d has no timestamp", e.Seq)
	}
	e.Time = e.Time.UTC()
	if err := w.enc.Encode(&e); err != nil { // Encode appends '\n'
		return err
	}
	w.lastSeq = e.Seq
	w.nEvents++
	t := e.Time
	if w.first == nil || t.Before(*w.first) {
		w.first = &t
	}
	if w.last == nil || t.After(*w.last) {
		tt := t
		w.last = &tt
	}
	return nil
}

func (w *Writer) finishEvents() error {
	if w.eventsDone {
		return nil
	}
	if err := w.openEvents(); err != nil {
		return err
	}
	if err := w.eventsBuf.Flush(); err != nil {
		return err
	}
	w.eventsDone = true
	return nil
}

// AddAttachment copies r into attachments/. a.ID, a.MediaType and a.Role are
// required; Path, SHA256 and Size are filled in. The stored file name is
// derived from the ID, never from user input, so it is always safe.
func (w *Writer) AddAttachment(a Attachment, r io.Reader) (Attachment, error) {
	if w.closed {
		return a, errors.New("repropack: writer closed")
	}
	if err := w.finishEvents(); err != nil {
		return a, err
	}
	if !validID(a.ID) {
		return a, fmt.Errorf("repropack: invalid attachment id %q", a.ID)
	}
	if w.ids[a.ID] {
		return a, fmt.Errorf("repropack: duplicate attachment id %q", a.ID)
	}
	if a.MediaType == "" {
		a.MediaType = "application/octet-stream"
	}
	if a.Role == "" {
		a.Role = RoleOther
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	a.Path = AttachmentsDir + a.ID + extensionFor(a.MediaType)
	method := zip.Deflate
	if isPrecompressed(a.MediaType) {
		method = zip.Store
	}
	f, err := w.zw.CreateHeader(&zip.FileHeader{Name: a.Path, Method: method, Modified: time.Unix(0, 0).UTC()})
	if err != nil {
		return a, err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), r)
	if err != nil {
		return a, err
	}
	a.Size = n
	a.SHA256 = hex.EncodeToString(h.Sum(nil))
	w.ids[a.ID] = true
	w.attachments = append(w.attachments, a)
	return a, nil
}

// Close fills integrity fields in m, writes manifest.json and finishes the
// archive. The returned manifest is exactly what was written.
func (w *Writer) Close(m Manifest) (Manifest, error) {
	if w.closed {
		return m, errors.New("repropack: writer closed")
	}
	if err := w.finishEvents(); err != nil {
		return m, err
	}
	w.closed = true
	m.Format = FormatName
	if m.SpecVersion == "" {
		m.SpecVersion = SpecVersion
	}
	if m.ReportID == "" {
		m.ReportID = NewID()
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	m.CreatedAt = m.CreatedAt.UTC()
	dropped := m.Events.Dropped
	m.Events = EventsInfo{
		Path:    EventsPath,
		SHA256:  hex.EncodeToString(w.eventsHash.Sum(nil)),
		Size:    w.eventsCnt.n,
		Count:   w.nEvents,
		First:   w.first,
		Last:    w.last,
		Dropped: dropped,
	}
	m.Attachments = append([]Attachment(nil), w.attachments...)
	m.ContentDigest = ComputeContentDigest(&m)
	if err := m.validateShape(); err != nil {
		return m, err
	}
	f, err := w.zw.CreateHeader(&zip.FileHeader{Name: ManifestPath, Method: zip.Deflate, Modified: time.Unix(0, 0).UTC()})
	if err != nil {
		return m, err
	}
	b, err := json.MarshalIndent(&m, "", "  ")
	if err != nil {
		return m, err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		return m, err
	}
	return m, w.zw.Close()
}

// ComputeContentDigest hashes the sorted integrity table: one line per file,
// "<path>\x00<sha256>\x00<size>\n". It identifies the report's content
// independently of ZIP layout and manifest formatting.
func ComputeContentDigest(m *Manifest) string {
	type row struct {
		p, h string
		n    int64
	}
	rows := []row{{m.Events.Path, m.Events.SHA256, m.Events.Size}}
	for _, a := range m.Attachments {
		rows = append(rows, row{a.Path, a.SHA256, a.Size})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].p < rows[j].p })
	h := sha256.New()
	for _, r := range rows {
		fmt.Fprintf(h, "%s\x00%s\x00%d\n", r.p, r.h, r.n)
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

func validID(id string) bool {
	if len(id) == 0 || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

var mediaExt = map[string]string{
	"image/png":            ".png",
	"image/jpeg":           ".jpg",
	"image/webp":           ".webp",
	"image/gif":            ".gif",
	"video/mp4":            ".mp4",
	"video/quicktime":      ".mov",
	"video/webm":           ".webm",
	"text/plain":           ".txt",
	"application/json":     ".json",
	"application/x-ndjson": ".jsonl",
}

func extensionFor(mt string) string {
	mt = strings.TrimSpace(strings.SplitN(mt, ";", 2)[0])
	if e, ok := mediaExt[mt]; ok {
		return e
	}
	return ".bin"
}

func isPrecompressed(mt string) bool {
	return strings.HasPrefix(mt, "image/") && mt != "image/svg+xml" || strings.HasPrefix(mt, "video/") || mt == "application/zip"
}
