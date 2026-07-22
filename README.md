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
- **Safe skill actions** - open, reinstall/update, remove, prune orphaned locks, or run bulk updates/removals with confirmation prompts.
- **Discover more from a source** - scan local checkouts or GitHub skill sources to find skills you have not installed yet.
- **Find and install new skills** - search skills.sh from inside LazySkills, preview the install command, then install to project or global scope with confirmation.

## 🛠️ Install

Recommended for macOS and Linux:

```bash
curl -fsSL https://lazyskills.sh/install | sh
```

Or use Homebrew:

```bash
brew install --cask alvinunreal/tap/lazyskills
```

Windows:

```powershell
irm https://lazyskills.sh/install.ps1 | iex
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
| `n` | Find new skills from skills.sh |
| `space` | Select a skill for bulk actions |
| `u` | Reinstall/update selected skill |
| `x` | Remove selected skill |
| `?` | Show the full keymap |

LazySkills previews actions before running them, and destructive actions require confirmation.

For agent-friendly registry search without opening the TUI:

```bash
lazyskills find --json "browser automation"
```

### Restore from lock files

Restore skills that are recorded in the project or global lock file but missing from disk:

```bash
lazyskills restore --global
lazyskills restore --project
lazyskills restore --all
```

Pass skill names to restore only those entries, or `--yes` to accept the preview non-interactively:

```bash
lazyskills restore --project code-review tdd
lazyskills restore --global --yes
```

Without a scope flag, restore checks both scopes. Before each install it rescans and revalidates that skill’s lock identity and restore command, then aborts without running remaining commands if the skill disappeared, its lock metadata changed, or its command changed—reporting any skills already restored. That narrows races with concurrent installs; it does not make restore fully race-free.

## 🔄 Updates

LazySkills can check for new releases and show the command to update your installation manually.

### CLI Update

Run the update command directly from your terminal to check for updates and get upgrade instructions:

```bash
lazyskills update
```

Options:
* `--check`: Query if a newer version is available. Exits 0 if status is checked cleanly.
* `--print-command`: Print the command required to upgrade on your channel (e.g. `brew upgrade`) and exit.

### TUI Update

When a newer version of LazySkills is available, an update notification will appear in the TUI footer:
`· U update (vX.Y.Z available)`

Press **`U`** (capital U) from the inventory view to open the update info modal, which will display the release details and the specific terminal command you need to run to upgrade manually.

## ⭐ Star for a chance to win stickers

<div align="center">

<img src="assets/stickers.webp" alt="Lazy Skills stickers" width="100%">

</div>

There are Lazy Skills stickers. Star the repo and you're entered into a draw to win some, shipped worldwide at no cost to you.

Winners are drawn from stargazers and announced in the [sticker giveaway issue](https://github.com/alvinunreal/lazyskills/issues/2), tagged by their GitHub username, so you'll get a notification if it's you. Shipping details are arranged privately from there.

## 📄 License

MIT - see [LICENSE](LICENSE).
