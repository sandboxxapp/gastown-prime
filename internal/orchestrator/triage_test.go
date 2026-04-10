package orchestrator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTriageClient implements TriageClient for testing.
type mockTriageClient struct {
	result   TriageResult
	err      error
	called   bool
	lastBody string
}

func (m *mockTriageClient) Triage(ctx context.Context, body string, stepID string) (TriageResult, error) {
	m.called = true
	m.lastBody = body
	return m.result, m.err
}

func TestTriageResultActions(t *testing.T) {
	tests := []struct {
		name   string
		result TriageResult
		want   Action
	}{
		{
			name:   "success routes to advance",
			result: TriageResult{Verdict: TriageSuccess},
			want:   ActionAdvance,
		},
		{
			name:   "failure routes to retry",
			result: TriageResult{Verdict: TriageFailure},
			want:   ActionRetry,
		},
		{
			name:   "unsure routes to escalate",
			result: TriageResult{Verdict: TriageUnsure},
			want:   ActionEscalate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.result.ToAction())
		})
	}
}

func TestTriageClientCalled(t *testing.T) {
	mock := &mockTriageClient{
		result: TriageResult{Verdict: TriageSuccess, Reason: "tests passed"},
	}

	result, err := mock.Triage(context.Background(), "some output", "test-step")
	require.NoError(t, err)
	assert.True(t, mock.called)
	assert.Equal(t, "some output", mock.lastBody)
	assert.Equal(t, TriageSuccess, result.Verdict)
}

func TestPromptBuilder(t *testing.T) {
	prompt := BuildTriagePrompt("running tests...\nall passed\n", "test")
	assert.Contains(t, prompt, "STEP_COMPLETE")
	assert.Contains(t, prompt, "running tests")
	assert.Contains(t, prompt, "test")
}
