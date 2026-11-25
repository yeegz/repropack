package viewer

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/yeegz/repropack/format"
)

//go:embed report.html.tmpl
var pageTmpl string

//go:embed report.css
var pageCSS string

//go:embed report.js
var pageJS string

// HTMLOptions bounds what is embedded in the page.
type HTMLOptions struct {
	MaxRows            int   // events rendered as rows (default 5000)
	MaxEmbedBytes      int64 // per attachment (default 12 MiB)
	MaxTotalEmbedBytes int64 // all attachments (default 48 MiB)
	Now                func() time.Time
}

// embeddable media types. SVG and HTML are deliberately excluded: they can
// carry script.
var embeddable = map[string]string{
	"image/png": "img", "image/jpeg": "img", "image/webp": "img", "image/gif": "img",
	"video/mp4": "video", "video/webm": "video", "video/quicktime": "video",
}

type attView struct {
	format.Attachment
	Kind    string // img, video, text, none
	DataURI template.URL
	Text    string
	Note    string
	Offset  string
	Pos     float64
	Notes   []format.Annotation
}

type rowView struct {
	format.Event
	Rel    string
	Clock  string
	Attrs  []kv
	Search string
	Detail bool
}

type kv struct{ K, V string }

type laneView struct {
	Lane
	Y    int
	Bars []bar
	Peak string
}

type bar struct {
	X, W, H, Y int
	Op         string
	Title      string
}

type metricView struct {
	MetricSeries
	Path  string
	Label string
	Y     int
}

type pageData struct {
	S         *Summary
	M         format.Manifest
	Rows      []rowView
	Hidden    int
	Atts      []attView
	Lanes     []laneView
	Metrics   []metricView
	Pins      []attView
	TLHeight  int
	TLDesc    string
	CSS       template.CSS
	JS        template.JS
	CSP       string
	Generated string
	Context   []kv
	Kinds     []string
	Levels    []string
	Duration  string
	Started   string
	OSLine    string
	AppLine   string
	ErrCount  int
	NotesFree []format.Annotation
}

// WriteHTML renders a self-contained HTML summary of r to w.
func WriteHTML(w io.Writer, r *format.Report, opt HTMLOptions) error {
	if opt.MaxRows <= 0 {
		opt.MaxRows = 5000
	}
	if opt.MaxEmbedBytes <= 0 {
		opt.MaxEmbedBytes = 12 << 20
	}
	if opt.MaxTotalEmbedBytes <= 0 {
		opt.MaxTotalEmbedBytes = 48 << 20
	}
	if opt.Now == nil {
		opt.Now = time.Now
	}
	s, err := Summarize(r, Options{MaxEvents: opt.MaxRows})
	if err != nil {
		return err
	}
	d := &pageData{S: s, M: s.Manifest, CSS: template.CSS(pageCSS), JS: template.JS(pageJS)}
	sum := sha256.Sum256([]byte(pageJS))
	d.CSP = "default-src 'none'; img-src data:; media-src data:; style-src 'unsafe-inline'; script-src 'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'; base-uri 'none'; form-action 'none'"
	d.Generated = opt.Now().UTC().Format("2 Jan 2006 15:04 MST")
	d.Duration = HumanDuration(s.Duration)
	d.Started = s.Start.UTC().Format("Mon 2 Jan 2006, 15:04:05 MST")
	if s.Manifest.OS != nil {
		d.OSLine = strings.TrimSpace(osName(s.Manifest.OS.Name) + " " + s.Manifest.OS.Version + " · " + s.Manifest.OS.Arch)
	}
	if a := s.Manifest.App; a != nil {
		d.AppLine = strings.TrimSpace(a.Name + " " + a.Version)
		if a.Build != "" {
			d.AppLine += " (" + a.Build + ")"
		}
	}
	d.ErrCount = s.Errors

	kinds, levels := map[string]bool{}, map[string]bool{}
	for _, e := range s.Events {
		rv := rowView{Event: e, Rel: Relative(s.Start, e.Time), Clock: e.Time.UTC().Format("15:04:05.000")}
		keys := make([]string, 0, len(e.Attrs))
		for k := range e.Attrs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			rv.Attrs = append(rv.Attrs, kv{k, valueText(e.Attrs[k])})
		}
		rv.Detail = e.Error != nil || len(rv.Attrs) > 3 || e.Span != nil
		msg := e.Message
		if msg == "" && e.Metric != nil {
			msg = fmt.Sprintf("%s = %g %s", e.Metric.Name, e.Metric.Value, e.Metric.Unit)
			rv.Message = msg
		}
		if msg == "" && e.Span != nil {
			rv.Message = fmt.Sprintf("%s (%s)", e.Span.Name, HumanDuration(e.Span.End.Sub(e.Span.Start)))
		}
		rv.Search = strings.ToLower(rv.Message + " " + e.Source)
		d.Rows = append(d.Rows, rv)
		kinds[e.Kind] = true
		if e.Level != "" {
			levels[e.Level] = true
		}
	}
	d.Hidden = s.TotalEvents - len(d.Rows)
	for k := range kinds {
		d.Kinds = append(d.Kinds, k)
	}
	sort.Strings(d.Kinds)
	for _, l := range []string{"trace", "debug", "info", "warn", "error", "fatal"} {
		if levels[l] {
			d.Levels = append(d.Levels, l)
		}
	}

	// Attachments: embed allowlisted media within budget.
	var budget = opt.MaxTotalEmbedBytes
	notesBy := map[string][]format.Annotation{}
	for _, n := range s.Manifest.Annotations {
		if n.AttachmentID != "" {
			notesBy[n.AttachmentID] = append(notesBy[n.AttachmentID], n)
		} else {
			d.NotesFree = append(d.NotesFree, n)
		}
	}
	for _, a := range s.Manifest.Attachments {
		av := attView{Attachment: a, Kind: "none", Notes: notesBy[a.ID], Offset: Relative(s.Start, a.CreatedAt), Pos: pos(s, a.CreatedAt)}
		mt := strings.TrimSpace(strings.SplitN(a.MediaType, ";", 2)[0])
		kind, ok := embeddable[mt]
		switch {
		case !s.Verified:
			av.Note = "Not shown: the report failed its integrity check."
		case ok && a.Size <= opt.MaxEmbedBytes && a.Size <= budget:
			b, err := r.ReadAttachment(a.ID, opt.MaxEmbedBytes)
			if err != nil {
				av.Note = "Could not read: " + err.Error()
				break
			}
			if !sniffOK(mt, b) {
				av.Note = "Not shown: content does not match its declared type."
				break
			}
			budget -= a.Size
			av.Kind = kind
			av.DataURI = template.URL("data:" + mt + ";base64," + base64.StdEncoding.EncodeToString(b))
		case ok:
			av.Note = fmt.Sprintf("Too large to embed (%s). Open the .repropack file to view it.", HumanBytes(a.Size))
		case isText(mt) && a.Size <= 256<<10:
			b, err := r.ReadAttachment(a.ID, 256<<10)
			if err == nil {
				av.Kind = "text"
				av.Text = strings.ToValidUTF8(string(b), "�")
			}
		default:
			av.Note = "This file type is not displayed in summaries."
		}
		d.Atts = append(d.Atts, av)
		if a.Role == format.RoleScreenshot || a.Role == format.RoleRecording {
			d.Pins = append(d.Pins, av)
		}
	}

	// Timeline geometry.
	const left, width, laneH = 120, 840, 26
	y := 34
	var desc []string
	for _, l := range s.Lanes {
		lv := laneView{Lane: l, Y: y}
		peakB := 0
		for i, c := range l.Buckets {
			if c > l.Buckets[peakB] {
				peakB = i
			}
			if c == 0 {
				continue
			}
			frac := float64(c) / float64(max(1, l.Max))
			h := 6 + int(frac*14)
			lv.Bars = append(lv.Bars, bar{
				X: left + i*width/Buckets, W: max(3, width/Buckets-2), H: h, Y: y + (laneH-h)/2,
				Op: fmt.Sprintf("%.2f", 0.35+0.65*frac), Title: fmt.Sprintf("%d %s near %s", c, strings.ToLower(l.Label), Relative(s.Start, s.Start.Add(time.Duration(float64(s.Duration)*float64(i)/float64(Buckets-1))))),
			})
		}
		lv.Peak = Relative(s.Start, s.Start.Add(time.Duration(float64(s.Duration)*float64(peakB)/float64(Buckets-1))))
		desc = append(desc, fmt.Sprintf("%s: %d, busiest near %s", l.Label, l.Total, lv.Peak))
		d.Lanes = append(d.Lanes, lv)
		y += laneH
	}
	for _, m := range s.Metrics {
		if len(d.Metrics) == 6 {
			break
		}
		label := fmt.Sprintf("%s (last %s, range %s–%s)", m.Name, num(m.Last, m.Unit), num(m.Min, m.Unit), num(m.Max, m.Unit))
		var p strings.Builder
		for i, pt := range m.Points {
			x := float64(left) + pos(s, pt.T)*float64(width)
			yy := float64(y+laneH-4) - 18*norm(pt.V, m.Min, m.Max)
			if i == 0 {
				fmt.Fprintf(&p, "M%.1f %.1f", x, yy)
			} else {
				fmt.Fprintf(&p, " L%.1f %.1f", x, yy)
			}
		}
		mv := metricView{MetricSeries: m, Path: p.String(), Label: label, Y: y}
		mv.Points = nil
		d.Metrics = append(d.Metrics, mv)
		desc = append(desc, label)
		y += laneH
	}
	d.TLHeight = y + 26
	d.TLDesc = fmt.Sprintf("Timeline of %s starting %s. %s.", d.Duration, d.Started, strings.Join(desc, "; "))
	if len(d.Pins) > 0 {
		d.TLDesc += fmt.Sprintf(" %d screenshots or recordings are marked on the top row.", len(d.Pins))
	}

	keys := make([]string, 0, len(s.Manifest.Context))
	for k := range s.Manifest.Context {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		d.Context = append(d.Context, kv{k, valueText(s.Manifest.Context[k])})
	}

	t, err := template.New("page").Funcs(template.FuncMap{
		"lower":  strings.ToLower,
		"bytes":  HumanBytes,
		"rel":    func(t time.Time) string { return Relative(s.Start, t) },
		"clock":  func(t time.Time) string { return t.UTC().Format("15:04:05.000") },
		"date":   func(t time.Time) string { return t.UTC().Format("2 Jan 2006, 15:04 MST") },
		"short":  func(h string) string { return h[:min(12, len(h))] },
		"x":      func(p float64) int { return left + int(p*float64(width)) },
		"pct":    func(p float64) string { return fmt.Sprintf("%.2f%%", p*100) },
		"levelc": levelClass,
		"add":    func(a, b int) int { return a + b },
		"dur":    func(a, b time.Time) string { return HumanDuration(b.Sub(a)) },
		"deref":  func(b *bool) bool { return b != nil && *b },
		"plural": func(n int, one, many string) string {
			if n == 1 {
				return one
			}
			return many
		},
		"kindLabel": kindLabel,
		"deref2":    func(t *time.Time) time.Time { return *t },
		"bytes64":   func(n uint64) string { return HumanBytes(int64(n)) },
	}).Parse(pageTmpl)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, d); err != nil {
		return err
	}
	_, err = w.Write(buf.Bytes())
	return err
}

func pos(s *Summary, t time.Time) float64 {
	if s.Duration <= 0 {
		return 0
	}
	return max(0, min(1, float64(t.Sub(s.Start))/float64(s.Duration)))
}

func norm(v, lo, hi float64) float64 {
	if hi <= lo {
		return 0.5
	}
	return (v - lo) / (hi - lo)
}

func num(v float64, unit string) string {
	switch unit {
	case "By":
		return HumanBytes(int64(v))
	case "%":
		return fmt.Sprintf("%.0f%%", v)
	case "ms":
		return fmt.Sprintf("%.0f ms", v)
	}
	if v == float64(int64(v)) {
		return fmt.Sprintf("%d%s", int64(v), unitSuffix(unit))
	}
	return fmt.Sprintf("%.2f%s", v, unitSuffix(unit))
}

func unitSuffix(u string) string {
	if u == "" || strings.HasPrefix(u, "{") {
		return ""
	}
	return " " + u
}

func valueText(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return "null"
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%g", t)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

func levelClass(l string) string {
	switch l {
	case "trace", "debug", "info", "warn", "error", "fatal":
		return l
	}
	return "info"
}

func kindLabel(k string) string {
	switch k {
	case format.KindLog:
		return "Log"
	case format.KindError:
		return "Error"
	case format.KindCrash:
		return "Crash"
	case format.KindMetric:
		return "Metric"
	case format.KindSpan:
		return "Span"
	case format.KindBreadcrumb:
		return "Action"
	case format.KindMarker:
		return "Marker"
	}
	return "Custom"
}

func osName(n string) string {
	switch n {
	case "darwin":
		return "macOS"
	case "windows":
		return "Windows"
	case "linux":
		return "Linux"
	}
	return n
}

func isText(mt string) bool {
	return strings.HasPrefix(mt, "text/") && mt != "text/html" || mt == "application/json" || mt == "application/x-ndjson"
}

// sniffOK checks magic bytes so a report cannot smuggle another format
// under an image or video type.
func sniffOK(mt string, b []byte) bool {
	has := func(p string) bool { return bytes.HasPrefix(b, []byte(p)) }
	switch mt {
	case "image/png":
		return has("\x89PNG\r\n\x1a\n")
	case "image/jpeg":
		return has("\xff\xd8\xff")
	case "image/gif":
		return has("GIF87a") || has("GIF89a")
	case "image/webp":
		return len(b) > 12 && has("RIFF") && string(b[8:12]) == "WEBP"
	case "video/mp4", "video/quicktime":
		return len(b) > 12 && (string(b[4:8]) == "ftyp" || string(b[4:8]) == "moov" || string(b[4:8]) == "wide" || string(b[4:8]) == "mdat")
	case "video/webm":
		return has("\x1a\x45\xdf\xa3")
	}
	return false
}
