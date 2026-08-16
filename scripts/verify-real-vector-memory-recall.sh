#!/usr/bin/env bash

# Compatibility entry for the former provider-driven recall fixture. Memory is
# owner state; Application selects it before bootstrap and projections expose
# only persisted recall traces.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

GOCACHE="${GOCACHE:-/tmp/workorch-go-build}"
export GOCACHE

echo "=== Memory Owner recall and Runtime bootstrap verification ==="

go test ./internal/modules/blocks/memory/... -count=1
go test ./internal/modules/application/workorchd/biz/appservice/activationports \
  -run 'TestSelectedWorkspaceMemoryItems(LoadsDefaultProviderOnlyForImplicitSelection|ReadsExplicitIDsWithWorkspaceScope|SkipsSemanticRecallForEmptyOwnerInventory)$' \
  -count=1
go test ./internal/modules/application/workorchd/biz/appservice/activationcore \
  -run 'TestBuildActivationMemoryRecallTrace' \
  -count=1
go test ./internal/modules/application/workorchd/biz/appservice \
  -run 'TestReadRunSnapshotScopesMemoryRecallTracesByWorkspace' \
  -count=1
go test ./internal/modules/application/workorchd/service/server \
  -run 'TestMemoryRecallTraceEndpointsReturnFilteredTraces' \
  -count=1

echo "Memory Owner recall and Runtime bootstrap verification passed"
