// Package sdk is the ReproPack Go SDK. A Recorder keeps a bounded,
// redacted buffer of recent diagnostic events alongside your existing
// logging, and turns it into a .repropack report on demand: when a user
// clicks "Report a problem", when a request fails, or when the process
// panics.
//
//	rec := sdk.New(sdk.Options{App: format.AppContext{Name: "billing", Version: "1.4.2"}})
//	logger := slog.New(rec.Handler(slog.NewJSONHandler(os.Stderr, nil)))
//	defer rec.CapturePanic("crash.repropack")
//	...
//	rec.WriteFile("report.repropack", sdk.ReportOptions{Title: "Invoice total wrong"})
//
// The SDK never uploads anything. It has no dependencies beyond the
// standard library and the format and redact packages.
package sdk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yeegz/repropack/format"
	"github.com/yeegz/repropack/redact"
)

// Version is the SDK version recorded in reports.
const Version = "0.1.0"

// Level aliases format levels for convenience.
type Level = string

// Filter decides whether an event is kept. It runs after redaction, so it
// never sees unredacted values. Returning false drops the event.
type Filter func(*format.Event) bool

// Options configures a Recorder. The zero value is usable.
type Options struct {
	App format.AppContext
	// Producer defaults to the app name and version.
	Producer *format.Producer
	// MaxEvents bounds the ring buffer by count (default 2000).
	MaxEvents int
	// MaxBytes bounds the approximate in-memory size of buffered events
	// (default 4 MiB). The oldest events are dropped first.
	MaxBytes int
	// MinLevel drops events below this level (default "debug").
	MinLevel Level
	// Filters run in order; any false drops the event.
	Filters []Filter
	// Policy redacts every event as it is recorded. Defaults to
	// redact.Default(). Use NoRedaction to disable explicitly.
	Policy *redact.Policy
	// Providers contribute context, events and attachments at report time.
	Providers []Provider
	// Now is a clock for tests.
	Now func() time.Time
}

// NoRedaction is a policy with no rules, for callers that redact elsewhere.
var NoRedaction = &redact.Policy{Name: "none"}

// Recorder buffers events. It is safe for concurrent use.
type Recorder struct {
	opt     Options
	policy  *redact.Policy
	minRank int

	mu      sync.Mutex
	ring    []entry
	head    int // index of oldest
	n       int
	bytes   int
	seq     uint64
	dropped uint64
	counts  redact.Counts
	ctx     map[string]any

	filtered atomic.Uint64
	started  time.Time
}

type entry struct {
	e    format.Event
	size int
}

// New creates a Recorder.
func New(opt Options) *Recorder {
	if opt.MaxEvents <= 0 {
		opt.MaxEvents = 2000
	}
	if opt.MaxBytes <= 0 {
		opt.MaxBytes = 4 << 20
	}
	if opt.MinLevel == "" {
		opt.MinLevel = format.LevelDebug
	}
	if opt.Now == nil {
		opt.Now = time.Now
	}
	p := opt.Policy
	if p == nil {
		p = redact.Default()
	}
	if opt.App.Runtime == "" {
		opt.App.Runtime = runtime.Version()
	}
	if opt.App.PID == 0 {
		opt.App.PID = os.Getpid()
	}
	r := &Recorder{
		opt:     opt,
		policy:  p,
		minRank: format.LevelRank(opt.MinLevel),
		ring:    make([]entry, opt.MaxEvents),
		counts:  redact.Counts{},
		ctx:     map[string]any{},
		started: opt.Now(),
	}
	st := r.started.UTC()
	r.opt.App.StartedAt = &st
	return r
}

// SetContext sets a custom context field included in every report. Values
// must be JSON scalars; other types are formatted with %v. Values are
// redacted when the report is written.
func (r *Recorder) SetContext(key string, value any) {
	switch value.(type) {
	case nil, string, bool, int, int64, uint64, float64, float32:
	default:
		value = fmt.Sprint(value)
	}
	r.mu.Lock()
	r.ctx[key] = value
	r.mu.Unlock()
}

// Record adds an event. Seq and Time are assigned if unset. The event is
// redacted and filtered before it is stored.
func (r *Recorder) Record(e format.Event) {
	if e.Kind == "" {
		e.Kind = format.KindLog
	}
	if e.Level != "" && format.LevelRank(e.Level) < r.minRank {
		r.filtered.Add(1)
		return
	}
	if e.Time.IsZero() {
		e.Time = r.opt.Now()
	}
	local := redact.Counts{}
	if len(r.policy.Rules) > 0 {
		e = r.policy.Event(e, local)
	}
	for _, f := range r.opt.Filters {
		if !f(&e) {
			r.filtered.Add(1)
			return
		}
	}
	size := approxSize(&e)
	r.mu.Lock()
	r.counts.Add(local)
	r.seq++
	e.Seq = r.seq
	if size > r.opt.MaxBytes {
		r.dropped++
		r.mu.Unlock()
		return
	}
	for r.n > 0 && (r.n == len(r.ring) || r.bytes+size > r.opt.MaxBytes) {
		r.bytes -= r.ring[r.head].size
		r.ring[r.head] = entry{}
		r.head = (r.head + 1) % len(r.ring)
		r.n--
		r.dropped++
	}
	r.ring[(r.head+r.n)%len(r.ring)] = entry{e, size}
	r.n++
	r.bytes += size
	r.mu.Unlock()
}

func approxSize(e *format.Event) int {
	n := 96 + len(e.Message) + len(e.Source) + len(e.Kind)
	for k, v := range e.Attrs {
		n += len(k) + 16
		if s, ok := v.(string); ok {
			n += len(s)
		}
	}
	for ei := e.Error; ei != nil; ei = ei.Cause {
		n += len(ei.Message) + len(ei.Raw) + len(ei.Type) + 64*len(ei.Stack)
	}
	return n
}

// Enabled reports whether events at level would be kept.
func (r *Recorder) Enabled(level Level) bool { return format.LevelRank(level) >= r.minRank }

// Log records a log event.
func (r *Recorder) Log(level Level, msg string, attrs map[string]any) {
	if !r.Enabled(level) {
		r.filtered.Add(1)
		return
	}
	r.Record(format.Event{Kind: format.KindLog, Level: level, Message: msg, Attrs: attrs})
}

// Breadcrumb records a user or system action that led up to a problem.
func (r *Recorder) Breadcrumb(msg string, attrs map[string]any) {
	r.Record(format.Event{Kind: format.KindBreadcrumb, Level: format.LevelInfo, Message: msg, Attrs: attrs})
}

// Metric records a measurement.
func (r *Recorder) Metric(name string, value float64, unit string) {
	r.Record(format.Event{Kind: format.KindMetric, Metric: &format.Metric{Name: name, Value: value, Unit: unit}})
}

// Error records an error with its wrap chain and the caller's stack.
func (r *Recorder) Error(err error, msg string, attrs map[string]any) {
	if err == nil {
		return
	}
	if msg == "" {
		msg = err.Error()
	}
	handled := true
	ei := ErrorInfo(err, 1)
	ei.Handled = &handled
	r.Record(format.Event{Kind: format.KindError, Level: format.LevelError, Message: msg, Attrs: attrs, Error: ei})
}

// Span records a completed operation.
func (r *Recorder) Span(name string, start, end time.Time, status string) {
	r.Record(format.Event{Kind: format.KindSpan, Time: end, Span: &format.Span{Name: name, Start: start.UTC(), End: end.UTC(), Status: status}})
}

// Stats reports buffer state.
type Stats struct {
	Buffered int
	Bytes    int
	Dropped  uint64
	Filtered uint64
	Recorded uint64
}

// Stats returns current buffer statistics.
func (r *Recorder) Stats() Stats {
	r.mu.Lock()
	defer r.mu.Unlock()
	return Stats{Buffered: r.n, Bytes: r.bytes, Dropped: r.dropped, Filtered: r.filtered.Load(), Recorded: r.seq}
}

// Snapshot copies the buffered events, oldest first.
func (r *Recorder) Snapshot() []format.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]format.Event, r.n)
	for i := 0; i < r.n; i++ {
		out[i] = r.ring[(r.head+i)%len(r.ring)].e
	}
	return out
}

// Reset clears the buffer (context fields are kept).
func (r *Recorder) Reset() {
	r.mu.Lock()
	for i := range r.ring {
		r.ring[i] = entry{}
	}
	r.head, r.n, r.bytes, r.dropped = 0, 0, 0, 0
	r.counts = redact.Counts{}
	r.mu.Unlock()
}

// Attachment is a file to include in a report.
type Attachment struct {
	Meta format.Attachment
	// Open returns the content. It is called once, while writing.
	Open func() (io.ReadCloser, error)
}

// FileAttachment attaches a file from disk.
func FileAttachment(path, mediaType, role string) Attachment {
	return Attachment{
		Meta: format.Attachment{ID: format.ShortID(), MediaType: mediaType, Role: role, Title: filepath.Base(path)},
		Open: func() (io.ReadCloser, error) { return os.Open(path) },
	}
}

// BytesAttachment attaches in-memory content.
func BytesAttachment(title, mediaType, role string, b []byte) Attachment {
	return Attachment{
		Meta: format.Attachment{ID: format.ShortID(), MediaType: mediaType, Role: role, Title: title},
		Open: func() (io.ReadCloser, error) { return io.NopCloser(bytesReader(b)), nil },
	}
}

// ReportOptions describes one report.
type ReportOptions struct {
	Title        string
	Description  string
	Reproduction *format.Reproduction
	Annotations  []format.Annotation
	Attachments  []Attachment
	// Extra events appended after the buffered ones (e.g. a crash event).
	Extra []format.Event
	// Timeout bounds provider collection (default 5s).
	Timeout time.Duration
}

// WriteReport writes a .repropack report of the current buffer to w.
// Provider failures are recorded as events rather than failing the report.
func (r *Recorder) WriteReport(ctx context.Context, w io.Writer, ro ReportOptions) (format.Manifest, error) {
	if ro.Title == "" {
		ro.Title = "Report from " + r.opt.App.Name
	}
	if ro.Timeout <= 0 {
		ro.Timeout = 5 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, ro.Timeout)
	defer cancel()

	pd := r.collect(cctx)
	for _, e := range pd.Events {
		r.Record(e)
	}
	for _, e := range ro.Extra {
		r.Record(e)
	}

	events := r.Snapshot()
	r.mu.Lock()
	dropped := r.dropped
	counts := redact.Counts{}
	counts.Add(r.counts)
	ctxFields := make(map[string]any, len(r.ctx)+len(pd.Context))
	for k, v := range r.ctx {
		ctxFields[k] = v
	}
	r.mu.Unlock()
	for k, v := range pd.Context {
		ctxFields[k] = v
	}

	fw := format.NewWriter(w)
	for _, e := range events {
		if err := fw.WriteEvent(e); err != nil {
			return format.Manifest{}, err
		}
	}
	atts := append(append([]Attachment(nil), pd.Attachments...), ro.Attachments...)
	for _, a := range atts {
		rc, err := a.Open()
		if err != nil {
			return format.Manifest{}, fmt.Errorf("attachment %q: %w", a.Meta.Title, err)
		}
		meta := a.Meta
		var src io.Reader = rc
		if redact.IsText(meta.MediaType) && len(r.policy.Rules) > 0 {
			b, err := io.ReadAll(io.LimitReader(rc, redact.MaxTextAttachment))
			if err != nil {
				rc.Close()
				return format.Manifest{}, err
			}
			src = bytesReader([]byte(r.policy.String("attachments/"+meta.ID, string(b), counts, nil)))
		}
		_, err = fw.AddAttachment(meta, src)
		rc.Close()
		if err != nil {
			return format.Manifest{}, err
		}
	}

	prod := format.Producer{Name: r.opt.App.Name, Version: r.opt.App.Version}
	if r.opt.Producer != nil {
		prod = *r.opt.Producer
	}
	if prod.Name == "" {
		prod.Name = "repropack-go"
	}
	prod.SDK = "repropack-go/" + Version
	app := r.opt.App
	m := format.Manifest{
		Title:        ro.Title,
		Description:  ro.Description,
		Producer:     prod,
		App:          &app,
		OS:           osContext(),
		Device:       deviceInfo(),
		Reproduction: ro.Reproduction,
		Context:      ctxFields,
		Annotations:  ro.Annotations,
		Events:       format.EventsInfo{Dropped: dropped},
	}
	if len(r.policy.Rules) > 0 {
		m = r.policy.Manifest(m, counts)
		// Redaction ran at record time. Verified stays false until a
		// verifier (repropack redact, the desktop app) re-scans the file.
		info := &format.RedactionInfo{Policy: r.policy.Name}
		for id, n := range counts {
			info.Rules = append(info.Rules, format.RedactionRule{ID: id, Matches: n})
		}
		sortRules(info.Rules)
		m.Redaction = info
	}
	return fw.Close(m)
}

// WriteFile writes a report atomically to path (via a temp file + rename).
func (r *Recorder) WriteFile(path string, ro ReportOptions) (format.Manifest, error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".repropack-*")
	if err != nil {
		return format.Manifest{}, err
	}
	defer os.Remove(tmp.Name())
	m, err := r.WriteReport(context.Background(), tmp, ro)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return m, err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return m, err
	}
	return m, os.Rename(tmp.Name(), path)
}

// ErrPanicked is returned in crash reports' error chain for non-error panics.
var ErrPanicked = errors.New("panic")
