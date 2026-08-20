package formula

import (
	"strings"
	"testing"
)

// TestPolecatWorkSubmitStep_UsesGHPRCreate verifies that the mol-polecat-work
// submit-and-exit step instructs polecats to create a GitHub PR via gh CLI,
// not submit to gt's merge queue.
func TestPolecatWorkSubmitStep_UsesGHPRCreate(t *testing.T) {
	content, err := GetEmbeddedFormulaContent("mol-polecat-work")
	if err != nil {
		t.Fatalf("GetEmbeddedFormulaContent: %v", err)
	}

	f, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// Find the submit-and-exit step
	var submitStep *Step
	for i := range f.Steps {
		if f.Steps[i].ID == "submit-and-exit" {
			submitStep = &f.Steps[i]
			break
		}
	}
	if submitStep == nil {
		t.Fatal("submit-and-exit step not found in mol-polecat-work")
	}

	desc := submitStep.Description

	// Must instruct polecats to create a PR via gh CLI
	if !strings.Contains(desc, "gh pr create") {
		t.Error("submit-and-exit step should contain 'gh pr create' instruction")
	}

	// Must NOT reference gt done for code task submission (MQ model)
	// gt done --status DEFERRED for report-only tasks is acceptable
	lines := strings.Split(desc, "\n")
	for _, line := range lines {
		if strings.Contains(line, "gt done") && !strings.Contains(line, "DEFERRED") {
			t.Errorf("submit-and-exit step should not reference 'gt done' for code tasks, found: %s", strings.TrimSpace(line))
		}
	}

	// Must NOT reference merge queue or MR bead for code submission
	if strings.Contains(desc, "MR bead") {
		t.Error("submit-and-exit step should not reference 'MR bead'")
	}
	if strings.Contains(desc, "merge queue") {
		t.Error("submit-and-exit step should not reference 'merge queue'")
	}
}

// TestPolecatWorkSubmitStep_ExitsSession verifies that the mol-polecat-work
// submit-and-exit step instructs polecats to terminate their session after
// creating a PR. If the session stays alive, agent_alive stays true and the
// reaper cannot clean up the polecat.
//
// The terminating command is `gt exit`. This test originally required a bare
// `/exit` (93d011fb, 2026-04-13) because `gt exit` did not kill its own tmux
// session back then; PR #45 (42173d99, 2026-04-22, sbx-gastown-xpuv) gave
// `gt exit` a detached self-terminate, which made the extra `/exit` redundant.
func TestPolecatWorkSubmitStep_ExitsSession(t *testing.T) {
	content, err := GetEmbeddedFormulaContent("mol-polecat-work")
	if err != nil {
		t.Fatalf("GetEmbeddedFormulaContent: %v", err)
	}

	f, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	var submitStep *Step
	for i := range f.Steps {
		if f.Steps[i].ID == "submit-and-exit" {
			submitStep = &f.Steps[i]
			break
		}
	}
	if submitStep == nil {
		t.Fatal("submit-and-exit step not found in mol-polecat-work")
	}

	desc := submitStep.Description

	// The step must instruct the polecat to terminate its session so
	// agent_alive goes false and the reaper can clean up.
	if !strings.Contains(desc, "gt exit") {
		t.Error("submit-and-exit step must contain 'gt exit' to terminate the session for reaper cleanup")
	}

	// Exiting is a deliverable, not an epilogue — the step has to say so, or
	// polecats push the PR and then sit idle holding a worktree and a session.
	if !strings.Contains(desc, "DELIVERABLE") {
		t.Error("submit-and-exit step must frame 'gt exit' as a DELIVERABLE, not an optional epilogue")
	}
}
