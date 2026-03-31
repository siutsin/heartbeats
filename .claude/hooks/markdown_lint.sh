#!/usr/bin/env bash

set -euo pipefail

# Check if the edited file is a Markdown file
if ! jq -re '.tool_input.file_path | test("\\.md$")' > /dev/null 2>&1; then
  exit 0
fi

make lint-markdown-fix
