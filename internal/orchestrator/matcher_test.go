package orchestrator

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatchOutcome(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		want    Outcome
		wantCat OutcomeCategory
	}{
		// Test pass patterns
		{
			name:    "go test pass",
			output:  "ok  \tgithub.com/foo/bar\t0.123s\n",
			want:    OutcomeTestPass,
			wantCat: CategorySuccess,
		},
		{
			name:    "bats test pass",
			output:  "1..5\nok 1 test one\nok 2 test two\n# All 5 tests passed\n",
			want:    OutcomeTestPass,
			wantCat: CategorySuccess,
		},
		{
			name:    "pytest pass",
			output:  "====== 12 passed in 3.45s ======\n",
			want:    OutcomeTestPass,
			wantCat: CategorySuccess,
		},

		// Test fail patterns
		{
			name:    "go test fail",
			output:  "FAIL\tgithub.com/foo/bar\t0.456s\n",
			want:    OutcomeTestFail,
			wantCat: CategoryFailure,
		},
		{
			name:    "bats test fail",
			output:  "not ok 3 test three\n",
			want:    OutcomeTestFail,
			wantCat: CategoryFailure,
		},
		{
			name:    "pytest fail",
			output:  "FAILED tests/test_foo.py::test_bar\n",
			want:    OutcomeTestFail,
			wantCat: CategoryFailure,
		},

		// Build patterns
		{
			name:    "go build ok",
			output:  "go: no errors\n",
			want:    OutcomeBuildOK,
			wantCat: CategorySuccess,
		},
		{
			name:    "go build success via exit",
			output:  "go build -o gt ./cmd/gt\n",
			want:    OutcomeBuildOK,
			wantCat: CategorySuccess,
		},
		{
			name:    "build error",
			output:  "./main.go:15:2: undefined: foo\n",
			want:    OutcomeBuildFail,
			wantCat: CategoryFailure,
		},
		{
			name:    "build error compile",
			output:  "# github.com/foo/bar\ncompilation error\n",
			want:    OutcomeBuildFail,
			wantCat: CategoryFailure,
		},

		// Git push
		{
			name:    "git push success",
			output:  "To github.com:sandboxxapp/repo.git\n * [new branch]      feature -> feature\n",
			want:    OutcomeGitPush,
			wantCat: CategorySuccess,
		},
		{
			name:    "git push with branch info",
			output:  "Branch 'feature' set up to track remote branch 'feature' from 'origin'.\n",
			want:    OutcomeGitPush,
			wantCat: CategorySuccess,
		},

		// PR created
		{
			name:    "gh pr create",
			output:  "Creating pull request for feature into main in sandboxxapp/repo\nhttps://github.com/sandboxxapp/repo/pull/42\n",
			want:    OutcomePRCreated,
			wantCat: CategorySuccess,
		},

		// Commit
		{
			name:    "git commit",
			output:  "[feature abc1234] Fix the thing\n 2 files changed, 10 insertions(+), 3 deletions(-)\n",
			want:    OutcomeCommit,
			wantCat: CategorySuccess,
		},

		// Ambiguous
		{
			name:    "unrecognized output",
			output:  "doing some work...\nprocessing...\ndone.\n",
			want:    OutcomeAmbiguous,
			wantCat: CategoryAmbiguous,
		},
		{
			name:    "empty output",
			output:  "",
			want:    OutcomeAmbiguous,
			wantCat: CategoryAmbiguous,
		},
	}

	m := NewMatcher()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := m.Match(tt.output)
			assert.Equal(t, tt.want, result.Outcome, "outcome mismatch")
			assert.Equal(t, tt.wantCat, result.Category, "category mismatch")
		})
	}
}

func TestMatcherPriorityOrder(t *testing.T) {
	// Test fail should take priority over test pass if both appear
	m := NewMatcher()
	output := "ok  \tgithub.com/foo/bar\t0.1s\nFAIL\tgithub.com/foo/baz\t0.2s\n"
	result := m.Match(output)
	// Failure patterns are checked first, so FAIL wins
	assert.Equal(t, OutcomeTestFail, result.Outcome)
}
