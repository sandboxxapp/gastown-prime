package bus

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStepEvent(t *testing.T) {
	ev := NewStepEvent("gastown", "Toast", "diagnose", StepAdvanced)
	assert.Equal(t, "gastown", ev.Rig)
	assert.Equal(t, "Toast", ev.Polecat)
	assert.Equal(t, "diagnose", ev.StepID)
	assert.Equal(t, StepAdvanced, ev.Type)
	assert.False(t, ev.Timestamp.IsZero())
}

func TestStepEventChannel(t *testing.T) {
	ev := NewStepEvent("gastown", "Toast", "test", StepCompleted)
	assert.Equal(t, "orchestrator:step:gastown", ev.Channel())
}

func TestStepEventJSON(t *testing.T) {
	ev := NewStepEvent("gastown", "Toast", "build", StepFailed)
	ev.Timestamp = time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	ev.Detail = "compilation error"

	data, err := ev.Marshal()
	require.NoError(t, err)

	parsed, err := UnmarshalStepEvent(data)
	require.NoError(t, err)
	assert.Equal(t, ev.Rig, parsed.Rig)
	assert.Equal(t, ev.Polecat, parsed.Polecat)
	assert.Equal(t, ev.StepID, parsed.StepID)
	assert.Equal(t, ev.Type, parsed.Type)
	assert.Equal(t, ev.Detail, parsed.Detail)
}

func TestLocalBus(t *testing.T) {
	b := NewLocalBus()

	received := make(chan StepEvent, 10)
	unsub := b.Subscribe("orchestrator:step:gastown", func(ev StepEvent) {
		received <- ev
	})
	defer unsub()

	ev := NewStepEvent("gastown", "Toast", "diagnose", StepAdvanced)
	err := b.Publish(ev)
	require.NoError(t, err)

	select {
	case got := <-received:
		assert.Equal(t, "diagnose", got.StepID)
		assert.Equal(t, StepAdvanced, got.Type)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestLocalBusMultipleSubscribers(t *testing.T) {
	b := NewLocalBus()

	count := 0
	ch := "orchestrator:step:rig1"

	unsub1 := b.Subscribe(ch, func(ev StepEvent) { count++ })
	unsub2 := b.Subscribe(ch, func(ev StepEvent) { count++ })
	defer unsub1()
	defer unsub2()

	ev := NewStepEvent("rig1", "agent", "step1", StepAdvanced)
	require.NoError(t, b.Publish(ev))

	// Give goroutines time to process
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 2, count)
}

func TestLocalBusUnsubscribe(t *testing.T) {
	b := NewLocalBus()

	called := false
	unsub := b.Subscribe("orchestrator:step:rig1", func(ev StepEvent) {
		called = true
	})
	unsub()

	ev := NewStepEvent("rig1", "agent", "step1", StepAdvanced)
	require.NoError(t, b.Publish(ev))

	time.Sleep(50 * time.Millisecond)
	assert.False(t, called)
}

func TestLocalBusNoSubscribers(t *testing.T) {
	b := NewLocalBus()
	ev := NewStepEvent("rig1", "agent", "step1", StepAdvanced)
	// Should not panic or error
	assert.NoError(t, b.Publish(ev))
}
