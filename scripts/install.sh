#!/usr/bin/env sh
set -eu

OWNER="alvinunreal"
REPO="lazyskills"
BINARY="lazyskills"
DEFAULT_BINDIR="/usr/local/bin"

usage() {
  cat <<EOF
Install lazyskills from GitHub Releases.

Usage:
  install.sh [-b <bindir>] [-v <version>]

Options:
  -b <bindir>   Install directory. Default: ${DEFAULT_BINDIR}
  -v <version>  Release version, for example v0.1.0. Default: latest.
  -h            Show this help.

Examples:
  curl -fsSL https://raw.githubusercontent.com/${OWNER}/${REPO}/main/scripts/install.sh | sh
  curl -fsSL https://raw.githubusercontent.com/${OWNER}/${REPO}/main/scripts/install.sh | sh -s -- -b ~/.local/bin
EOF
}

BINDIR="${BINDIR:-$DEFAULT_BINDIR}"
VERSION="${VERSION:-latest}"

while getopts "b:v:h" opt; do
  case "$opt" in
    b) BINDIR="$OPTARG" ;;
    v) VERSION="$OPTARG" ;;
    h) usage; exit 0 ;;
    *) usage; exit 1 ;;
  esac
done

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "error: $1 is required" >&2
    exit 1
  fi
}

need curl
need tar

prompt_star_repo() {
  if ! command -v gh >/dev/null 2>&1; then
    return 0
  fi

  if [ ! -r /dev/tty ] || [ ! -w /dev/tty ]; then
    return 0
  fi

  printf "Would you like to star ${OWNER}/${REPO} on GitHub? [Y/n] " >/dev/tty
  IFS= read -r answer </dev/tty || return 0

  case "$answer" in
    ""|[Yy]|[Yy][Ee][Ss])
      if gh repo star "${OWNER}/${REPO}" >/dev/null 2>&1; then
        echo "Starred ${OWNER}/${REPO}."
      else
        echo "Note: could not star ${OWNER}/${REPO} with GitHub CLI; continuing." >&2
      fi
      ;;
  esac
}

OS="$(uname -s)"
ARCH="$(uname -m)"

case "$OS" in
  Darwin) GOOS="Darwin" ;;
  Linux) GOOS="Linux" ;;
  *) echo "error: unsupported OS: $OS" >&2; exit 1 ;;
esac

case "$ARCH" in
  x86_64|amd64) GOARCH="x86_64" ;;
  arm64|aarch64) GOARCH="arm64" ;;
  *) echo "error: unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

ARCHIVE="${REPO}_${GOOS}_${GOARCH}.tar.gz"
BASE_URL="https://github.com/${OWNER}/${REPO}/releases"
if [ "$VERSION" = "latest" ]; then
  DOWNLOAD_URL="${BASE_URL}/latest/download/${ARCHIVE}"
  CHECKSUM_URL="${BASE_URL}/latest/download/checksums.txt"
else
  DOWNLOAD_URL="${BASE_URL}/download/${VERSION}/${ARCHIVE}"
  CHECKSUM_URL="${BASE_URL}/download/${VERSION}/checksums.txt"
fi

TMPDIR="$(mktemp -d)"
cleanup() { rm -rf "$TMPDIR"; }
trap cleanup EXIT INT TERM

echo "Downloading ${ARCHIVE}..."
curl -fsSL "$DOWNLOAD_URL" -o "$TMPDIR/$ARCHIVE"
curl -fsSL "$CHECKSUM_URL" -o "$TMPDIR/checksums.txt"

echo "Verifying checksum..."
if command -v sha256sum >/dev/null 2>&1; then
  (cd "$TMPDIR" && grep "  ${ARCHIVE}$" checksums.txt | sha256sum -c -)
elif command -v shasum >/dev/null 2>&1; then
  (cd "$TMPDIR" && grep "  ${ARCHIVE}$" checksums.txt | shasum -a 256 -c -)
else
  echo "warning: sha256sum or shasum not found; skipping checksum verification" >&2
fi

tar -xzf "$TMPDIR/$ARCHIVE" -C "$TMPDIR" "$BINARY"

if [ ! -d "$BINDIR" ]; then
  echo "Creating ${BINDIR}..."
  if mkdir -p "$BINDIR" 2>/dev/null; then
    :
  elif command -v sudo >/dev/null 2>&1; then
    sudo mkdir -p "$BINDIR"
  else
    echo "error: cannot create ${BINDIR}; choose another directory with -b" >&2
    exit 1
  fi
fi

TARGET="${BINDIR}/${BINARY}"
echo "Installing to ${TARGET}..."
if mv "$TMPDIR/$BINARY" "$TARGET" 2>/dev/null; then
  chmod 755 "$TARGET"
elif command -v sudo >/dev/null 2>&1; then
  sudo mv "$TMPDIR/$BINARY" "$TARGET"
  sudo chmod 755 "$TARGET"
else
  echo "error: cannot write to ${BINDIR}; choose another directory with -b" >&2
  exit 1
fi

echo "Installed: $($TARGET version | tr '\n' ' ' | sed 's/ $//')"

case ":$PATH:" in
  *":$BINDIR:"*) ;;
  *) echo "Note: ${BINDIR} is not on your PATH." ;;
esac

prompt_star_repo
