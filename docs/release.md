# Release Runbook

This document describes how to release LazySkills. It is written for future AI agents and maintainers who need to run the release safely and repeatably.

## Current release setup

LazySkills releases are built with GoReleaser from Git tags. The release workflow publishes GitHub Release assets and package-manager manifests.

The current supported channels are:

- GitHub Release archives for macOS, Linux, and Windows.
- `checksums.txt` for release asset verification.
- Homebrew cask in `alvinunreal/homebrew-tap`.
- Scoop manifest in `alvinunreal/scoop-bucket`.
- Linux `.deb` packages for amd64 and arm64.
- Linux `.rpm` packages for amd64 and arm64.
- Go installation through `go install github.com/alvinunreal/lazyskills/cmd/lazyskills@latest`.
- Windows PowerShell installer through `scripts/install.ps1`.
- macOS/Linux installer through `scripts/install.sh`.
- Winget update workflow through `.github/workflows/winget.yml`.

The first successful public release is `v0.1.1`. The `v0.1.0` tag exists, but that release attempt failed in CI. Do not reuse or move it.

## Required repository secrets

These secrets must exist in the main repository, `alvinunreal/lazyskills`, under **Settings → Secrets and variables → Actions**.

| Secret | Purpose |
| --- | --- |
| `HOMEBREW_TAP_GITHUB_TOKEN` | Pushes the Homebrew cask to `alvinunreal/homebrew-tap`. |
| `SCOOP_BUCKET_GITHUB_TOKEN` | Pushes the Scoop manifest to `alvinunreal/scoop-bucket`. |
| `WINGET_TOKEN` | Opens or updates Winget manifest pull requests. |

The Homebrew and Scoop tokens need write access to their target repositories. The Winget token should be a classic GitHub token with `public_repo` scope.

## Pre-release checks

Start from a clean working tree unless the user explicitly asks to include current local changes in the release.

Run these commands:

```bash
git status --short
git diff --stat
git log --oneline -10
go test ./...
go build -o /tmp/lazyskills-test ./cmd/lazyskills
/tmp/lazyskills-test version
HOMEBREW_TAP_GITHUB_TOKEN=dummy SCOOP_BUCKET_GITHUB_TOKEN=dummy WINGET_TOKEN=dummy go run github.com/goreleaser/goreleaser/v2@latest check
```

The GoReleaser check must pass before tagging. If it fails, fix the configuration first. Do not tag a release with a known failing GoReleaser configuration.

## Choosing the next version

Use SemVer tags with a leading `v`.

Examples:

```text
v0.1.2
v0.2.0
v1.0.0
```

Use a patch version for fixes and packaging corrections. Use a minor version for new user-facing features while the project is pre-`v1.0.0`.

Before creating a tag, check both local and remote tags:

```bash
git tag --list "vX.Y.Z"
git ls-remote --tags origin "refs/tags/vX.Y.Z"
```

If the tag already exists, stop and ask for guidance. Do not force-push or move release tags unless the user explicitly requests destructive tag replacement.

## Commit and push changes

Review the full diff before committing. Check for secrets and unrelated files.

```bash
git status --short
git diff --stat
git diff
```

Stage only intended files. Then commit with a concise message that matches the repository style.

Example:

```bash
git add <intended-files>
git commit -m "Prepare vX.Y.Z release"
git push origin main
```

Wait for the normal `CI` workflow on `main` to pass before pushing the release tag when possible.

## Create the release

Create and push the tag:

```bash
git tag vX.Y.Z
git push origin vX.Y.Z
```

Pushing the tag starts `.github/workflows/release.yml`. The release workflow runs tests, builds with GoReleaser, uploads GitHub Release assets, and publishes Homebrew and Scoop metadata.

## Verify the release

Check workflow status:

```bash
gh run list --repo alvinunreal/lazyskills --limit 10
gh run watch <run-id> --repo alvinunreal/lazyskills --exit-status
```

Check the release and assets:

```bash
gh release view vX.Y.Z --repo alvinunreal/lazyskills --json url,tagName,name,isPrerelease,isDraft,assets
```

Expected assets include:

- `checksums.txt`
- `lazyskills_Darwin_arm64.tar.gz`
- `lazyskills_Darwin_x86_64.tar.gz`
- `lazyskills_Linux_arm64.tar.gz`
- `lazyskills_Linux_x86_64.tar.gz`
- `lazyskills_Windows_arm64.zip`
- `lazyskills_Windows_x86_64.zip`
- `lazyskills_X.Y.Z_linux_amd64.deb`
- `lazyskills_X.Y.Z_linux_arm64.deb`
- `lazyskills_X.Y.Z_linux_amd64.rpm`
- `lazyskills_X.Y.Z_linux_arm64.rpm`

Check package-manager outputs:

```bash
gh api repos/alvinunreal/homebrew-tap/contents/Casks/lazyskills.rb --jq '.html_url'
gh api repos/alvinunreal/scoop-bucket/contents/bucket/lazyskills.json --jq '.html_url'
```

## Smoke-test installation

After the release succeeds, test at least one archive or installer when practical.

Homebrew:

```bash
brew install --cask alvinunreal/tap/lazyskills
lazyskills version
```

Install script:

```bash
curl -fsSL https://raw.githubusercontent.com/alvinunreal/lazyskills/main/scripts/install.sh | sh
lazyskills version
```

Go install:

```bash
go install github.com/alvinunreal/lazyskills/cmd/lazyskills@latest
lazyskills version
```

Scoop:

```powershell
scoop bucket add alvinunreal https://github.com/alvinunreal/scoop-bucket
scoop install lazyskills
lazyskills version
```

## Winget notes

The Winget workflow is triggered by GitHub Release events. GitHub can suppress workflows that are triggered by releases created with `GITHUB_TOKEN`, so a successful GitHub Release does not always mean the Winget workflow ran.

If Winget does not run, check whether the package has already been bootstrapped in `microsoft/winget-pkgs`. The first Winget submission may need to be created manually. After the first accepted submission, future updates can be automated.

## Failure handling

If CI fails after pushing `main`, fix the issue in a follow-up commit and push again.

If the release workflow fails after pushing a tag:

1. Inspect the failed logs with `gh run view <run-id> --log-failed`.
2. Fix the issue in a new commit on `main`.
3. Push the fix.
4. Create a new patch tag, for example `v0.1.2`.

Do not move an already-pushed release tag unless the user explicitly asks for destructive tag replacement.

## Known historical issue

The `v0.1.0` release failed because a Linux CI runner exposed environment variables that affected agent detector tests. The fix isolated test environment variables in `internal/agents/agents_test.go`. Future test failures on CI should be treated as real until proven otherwise.
