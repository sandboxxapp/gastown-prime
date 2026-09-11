package formula

import (
	"strings"
	"testing"
)

// findReviewPRStep parses mol-polecat-review-pr and returns the named step,
// failing the test if it cannot be found.
func findReviewPRStep(t *testing.T, id string) *Step {
	t.Helper()

	content, err := GetEmbeddedFormulaContent("mol-polecat-review-pr")
	if err != nil {
		t.Fatalf("GetEmbeddedFormulaContent: %v", err)
	}

	f, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	for i := range f.Steps {
		if f.Steps[i].ID == id {
			return &f.Steps[i]
		}
	}
	t.Fatalf("step %q not found in mol-polecat-review-pr", id)
	return nil
}

// TestReviewPRDecision_MinorIssuesApprove guards the operator ruling of
// 2026-09-11 (sbx-gastown-84iet2): "minor issues, easily fixed" must map to
// APPROVE plus a followup bead, NOT to REQUEST_CHANGES.
//
// The old matrix row — "| Minor issues, easily fixed | REQUEST_CHANGES ... |" —
// told a reviewer who found a nit to block a mergeable PR. That is what drove
// reviewers to invent the unusable "APPROVE WITH CHANGES" middle state.
func TestReviewPRDecision_MinorIssuesApprove(t *testing.T) {
	desc := findReviewPRStep(t, "make-decision").Description

	// The regression itself: the old row must not come back.
	if strings.Contains(desc, "| Minor issues, easily fixed | REQUEST_CHANGES") {
		t.Error("make-decision maps minor issues to REQUEST_CHANGES again — the 2026-09-11 ruling maps them to APPROVE + followup bead")
	}

	// And the replacement row must be present and reachable.
	lower := strings.ToLower(desc)
	if !strings.Contains(lower, "minor issues") {
		t.Error("make-decision matrix should still have a row for minor issues")
	}
	if !strings.Contains(desc, "followup bead") {
		t.Error("make-decision should route non-blocking findings to a followup bead")
	}
}

// TestReviewPRDecision_BansApproveWithChanges verifies the invented middle
// verdict is named and explicitly forbidden, so a reviewer who reaches for it
// is told what to write instead.
func TestReviewPRDecision_BansApproveWithChanges(t *testing.T) {
	desc := findReviewPRStep(t, "make-decision").Description

	if !strings.Contains(desc, "APPROVE WITH CHANGES") {
		t.Error("make-decision must name 'APPROVE WITH CHANGES' in order to forbid it")
	}
	if !strings.Contains(desc, "IS NOT A VERDICT") {
		t.Error("make-decision must state that 'APPROVE WITH CHANGES' is not a verdict")
	}
}

// TestReviewPRDecision_RequestChangesTriggers verifies REQUEST_CHANGES is
// reserved to the four "must change before merge" conditions from the operator
// ruling, rather than being the default for any imperfection.
func TestReviewPRDecision_RequestChangesTriggers(t *testing.T) {
	desc := findReviewPRStep(t, "make-decision").Description
	lower := strings.ToLower(desc)

	for _, want := range []string{
		"defect that reaches users",
		"false claim",
		"let a real regression through undetected",
		"does not reproduce",
	} {
		if !strings.Contains(lower, strings.ToLower(want)) {
			t.Errorf("make-decision should name the REQUEST_CHANGES trigger %q", want)
		}
	}

	if !strings.Contains(desc, "REQUEST_CHANGES is reserved") {
		t.Error("make-decision should state that REQUEST_CHANGES is reserved to those triggers")
	}
}

// TestReviewPRDecision_FollowupsFiledBeforeVerdict verifies the followup beads
// are created in make-decision. The verdict line must name their ids, so they
// cannot be deferred to the file-followups step that runs after submit-review.
func TestReviewPRDecision_FollowupsFiledBeforeVerdict(t *testing.T) {
	desc := findReviewPRStep(t, "make-decision").Description

	if !strings.Contains(desc, "bd create") {
		t.Error("make-decision must instruct the reviewer to file followup beads with 'bd create' before the verdict is posted")
	}
}

// TestReviewPRSubmit_VerdictOnFirstLine verifies the second half of the ruling:
// the verdict is on the FIRST LINE of the review comment and the followup bead
// ids are named in it. The mayor decides merge-or-foreman off that line.
func TestReviewPRSubmit_VerdictOnFirstLine(t *testing.T) {
	desc := findReviewPRStep(t, "submit-review").Description

	if !strings.Contains(desc, "FIRST LINE") {
		t.Error("submit-review must require the verdict on the FIRST LINE of the review comment")
	}
	if !strings.Contains(desc, "followups:") {
		t.Error("submit-review must require a 'followups:' field carrying the bead ids")
	}
	// The canonical first-line shape.
	if !strings.Contains(desc, "🤖 Gastown review — <VERDICT> (tracking {{issue}}) — followups:") {
		t.Error("submit-review must show the canonical first-line shape with the followups field")
	}
	// The closed verdict set, and nothing else.
	for _, want := range []string{"APPROVE", "REQUEST_CHANGES", "NEEDS_DISCUSSION"} {
		if !strings.Contains(desc, want) {
			t.Errorf("submit-review should name the verdict token %q", want)
		}
	}
	if !strings.Contains(desc, "Never `APPROVE WITH CHANGES`") {
		t.Error("submit-review must forbid 'APPROVE WITH CHANGES' on the verdict line")
	}
}
