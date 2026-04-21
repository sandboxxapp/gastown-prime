package workspace

import (
	"os"
	"path/filepath"
	"strings"
)

// RigMarker is the filename that identifies a rig directory. Its presence in a
// directory means "this dir is a rig"; absence means "keep looking."
const RigMarker = "rig.json"

// FindRigFromCwd returns the rig name and absolute path for the rig containing
// cwd, or ("", "") if cwd is not inside a rig.
//
// Resolution order:
//  1. Walk upward from cwd toward townRoot looking for a rig.json marker. The
//     first directory with rig.json is the rig.
//  2. Fall back to splitting the path relative to townRoot. This supports
//     legacy layouts without rig.json markers. The "rigs/" container directory
//     (when rigs live under townRoot/rigs/<name>/) is skipped so the returned
//     rig name is the actual rig, not the container.
//
// cwd and townRoot may be relative; both are resolved to absolute paths.
// If cwd is outside townRoot, ("", "") is returned.
func FindRigFromCwd(cwd, townRoot string) (name, path string) {
	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return "", ""
	}
	absTown, err := filepath.Abs(townRoot)
	if err != nil {
		return "", ""
	}

	rel, err := filepath.Rel(absTown, absCwd)
	if err != nil {
		return "", ""
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", ""
	}

	// Pass 1: walk up from cwd looking for a rig.json marker. Stop once we
	// reach townRoot — rig.json at the town level is not a rig.
	current := absCwd
	for current != absTown {
		if _, err := os.Stat(filepath.Join(current, RigMarker)); err == nil {
			return filepath.Base(current), current
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	// Pass 2: path-component fallback for layouts without rig.json.
	if rel == "." {
		return "", ""
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) == 0 || parts[0] == "" {
		return "", ""
	}
	// Skip the "rigs/" container when present (rigs/<rig>/... layout).
	if parts[0] == "rigs" {
		if len(parts) < 2 || parts[1] == "" {
			return "", ""
		}
		return parts[1], filepath.Join(absTown, parts[0], parts[1])
	}
	return parts[0], filepath.Join(absTown, parts[0])
}
