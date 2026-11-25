package format

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

var (
	hexSHA256    = regexp.MustCompile(`^[0-9a-f]{64}$`)
	extensionKey = regexp.MustCompile(`^x-[a-z0-9-]+\.[a-z0-9_.-]+$`)
)

const (
	maxTitle      = 300
	maxText       = 64 << 10
	maxContextKV  = 256
	maxAnnotation = 1000
)

// validateShape checks the manifest's structure without touching content.
func (m *Manifest) validateShape() error {
	var p []Problem
	add := func(path, f string, a ...any) { p = append(p, Problem{path, fmt.Sprintf(f, a...)}) }
	if m.Format != FormatName {
		add("format", "must be %q", FormatName)
	}
	major, _, ok := parseVersion(m.SpecVersion)
	switch {
	case !ok:
		add("spec_version", "must look like MAJOR.MINOR")
	case major != SpecMajor:
		add("spec_version", "major version %d is not supported (this reader supports %d.x)", major, SpecMajor)
	}
	if m.ReportID == "" || len(m.ReportID) > 64 {
		add("report_id", "required, at most 64 characters")
	}
	if m.CreatedAt.IsZero() {
		add("created_at", "required")
	}
	if strings.TrimSpace(m.Title) == "" || len(m.Title) > maxTitle {
		add("title", "required, at most %d bytes", maxTitle)
	}
	if len(m.Description) > maxText {
		add("description", "longer than %d bytes", maxText)
	}
	if m.Producer.Name == "" {
		add("producer.name", "required")
	}
	if len(m.Context) > maxContextKV {
		add("context", "more than %d fields", maxContextKV)
	}
	for k, v := range m.Context {
		if !isScalar(v) {
			add("context."+k, "values must be JSON scalars")
		}
	}
	if len(m.Annotations) > maxAnnotation {
		add("annotations", "more than %d annotations", maxAnnotation)
	}
	for i, a := range m.Annotations {
		if a.ID == "" || len(a.Text) > maxText {
			add(fmt.Sprintf("annotations[%d]", i), "needs an id and at most %d bytes of text", maxText)
		}
	}
	if m.Events.Path != EventsPath {
		add("events.path", "must be %q", EventsPath)
	}
	if !hexSHA256.MatchString(m.Events.SHA256) {
		add("events.sha256", "must be 64 lowercase hex characters")
	}
	if m.Events.Size < 0 || m.Events.Count < 0 {
		add("events", "size and count must not be negative")
	}
	ids := map[string]bool{}
	paths := map[string]bool{}
	for i, a := range m.Attachments {
		at := fmt.Sprintf("attachments[%d]", i)
		if !validID(a.ID) {
			add(at+".id", "must be 1-64 characters of [A-Za-z0-9_-]")
		}
		if ids[a.ID] {
			add(at+".id", "duplicate")
		}
		ids[a.ID] = true
		if msg := checkEntryName(a.Path); msg != "" || !strings.HasPrefix(a.Path, AttachmentsDir) {
			add(at+".path", "must be attachments/<name>")
		}
		if paths[a.Path] {
			add(at+".path", "duplicate")
		}
		paths[a.Path] = true
		if !hexSHA256.MatchString(a.SHA256) {
			add(at+".sha256", "must be 64 lowercase hex characters")
		}
		if a.Size < 0 {
			add(at+".size", "must not be negative")
		}
		if a.MediaType == "" || len(a.MediaType) > 127 {
			add(at+".media_type", "required")
		}
		if a.Role == "" {
			add(at+".role", "required")
		}
	}
	for i, a := range m.Annotations {
		if a.AttachmentID != "" && !ids[a.AttachmentID] {
			add(fmt.Sprintf("annotations[%d].attachment_id", i), "refers to unknown attachment")
		}
	}
	if m.Redaction != nil {
		for _, id := range m.Redaction.ReviewedAttachments {
			if !ids[id] {
				add("redaction.reviewed_attachments", "refers to unknown attachment %q", id)
			}
		}
	}
	if !strings.HasPrefix(m.ContentDigest, "sha256:") || !hexSHA256.MatchString(strings.TrimPrefix(m.ContentDigest, "sha256:")) {
		add("content_digest", "must be sha256:<64 hex>")
	}
	for k := range m.Extensions {
		if !extensionKey.MatchString(k) {
			add("extensions."+k, "keys must look like x-<vendor>.<name>")
		}
	}
	if len(p) > 0 {
		return &ValidationError{p}
	}
	return nil
}

// check validates a single event; "" means valid.
func (e *Event) check() string {
	switch {
	case e.Seq == 0:
		return "seq must be >= 1"
	case e.Time.IsZero():
		return "ts is required"
	case e.Kind == "":
		return "kind is required"
	case len(e.Kind) > 64:
		return "kind too long"
	case len(e.Message) > maxText*4:
		return "msg too long"
	}
	switch e.Kind {
	case KindMetric:
		if e.Metric == nil || e.Metric.Name == "" {
			return "metric events need metric.name"
		}
	case KindSpan:
		if e.Span == nil || e.Span.Name == "" {
			return "span events need span.name"
		}
	case KindError, KindCrash:
		if e.Error == nil {
			return e.Kind + " events need an error object"
		}
	}
	if depth(e.Error) > 32 {
		return "error cause chain deeper than 32"
	}
	return ""
}

func depth(e *ErrorInfo) int {
	n := 0
	for ; e != nil; e = e.Cause {
		n++
	}
	return n
}

func parseVersion(v string) (major, minor int, ok bool) {
	a, b, found := strings.Cut(v, ".")
	if !found {
		return 0, 0, false
	}
	ma, err1 := strconv.Atoi(a)
	mi, err2 := strconv.Atoi(b)
	if err1 != nil || err2 != nil || ma < 0 || mi < 0 {
		return 0, 0, false
	}
	return ma, mi, true
}

func isScalar(v any) bool {
	if v == nil {
		return true
	}
	switch reflect.TypeOf(v).Kind() {
	case reflect.String, reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
		return true
	}
	return false
}
