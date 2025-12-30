#!/bin/bash
set -euo pipefail

# Hook to automatically load AGENTS.md content at session start
# This implements the workaround from https://github.com/anthropics/claude-code/issues/6235

: "${CLAUDE_PROJECT_DIR:?CLAUDE_PROJECT_DIR is not set}"

AGENTS_MD_PATH="$CLAUDE_PROJECT_DIR/AGENTS.md"

if [ -f "$AGENTS_MD_PATH" ]; then
    echo "[INFO] Loading agent instructions from AGENTS.md..." >&2
    echo "" >&2
    cat "$AGENTS_MD_PATH"
else
    echo "[WARNING] AGENTS.md not found at: $AGENTS_MD_PATH" >&2
fi
