# Gas Town Architecture

Technical architecture for Gas Town multi-agent workspace management.

## Two-Level Beads Architecture

Gas Town uses a two-level beads architecture to separate organizational coordination
from project implementation work.

| Level | Location | Prefix | Purpose |
|-------|----------|--------|---------|
| **Town** | `~/gt/.beads/` | `hq-*` | Cross-rig coordination, Mayor mail, agent identity |
| **Rig** | `<rig>/mayor/rig/.beads/` | project prefix | Implementation work, MRs, project issues |

### Town-Level Beads (`~/gt/.beads/`)

Organizational chain for cross-rig coordination:
- Mayor mail and messages
- Convoy coordination (batch work across rigs)
- Strategic issues and decisions
- **Town-level agent beads** (Mayor, Deacon)
- **Role definition beads** (global templates)

### Rig-Level Beads (`<rig>/mayor/rig/.beads/`)

Project chain for implementation work:
- Bugs, features, tasks for the project
- Merge requests and code reviews
- Project-specific molecules
- **Rig-level agent beads** (Witness, Refinery, Polecats)

## Agent Bead Storage

Agent beads track lifecycle state for each agent. Storage location depends on
the agent's scope.

| Agent Type | Scope | Bead Location | Bead ID Format |
|------------|-------|---------------|----------------|
| Mayor | Town | `~/gt/.beads/` | `hq-mayor` |
| Deacon | Town | `~/gt/.beads/` | `hq-deacon` |
| Boot | Town | `~/gt/.beads/` | `hq-boot` |
| Dogs | Town | `~/gt/.beads/` | `hq-dog-<name>` |
| Witness | Rig | `<rig>/.beads/` | `<prefix>-<rig>-witness` |
| Refinery | Rig | `<rig>/.beads/` | `<prefix>-<rig>-refinery` |
| Polecats | Rig | `<rig>/.beads/` | `<prefix>-<rig>-polecat-<name>` |
| Crew | Rig | `<rig>/.beads/` | `<prefix>-<rig>-crew-<name>` |

### Role Beads

Role beads are global templates stored in town beads with `hq-` prefix:
- `hq-mayor-role` - Mayor role definition
- `hq-deacon-role` - Deacon role definition
- `hq-boot-role` - Boot role definition
- `hq-witness-role` - Witness role definition
- `hq-refinery-role` - Refinery role definition
- `hq-polecat-role` - Polecat role definition
- `hq-crew-role` - Crew role definition
- `hq-dog-role` - Dog role definition

Each agent bead references its role bead via the `role_bead` field.

## Agent Taxonomy

### Town-Level Agents (Cross-Rig)

| Agent | Role | Persistence |
|-------|------|-------------|
| **Mayor** | Global coordinator, handles cross-rig communication and escalations | Persistent |
| **Deacon** | Daemon beacon — receives heartbeats, runs plugins and monitoring | Persistent |
| **Boot** | Deacon watchdog — spawned by daemon for triage decisions when Deacon is down | Ephemeral |
| **Dogs** | Long-running workers for cross-rig batch work | Variable |

### Rig-Level Agents (Per-Project)

| Agent | Role | Persistence |
|-------|------|-------------|
| **Witness** | Monitors polecat health, handles nudging and cleanup | Persistent |
| **Refinery** | Processes merge queue, runs verification | Persistent |
| **Polecats** | Workers with persistent identity, assigned to specific issues | Persistent identity, ephemeral sessions |
| **Crew** | Human workspaces — full git clones, user-managed lifecycle | Persistent |

## Directory Structure

```
~/gt/                           Town root
├── .beads/                     Town-level beads (hq-* prefix)
│   ├── metadata.json           Beads config (dolt_mode, dolt_database)
│   └── routes.jsonl            Prefix → rig routing table
├── .dolt-data/                 Centralized Dolt data directory
│   ├── hq/                     Town beads database (hq-* prefix)
│   ├── gastown/                Gastown rig database (gt-* prefix)
│   ├── beads/                  Beads rig database (bd-* prefix)
│   └── <other rigs>/           Per-rig databases
├── daemon/                     Daemon runtime state
│   ├── dolt-state.json         Dolt server state (pid, port, databases)
│   ├── dolt-server.log         Server log
│   └── dolt.pid                Server PID file
├── deacon/                     Deacon workspace
│   └── dogs/<name>/            Dog worker directories
├── mayor/                      Mayor agent home
│   ├── town.json               Town configuration
│   ├── rigs.json               Rig registry
│   ├── daemon.json             Daemon patrol config
│   └── accounts.json           Claude Code account management
├── settings/                   Town-level settings
│   ├── config.json             Town settings (agents, themes)
│   └── escalation.json         Escalation routes and contacts
├── directives/                 Town-level role directives (operator policy)
│   └── <role>.md               Markdown injected at prime time
├── formula-overlays/           Town-level formula overlays
│   └── <formula>.toml          TOML step overrides (replace/append/skip)
├── config/
│   └── messaging.json          Mail lists, queues, channels
└── <rig>/                      Project container (NOT a git clone)
    ├── config.json             Rig identity and beads prefix
    ├── .repo.git/              Shared BARE repo — worktree base for refinery
    │                           and polecats (created by `gt rig add`)
    ├── directives/             Rig-level role directives (overrides town)
    │   └── <role>.md
    ├── formula-overlays/       Rig-level formula overlays (full precedence)
    │   └── <formula>.toml
    ├── mayor/rig/              Canonical clone (beads live here, NOT an agent)
    │   └── .beads/             Rig-level beads (redirected to Dolt)
    ├── refinery/               Refinery agent home
    │   └── rig/                Worktree from .repo.git
    ├── witness/                Witness agent home (no clone)
    ├── crew/                   Crew parent
    │   └── <name>/             Human workspaces (full clones)
    └── polecats/               Polecats parent
        └── <name>/<rigname>/   Worker worktrees from .repo.git
```

**A rig lives at `<townRoot>/<rigName>` — directly at the town root.** gt derives
every rig path as `filepath.Join(townRoot, rigName)`
(`internal/rig/manager.go:179`, `:334`, `:1449`; `internal/rig/types.go:118`).
There is no `rigs/` container in the rig-management layer. Two peripheral paths
do tolerate one as a compatibility fallback — bare-repo discovery
(`internal/rig/repo.go:30`) and cwd→rig detection
(`internal/workspace/rig.go:66`) — but name→path resolution does not, so a
`rigs/`-based layout is not a supported arrangement.

A rig exists because it is **registered in `mayor/rigs.json`**, not because a
directory is present: `DiscoverRigs` iterates the config map and never scans the
filesystem (`internal/rig/manager.go:132`).

**Note**: No per-directory CLAUDE.md or AGENTS.md is created. Only `~/gt/CLAUDE.md`
(town-root identity anchor) exists on disk. Full context is injected by `gt prime`
via SessionStart hook.

### Worktree Architecture

Polecats and refinery are git worktrees, not full clones. This enables fast spawning
and shared object storage.

**The worktree base is the shared bare repo `<rig>/.repo.git`, not `mayor/rig`.**
`gt rig add` clones it at `internal/rig/manager.go:396` and cuts the refinery
worktree from it at `:704`. Polecats resolve their base through
`rig.FindBareRepo` (`internal/polecat/manager.go:451`), which checks
`.repo.git` then `repo.git` (`internal/rig/repo.go:12`). `mayor/rig` is only the
fallback when no bare repo is found, and the code labels it *"legacy
architecture"* (`internal/polecat/manager.go:455`); with neither present, polecat
creation fails with `"no repo base found (neither .repo.git/repo.git nor
mayor/rig exists)"` (`:458`).

The bare repo is shared deliberately: it is what lets the refinery see polecat
branches without a remote round-trip (`internal/rig/manager.go:696-698`). The
mayor clone is kept separate — a real clone, `--reference`d off the bare repo to
avoid a second download — precisely because it does *not* need branch visibility
(`:486-489`).

Crew workspaces (`crew/<name>/`) are full git clones for human developers who need
independent repos. Polecat sessions are ephemeral and benefit from worktree efficiency.

### Path Resolution and Symlinks

gt resolves rig paths **logically**, not physically. `workspace.Find` walks
upward from a start directory using `filepath.Abs` only and deliberately does
not canonicalise — the comment at `internal/workspace/find.go:32` reads *"Does
not resolve symlinks to stay consistent with `os.Getwd()`."* Rig existence is
checked with `os.Stat` everywhere (`internal/rig/manager.go:182`, `:337`,
`:1451`; `internal/rig/repo.go:25`; `internal/polecat/manager.go:483`), never
`os.Lstat`.

Because `os.Stat` follows symlinks, **a symlink at `<townRoot>/<rigName>`
satisfies every existence check gt performs.** Rig loading, polecat creation,
worktree cutting and beads resolution all work through one. This is easy to
mistake for support. It is not — it is transparency, and it stops at exactly one
class of operation: **turning a path into a key.**

The only place gt intentionally canonicalises is the tmux socket name, where
`townSocketName` calls `filepath.EvalSymlinks(townRoot)` before hashing
(`internal/session/registry.go:164`) so that two spellings of one town share a
socket. tmux *session* names never touch the filesystem at all — they are
derived from the rig's beads prefix (`internal/session/names.go:30-47`), so
session naming is immune to layout entirely.

**The trap: Claude Code transcript directories.** Claude Code stores transcripts
at `~/.claude/projects/<slug>/`, where the slug is the agent's working directory
with path separators replaced by `-`. gt recomputes that slug to find them:

```go
// internal/agentlog/claudecode.go:102
abs, _ := filepath.Abs(workDir)                    // Abs — NOT EvalSymlinks
hash := strings.ReplaceAll(filepath.ToSlash(abs), "/", "-")
```

gt feeds this the *logical* path derived from `rig.Path`. Claude Code keys on
its own working directory, which the OS reports *physically*. **If any component
of the rig path is a symlink, the two spellings differ and the lookup silently
finds nothing** — `waitForNewestJSONL` times out after 30s and loops forever
without surfacing an error (`internal/agentlog/claudecode.go:71-81`).

Two consequences worth knowing before you design a layout:

1. **Do not place rigs behind symlinks.** Everything will appear to work; only
   transcript-dependent features (agent logging, session liveness, cost
   attribution) will quietly return nothing.
2. **The slug is implemented twice and the copies disagree.**
   `internal/agentlog/claudecode.go:114` replaces `/` only;
   `internal/cmd/costs.go:690-691` replaces `/` *and* `_`. Claude Code's real
   encoding also collapses `_` and `.` to `-`. Since rig names may legally
   contain underscores — and `internal/rig/manager.go:313` actively recommends
   them when a name is otherwise invalid — `agentlog` computes a non-existent
   directory for any underscore-named rig.

### Rig Detection and the `rig.json` Marker

`workspace.FindRigFromCwd` (`internal/workspace/rig.go:26`) maps a working
directory back to a rig in two passes: first walking upward looking for a
`rig.json` marker (`rig.go:46-56`), then falling back to splitting the path
relative to the town root, skipping a leading `rigs/` component if present
(`rig.go:66-72`).

**`gt rig add` never creates `rig.json`.** The marker is only ever read
(`internal/workspace/rig.go:48`, `internal/cmd/rig_detect.go:101`,
`internal/doctor/role_beads_check.go:63`), so pass 1 fires only for rigs
assembled by something other than gt. For gt-created rigs, detection lands on
pass 2 and is accepted on the presence of `config.json`
(`internal/cmd/rig_detect.go:104`).

## Storage Layer: Dolt SQL Server

All beads data is stored in a single Dolt SQL Server process per town. There is
no embedded Dolt fallback — if the server is down, `bd` fails fast with a clear
error pointing to `gt dolt start`.

```
┌─────────────────────────────────┐
│  Dolt SQL Server (per town)     │
│  Port 3307, managed by daemon   │
│  Data: ~/gt/.dolt-data/         │
└──────────┬──────────────────────┘
           │ MySQL protocol
    ┌──────┼──────┬──────────┐
    │      │      │          │
  USE hq  USE gastown  USE beads  ...
```

Each rig database is a subdirectory under `.dolt-data/`. The daemon monitors
the server on every heartbeat and auto-restarts on crash.

For write concurrency, all agents write directly to `main` using transaction
discipline (`BEGIN` / `DOLT_COMMIT` / `COMMIT` atomically). This eliminates
branch proliferation and ensures immediate cross-agent visibility.

See [dolt-storage.md](dolt-storage.md) for full details.

## Beads Routing

The `routes.jsonl` file maps issue ID prefixes to rig locations (relative to town root):

```jsonl
{"prefix":"hq-","path":"."}
{"prefix":"gt-","path":"gastown/mayor/rig"}
{"prefix":"bd-","path":"beads/mayor/rig"}
```

Routes point to `mayor/rig` because that's where the canonical `.beads/` lives.
This enables transparent cross-rig beads operations:

```bash
bd show hq-mayor    # Routes to town beads (~/.gt/.beads)
bd show gt-xyz      # Routes to gastown/mayor/rig/.beads
```

## Beads Redirects

Worktrees (polecats, refinery, crew) don't have their own beads databases. Instead,
they use a `.beads/redirect` file that points to the canonical beads location:

```
polecats/alpha/.beads/redirect → ../../mayor/rig/.beads
refinery/rig/.beads/redirect   → ../../mayor/rig/.beads
```

`ResolveBeadsDir()` follows redirect chains (max depth 3) with circular detection.
This ensures all agents in a rig share a single beads database via the Dolt server.

## Merge Queue: Batch-then-Bisect

The refinery processes MRs through a batch-then-bisect merge queue (Bors-style).
This is a core capability, not a pluggable strategy.

### How It Works

```
MRs waiting:  [A, B, C, D]
                    ↓
Batch:        Rebase A..D as a stack on main
                    ↓
Test tip:     Run tests on D (tip of stack)
                    ↓
If PASS:      Fast-forward merge all 4 → done
If FAIL:      Binary bisect → test B (midpoint)
                    ↓
              If B passes: C or D broke it → bisect [C,D]
              If B fails:  A or B broke it → bisect [A,B]
```

### Implementation Phases

| Phase | Bead | What | Status |
|-------|------|------|--------|
| 1: GatesParallel | gt-8b2i | Run test + lint concurrently per MR | In progress |
| 2: Batch-then-bisect | gt-i2vm | Bors-style batching with binary bisect | Blocked by Phase 1 |
| 3: Pre-verification | gt-lu84 | Polecats run tests before MR submission | Blocked by Phase 2 |

Gates (test command, lint, etc.) are pluggable. The batching strategy is core.

Design doc: produced by gt-yxx0 review.

## Polecat Lifecycle: Self-Managed Completion

Polecats manage their own lifecycle end-to-end. The Witness observes but does NOT
gate completion. This prevents the Witness from becoming a bottleneck.

### Polecat Completion Flow

```
Polecat finishes work
  → Push branch to remote
  → Submit MR (bd update --mr-ready)
  → Update bead status
  → Tear down worktree
  → Go idle (available for next assignment)
```

The Witness monitors for stuck/zombie polecats (no activity for extended period)
and nudges or escalates. It does NOT process completion — that's the polecat's job.

Design bead: gt-0wkk.

## Data Plane Lifecycle

All beads data flows through a six-stage lifecycle managed by Dogs:

```
CREATE → LIVE → CLOSE → DECAY → COMPACT → FLATTEN
  │        │       │        │        │          │
  Dolt   active   done   DELETE   REBASE     SQUASH
  commit  work    bead    rows    commits    all history
                         >7-30d  together   to 1 commit
```

Stages 1-3 are automated today. Stages 4-6 are being shipped via Dog automation
(gt-at0i Reaper DELETE, gt-l8dc Compactor REBASE, gt-emm4 Doctor gc).

See [dolt-storage.md](dolt-storage.md) for full details.

## Deployment Artifacts

Gas Town and Beads are distributed through multiple channels. Tag pushes (`v*`)
trigger GitHub Actions release workflows that build and publish everything.

### Gas Town (`gt`)

| Channel | Artifact | Trigger |
|---------|----------|---------|
| **GitHub Releases** | Platform binaries (darwin/linux/windows, amd64/arm64) + checksums | GoReleaser on tag push |
| **Homebrew** | `brew install steveyegge/gastown/gt` — formula auto-updated on release | `update-homebrew` job pushes to `steveyegge/homebrew-gastown` |
| **npm** | `npx @gastown/gt` — wrapper that downloads the correct binary | OIDC trusted publishing (no token) |
| **Local build** | `go build -o $(go env GOPATH)/bin/gt ./cmd/gt` | Manual |

### Beads (`bd`)

| Channel | Artifact | Trigger |
|---------|----------|---------|
| **GitHub Releases** | Platform binaries + checksums | GoReleaser on tag push |
| **Homebrew** | `brew install steveyegge/beads/bd` | `update-homebrew` job |
| **npm** | `npx @beads/bd` — wrapper that downloads the correct binary | OIDC trusted publishing (no token) |
| **PyPI** | `beads-mcp` — MCP server integration | `publish-pypi` job with `PYPI_API_TOKEN` secret |
| **Local build** | `go build -o $(go env GOPATH)/bin/bd ./cmd/bd` | Manual |

### npm Authentication

Both repos use **OIDC trusted publishing** — no `NPM_TOKEN` secret needed.
Authentication is handled by GitHub's OIDC provider. The workflow needs:

```yaml
permissions:
  id-token: write  # Required for npm trusted publishing
```

Configure on npmjs.com: Package Settings → Trusted Publishers → link to the
GitHub repo and `release.yml` workflow file.

### What the binary embeds

The Go binary is the primary distribution vehicle. It embeds:
- **Role templates** — Agent priming context, served by `gt prime`
- **Formula definitions** — Workflow molecules, served by `bd mol`
- **Doctor checks** — Health diagnostics, including migration checks
- **Default configs** — `daemon.json` lifecycle defaults, operational thresholds

This means upgrading the binary automatically propagates most fixes. Files that
are NOT embedded (and require `gt doctor` or `gt upgrade` to update):
- Town-root `CLAUDE.md` (created at `gt install` time)
- `daemon.json` patrol entries (created at install, extended by `EnsureLifecycleDefaults`)
- Claude Code hooks (`.claude/settings.json` managed sections)
- Dolt schema (migrations run on first `bd` command after upgrade)

## Role Directives and Formula Overlays

Operators can customize agent behavior at the town or rig level without
modifying the Go binary or embedded templates. This follows the property layer
model (rig > town > system) and the hooks override precedent.

### Role Directives

Per-role Markdown files injected during `gt prime`, after the role template but
before context files and handoff content. Operator policy that overrides formula
instructions where they conflict.

```
~/gt/directives/<role>.md              # Town-level (all rigs)
~/gt/<rig>/directives/<role>.md        # Rig-level
```

Both levels concatenate (rig content appears last and wins conflicts).
Implemented in `internal/config/directives.go` (`LoadRoleDirective`),
integrated via `outputRoleDirectives()` in `internal/cmd/prime_output.go`.

### Formula Overlays

Per-formula TOML files that modify individual steps. Applied post-parse before
rendering in `showFormulaStepsFull()`.

```
~/gt/formula-overlays/<formula>.toml   # Town-level
~/gt/<rig>/formula-overlays/<formula>.toml  # Rig-level (full precedence)
```

Rig-level overlays fully replace town-level (not merged). Three override modes:

| Mode | Effect |
|------|--------|
| `replace` | Swap the step description entirely |
| `append` | Add text after the existing step description |
| `skip` | Remove the step (dependents inherit its needs) |

Implemented in `internal/formula/overlay.go` (`LoadFormulaOverlay`,
`ApplyOverlays`). `gt doctor` validates overlay step IDs against current
formula definitions and can auto-fix stale references.

See [directives-and-overlays.md](directives-and-overlays.md) for the full
reference with examples and design rationale.

## See Also

- [dolt-storage.md](dolt-storage.md) - Dolt storage architecture
- [reference.md](../reference.md) - Command reference
- [directives-and-overlays.md](directives-and-overlays.md) - Directives and overlays reference
- [molecules.md](../concepts/molecules.md) - Workflow molecules
- [identity.md](../concepts/identity.md) - Agent identity and BD_ACTOR
