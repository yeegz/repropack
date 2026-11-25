package sdk

import (
	"context"
	"log/slog"
	"strings"

	"github.com/yeegz/repropack/format"
)

// Handler returns a slog.Handler that records every log record into the
// Recorder and then forwards it unchanged to next (which may be nil). Your
// existing log output, format and destinations are not affected.
func (r *Recorder) Handler(next slog.Handler) slog.Handler {
	return &handler{rec: r, next: next}
}

type handler struct {
	rec    *Recorder
	next   slog.Handler
	attrs  []prefixedAttr
	groups []string
}

// prefixedAttr remembers the group path that was open when WithAttrs ran.
type prefixedAttr struct {
	prefix string
	attr   slog.Attr
}

func (h *handler) Enabled(ctx context.Context, l slog.Level) bool {
	if format.LevelRank(slogLevel(l)) >= h.rec.minRank {
		return true
	}
	return h.next != nil && h.next.Enabled(ctx, l)
}

func (h *handler) Handle(ctx context.Context, rec slog.Record) error {
	lvl := slogLevel(rec.Level)
	if format.LevelRank(lvl) >= h.rec.minRank {
		attrs := map[string]any{}
		prefix := strings.Join(h.groups, ".")
		for _, a := range h.attrs {
			put(attrs, a.prefix, a.attr)
		}
		rec.Attrs(func(a slog.Attr) bool { put(attrs, prefix, a); return true })
		e := format.Event{Time: rec.Time, Kind: format.KindLog, Level: lvl, Message: rec.Message, Source: "slog"}
		if len(attrs) > 0 {
			e.Attrs = attrs
		}
		for k, v := range attrs {
			if err, ok := v.(error); ok {
				if e.Error == nil {
					e.Error = chain(err, 0)
				}
				attrs[k] = err.Error()
			}
		}
		if tid, ok := attrs["trace_id"].(string); ok {
			e.TraceID = tid
		}
		if sid, ok := attrs["span_id"].(string); ok {
			e.SpanID = sid
		}
		h.rec.Record(e)
	}
	if h.next != nil && h.next.Enabled(ctx, rec.Level) {
		return h.next.Handle(ctx, rec)
	}
	return nil
}

func put(m map[string]any, prefix string, a slog.Attr) {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return
	}
	key := a.Key
	if prefix != "" {
		key = prefix + "." + key
	}
	if a.Value.Kind() == slog.KindGroup {
		for _, g := range a.Value.Group() {
			put(m, key, g)
		}
		return
	}
	switch a.Value.Kind() {
	case slog.KindString:
		m[key] = a.Value.String()
	case slog.KindInt64:
		m[key] = a.Value.Int64()
	case slog.KindUint64:
		m[key] = a.Value.Uint64()
	case slog.KindFloat64:
		m[key] = a.Value.Float64()
	case slog.KindBool:
		m[key] = a.Value.Bool()
	case slog.KindDuration:
		m[key] = a.Value.Duration().String()
	case slog.KindTime:
		m[key] = a.Value.Time().UTC().Format("2006-01-02T15:04:05.000Z07:00")
	default:
		if err, ok := a.Value.Any().(error); ok {
			m[key] = err
			return
		}
		m[key] = a.Value.String()
	}
}

func (h *handler) WithAttrs(as []slog.Attr) slog.Handler {
	c := *h
	prefix := strings.Join(h.groups, ".")
	c.attrs = append([]prefixedAttr(nil), h.attrs...)
	for _, a := range as {
		c.attrs = append(c.attrs, prefixedAttr{prefix, a})
	}
	if h.next != nil {
		c.next = h.next.WithAttrs(as)
	}
	return &c
}

func (h *handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	c := *h
	c.groups = append(append([]string(nil), h.groups...), name)
	if h.next != nil {
		c.next = h.next.WithGroup(name)
	}
	return &c
}

func slogLevel(l slog.Level) string {
	switch {
	case l < slog.LevelDebug:
		return format.LevelTrace
	case l < slog.LevelInfo:
		return format.LevelDebug
	case l < slog.LevelWarn:
		return format.LevelInfo
	case l < slog.LevelError:
		return format.LevelWarn
	case l < slog.LevelError+4:
		return format.LevelError
	}
	return format.LevelFatal
}
