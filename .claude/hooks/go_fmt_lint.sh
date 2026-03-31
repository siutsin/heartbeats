#!/usr/bin/env bash

set -euo pipefail

# Check if the edited file is a Go file
if ! jq -re '.tool_input.file_path | test("\\.go$")' > /dev/null 2>&1; then
  exit 0
fi

make fmt lint
