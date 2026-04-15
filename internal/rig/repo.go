package rig

import (
	"os"
	"path/filepath"
)

// bareRepoCandidates lists directory names to check for the shared bare repo,
// in priority order. ".repo.git" is the canonical name created by `gt rig add`.
// "repo.git" supports bridge rig layouts where the bare clone lives at
// rigs/<rig>/repo.git (without the dot prefix).
var bareRepoCandidates = []string{".repo.git", "repo.git"}

// FindBareRepo returns the absolute path of the bare repository directory
// inside rigPath, or "" if none exists. It checks candidates in priority order:
// .repo.git (standard), then repo.git (bridge rig layout).
//
// As a fallback, if rigPath is <townRoot>/<rigName>/, it also checks
// <townRoot>/rigs/<rigName>/ for the same candidates. This supports bridge
// layouts where rig metadata (including bare repos) lives under a rigs/
// subdirectory while gt creates workspace dirs at the town root level.
func FindBareRepo(rigPath string) string {
	for _, name := range bareRepoCandidates {
		p := filepath.Join(rigPath, name)
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return p
		}
	}

	// Fallback: check <townRoot>/rigs/<rigName>/ for bridge rig layout.
	// rigPath is typically <townRoot>/<rigName>/, so derive the rigs/ path.
	parent := filepath.Dir(rigPath)
	rigName := filepath.Base(rigPath)
	rigsPath := filepath.Join(parent, "rigs", rigName)
	if rigsPath != rigPath {
		for _, name := range bareRepoCandidates {
			p := filepath.Join(rigsPath, name)
			if info, err := os.Stat(p); err == nil && info.IsDir() {
				return p
			}
		}
	}

	return ""
}
