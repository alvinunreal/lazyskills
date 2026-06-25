<div align="center">

<img src="assets/lazyskills-wide.svg" alt="Lazy Skills" width="640">

# Lazy Skills

**Blazing-fast mission control for agent skills.**

[![CI](https://img.shields.io/github/actions/workflow/status/alvinunreal/lazyskills/ci.yml?branch=main&style=flat-square&label=CI&labelColor=0f172a&color=3dbbff)](https://github.com/alvinunreal/lazyskills/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/alvinunreal/lazyskills?style=flat-square)](https://goreportcard.com/report/github.com/alvinunreal/lazyskills)
[![Release](https://img.shields.io/github/v/release/alvinunreal/lazyskills?style=flat-square&labelColor=0f172a&color=ff79f2)](https://github.com/alvinunreal/lazyskills/releases/latest)
[![Go](https://img.shields.io/github/go-mod/go-version/alvinunreal/lazyskills?style=flat-square&logo=go&logoColor=white&label=Go&labelColor=0f172a&color=3dbbff)](go.mod)
[![License](https://img.shields.io/badge/license-MIT-b253f5?style=flat-square&labelColor=0f172a)](LICENSE)
[![Stars](https://img.shields.io/github/stars/alvinunreal/lazyskills?style=flat-square&labelColor=0f172a&color=556bf4)](https://github.com/alvinunreal/lazyskills/stargazers)

<img src="assets/banner.webp" alt="Lazy Skills - mission control for agent skills. Stay lazy." width="100%">

_"I don't debug broken skills by hand - I have a TUI to be disappointed for me."_

</div>

## 🌴 Overview

LazySkills is a blazing-fast terminal UI for managing agent skills. It gives you one place to see what is installed, which agents can use each skill, why visibility may be broken, and what actions are safe to run next.

<div align="center">

<img src="assets/demo.gif" alt="Lazy Skills terminal UI demo" width="100%">

</div>

## ✨ Features

- **See every skill in one place** - project, global, universal, and agent-specific skills in a single TUI.
- **Check agent visibility** - switch agents and see which skills are actually usable by Claude Code, OpenCode, Codex, Cursor, Gemini CLI, and many more.
- **Spot broken installs fast** - highlights missing `SKILL.md`, invalid frontmatter, broken symlinks, missing lock entries, and ghost agent skills.
- **Preview before you act** - inspect metadata, rendered skill content, and the exact command LazySkills is about to run.
- **Bundle project skills safely** - export a reproducible project bundle and preview import plans before confirming any install changes.
- **Safe skill actions** - open, reinstall/update, remove, prune orphaned locks, or run bulk updates/removals with confirmation prompts.
- **Discover more from a source** - scan local checkouts or GitHub skill sources to find skills you have not installed yet.

## 🛠️ Install

Recommended for macOS and Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/alvinunreal/lazyskills/main/scripts/install.sh | sh
```

Or use Homebrew:

```bash
brew install --cask alvinunreal/tap/lazyskills
```

Windows:

```powershell
irm https://raw.githubusercontent.com/alvinunreal/lazyskills/main/scripts/install.ps1 | iex
```

Go users:

```bash
go install github.com/alvinunreal/lazyskills/cmd/lazyskills@latest
```

Then launch it:

```bash
lazyskills
```

## 🚀 Getting started

Run LazySkills from a project:

```bash
lazyskills
```

Start with the left pane. It groups skills by source, so you can quickly see what came from the same repo, local folder, or custom install.

Useful keys:

| Key | What it does |
| --- | --- |
| `/` | Search skills |
| `a` | Cycle through agent visibility filters |
| `f` | Toggle project/global/all scopes |
| `enter` | Open details for a skill or source |
| `c` | Show available actions |
| `space` | Select a skill for bulk actions |
| `u` | Reinstall/update selected skill |
| `x` | Remove selected skill |
| `?` | Show the full keymap |

LazySkills previews actions before running them, and destructive actions require confirmation.

Use the action picker (`c`) on the main screen to export a project skill bundle to `.lazyskills/skills.bundle.json` or import one with a dry-run preview before confirming.

## 🔄 Updates

Keep LazySkills up to date using the built-in update command or the TUI interface.

### CLI Update

Run the update command directly from your terminal:

```bash
lazyskills update
```

Options:
* `--check`: Query if a newer version is available without downloading or printing commands. Exits 0.
* `--print-command`: Print the command required to upgrade on your channel (e.g. `brew upgrade`) and exit.
* `--yes`: Automatically perform the update if supported by the installation channel (direct Unix manual binary installations only).

### TUI Update

When a newer version of LazySkills is available, an update notification will automatically appear in the TUI footer:
`· U update (vX.Y.Z available)`

Press **`U`** (capital U) from the inventory view to open the update flow:
* For **manual binary installs** on macOS/Linux, you can perform the update directly inside the TUI.
* For **managed package installs** (like Homebrew, Scoop, WinGet, deb, or rpm packages), the TUI will display the specific terminal command you need to run to upgrade.

## ⭐ Star for a chance to win stickers

<div align="center">

<img src="assets/stickers.webp" alt="Lazy Skills stickers" width="100%">

</div>

There are Lazy Skills stickers. Star the repo and you're entered into a draw to win some, shipped worldwide at no cost to you.

Winners are drawn from stargazers and announced in the [sticker giveaway issue](https://github.com/alvinunreal/lazyskills/issues/2), tagged by their GitHub username, so you'll get a notification if it's you. Shipping details are arranged privately from there.

## 📄 License

MIT - see [LICENSE](LICENSE).
