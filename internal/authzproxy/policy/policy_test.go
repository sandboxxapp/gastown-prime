package policy

import "testing"

func TestClassifyTool_ReadPrefixes(t *testing.T) {
	for _, tool := range []string{"get_issue", "list_issues", "search_code", "read_resource", "find_organizations", "query", "resolve_library_id"} {
		if got := ClassifyTool(tool); got != "read" {
			t.Errorf("ClassifyTool(%q) = %q, want read", tool, got)
		}
	}
}

func TestClassifyTool_ReadExact(t *testing.T) {
	for _, tool := range []string{"controls", "documents", "frameworks", "tests", "vulnerabilities", "people", "risks"} {
		if got := ClassifyTool(tool); got != "read" {
			t.Errorf("ClassifyTool(%q) = %q, want read", tool, got)
		}
	}
}

func TestClassifyTool_WritePrefixes(t *testing.T) {
	for _, tool := range []string{"create_issue", "save_issue", "update_document", "delete_file", "push_files", "merge_pull_request", "send_message", "add_comment_to_pending_review", "remove_related_to"} {
		if got := ClassifyTool(tool); got != "write" {
			t.Errorf("ClassifyTool(%q) = %q, want write", tool, got)
		}
	}
}

func TestClassifyTool_UnknownDefaultsToWrite(t *testing.T) {
	for _, tool := range []string{"fork_repository", "assign_copilot_to_issue", "execute", "activate"} {
		if got := ClassifyTool(tool); got != "write" {
			t.Errorf("ClassifyTool(%q) = %q, want write (unknown)", tool, got)
		}
	}
}

func TestParseAuthz_Valid(t *testing.T) {
	authz, err := ParseAuthz([]byte(`{"role":"polecat","agent_id":"p-123","bead":"sbx-test","mcps":{"github":{"mode":"read"},"linear":{"mode":"read,write"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if authz.Role != "polecat" {
		t.Errorf("role = %q", authz.Role)
	}
	if len(authz.MCPs) != 2 {
		t.Errorf("len(mcps) = %d", len(authz.MCPs))
	}
}

func TestParseAuthz_Invalid(t *testing.T) {
	if _, err := ParseAuthz([]byte("nope")); err == nil {
		t.Error("expected error")
	}
}

func TestChecker_ReadAllowed(t *testing.T) {
	c := NewChecker(&Authz{MCPs: map[string]MCPPolicy{"github": {Mode: "read", Tools: []string{"*"}}}})
	r := c.Check("github", "get_file_contents")
	if !r.Allowed {
		t.Errorf("blocked: %s", r.Reason)
	}
}

func TestChecker_WriteBlocked(t *testing.T) {
	c := NewChecker(&Authz{MCPs: map[string]MCPPolicy{"github": {Mode: "read", Tools: []string{"*"}}}})
	r := c.Check("github", "create_pull_request")
	if r.Allowed {
		t.Error("should be blocked")
	}
}

func TestChecker_WriteAllowed(t *testing.T) {
	c := NewChecker(&Authz{MCPs: map[string]MCPPolicy{"linear": {Mode: "read,write", Tools: []string{"*"}}}})
	r := c.Check("linear", "save_issue")
	if !r.Allowed {
		t.Errorf("blocked: %s", r.Reason)
	}
}

func TestChecker_MCPNotInAuthz(t *testing.T) {
	c := NewChecker(&Authz{MCPs: map[string]MCPPolicy{"github": {Mode: "read"}}})
	r := c.Check("linear", "get_issue")
	if r.Allowed {
		t.Error("should be blocked — MCP not in authz")
	}
}

func TestChecker_ToolGlobFilter(t *testing.T) {
	c := NewChecker(&Authz{MCPs: map[string]MCPPolicy{"iterable": {Mode: "read", Tools: []string{"get_*", "list_*"}}}})
	if r := c.Check("iterable", "get_campaigns"); !r.Allowed {
		t.Errorf("get_campaigns blocked: %s", r.Reason)
	}
	if r := c.Check("iterable", "search_events"); r.Allowed {
		t.Error("search_events should not match get_*/list_*")
	}
}

func TestChecker_EmptyToolsDefaultsToAll(t *testing.T) {
	c := NewChecker(&Authz{MCPs: map[string]MCPPolicy{"github": {Mode: "read"}}})
	if r := c.Check("github", "get_file_contents"); !r.Allowed {
		t.Error("empty tools should allow all")
	}
}

func TestChecker_OverrideClassification(t *testing.T) {
	c := NewChecker(&Authz{MCPs: map[string]MCPPolicy{"github": {Mode: "read", Tools: []string{"*"}, Overrides: map[string]string{"fork_repository": "read"}}}})
	if r := c.Check("github", "fork_repository"); !r.Allowed {
		t.Errorf("fork with read override blocked: %s", r.Reason)
	}
}

func TestChecker_FilterTools(t *testing.T) {
	c := NewChecker(&Authz{MCPs: map[string]MCPPolicy{"github": {Mode: "read", Tools: []string{"get_*", "list_*"}}}})
	out := c.FilterTools("github", []string{"get_issue", "create_pr", "list_commits", "delete_file"})
	if len(out) != 2 {
		t.Errorf("filtered = %v, want [get_issue list_commits]", out)
	}
}

// --- GCP policy tests ---

func TestCheckGCP_ProfileAllowed(t *testing.T) {
	c := NewChecker(&Authz{
		MCPs: map[string]MCPPolicy{},
		GCP: &GCPAuthz{Profiles: map[string]GCPProfile{
			"terraform-plan": {TargetSA: "sa@proj.iam.gserviceaccount.com", Scopes: []string{"compute.readonly"}},
		}},
	})
	r := c.CheckGCP("terraform-plan")
	if !r.Allowed {
		t.Errorf("terraform-plan should be allowed: %s", r.Reason)
	}
}

func TestCheckGCP_ProfileNotInAuthz(t *testing.T) {
	c := NewChecker(&Authz{
		MCPs: map[string]MCPPolicy{},
		GCP: &GCPAuthz{Profiles: map[string]GCPProfile{
			"terraform-plan": {TargetSA: "sa@proj.iam.gserviceaccount.com"},
		}},
	})
	r := c.CheckGCP("terraform-apply")
	if r.Allowed {
		t.Error("terraform-apply should be blocked — not in authz")
	}
}

func TestCheckGCP_NoGCPAuthz(t *testing.T) {
	c := NewChecker(&Authz{MCPs: map[string]MCPPolicy{}})
	r := c.CheckGCP("terraform-plan")
	if r.Allowed {
		t.Error("should be blocked — no GCP authz at all")
	}
}

func TestParseAuthz_WithGCP(t *testing.T) {
	raw := `{"role":"polecat","mcps":{},"gcp":{"profiles":{"tf-plan":{"target_sa":"sa@proj.iam","scopes":["compute"],"lifetime":"3600s"}}}}`
	authz, err := ParseAuthz([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if authz.GCP == nil {
		t.Fatal("GCP is nil")
	}
	if len(authz.GCP.Profiles) != 1 {
		t.Errorf("profiles = %d, want 1", len(authz.GCP.Profiles))
	}
	p := authz.GCP.Profiles["tf-plan"]
	if p.TargetSA != "sa@proj.iam" {
		t.Errorf("target_sa = %q", p.TargetSA)
	}
}

func TestMatchToolGlob(t *testing.T) {
	tests := []struct {
		p, t string
		want bool
	}{
		{"*", "anything", true}, {"get_*", "get_issue", true}, {"get_*", "list_issues", false},
	}
	for _, tt := range tests {
		if got := MatchToolGlob(tt.p, tt.t); got != tt.want {
			t.Errorf("MatchToolGlob(%q, %q) = %v, want %v", tt.p, tt.t, got, tt.want)
		}
	}
}
