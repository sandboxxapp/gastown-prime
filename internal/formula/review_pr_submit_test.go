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

// TestReviewPRSubmitStep_IdentityFallback verifies the step posts as KbotSB — the
// credential on each gh call, since shell state does not survive a tool call — and
// falls back to ambient auth (disclosed in the body) when the login does not come
// back KbotSB. The PR post is never silently skipped (sbx-gastown-xs1uq7: operator
// re-scope KEEPS "post as KbotSB"; only the stale tzpay wording goes).
func TestReviewPRSubmitStep_IdentityFallback(t *testing.T) {
	desc := findReviewPRSubmitStep(t).Description

	if !strings.Contains(desc, "Post the review as KbotSB") {
		t.Error("submit-review step should direct the reviewer to post as KbotSB")
	}
	if !strings.Contains(desc, "mayor/credentials/kbotsb.token") {
		t.Error("submit-review step should name the KbotSB credential file")
	}
	if !strings.Contains(desc, `GH_TOKEN="$(cat "$KB")" gh pr review`) {
		t.Error("submit-review step should put the KbotSB credential on each gh pr review call")
	}
	if !strings.Contains(desc, "ambient auth") {
		t.Error("submit-review step should describe the ambient-auth fallback when ME is not KbotSB")
	}
	if !strings.Contains(desc, "DENY_BOT_LOGINS") {
		t.Error("submit-review step should disclose that a KbotSB review is not auto-picked-up by the foreman dispatch")
	}
	// The PR post must never be silently skipped.
	if !strings.Contains(desc, "NEVER silently skip") {
		t.Error("submit-review step must state the PR post is NEVER silently skipped")
	}
	// The stale claim that KbotSB authenticates as the operator (sbx-gastown-tzpay)
	// is false since 2026-09-24 and must not come back.
	for _, stale := range []string{"tzpay", "authenticates as the\nOPERATOR", "KbotSB is down", "until the KbotSB fix lands"} {
		if strings.Contains(desc, stale) {
			t.Errorf("submit-review step still carries stale KbotSB wording %q", stale)
		}
	}
}

// TestReviewPRSubmitStep_NoOperatorSelfApprove verifies the step forbids a formal
// `gh pr review --approve` when the reviewer authored the PR (GitHub rejects
// self-approval), gating formal review STATE on ME vs PR_AUTHOR.
func TestReviewPRSubmitStep_NoOperatorSelfApprove(t *testing.T) {
	desc := findReviewPRSubmitStep(t).Description

	if !strings.Contains(strings.ToLower(desc), "self-approval") {
		t.Error("submit-review step should warn that self-approval is rejected")
	}
	if !strings.Contains(desc, "gated on authorship") || !strings.Contains(desc, "PR_AUTHOR") {
		t.Error("submit-review step should gate formal review state on ME vs PR_AUTHOR")
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
