package beads

import (
	"fmt"
	"strings"
	"unicode"
)

// rigNameForbiddenChars are characters that break paths or the agent-ID grammar
// itself. Hyphens are deliberately NOT in this set: ParseAgentBeadID scans
// right-to-left against the known role set, so a hyphenated rig segment is
// recoverable. See ValidateRigNameForAgentIDs.
const rigNameForbiddenChars = `./\`

// ValidateRigNameForAgentIDs reports whether name can serve as the <rig> segment
// of an agent ID without making the ID ambiguous to ParseAgentBeadID.
//
// Agent IDs are <prefix>-<rig>-<role>[-<name>]. Hyphen is the delimiter, which
// historically meant rig names could not contain one — and that ban is why rigs
// here were renamed away from their GitHub repo names (news-api -> news_api).
// The ban is no longer necessary: ParseAgentBeadID scans from the RIGHT for a
// token in ValidAgentRoles, so "sbx-waypoints-admin-web-polecat-nux" recovers
// rig "waypoints-admin-web" unambiguously.
//
// Two narrow shapes still genuinely break the parse, and only these are rejected:
//
//   - The FIRST segment is a named role (crew, polecat) or "dog". ParseAgentBeadID
//     checks those before the right-scan, because <prefix>-<role>-<name> is the
//     collapsed form used when prefix == rig. Rig "crew-tools" would parse as
//     rig=<prefix>, role="crew", name="tools-...".
//   - The LAST segment is a named role (crew, polecat). The right-scan prefers the
//     named-role reading, so singleton IDs mis-parse: rig "tools-crew" with role
//     "witness" parses as rig="tools", role="crew", name="witness".
//
// A role token elsewhere is fine: "my-witness", "a-crew-b" and "tools-dog" all
// round-trip. Reserved-name rejection is a separate, town-level concern and stays
// with the rig manager.
func ValidateRigNameForAgentIDs(name string) error {
	if name == "" {
		return fmt.Errorf("rig name cannot be empty")
	}

	if idx := strings.IndexFunc(name, isForbiddenRigNameRune); idx >= 0 {
		msg := fmt.Sprintf("rig name %q contains invalid characters; dots, spaces, and path separators are not allowed (hyphens and underscores are fine)", name)
		if suggestion := suggestRigName(name); suggestion != "" {
			msg += fmt.Sprintf(". Try %q instead", suggestion)
		}
		return fmt.Errorf("%s", msg)
	}

	segments := strings.Split(name, "-")
	for _, seg := range segments {
		if seg == "" {
			return fmt.Errorf("rig name %q has an empty segment; it cannot start or end with '-' or contain '--'", name)
		}
	}

	first := segments[0]
	if isNamedRole(first) || isTownLevelNamedRole(first) {
		return fmt.Errorf("rig name %q is ambiguous in agent IDs: a name starting with %q is parsed as the collapsed <prefix>-<role>-<name> form", name, first)
	}

	last := segments[len(segments)-1]
	if isNamedRole(last) {
		return fmt.Errorf("rig name %q is ambiguous in agent IDs: a name ending in %q is parsed as the role of a %s agent", name, last, last)
	}

	return nil
}

// isForbiddenRigNameRune reports whether r cannot appear in a rig name.
func isForbiddenRigNameRune(r rune) bool {
	if unicode.IsSpace(r) || unicode.IsControl(r) {
		return true
	}
	return strings.ContainsRune(rigNameForbiddenChars, r)
}

// suggestRigName converts a name containing forbidden characters into a
// hyphenated candidate. It returns "" when the candidate would itself be
// rejected, so we never send the operator around the loop twice.
func suggestRigName(name string) string {
	candidate := strings.Map(func(r rune) rune {
		if isForbiddenRigNameRune(r) {
			return '-'
		}
		return unicode.ToLower(r)
	}, name)

	// Collapse the runs of '-' that substitution can create, then trim the edges,
	// so the candidate has no empty segments.
	for strings.Contains(candidate, "--") {
		candidate = strings.ReplaceAll(candidate, "--", "-")
	}
	candidate = strings.Trim(candidate, "-")

	if candidate == "" || candidate == name {
		return ""
	}
	if ValidateRigNameForAgentIDs(candidate) != nil {
		return ""
	}
	return candidate
}
