//go:build windows

package session

// On Windows there are no `ps` semantics the marker scan relies on, and the
// daemon's polecat fleet does not run there. Wire a no-op scanner so
// PolecatProcessAlive compiles and always reports "not alive".
func init() {
	scanProcessesByMarker = func(string) []string { return nil }
}
