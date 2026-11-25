package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/yeegz/repropack/format"
	"github.com/yeegz/repropack/sdk"
)

const maxIngestLine = 1 << 20

var (
	tsPrefix = regexp.MustCompile(`^\[?(\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:[.,]\d{1,9})?(?:Z|[+-]\d{2}:?\d{2})?)\]?\s*`)
	lvlWord  = regexp.MustCompile(`(?i)^\[?(trace|debug|info|information|notice|warn|warning|error|err|fatal|critical|panic)\]?[:\s]\s*`)
	logfmtKV = regexp.MustCompile(`(?:^|\s)(time|ts|level|lvl|msg)=("(?:[^"\\]|\\.)*"|\S+)`)
)

// ingestLog parses a plain-text log: JSON lines (slog/zap/zerolog style),
// logfmt, or "<timestamp> <LEVEL> message" lines. Unparseable lines become
// continuation text of the previous event (stack traces) or plain events.
func ingestLog(rec *sdk.Recorder, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	st, _ := f.Stat()
	fallback := st.ModTime()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), maxIngestLine)
	var pending *format.Event
	flush := func() {
		if pending != nil {
			rec.Record(*pending)
			pending = nil
		}
	}
	source := pathBase(path)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if e, ok := parseJSONLog(line); ok {
			flush()
			e.Source = source
			pending = &e
			continue
		}
		if e, ok := parseLogfmt(line); ok {
			flush()
			e.Source = source
			pending = &e
			continue
		}
		if m := tsPrefix.FindStringSubmatch(line); m != nil {
			flush()
			e := format.Event{Kind: format.KindLog, Source: source, Time: parseTime(m[1], fallback)}
			rest := line[len(m[0]):]
			if lm := lvlWord.FindStringSubmatch(rest); lm != nil {
				e.Level = normLevel(lm[1])
				rest = rest[len(lm[0]):]
			}
			e.Message = rest
			pending = &e
			continue
		}
		if pending != nil && (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "goroutine ") || strings.HasPrefix(line, "Caused by")) {
			if len(pending.Message) < 64<<10 {
				pending.Message += "\n" + line
			}
			continue
		}
		flush()
		pending = &format.Event{Kind: format.KindLog, Source: source, Time: fallback, Message: line}
	}
	flush()
	if err := sc.Err(); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func pathBase(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}

func parseJSONLog(line string) (format.Event, bool) {
	if !strings.HasPrefix(line, "{") {
		return format.Event{}, false
	}
	var m map[string]any
	if json.Unmarshal([]byte(line), &m) != nil {
		return format.Event{}, false
	}
	e := format.Event{Kind: format.KindLog, Attrs: map[string]any{}}
	for k, v := range m {
		s, _ := v.(string)
		switch strings.ToLower(k) {
		case "time", "ts", "timestamp", "@timestamp":
			e.Time = parseTime(s, time.Time{})
			if f, ok := v.(float64); ok {
				e.Time = time.Unix(0, int64(f*1e9))
			}
		case "level", "lvl", "severity":
			e.Level = normLevel(s)
		case "msg", "message":
			e.Message = s
		case "error", "err":
			e.Attrs[k] = v
			if s != "" {
				e.Error = &format.ErrorInfo{Message: s}
			}
		case "trace_id", "traceid":
			e.TraceID = s
		case "span_id", "spanid":
			e.SpanID = s
		default:
			e.Attrs[k] = v
		}
	}
	if len(e.Attrs) == 0 {
		e.Attrs = nil
	}
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	return e, true
}

func parseLogfmt(line string) (format.Event, bool) {
	ms := logfmtKV.FindAllStringSubmatch(line, -1)
	if len(ms) < 2 {
		return format.Event{}, false
	}
	e := format.Event{Kind: format.KindLog}
	for _, m := range ms {
		v := m[2]
		if strings.HasPrefix(v, `"`) {
			var s string
			if json.Unmarshal([]byte(v), &s) == nil {
				v = s
			}
		}
		switch m[1] {
		case "time", "ts":
			e.Time = parseTime(v, time.Time{})
		case "level", "lvl":
			e.Level = normLevel(v)
		case "msg":
			e.Message = v
		}
	}
	if e.Time.IsZero() || e.Message == "" {
		return format.Event{}, false
	}
	return e, true
}

func parseTime(s string, fallback time.Time) time.Time {
	s = strings.Replace(s, ",", ".", 1)
	for _, l := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999Z0700", "2006-01-02 15:04:05.999999999Z07:00", "2006-01-02 15:04:05.999999999", "2006-01-02T15:04:05.999999999"} {
		if t, err := time.ParseInLocation(l, s, time.Local); err == nil {
			return t
		}
	}
	return fallback
}

func normLevel(s string) string {
	switch strings.ToLower(s) {
	case "trace":
		return format.LevelTrace
	case "debug":
		return format.LevelDebug
	case "info", "information", "notice":
		return format.LevelInfo
	case "warn", "warning":
		return format.LevelWarn
	case "error", "err":
		return format.LevelError
	case "fatal", "critical", "panic", "dpanic":
		return format.LevelFatal
	}
	return ""
}

// ingestJSONL reads ReproPack-shaped events (as produced by 'inspect
// --events --json'), re-sequencing them.
func ingestJSONL(rec *sdk.Recorder, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), maxIngestLine)
	n := 0
	for sc.Scan() {
		n++
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		var e format.Event
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			return fmt.Errorf("%s:%d: %w", path, n, err)
		}
		e.Seq = 0
		if e.Time.IsZero() {
			return fmt.Errorf("%s:%d: event has no ts", path, n)
		}
		rec.Record(e)
	}
	return sc.Err()
}
