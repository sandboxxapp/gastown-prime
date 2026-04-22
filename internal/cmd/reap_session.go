package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/tmux"
)

var (
	reapSessionName  string
	reapSessionDelay time.Duration
)

// reapSessionCmd is the detached helper that `gt exit` (and future polecat
// self-terminate paths) spawn so the session kill survives gt's own exit.
// Previously gt exit used an in-process goroutine, which was torn down the
// instant runExit returned — leaving orphan tmux sessions (sbx-gastown-xpuv).
var reapSessionCmd = &cobra.Command{
	Use:    "_reap-session",
	Short:  "Kill a tmux session after a delay (internal: self-terminate helper)",
	Hidden: true,
	RunE:   runReapSession,
}

func init() {
	reapSessionCmd.Flags().StringVar(&reapSessionName, "session", "", "tmux session name to kill")
	reapSessionCmd.Flags().DurationVar(&reapSessionDelay, "delay", 3*time.Second, "grace period before kill")
	_ = reapSessionCmd.MarkFlagRequired("session")
	rootCmd.AddCommand(reapSessionCmd)
}

func runReapSession(cmd *cobra.Command, args []string) error {
	if reapSessionName == "" {
		return fmt.Errorf("--session is required")
	}
	if reapSessionDelay > 0 {
		time.Sleep(reapSessionDelay)
	}
	// NewTmux picks up the town socket set by persistentPreRun → InitRegistry,
	// so the kill targets the correct tmux server even when invoked detached.
	t := tmux.NewTmux()
	if err := t.KillSessionWithProcesses(reapSessionName); err != nil {
		return fmt.Errorf("killing %s: %w", reapSessionName, err)
	}
	return nil
}
