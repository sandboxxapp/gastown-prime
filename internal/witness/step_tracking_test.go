package witness

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifyStepTransition(t *testing.T) {
	assert.Equal(t, ProtoStepAdvanced, ClassifyMessage("STEP_ADVANCED Toast diagnose"))
	assert.Equal(t, ProtoStepFailed, ClassifyMessage("STEP_FAILED Toast implement"))
	assert.Equal(t, ProtoStepRetry, ClassifyMessage("STEP_RETRY Toast test"))
	assert.Equal(t, ProtoStepTriaged, ClassifyMessage("STEP_TRIAGED Toast verify"))
	assert.Equal(t, ProtoStepEscalated, ClassifyMessage("STEP_ESCALATED Toast deploy"))
}

func TestParseStepAdvanced(t *testing.T) {
	payload, err := ParseStepAdvanced(
		"STEP_ADVANCED Toast diagnose",
		"FromStep: init\nToStep: diagnose\nOutcome: test_pass\n",
	)
	require.NoError(t, err)
	assert.Equal(t, "Toast", payload.PolecatName)
	assert.Equal(t, "init", payload.FromStep)
	assert.Equal(t, "diagnose", payload.ToStep)
	assert.Equal(t, "test_pass", payload.Outcome)
}

func TestParseStepFailed(t *testing.T) {
	payload, err := ParseStepFailed(
		"STEP_FAILED Toast implement",
		"Step: implement\nAttempt: 2\nError: compilation error\n",
	)
	require.NoError(t, err)
	assert.Equal(t, "Toast", payload.PolecatName)
	assert.Equal(t, "implement", payload.Step)
	assert.Equal(t, 2, payload.Attempt)
	assert.Equal(t, "compilation error", payload.Error)
}

func TestParseStepTriaged(t *testing.T) {
	payload, err := ParseStepTriaged(
		"STEP_TRIAGED Toast verify",
		"Step: verify\nVerdict: success\nReason: output looks like test pass\n",
	)
	require.NoError(t, err)
	assert.Equal(t, "Toast", payload.PolecatName)
	assert.Equal(t, "verify", payload.Step)
	assert.Equal(t, "success", payload.Verdict)
	assert.Equal(t, "output looks like test pass", payload.Reason)
}

func TestStepTracker(t *testing.T) {
	tracker := NewStepTracker()

	// Record step transitions
	tracker.RecordAdvance("Toast", "init", "diagnose")
	tracker.RecordAdvance("Toast", "diagnose", "implement")

	state := tracker.GetState("Toast")
	require.NotNil(t, state)
	assert.Equal(t, "implement", state.CurrentStep)
	assert.Equal(t, 2, state.StepCount)
	assert.False(t, state.LastTransition.IsZero())

	// Record failure
	tracker.RecordFailure("Toast", "implement", 1, "build error")
	state = tracker.GetState("Toast")
	assert.Equal(t, "implement", state.CurrentStep)
	assert.Equal(t, 1, state.RetryCount)

	// Unknown polecat
	assert.Nil(t, tracker.GetState("Unknown"))
}

func TestStepTrackerBudget(t *testing.T) {
	tracker := NewStepTracker()
	tracker.SetBudget("Toast", StepBudget{
		MaxSteps:  10,
		MaxTokens: 100000,
	})

	tracker.RecordAdvance("Toast", "", "step1")
	tracker.AddTokens("Toast", 5000)

	state := tracker.GetState("Toast")
	require.NotNil(t, state)
	assert.Equal(t, 1, state.StepCount)
	assert.Equal(t, int64(5000), state.TokensUsed)
	assert.False(t, tracker.IsOverBudget("Toast"))

	// Exceed budget
	tracker.AddTokens("Toast", 100000)
	assert.True(t, tracker.IsOverBudget("Toast"))
}

func TestStepTrackerLiveness(t *testing.T) {
	tracker := NewStepTracker()
	tracker.RecordAdvance("Toast", "", "step1")

	// Recently active
	assert.False(t, tracker.IsStale("Toast", 5*time.Minute))

	// Force staleness by setting old timestamp
	state := tracker.GetState("Toast")
	state.LastTransition = time.Now().Add(-10 * time.Minute)
	assert.True(t, tracker.IsStale("Toast", 5*time.Minute))
}
