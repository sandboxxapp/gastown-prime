package polecat

import (
	"os"
	"testing"
)

// TestAllocateAvoidingLive_SkipsLiveName verifies the settled-name gate
// (sbx-gastown-gsyki): a themed name whose prior occupant is still live is
// skipped and the next genuinely-free name is returned instead.
func TestAllocateAvoidingLive_SkipsLiveName(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-live-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	pool := NewNamePoolWithConfig(tmpDir, "testrig", "mad-max", nil, DefaultPoolSize)

	// furiosa (the head of mad-max) is still settling — a live process exists.
	isLive := func(name string) bool { return name == "furiosa" }

	name, err := pool.AllocateAvoidingLive(isLive)
	if err != nil {
		t.Fatalf("AllocateAvoidingLive error: %v", err)
	}
	if name == "furiosa" {
		t.Fatalf("allocated a name with a live prior occupant: %s", name)
	}
	if name != "nux" {
		t.Errorf("expected the next free name (nux), got %s", name)
	}
	if pool.InUse["furiosa"] {
		t.Errorf("furiosa must not be marked InUse — it was skipped, not claimed")
	}
}

// TestAllocateAvoidingLive_ReusableOnceSettled verifies that once the prior
// occupant is fully gone (isLive returns false), the name becomes allocatable
// again.
func TestAllocateAvoidingLive_ReusableOnceSettled(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-live-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	pool := NewNamePoolWithConfig(tmpDir, "testrig", "mad-max", nil, DefaultPoolSize)

	live := true
	isLive := func(name string) bool { return name == "furiosa" && live }

	// While furiosa is live it is skipped (cursor lands on nux).
	if name, _ := pool.AllocateAvoidingLive(isLive); name != "nux" {
		t.Fatalf("expected nux while furiosa live, got %s", name)
	}

	// Reset the pool so furiosa is at the head of a fresh cursor again.
	pool.Reset()
	live = false

	name, err := pool.AllocateAvoidingLive(isLive)
	if err != nil {
		t.Fatalf("AllocateAvoidingLive error: %v", err)
	}
	if name != "furiosa" {
		t.Errorf("expected furiosa to be reusable once settled, got %s", name)
	}
}

// TestAllocateAvoidingLive_ExhaustionFallsToOverflow verifies that when every
// free themed name is still live (unsettled), allocation falls through to
// overflow numbering exactly like the in-use exhaustion path.
func TestAllocateAvoidingLive_ExhaustionFallsToOverflow(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-live-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Small pool so exhaustion is cheap to assert.
	pool := NewNamePoolWithConfig(tmpDir, "gastown", "mad-max", nil, 3)

	// Every themed name reports a live prior occupant.
	allLive := func(string) bool { return true }

	name, err := pool.AllocateAvoidingLive(allLive)
	if err != nil {
		t.Fatalf("AllocateAvoidingLive error: %v", err)
	}
	if name != "4" {
		t.Errorf("expected overflow name 4 when all themed names are unsettled, got %s", name)
	}
}

// TestAllocateAvoidingLive_CursorAdvancesNormally verifies the round-robin
// cursor (sbx-gastown-uzrj) is unaffected by the liveness gate: with no live
// names it behaves exactly like Allocate, advancing one slot per allocation.
func TestAllocateAvoidingLive_CursorAdvancesNormally(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-live-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	pool := NewNamePoolWithConfig(tmpDir, "testrig", "mad-max", nil, DefaultPoolSize)

	none := func(string) bool { return false }

	want := []string{"furiosa", "nux", "slit"}
	for i, expected := range want {
		name, err := pool.AllocateAvoidingLive(none)
		if err != nil {
			t.Fatalf("AllocateAvoidingLive %d error: %v", i, err)
		}
		if name != expected {
			t.Errorf("allocation %d: expected %s, got %s", i, expected, name)
		}
	}
}

// TestAllocateAvoidingLive_SkippedNameDoesNotStealCursor verifies that skipping
// a live name does not advance the cursor onto it: the name claimed is where the
// cursor lands, so a live name skipped this round is re-tested next round.
func TestAllocateAvoidingLive_SkippedNameDoesNotStealCursor(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-live-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	pool := NewNamePoolWithConfig(tmpDir, "testrig", "mad-max", nil, DefaultPoolSize)

	// nux (index 1) is live; furiosa (index 0) is free.
	isLive := func(name string) bool { return name == "nux" }

	// First allocation claims furiosa (index 0); cursor -> index 1 (nux).
	if name, _ := pool.AllocateAvoidingLive(isLive); name != "furiosa" {
		t.Fatalf("expected furiosa first, got %s", name)
	}
	// Second allocation starts at nux (live -> skip) and lands on slit (index 2).
	if name, _ := pool.AllocateAvoidingLive(isLive); name != "slit" {
		t.Fatalf("expected slit after skipping live nux, got %s", name)
	}
	if pool.InUse["nux"] {
		t.Errorf("nux was live and must not be claimed")
	}
}
