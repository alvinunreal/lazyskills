<div align="center">

<img src="assets/lazyskills-wide.svg" alt="Lazy Skills" width="640">

# Lazy Skills

**Mission control for agent skills.**

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

LazySkills is a terminal UI for agent skills. It shows what skills are installed, which agents can see them, why visibility may be broken, and provides guarded actions for common skill operations, all from one screen.

## ⭐ Star for a chance to win stickers

<div align="center">

<img src="assets/stickers.webp" alt="Lazy Skills stickers" width="100%">

</div>

There are Lazy Skills stickers. Star the repo and you're entered into a draw to win some, shipped worldwide at no cost to you.

Winners are drawn from stargazers and announced in the [sticker giveaway issue](https://github.com/alvinunreal/lazyskills/issues/2), tagged by their GitHub username, so you'll get a notification if it's you. Shipping details are arranged privately from there.

## ✨ Features

✅ Fast, keyboard-driven Bubble Tea TUI  
✅ Project and global skill inventory  
✅ Agent compatibility diagnostics based on the upstream `skills` registry  
✅ Visibility reasons per agent, including missing links, broken symlinks, unsupported global installs, and ghost agent-directory skills  
✅ `SKILL.md` preview with safe terminal sanitization  
✅ Responsive panes and scrollable details  
✅ Source/repo group headers in the skill list  
✅ Guarded actions: refresh/rescan, open in `$EDITOR`, reinstall/update, remove, and bulk update/remove via the official `skills` CLI  
✅ Source/repo-aware selection for grouped workflows  
✅ Structured command execution with confirmation, captured output, and rescan after successful mutations

## 🛠️ Installation

### Homebrew

```bash
brew install --cask alvinunreal/tap/lazyskills
```

### macOS / Linux install script

```bash
curl -fsSL https://raw.githubusercontent.com/alvinunreal/lazyskills/main/scripts/install.sh | sh
```

The script installs to `/usr/local/bin` by default and verifies the release checksum. To install somewhere else:

```bash
curl -fsSL https://raw.githubusercontent.com/alvinunreal/lazyskills/main/scripts/install.sh | sh -s -- -b ~/.local/bin
```

### Windows PowerShell

```powershell
iwr https://raw.githubusercontent.com/alvinunreal/lazyskills/main/scripts/install.ps1 -useb | iex
```

The PowerShell installer installs to `%LOCALAPPDATA%\Programs\lazyskills\bin` and adds it to your user PATH.

### Windows Scoop

```powershell
scoop bucket add alvinunreal https://github.com/alvinunreal/scoop-bucket
scoop install lazyskills
```

### Windows WinGet

```powershell
winget install alvinunreal.lazyskills
```

WinGet availability may lag behind GitHub Releases because new versions are submitted to `microsoft/winget-pkgs` as pull requests.

### Go

```bash
go install github.com/alvinunreal/lazyskills/cmd/lazyskills@latest
```

### Nix experimental

Run directly from GitHub:

```bash
nix run github:alvinunreal/lazyskills
```

Or from a local checkout:

```bash
nix run
```

### Manual download

Download a binary archive from [GitHub Releases](https://github.com/alvinunreal/lazyskills/releases), verify it with `checksums.txt`, and place `lazyskills` on your PATH.

### Linux packages

LazySkills publishes `.deb` and `.rpm` packages for Linux amd64 and arm64 releases.

Debian / Ubuntu:

```bash
curl -LO https://github.com/alvinunreal/lazyskills/releases/download/v0.1.1/lazyskills_0.1.1_linux_amd64.deb
sudo apt install ./lazyskills_0.1.1_linux_amd64.deb
```

Fedora / RHEL / openSUSE:

```bash
curl -LO https://github.com/alvinunreal/lazyskills/releases/download/v0.1.1/lazyskills_0.1.1_linux_amd64.rpm
sudo dnf install ./lazyskills_0.1.1_linux_amd64.rpm
```

Use the `arm64` package instead on ARM Linux machines.

### Build from source

```bash
go build -o lazyskills ./cmd/lazyskills
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

Check the installed version:

```bash
lazyskills version
```

## ⌨️ Usage

```bash
lazyskills [--cwd <path>]
lazyskills scan --json [--cwd <path>]
```

Common TUI keys:

| Key | Action |
| --- | --- |
| `↑/↓`, `j/k` | Move selection (Inventory) or scroll (Metadata/Preview) |
| `gg` / `G` | Jump to top / bottom (also `home` / `end`) |
| `1` / `2` / `3` | Focus the Inventory / Metadata / Preview pane |
| `tab`, `shift+tab` | Cycle focus through panes |
| `←/→` | Jump source groups (Inventory) or switch pane focus |
| `h/l` (`-`/`+`) | Collapse / expand the current source group |
| `[` / `]` | Jump to previous / next source group |
| `f` / `F` | Cycle scope filter (All/Project/Global) / reset to All |
| `a` / `A` | Cycle agent filter / reset to all agents |
| `/` | Search skills |
| `space` | Mark/unmark skill for bulk actions |
| `s` | Mark all skills from the current source/repo |
| `o` | Open current skill in `$EDITOR` |
| `u` / `x` | Update / remove current skill (or marked skills if any) |
| `c` | Show guarded actions |
| `enter` | Open detail for the selection (runs the action in the command picker) |
| `d` | Discover available skills for the selected source |
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

## 🚢 Releases

LazySkills uses [GoReleaser](https://goreleaser.com/) to publish cross-platform binaries, checksums, GitHub Releases, and the Homebrew formula.

Release a new version by pushing a SemVer tag:

```bash
git tag v0.1.0
git push origin v0.1.0
```

The release workflow builds:

- macOS Intel and Apple Silicon
- Linux amd64 and arm64
- Windows amd64 and arm64
- Linux `.deb` and `.rpm` packages

Nix users can run LazySkills with `nix run github:alvinunreal/lazyskills` once the flake dependency hash is finalized.

Homebrew publishing requires the repository secret `HOMEBREW_TAP_GITHUB_TOKEN` with permission to push to `alvinunreal/homebrew-tap`.

Scoop publishing requires the repository secret `SCOOP_BUCKET_GITHUB_TOKEN` with permission to push to `alvinunreal/scoop-bucket`.

WinGet publishing requires the repository secret `WINGET_TOKEN`, a classic GitHub token with `public_repo` scope. The first WinGet submission may need to be created manually before automated updates are accepted.
