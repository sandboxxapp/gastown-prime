package cmd

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/authzproxy"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var mcpCmd = &cobra.Command{
	Use:     "mcp",
	GroupID: GroupServices,
	Short:   "Manage MCP server delegation for polecats",
	Long: `Manage MCP (Model Context Protocol) server delegation for dispatched polecats.

Polecats dispatched via 'gt sling --mcp <name>:<scope>' get scoped access to
MCP servers through the authz-proxy daemon. The daemon spawns upstream MCP
servers on behalf of polecats using launch specs stored in .mcp-secrets.json.

This command group manages the secrets file (listing, syncing from the mayor's
MCP configs) and verifies delegation readiness.`,
	RunE: requireSubcommand,
}

var mcpListCmd = &cobra.Command{
	Use:   "list",
	Short: "List MCPs configured in the mayor and available for delegation",
	Long: `List MCPs configured in the mayor's Claude Code and in the authz-proxy secrets.

Shows a table with three columns:
  - MAYOR:   whether the MCP is configured for the mayor (~/.claude/.mcp.json or town .mcp.json)
  - SECRETS: whether the MCP is populated in .mcp-secrets.json (ready for delegation)
  - LAUNCH:  the upstream command used by the proxy to spawn the MCP

Use 'gt mcp sync' to populate missing entries from the mayor's config.`,
	RunE: runMCPList,
}

var (
	mcpSyncDry   bool
	mcpSyncPrune bool
)

var mcpSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync the mayor's MCP configs into .mcp-secrets.json for delegation",
	Long: `Sync the mayor's MCP configs into the authz-proxy secrets file.

Reads MCP server launch specs from:
  1. ~/.claude/.mcp.json (user-scope Claude Code config)
  2. <town>/.mcp.json (project-scope Claude Code config)

...and writes them into <town>/.mcp-secrets.json (the authz-proxy secrets file).
Existing non-MCP keys (gcp_profiles) are preserved.

When the same server name appears in both sources, the project-scope config wins.

Sensitive values (API keys, tokens) already present in the source env: blocks are
carried through verbatim. If the mayor's MCP config references an external
credentials file (e.g. VANTA_ENV_FILE), only the reference is synced.`,
	RunE: runMCPSync,
}

var mcpDoctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check whether MCP delegation is ready for polecat dispatch",
	Long: `Verify that the authz-proxy daemon is reachable and that MCP delegation is
configured correctly.

Checks:
  - authz_proxy configured in town settings
  - authz-proxy binary exists and is executable
  - daemon socket is reachable
  - .mcp-secrets.json is present and parseable
  - at least one delegatable MCP is configured`,
	RunE: runMCPDoctor,
}

func init() {
	mcpSyncCmd.Flags().BoolVar(&mcpSyncDry, "dry-run", false, "Show what would change without writing")
	mcpSyncCmd.Flags().BoolVar(&mcpSyncPrune, "prune", false, "Remove MCP entries from secrets that aren't in the mayor's config")

	// Doctor reports its own findings; cobra's usage dump on error is noise.
	mcpDoctorCmd.SilenceUsage = true
	mcpDoctorCmd.SilenceErrors = true

	mcpCmd.AddCommand(mcpListCmd)
	mcpCmd.AddCommand(mcpSyncCmd)
	mcpCmd.AddCommand(mcpDoctorCmd)
	rootCmd.AddCommand(mcpCmd)
}

type mcpConfig struct {
	MCPServers map[string]authzproxy.MCPServerSpec `json:"mcpServers"`
}

// readMCPConfig reads a Claude Code .mcp.json file. Returns an empty map if the
// file is missing.
func readMCPConfig(path string) (map[string]authzproxy.MCPServerSpec, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: caller-controlled path
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]authzproxy.MCPServerSpec{}, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var cfg mcpConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if cfg.MCPServers == nil {
		return map[string]authzproxy.MCPServerSpec{}, nil
	}
	return cfg.MCPServers, nil
}

// mayorMCPSources returns the paths gt reads for mayor-side MCP configs, in
// ascending precedence order (later entries override earlier ones).
func mayorMCPSources(townRoot string) []string {
	paths := []string{}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".claude", ".mcp.json"))
	}
	paths = append(paths, filepath.Join(townRoot, ".mcp.json"))
	return paths
}

func loadMayorMCPs(townRoot string) (map[string]authzproxy.MCPServerSpec, []string, error) {
	merged := make(map[string]authzproxy.MCPServerSpec)
	var sourcesRead []string
	for _, path := range mayorMCPSources(townRoot) {
		servers, err := readMCPConfig(path)
		if err != nil {
			return nil, nil, err
		}
		if len(servers) == 0 {
			continue
		}
		sourcesRead = append(sourcesRead, path)
		maps.Copy(merged, servers)
	}
	return merged, sourcesRead, nil
}

func loadAuthzProxyConfig(townRoot string) (*config.AuthzProxyConfig, error) {
	settingsPath := config.TownSettingsPath(townRoot)
	settings, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		return nil, fmt.Errorf("loading town settings: %w", err)
	}
	if settings.AuthzProxy == nil {
		return nil, fmt.Errorf("authz_proxy not configured in %s — add an authz_proxy block with binary, socket, and secrets_path", settingsPath)
	}
	return settings.AuthzProxy, nil
}

func runMCPList(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}
	proxyCfg, err := loadAuthzProxyConfig(townRoot)
	if err != nil {
		return err
	}

	mayor, _, err := loadMayorMCPs(townRoot)
	if err != nil {
		return err
	}

	secrets, err := authzproxy.LoadMCPServersFromSecrets(proxyCfg.SecretsPath)
	if err != nil {
		return err
	}

	names := make(map[string]struct{})
	for n := range mayor {
		names[n] = struct{}{}
	}
	for n := range secrets {
		names[n] = struct{}{}
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)

	if len(sorted) == 0 {
		fmt.Println("No MCPs found.")
		fmt.Printf("  Checked: ~/.claude/.mcp.json, %s/.mcp.json, %s\n", townRoot, proxyCfg.SecretsPath)
		return nil
	}

	fmt.Printf("%-20s %-8s %-10s %s\n", "NAME", "MAYOR", "SECRETS", "LAUNCH")
	for _, name := range sorted {
		mayorMark := "-"
		if _, ok := mayor[name]; ok {
			mayorMark = "yes"
		}
		secretsMark := "-"
		launch := ""
		if spec, ok := secrets[name]; ok {
			secretsMark = "yes"
			launch = launchSummary(spec)
		} else if spec, ok := mayor[name]; ok {
			launch = launchSummary(spec)
		}
		fmt.Printf("%-20s %-8s %-10s %s\n", name, mayorMark, secretsMark, launch)
	}

	var missing []string
	for n := range mayor {
		if _, ok := secrets[n]; !ok {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		fmt.Printf("\n%s %d MCP(s) in mayor config but not in secrets: %v\n", style.Warning.Render("⚠"), len(missing), missing)
		fmt.Printf("  Run %s to populate.\n", style.Bold.Render("gt mcp sync"))
	}
	return nil
}

func launchSummary(spec authzproxy.MCPServerSpec) string {
	if len(spec.Args) > 0 {
		return fmt.Sprintf("%s %s", spec.Command, spec.Args[0])
	}
	return spec.Command
}

func runMCPSync(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}
	proxyCfg, err := loadAuthzProxyConfig(townRoot)
	if err != nil {
		return err
	}
	if proxyCfg.SecretsPath == "" {
		return fmt.Errorf("authz_proxy.secrets_path is empty in town settings")
	}

	mayor, sources, err := loadMayorMCPs(townRoot)
	if err != nil {
		return err
	}

	if len(sources) == 0 {
		fmt.Printf("%s No mayor MCP configs found (checked ~/.claude/.mcp.json and %s/.mcp.json)\n",
			style.Warning.Render("⚠"), townRoot)
		return nil
	}

	fmt.Printf("Mayor MCP sources read:\n")
	for _, p := range sources {
		fmt.Printf("  - %s\n", p)
	}

	existing, err := authzproxy.LoadMCPServersFromSecrets(proxyCfg.SecretsPath)
	if err != nil {
		return err
	}

	var toAdd, toUpdate []string
	for name, spec := range mayor {
		if prev, ok := existing[name]; ok {
			a, _ := json.Marshal(prev)
			b, _ := json.Marshal(spec)
			if string(a) != string(b) {
				toUpdate = append(toUpdate, name)
			}
		} else {
			toAdd = append(toAdd, name)
		}
	}
	var toPrune []string
	if mcpSyncPrune {
		for name := range existing {
			if _, ok := mayor[name]; !ok {
				toPrune = append(toPrune, name)
			}
		}
	}
	sort.Strings(toAdd)
	sort.Strings(toUpdate)
	sort.Strings(toPrune)

	if len(toAdd) == 0 && len(toUpdate) == 0 && len(toPrune) == 0 {
		fmt.Printf("%s .mcp-secrets.json already in sync with mayor (%d MCP(s))\n", style.Bold.Render("✓"), len(mayor))
		return nil
	}

	if len(toAdd) > 0 {
		fmt.Printf("  %s add: %v\n", style.Bold.Render("+"), toAdd)
	}
	if len(toUpdate) > 0 {
		fmt.Printf("  %s update: %v\n", style.Bold.Render("~"), toUpdate)
	}
	if len(toPrune) > 0 {
		fmt.Printf("  %s prune: %v\n", style.Warning.Render("-"), toPrune)
	}

	if mcpSyncDry {
		fmt.Printf("\n%s Dry run — no changes written to %s\n", style.Dim.Render("→"), proxyCfg.SecretsPath)
		return nil
	}

	changed, err := authzproxy.WriteMCPServersToSecrets(proxyCfg.SecretsPath, mayor, mcpSyncPrune)
	if err != nil {
		return err
	}
	fmt.Printf("\n%s Wrote %d change(s) to %s\n", style.Bold.Render("✓"), len(changed), proxyCfg.SecretsPath)
	return nil
}

func runMCPDoctor(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}
	proxyCfg, err := loadAuthzProxyConfig(townRoot)
	if err != nil {
		fmt.Printf("%s %v\n", style.Warning.Render("✗"), err)
		return err
	}

	fmt.Printf("%s authz_proxy configured in town settings\n", style.Bold.Render("✓"))
	fmt.Printf("  binary:       %s\n", proxyCfg.Binary)
	fmt.Printf("  socket:       %s\n", proxyCfg.Socket)
	fmt.Printf("  secrets_path: %s\n", proxyCfg.SecretsPath)
	fmt.Println()

	binaryOK := true
	if info, err := os.Stat(proxyCfg.Binary); err != nil {
		fmt.Printf("%s authz-proxy binary not found at %s\n", style.Warning.Render("✗"), proxyCfg.Binary)
		binaryOK = false
	} else if info.Mode()&0111 == 0 {
		fmt.Printf("%s authz-proxy binary at %s is not executable\n", style.Warning.Render("✗"), proxyCfg.Binary)
		binaryOK = false
	} else {
		fmt.Printf("%s authz-proxy binary is executable\n", style.Bold.Render("✓"))
	}

	socketOK := true
	if err := authzproxy.CheckDaemonSocket(proxyCfg.Socket); err != nil {
		fmt.Printf("%s daemon socket unreachable: %v\n", style.Warning.Render("✗"), err)
		fmt.Printf("  Start the daemon:\n")
		fmt.Printf("    %s daemon --secrets %s --socket %s\n",
			proxyCfg.Binary, proxyCfg.SecretsPath, proxyCfg.Socket)
		socketOK = false
	} else {
		fmt.Printf("%s daemon socket reachable\n", style.Bold.Render("✓"))
	}

	secrets, err := authzproxy.LoadMCPServersFromSecrets(proxyCfg.SecretsPath)
	if err != nil {
		fmt.Printf("%s could not parse secrets: %v\n", style.Warning.Render("✗"), err)
		return err
	}
	if len(secrets) == 0 {
		fmt.Printf("%s no MCP servers configured in %s\n", style.Warning.Render("⚠"), proxyCfg.SecretsPath)
		fmt.Printf("  Run %s to populate from the mayor's config.\n", style.Bold.Render("gt mcp sync"))
	} else {
		names := make([]string, 0, len(secrets))
		for n := range secrets {
			names = append(names, n)
		}
		sort.Strings(names)
		fmt.Printf("%s %d delegatable MCP server(s): %v\n", style.Bold.Render("✓"), len(secrets), names)
	}

	fmt.Println()
	if binaryOK && socketOK && len(secrets) > 0 {
		fmt.Printf("%s MCP delegation ready. Dispatch with:\n", style.Bold.Render("✓"))
		fmt.Printf("    gt sling <bead> --mcp <name>:<scope>\n")
		return nil
	}
	fmt.Printf("%s MCP delegation not ready — see warnings above.\n", style.Warning.Render("⚠"))
	return fmt.Errorf("mcp delegation not ready")
}
