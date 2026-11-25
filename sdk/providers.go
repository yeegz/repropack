package sdk

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"github.com/yeegz/repropack/format"
)

// ProviderData is what a Provider contributes to a report.
type ProviderData struct {
	Context     map[string]any
	Events      []format.Event
	Attachments []Attachment
}

// Provider contributes diagnostics when a report is written. Implementations
// must respect ctx cancellation and must not block indefinitely.
type Provider interface {
	Name() string
	Collect(ctx context.Context) (ProviderData, error)
}

// ProviderFunc adapts a function to Provider.
type ProviderFunc struct {
	ID string
	Fn func(ctx context.Context) (ProviderData, error)
}

func (p ProviderFunc) Name() string { return p.ID }
func (p ProviderFunc) Collect(ctx context.Context) (ProviderData, error) {
	return p.Fn(ctx)
}

func (r *Recorder) collect(ctx context.Context) ProviderData {
	var all ProviderData
	all.Context = map[string]any{}
	type res struct {
		name string
		d    ProviderData
		err  error
	}
	ch := make(chan res, len(r.opt.Providers))
	for _, p := range r.opt.Providers {
		go func(p Provider) {
			defer func() {
				if v := recover(); v != nil {
					ch <- res{p.Name(), ProviderData{}, fmt.Errorf("provider panicked: %v", v)}
				}
			}()
			d, err := p.Collect(ctx)
			ch <- res{p.Name(), d, err}
		}(p)
	}
	got := make([]res, 0, len(r.opt.Providers))
	for range r.opt.Providers {
		select {
		case x := <-ch:
			got = append(got, x)
		case <-ctx.Done():
			all.Events = append(all.Events, format.Event{Kind: format.KindLog, Level: format.LevelWarn, Source: "repropack",
				Message: "provider collection timed out; some providers were skipped"})
			goto done
		}
	}
done:
	sort.Slice(got, func(i, j int) bool { return got[i].name < got[j].name })
	for _, x := range got {
		if x.err != nil {
			all.Events = append(all.Events, format.Event{Kind: format.KindLog, Level: format.LevelWarn, Source: "repropack",
				Message: "provider " + x.name + " failed", Attrs: map[string]any{"error": x.err.Error()}})
			continue
		}
		for k, v := range x.d.Context {
			all.Context[x.name+"."+k] = v
		}
		all.Events = append(all.Events, x.d.Events...)
		all.Attachments = append(all.Attachments, x.d.Attachments...)
	}
	return all
}

// RuntimeProvider adds Go runtime state: goroutines, heap, GC, build info,
// plus a heap metric event.
func RuntimeProvider() Provider {
	return ProviderFunc{ID: "go", Fn: func(ctx context.Context) (ProviderData, error) {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		c := map[string]any{
			"goroutines":     runtime.NumGoroutine(),
			"gomaxprocs":     runtime.GOMAXPROCS(0),
			"heap_alloc":     ms.HeapAlloc,
			"heap_objects":   ms.HeapObjects,
			"gc_cycles":      ms.NumGC,
			"gc_pause_total": time.Duration(ms.PauseTotalNs).String(),
		}
		if bi, ok := debug.ReadBuildInfo(); ok {
			c["main_module"] = bi.Main.Path
			c["main_version"] = bi.Main.Version
			for _, s := range bi.Settings {
				switch s.Key {
				case "vcs.revision", "vcs.modified", "GOOS", "GOARCH", "CGO_ENABLED":
					c[s.Key] = s.Value
				}
			}
		}
		return ProviderData{Context: c, Events: []format.Event{
			{Kind: format.KindMetric, Source: "go", Metric: &format.Metric{Name: "go.heap_alloc", Value: float64(ms.HeapAlloc), Unit: "By"}},
			{Kind: format.KindMetric, Source: "go", Metric: &format.Metric{Name: "go.goroutines", Value: float64(runtime.NumGoroutine()), Unit: "{goroutine}"}},
		}}, nil
	}}
}

// EnvProvider includes only the named environment variables. Nothing is
// read implicitly; values are redacted like any other context.
func EnvProvider(names ...string) Provider {
	return ProviderFunc{ID: "env", Fn: func(ctx context.Context) (ProviderData, error) {
		c := map[string]any{}
		for _, n := range names {
			if v, ok := os.LookupEnv(n); ok {
				c[n] = v
			}
		}
		return ProviderData{Context: c}, nil
	}}
}

// GoroutineDumpProvider attaches a full goroutine dump as text.
func GoroutineDumpProvider() Provider {
	return ProviderFunc{ID: "goroutines", Fn: func(ctx context.Context) (ProviderData, error) {
		return ProviderData{Attachments: []Attachment{BytesAttachment("goroutines.txt", "text/plain", format.RoleDump, allStacks())}}, nil
	}}
}

func osContext() *format.OSContext {
	o := &format.OSContext{Name: runtime.GOOS, Arch: runtime.GOARCH}
	if tz, _ := time.Now().Zone(); tz != "" {
		o.Timezone = time.Local.String()
	}
	o.Locale = firstNonEmpty(os.Getenv("LC_ALL"), os.Getenv("LANG"))
	o.Version, o.Kernel = osVersion()
	return o
}

func deviceInfo() *format.DeviceInfo {
	return &format.DeviceInfo{CPUCores: runtime.NumCPU()}
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}
