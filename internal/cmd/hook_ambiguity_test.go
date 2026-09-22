package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

func testHooked(id string, priority int, attachedAt string) *beads.Issue {
	desc := "work description"
	if attachedAt != "" {
		desc = "attached_molecule: wisp-" + id + "\nattached_at: " + attachedAt + "\n\n" + desc
	}
	return &beads.Issue{
		ID:          id,
		Title:       "work item " + id,
		Status:      beads.StatusHooked,
		Priority:    priority,
		Description: desc,
	}
}

func TestRenderForeignHookWarning(t *testing.T) {
	slot := "sandboxx-backend/jukka"
	slung := "sbx-gastown-8x56sm"

	t.Run("clean slot carrying only the slung bead is silent", func(t *testing.T) {
		got := renderForeignHookWarning(slot, slung, []*beads.Issue{
			testHooked(slung, 1, "2026-09-22T14:44:54Z"),
		}, nil)
		if got != "" {
			t.Errorf("want no warning for a clean slot, got:\n%s", got)
		}
	})

	t.Run("empty slot is silent", func(t *testing.T) {
		if got := renderForeignHookWarning(slot, slung, nil, nil); got != "" {
			t.Errorf("want no warning for an empty slot, got:\n%s", got)
		}
	})

	t.Run("foreign hook names both beads and the remedy", func(t *testing.T) {
		got := renderForeignHookWarning(slot, slung, []*beads.Issue{
			testHooked("sbx-gastown-fvl9sz", 1, "2026-09-17T10:02:11Z"),
			testHooked(slung, 1, "2026-09-22T14:44:54Z"),
		}, nil)

		for _, want := range []string{
			"ARMED HOOK",
			slot,
			slung,
			"sbx-gastown-fvl9sz",
			"gt unsling sbx-gastown-fvl9sz",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("warning missing %q, got:\n%s", want, got)
			}
		}
	})

	t.Run("every foreign hook gets its own unsling line", func(t *testing.T) {
		got := renderForeignHookWarning(slot, slung, []*beads.Issue{
			testHooked("stale-a", 0, "2026-09-01T00:00:00Z"),
			testHooked("stale-b", 2, ""),
			testHooked(slung, 1, "2026-09-22T14:44:54Z"),
		}, nil)

		if !strings.Contains(got, "2 other hooked bead(s)") {
			t.Errorf("warning should count both foreign hooks, got:\n%s", got)
		}
		for _, want := range []string{"gt unsling stale-a", "gt unsling stale-b"} {
			if !strings.Contains(got, want) {
				t.Errorf("warning missing %q, got:\n%s", want, got)
			}
		}
		// An unstamped foreign hook must still be described, not skipped.
		if !strings.Contains(got, "no attached_at") {
			t.Errorf("unstamped foreign hook should be flagged as such, got:\n%s", got)
		}
	})

	// The requirement that keeps this guard honest: a check that could not run
	// must never look like a clean slot.
	t.Run("a failed query warns instead of going silent", func(t *testing.T) {
		got := renderForeignHookWarning(slot, slung, nil, errors.New("dial tcp 127.0.0.1:3307: connection refused"))
		if got == "" {
			t.Fatal("an unrunnable check must not be silent")
		}
		for _, want := range []string{
			"DID NOT RUN",
			slot,
			"connection refused",
			"bd list --status=hooked --assignee " + slot,
		} {
			if !strings.Contains(got, want) {
				t.Errorf("warning missing %q, got:\n%s", want, got)
			}
		}
	})

	t.Run("a failed query wins over a list that looks clean", func(t *testing.T) {
		got := renderForeignHookWarning(slot, slung, []*beads.Issue{}, errors.New("boom"))
		if !strings.Contains(got, "DID NOT RUN") {
			t.Errorf("listErr must take precedence over an empty result, got:\n%s", got)
		}
	})
}

func TestRenderHookAmbiguityWarning(t *testing.T) {
	agent := "gastown-prime/polecats/portia"

	t.Run("single hook is silent", func(t *testing.T) {
		got := renderHookAmbiguityWarning(agent, testHooked("only", 1, "2026-09-22T14:44:54Z"), nil)
		if got != "" {
			t.Errorf("want no warning for an unambiguous hook, got:\n%s", got)
		}
	})

	t.Run("no hook at all is silent", func(t *testing.T) {
		if got := renderHookAmbiguityWarning(agent, nil, nil); got != "" {
			t.Errorf("want no warning when nothing is hooked, got:\n%s", got)
		}
	})

	t.Run("names the winner, every loser, and how the pick was made", func(t *testing.T) {
		winner := testHooked("fresh-p1", 1, "2026-09-22T14:44:54Z")
		losers := []*beads.Issue{
			testHooked("stale-p0", 0, "2026-09-17T10:02:11Z"),
			testHooked("older-p2", 2, "2026-09-01T00:00:00Z"),
		}
		got := renderHookAmbiguityWarning(agent, winner, losers)

		for _, want := range []string{
			"AMBIGUOUS HOOK",
			"3 beads are hooked",
			agent,
			"RUNNING: fresh-p1",
			"IGNORED: stale-p0",
			"IGNORED: older-p2",
			"most recent attached_at",
			"gt unsling",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("warning missing %q, got:\n%s", want, got)
			}
		}
	})

	t.Run("an unstamped pick says the pick may be wrong", func(t *testing.T) {
		got := renderHookAmbiguityWarning(agent,
			testHooked("first", 0, ""),
			[]*beads.Issue{testHooked("second", 1, "")})

		if !strings.Contains(got, "WRONG bead") {
			t.Errorf("an unarbitrated pick must be flagged as unreliable, got:\n%s", got)
		}
		if strings.Contains(got, "Picked by most recent attached_at") {
			t.Errorf("must not claim an attached_at pick when there is no stamp, got:\n%s", got)
		}
	})
}

func TestForeignHooks(t *testing.T) {
	slung := "slung-bead"
	got := foreignHooks([]*beads.Issue{
		testHooked(slung, 1, ""),
		nil,
		testHooked("other", 1, ""),
	}, slung)

	if len(got) != 1 || got[0].ID != "other" {
		t.Fatalf("foreignHooks = %v, want exactly [other]", got)
	}
}

func TestTruncateDisplay(t *testing.T) {
	tests := []struct {
		name   string
		in     string
		maxLen int
		want   string
	}{
		{name: "short string untouched", in: "abc", maxLen: 10, want: "abc"},
		{name: "exact length untouched", in: "abcde", maxLen: 5, want: "abcde"},
		{name: "truncates with ellipsis", in: "abcdefgh", maxLen: 5, want: "abcd…"},
		{name: "non-positive max is a no-op", in: "abcdefgh", maxLen: 0, want: "abcdefgh"},
		{name: "maxLen 1 is just the ellipsis", in: "abcdefgh", maxLen: 1, want: "…"},
		// Byte slicing would split the glyph and emit mojibake; the statusline
		// badge puts a multi-byte rune at the front of every truncated line.
		{name: "rune aware at a multibyte boundary", in: "⚠2 sbx-gastown-abc", maxLen: 4, want: "⚠2 …"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncateDisplay(tt.in, tt.maxLen); got != tt.want {
				t.Errorf("truncateDisplay(%q, %d) = %q, want %q", tt.in, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestDescribeBead(t *testing.T) {
	t.Run("stamped bead reports its attach time", func(t *testing.T) {
		got := describeBead(testHooked("abc", 1, "2026-09-22T14:44:54Z"))
		for _, want := range []string{"abc", "[P1]", "attached 2026-09-22T14:44:54Z"} {
			if !strings.Contains(got, want) {
				t.Errorf("describeBead missing %q, got %q", want, got)
			}
		}
	})

	t.Run("unstamped bead says so", func(t *testing.T) {
		if got := describeBead(testHooked("abc", 1, "")); !strings.Contains(got, "no attached_at") {
			t.Errorf("describeBead should flag a missing stamp, got %q", got)
		}
	})

	t.Run("nil bead does not panic", func(t *testing.T) {
		if got := describeBead(nil); got == "" {
			t.Error("describeBead(nil) should render a placeholder")
		}
	})
}
