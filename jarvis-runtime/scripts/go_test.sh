#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: go_test.sh <workspace_path>" >&2
  exit 2
fi

WS="$1"
cd "$WS"
go test ./...
