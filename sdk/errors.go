package sdk

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"sort"
	"strings"

	"github.com/yeegz/repropack/format"
)

// ErrorInfo converts err (and its errors.Unwrap chain) into format.ErrorInfo,
// with the current goroutine's stack starting skip frames above the caller.
func ErrorInfo(err error, skip int) *format.ErrorInfo {
	root := chain(err, 0)
	root.Stack = Stack(skip + 1)
	return root
}

func chain(err error, depth int) *format.ErrorInfo {
	ei := &format.ErrorInfo{Type: reflect.TypeOf(err).String(), Message: err.Error()}
	if depth >= 16 {
		return ei
	}
	if u := errors.Unwrap(err); u != nil {
		ei.Cause = chain(u, depth+1)
	} else if j, ok := err.(interface{ Unwrap() []error }); ok {
		if errs := j.Unwrap(); len(errs) > 0 && errs[0] != nil {
			ei.Cause = chain(errs[0], depth+1)
		}
	}
	return ei
}

// Stack captures up to 64 frames of the calling goroutine.
func Stack(skip int) []format.Frame {
	pcs := make([]uintptr, 64)
	n := runtime.Callers(skip+2, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	var out []format.Frame
	for {
		f, more := frames.Next()
		if !strings.HasPrefix(f.Function, "runtime.") || len(out) > 0 {
			mod, fn := splitFunc(f.Function)
			out = append(out, format.Frame{Function: fn, Module: mod, File: f.File, Line: f.Line})
		}
		if !more {
			break
		}
	}
	return out
}

func splitFunc(full string) (module, fn string) {
	slash := strings.LastIndex(full, "/")
	dot := strings.Index(full[slash+1:], ".")
	if dot < 0 {
		return "", full
	}
	return full[:slash+1+dot], full[slash+2+dot:]
}

// CapturePanic must be deferred. If the goroutine panics, it writes a crash
// report to path (best effort, bounded by a 5 s provider timeout) and then
// re-panics so normal crash behaviour is preserved.
//
//	defer rec.CapturePanic("crash.repropack")
func (r *Recorder) CapturePanic(path string) {
	v := recover()
	if v == nil {
		return
	}
	r.WriteCrash(path, v, debugStack())
	panic(v)
}

// WriteCrash writes a crash report for a recovered panic value.
func (r *Recorder) WriteCrash(path string, v any, stack []byte) error {
	var err error
	switch t := v.(type) {
	case error:
		err = t
	default:
		err = fmt.Errorf("%w: %v", ErrPanicked, t)
	}
	handled := false
	ei := chain(err, 0)
	ei.Handled = &handled
	ei.Raw = string(stack)
	ei.Stack = parseGoroutineStack(stack)
	ev := format.Event{Kind: format.KindCrash, Level: format.LevelFatal, Message: "panic: " + err.Error(), Error: ei}
	_, werr := r.WriteFile(path, ReportOptions{
		Title:       "Crash: " + truncate(err.Error(), 120),
		Extra:       []format.Event{ev},
		Attachments: []Attachment{BytesAttachment("goroutines.txt", "text/plain", format.RoleDump, allStacks())},
	})
	return werr
}

func debugStack() []byte {
	buf := make([]byte, 64<<10)
	return buf[:runtime.Stack(buf, false)]
}

func allStacks() []byte {
	buf := make([]byte, 1<<20)
	return buf[:runtime.Stack(buf, true)]
}

// parseGoroutineStack extracts frames from runtime.Stack output.
func parseGoroutineStack(b []byte) []format.Frame {
	lines := bytes.Split(b, []byte("\n"))
	var out []format.Frame
	for i := 1; i+1 < len(lines); i += 2 {
		fn := string(bytes.TrimSpace(lines[i]))
		loc := string(bytes.TrimSpace(lines[i+1]))
		if fn == "" || strings.HasPrefix(fn, "created by") || !strings.Contains(loc, ":") {
			break
		}
		if p := strings.LastIndex(fn, "("); p > 0 {
			fn = fn[:p]
		}
		if sp := strings.Index(loc, " +0x"); sp > 0 {
			loc = loc[:sp]
		}
		c := strings.LastIndex(loc, ":")
		line := 0
		fmt.Sscanf(loc[c+1:], "%d", &line)
		mod, f := splitFunc(fn)
		out = append(out, format.Frame{Function: f, Module: mod, File: loc[:c], Line: line})
	}
	// Drop frames inside the panic machinery and this package.
	for len(out) > 0 && (out[0].Function == "panic" || strings.HasPrefix(out[0].Module, "runtime") || isSDKInternal(out[0])) {
		out = out[1:]
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func sortRules(r []format.RedactionRule) {
	sort.Slice(r, func(i, j int) bool { return r[i].ID < r[j].ID })
}

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }

func isSDKInternal(f format.Frame) bool {
	return strings.HasSuffix(f.Module, "repropack/sdk") && (strings.HasPrefix(f.Function, "(*Recorder).") || f.Function == "debugStack")
}
