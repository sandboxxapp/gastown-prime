package tmux

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Captures of Claude Code's workspace trust dialog (sbx-gastown-hu03s0).
//
// Both versions render the dialog through the same confirmation component with
// cancelFirst:true, focus:"cancel", hideIndexes:true, so "No, exit" is listed
// FIRST and PRE-SELECTED on 2.1.282 as well as on 2.1.325 (read from the
// render source in ~/.local/share/claude/versions/<v>). 2.1.325 adds
// disablePrintableKeybindings:true, so digit/letter shortcuts no longer
// select an option; only the arrow keys move the cursor.
const (
	claudeTrustDialog2_1_282 = `────────────────────────────────────────────────────────────────────────────────
 Accessing workspace:

 /Users/someone/rigs/sandboxx-print-indexer

 Quick safety check: Is this a project you created or one you trust? (Like your
 own code, a well-known open source project, or work from your team). If not,
 take a moment to review what's in this folder first.

 Claude Code'll be able to read, edit, and execute files here.

 Security guide

 ❯ No, exit
   Yes, I trust this folder

 Enter to confirm · Esc to cancel
`

	// As observed live by the mayor on 2026-11-16 (first Fable 5.2 dispatch).
	claudeTrustDialog2_1_325 = `────────────────────────────────────────────────────────────────────────────────
 Accessing workspace:

 /Users/someone/rigs/sandboxx-print-indexer

 Quick safety check: Is this a project you created or one you trust? (Like your
 own code, a well-known open source project, or work from your team). If not,
 take a moment to review what's in this folder first.

 Claude Code'll be able to read, edit, and execute files here.

 Security guide

 ❯ No, exit
   Yes, I trust this folder

 Enter to confirm · Esc to cancel
`

	// The same dialog after one Down press: the cursor is on the accepting option.
	claudeTrustDialog2_1_325Accepting = `────────────────────────────────────────────────────────────────────────────────
 Accessing workspace:

 /Users/someone/rigs/sandboxx-print-indexer

 Quick safety check: Is this a project you created or one you trust? (Like your
 own code, a well-known open source project, or work from your team). If not,
 take a moment to review what's in this folder first.

 Claude Code'll be able to read, edit, and execute files here.

 Security guide

   No, exit
 ❯ Yes, I trust this folder

 Enter to confirm · Esc to cancel
`

	// When the folder pre-approves permissions (offerBackstop), the declining
	// option continues without them instead of exiting.
	claudeTrustDialogBackstop = `│ Quick safety check: Is this a project you created or one you trust?
│ ⚠ This folder pre-approves 2 tool permissions in .claude/settings.json:
│   Bash(make:*), Read
│ These will apply without asking. Only proceed if you trust this configuration.
│ ❯ No, continue without these permissions
│   Yes, I trust this folder
`

	// Codex trust prompt, accepting option first and focused.
	codexTrustDialog = `> You are in /tmp/demo

  Do you trust the contents of this directory? Working with untrusted contents
  comes with higher risk of prompt injection.

› 1. Yes, continue
  2. No, quit

  Press enter to continue
`
)

func TestTrustAcceptOptionState(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		content     string
		wantFound   bool
		wantFocused bool
	}{
		{"claude 2.1.282 pre-selects No, exit", claudeTrustDialog2_1_282, true, false},
		{"claude 2.1.325 pre-selects No, exit", claudeTrustDialog2_1_325, true, false},
		{"claude 2.1.325 after Down", claudeTrustDialog2_1_325Accepting, true, true},
		{"claude backstop variant, boxed", claudeTrustDialogBackstop, true, false},
		{"codex accepting option focused", codexTrustDialog, true, true},
		{"dialog text without options", "Quick safety check: Is this a project you created or one you trust?", false, false},
		{"unknown accepting label", "Quick safety check\n❯ No, exit\n  Sure, go ahead", false, false},
		{"shell prompt", "user@host:~$", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found, focused := trustAcceptOptionState(tt.content)
			if found != tt.wantFound || focused != tt.wantFocused {
				t.Errorf("trustAcceptOptionState() = (found=%v, focused=%v), want (found=%v, focused=%v)",
					found, focused, tt.wantFound, tt.wantFocused)
			}
		})
	}
}

func TestContainsWorkspaceTrustDialog_ClaudeVersions(t *testing.T) {
	t.Parallel()
	for name, content := range map[string]string{
		"2.1.282":  claudeTrustDialog2_1_282,
		"2.1.325":  claudeTrustDialog2_1_325,
		"backstop": claudeTrustDialogBackstop,
	} {
		if !containsWorkspaceTrustDialog(content) {
			t.Errorf("%s: trust dialog not detected", name)
		}
	}
}

// fakeTrustDialogScript renders a two-option select like Claude Code's trust
// dialog with the first option pre-selected, moves the "❯" cursor on Up/Down,
// and on Enter writes the confirmed option's label to $1 and then idles, so
// keys sent after the dialog closes land on a live pane.
const fakeTrustDialogScript = `#!/bin/bash
out="$1"; shift
opts=("$@")
sel=0
draw() {
  clear
  printf ' Accessing workspace:\n\n /tmp/demo\n\n'
  printf ' Quick safety check: Is this a project you created or one you trust?\n\n'
  for i in "${!opts[@]}"; do
    if [ "$i" -eq "$sel" ]; then printf ' ❯ %s\n' "${opts[$i]}"; else printf '   %s\n' "${opts[$i]}"; fi
  done
  printf '\n Enter to confirm · Esc to cancel\n'
}
draw
while IFS= read -rsn1 k; do
  if [ "$k" = $'\e' ]; then
    read -rsn2 -t 1 rest
    case "$rest" in
      '[B'|'OB') sel=$(( (sel + 1) % ${#opts[@]} )) ;;
      '[A'|'OA') sel=$(( (sel + ${#opts[@]} - 1) % ${#opts[@]} )) ;;
    esac
    draw
  elif [ -z "$k" ]; then
    printf '%s' "${opts[$sel]}" > "$out"
    exec sleep 60
  fi
done
`

// startFakeTrustDialog runs the fake dialog in a session on an isolated tmux
// server (never the live town server) and waits until it has rendered.
func startFakeTrustDialog(t *testing.T, opts ...string) (tm *Tmux, session, outFile string) {
	t.Helper()
	if !hasTmux() {
		t.Skip("tmux not installed")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "dialog.sh")
	if err := os.WriteFile(script, []byte(fakeTrustDialogScript), 0o755); err != nil {
		t.Fatal(err)
	}
	outFile = filepath.Join(dir, "choice")

	tm = NewTmuxWithSocket(fmt.Sprintf("gt-test-trust-%d", os.Getpid()))
	t.Cleanup(func() { _, _ = tm.run("kill-server") })
	session = "gt-test-trust-select"

	args := []string{"bash", script, outFile}
	for _, o := range opts {
		args = append(args, "'"+o+"'")
	}
	if err := tm.NewSessionWithCommand(session, dir, strings.Join(args, " ")); err != nil {
		t.Fatalf("NewSessionWithCommand: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		content, err := tm.CapturePane(session, 30)
		if err == nil && strings.Contains(content, "Enter to confirm") {
			return tm, session, outFile
		}
		if time.Now().After(deadline) {
			t.Fatalf("fake trust dialog did not render; last capture: %q (err %v)", content, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func readChoice(t *testing.T, outFile string) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if b, err := os.ReadFile(outFile); err == nil {
			return string(b)
		}
		if time.Now().After(deadline) {
			return ""
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// TestAcceptWorkspaceTrustDialog_SelectsAcceptWhenDeclinePreselected is the
// regression test for sbx-gastown-hu03s0: Claude Code pre-selects "No, exit",
// and the old Enter-on-pre-selection confirmed it.
func TestAcceptWorkspaceTrustDialog_SelectsAcceptWhenDeclinePreselected(t *testing.T) {
	tm, session, outFile := startFakeTrustDialog(t, "No, exit", "Yes, I trust this folder")

	if err := tm.AcceptWorkspaceTrustDialog(session); err != nil {
		t.Fatalf("AcceptWorkspaceTrustDialog: %v", err)
	}
	if got := readChoice(t, outFile); got != "Yes, I trust this folder" {
		t.Errorf("confirmed option = %q, want %q", got, "Yes, I trust this folder")
	}
}

// TestAcceptWorkspaceTrustDialog_AcceptAlreadySelected covers a dialog whose
// accepting option is first and focused (Codex): Enter only, no Down.
func TestAcceptWorkspaceTrustDialog_AcceptAlreadySelected(t *testing.T) {
	tm, session, outFile := startFakeTrustDialog(t, "Yes, I trust this folder", "No, exit")

	if err := tm.AcceptWorkspaceTrustDialog(session); err != nil {
		t.Fatalf("AcceptWorkspaceTrustDialog: %v", err)
	}
	if got := readChoice(t, outFile); got != "Yes, I trust this folder" {
		t.Errorf("confirmed option = %q, want %q", got, "Yes, I trust this folder")
	}
}

// TestAcceptWorkspaceTrustDialog_RefusesWithoutAcceptOption verifies that a
// trust dialog with no recognised accepting option gets no keypress at all.
func TestAcceptWorkspaceTrustDialog_RefusesWithoutAcceptOption(t *testing.T) {
	tm, session, outFile := startFakeTrustDialog(t, "No, exit", "Sure, go ahead")

	err := tm.AcceptWorkspaceTrustDialog(session)
	if err == nil {
		t.Fatal("AcceptWorkspaceTrustDialog: want an error when no accepting option is shown, got nil")
	}
	if !strings.Contains(err.Error(), "refusing to press Enter") {
		t.Errorf("error = %v, want it to say Enter was refused", err)
	}
	if b, readErr := os.ReadFile(outFile); readErr == nil {
		t.Errorf("an option was confirmed (%q); want no keypress", string(b))
	}
}

// TestDismissStartupDialogsBlind_SelectsAcceptOnTrustDialog verifies the
// witness stall remediation no longer confirms the pre-selected "No, exit".
func TestDismissStartupDialogsBlind_SelectsAcceptOnTrustDialog(t *testing.T) {
	tm, session, outFile := startFakeTrustDialog(t, "No, exit", "Yes, I trust this folder")

	if err := tm.DismissStartupDialogsBlind(session); err != nil {
		t.Fatalf("DismissStartupDialogsBlind: %v", err)
	}
	if got := readChoice(t, outFile); got != "Yes, I trust this folder" {
		t.Errorf("confirmed option = %q, want %q", got, "Yes, I trust this folder")
	}
}
