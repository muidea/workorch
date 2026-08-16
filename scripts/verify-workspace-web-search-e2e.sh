#!/usr/bin/env bash

# The former deployed fixture generated a retired execution graph. Keep the
# public target as a deterministic owner/Runtime integration gate until a live
# provider fixture can emit the current semantic-plan and operations documents.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

"$ROOT_DIR/scripts/verify-workspace-web-search.sh"

cd "$ROOT_DIR"
go test ./internal/modules/application/workorchd/service/server \
  -run 'Test(WorkorchDActivationHTTPProjectsRequestID|WorkorchDUnsupportedMutatingAPINativeRoutesNotRegistered)$' \
  -count=1

echo "Web Search owner and Runtime integration verification passed"
