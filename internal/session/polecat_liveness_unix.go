//go:build !windows

package session

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func init() {
	scanProcessesByMarker = findProcessesByMarker
}

// findProcessesByMarker returns the PIDs of live processes whose full command
// line contains marker (an exact substring match). This process's own PID is
// excluded so a marker that happens to appear in this process's argv (e.g. while
// testing) can never target self.
//
// Uses `ps -axww -o pid=,command=`: BSD-style flags accepted by both macOS and
// procps; `-ww` defeats command-line truncation so the marker isn't cut off;
// the trailing `=` on each column suppresses headers. This mirrors the deacon
// reaper's orphan scan (deacon/orphan_kill_unix.go) so the namepool gate and the
// reaper agree on what "a live polecat process" means.
func findProcessesByMarker(marker string) []string {
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
