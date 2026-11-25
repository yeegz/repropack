package otlp

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/yeegz/repropack/format"
)

func TestRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	w := format.NewWriter(&buf)
	t0 := time.Date(2026, 11, 25, 1, 2, 3, 400, time.UTC)
	w.WriteEvent(format.Event{Time: t0, Level: "warn", Message: "slow", Attrs: map[string]any{"ms": 1200.0, "route": "/x", "tags": []any{"a"}}, TraceID: "4bf92f3577b34da6a3ce929d0e0e4736", SpanID: "00f067aa0ba902b7"})
	w.WriteEvent(format.Event{Time: t0.Add(time.Second), Kind: "error", Level: "error", Message: "failed", Error: &format.ErrorInfo{Type: "E", Message: "boom", Raw: "stack"}})
	w.WriteEvent(format.Event{Time: t0, Kind: "metric", Metric: &format.Metric{Name: "m", Value: 1}})
	w.Close(format.Manifest{Title: "t", Producer: format.Producer{Name: "p"}, App: &format.AppContext{Name: "svc"}})
	r, err := format.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()), format.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	n, err := Export(r, &out)
	if err != nil || n != 2 {
		t.Fatal(n, err)
	}
	js := out.String()
	for _, want := range []string{`"severityNumber":13`, `"timeUnixNano":"1795568523000000400"`, `"intValue":"1200"`, `"traceId":"4bf92f3577b34da6a3ce929d0e0e4736"`, `"service.name"`} {
		if !strings.Contains(js, want) {
			t.Errorf("missing %s in %s", want, js)
		}
	}
	ev, ctx, err := Import(strings.NewReader(js + js)) // JSON Lines of two documents
	if err != nil {
		t.Fatal(err)
	}
	if len(ev) != 4 || ev[0].Level != "warn" || !ev[0].Time.Equal(t0) || ev[0].Attrs["route"] != "/x" || ev[0].TraceID == "" {
		t.Fatalf("%+v", ev[0])
	}
	var errEv *format.Event
	for i := range ev {
		if ev[i].Kind == "error" {
			errEv = &ev[i]
		}
	}
	if errEv == nil || errEv.Error.Message != "boom" || errEv.Error.Raw != "stack" {
		t.Fatalf("%+v", ev)
	}
	if ctx["resource.service.name"] != "svc" {
		t.Fatal(ctx)
	}
}

func TestImportRejectsGarbage(t *testing.T) {
	if _, _, err := Import(strings.NewReader("{not json")); err == nil {
		t.Fatal("expected error")
	}
	if _, _, err := Import(strings.NewReader("")); err == nil {
		t.Fatal("expected error")
	}
}

func FuzzImport(f *testing.F) {
	f.Add(`{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"timeUnixNano":"1","body":{"kvlistValue":{"values":[{"key":"a","value":{"arrayValue":{"values":[{"intValue":"x"}]}}}]}}}]}]}]}`)
	f.Fuzz(func(t *testing.T, s string) { Import(strings.NewReader(s)) })
}
