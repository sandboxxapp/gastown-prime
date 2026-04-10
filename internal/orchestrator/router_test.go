package orchestrator

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRoute(t *testing.T) {
	tests := []struct {
		name       string
		match      MatchResult
		hasNext    bool
		wantAction Action
	}{
		{
			name:       "success with next step advances",
			match:      MatchResult{Outcome: OutcomeTestPass, Category: CategorySuccess},
			hasNext:    true,
			wantAction: ActionAdvance,
		},
		{
			name:       "success without next step completes",
			match:      MatchResult{Outcome: OutcomeTestPass, Category: CategorySuccess},
			hasNext:    false,
			wantAction: ActionComplete,
		},
		{
			name:       "failure routes to retry",
			match:      MatchResult{Outcome: OutcomeTestFail, Category: CategoryFailure},
			hasNext:    true,
			wantAction: ActionRetry,
		},
		{
			name:       "build failure routes to retry",
			match:      MatchResult{Outcome: OutcomeBuildFail, Category: CategoryFailure},
			hasNext:    true,
			wantAction: ActionRetry,
		},
		{
			name:       "ambiguous routes to triage",
			match:      MatchResult{Outcome: OutcomeAmbiguous, Category: CategoryAmbiguous},
			hasNext:    true,
			wantAction: ActionTriage,
		},
		{
			name:       "pr created with next step advances",
			match:      MatchResult{Outcome: OutcomePRCreated, Category: CategorySuccess},
			hasNext:    true,
			wantAction: ActionAdvance,
		},
		{
			name:       "commit success advances",
			match:      MatchResult{Outcome: OutcomeCommit, Category: CategorySuccess},
			hasNext:    true,
			wantAction: ActionAdvance,
		},
	}

	r := NewRouter()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := r.Route(tt.match, tt.hasNext)
			assert.Equal(t, tt.wantAction, decision.Action)
		})
	}
}

func TestRouteRetryExhaustion(t *testing.T) {
	r := NewRouter()
	r.MaxRetries = 2

	match := MatchResult{Outcome: OutcomeTestFail, Category: CategoryFailure}

	// First two failures retry
	d1 := r.Route(match, true)
	assert.Equal(t, ActionRetry, d1.Action)

	d2 := r.RouteWithAttempt(match, true, 2)
	assert.Equal(t, ActionRetry, d2.Action)

	// Third failure escalates
	d3 := r.RouteWithAttempt(match, true, 3)
	assert.Equal(t, ActionEscalate, d3.Action)
}

func TestRouteTriageEscalation(t *testing.T) {
	r := NewRouter()

	// Triage result with escalation flag set
	triageResult := MatchResult{
		Outcome:  OutcomeAmbiguous,
		Category: CategoryAmbiguous,
	}

	// First time: triage
	d := r.Route(triageResult, true)
	assert.Equal(t, ActionTriage, d.Action)

	// After triage fails (still ambiguous): escalate to mayor
	d2 := r.RouteAfterTriage(triageResult, true)
	assert.Equal(t, ActionEscalate, d2.Action)
}
