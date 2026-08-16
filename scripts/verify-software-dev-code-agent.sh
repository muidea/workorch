#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

GOCACHE="${GOCACHE:-/tmp/workorch-go-build}"
export GOCACHE

echo "=== software-dev Runtime operation verification ==="

echo "-> current template, Soul and Runtime document contracts"
go test ./internal/pkg/workorchdefaults ./internal/pkg/workorchruntimecontract -count=1

echo "-> Application builds a definition seed without claiming owner feasibility"
go test ./internal/modules/application/workorchd/biz/appservice/activationcore \
  -run 'TestRuntimeBootstrap(AcceptsDefinitionSeedWithoutPrecompiledProof|DoesNotClaimShellOwnerRecipeFeasibility|SoftwareDevProfileIncludesManagedValidation)$' \
  -count=1

echo "-> File Owner enforces create-only writes and revision-bound replacement"
go test ./internal/modules/blocks/file/biz \
  -run 'TestRuntimeAdapter(AdmissibilityPublishesCreateOnlyWriteAndRevisionBoundReplacement|PatchApplyFileWriteRejectsExistingTargetWithoutBaseHash|PatchApplyRejectsBaseHashMismatch)$' \
  -count=1

echo "-> Runtime replaces existing targets and survives receipt-bound restart"
go test ./internal/modules/kernel/orchestrator/biz \
  -run 'TestRuntimeFileBackedPatchVariant(Matrix|RestartAfterOwnerReceipt)$' \
  -count=1

echo "software-dev Runtime operation verification passed"
