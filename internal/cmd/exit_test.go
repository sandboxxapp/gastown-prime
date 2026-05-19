package cmd

import (
	"strings"
	"testing"
)

func TestExtractAttachedMolecule(t *testing.T) {
	tests := []struct {
		name        string
		description string
		want        string
	}{
		{
			name:        "present",
			description: "attached_molecule: sbx-gastown-wisp-0un6m\nattached_formula: mol-polecat-work\n",
			want:        "sbx-gastown-wisp-0un6m",
		},
		{
			name:        "empty description",
			description: "",
			want:        "",
		},
		{
			name:        "no molecule field",
			description: "Some random description\nwith multiple lines",
			want:        "",
		},
		{
			name:        "trailing whitespace",
			description: "attached_molecule: sbx-gastown-wisp-abc  \n",
			want:        "sbx-gastown-wisp-abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractAttachedMolecule(tt.description)
			if got != tt.want {
				t.Errorf("extractAttachedMolecule() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildDomainNote_WispNotes(t *testing.T) {
	tests := []struct {
		name      string
		notes     string
		design    string
		exitNotes string
		wisps     []wispNote
		wantSubs  []string // substrings that must appear
		notSubs   []string // substrings that must NOT appear
	}{
		{
			name:      "with wisp notes",
			notes:     "Root bead notes",
			design:    "",
			exitNotes: "fallback",
			wisps: []wispNote{
				{ID: "wisp-1", Title: "step 1", Notes: "Found a bug in auth"},
				{ID: "wisp-2", Title: "step 2", Notes: "Fixed the migration"},
			},
			wantSubs: []string{
				"## Wisp Notes",
				"### wisp-1: step 1",
				"Found a bug in auth",
				"### wisp-2: step 2",
				"Fixed the migration",
				"## Notes",
				"Root bead notes",
			},
			notSubs: []string{"fallback"},
		},
		{
			name:      "no wisps falls back to exit notes",
			notes:     "",
			design:    "",
			exitNotes: "Polecat exit: branch pushed",
			wisps:     nil,
			wantSubs:  []string{"Polecat exit: branch pushed"},
			notSubs:   []string{"## Wisp Notes"},
		},
		{
			name:      "wisps but no root notes",
			notes:     "",
			design:    "",
			exitNotes: "fallback",
			wisps: []wispNote{
				{ID: "wisp-x", Title: "only step", Notes: "Important finding"},
			},
			wantSubs: []string{"## Wisp Notes", "Important finding"},
			notSubs:  []string{"fallback"}, // wisps present → no fallback
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildDomainNote("test-issue", "test-branch", tt.notes, tt.design, tt.exitNotes, tt.wisps)
			for _, sub := range tt.wantSubs {
				if !strings.Contains(got, sub) {
					t.Errorf("buildDomainNote() missing expected substring %q\ngot:\n%s", sub, got)
				}
			}
			for _, sub := range tt.notSubs {
				if strings.Contains(got, sub) {
					t.Errorf("buildDomainNote() should not contain %q\ngot:\n%s", sub, got)
				}
			}
		})
	}
}

func TestBuildExitNotesArgs_UsesAppend(t *testing.T) {
	// Regression: gt exit previously used `bd update --notes <X>` which
	// OVERWRITES, clobbering rich design/findings notes the polecat persisted
	// during work (sbx-gastown-9tf8). Must use --append-notes so the
	// boilerplate epilogue is added to existing content rather than replacing it.
	args := buildExitNotesArgs("sbx-gastown-abc", "Polecat exit: branch X pushed.")

	want := []string{"update", "sbx-gastown-abc", "--append-notes", "Polecat exit: branch X pushed."}
	if len(args) != len(want) {
		t.Fatalf("args length = %d, want %d (args=%v)", len(args), len(want), args)
	}
	for i, w := range want {
		if args[i] != w {
			t.Errorf("args[%d] = %q, want %q", i, args[i], w)
		}
	}

	for _, a := range args {
		if a == "--notes" {
			t.Errorf("buildExitNotesArgs must NOT use --notes (clobbers existing); args=%v", args)
		}
	}
}

func TestBuildDomainNote_BackwardCompat(t *testing.T) {
	// Verify backward compatibility: calling with nil wisps produces same
	// output as the original function (no wisp section).
	got := buildDomainNote("issue-1", "main", "some notes", "some design", "exit msg", nil)

	if strings.Contains(got, "## Wisp Notes") {
		t.Error("nil wisps should not produce a Wisp Notes section")
	}
	if !strings.Contains(got, "## Notes") {
		t.Error("should contain Notes section")
	}
	if !strings.Contains(got, "## Design") {
		t.Error("should contain Design section")
	}
	// With notes+design present, exit notes should not appear as fallback
	if strings.Contains(got, "exit msg") {
		t.Error("exit notes should not appear when bead notes/design are present")
	}
}
