package formula

import (
	"io/fs"
	"strings"
	"testing"
	"unicode/utf8"
)

// Claude Code's additionalContext limit is a hard, non-configurable 10,000
// characters. Over-cap content is not truncated — it is SPILLED to a file with
// only a ~2 KB preview reaching the session, so an over-cap law does not arrive
// short, it DISAPPEARS. lawSummaryBudget leaves headroom for the hook wrapper
// that frames the summary before delivery.
const (
	lawSummaryHardCap = 10000
	lawSummaryBudget  = 9000
)

// TestLawSummariesFitContextCap guards the embedded laws-* constraint formulas
// against re-growing past the delivery cap.
//
// Why the EMBED needs this guard — it reaches agents by TWO routes, and the
// first one is easy to miss:
//
//  1. READ. `gt prime` renders the role's laws from this embed and nowhere
//     else: outputRoleLaws (internal/cmd/prime_laws.go:98) calls
//     GetEmbeddedFormulaContent, which reads formulasFS and has no disk path.
//     (Before PR #71 / 452eae4f there was no prime->laws code at all, which is
//     where the since-obsolete "gt prime never renders a law" claim came from.
//     The len(Steps) == 0 guards in prime_molecule.go are real but belong to
//     the MOLECULE renderer, which laws never reach.)
//  2. WRITE. UpdateFormulas (embed.go:277) writes embedded content onto the
//     town's .beads/formulas/, and laws-* are absent from .installed.json, so
//     they take the "untracked - safe to update" branch (embed.go:320-323) and
//     get overwritten from the embed by `gt doctor --fix` and `gt upgrade`.
//     The bridge's SessionStart hooks then read those files off disk.
//
// So an over-cap law here is delivered over-cap on both routes.
//
// Measured in characters, not bytes: the laws are em-dash-heavy UTF-8 and
// len() on the byte slice overstates by ~1%.
func TestLawSummariesFitContextCap(t *testing.T) {
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
			data, err := formulasFS.ReadFile("formulas/" + name)
			if err != nil {
				t.Fatalf("reading formula: %v", err)
			}
			f, err := Parse(data)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if f.Rule == nil {
				t.Fatalf("laws-* formula has no [rule] block")
			}
			// Count runes, not bytes — the cap is a character limit.
			n := utf8.RuneCountInString(f.Rule.Summary)
			switch {
			case n > lawSummaryHardCap:
				t.Errorf("[rule].summary is %d chars, over the %d hard cap "+
					"(%d%%): this law will be SPILLED to a file and will not "+
					"reach the agent. Move content to the LAW TAIL.",
					n, lawSummaryHardCap, 100*n/lawSummaryHardCap)
			case n > lawSummaryBudget:
				t.Errorf("[rule].summary is %d chars, over the %d budget "+
					"(%d%%): under the %d hard cap but with no headroom for the "+
					"hook wrapper. Move content to the LAW TAIL.",
					n, lawSummaryBudget, 100*n/lawSummaryBudget, lawSummaryHardCap)
			default:
				t.Logf("%d chars (%d%% of budget)", n, 100*n/lawSummaryBudget)
			}
		})
	}

	if seen == 0 {
		t.Fatal("no laws-*.formula.toml found in the embed — this guard is " +
			"silently passing over an empty set")
	}
}
