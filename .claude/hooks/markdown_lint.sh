#!/usr/bin/env bash

set -euo pipefail

# Ensure jq is available; if not, skip the hook gracefully
if ! command -v jq >/dev/null 2>&1; then
  exit 0
fi

# Check if the edited file is a Markdown file
if ! jq -re '.tool_input.file_path | test("\\.md$")' > /dev/null 2>&1; then
  exit 0
fi

make lint-markdown-fix
