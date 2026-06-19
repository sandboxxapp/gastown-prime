//go:build windows

package deacon

// On Windows there are no POSIX signals or `ps` semantics the reaper relies on,
// and the daemon's polecat fleet does not run there. Wire no-op seams so the
// shared kill logic compiles and reports "none".
func init() {
	listPolecatProcessesFn = func(string) []string { return nil }
	terminateProcessFn = func(string) {}
	killProcessFn = func(string) {}
}
