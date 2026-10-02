#!/bin/sh
# Installs the repo's git hooks into .git/hooks.
# Run from the repo root: `sh .githooks/install.sh` (or `make install-hooks`).

set -e

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SRC="$ROOT_DIR/.githooks/pre-commit"
DST_DIR="$ROOT_DIR/.git/hooks"
DST="$DST_DIR/pre-commit"

mkdir -p "$DST_DIR"
cp "$SRC" "$DST"
chmod +x "$DST"

echo "Pre-commit hook installed successfully"
