package beads

import (
	"sort"
	"time"
)

// attachedAtLayouts are the timestamp formats accepted for the `attached_at`
// dispatch stamp. `gt sling` writes RFC3339 in UTC
// (internal/cmd/sling_helpers.go); the looser forms are tolerated because the
// field lives as free text in a bead description and has been hand-edited.
var attachedAtLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
}

// AttachedAtTime parses an issue's `attached_at` dispatch stamp — the moment
// `gt sling` attached the work — returning ok=false when the bead carries no
// parseable stamp (hand-hooked beads, and slings that attached no molecule).
func AttachedAtTime(issue *Issue) (time.Time, bool) {
	fields := ParseAttachmentFields(issue)
	if fields == nil || fields.AttachedAt == "" {
		return time.Time{}, false
	}
	for _, layout := range attachedAtLayouts {
		if t, err := time.Parse(layout, fields.AttachedAt); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// SelectHookedBead arbitrates between the beads hooked to a single agent and
// returns the one to run plus every bead that lost.
//
// WHY THIS EXISTS: a polecat slot name is recycled while a bead from a previous
// occupant is still `hooked` against it — nothing ever transitions a bead out
// of `hooked` except `gt exit` / `bd close` / `gt unsling`, and the namepool's
// liveness gate does not consult bead state. Callers used to take
// `hookedBeads[0]`, and `bd list` orders by priority ASC then created_at DESC,
// so a *stale P0 beat a freshly-slung P1 deterministically* and the polecat ran
// the wrong work with every signal green (sbx-gastown-qrfaa6).
//
// The order is a total, deterministic one over the input slice:
//
//  1. a bead with a parseable `attached_at` beats one without;
//  2. among stamped beads, the most recently attached wins;
//  3. otherwise the caller's input order is preserved (stable sort).
//
// Rule 3 matters: when no bead carries a stamp this returns `hooked[0]`, i.e.
// exactly the previous behaviour, so paths that pass a non-dispatch list (e.g.
// prime's `in_progress` fallback, whose beads are never stamped) are unchanged.
//
// FAILURE MODE: `attached_at` is written only when a sling attaches a molecule,
// and only if the field was empty (sling_helpers.go), so a re-slung or
// formula-less dispatch can carry a stale stamp or none at all. Then rule 3
// applies and the stale bead can still win. That is why every caller that can
// address a human must ALSO announce the ambiguity — arbitration narrows the
// hazard, the warning is what removes it.
//
// Returns (nil, nil) for an empty input.
func SelectHookedBead(hooked []*Issue) (winner *Issue, losers []*Issue) {
	if len(hooked) == 0 {
		return nil, nil
	}
	if len(hooked) == 1 {
		return hooked[0], nil
	}

	ordered := make([]*Issue, len(hooked))
	copy(ordered, hooked)

	sort.SliceStable(ordered, func(i, j int) bool {
		ti, iStamped := AttachedAtTime(ordered[i])
		tj, jStamped := AttachedAtTime(ordered[j])
		if iStamped != jStamped {
			return iStamped // stamped sorts ahead of unstamped
		}
		if !iStamped || ti.Equal(tj) {
			return false // indistinguishable: keep input order
		}
		return ti.After(tj)
	})

	return ordered[0], ordered[1:]
}
