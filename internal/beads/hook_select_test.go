package beads

import (
	"testing"
)

// hookedBead builds an issue as it appears in `bd list --status=hooked`.
// attachedAt "" means the bead carries no dispatch stamp.
func hookedBead(id string, priority int, attachedAt string) *Issue {
	desc := "some prose about the work"
	if attachedAt != "" {
		desc = "attached_molecule: wisp-" + id + "\nattached_at: " + attachedAt + "\n\n" + desc
	}
	return &Issue{
		ID:          id,
		Title:       "work item " + id,
		Status:      StatusHooked,
		Priority:    priority,
		Description: desc,
	}
}

func ids(issues []*Issue) []string {
	out := make([]string, 0, len(issues))
	for _, issue := range issues {
		out = append(out, issue.ID)
	}
	return out
}

func equalIDs(got []*Issue, want []string) bool {
	g := ids(got)
	if len(g) != len(want) {
		return false
	}
	for i := range g {
		if g[i] != want[i] {
			return false
		}
	}
	return true
}

func TestSelectHookedBead(t *testing.T) {
	tests := []struct {
		name       string
		hooked     []*Issue
		wantWinner string
		wantLosers []string
	}{
		{
			name:       "empty input selects nothing",
			hooked:     nil,
			wantWinner: "",
		},
		{
			name:       "exactly one hook is returned unchanged with no losers",
			hooked:     []*Issue{hookedBead("only", 1, "2026-09-22T14:44:54Z")},
			wantWinner: "only",
		},
		{
			// The measured defect (sbx-gastown-qrfaa6): bd orders by priority
			// ASC, so the stale P0 arrives first and [0] ran it. The freshly
			// slung P1 must win.
			name: "stale high-priority hook loses to the freshly slung one",
			hooked: []*Issue{
				hookedBead("stale-p0", 0, "2026-09-17T10:02:11Z"),
				hookedBead("fresh-p1", 1, "2026-09-22T14:44:54Z"),
			},
			wantWinner: "fresh-p1",
			wantLosers: []string{"stale-p0"},
		},
		{
			name: "newest attached_at wins regardless of input order",
			hooked: []*Issue{
				hookedBead("newest", 2, "2026-09-22T14:44:54Z"),
				hookedBead("oldest", 2, "2026-01-02T03:04:05Z"),
				hookedBead("middle", 2, "2026-05-05T05:05:05Z"),
			},
			wantWinner: "newest",
			wantLosers: []string{"middle", "oldest"},
		},
		{
			name: "a stamped bead beats an unstamped one even when it sorts later",
			hooked: []*Issue{
				hookedBead("handhooked", 0, ""),
				hookedBead("slung", 3, "2026-01-02T03:04:05Z"),
			},
			wantWinner: "slung",
			wantLosers: []string{"handhooked"},
		},
		{
			// Rule 3: with nothing to arbitrate on, behaviour is byte-identical
			// to the old hookedBeads[0] — this is what keeps prime's
			// in_progress fallback (never stamped) unchanged.
			name: "no stamps anywhere preserves input order",
			hooked: []*Issue{
				hookedBead("first", 0, ""),
				hookedBead("second", 1, ""),
				hookedBead("third", 2, ""),
			},
			wantWinner: "first",
			wantLosers: []string{"second", "third"},
		},
		{
			name: "identical stamps preserve input order",
			hooked: []*Issue{
				hookedBead("first", 0, "2026-09-22T14:44:54Z"),
				hookedBead("second", 1, "2026-09-22T14:44:54Z"),
			},
			wantWinner: "first",
			wantLosers: []string{"second"},
		},
		{
			name: "an unparseable stamp is treated as unstamped",
			hooked: []*Issue{
				hookedBead("garbage", 0, "last tuesday"),
				hookedBead("stamped", 1, "2026-01-02T03:04:05Z"),
			},
			wantWinner: "stamped",
			wantLosers: []string{"garbage"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			winner, losers := SelectHookedBead(tt.hooked)

			if tt.wantWinner == "" {
				if winner != nil {
					t.Fatalf("want nil winner, got %s", winner.ID)
				}
				if len(losers) != 0 {
					t.Fatalf("want no losers, got %v", ids(losers))
				}
				return
			}

			if winner == nil {
				t.Fatalf("want winner %s, got nil", tt.wantWinner)
			}
			if winner.ID != tt.wantWinner {
				t.Errorf("winner = %s, want %s", winner.ID, tt.wantWinner)
			}
			if !equalIDs(losers, tt.wantLosers) {
				t.Errorf("losers = %v, want %v", ids(losers), tt.wantLosers)
			}
		})
	}
}

// The callers pass a slice they got from beads.List and keep using it; the
// arbitration must not reorder it underneath them.
func TestSelectHookedBeadDoesNotMutateInput(t *testing.T) {
	input := []*Issue{
		hookedBead("stale-p0", 0, "2026-09-17T10:02:11Z"),
		hookedBead("fresh-p1", 1, "2026-09-22T14:44:54Z"),
	}
	before := ids(input)

	winner, _ := SelectHookedBead(input)
	if winner.ID != "fresh-p1" {
		t.Fatalf("winner = %s, want fresh-p1", winner.ID)
	}

	if after := ids(input); after[0] != before[0] || after[1] != before[1] {
		t.Errorf("input slice was reordered: %v -> %v", before, after)
	}
}

func TestAttachedAtTime(t *testing.T) {
	tests := []struct {
		name      string
		issue     *Issue
		wantOK    bool
		wantRFC   string
	}{
		{name: "nil issue", issue: nil, wantOK: false},
		{name: "no attachment fields", issue: &Issue{ID: "x", Description: "plain prose"}, wantOK: false},
		{
			name:    "rfc3339 utc as gt sling writes it",
			issue:   hookedBead("a", 1, "2026-09-22T14:44:54Z"),
			wantOK:  true,
			wantRFC: "2026-09-22T14:44:54Z",
		},
		{
			name:    "offset timestamp is normalised to utc",
			issue:   hookedBead("b", 1, "2026-09-22T16:44:54+02:00"),
			wantOK:  true,
			wantRFC: "2026-09-22T14:44:54Z",
		},
		{
			name:    "hand-edited bare timestamp is tolerated",
			issue:   hookedBead("c", 1, "2026-09-22 14:44:54"),
			wantOK:  true,
			wantRFC: "2026-09-22T14:44:54Z",
		},
		{name: "unparseable stamp", issue: hookedBead("d", 1, "whenever"), wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := AttachedAtTime(tt.issue)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if formatted := got.Format("2006-01-02T15:04:05Z"); formatted != tt.wantRFC {
				t.Errorf("time = %s, want %s", formatted, tt.wantRFC)
			}
		})
	}
}
