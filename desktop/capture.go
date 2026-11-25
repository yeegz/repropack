package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Capture modes. Every mode requires the person to choose what is captured
// in the operating system's own picker; the app never captures silently.
const (
	ModeWindow = "window" // click one window
	ModeArea   = "area"   // drag a rectangle
	ModeScreen = "screen" // pick a display in the system recording toolbar
)

var errUnsupported = errors.New("screen capture is not available on this platform yet; add an existing screenshot or recording instead")
var errCancelled = errors.New("capture cancelled")

// Capabilities tells the UI what this platform supports.
type Capabilities struct {
	OS          string `json:"os"`
	Screenshots bool   `json:"screenshots"`
	Recording   bool   `json:"recording"`
	ProcessList bool   `json:"processList"`
	Note        string `json:"note"`
}

func capabilities() Capabilities {
	c := Capabilities{OS: runtime.GOOS}
	switch runtime.GOOS {
	case "darwin":
		_, err := exec.LookPath("screencapture")
		c.Screenshots, c.Recording, c.ProcessList = err == nil, err == nil, true
		c.Note = "macOS asks for Screen Recording permission the first time. You choose the window or area each time."
	case "linux":
		c.ProcessList = true
		c.Note = "Built-in capture is not available on Linux yet. Add screenshots or recordings you made with your desktop's tools."
	default:
		c.Note = "Built-in capture is not available on this system yet. Add screenshots or recordings you made with your system's tools."
	}
	return c
}

// screenshot runs the system picker and writes a PNG to path.
func screenshot(ctx context.Context, mode, path string) error {
	if runtime.GOOS != "darwin" {
		return errUnsupported
	}
	args := []string{"-i", "-x", "-o"}
	switch mode {
	case ModeWindow:
		args = append(args, "-w")
	case ModeArea:
		args = append(args, "-s")
	default:
		args = append(args, "-W")
	}
	cmd := exec.CommandContext(ctx, "/usr/sbin/screencapture", append(args, path)...)
	if err := cmd.Run(); err != nil {
		return err
	}
	if st, err := os.Stat(path); err != nil || st.Size() == 0 {
		os.Remove(path)
		return errCancelled // Esc in the picker produces no file
	}
	return nil
}

// recorder wraps one screen recording.
type recording struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	path    string
	started time.Time
	done    chan error
}

// startRecording opens the system recording picker; macOS shows its own
// toolbar and menu-bar stop button in addition to ours.
func startRecording(path string, maxSeconds int) (*recording, error) {
	if runtime.GOOS != "darwin" {
		return nil, errUnsupported
	}
	if maxSeconds <= 0 || maxSeconds > 600 {
		maxSeconds = 300
	}
	cmd := exec.Command("/usr/sbin/screencapture", "-i", "-J", "video", "-U", "-k", "-x", "-V", strconv.Itoa(maxSeconds), path)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	r := &recording{cmd: cmd, path: path, started: time.Now(), done: make(chan error, 1)}
	go func() { r.done <- cmd.Wait() }()
	return r, nil
}

// stop asks screencapture to finish (SIGINT saves the file) and waits.
func (r *recording) stop() (time.Duration, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cmd.ProcessState == nil {
		r.cmd.Process.Signal(syscall.SIGINT)
	}
	select {
	case <-r.done:
	case <-time.After(15 * time.Second):
		r.cmd.Process.Kill()
		<-r.done
	}
	d := time.Since(r.started)
	if st, err := os.Stat(r.path); err != nil || st.Size() == 0 {
		os.Remove(r.path)
		return d, errCancelled
	}
	return d, nil
}

// Process is an entry in the "watch an app" picker.
type Process struct {
	PID  int    `json:"pid"`
	Name string `json:"name"`
	App  bool   `json:"app"`
}

// listProcesses returns the current user's processes, apps first. Only the
// name and PID are read; nothing is stored unless the person picks one.
func listProcesses(ctx context.Context) ([]Process, error) {
	if runtime.GOOS == "windows" {
		return nil, errUnsupported
	}
	out, err := exec.CommandContext(ctx, "ps", "-U", strconv.Itoa(os.Getuid()), "-o", "pid=,comm=").Output()
	if err != nil {
		return nil, err
	}
	self := os.Getpid()
	seen := map[string]bool{}
	var ps []Process
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		sp := strings.IndexByte(line, ' ')
		if sp < 0 {
			continue
		}
		pid, err := strconv.Atoi(line[:sp])
		if err != nil || pid == self {
			continue
		}
		comm := strings.TrimSpace(line[sp+1:])
		p := Process{PID: pid}
		if i := strings.Index(comm, ".app/Contents/MacOS/"); i >= 0 {
			// Only apps the person installed count as apps; system
			// services under /System are listed as background processes.
			p.App = strings.HasPrefix(comm, "/Applications/") || strings.Contains(comm, "/Users/")
			p.Name = comm[strings.LastIndex(comm[:i], "/")+1 : i]
			if strings.Contains(comm[i:], "Helper") || strings.Contains(comm[:i], ".app/") {
				continue // helpers and nested apps
			}
		} else {
			p.Name = comm[strings.LastIndex(comm, "/")+1:]
		}
		key := p.Name
		if seen[key] && p.App {
			continue
		}
		seen[key] = true
		ps = append(ps, p)
	}
	sort.SliceStable(ps, func(i, j int) bool {
		if ps[i].App != ps[j].App {
			return ps[i].App
		}
		return strings.ToLower(ps[i].Name) < strings.ToLower(ps[j].Name)
	})
	return ps, nil
}

// sample reads CPU % and resident memory for pid.
func sample(ctx context.Context, pid int) (cpu float64, rssBytes float64, err error) {
	out, err := exec.CommandContext(ctx, "ps", "-o", "%cpu=,rss=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0, 0, err
	}
	f := strings.Fields(string(out))
	if len(f) < 2 {
		return 0, 0, errors.New("process exited")
	}
	cpu, _ = strconv.ParseFloat(f[0], 64)
	kb, _ := strconv.ParseFloat(f[1], 64)
	return cpu, kb * 1024, nil
}

// systemInfo collects OS and hardware details shown on the review screen.
func systemInfo(ctx context.Context) (version, build, model, cpu string, mem uint64) {
	run := func(name string, args ...string) string {
		b, err := exec.CommandContext(ctx, name, args...).Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(b))
	}
	if runtime.GOOS == "darwin" {
		version = run("sw_vers", "-productVersion")
		build = run("sw_vers", "-buildVersion")
		model = run("sysctl", "-n", "hw.model")
		cpu = run("sysctl", "-n", "machdep.cpu.brand_string")
		m, _ := strconv.ParseUint(run("sysctl", "-n", "hw.memsize"), 10, 64)
		mem = m
	}
	return
}
