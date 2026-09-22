package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/style"
)

// The armed-hook hazard, in one paragraph.
//
// A polecat slot name is reissued by the namepool while a bead from a previous
// occupant is still `hooked` against it. `gt prime` then had to choose, and
// chose `hookedBeads[0]` — which `bd list`'s ORDER BY priority ASC, created_at
// DESC makes the *highest-priority* hook, not the one just slung. The polecat
// then runs the wrong bead, correctly and to completion, with every signal
// green. Measured three times on 2026-09-22 (sbx-gastown-qrfaa6); the mayor's
// sweep found 14 slots holding 14 hooked beads, 9 of them naming merged PRs.
//
// Two additive guards live here, and NEITHER mutates a bead:
//
//   - renderForeignHookWarning — `gt sling`, operator-facing: the slot you just
//     dispatched into already carries someone else's hook. Clear it or move on,
//     but you cannot say you were not told.
//   - renderHookAmbiguityWarning — `gt prime`, agent-facing: more than one bead
//     is hooked here, this is the one being run and these are the ones that lost.
//
// The arbitration itself is beads.SelectHookedBead. The allocation-side
// liveness gate (polecat.Manager.nameHasLiveOccupant) is deliberately NOT
// involved: it reuses process/session liveness on purpose and coupling it to
// bead state was consciously declined by its author (sbx-gastown-gsyki).

// truncateDisplay shortens s to at most maxLen runes, appending an ellipsis.
// Rune-aware because these lines carry warning glyphs and bead titles that are
// not guaranteed ASCII; a byte slice would split a rune and emit mojibake.
func truncateDisplay(s string, maxLen int) string {
	if maxLen <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen == 1 {
		return "…"
	}
	return string(runes[:maxLen-1]) + "…"
}

// describeBead renders one bead for a warning line: id, priority, attach stamp
// and a clipped title.
func describeBead(issue *beads.Issue) string {
	if issue == nil {
		return "(nil bead)"
	}
	stamp := "no attached_at"
	if t, ok := beads.AttachedAtTime(issue); ok {
		stamp = "attached " + t.Format("2006-01-02T15:04:05Z")
	}
	return fmt.Sprintf("%s  [P%d]  %s  (%s)",
		issue.ID, issue.Priority, truncateDisplay(issue.Title, 60), stamp)
}

// foreignHooks returns the beads hooked to a slot that are NOT the bead being
// slung — i.e. residue from a previous occupant of a recycled slot name.
func foreignHooks(hooked []*beads.Issue, slungBeadID string) []*beads.Issue {
	var foreign []*beads.Issue
	for _, issue := range hooked {
		if issue == nil || issue.ID == slungBeadID {
			continue
		}
		foreign = append(foreign, issue)
	}
	return foreign
}

// renderForeignHookWarning builds the `gt sling` warning for a slot that
// already carries a hooked bead other than the one just slung.
//
// listErr is the error from the hooked-bead query, or nil. It is a parameter
// rather than a swallowed detail on purpose: an absent warning must never be
// indistinguishable from a clean slot, so a check that could not RUN says so
// just as loudly as a check that FAILED.
//
// Returns "" only when the query succeeded and the slot carries no foreign hook.
func renderForeignHookWarning(targetAgent, slungBeadID string, hooked []*beads.Issue, listErr error) string {
	if listErr != nil {
		var b strings.Builder
		fmt.Fprintf(&b, "\n%s\n", style.Warning.Render(
			"⚠️  ARMED-HOOK CHECK DID NOT RUN — this slot is NOT known to be clean"))
		fmt.Fprintf(&b, "    slot:  %s\n", targetAgent)
		fmt.Fprintf(&b, "    error: %v\n", listErr)
		fmt.Fprintf(&b, "    A stale hook on this slot would hijack the dispatch. Check by hand:\n")
		fmt.Fprintf(&b, "      bd list --status=hooked --assignee %s\n\n", targetAgent)
		return b.String()
	}

	foreign := foreignHooks(hooked, slungBeadID)
	if len(foreign) == 0 {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "\n%s\n", style.Warning.Render(fmt.Sprintf(
		"⚠️  ARMED HOOK — slot %s already carries %d other hooked bead(s)", targetAgent, len(foreign))))
	fmt.Fprintf(&b, "    slung now:   %s\n", slungBeadID)
	for _, issue := range foreign {
		fmt.Fprintf(&b, "    also hooked: %s\n", describeBead(issue))
	}
	fmt.Fprintf(&b, "    The slot name was recycled while these stayed hooked. gt prime runs the\n")
	fmt.Fprintf(&b, "    most recently attached bead, but a stale hook stays armed for the NEXT\n")
	fmt.Fprintf(&b, "    dispatch into this slot. Clear it, or re-dispatch with --create:\n")
	for _, issue := range foreign {
		fmt.Fprintf(&b, "      gt unsling %s\n", issue.ID)
	}
	b.WriteString("\n")
	return b.String()
}

// warnForeignHook performs the sling-side armed-hook check and prints the
// warning. Never mutates a bead — detection only, by design: clearing a hook
// would discard genuinely live work whose polecat merely died.
func warnForeignHook(hookDir, targetAgent, slungBeadID string) {
	b := beads.New(hookDir)
	hooked, err := b.List(beads.ListOptions{
		Status:   beads.StatusHooked,
		Assignee: targetAgent,
		Priority: -1,
	})
	if msg := renderForeignHookWarning(targetAgent, slungBeadID, hooked, err); msg != "" {
		fmt.Print(msg)
	}
}

// renderHookAmbiguityWarning builds the `gt prime` warning shown when a slot
// carries more than one hooked bead. The agent is told which bead it is about
// to run, which ones lost, and how the pick was made — converting a silent
// wrong answer into a visible one.
//
// Returns "" when there is nothing ambiguous (0 or 1 hooked bead).
func renderHookAmbiguityWarning(agentID string, winner *beads.Issue, losers []*beads.Issue) string {
	if winner == nil || len(losers) == 0 {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "\n%s\n", style.Bold.Render(fmt.Sprintf(
		"## ⚠️  AMBIGUOUS HOOK — %d beads are hooked to %s", len(losers)+1, agentID)))
	fmt.Fprintf(&b, "RUNNING: %s\n", describeBead(winner))
	for _, issue := range losers {
		fmt.Fprintf(&b, "IGNORED: %s\n", describeBead(issue))
	}
	if _, stamped := beads.AttachedAtTime(winner); stamped {
		b.WriteString("Picked by most recent attached_at (the dispatch stamp gt sling writes).\n")
	} else {
		b.WriteString("No bead carries a parseable attached_at — this pick is by bd's list order\n")
		b.WriteString("(priority, then newest-created) and may well be the WRONG bead.\n")
	}
	b.WriteString("A recycled slot name inherits the previous occupant's hook. If RUNNING is not\n")
	b.WriteString("the work you were dispatched on, STOP — do not work it, do not close it.\n")
	b.WriteString("Report it and let the mayor run `gt unsling <bead>` on the stale one.\n\n")
	return b.String()
}

// warnHookAmbiguity prints the prime-side ambiguity warning to stderr, matching
// the DATABASE ERROR warning's channel (prime.go, GH#2638) so it lands in the
// agent's pane alongside the rest of prime's diagnostics.
var warnHookAmbiguity = func(agentID string, winner *beads.Issue, losers []*beads.Issue) {
	if msg := renderHookAmbiguityWarning(agentID, winner, losers); msg != "" {
		fmt.Fprint(os.Stderr, msg)
	}
}
