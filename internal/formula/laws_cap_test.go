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
// Why the EMBED needs this guard even though `gt prime` never renders a law:
// both prime renderers return early on len(Steps) == 0 (prime_molecule.go), and
// laws-* carry [rule] prose with zero [[steps]], so nothing renders them. The
// embed still reaches agents, by a less obvious route — UpdateFormulas
// (embed.go:277) writes embedded content onto the town's .beads/formulas/, and
// laws-* are absent from .installed.json, so they take the "untracked - safe to
// update" branch (embed.go:317-322) and get overwritten from the embed by
// `gt doctor --fix` and `gt upgrade`. The bridge's SessionStart hooks then read
// those files off disk. So an over-cap law here becomes an over-cap law
// delivered there.
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
