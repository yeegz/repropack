package format

import (
	"archive/zip"
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

// Limits bound the resources a Reader will spend on an untrusted archive.
// Zero fields take the DefaultLimits value.
type Limits struct {
	MaxArchiveBytes    int64 // compressed size of the whole file
	MaxEntries         int
	MaxManifestBytes   int64
	MaxEventsBytes     int64 // uncompressed events.jsonl
	MaxEventLineBytes  int
	MaxEvents          int
	MaxAttachmentBytes int64 // uncompressed, per attachment
	MaxTotalBytes      int64 // uncompressed, all entries
	MaxRatio           int64 // uncompressed/compressed per entry
}

// DefaultLimits are generous for real reports and small enough to stop
// decompression bombs on a laptop.
var DefaultLimits = Limits{
	MaxArchiveBytes:    1 << 30,
	MaxEntries:         4096,
	MaxManifestBytes:   4 << 20,
	MaxEventsBytes:     256 << 20,
	MaxEventLineBytes:  1 << 20,
	MaxEvents:          2_000_000,
	MaxAttachmentBytes: 512 << 20,
	MaxTotalBytes:      2 << 30,
	MaxRatio:           200,
}

func (l Limits) withDefaults() Limits {
	d := DefaultLimits
	if l.MaxArchiveBytes > 0 {
		d.MaxArchiveBytes = l.MaxArchiveBytes
	}
	if l.MaxEntries > 0 {
		d.MaxEntries = l.MaxEntries
	}
	if l.MaxManifestBytes > 0 {
		d.MaxManifestBytes = l.MaxManifestBytes
	}
	if l.MaxEventsBytes > 0 {
		d.MaxEventsBytes = l.MaxEventsBytes
	}
	if l.MaxEventLineBytes > 0 {
		d.MaxEventLineBytes = l.MaxEventLineBytes
	}
	if l.MaxEvents > 0 {
		d.MaxEvents = l.MaxEvents
	}
	if l.MaxAttachmentBytes > 0 {
		d.MaxAttachmentBytes = l.MaxAttachmentBytes
	}
	if l.MaxTotalBytes > 0 {
		d.MaxTotalBytes = l.MaxTotalBytes
	}
	if l.MaxRatio > 0 {
		d.MaxRatio = l.MaxRatio
	}
	return d
}

// ErrInvalid wraps every validation failure.
var ErrInvalid = errors.New("invalid repropack")

// Problem is one validation finding.
type Problem struct {
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

// ValidationError lists every problem found.
type ValidationError struct{ Problems []Problem }

func (e *ValidationError) Error() string {
	var b strings.Builder
	b.WriteString("invalid repropack: ")
	for i, p := range e.Problems {
		if i > 0 {
			b.WriteString("; ")
		}
		if p.Path != "" {
			b.WriteString(p.Path + ": ")
		}
		b.WriteString(p.Message)
		if i == 9 && len(e.Problems) > 10 {
			fmt.Fprintf(&b, "; and %d more", len(e.Problems)-10)
			break
		}
	}
	return b.String()
}

func (e *ValidationError) Unwrap() error { return ErrInvalid }

// Report is an opened, structurally validated archive. Event and attachment
// content is verified lazily (Events, OpenAttachment) or fully (Verify).
// All content is untrusted data: never execute or render it as markup.
type Report struct {
	Manifest Manifest
	zr       *zip.Reader
	files    map[string]*zip.File
	limits   Limits
	closer   io.Closer
	attByID  map[string]*Attachment
}

// Open opens a .repropack file from disk.
func Open(path string, lim Limits) (*Report, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if !st.Mode().IsRegular() {
		f.Close()
		return nil, &ValidationError{[]Problem{{Message: "not a regular file"}}}
	}
	r, err := NewReader(f, st.Size(), lim)
	if err != nil {
		f.Close()
		return nil, err
	}
	r.closer = f
	return r, nil
}

// Close releases the underlying file, if any.
func (r *Report) Close() error {
	if r.closer != nil {
		return r.closer.Close()
	}
	return nil
}

// NewReader parses the archive structure and manifest. It rejects unsafe
// names, duplicate entries, unknown entries, oversize or suspiciously
// compressed entries and malformed manifests before reading any content.
func NewReader(ra io.ReaderAt, size int64, lim Limits) (*Report, error) {
	lim = lim.withDefaults()
	if size > lim.MaxArchiveBytes {
		return nil, &ValidationError{[]Problem{{Message: fmt.Sprintf("archive is %d bytes; limit is %d", size, lim.MaxArchiveBytes)}}}
	}
	zr, err := zip.NewReader(ra, size)
	if err != nil {
		return nil, &ValidationError{[]Problem{{Message: "not a ZIP archive: " + err.Error()}}}
	}
	var probs []Problem
	add := func(p, f string, a ...any) { probs = append(probs, Problem{p, fmt.Sprintf(f, a...)}) }
	if len(zr.File) > lim.MaxEntries {
		return nil, &ValidationError{[]Problem{{Message: fmt.Sprintf("%d entries; limit is %d", len(zr.File), lim.MaxEntries)}}}
	}
	files := make(map[string]*zip.File, len(zr.File))
	var total uint64
	for _, f := range zr.File {
		name := f.Name
		if msg := checkEntryName(name); msg != "" {
			add(safeName(name), "%s", msg)
			continue
		}
		if _, dup := files[name]; dup {
			add(name, "duplicate entry")
			continue
		}
		if !f.Mode().IsRegular() {
			add(name, "not a regular file (mode %v)", f.Mode())
			continue
		}
		if f.Method != zip.Store && f.Method != zip.Deflate {
			add(name, "unsupported compression method %d", f.Method)
			continue
		}
		if f.Flags&0x1 != 0 {
			add(name, "encrypted entries are not supported")
			continue
		}
		total += f.UncompressedSize64
		if f.CompressedSize64 > 0 && f.UncompressedSize64 > 1<<20 && f.UncompressedSize64/f.CompressedSize64 > uint64(lim.MaxRatio) {
			add(name, "compression ratio %d:1 exceeds limit", f.UncompressedSize64/f.CompressedSize64)
			continue
		}
		files[name] = f
	}
	if total > uint64(lim.MaxTotalBytes) {
		add("", "declared uncompressed size %d exceeds limit %d", total, lim.MaxTotalBytes)
	}
	mf, ok := files[ManifestPath]
	if !ok {
		add(ManifestPath, "missing")
	}
	if len(probs) > 0 {
		return nil, &ValidationError{probs}
	}
	raw, err := readAllLimited(mf, lim.MaxManifestBytes)
	if err != nil {
		return nil, &ValidationError{[]Problem{{ManifestPath, err.Error()}}}
	}
	if !utf8.Valid(raw) {
		return nil, &ValidationError{[]Problem{{ManifestPath, "not valid UTF-8"}}}
	}
	var m Manifest
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(&m); err != nil {
		return nil, &ValidationError{[]Problem{{ManifestPath, "malformed JSON: " + err.Error()}}}
	}
	if err := m.validateShape(); err != nil {
		return nil, err
	}
	r := &Report{Manifest: m, zr: zr, files: files, limits: lim, attByID: map[string]*Attachment{}}
	// Cross-check the integrity table against the archive's entries.
	listed := map[string]bool{ManifestPath: true, m.Events.Path: true}
	if f, ok := files[m.Events.Path]; !ok {
		add(m.Events.Path, "listed in manifest but missing from archive")
	} else if int64(f.UncompressedSize64) != m.Events.Size {
		add(m.Events.Path, "size %d does not match manifest %d", f.UncompressedSize64, m.Events.Size)
	} else if m.Events.Size > lim.MaxEventsBytes {
		add(m.Events.Path, "%d bytes exceeds limit %d", m.Events.Size, lim.MaxEventsBytes)
	}
	for i := range r.Manifest.Attachments {
		a := &r.Manifest.Attachments[i]
		listed[a.Path] = true
		r.attByID[a.ID] = a
		f, ok := files[a.Path]
		switch {
		case !ok:
			add(a.Path, "listed in manifest but missing from archive")
		case int64(f.UncompressedSize64) != a.Size:
			add(a.Path, "size %d does not match manifest %d", f.UncompressedSize64, a.Size)
		case a.Size > lim.MaxAttachmentBytes:
			add(a.Path, "%d bytes exceeds limit %d", a.Size, lim.MaxAttachmentBytes)
		}
	}
	for name := range files {
		if !listed[name] {
			add(name, "entry is not listed in the manifest")
		}
	}
	if len(probs) > 0 {
		return nil, &ValidationError{probs}
	}
	return r, nil
}

// checkEntryName enforces the spec's path rules. Returning "" means safe.
func checkEntryName(name string) string {
	switch {
	case name == "":
		return "empty entry name"
	case len(name) > 255:
		return "entry name too long"
	case !utf8.ValidString(name):
		return "entry name is not UTF-8"
	case strings.ContainsAny(name, "\\:\x00"):
		return "entry name contains a forbidden character"
	case strings.HasPrefix(name, "/"):
		return "absolute entry name"
	case strings.HasSuffix(name, "/"):
		return "directory entries are not allowed"
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return "entry name contains a control character"
		}
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return "entry name has an empty, '.' or '..' segment"
		}
	}
	if name == ManifestPath || name == EventsPath {
		return ""
	}
	if strings.HasPrefix(name, AttachmentsDir) && strings.Count(name, "/") == 1 {
		return ""
	}
	return "entry is outside the allowed layout"
}

func safeName(s string) string {
	s = strings.ToValidUTF8(s, "?")
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return '?'
		}
		return r
	}, s)
	if len(s) > 80 {
		s = s[:80] + "…"
	}
	return s
}

func readAllLimited(f *zip.File, max int64) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	b, err := io.ReadAll(io.LimitReader(rc, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, fmt.Errorf("exceeds %d byte limit", max)
	}
	return b, nil
}

// hashingReader enforces an exact size and checks the SHA-256 at EOF.
type hashingReader struct {
	r      io.Reader
	h      hashWriter
	want   string
	size   int64
	n      int64
	path   string
	closer io.Closer
}

type hashWriter interface {
	io.Writer
	Sum([]byte) []byte
}

func (h *hashingReader) Read(p []byte) (int, error) {
	n, err := h.r.Read(p)
	h.n += int64(n)
	h.h.Write(p[:n])
	if h.n > h.size {
		return n, &ValidationError{[]Problem{{h.path, "content longer than declared size"}}}
	}
	if err == io.EOF {
		if h.n != h.size {
			return n, &ValidationError{[]Problem{{h.path, "content shorter than declared size"}}}
		}
		if got := hex.EncodeToString(h.h.Sum(nil)); got != strings.ToLower(h.want) {
			return n, &ValidationError{[]Problem{{h.path, "SHA-256 mismatch"}}}
		}
	}
	return n, err
}

func (h *hashingReader) Close() error { return h.closer.Close() }

func (r *Report) openVerified(path, sum string, size int64) (io.ReadCloser, error) {
	f, ok := r.files[path]
	if !ok {
		return nil, fmt.Errorf("%s: not in archive", path)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	return &hashingReader{r: io.LimitReader(rc, size+1), h: sha256.New(), want: sum, size: size, path: path, closer: rc}, nil
}

// Attachment returns the attachment metadata for id.
func (r *Report) Attachment(id string) (*Attachment, bool) {
	a, ok := r.attByID[id]
	return a, ok
}

// OpenAttachment streams an attachment's bytes. The returned reader fails
// with a ValidationError at EOF if the size or hash does not match, so
// callers must read to EOF and check the error before trusting the content.
func (r *Report) OpenAttachment(id string) (io.ReadCloser, error) {
	a, ok := r.attByID[id]
	if !ok {
		return nil, fmt.Errorf("no attachment %q", id)
	}
	return r.openVerified(a.Path, a.SHA256, a.Size)
}

// ReadAttachment reads and verifies a whole attachment into memory, up to max bytes.
func (r *Report) ReadAttachment(id string, max int64) ([]byte, error) {
	a, ok := r.attByID[id]
	if !ok {
		return nil, fmt.Errorf("no attachment %q", id)
	}
	if a.Size > max {
		return nil, fmt.Errorf("attachment %s is %d bytes; limit %d", id, a.Size, max)
	}
	rc, err := r.OpenAttachment(id)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// Events calls fn for every event in order. Each line is size-limited,
// strictly ordered by seq and schema-checked. The stream hash is verified
// at the end; if it fails, an error is returned after all callbacks ran,
// so callers that need trusted data must discard results on error.
func (r *Report) Events(fn func(Event) error) error {
	rc, err := r.openVerified(r.Manifest.Events.Path, r.Manifest.Events.SHA256, r.Manifest.Events.Size)
	if err != nil {
		return err
	}
	defer rc.Close()
	sc := bufio.NewScanner(rc)
	sc.Buffer(make([]byte, 64<<10), r.limits.MaxEventLineBytes)
	var prev uint64
	n := 0
	line := 0
	for sc.Scan() {
		line++
		b := sc.Bytes()
		if len(bytes.TrimSpace(b)) == 0 {
			return &ValidationError{[]Problem{{fmt.Sprintf("%s:%d", EventsPath, line), "blank line"}}}
		}
		var e Event
		if err := json.Unmarshal(b, &e); err != nil {
			return &ValidationError{[]Problem{{fmt.Sprintf("%s:%d", EventsPath, line), "malformed JSON: " + err.Error()}}}
		}
		if msg := e.check(); msg != "" {
			return &ValidationError{[]Problem{{fmt.Sprintf("%s:%d", EventsPath, line), msg}}}
		}
		if e.Seq <= prev {
			return &ValidationError{[]Problem{{fmt.Sprintf("%s:%d", EventsPath, line), fmt.Sprintf("seq %d not greater than %d", e.Seq, prev)}}}
		}
		prev = e.Seq
		n++
		if n > r.limits.MaxEvents {
			return &ValidationError{[]Problem{{EventsPath, fmt.Sprintf("more than %d events", r.limits.MaxEvents)}}}
		}
		if err := fn(e); err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return &ValidationError{[]Problem{{fmt.Sprintf("%s:%d", EventsPath, line+1), "line exceeds limit"}}}
		}
		return err
	}
	if n != r.Manifest.Events.Count {
		return &ValidationError{[]Problem{{EventsPath, fmt.Sprintf("%d events; manifest says %d", n, r.Manifest.Events.Count)}}}
	}
	return nil
}

// ReadEvents loads all events into memory (bounded by Limits).
func (r *Report) ReadEvents() ([]Event, error) {
	out := make([]Event, 0, min(r.Manifest.Events.Count, 100_000))
	err := r.Events(func(e Event) error { out = append(out, e); return nil })
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Verify reads every entry and checks sizes, hashes, event ordering and the
// content digest. A nil result means the report is intact and well-formed.
func (r *Report) Verify() error {
	var probs []Problem
	if err := r.Events(func(Event) error { return nil }); err != nil {
		probs = append(probs, asProblems(err)...)
	}
	for _, a := range r.Manifest.Attachments {
		rc, err := r.OpenAttachment(a.ID)
		if err != nil {
			probs = append(probs, Problem{a.Path, err.Error()})
			continue
		}
		_, err = io.Copy(io.Discard, rc)
		rc.Close()
		if err != nil {
			probs = append(probs, asProblems(err)...)
		}
	}
	if d := ComputeContentDigest(&r.Manifest); d != r.Manifest.ContentDigest {
		probs = append(probs, Problem{ManifestPath, "content_digest does not match the integrity table"})
	}
	if len(probs) > 0 {
		return &ValidationError{probs}
	}
	return nil
}

func asProblems(err error) []Problem {
	var ve *ValidationError
	if errors.As(err, &ve) {
		return ve.Problems
	}
	return []Problem{{Message: err.Error()}}
}
