package formula

import (
	"io/fs"
	"regexp"
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

// reviewPostRe matches an instruction that SUBMITS a formal review, as opposed
// to prose that merely names `gh pr review`.
var reviewPostRe = regexp.MustCompile(`gh pr review \S+ +--(approve|request-changes|comment)\b`)

// TestReviewPostingSteps_NeverInstructBothPosts guards sbx-gastown-2rlv97: every
// review used to land TWICE (a `gh pr comment`, then a formal `gh pr review` with
// the same body), doubling the heaviest part of each PR page. Every embedded step
// that submits a formal review must make it ONE post: no single command block
// runs both, and the step says "Never run both".
func TestReviewPostingSteps_NeverInstructBothPosts(t *testing.T) {
	entries, err := fs.ReadDir(formulasFS, "formulas")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	posting := 0
	for _, e := range entries {
		content, err := formulasFS.ReadFile("formulas/" + e.Name())
		if err != nil {
			t.Fatalf("ReadFile %s: %v", e.Name(), err)
		}
		f, err := Parse(content)
		if err != nil {
			// Some embedded formulas (aspects) do not parse standalone; they must
			// not hide a review post from this guard.
			if reviewPostRe.Match(content) {
				t.Errorf("%s submits a formal review but does not parse (%v), so its steps cannot be checked", e.Name(), err)
			}
			continue
		}
		for _, s := range f.Steps {
			if !reviewPostRe.MatchString(s.Description) {
				continue
			}
			posting++
			where := e.Name() + " step " + s.ID
			if !strings.Contains(s.Description, "Never run both") {
				t.Errorf("%s submits a formal review but does not state the one-post rule (\"Never run both\")", where)
			}
			for i, block := range strings.Split(s.Description, "```")[1:] {
				if i%2 != 0 { // odd segments sit between fences, not inside them
					continue
				}
				if reviewPostRe.MatchString(block) && strings.Contains(block, "gh pr comment") {
					t.Errorf("%s has a command block that posts BOTH a formal review and a comment:\n%s", where, block)
				}
			}
		}
	}
	if posting == 0 {
		t.Fatal("no embedded step submits a formal review — the review-pr submit step moved; update this test")
	}
}

// TestReviewPRSubmitStep_PostsOnce verifies the review-pr submit step makes the
// formal review the one post when the reviewer did not author the PR, the comment
// the one post when it did, and that the old double-post wording is gone.
func TestReviewPRSubmitStep_PostsOnce(t *testing.T) {
	desc := findReviewPRSubmitStep(t).Description

	for _, want := range []string{
		"ONE post, never two",
		"`ME != PR_AUTHOR`",
		"the\nFORMAL review is your one post",
		"`ME == PR_AUTHOR`",
		"the COMMENT is your one post",
		"--json reviews,comments", // the landed-once check counts both surfaces
		"`now` must be `N0+1`",
	} {
		if !strings.Contains(desc, want) {
			t.Errorf("submit-review step should contain %q", want)
		}
	}
	for _, stale := range []string{
		"Post the content comment (ALWAYS)",
		"optionally add `gh pr review --comment` carrying the same",
		"The full review CONTENT is a comment on the PR",
	} {
		if strings.Contains(desc, stale) {
			t.Errorf("submit-review step still carries the double-post wording %q", stale)
		}
	}
}

// TestReviewPRSubmitStep_CountIsPerRound guards sbx-gastown-5l17vk: {{issue}} is
// the source bead and every re-review round reuses it, so a count matching
// "tracking {{issue}}" includes earlier rounds' posts. An absolute "must print 1"
// reads a correct round-2 post as a duplicate and lets a refused round-2 review
// go unposted. The check must be relative to a baseline taken before any post.
func TestReviewPRSubmitStep_CountIsPerRound(t *testing.T) {
	desc := findReviewPRSubmitStep(t).Description

	baseline := strings.Index(desc, "> /tmp/gastown-review-{{issue}}.n0")
	if baseline < 0 {
		t.Fatal("submit-review step must write the pre-post baseline count to /tmp/gastown-review-{{issue}}.n0")
	}
	if post := reviewPostRe.FindStringIndex(desc); post == nil || post[0] < baseline {
		t.Error("the baseline count must come BEFORE the first formal review post")
	}
	if post := strings.Index(desc, "gh pr comment {{pr_url}}"); post < baseline {
		t.Error("the baseline count must come BEFORE the comment post")
	}
	for _, want := range []string{
		`N0="$(cat /tmp/gastown-review-{{issue}}.n0)"`,
		"never retake it after an attempt",
		"must still show `N0`", // the refused-review fallback compares against the baseline
		"Never compare `now`\nwith `1`",
		"showed `now = N0+1`", // exit criteria
	} {
		if !strings.Contains(desc, want) {
			t.Errorf("submit-review step should contain %q", want)
		}
	}
	for _, stale := range []string{"It must print `1`", "count\ncheck above printed `1`", "`2` or more means a duplicate"} {
		if strings.Contains(desc, stale) {
			t.Errorf("submit-review step still carries the absolute-count wording %q", stale)
		}
	}
}

// TestReviewPRSubmitStep_CollapsesDetail verifies the body template puts the long
// sections inside one collapsed <details> block after the short summary, with
// the blank lines GitHub needs to render markdown inside it.
func TestReviewPRSubmitStep_CollapsesDetail(t *testing.T) {
	desc := findReviewPRSubmitStep(t).Description

	open := strings.Index(desc, "<details>\n<summary>")
	if open < 0 {
		t.Fatal("submit-review body template should open a <details><summary> block")
	}
	if !strings.Contains(desc, "</summary>\n\n") || !strings.Contains(desc, "\n\n</details>") {
		t.Error("<details> block needs a blank line after </summary> and before </details>, or GitHub renders raw markdown")
	}
	tmpl := desc[strings.Index(desc, "cat > /tmp/gastown-review-{{issue}}.md"):]
	open = strings.Index(tmpl, "<details>")
	for _, above := range []string{"## Verdict", "## Rationale", "## Blocking", "## Operator decisions required"} {
		if i := strings.Index(tmpl, above); i < 0 || i > open {
			t.Errorf("template section %q must sit above the collapsed <details> block", above)
		}
	}
	for _, below := range []string{"## Findings", "## Tests", "## Follow-ups"} {
		if i := strings.Index(tmpl, below); i < open {
			t.Errorf("template section %q must sit inside the collapsed <details> block", below)
		}
	}
}
