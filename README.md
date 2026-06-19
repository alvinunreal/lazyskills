<div align="center">

<img src="assets/lazyskills-wide.svg" alt="Lazy Skills" width="640">

# Lazy Skills

**Mission control for agent skills to _stay lazy._**

[![Go](https://img.shields.io/badge/Go-1.26-3dbbff?style=flat-square&logo=go&logoColor=white&labelColor=0f172a)](https://go.dev)
[![TUI](https://img.shields.io/badge/TUI-Bubble%20Tea-ff79f2?style=flat-square&labelColor=0f172a)](https://github.com/charmbracelet/bubbletea)
[![License](https://img.shields.io/badge/license-MIT-b253f5?style=flat-square&labelColor=0f172a)](LICENSE)
[![Status](https://img.shields.io/badge/status-early-556bf4?style=flat-square&labelColor=0f172a)](#)

<img src="assets/banner.webp" alt="Lazy Skills — mission control for agent skills. Stay lazy." width="100%">

_"I don't debug broken skills by hand - I have a TUI to be disappointed for me."_

</div>

## 🌴 Overview

LazySkills is a terminal UI for agent skills. It shows what skills are installed, which agents can see them, why visibility may be broken, and provides guarded actions for common skill operations — all from one screen.

It is designed to complement the official `skills` CLI, not replace it.

## ✨ Features

✅ Fast, keyboard-driven Bubble Tea TUI  
✅ Project and global skill inventory  
✅ Agent compatibility diagnostics based on the upstream `skills` registry  
✅ Visibility reasons per agent, including missing links, broken symlinks, unsupported global installs, and ghost agent-directory skills  
✅ `SKILL.md` preview with safe terminal sanitization  
✅ Responsive panes and scrollable details  
✅ Source/repo group headers in the skill list  
✅ Guarded actions — refresh/rescan, open in `$EDITOR`, reinstall/update, remove, and bulk update/remove via the official `skills` CLI  
✅ Source/repo-aware selection for grouped workflows  
✅ Structured command execution with confirmation, captured output, and rescan after successful mutations

## 🛠️ Install / build

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

## ⌨️ Usage

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

## ⚡ How actions work

LazySkills delegates install/update/remove behavior to the official `skills` CLI. It resolves commands as:

1. `skills` if available on `PATH`
2. `npx --yes skills` as fallback

Commands are executed as structured argv, not shell strings. Output is captured, sanitized, capped, and displayed in the TUI. Successful mutations trigger a rescan.

## 🤖 Supported agents

LazySkills mirrors the upstream `vercel-labs/skills` agent registry, including universal `.agents/skills` agents, agent-specific skill directories, global support, and installed-agent detection heuristics.

The registry is manually ported for now. A generated parity check is a planned follow-up.

## 🛟 Safety and limitations

- LazySkills does not reimplement remote clone/install/update/remove logic in Go.
- Search/marketplace flows are not implemented.
- `skills use` integration is not implemented.
- Agent cycling is functional but not yet a searchable picker.

## 🧪 Development

```bash
go test ./...
go build ./cmd/lazyskills
```
