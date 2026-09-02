package formula

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// EnvLawsAuthoringDir points the drift guard at the town whose
// .beads/formulas/ holds the authoring copies of the laws-* files. It exists so
// the guard can be aimed explicitly (CI, a second town, a bisect) instead of
// relying on the ambient GT_ROOT / GT_TOWN_ROOT of a dispatched session.
const EnvLawsAuthoringDir = "GT_LAWS_AUTHORING_DIR"

// TestEmbeddedLawsMatchAuthoringCopy fails when an embedded laws-* file has
// drifted from the town's authoring copy in .beads/formulas/.
//
// # Why this guard exists
//
// The laws-* family lives in TWO places and BOTH are live delivery paths:
//
//   - The embed here is what `gt prime` renders. outputRoleLaws
//     (internal/cmd/prime_laws.go:98) calls GetEmbeddedFormulaContent, which
//     reads formulasFS and has NO disk path — the embed is not a fallback, it
//     is the ONLY source a rendered law can come from. That has been true since
//     PR #71 (452eae4f); the older claim that "gt prime never renders a law" is
//     obsolete.
//   - The town's .beads/formulas/ copy is what the bridge's SessionStart hooks
//     read, and it is ALSO downstream of this embed: laws-* are absent from
//     .installed.json, so UpdateFormulas classifies them "untracked - safe to
//     update" (embed.go:320-323) and `gt doctor --fix` / `gt upgrade` overwrite
//     the disk copy from here.
//
// So the two copies must agree, and a divergence is dangerous in both
// directions: an agent primed by gt gets the embed's text, while a bridge-side
// law edit is armed to be silently reverted by the next `gt doctor --fix`.
//
// # Why a version comparison is not enough
//
// Measured 2026-09-02 (sbx-gastown-ezric): five laws-* files had drifted, and
// FOUR of them at IDENTICAL version numbers — laws-context-db (6 vs 6),
// laws-mayor (9 vs 9), laws-polecat (12 vs 12), laws-prospector (5 vs 5); only
// laws-deacon had bumped (12 vs 13). A `version =` diff reported "in sync"
// while the shipped binary was rendering polecats a context-db recipe missing
// the X-Ctx-Agent header. The guard therefore compares CONTENT, not versions.
//
// # Scope, stated honestly
//
// This guard needs both trees on disk, so it runs where the drift is actually
// introduced and observed — an agent or operator working inside a town — and
// SKIPS in a bare CI checkout that has no bridge tree. It is a pre-commit-grade
// check, not a hosted-CI one. Closing that gap needs a bridge-side check, which
// lives in the other repo.
func TestEmbeddedLawsMatchAuthoringCopy(t *testing.T) {
	t.Parallel()

	dir := lawsAuthoringDir()
	if dir == "" {
		t.Skipf("no authoring copy reachable: set %s (or GT_ROOT / GT_TOWN_ROOT) "+
			"to a town whose .beads/formulas/ holds the laws-* authoring copies. "+
			"This guard cannot detect embed/disk law drift without both trees.",
			EnvLawsAuthoringDir)
	}
	t.Logf("comparing embedded laws-* against authoring copies in %s", dir)

	entries, err := fs.ReadDir(formulasFS, "formulas")
	if err != nil {
		t.Fatalf("reading embedded formulas: %v", err)
	}

	seen := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, "laws-") ||
			!strings.HasSuffix(name, ".formula.toml") {
			continue
		}
		seen++
		t.Run(name, func(t *testing.T) {
			embedded, err := formulasFS.ReadFile("formulas/" + name)
			if err != nil {
				t.Fatalf("reading embedded formula: %v", err)
			}

			authored, err := os.ReadFile(filepath.Join(dir, name))
			if os.IsNotExist(err) {
				// Embed-only law: nothing to compare against, and not a
				// defect — a law can be introduced here before the town has
				// it. The clobber path will install it on the next update.
				t.Skipf("no authoring copy in %s", dir)
			}
			if err != nil {
				t.Fatalf("reading authoring copy: %v", err)
			}

			if string(embedded) != string(authored) {
				t.Errorf("embedded %s has DRIFTED from the authoring copy at %s.\n"+
					"Both copies are live: the embed is what `gt prime` renders "+
					"(prime_laws.go:98, embed-only, no disk fallback), and the "+
					"authoring copy is what the bridge's SessionStart hooks read "+
					"AND what `gt doctor --fix` will silently overwrite from this "+
					"embed (embed.go:320-323).\n"+
					"Adjudicate PER FILE before syncing — read the embed-only side "+
					"of the diff and confirm it is superseded text, because the "+
					"embed has been ahead before (PR #68 left mol-witness-patrol "+
					"and mol-refinery-patrol alone for exactly this reason):\n"+
					"  diff -u internal/formula/formulas/%s %s/%s",
					name, dir, name, dir, name)
			}
		})
	}

	if seen == 0 {
		t.Fatal("no laws-*.formula.toml found in the embed — this guard is " +
			"silently passing over an empty set")
	}
}

// lawsAuthoringDir resolves the directory holding the authoring copies of the
// laws-* files, or "" when none is reachable.
//
// GT_LAWS_AUTHORING_DIR is taken as the formulas directory itself; the town-root
// variables are taken as a town and have .beads/formulas appended. A path that
// does not resolve to a directory is treated as absent rather than as an error,
// so a stale variable degrades to a skip.
func lawsAuthoringDir() string {
	if dir := strings.TrimSpace(os.Getenv(EnvLawsAuthoringDir)); dir != "" {
		if isDir(dir) {
			return dir
		}
		return ""
	}
	for _, env := range []string{"GT_ROOT", "GT_TOWN_ROOT"} {
		root := strings.TrimSpace(os.Getenv(env))
		if root == "" {
			continue
		}
		if dir := filepath.Join(root, ".beads", "formulas"); isDir(dir) {
			return dir
		}
	}
	return ""
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
