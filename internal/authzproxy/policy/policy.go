// Package policy implements MCP tool authorization and read/write classification.
package policy

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// Checker determines whether a tool call or credential request is allowed.
type Checker interface {
	Check(mcp, tool string) Result
	CheckGCP(profile string) Result
	FilterTools(mcp string, tools []string) []string
}

// Result is the outcome of a policy check.
type Result struct {
	Allowed        bool
	Classification string // "read" or "write"
	Reason         string // empty if allowed
}

// MCPPolicy defines access for a single MCP server.
type MCPPolicy struct {
	Mode      string            `json:"mode"`                // "read" or "read,write"
	Tools     []string          `json:"tools,omitempty"`     // glob patterns, empty = all
	Overrides map[string]string `json:"overrides,omitempty"` // tool -> "read"/"write"
}

// GCPProfile defines a GCP token minting target in the authz context.
type GCPProfile struct {
	// Mode selects the minting strategy: "impersonate" (default) or "downscope".
	Mode     string   `json:"mode,omitempty"`
	TargetSA string   `json:"target_sa,omitempty"`
	Scopes   []string `json:"scopes"`
	Lifetime string   `json:"lifetime,omitempty"`
	// Project is the GCP project ID; optional, reserved for future CAB/STS scoping.
	Project string `json:"project,omitempty"`
}

// GCPAuthz defines GCP credential access for a client.
type GCPAuthz struct {
	Profiles map[string]GCPProfile `json:"profiles"`
}

// Authz is the authorization context for a client connection.
type Authz struct {
	Role    string               `json:"role"`
	AgentID string               `json:"agent_id"`
	Bead    string               `json:"bead"`
	MCPs    map[string]MCPPolicy `json:"mcps"`
	GCP     *GCPAuthz            `json:"gcp,omitempty"`
}

// ParseAuthz parses an authz JSON context.
func ParseAuthz(data []byte) (*Authz, error) {
	var authz Authz
	if err := json.Unmarshal(data, &authz); err != nil {
		return nil, fmt.Errorf("parse authz: %w", err)
	}
	if authz.MCPs == nil {
		authz.MCPs = make(map[string]MCPPolicy)
	}
	return &authz, nil
}

// AuthzChecker implements Checker using an Authz context.
type AuthzChecker struct {
	authz *Authz
}

// NewChecker creates a policy checker from an authz context.
func NewChecker(authz *Authz) Checker {
	return &AuthzChecker{authz: authz}
}

// Check evaluates whether a tool call is allowed.
func (c *AuthzChecker) Check(mcp, tool string) Result {
	pol, ok := c.authz.MCPs[mcp]
	if !ok {
		return Result{Allowed: false, Reason: fmt.Sprintf("MCP %q not in authz context", mcp)}
	}

	if !matchesAnyGlob(pol.Tools, tool) {
		return Result{Allowed: false, Reason: fmt.Sprintf("tool %q not matched by globs %v for %q", tool, pol.Tools, mcp)}
	}

	classification := ClassifyTool(tool)
	if override, ok := pol.Overrides[tool]; ok {
		classification = override
	}

	if classification == "write" && !strings.Contains(pol.Mode, "write") {
		return Result{
			Allowed:        false,
			Classification: classification,
			Reason:         fmt.Sprintf("write tool %q blocked by read-only policy for %q", tool, mcp),
		}
	}

	return Result{Allowed: true, Classification: classification}
}

// CheckGCP evaluates whether a GCP credential profile is allowed.
func (c *AuthzChecker) CheckGCP(profile string) Result {
	if c.authz.GCP == nil {
		return Result{Allowed: false, Reason: "no GCP access in authz context"}
	}
	_, ok := c.authz.GCP.Profiles[profile]
	if !ok {
		return Result{Allowed: false, Reason: fmt.Sprintf("GCP profile %q not in authz context", profile)}
	}
	return Result{Allowed: true, Classification: "gcp"}
}

// FilterTools returns only tools allowed by the policy for an MCP.
func (c *AuthzChecker) FilterTools(mcp string, tools []string) []string {
	pol, ok := c.authz.MCPs[mcp]
	if !ok {
		return nil
	}
	var out []string
	for _, t := range tools {
		if matchesAnyGlob(pol.Tools, t) {
			out = append(out, t)
		}
	}
	return out
}

// Read/write classification heuristics.
var (
	readPrefixes  = []string{"get_", "list_", "search_", "read_", "find_", "query", "resolve"}
	readExact     = map[string]bool{"controls": true, "documents": true, "frameworks": true, "tests": true, "vulnerabilities": true, "people": true, "risks": true}
	writePrefixes = []string{"create_", "save_", "update_", "delete_", "push_", "merge_", "send_", "add_", "remove_"}
)

// ClassifyTool returns "read" or "write". Unknown defaults to "write" (conservative).
func ClassifyTool(tool string) string {
	lower := strings.ToLower(tool)
	if readExact[lower] {
		return "read"
	}
	for _, p := range readPrefixes {
		if strings.HasPrefix(lower, p) {
			return "read"
		}
	}
	for _, p := range writePrefixes {
		if strings.HasPrefix(lower, p) {
			return "write"
		}
	}
	return "write"
}

// MatchToolGlob checks if a tool name matches a glob pattern.
func MatchToolGlob(pattern, tool string) bool {
	matched, _ := filepath.Match(pattern, tool)
	return matched
}

func matchesAnyGlob(patterns []string, tool string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, p := range patterns {
		if MatchToolGlob(p, tool) {
			return true
		}
	}
	return false
}
