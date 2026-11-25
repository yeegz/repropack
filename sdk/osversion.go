package sdk

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

var (
	osOnce          sync.Once
	osVer, osKernel string
)

// osVersion is cached and bounded to 1 s; it never fails a report.
func osVersion() (string, string) {
	osOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		run := func(name string, args ...string) string {
			b, err := exec.CommandContext(ctx, name, args...).Output()
			if err != nil {
				return ""
			}
			return strings.TrimSpace(string(b))
		}
		switch runtime.GOOS {
		case "darwin":
			osVer = run("sw_vers", "-productVersion")
			osKernel = run("uname", "-r")
		case "linux":
			if b, err := os.ReadFile("/etc/os-release"); err == nil {
				for _, l := range strings.Split(string(b), "\n") {
					if v, ok := strings.CutPrefix(l, "PRETTY_NAME="); ok {
						osVer = strings.Trim(v, `"`)
					}
				}
			}
			osKernel = run("uname", "-r")
		case "windows":
			osVer = run("cmd", "/c", "ver")
		}
	})
	return osVer, osKernel
}
