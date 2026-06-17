# LazySkills

LazySkills is a terminal control room for agent skills. It shows what skills are installed, which agents can see them, why visibility may be broken, and provides guarded actions for common skill operations.

It is designed to complement the official `skills` CLI, not replace it.

## Features

- LazyGit-style Bubble Tea TUI
- Project and global skill inventory
- Agent compatibility diagnostics based on the upstream `skills` registry
- Visibility reasons per agent, including missing links, broken symlinks, unsupported global installs, and ghost agent-directory skills
- `SKILL.md` preview with safe terminal sanitization
- Responsive panes and scrollable details
- Source/repo group headers in the skill list
- Guarded actions:
  - refresh/rescan
  - open selected skill in `$EDITOR`
  - reinstall/update selected skill through the official `skills` CLI
  - remove selected skill through the official `skills` CLI
  - bulk reinstall/update or remove marked skills
- Source/repo-aware selection for grouped workflows
- Structured command execution with confirmation, captured output, and rescan after successful mutations

## Install / build

```bash
go build ./cmd/lazyskills
```

Then run:

```bash
./lazyskills
```

Or scan as JSON:

```bash
./lazyskills scan --json
./lazyskills scan --json --cwd /path/to/project
```

## Usage

```bash
lazyskills [--cwd <path>]
lazyskills scan --json [--cwd <path>]
```

Common TUI keys:

| Key | Action |
| --- | --- |
| `↑/↓`, `j/k` | Move selection |
| `space` | Mark/unmark skill for bulk actions |
| `s` | Mark all skills from the current source/repo |
| `o` | Open current skill in `$EDITOR` |
| `u` | Update current skill, or marked skills if any are selected |
| `x` | Remove current skill, or marked skills if any are selected |
| `tab`, `shift+tab`, `←/→` | Change project/global scope filter |
| `a` | Cycle agent filter |
| `A` | Reset agent filter to all agents |
| `/` | Search skills |
| `c` | Show guarded actions |
| `enter` | Run selected action when actions are visible |
| `r` | Refresh scan |
| `?` | Toggle help |
| `q` | Quit |

Mutating actions open a centered confirmation prompt. Press Enter for the default yes, or type `y`, `yes`, or the displayed phrase. Press `n` or Esc to cancel.

The skill list groups adjacent skills by source/repo when lock metadata is available. Skill details also show the exact folder and ref. Use `s` to mark every skill from the current source/repo, then `u` or `x` to update or remove that group with confirmation.

## How actions work

LazySkills delegates install/update/remove behavior to the official `skills` CLI. It resolves commands as:

1. `skills` if available on `PATH`
2. `npx --yes skills` as fallback

Commands are executed as structured argv, not shell strings. Output is captured, sanitized, capped, and displayed in the TUI. Successful mutations trigger a rescan.

## Supported agents

LazySkills mirrors the upstream `vercel-labs/skills` agent registry, including universal `.agents/skills` agents, agent-specific skill directories, global support, and installed-agent detection heuristics.

The registry is manually ported for now. A generated parity check is a planned follow-up.

## Safety and limitations

- LazySkills does not reimplement remote clone/install/update/remove logic in Go.
- Search/marketplace flows are not implemented.
- `skills use` integration is not implemented.
- Agent cycling is functional but not yet a searchable picker.

## Development

```bash
go test ./...
go build ./cmd/lazyskills
```
