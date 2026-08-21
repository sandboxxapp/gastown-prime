package beads

import (
	"strings"
	"testing"
)

// TestValidateRigNameForAgentIDs_AllowsHyphens is the core of sbx-gastown-oxta6:
// rig names must be allowed to match their GitHub repo name, which means hyphens.
// ParseAgentBeadID has scanned right-to-left against the known role set since it
// was written (see its doc comment), so a hyphenated rig segment is recoverable.
func TestValidateRigNameForAgentIDs_AllowsHyphens(t *testing.T) {
	// Every hyphenated rig name live in this town today, plus the two that were
	// renamed to underscores only to satisfy the old ban.
	valid := []string{
		"ads-dw",
		"ads-projects",
		"airflow-composer",
		"pb-ccm-exec",
		"sandboxx-ads-looker",
		"sandboxx-android",
		"sandboxx-backend",
		"sandboxx-ios",
		"sandboxx-marketing-service",
		"sandboxx-store",
		"sandboxx-terraform",
		"sandboxx-web",
		"waypoints-admin-web",
		"waypoints-android",
		"waypoints-api",
		"waypoints-ios",
		"waypoints-web",
		"gastown-prime",
		"news-api",              // currently spelled news_api because of the ban
		"sandboxx-file-service", // currently spelled sandboxx_file_service
		// Underscored and bare names must keep working exactly as before.
		"news_api",
		"sandboxx_file_service",
		"gastown_upstream",
		"bridge",
		"cms",
		// A role token is only a problem at the edges — see the reject table.
		"my-witness",  // trailing SINGLETON role: recoverable
		"my-refinery", // trailing SINGLETON role: recoverable
		"a-crew-b",    // named role in the MIDDLE: recoverable
		"a-polecat-b", // named role in the MIDDLE: recoverable
		"tools-dog",   // "dog" is only special as the FIRST segment
		"a-b-c-d-e",   // arbitrarily many hyphens
		"witness",     // bare singleton role as a whole name: recoverable
		"refinery",
	}

	for _, name := range valid {
		t.Run(name, func(t *testing.T) {
			if err := ValidateRigNameForAgentIDs(name); err != nil {
				t.Errorf("ValidateRigNameForAgentIDs(%q) = %v, want nil", name, err)
			}
		})
	}
}

// TestValidateRigNameForAgentIDs_RejectsAmbiguous locks in the NARROW set of
// names that genuinely break ParseAgentBeadID, measured by round-tripping every
// role against every shape. These are the only reasons the old blanket ban had.
func TestValidateRigNameForAgentIDs_RejectsAmbiguous(t *testing.T) {
	tests := []struct {
		name    string
		wantErr string
	}{
		// Path/ID-breaking characters — unchanged from the old ban.
		{"my.rig", "invalid characters"},
		{"my rig", "invalid characters"},
		{"my/rig", "invalid characters"},
		{`my\rig`, "invalid characters"},
		{"my\trig", "invalid characters"},

		// Empty segments produce empty rig components.
		{"", "cannot be empty"},
		{"-leading", "empty segment"},
		{"trailing-", "empty segment"},
		{"double--hyphen", "empty segment"},

		// Leading named/town-named role: the collapsed-form and dog branches of
		// ParseAgentBeadID fire first and swallow the rest of the name.
		{"crew-tools", "ambiguous"},
		{"polecat-lab", "ambiguous"},
		{"dog-house", "ambiguous"},

		// Trailing named role: the right-scan prefers the named-role reading, so
		// singleton IDs for this rig mis-parse.
		{"tools-crew", "ambiguous"},
		{"tools-polecat", "ambiguous"},

		// A bare named role is both first and last segment.
		{"crew", "ambiguous"},
		{"polecat", "ambiguous"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRigNameForAgentIDs(tt.name)
			if err == nil {
				t.Fatalf("ValidateRigNameForAgentIDs(%q) = nil, want error containing %q", tt.name, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("ValidateRigNameForAgentIDs(%q) error = %q, want it to contain %q", tt.name, err, tt.wantErr)
			}
		})
	}
}

// TestValidateRigNameForAgentIDs_RejectsUppercase guards the invariant the
// suggestion text relies on: agent IDs are lowercase.
func TestValidateRigNameForAgentIDs_SuggestsUsableAlternative(t *testing.T) {
	err := ValidateRigNameForAgentIDs("my.rig")
	if err == nil {
		t.Fatal("expected error")
	}
	// The suggestion must itself be a name this function accepts, or we send the
	// operator around the loop a second time.
	if !strings.Contains(err.Error(), `"my-rig"`) {
		t.Errorf("error %q should suggest the hyphenated form now that hyphens are legal", err)
	}
	if verr := ValidateRigNameForAgentIDs("my-rig"); verr != nil {
		t.Errorf("suggested name %q is itself rejected: %v", "my-rig", verr)
	}
}

// TestAgentIDRoundTripHyphenatedRigs is the property the ban existed to protect:
// construct an agent ID for a rig, parse it back, get the same rig/role/name.
func TestAgentIDRoundTripHyphenatedRigs(t *testing.T) {
	rigs := []string{
		"gastown-prime",
		"sandboxx-backend",
		"waypoints-admin-web",
		"pb-ccm-exec",
		"a-b-c-d-e",
		"my-witness",
		"news_api",
		"news-api",
		"bridge",
	}
	roles := []struct{ role, name string }{
		{"witness", ""},
		{"refinery", ""},
		{"polecat", "gentian"},
		{"crew", "max"},
	}

	for _, rigName := range rigs {
		// Every name exercised here must be one the validator accepts, otherwise
		// the round-trip claim is about names that can never exist.
		if err := ValidateRigNameForAgentIDs(rigName); err != nil {
			t.Fatalf("test fixture %q is rejected by the validator: %v", rigName, err)
		}
		for _, r := range roles {
			id := AgentBeadIDWithPrefix("sbx", rigName, r.role, r.name)
			t.Run(id, func(t *testing.T) {
				if err := ValidateAgentID(id); err != nil {
					t.Errorf("ValidateAgentID(%q) = %v, want nil", id, err)
				}
				gotRig, gotRole, gotName, ok := ParseAgentBeadID(id)
				if !ok {
					t.Fatalf("ParseAgentBeadID(%q) failed", id)
				}
				if gotRig != rigName || gotRole != r.role || gotName != r.name {
					t.Errorf("ParseAgentBeadID(%q) = (%q, %q, %q), want (%q, %q, %q)",
						id, gotRig, gotRole, gotName, rigName, r.role, r.name)
				}
			})
		}
	}
}
