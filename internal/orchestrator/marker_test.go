package orchestrator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectStepComplete(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		found    bool
		stepID   string
		wantBody string
	}{
		{
			name:   "no marker",
			output: "some random output\nfoo bar\n",
			found:  false,
		},
		{
			name:     "simple marker",
			output:   "running tests...\nall passed\nSTEP_COMPLETE diagnose\n",
			found:    true,
			stepID:   "diagnose",
			wantBody: "running tests...\nall passed",
		},
		{
			name:     "marker with surrounding text",
			output:   "prompt> do the thing\ncompiling...\ndone\nSTEP_COMPLETE implement\nsome trailing text\n",
			found:    true,
			stepID:   "implement",
			wantBody: "prompt> do the thing\ncompiling...\ndone",
		},
		{
			name:     "multiple markers returns last",
			output:   "first\nSTEP_COMPLETE step1\nsecond\nSTEP_COMPLETE step2\n",
			found:    true,
			stepID:   "step2",
			wantBody: "second",
		},
		{
			name:   "partial marker not matched",
			output: "STEP_COMPLET diagnose\n",
			found:  false,
		},
		{
			name:   "marker without step ID",
			output: "STEP_COMPLETE\n",
			found:  false,
		},
		{
			name:     "marker with extra whitespace",
			output:   "output here\nSTEP_COMPLETE   test  \n",
			found:    true,
			stepID:   "test",
			wantBody: "output here",
		},
		{
			name:   "empty output",
			output: "",
			found:  false,
		},
		{
			name:     "marker at very start",
			output:   "STEP_COMPLETE init\n",
			found:    true,
			stepID:   "init",
			wantBody: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectStepComplete(tt.output)
			if !tt.found {
				assert.Nil(t, result)
				return
			}
			require.NotNil(t, result)
			assert.Equal(t, tt.stepID, result.StepID)
			assert.Equal(t, tt.wantBody, result.Body)
		})
	}
}

func TestExtractOutputSinceLastPrompt(t *testing.T) {
	tests := []struct {
		name       string
		output     string
		promptMark string
		want       string
	}{
		{
			name:       "extracts after prompt",
			output:     "old stuff\n❯ do work\ntest output\nbuild ok\n",
			promptMark: "❯",
			want:       "do work\ntest output\nbuild ok",
		},
		{
			name:       "no prompt found returns all",
			output:     "just output\nno prompt here\n",
			promptMark: "❯",
			want:       "just output\nno prompt here",
		},
		{
			name:       "multiple prompts returns after last",
			output:     "old\n❯ first\nresult1\n❯ second\nresult2\n",
			promptMark: "❯",
			want:       "second\nresult2",
		},
		{
			name:       "empty output",
			output:     "",
			promptMark: "❯",
			want:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractOutputSinceLastPrompt(tt.output, tt.promptMark)
			assert.Equal(t, tt.want, got)
		})
	}
}
