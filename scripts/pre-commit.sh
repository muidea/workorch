#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

echo "=== gofmt ==="
make fmt

echo "=== fmt-check ==="
make fmt-check

echo "=== vet ==="
make vet

echo "pre-commit checks passed"
