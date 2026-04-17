# MCP Delegation to Polecats

Polecats dispatched via `gt sling --mcp <name>:<scope>` get scoped access to
MCP (Model Context Protocol) servers that the mayor has configured. Delegation
goes through the **authz-proxy** daemon so polecats get per-dispatch tokens
with tool-level authorization — they don't see the mayor's raw OAuth tokens.

This guide covers operator setup, the delegation lifecycle, and the scope syntax.

## Prerequisites

1. **authz-proxy binary.** Built from `cmd/authz-proxy/` in the bridge repo.
   The binary runs a daemon (shared across dispatches) and a per-dispatch
   stdio↔socket frontend that Claude Code spawns as an MCP server.

2. **Town settings.** `settings/config.json` must include an `authz_proxy` block:
   ```json
   {
     "authz_proxy": {
       "binary": "/path/to/authz-proxy",
       "socket": "/path/to/.gastown/mcp-proxy.sock",
       "secrets_path": "/path/to/.mcp-secrets.json"
     }
   }
   ```

3. **`.mcp-secrets.json` populated.** The daemon reads this file to know how to
   spawn upstream MCP servers on behalf of polecats. Populate it with
   `gt mcp sync` (see below).

4. **Daemon running.** Launch once per town session:
   ```
   authz-proxy daemon --secrets <secrets_path> --socket <socket>
   ```

Run `gt mcp doctor` any time to verify all four prerequisites.

## Operator commands

### `gt mcp list`

Shows every MCP seen in either the mayor's Claude Code config or the secrets
file, with a per-row readiness indicator.

```
NAME                 MAYOR    SECRETS    LAUNCH
iterable             yes      -          npx -y
vanta                yes      yes        npx -y
```

`MAYOR=yes, SECRETS=-` → the mayor has the MCP configured but it isn't
delegatable yet. Run `gt mcp sync`.

### `gt mcp sync [--dry-run] [--prune]`

Merges the mayor's MCP launch specs into `.mcp-secrets.json`:

- Reads `~/.claude/.mcp.json` and `<town>/.mcp.json` (project scope wins).
- Writes each `mcpServers` entry (command, args, env) verbatim into the
  secrets file.
- Preserves non-MCP keys (`gcp_profiles`).
- `--prune` removes MCP entries not in the mayor's config (otherwise the
  secrets file only grows).
- `--dry-run` prints the diff without writing.

Sensitive values already present in the source `env:` block (API keys,
credential file paths) are carried through unchanged.

### `gt mcp doctor`

End-to-end readiness check. Exits non-zero if any prerequisite is missing.

## Dispatching with `--mcp`

```
gt sling <bead> <rig> --mcp vanta:read,write --mcp linear:read
```

Scope syntax: `<name>:<mode>` where `<mode>` is one of `read`, `write`, or
`read,write`. Omitting the mode defaults to `read`.

Before the polecat is spawned, `gt sling`:

1. Validates that every requested MCP is present in `.mcp-secrets.json`.
   If not, dispatch aborts with a message pointing at `gt mcp sync`.
2. Checks that the authz-proxy daemon socket is reachable.
3. Writes `.bridge/mcp-authz.json` with the polecat's authz context
   (role, agent_id, bead, per-MCP scopes).
4. Writes `.mcp.json` in the worktree root, pointing each MCP at the
   authz-proxy frontend binary (Claude Code spawns it per MCP).
5. Adds `mcp__<name>__*` patterns to the polecat's `.claude/settings.json`
   `permissions.allow` list so Claude Code will actually call the tools.

When the polecat runs an MCP tool, the frontend forwards the call to the
daemon, which classifies the tool as read/write, enforces the polecat's
authz context, and routes allowed calls to the upstream MCP server.

## Tool classification (read vs. write)

The policy package (`cmd/authz-proxy/policy`) classifies tool names by prefix:

- **Read**: `get_`, `list_`, `search_`, `read_`, `find_`, `query*`, `resolve*`,
  plus a small exact-match set (`controls`, `documents`, `frameworks`, etc.).
- **Write**: `create_`, `save_`, `update_`, `delete_`, `push_`, `merge_`,
  `send_`, `add_`, `remove_`.
- **Unknown**: conservatively classified as **write** (fail-closed).

A polecat dispatched with `vanta:read` calling a `save_*` tool gets a
permission-denied response from the daemon — the call never reaches Vanta.

## Adding a new delegatable MCP

1. Add the MCP to the mayor's Claude Code config (`~/.claude/.mcp.json` for
   user scope, `<town>/.mcp.json` for project scope). Use the standard
   Claude Code `mcpServers` entry shape: `{command, args, env}`.
2. Authenticate (if OAuth): follow the upstream MCP's install instructions.
   If the mayor already has credentials, you're done.
3. `gt mcp sync` — writes the launch spec into `.mcp-secrets.json`.
4. `gt mcp doctor` — confirms it's now delegatable.
5. Dispatch: `gt sling <bead> <rig> --mcp <name>:<scope>`.

## Failure modes

| Symptom | Cause | Fix |
|---------|-------|-----|
| `gt sling` aborts with "MCP server(s) not found in .mcp-secrets.json" | MCP not synced | `gt mcp sync` |
| `gt sling` aborts with "daemon socket not found" | Daemon not running | Start `authz-proxy daemon ...` |
| Polecat's MCP tool call returns "MCP not in authz context" | Dispatched without `--mcp <name>` | Re-sling with the right flag |
| Polecat's write tool returns "blocked by read-only policy" | Dispatched with `:read` instead of `:read,write` | Re-sling with wider scope |
| `gt mcp list` shows an MCP `SECRETS=yes` but `MAYOR=-` | Secrets file has stale entries | `gt mcp sync --prune` |

## Security boundary

- Polecats never see the mayor's raw upstream credentials. The daemon holds
  them and spawns the upstream process on the polecat's behalf.
- Each polecat connects fresh through the frontend bridge with its own
  `mcp-authz.json`. The authz context is per-dispatch — two polecats with
  different scopes can't leak into each other.
- GCP credentials follow the same pattern via `--gcp <profile>`: the daemon
  mints short-lived impersonated or downscoped tokens; the polecat never
  sees the mayor's ADC.
