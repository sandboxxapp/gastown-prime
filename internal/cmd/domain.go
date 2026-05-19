package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/workspace"
)

var domainCmd = &cobra.Command{
	Use:     "domain",
	GroupID: GroupWorkspace,
	Short:   "Browse rig-specific domain documentation",
	Long: `Browse the domain library for a rig: list TOC, read a doc on demand,
or grep across the docs.

Polecats get the TOC at prime time (no inlined content). Use these
subcommands to fetch individual docs only when needed.

Examples:
  gt domain list                   # TOC for the current rig
  gt domain list <rig>             # TOC for a specific rig
  gt domain read auth/token.md     # Render a doc + freshness + linked notes
  gt domain search "firebase"      # Grep across the current rig's domain dir`,
}

var domainListCmd = &cobra.Command{
	Use:   "list [rig]",
	Short: "List domain docs as a table of contents",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runDomainList,
}

var domainReadCmd = &cobra.Command{
	Use:   "read <relpath>",
	Short: "Render a domain doc with freshness and linked notes",
	Args:  cobra.ExactArgs(1),
	RunE:  runDomainRead,
}

var domainSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Grep across the current rig's domain directory",
	Args:  cobra.ExactArgs(1),
	RunE:  runDomainSearch,
}

var domainRigFlag string

func init() {
	domainReadCmd.Flags().StringVar(&domainRigFlag, "rig", "", "Rig name (defaults to current rig)")
	domainSearchCmd.Flags().StringVar(&domainRigFlag, "rig", "", "Rig name (defaults to current rig)")

	domainCmd.AddCommand(domainListCmd)
	domainCmd.AddCommand(domainReadCmd)
	domainCmd.AddCommand(domainSearchCmd)
	rootCmd.AddCommand(domainCmd)
}

// resolveRig returns (townRoot, rigName) honoring an explicit argument first,
// then the --rig flag, then the current working directory.
func resolveRig(explicit string) (string, string, error) {
	townRoot, err := workspace.FindFromCwd()
	if err != nil || townRoot == "" {
		return "", "", fmt.Errorf("not in a Gas Town workspace")
	}
	if explicit != "" {
		return townRoot, explicit, nil
	}
	if domainRigFlag != "" {
		return townRoot, domainRigFlag, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return townRoot, "", fmt.Errorf("getting cwd: %w", err)
	}
	rig := detectRigFromPath(townRoot, cwd)
	if rig == "" {
		return townRoot, "", fmt.Errorf("could not detect rig from %s — pass <rig> explicitly", cwd)
	}
	return townRoot, rig, nil
}

func runDomainList(cmd *cobra.Command, args []string) error {
	explicit := ""
	if len(args) > 0 {
		explicit = args[0]
	}
	townRoot, rig, err := resolveRig(explicit)
	if err != nil {
		return err
	}
	return writeDomainTOC(cmd.OutOrStdout(), townRoot, rig)
}

func writeDomainTOC(w io.Writer, townRoot, rig string) error {
	docs := config.LoadDomainDocsTOC(townRoot, rig)
	if len(docs) == 0 {
		fmt.Fprintf(w, "No domain docs found for rig %q at %s.\n", rig, filepath.Join(townRoot, rig, "domain"))
		return nil
	}
	fmt.Fprintf(w, "# Domain Library — %s (%d docs)\n\n", rig, len(docs))
	fmt.Fprintln(w, "| Path | Topic | Last-touched |")
	fmt.Fprintln(w, "|------|-------|--------------|")
	for _, doc := range docs {
		topic := doc.Summary
		if topic == "" {
			topic = doc.Title
		}
		topic = sanitizeTableCell(topic)
		last := doc.LastTouched
		if last == "" {
			last = "—"
		}
		fmt.Fprintf(w, "| `%s` | %s | %s |\n", doc.RelPath, topic, last)
	}
	return nil
}

func runDomainRead(cmd *cobra.Command, args []string) error {
	relpath := args[0]
	townRoot, rig, err := resolveRig("")
	if err != nil {
		return err
	}
	return writeDomainDoc(cmd.OutOrStdout(), townRoot, rig, relpath)
}

func writeDomainDoc(w io.Writer, townRoot, rig, relpath string) error {
	domainDir := filepath.Join(townRoot, rig, "domain")
	clean := filepath.Clean(relpath)
	if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return fmt.Errorf("invalid relpath %q: must be inside %s", relpath, domainDir)
	}
	absPath := filepath.Join(domainDir, clean)
	data, err := os.ReadFile(absPath) //nolint:gosec // G304: path constrained to domain dir
	if err != nil {
		return fmt.Errorf("reading %s: %w", relpath, err)
	}

	last := ""
	cmd := exec.Command("git", "log", "-1", "--format=%ad", "--date=short", "--", filepath.Base(absPath)) //nolint:gosec // G204
	cmd.Dir = filepath.Dir(absPath)
	if out, err := cmd.Output(); err == nil {
		last = strings.TrimSpace(string(out))
	}

	fmt.Fprintf(w, "# %s\n", relpath)
	if last != "" {
		fmt.Fprintf(w, "_Last-touched: %s_\n", last)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, strings.TrimSpace(string(data)))

	// List related raw notes from domain/notes/
	notes := relatedNotes(domainDir, clean)
	if len(notes) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "## Related raw notes")
		fmt.Fprintln(w)
		for _, n := range notes {
			fmt.Fprintf(w, "- `notes/%s`\n", n)
		}
	}
	return nil
}

// relatedNotes returns filenames under domain/notes/ that reference the doc's
// topic. Heuristic: any notes file whose name shares the doc's stem prefix.
func relatedNotes(domainDir, relpath string) []string {
	notesDir := filepath.Join(domainDir, "notes")
	entries, err := os.ReadDir(notesDir)
	if err != nil {
		return nil
	}
	stem := strings.TrimSuffix(filepath.Base(relpath), ".md")
	if stem == "" {
		return nil
	}
	var hits []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		if strings.HasPrefix(e.Name(), stem) || strings.Contains(e.Name(), stem) {
			hits = append(hits, e.Name())
		}
	}
	sort.Strings(hits)
	return hits
}

func runDomainSearch(cmd *cobra.Command, args []string) error {
	query := args[0]
	townRoot, rig, err := resolveRig("")
	if err != nil {
		return err
	}
	domainDir := filepath.Join(townRoot, rig, "domain")
	if _, err := os.Stat(domainDir); err != nil {
		return fmt.Errorf("no domain dir at %s", domainDir)
	}
	// Delegate to grep -rn for familiar output. Limit to *.md.
	grep := exec.Command("grep", "-rn", "--include=*.md", query, domainDir) //nolint:gosec // G204: query passed as argv, not shell
	grep.Stdout = cmd.OutOrStdout()
	grep.Stderr = cmd.ErrOrStderr()
	err = grep.Run()
	if err != nil {
		// grep exits 1 when no matches — surface that without wrapping in our error.
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			fmt.Fprintf(cmd.OutOrStdout(), "no matches for %q in %s\n", query, domainDir)
			return nil
		}
		return fmt.Errorf("grep failed: %w", err)
	}
	return nil
}
