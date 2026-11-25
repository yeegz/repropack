// Package otlp converts between ReproPack events and OpenTelemetry OTLP
// JSON log records (opentelemetry.io/docs/specs/otlp, JSON encoding).
//
// Only the subset needed for logs is implemented, with no dependency on the
// OpenTelemetry SDK: hex trace/span IDs, integer severity numbers, int64 as
// decimal strings and lowerCamelCase keys, as the OTLP JSON rules require.
package otlp

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"time"

	"github.com/yeegz/repropack/format"
)

// LogsData is the OTLP JSON top-level logs document.
type LogsData struct {
	ResourceLogs []ResourceLogs `json:"resourceLogs"`
}

type ResourceLogs struct {
	Resource  Resource    `json:"resource"`
	ScopeLogs []ScopeLogs `json:"scopeLogs"`
}

type Resource struct {
	Attributes []KeyValue `json:"attributes,omitempty"`
}

type ScopeLogs struct {
	Scope      Scope       `json:"scope"`
	LogRecords []LogRecord `json:"logRecords"`
}

type Scope struct {
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
}

type LogRecord struct {
	TimeUnixNano         string     `json:"timeUnixNano,omitempty"`
	ObservedTimeUnixNano string     `json:"observedTimeUnixNano,omitempty"`
	SeverityNumber       int        `json:"severityNumber,omitempty"`
	SeverityText         string     `json:"severityText,omitempty"`
	Body                 *AnyValue  `json:"body,omitempty"`
	Attributes           []KeyValue `json:"attributes,omitempty"`
	TraceID              string     `json:"traceId,omitempty"`
	SpanID               string     `json:"spanId,omitempty"`
	EventName            string     `json:"eventName,omitempty"`
}

type KeyValue struct {
	Key   string   `json:"key"`
	Value AnyValue `json:"value"`
}

type AnyValue struct {
	StringValue *string       `json:"stringValue,omitempty"`
	BoolValue   *bool         `json:"boolValue,omitempty"`
	IntValue    *string       `json:"intValue,omitempty"`
	DoubleValue *float64      `json:"doubleValue,omitempty"`
	ArrayValue  *ArrayValue   `json:"arrayValue,omitempty"`
	KvlistValue *KeyValueList `json:"kvlistValue,omitempty"`
}

type ArrayValue struct {
	Values []AnyValue `json:"values"`
}

type KeyValueList struct {
	Values []KeyValue `json:"values"`
}

var sevNum = map[string]int{format.LevelTrace: 1, format.LevelDebug: 5, format.LevelInfo: 9, format.LevelWarn: 13, format.LevelError: 17, format.LevelFatal: 21}

func severityLevel(n int) string {
	switch {
	case n <= 0:
		return ""
	case n < 5:
		return format.LevelTrace
	case n < 9:
		return format.LevelDebug
	case n < 13:
		return format.LevelInfo
	case n < 17:
		return format.LevelWarn
	case n < 21:
		return format.LevelError
	}
	return format.LevelFatal
}

func str(s string) AnyValue { return AnyValue{StringValue: &s} }

func toAny(v any, depth int) AnyValue {
	if depth > 8 {
		return str(fmt.Sprint(v))
	}
	switch t := v.(type) {
	case string:
		return str(t)
	case bool:
		return AnyValue{BoolValue: &t}
	case float64:
		if t == float64(int64(t)) && t < 1e15 && t > -1e15 {
			s := strconv.FormatInt(int64(t), 10)
			return AnyValue{IntValue: &s}
		}
		return AnyValue{DoubleValue: &t}
	case int:
		s := strconv.Itoa(t)
		return AnyValue{IntValue: &s}
	case int64:
		s := strconv.FormatInt(t, 10)
		return AnyValue{IntValue: &s}
	case []any:
		a := &ArrayValue{}
		for _, x := range t {
			a.Values = append(a.Values, toAny(x, depth+1))
		}
		return AnyValue{ArrayValue: a}
	case map[string]any:
		return AnyValue{KvlistValue: &KeyValueList{Values: kvs(t, depth+1)}}
	case nil:
		return str("")
	}
	return str(fmt.Sprint(v))
}

func kvs(m map[string]any, depth int) []KeyValue {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]KeyValue, 0, len(keys))
	for _, k := range keys {
		out = append(out, KeyValue{k, toAny(m[k], depth)})
	}
	return out
}

func fromAny(v AnyValue, depth int) any {
	switch {
	case depth > 8:
		return nil
	case v.StringValue != nil:
		return *v.StringValue
	case v.BoolValue != nil:
		return *v.BoolValue
	case v.IntValue != nil:
		if n, err := strconv.ParseInt(*v.IntValue, 10, 64); err == nil {
			return float64(n)
		}
		return *v.IntValue
	case v.DoubleValue != nil:
		return *v.DoubleValue
	case v.ArrayValue != nil:
		out := make([]any, 0, len(v.ArrayValue.Values))
		for _, x := range v.ArrayValue.Values {
			out = append(out, fromAny(x, depth+1))
		}
		return out
	case v.KvlistValue != nil:
		m := map[string]any{}
		for _, kv := range v.KvlistValue.Values {
			m[kv.Key] = fromAny(kv.Value, depth+1)
		}
		return m
	}
	return nil
}

// Export writes the report's log-like events (log, error, crash, breadcrumb,
// marker, custom) as a single OTLP JSON LogsData document.
func Export(r *format.Report, w io.Writer) (int, error) {
	m := r.Manifest
	res := Resource{}
	if m.App != nil {
		res.Attributes = append(res.Attributes, KeyValue{"service.name", str(m.App.Name)})
		if m.App.Version != "" {
			res.Attributes = append(res.Attributes, KeyValue{"service.version", str(m.App.Version)})
		}
	}
	if m.OS != nil {
		res.Attributes = append(res.Attributes, KeyValue{"os.type", str(m.OS.Name)})
		if m.OS.Version != "" {
			res.Attributes = append(res.Attributes, KeyValue{"os.version", str(m.OS.Version)})
		}
	}
	res.Attributes = append(res.Attributes, KeyValue{"repropack.report_id", str(m.ReportID)})
	sl := ScopeLogs{Scope: Scope{Name: "repropack", Version: format.SpecVersion}}
	err := r.Events(func(e format.Event) error {
		if e.Kind == format.KindMetric || e.Kind == format.KindSpan {
			return nil
		}
		ts := strconv.FormatInt(e.Time.UnixNano(), 10)
		lr := LogRecord{TimeUnixNano: ts, ObservedTimeUnixNano: ts, SeverityNumber: sevNum[e.Level], SeverityText: e.Level, TraceID: e.TraceID, SpanID: e.SpanID}
		if e.Message != "" {
			b := str(e.Message)
			lr.Body = &b
		}
		lr.Attributes = kvs(e.Attrs, 0)
		lr.Attributes = append(lr.Attributes, KeyValue{"repropack.kind", str(e.Kind)}, KeyValue{"repropack.seq", toAny(int64(e.Seq), 0)})
		if e.Source != "" {
			lr.Attributes = append(lr.Attributes, KeyValue{"repropack.source", str(e.Source)})
		}
		if e.Error != nil {
			lr.Attributes = append(lr.Attributes, KeyValue{"exception.message", str(e.Error.Message)})
			if e.Error.Type != "" {
				lr.Attributes = append(lr.Attributes, KeyValue{"exception.type", str(e.Error.Type)})
			}
			if st := stackText(e.Error); st != "" {
				lr.Attributes = append(lr.Attributes, KeyValue{"exception.stacktrace", str(st)})
			}
		}
		sl.LogRecords = append(sl.LogRecords, lr)
		return nil
	})
	if err != nil {
		return 0, err
	}
	doc := LogsData{ResourceLogs: []ResourceLogs{{Resource: res, ScopeLogs: []ScopeLogs{sl}}}}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return len(sl.LogRecords), enc.Encode(doc)
}

func stackText(e *format.ErrorInfo) string {
	if e.Raw != "" {
		return e.Raw
	}
	s := ""
	for _, f := range e.Stack {
		s += fmt.Sprintf("%s.%s\n\t%s:%d\n", f.Module, f.Function, f.File, f.Line)
	}
	return s
}

// MaxImportBytes bounds the OTLP document size accepted by Import.
const MaxImportBytes = 128 << 20

// Import reads OTLP JSON logs (one LogsData document, or JSON Lines of them
// as written by the OpenTelemetry file exporter) and returns events sorted
// by time, plus resource attributes flattened into context.
func Import(r io.Reader) ([]format.Event, map[string]any, error) {
	dec := json.NewDecoder(io.LimitReader(r, MaxImportBytes))
	ctx := map[string]any{}
	var events []format.Event
	docs := 0
	for {
		var d LogsData
		err := dec.Decode(&d)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("otlp: %w", err)
		}
		docs++
		for _, rl := range d.ResourceLogs {
			for _, kv := range rl.Resource.Attributes {
				ctx["resource."+kv.Key] = scalar(fromAny(kv.Value, 0))
			}
			for _, sl := range rl.ScopeLogs {
				for _, lr := range sl.LogRecords {
					e := format.Event{Kind: format.KindLog, Level: lr.SeverityText, TraceID: lr.TraceID, SpanID: lr.SpanID, Source: sl.Scope.Name}
					if lv := severityLevel(lr.SeverityNumber); lv != "" {
						e.Level = lv
					}
					if format.LevelRank(e.Level) == 2 && e.Level != format.LevelInfo {
						e.Level = format.LevelInfo
					}
					ns, _ := strconv.ParseInt(firstNonEmpty(lr.TimeUnixNano, lr.ObservedTimeUnixNano), 10, 64)
					e.Time = time.Unix(0, ns).UTC()
					if ns == 0 {
						e.Time = time.Unix(0, 0).UTC()
					}
					if lr.Body != nil {
						if s, ok := fromAny(*lr.Body, 0).(string); ok {
							e.Message = s
						} else {
							b, _ := json.Marshal(fromAny(*lr.Body, 0))
							e.Message = string(b)
						}
					}
					attrs := map[string]any{}
					var exMsg, exType, exStack string
					for _, kv := range lr.Attributes {
						v := fromAny(kv.Value, 0)
						switch kv.Key {
						case "exception.message":
							exMsg, _ = v.(string)
						case "exception.type":
							exType, _ = v.(string)
						case "exception.stacktrace":
							exStack, _ = v.(string)
						case "repropack.kind":
							if s, _ := v.(string); s != "" {
								e.Kind = s
							}
						case "repropack.source":
							e.Source, _ = v.(string)
						case "repropack.seq":
						default:
							attrs[kv.Key] = v
						}
					}
					if exMsg != "" || exType != "" {
						e.Error = &format.ErrorInfo{Message: exMsg, Type: exType, Raw: exStack}
						if e.Kind == format.KindLog {
							e.Kind = format.KindError
						}
					}
					if (e.Kind == format.KindError || e.Kind == format.KindCrash) && e.Error == nil {
						e.Error = &format.ErrorInfo{Message: e.Message}
					}
					if len(attrs) > 0 {
						e.Attrs = attrs
					}
					if lr.EventName != "" {
						e.Kind = format.KindCustom
						if e.Attrs == nil {
							e.Attrs = map[string]any{}
						}
						e.Attrs["event.name"] = lr.EventName
					}
					events = append(events, e)
				}
			}
		}
	}
	if docs == 0 {
		return nil, nil, fmt.Errorf("otlp: no LogsData documents found")
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].Time.Before(events[j].Time) })
	return events, ctx, nil
}

func scalar(v any) any {
	switch v.(type) {
	case string, bool, float64, nil:
		return v
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
