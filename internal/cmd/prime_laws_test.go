package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/formula"
)

// TestLawsFormulaForRole checks the derived formula name, including the
// no-laws-possible roles.
func TestLawsFormulaForRole(t *testing.T) {
	t.Parallel()

	tests := []struct {
		role Role
		want string
	}{
		{RoleMayor, "laws-mayor"},
		{RoleDeacon, "laws-deacon"},
		{RoleWitness, "laws-witness"},
		{RolePolecat, "laws-polecat"},
		// Real prime roles that have no laws-* file today. The name is still
		// derived; the embed lookup is what degrades.
		{RoleRefinery, "laws-refinery"},
		{RoleCrew, "laws-crew"},
		{RoleBoot, "laws-boot"},
		{RoleDog, "laws-dog"},
		// No role, no laws.
		{RoleUnknown, ""},
		{Role(""), ""},
	}

	for _, tt := range tests {
		if got := lawsFormulaForRole(tt.role); got != tt.want {
			t.Errorf("lawsFormulaForRole(%q) = %q, want %q", tt.role, got, tt.want)
		}
	}
}

// TestOutputRoleLawsRendersVerbatim is the core guarantee: the [rule].summary
// reaches the agent byte-for-byte. bridge #183 shipped a regex scraper that
// skipped TOML escape decoding and corrupted 11 safety lines; this test fails
// if anyone reintroduces summarising, reflowing, or re-splitting.
func TestOutputRoleLawsRendersVerbatim(t *testing.T) {
	t.Parallel()

	content, err := formula.GetEmbeddedFormulaContent("laws-polecat")
	if err != nil {
		t.Fatalf("laws-polecat must be embedded: %v", err)
	}
	f, err := formula.Parse(content)
	if err != nil {
		t.Fatalf("parsing laws-polecat: %v", err)
	}
	if f.Rule == nil {
		t.Fatal("laws-polecat has no [rule] block")
	}

	var buf bytes.Buffer
	outputRoleLaws(RoleContext{Role: RolePolecat}, &buf, false)
	got := buf.String()

	want := strings.TrimRight(f.Rule.Summary, "\n")
	if !strings.Contains(got, want) {
		t.Errorf("rendered laws do not contain [rule].summary verbatim.\n"+
			"summary is %d chars, output is %d chars", len(want), len(got))
	}
}

// TestOutputRoleLawsCarriesLawTail guards the split-law contract: the LAW TAIL
// pointer is the trailing block of [rule].summary, not a separate field, so a
// verbatim render must carry it. If prime ever starts trimming the summary,
// agents lose the pointer to the other half of their laws.
//
// Only 5 of the 13 embedded laws are split (bridge #183 / gastown-prime #70):
// context-db, deacon, mayor, polecat, pressman. Of the roles prime renders,
// that means mayor, deacon and polecat carry a tail and witness does not —
// witness is unsplit and complete inline, which is not a defect.
func TestOutputRoleLawsCarriesLawTail(t *testing.T) {
	t.Parallel()

	for _, role := range []Role{RolePolecat, RoleMayor, RoleDeacon} {
		var buf bytes.Buffer
		outputRoleLaws(RoleContext{Role: role}, &buf, false)
		got := buf.String()

		if got == "" {
			t.Errorf("role %q rendered no laws at all", role)
			continue
		}
		if !strings.Contains(got, "LAW TAIL") {
			t.Errorf("role %q is a split law but rendered without the LAW TAIL pointer", role)
		}
	}

	// Unsplit laws must still render — they just have no tail.
	var buf bytes.Buffer
	outputRoleLaws(RoleContext{Role: RoleWitness}, &buf, false)
	if buf.Len() == 0 {
		t.Error("witness laws are unsplit but must still render")
	}
}

// TestOutputRoleLawsDegradesCleanly covers roles with no laws-<role> file.
// "Cleanly" means: no panic, no error, and critically NOTHING on the writer —
// prime output is fed verbatim to an agent, so a stray warning there reads as
// an instruction.
func TestOutputRoleLawsDegradesCleanly(t *testing.T) {
	t.Parallel()

	for _, role := range []Role{RoleRefinery, RoleCrew, RoleBoot, RoleDog, RoleUnknown, Role("")} {
		var buf bytes.Buffer
		outputRoleLaws(RoleContext{Role: role}, &buf, false)
		if got := buf.String(); got != "" {
			t.Errorf("role %q should emit nothing, got %q", role, got)
		}
	}
}

// TestOutputRoleLawsSuppression covers the no-double-delivery contract. A town
// that already ships laws by another channel (the bridge's SessionStart hooks)
// must be able to opt out without a flag day.
func TestOutputRoleLawsSuppression(t *testing.T) {
	// Not parallel: mutates the process env and the primeNoLaws global.
	t.Run("flag", func(t *testing.T) {
		primeNoLaws = true
		defer func() { primeNoLaws = false }()

		var buf bytes.Buffer
		outputRoleLaws(RoleContext{Role: RolePolecat}, &buf, false)
		if got := buf.String(); got != "" {
			t.Errorf("--no-laws should suppress, got %d chars", len(got))
		}
	})

	t.Run("env falsey values suppress", func(t *testing.T) {
		for _, v := range []string{"0", "off", "false", "no", "OFF", " False "} {
			t.Setenv(EnvPrimeLaws, v)

			var buf bytes.Buffer
			outputRoleLaws(RoleContext{Role: RolePolecat}, &buf, false)
			if got := buf.String(); got != "" {
				t.Errorf("%s=%q should suppress, got %d chars", EnvPrimeLaws, v, len(got))
			}
		}
	})

	t.Run("unset and truthy values render", func(t *testing.T) {
		for _, v := range []string{"", "1", "auto", "on", "true"} {
			t.Setenv(EnvPrimeLaws, v)

			var buf bytes.Buffer
			outputRoleLaws(RoleContext{Role: RolePolecat}, &buf, false)
			if buf.Len() == 0 {
				t.Errorf("%s=%q should render laws, got nothing", EnvPrimeLaws, v)
			}
		}
	})
}

// TestOutputRoleLawsHeader checks the framing that tells an agent what it is
// reading and what a violation costs.
func TestOutputRoleLawsHeader(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	outputRoleLaws(RoleContext{Role: RolePolecat}, &buf, false)
	got := buf.String()

	for _, want := range []string{"LAWS — POLECAT", "laws-polecat", "Violation severity"} {
		if !strings.Contains(got, want) {
			t.Errorf("laws header missing %q", want)
		}
	}
}

// TestOutputRoleLawsExplain checks --explain reports the decision for both the
// rendered and the degraded path, so an operator can tell "no laws for this
// role" apart from "laws suppressed".
func TestOutputRoleLawsExplain(t *testing.T) {
	t.Parallel()

	var rendered bytes.Buffer
	outputRoleLaws(RoleContext{Role: RolePolecat}, &rendered, true)
	if !strings.Contains(rendered.String(), "[EXPLAIN] Role laws: rendering laws-polecat") {
		t.Error("explain should report the rendered formula and its size")
	}

	var missing bytes.Buffer
	outputRoleLaws(RoleContext{Role: RoleRefinery}, &missing, true)
	if !strings.Contains(missing.String(), "[EXPLAIN] Role laws: no embedded laws-refinery") {
		t.Errorf("explain should report the missing formula, got %q", missing.String())
	}
}

// TestAllEmbeddedLawsParse is a guard on the whole set rather than the four
// roles prime renders today: every laws-* in the embed must parse with the
// real TOML parser and carry a non-empty [rule].summary. A laws file that
// parses but has an empty summary would silently render nothing.
func TestAllEmbeddedLawsParse(t *testing.T) {
	t.Parallel()

	roles := []string{
		"archivist", "context-db", "cross-service-team", "deacon", "foreman",
		"gastown-testing", "informant", "mayor", "polecat", "pressman",
		"prospector", "vivisectionist", "witness",
	}

	for _, role := range roles {
		name := "laws-" + role
		content, err := formula.GetEmbeddedFormulaContent(name)
		if err != nil {
			t.Errorf("%s not embedded: %v", name, err)
			continue
		}
		f, err := formula.Parse(content)
		if err != nil {
			t.Errorf("%s failed to parse: %v", name, err)
			continue
		}
		if f.Rule == nil || strings.TrimSpace(f.Rule.Summary) == "" {
			t.Errorf("%s has no [rule].summary — it would render as nothing", name)
		}
	}
}
