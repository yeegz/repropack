// Package viewer turns an untrusted .repropack report into a display model
// and a self-contained, accessible HTML summary. Nothing from a report is
// executed: text is escaped by html/template, only allowlisted raster image
// and video types are embedded, and the page ships a strict CSP.
package viewer

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/yeegz/repropack/format"
	"github.com/yeegz/repropack/redact"
)

// Summary is a display-ready digest of a report.
type Summary struct {
	Manifest    format.Manifest `json:"manifest"`
	Integrity   string          `json:"integrity"` // "verified" or the failure
	Verified    bool            `json:"verified"`
	Start       time.Time       `json:"start"`
	End         time.Time       `json:"end"`
	Duration    time.Duration   `json:"duration"`
	ByKind      map[string]int  `json:"by_kind"`
	ByLevel     map[string]int  `json:"by_level"`
	Headline    string          `json:"headline"`
	FirstError  *format.Event   `json:"first_error,omitempty"`
	Errors      int             `json:"errors"`
	Lanes       []Lane          `json:"lanes"`
	Metrics     []MetricSeries  `json:"metrics"`
	Unreviewed  []string        `json:"unreviewed_media"`
	Events      []format.Event  `json:"events,omitempty"`
	Truncated   bool            `json:"truncated"`
	TotalEvents int             `json:"total_events"`
}

// Lane is one row of the timeline: event density per bucket.
type Lane struct {
	Name    string `json:"name"`
	Label   string `json:"label"`
	Buckets []int  `json:"buckets"`
	Max     int    `json:"max"`
	Total   int    `json:"total"`
}

// MetricSeries is a sampled numeric series.
type MetricSeries struct {
	Name   string  `json:"name"`
	Unit   string  `json:"unit"`
	Points []Point `json:"points"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	Last   float64 `json:"last"`
}

// Point is one sample.
type Point struct {
	T time.Time `json:"t"`
	V float64   `json:"v"`
}

// Buckets is the timeline resolution.
const Buckets = 96

// Options bounds the summary.
type Options struct {
	// MaxEvents kept in Summary.Events (default 20000). Counts and lanes
	// always cover every event.
	MaxEvents int
}

// Summarize verifies the report and builds a Summary. Integrity failures
// are reported in the summary rather than returned, so a damaged report can
// still be inspected — the UI must show the warning prominently.
func Summarize(r *format.Report, opt Options) (*Summary, error) {
	if opt.MaxEvents <= 0 {
		opt.MaxEvents = 20000
	}
	s := &Summary{Manifest: r.Manifest, ByKind: map[string]int{}, ByLevel: map[string]int{}}
	if err := r.Verify(); err != nil {
		s.Integrity = err.Error()
	} else {
		s.Integrity = "verified"
		s.Verified = true
	}
	var events []format.Event
	metrics := map[string]*MetricSeries{}
	err := r.Events(func(e format.Event) error {
		s.TotalEvents++
		s.ByKind[e.Kind]++
		if e.Level != "" {
			s.ByLevel[e.Level]++
		}
		if e.Kind == format.KindError || e.Kind == format.KindCrash || format.LevelRank(e.Level) >= format.LevelRank(format.LevelError) {
			s.Errors++
			if s.FirstError == nil || e.Kind == format.KindCrash && s.FirstError.Kind != format.KindCrash {
				ec := e
				s.FirstError = &ec
			}
		}
		if e.Metric != nil {
			ms := metrics[e.Metric.Name]
			if ms == nil {
				ms = &MetricSeries{Name: e.Metric.Name, Unit: e.Metric.Unit, Min: e.Metric.Value, Max: e.Metric.Value}
				metrics[e.Metric.Name] = ms
			}
			if len(ms.Points) < 2000 {
				ms.Points = append(ms.Points, Point{e.Time, e.Metric.Value})
			}
			ms.Min = min(ms.Min, e.Metric.Value)
			ms.Max = max(ms.Max, e.Metric.Value)
			ms.Last = e.Metric.Value
		}
		if len(events) < opt.MaxEvents {
			events = append(events, e)
		} else {
			s.Truncated = true
		}
		return nil
	})
	if err != nil {
		// Event stream unreadable: keep what we have, flag integrity.
		s.Verified = false
		s.Integrity = err.Error()
	}
	s.Events = events
	if m := r.Manifest.Events; m.First != nil && m.Last != nil {
		s.Start, s.End = *m.First, *m.Last
	} else if len(events) > 0 {
		s.Start, s.End = events[0].Time, events[len(events)-1].Time
	} else {
		s.Start, s.End = r.Manifest.CreatedAt, r.Manifest.CreatedAt
	}
	for _, a := range r.Manifest.Attachments {
		if a.CreatedAt.Before(s.Start) && !a.CreatedAt.IsZero() && s.Start.Sub(a.CreatedAt) < 24*time.Hour {
			s.Start = a.CreatedAt
		}
		if a.CreatedAt.After(s.End) && a.CreatedAt.Sub(s.End) < 24*time.Hour {
			s.End = a.CreatedAt
		}
	}
	s.Duration = s.End.Sub(s.Start)
	s.Lanes = lanes(events, s.Start, s.End)
	for _, ms := range metrics {
		s.Metrics = append(s.Metrics, *ms)
	}
	sort.Slice(s.Metrics, func(i, j int) bool { return s.Metrics[i].Name < s.Metrics[j].Name })
	for _, a := range redact.UnreviewedMedia(&r.Manifest) {
		s.Unreviewed = append(s.Unreviewed, a.ID)
	}
	s.Headline = headline(s)
	return s, nil
}

func lanes(events []format.Event, start, end time.Time) []Lane {
	defs := []struct{ name, label string }{
		{"crash", "Crashes"}, {"error", "Errors"}, {"warn", "Warnings"}, {"log", "Logs"}, {"breadcrumb", "Actions"}, {"span", "Spans"}, {"other", "Other"},
	}
	idx := map[string]int{}
	out := make([]Lane, len(defs))
	for i, d := range defs {
		out[i] = Lane{Name: d.name, Label: d.label, Buckets: make([]int, Buckets)}
		idx[d.name] = i
	}
	span := end.Sub(start)
	for _, e := range events {
		lane := "other"
		switch {
		case e.Kind == format.KindCrash:
			lane = "crash"
		case e.Kind == format.KindError || format.LevelRank(e.Level) >= 4:
			lane = "error"
		case e.Level == format.LevelWarn:
			lane = "warn"
		case e.Kind == format.KindLog:
			lane = "log"
		case e.Kind == format.KindBreadcrumb || e.Kind == format.KindMarker:
			lane = "breadcrumb"
		case e.Kind == format.KindSpan:
			lane = "span"
		case e.Kind == format.KindMetric:
			continue
		}
		b := 0
		if span > 0 {
			b = int(float64(e.Time.Sub(start)) / float64(span) * float64(Buckets-1))
			b = max(0, min(Buckets-1, b))
		}
		l := &out[idx[lane]]
		l.Buckets[b]++
		l.Total++
		l.Max = max(l.Max, l.Buckets[b])
	}
	var keep []Lane
	for _, l := range out {
		if l.Total > 0 {
			keep = append(keep, l)
		}
	}
	return keep
}

func headline(s *Summary) string {
	switch {
	case s.FirstError != nil && s.FirstError.Kind == format.KindCrash:
		return "The app crashed: " + errText(s.FirstError)
	case s.FirstError != nil:
		n := ""
		if s.Errors > 1 {
			n = fmt.Sprintf(" (and %d more)", s.Errors-1)
		}
		return "An error was recorded: " + errText(s.FirstError) + n
	case s.TotalEvents == 0 && len(s.Manifest.Attachments) > 0:
		return "Visual evidence only — no diagnostic events were recorded."
	case s.TotalEvents == 0:
		return "No diagnostic events were recorded."
	}
	return fmt.Sprintf("No errors in %d recorded events.", s.TotalEvents)
}

func errText(e *format.Event) string {
	t := e.Message
	if e.Error != nil && e.Error.Message != "" {
		t = e.Error.Message
	}
	t = strings.TrimSpace(strings.SplitN(t, "\n", 2)[0])
	if len(t) > 160 {
		t = t[:160] + "…"
	}
	return t
}

// Relative formats an offset from the report start as m:ss.mmm.
func Relative(start, t time.Time) string {
	d := t.Sub(start)
	sign := ""
	if d < 0 {
		sign, d = "−", -d
	}
	m := int(d / time.Minute)
	sec := float64(d%time.Minute) / float64(time.Second)
	if m >= 60 {
		return fmt.Sprintf("%s%d:%02d:%06.3f", sign, m/60, m%60, sec)
	}
	return fmt.Sprintf("%s%d:%06.3f", sign, m, sec)
}

// HumanDuration renders durations for people.
func HumanDuration(d time.Duration) string {
	switch {
	case d < time.Second:
		return fmt.Sprintf("%d ms", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf("%.1f s", d.Seconds())
	case d < time.Hour:
		return fmt.Sprintf("%d min %d s", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%d h %d min", int(d.Hours()), int(d.Minutes())%60)
}

// HumanBytes renders sizes for people.
func HumanBytes(n int64) string {
	const k = 1024
	switch {
	case n < k:
		return fmt.Sprintf("%d B", n)
	case n < k*k:
		return fmt.Sprintf("%.1f KB", float64(n)/k)
	case n < k*k*k:
		return fmt.Sprintf("%.1f MB", float64(n)/(k*k))
	}
	return fmt.Sprintf("%.2f GB", float64(n)/(k*k*k))
}
