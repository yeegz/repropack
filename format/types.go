// Package format defines the ReproPack portable bug report format (.repropack)
// and provides a streaming writer, a hardened reader and a validator.
//
// A .repropack file is a ZIP archive containing:
//
//	manifest.json        report metadata, context, integrity table (required)
//	events.jsonl         newline-delimited JSON events, ordered by seq (required, may be empty)
//	attachments/<name>   screenshots, recordings, log files and other evidence (optional)
//
// The package depends only on the Go standard library so that it can be
// vendored or reimplemented in any language. See spec/SPEC.md.
package format

import (
	"encoding/json"
	"time"
)

const (
	// FormatName is the value of Manifest.Format.
	FormatName = "repropack"
	// SpecVersion is the version of the specification this package writes.
	SpecVersion = "1.0"
	// SpecMajor is the major version this package can read.
	SpecMajor = 1

	// FileExtension is the conventional file extension.
	FileExtension = ".repropack"
	// MediaType is the IANA-style media type used for .repropack files.
	MediaType = "application/vnd.repropack+zip"

	ManifestPath   = "manifest.json"
	EventsPath     = "events.jsonl"
	AttachmentsDir = "attachments/"
)

// Manifest is the root metadata document stored at manifest.json.
type Manifest struct {
	Format      string    `json:"format"`
	SpecVersion string    `json:"spec_version"`
	ReportID    string    `json:"report_id"`
	CreatedAt   time.Time `json:"created_at"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`

	Producer     Producer      `json:"producer"`
	App          *AppContext   `json:"app,omitempty"`
	OS           *OSContext    `json:"os,omitempty"`
	Device       *DeviceInfo   `json:"device,omitempty"`
	Reproduction *Reproduction `json:"reproduction,omitempty"`

	// Context holds custom, flat key/value fields supplied by the producer.
	// Values must be JSON scalars (string, number, bool, null).
	Context map[string]any `json:"context,omitempty"`

	Annotations []Annotation   `json:"annotations,omitempty"`
	Events      EventsInfo     `json:"events"`
	Attachments []Attachment   `json:"attachments,omitempty"`
	Redaction   *RedactionInfo `json:"redaction,omitempty"`

	// ContentDigest is "sha256:<hex>" over the sorted integrity table.
	// See ComputeContentDigest.
	ContentDigest string `json:"content_digest"`

	// Extensions carries namespaced producer-specific data. Keys must look
	// like "x-<vendor>.<name>". Readers must ignore extensions they do not
	// understand.
	Extensions map[string]json.RawMessage `json:"extensions,omitempty"`
}

// Producer identifies the software that created the report.
type Producer struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	// SDK names the library used, e.g. "repropack-go/0.1.0".
	SDK string `json:"sdk,omitempty"`
}

// AppContext describes the application the report is about.
type AppContext struct {
	Name        string `json:"name"`
	Version     string `json:"version,omitempty"`
	Build       string `json:"build,omitempty"`
	Environment string `json:"environment,omitempty"`
	Runtime     string `json:"runtime,omitempty"`
	PID         int    `json:"pid,omitempty"`
	// StartedAt is when the observed process started, if known.
	StartedAt *time.Time `json:"started_at,omitempty"`
}

// OSContext describes the operating system.
type OSContext struct {
	Name     string `json:"name"`
	Version  string `json:"version,omitempty"`
	Build    string `json:"build,omitempty"`
	Arch     string `json:"arch,omitempty"`
	Kernel   string `json:"kernel,omitempty"`
	Locale   string `json:"locale,omitempty"`
	Timezone string `json:"timezone,omitempty"`
}

// DeviceInfo describes the hardware.
type DeviceInfo struct {
	Model       string `json:"model,omitempty"`
	CPU         string `json:"cpu,omitempty"`
	CPUCores    int    `json:"cpu_cores,omitempty"`
	MemoryBytes uint64 `json:"memory_bytes,omitempty"`
	Displays    int    `json:"displays,omitempty"`
}

// Reproduction is the human description of the problem.
type Reproduction struct {
	Steps    []string `json:"steps,omitempty"`
	Expected string   `json:"expected,omitempty"`
	Actual   string   `json:"actual,omitempty"`
	// Frequency is free text such as "always", "sometimes", "once".
	Frequency string `json:"frequency,omitempty"`
}

// Annotation is a user note, optionally pinned to a time or attachment.
type Annotation struct {
	ID           string     `json:"id"`
	Text         string     `json:"text"`
	At           *time.Time `json:"at,omitempty"`
	AttachmentID string     `json:"attachment_id,omitempty"`
	Author       string     `json:"author,omitempty"`
}

// EventsInfo is the integrity entry for events.jsonl.
type EventsInfo struct {
	Path   string     `json:"path"`
	SHA256 string     `json:"sha256"`
	Size   int64      `json:"size"`
	Count  int        `json:"count"`
	First  *time.Time `json:"first,omitempty"`
	Last   *time.Time `json:"last,omitempty"`
	// Dropped counts events discarded by a bounded buffer before export.
	Dropped uint64 `json:"dropped,omitempty"`
}

// Attachment roles.
const (
	RoleScreenshot = "screenshot"
	RoleRecording  = "recording"
	RoleLog        = "log"
	RoleDump       = "dump"
	RoleOther      = "other"
)

// Attachment is an evidence file stored under attachments/.
type Attachment struct {
	ID        string    `json:"id"`
	Path      string    `json:"path"`
	MediaType string    `json:"media_type"`
	SHA256    string    `json:"sha256"`
	Size      int64     `json:"size"`
	Role      string    `json:"role"`
	Title     string    `json:"title,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Width     int       `json:"width,omitempty"`
	Height    int       `json:"height,omitempty"`
	// DurationMS is set for recordings.
	DurationMS int64 `json:"duration_ms,omitempty"`
	// Annotated is true when the image has user markup (including
	// redaction boxes) burned into its pixels.
	Annotated bool `json:"annotated,omitempty"`
}

// RedactionInfo records what was removed, never the removed values.
type RedactionInfo struct {
	Policy     string          `json:"policy"`
	Rules      []RedactionRule `json:"rules,omitempty"`
	Manual     int             `json:"manual,omitempty"`
	Verified   bool            `json:"verified"`
	VerifiedAt *time.Time      `json:"verified_at,omitempty"`
	// ReviewedAttachments lists attachment IDs a person reviewed visually.
	ReviewedAttachments []string `json:"reviewed_attachments,omitempty"`
}

// RedactionRule summarises one rule's effect.
type RedactionRule struct {
	ID          string `json:"id"`
	Description string `json:"description,omitempty"`
	Matches     int    `json:"matches"`
}

// Event kinds.
const (
	KindLog        = "log"
	KindError      = "error"
	KindCrash      = "crash"
	KindMetric     = "metric"
	KindSpan       = "span"
	KindBreadcrumb = "breadcrumb"
	KindMarker     = "marker"
	KindCustom     = "custom"
)

// KnownKinds lists kinds defined by spec 1.0. Unknown kinds are valid and
// must be displayed as custom.
var KnownKinds = []string{KindLog, KindError, KindCrash, KindMetric, KindSpan, KindBreadcrumb, KindMarker, KindCustom}

// Levels, ordered by severity.
const (
	LevelTrace = "trace"
	LevelDebug = "debug"
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"
	LevelFatal = "fatal"
)

// LevelRank orders levels; unknown levels rank as info.
func LevelRank(l string) int {
	switch l {
	case LevelTrace:
		return 0
	case LevelDebug:
		return 1
	case LevelInfo, "":
		return 2
	case LevelWarn:
		return 3
	case LevelError:
		return 4
	case LevelFatal:
		return 5
	}
	return 2
}

// Event is one line of events.jsonl.
type Event struct {
	Seq     uint64         `json:"seq"`
	Time    time.Time      `json:"ts"`
	Kind    string         `json:"kind"`
	Level   string         `json:"level,omitempty"`
	Source  string         `json:"source,omitempty"`
	Message string         `json:"msg,omitempty"`
	Attrs   map[string]any `json:"attrs,omitempty"`
	TraceID string         `json:"trace_id,omitempty"`
	SpanID  string         `json:"span_id,omitempty"`
	Error   *ErrorInfo     `json:"error,omitempty"`
	Metric  *Metric        `json:"metric,omitempty"`
	Span    *Span          `json:"span,omitempty"`
}

// ErrorInfo describes an error or crash.
type ErrorInfo struct {
	Type    string     `json:"type,omitempty"`
	Message string     `json:"message"`
	Stack   []Frame    `json:"stack,omitempty"`
	Cause   *ErrorInfo `json:"cause,omitempty"`
	Raw     string     `json:"raw_stack,omitempty"`
	Handled *bool      `json:"handled,omitempty"`
}

// Frame is a stack frame.
type Frame struct {
	Function string `json:"function,omitempty"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Module   string `json:"module,omitempty"`
}

// Metric is a single measurement.
type Metric struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit,omitempty"`
}

// Span is a completed unit of work, compatible with OpenTelemetry spans.
type Span struct {
	Name         string    `json:"name"`
	TraceID      string    `json:"trace_id,omitempty"`
	SpanID       string    `json:"span_id,omitempty"`
	ParentSpanID string    `json:"parent_span_id,omitempty"`
	Start        time.Time `json:"start"`
	End          time.Time `json:"end"`
	Status       string    `json:"status,omitempty"`
}
