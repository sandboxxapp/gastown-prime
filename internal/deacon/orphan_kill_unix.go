//go:build !windows

package deacon

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func init() {
	listPolecatProcessesFn = findPolecatProcessesByMarker
	terminateProcessFn = func(pid string) { signalPolecatPID(pid, syscall.SIGTERM) }
	killProcessFn = func(pid string) { signalPolecatPID(pid, syscall.SIGKILL) }
}

// findPolecatProcessesByMarker returns the PIDs of live processes whose full
// command line contains marker (an exact substring match). The reaper's own
// PID is excluded so a marker that happens to appear in this process's argv
// (e.g. while testing) can never target self.
//
// Uses `ps -axww -o pid=,command=`: BSD-style flags accepted by both macOS and
// procps; `-ww` defeats command-line truncation so the marker isn't cut off;
// the trailing `=` on each column suppresses headers.
func findPolecatProcessesByMarker(marker string) []string {
	out, err := exec.Command("ps", "-axww", "-o", "pid=,command=").Output()
	if err != nil {
		return nil
	}

	self := strconv.Itoa(os.Getpid())
	var pids []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, marker) {
			continue
		}
		// "PID command..." — the PID is the first whitespace-delimited field.
		fields := strings.SplitN(line, " ", 2)
		pid := strings.TrimSpace(fields[0])
		if pid == "" || pid == self {
			continue
		}
		if _, err := strconv.Atoi(pid); err != nil {
			continue // defensive: skip any non-numeric leading token
		}
		pids = append(pids, pid)
	}
	return pids
}

// signalPolecatPID sends sig to pid. PIDs <= 1 are refused so a malformed or
// empty value can never signal init or a whole process group.
func signalPolecatPID(pid string, sig syscall.Signal) {
	n, err := strconv.Atoi(pid)
	if err != nil || n <= 1 {
		return
	}
	_ = syscall.Kill(n, sig)
}
