package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/steveyegge/gastown/internal/cli"
	"github.com/steveyegge/gastown/internal/formula"
)

// EnvPrimeLaws suppresses law rendering in `gt prime` when set to a falsey
// value ("0", "off", "false", "no"). It exists so a town that already delivers
// laws by another channel — notably the bridge, whose .claude/hooks/
// mayor-context.sh (SessionStart) and agent-laws.sh (SubagentStart) have
// carried the laws since before prime could — can opt out per-town without a
// flag day. Set it BEFORE upgrading gt and no agent ever sees the laws twice;
// unset it when the hooks are retired.
const EnvPrimeLaws = "GT_PRIME_LAWS"

// primeNoLaws is the one-off command-line escape hatch (`gt prime --no-laws`).
var primeNoLaws bool

// lawsSuppressed reports whether law rendering has been turned off, either by
// the --no-laws flag or by GT_PRIME_LAWS being set to a falsey value.
//
// Note the asymmetry: the env var suppresses only on an explicit falsey value,
// so GT_PRIME_LAWS=1, GT_PRIME_LAWS=auto, and an unset variable all render.
// Defaulting to render is the point of the bead — every town that is not the
// bridge is currently lawless and needs no configuration to be fixed.
func lawsSuppressed() bool {
	if primeNoLaws {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvPrimeLaws))) {
	case "0", "off", "false", "no":
		return true
	}
	return false
}

// lawsFormulaForRole returns the embedded constraint-formula name carrying the
// laws for a role, or "" when the role can have none.
//
// The name is derived rather than table-mapped so that dropping a new
// laws-<role>.formula.toml into internal/formula/formulas/ starts rendering
// with no code change here. Today that matters for refinery, crew, boot and
// dog, which are real prime roles with no laws file yet.
func lawsFormulaForRole(role Role) string {
	if role == RoleUnknown || role == "" {
		return ""
	}
	return "laws-" + string(role)
}

// outputRoleLaws renders the role's standing laws as the first substantive
// block of `gt prime` output.
//
// Position is load-bearing, not cosmetic. `gt prime --hook` output reaches the
// agent as SessionStart content, and an over-large payload is not truncated —
// the harness spills it to a file and shows only a ~2 KB preview. Polecat prime
// already measures ~25 KB (refinery ~50 KB), so anything rendered after the
// role template is past the preview window. Laws go first or they go unread.
//
// The [rule].summary is emitted VERBATIM. The laws-* files are already split
// into an inline SAFETY CORE plus a trailing LAW TAIL pointer (see
// laws-polecat.formula.toml) — the tail is part of the summary string, not a
// separate field, so rendering the summary whole carries the pointer for free.
// Do not summarise, re-split, or reflow: bridge #183 corrupted 11 safety lines
// by scraping this text with a regex instead of decoding the TOML.
//
// w and explainEnabled are injected so tests can capture output without
// mutating os.Stdout or the primeExplain global (same pattern as
// outputRoleDirectives).
func outputRoleLaws(ctx RoleContext, w io.Writer, explainEnabled bool) {
	explainf := func(format string, args ...any) {
		if explainEnabled {
			fmt.Fprintf(w, "\n[EXPLAIN] "+format+"\n", args...)
		}
	}

	if lawsSuppressed() {
		explainf("Role laws: suppressed (--no-laws or %s), assuming another channel delivers them", EnvPrimeLaws)
		return
	}

	name := lawsFormulaForRole(ctx.Role)
	if name == "" {
		explainf("Role laws: role %q has no laws formula", ctx.Role)
		return
	}

	// Degrade cleanly: a role with no laws-<role>.formula.toml is the normal
	// case for refinery/crew/boot/dog, not an error. Never warn on stdout —
	// prime output is fed verbatim to an agent, and a warning there reads as
	// an instruction.
	content, err := formula.GetEmbeddedFormulaContent(name)
	if err != nil {
		explainf("Role laws: no embedded %s (%v)", name, err)
		return
	}

	f, err := formula.Parse(content)
	if err != nil {
		explainf("Role laws: %s failed to parse (%v)", name, err)
		return
	}
	if f.Rule == nil || strings.TrimSpace(f.Rule.Summary) == "" {
		explainf("Role laws: %s has no [rule].summary", name)
		return
	}

	explainf("Role laws: rendering %s (%d chars) ahead of role context", name, len(f.Rule.Summary))

	fmt.Fprintln(w)
	fmt.Fprintf(w, "## ⚖️ LAWS — %s (binding, read before anything else)\n", strings.ToUpper(string(ctx.Role)))
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Source: `%s` (constraint formula, compiled into %s).\n", name, cli.Name())
	if f.Rule.Severity != "" {
		fmt.Fprintf(w, "Violation severity: **%s**.\n", f.Rule.Severity)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, strings.TrimRight(f.Rule.Summary, "\n"))
	fmt.Fprintln(w)
}
