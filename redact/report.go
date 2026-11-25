package redact

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/yeegz/repropack/format"
)

// MaxTextAttachment bounds the size of a text attachment that is scanned
// and rewritten in memory.
const MaxTextAttachment = 32 << 20

// IsText reports whether an attachment media type is scanned as text.
func IsText(mediaType string) bool {
	mt := strings.TrimSpace(strings.SplitN(mediaType, ";", 2)[0])
	return strings.HasPrefix(mt, "text/") || mt == "application/json" || mt == "application/x-ndjson"
}

// Event redacts one event in place and returns it.
func (p *Policy) Event(e format.Event, c Counts) format.Event {
	f := fmt.Sprintf("events[%d]", e.Seq)
	e.Message = p.String(f+".msg", e.Message, c, nil)
	e.Source = p.String(f+".source", e.Source, c, nil)
	if e.Attrs != nil {
		e.Attrs = p.Value(f+".attrs", e.Attrs, c).(map[string]any)
	}
	for ei := e.Error; ei != nil; ei = ei.Cause {
		if ei == e.Error {
			cp := *ei
			e.Error = &cp
			ei = e.Error
		}
		ei.Message = p.String(f+".error", ei.Message, c, nil)
		ei.Raw = p.String(f+".error.raw_stack", ei.Raw, c, nil)
		if len(ei.Stack) > 0 {
			st := make([]format.Frame, len(ei.Stack))
			for i, fr := range ei.Stack {
				fr.File = p.String(f+".error.stack", fr.File, c, nil)
				st[i] = fr
			}
			ei.Stack = st
		}
		if ei.Cause != nil {
			cp := *ei.Cause
			ei.Cause = &cp
		}
	}
	if e.Span != nil {
		s := *e.Span
		s.Name = p.String(f+".span.name", s.Name, c, nil)
		e.Span = &s
	}
	return e
}

// Manifest redacts the human-written and context fields of a manifest.
func (p *Policy) Manifest(m format.Manifest, c Counts) format.Manifest {
	m.Title = p.String("title", m.Title, c, nil)
	m.Description = p.String("description", m.Description, c, nil)
	if m.Reproduction != nil {
		r := *m.Reproduction
		r.Steps = append([]string(nil), r.Steps...)
		for i := range r.Steps {
			r.Steps[i] = p.String(fmt.Sprintf("reproduction.steps[%d]", i), r.Steps[i], c, nil)
		}
		r.Expected = p.String("reproduction.expected", r.Expected, c, nil)
		r.Actual = p.String("reproduction.actual", r.Actual, c, nil)
		m.Reproduction = &r
	}
	if m.Context != nil {
		m.Context = p.Value("context", m.Context, c).(map[string]any)
	}
	if len(m.Annotations) > 0 {
		an := append([]format.Annotation(nil), m.Annotations...)
		for i := range an {
			an[i].Text = p.String(fmt.Sprintf("annotations[%d]", i), an[i].Text, c, nil)
		}
		m.Annotations = an
	}
	if m.App != nil {
		a := *m.App
		a.Environment = p.String("app.environment", a.Environment, c, nil)
		m.App = &a
	}
	for i := range m.Attachments {
		m.Attachments[i].Title = p.String("attachments.title", m.Attachments[i].Title, c, nil)
	}
	return m
}

// Options controls RedactReport.
type Options struct {
	// ReviewedAttachments are image/video attachment IDs a person has
	// inspected. They are recorded in the manifest.
	ReviewedAttachments []string
	// Manual counts redactions a person made by hand (e.g. boxes drawn on
	// a screenshot) to be recorded alongside rule matches.
	Manual int
	// Producer, if set, replaces the manifest producer.
	Producer *format.Producer
}

// RedactReport writes a redacted copy of r to out and verifies the result by
// re-reading it and scanning again. It returns the written manifest.
func RedactReport(r *format.Report, out io.Writer, p *Policy, opt Options) (format.Manifest, error) {
	var buf bytes.Buffer
	w := format.NewWriter(&buf)
	c := Counts{}
	err := r.Events(func(e format.Event) error {
		return w.WriteEvent(p.Event(e, c))
	})
	if err != nil {
		return format.Manifest{}, err
	}
	for _, a := range r.Manifest.Attachments {
		if IsText(a.MediaType) {
			b, err := r.ReadAttachment(a.ID, MaxTextAttachment)
			if err != nil {
				return format.Manifest{}, err
			}
			if utf8.Valid(b) {
				b = []byte(p.String("attachments/"+a.ID, string(b), c, nil))
			}
			if _, err := w.AddAttachment(a, bytes.NewReader(b)); err != nil {
				return format.Manifest{}, err
			}
			continue
		}
		rc, err := r.OpenAttachment(a.ID)
		if err != nil {
			return format.Manifest{}, err
		}
		_, err = w.AddAttachment(a, rc)
		rc.Close()
		if err != nil {
			return format.Manifest{}, err
		}
	}
	m := p.Manifest(r.Manifest, c)
	if opt.Producer != nil {
		m.Producer = *opt.Producer
	}
	m.Redaction = mergeInfo(m.Redaction, p, c, opt)
	written, err := w.Close(m)
	if err != nil {
		return written, err
	}
	// Verify: re-open what we wrote and confirm nothing matches.
	b := buf.Bytes()
	rr, err := format.NewReader(bytes.NewReader(b), int64(len(b)), format.Limits{})
	if err != nil {
		return written, fmt.Errorf("redact: verification failed to reopen output: %w", err)
	}
	if err := rr.Verify(); err != nil {
		return written, fmt.Errorf("redact: verification failed: %w", err)
	}
	fs, err := ScanReport(rr, p)
	if err != nil {
		return written, err
	}
	if len(fs) > 0 {
		return written, fmt.Errorf("redact: verification found %d remaining matches (first: %s in %s)", len(fs), fs[0].RuleID, fs[0].Field)
	}
	_, err = out.Write(b)
	return written, err
}

func mergeInfo(prev *format.RedactionInfo, p *Policy, c Counts, opt Options) *format.RedactionInfo {
	now := time.Now().UTC()
	info := &format.RedactionInfo{Policy: p.Name, Verified: true, VerifiedAt: &now, Manual: opt.Manual}
	total := Counts{}
	if prev != nil {
		for _, r := range prev.Rules {
			total[r.ID] += r.Matches
		}
		info.Manual += prev.Manual
		info.ReviewedAttachments = append(info.ReviewedAttachments, prev.ReviewedAttachments...)
		if prev.Policy != "" && prev.Policy != p.Name {
			info.Policy = prev.Policy + "+" + p.Name
		}
	}
	total.Add(c)
	desc := map[string]string{}
	for _, r := range p.Rules {
		desc[r.ID] = r.Description
	}
	for id, n := range total {
		info.Rules = append(info.Rules, format.RedactionRule{ID: id, Description: desc[id], Matches: n})
	}
	sort.Slice(info.Rules, func(i, j int) bool { return info.Rules[i].ID < info.Rules[j].ID })
	seen := map[string]bool{}
	ids := info.ReviewedAttachments[:0]
	for _, id := range append(info.ReviewedAttachments, opt.ReviewedAttachments...) {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	info.ReviewedAttachments = ids
	return info
}

// ScanReport finds sensitive values anywhere in a report's text: manifest,
// events and text attachments. Key-based rules are reported as findings too.
func ScanReport(r *format.Report, p *Policy) ([]Finding, error) {
	var out []Finding
	scanVal := func(field string, v any) {
		walk(field, v, func(f string, key string, s string) {
			if key != "" {
				if rule := p.KeyRule(key); rule != nil && s != "" && !strings.HasPrefix(s, "[REDACTED:") {
					out = append(out, Finding{RuleID: rule.ID, Field: f, End: len(s), Value: s, Preview: mask(s)})
					return
				}
			}
			out = append(out, p.Scan(f, s)...)
		})
	}
	m := r.Manifest
	scanVal("title", m.Title)
	scanVal("description", m.Description)
	if m.Reproduction != nil {
		for i, s := range m.Reproduction.Steps {
			scanVal(fmt.Sprintf("reproduction.steps[%d]", i), s)
		}
		scanVal("reproduction.expected", m.Reproduction.Expected)
		scanVal("reproduction.actual", m.Reproduction.Actual)
	}
	scanVal("context", m.Context)
	for i, a := range m.Annotations {
		scanVal(fmt.Sprintf("annotations[%d]", i), a.Text)
	}
	err := r.Events(func(e format.Event) error {
		f := fmt.Sprintf("events[%d]", e.Seq)
		scanVal(f+".msg", e.Message)
		scanVal(f+".source", e.Source)
		scanVal(f+".attrs", map[string]any(e.Attrs))
		for ei := e.Error; ei != nil; ei = ei.Cause {
			scanVal(f+".error", ei.Message)
			scanVal(f+".error.raw_stack", ei.Raw)
			for _, fr := range ei.Stack {
				scanVal(f+".error.stack", fr.File)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, a := range m.Attachments {
		if !IsText(a.MediaType) || a.Size > MaxTextAttachment {
			continue
		}
		b, err := r.ReadAttachment(a.ID, MaxTextAttachment)
		if err != nil {
			return nil, err
		}
		if utf8.Valid(b) {
			out = append(out, p.Scan("attachments/"+a.ID, string(b))...)
		}
	}
	return out, nil
}

func walk(field string, v any, fn func(field, key, s string)) {
	switch t := v.(type) {
	case string:
		fn(field, "", t)
	case map[string]any:
		for k, vv := range t {
			if s, ok := vv.(string); ok {
				fn(field+"."+k, k, s)
				continue
			}
			walk(field+"."+k, vv, fn)
		}
	case []any:
		for i, vv := range t {
			walk(fmt.Sprintf("%s[%d]", field, i), vv, fn)
		}
	}
}

// UnreviewedMedia lists image and video attachments no person has marked
// as reviewed. Pattern redaction cannot see inside them.
func UnreviewedMedia(m *format.Manifest) []format.Attachment {
	reviewed := map[string]bool{}
	if m.Redaction != nil {
		for _, id := range m.Redaction.ReviewedAttachments {
			reviewed[id] = true
		}
	}
	var out []format.Attachment
	for _, a := range m.Attachments {
		if (strings.HasPrefix(a.MediaType, "image/") || strings.HasPrefix(a.MediaType, "video/")) && !reviewed[a.ID] {
			out = append(out, a)
		}
	}
	return out
}
