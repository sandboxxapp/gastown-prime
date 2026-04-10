package formula

import (
	"strings"
	"testing"
)

// TestMolPolecatWorkTDDExists verifies the mol-polecat-work-tdd embedded formula exists.
func TestMolPolecatWorkTDDExists(t *testing.T) {
	content, err := GetEmbeddedFormulaContent("mol-polecat-work-tdd")
	if err != nil {
		t.Fatalf("mol-polecat-work-tdd formula not found in embedded FS: %v", err)
	}
	if len(content) == 0 {
		t.Fatal("mol-polecat-work-tdd formula content is empty")
	}
}

// TestMolPolecatWorkTDDParses verifies the formula parses correctly.
func TestMolPolecatWorkTDDParses(t *testing.T) {
	content, err := GetEmbeddedFormulaContent("mol-polecat-work-tdd")
	if err != nil {
		t.Fatalf("loading formula: %v", err)
	}
	f, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if f.Name != "mol-polecat-work-tdd" {
		t.Errorf("formula name = %q, want %q", f.Name, "mol-polecat-work-tdd")
	}
	if f.Type != TypeWorkflow {
		t.Errorf("formula type = %q, want %q", f.Type, TypeWorkflow)
	}
	if len(f.Extends) == 0 {
		t.Fatal("formula should extend mol-polecat-work")
	}
	if f.Extends[0] != "mol-polecat-work" {
		t.Errorf("extends[0] = %q, want %q", f.Extends[0], "mol-polecat-work")
	}
	if f.Compose == nil {
		t.Fatal("formula should have compose rules")
	}
	if len(f.Compose.Expand) == 0 {
		t.Fatal("formula should have at least one expand rule")
	}
	if f.Compose.Expand[0].Target != "implement" {
		t.Errorf("expand[0].target = %q, want %q", f.Compose.Expand[0].Target, "implement")
	}
	if f.Compose.Expand[0].With != "tdd-cycle" {
		t.Errorf("expand[0].with = %q, want %q", f.Compose.Expand[0].With, "tdd-cycle")
	}
}

// TestMolPolecatWorkTDDResolves verifies composition resolves correctly:
// mol-polecat-work steps + tdd-cycle expansion replacing "implement".
func TestMolPolecatWorkTDDResolves(t *testing.T) {
	content, err := GetEmbeddedFormulaContent("mol-polecat-work-tdd")
	if err != nil {
		t.Fatalf("loading formula: %v", err)
	}
	f, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	resolved, err := Resolve(f, nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	// mol-polecat-work has 8 steps. tdd-cycle replaces "implement" with 5 steps.
	// So: 8 - 1 + 5 = 12 steps expected.
	if len(resolved.Steps) < 11 {
		t.Errorf("resolved formula has %d steps, want at least 11 (base steps minus implement + tdd expansion)", len(resolved.Steps))
	}

	// Verify the tdd-cycle steps are present (they should be prefixed with "implement.")
	tddStepIDs := []string{
		"implement.write-tests",
		"implement.verify-red",
		"implement.implement",
		"implement.verify-green",
		"implement.refactor",
	}
	stepMap := make(map[string]bool)
	for _, s := range resolved.Steps {
		stepMap[s.ID] = true
	}
	for _, id := range tddStepIDs {
		if !stepMap[id] {
			t.Errorf("missing expected tdd-cycle step %q in resolved formula", id)
		}
	}

	// The original "implement" step should be gone (replaced by tdd-cycle steps).
	if stepMap["implement"] {
		t.Error("original 'implement' step should have been replaced by tdd-cycle expansion")
	}

	// Verify commit-changes now depends on implement.refactor (the last tdd step)
	// instead of the original "implement" step.
	for _, s := range resolved.Steps {
		if s.ID == "commit-changes" {
			found := false
			for _, need := range s.Needs {
				if need == "implement.refactor" {
					found = true
				}
			}
			if !found {
				t.Errorf("commit-changes needs = %v, want to include 'implement.refactor'", s.Needs)
			}
			break
		}
	}

	// Verify topological sort works on the resolved formula.
	order, err := resolved.TopologicalSort()
	if err != nil {
		t.Errorf("TopologicalSort failed on resolved formula: %v", err)
	}
	t.Logf("Resolved step order: %v", order)
}

// TestMolPolecatWorkTDDVarsInherited verifies the formula inherits vars from mol-polecat-work.
func TestMolPolecatWorkTDDVarsInherited(t *testing.T) {
	content, err := GetEmbeddedFormulaContent("mol-polecat-work-tdd")
	if err != nil {
		t.Fatalf("loading formula: %v", err)
	}
	f, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	resolved, err := Resolve(f, nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	// Must inherit issue var (required) from mol-polecat-work.
	issueVar, ok := resolved.Vars["issue"]
	if !ok {
		t.Fatal("resolved formula missing 'issue' var (should be inherited from mol-polecat-work)")
	}
	if !issueVar.Required {
		t.Error("issue var should be required")
	}

	// Must inherit base_branch var.
	baseBranch, ok := resolved.Vars["base_branch"]
	if !ok {
		t.Fatal("resolved formula missing 'base_branch' var")
	}
	if baseBranch.Default != "main" {
		t.Errorf("base_branch default = %q, want %q", baseBranch.Default, "main")
	}
}

// TestTDDCycleExpansionStepDescriptions verifies the expanded tdd-cycle steps
// have the target step's title/description substituted correctly.
func TestTDDCycleExpansionStepDescriptions(t *testing.T) {
	content, err := GetEmbeddedFormulaContent("mol-polecat-work-tdd")
	if err != nil {
		t.Fatalf("loading formula: %v", err)
	}
	f, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	resolved, err := Resolve(f, nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	// The write-tests step title should reference the original implement step's title.
	for _, s := range resolved.Steps {
		if s.ID == "implement.write-tests" {
			if !strings.Contains(s.Title, "Implement") {
				t.Errorf("write-tests title = %q, expected to contain target step title", s.Title)
			}
			return
		}
	}
	t.Error("implement.write-tests step not found in resolved formula")
}
