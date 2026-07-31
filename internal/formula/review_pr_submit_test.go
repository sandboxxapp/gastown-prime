package formula

import (
	"strings"
	"testing"
)

// findReviewPRSubmitStep parses mol-polecat-review-pr and returns its
// submit-review step, failing the test if it cannot be found.
func findReviewPRSubmitStep(t *testing.T) *Step {
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
		if f.Steps[i].ID == "submit-review" {
			return &f.Steps[i]
		}
	}
	t.Fatal("submit-review step not found in mol-polecat-review-pr")
	return nil
}

// TestReviewPRSubmitStep_PostsContentToPR verifies that the mol-polecat-review-pr
// submit-review step instructs the reviewer to post the full review CONTENT to the
// PR as a comment via `gh pr comment` — not just a one-line approval stamp.
func TestReviewPRSubmitStep_PostsContentToPR(t *testing.T) {
	desc := findReviewPRSubmitStep(t).Description

	if !strings.Contains(desc, "gh pr comment {{pr_url}}") {
		t.Error("submit-review step must post the review via 'gh pr comment {{pr_url}}'")
	}

	// The comment must carry substance, not a stamp. Check for the required
	// content sections called out in the operator directive.
	for _, want := range []string{
		"Verdict",
		"Rationale",
		"Operator decisions", // maintainer sign-off items
		"Tests",              // test assessment
		"Follow-up",          // follow-up beads
	} {
		if !strings.Contains(desc, want) {
			t.Errorf("submit-review step should require %q content in the PR comment", want)
		}
	}
}

// TestReviewPRSubmitStep_HeadersAsGastownReview verifies the review comment is
// headered so it reads as a relayed Gastown review with the verdict and tracking
// bead.
func TestReviewPRSubmitStep_HeadersAsGastownReview(t *testing.T) {
	desc := findReviewPRSubmitStep(t).Description

	if !strings.Contains(desc, "🤖 Gastown review") {
		t.Error("submit-review step must header the comment as '🤖 Gastown review'")
	}
	if !strings.Contains(desc, "tracking {{issue}}") {
		t.Error("submit-review header must reference the tracking bead 'tracking {{issue}}'")
	}
}

// TestReviewPRSubmitStep_IdentityFallback verifies the step prefers the KbotSB bot
// identity but falls back to posting the content under the operator identity when
// the bot is unavailable — and never silently skips the PR post.
func TestReviewPRSubmitStep_IdentityFallback(t *testing.T) {
	desc := findReviewPRSubmitStep(t).Description

	if !strings.Contains(desc, "KbotSB") {
		t.Error("submit-review step should reference the KbotSB bot identity as preferred")
	}
	if !strings.Contains(desc, "gh auth status") {
		t.Error("submit-review step should detect identity via 'gh auth status'")
	}
	if !strings.Contains(desc, "fallback") && !strings.Contains(desc, "Fallback") {
		t.Error("submit-review step should describe an operator-identity fallback")
	}
	// The PR post must never be silently skipped.
	if !strings.Contains(desc, "NEVER silently skip") {
		t.Error("submit-review step must state the PR post is NEVER silently skipped")
	}
}

// TestReviewPRSubmitStep_NoOperatorSelfApprove verifies the step forbids posting a
// formal `gh pr review --approve` stamp under the operator identity (GitHub rejects
// self-approval), reserving formal review STATE for the genuine bot path.
func TestReviewPRSubmitStep_NoOperatorSelfApprove(t *testing.T) {
	desc := findReviewPRSubmitStep(t).Description

	// It must explicitly warn against the operator self-approval stamp.
	lower := strings.ToLower(desc)
	if !strings.Contains(lower, "self-approval") && !strings.Contains(lower, "operator\nidentity") && !strings.Contains(lower, "operator identity") {
		t.Error("submit-review step should warn that operator-identity self-approval is rejected")
	}
	// Formal review state must be gated to the bot path.
	if !strings.Contains(desc, "bot path") {
		t.Error("submit-review step should reserve formal review state for the bot path")
	}
}

// TestReviewPRSubmitStep_RecordsOnBead verifies the verdict is always recorded on
// the tracking bead as a backstop, in addition to the PR comment.
func TestReviewPRSubmitStep_RecordsOnBead(t *testing.T) {
	desc := findReviewPRSubmitStep(t).Description

	if !strings.Contains(desc, "bd update {{issue}}") {
		t.Error("submit-review step must record the verdict on the tracking bead via 'bd update {{issue}}'")
	}
}
