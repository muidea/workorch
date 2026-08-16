#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"
source "$ROOT_DIR/scripts/lib/go-test-must-run.sh"

GOCACHE="${GOCACHE:-/tmp/workorch-go-build}"
export GOCACHE

echo "=== workspace file / diff / patch verification ==="

echo "-> File Owner: target discovery, create-only write, revision-bound replacement"
run_go_test ./internal/modules/blocks/file/biz \
  -run '^(TestRuntimeAdapterAdmissibilityPublishesCreateOnlyWriteAndRevisionBoundReplacement|TestRuntimeAdapterReadReturnsContentAndHash|TestRuntimeAdapterListSkipsGeneratedDirectories|TestRuntimeAdapterRejectsPathEscape|TestRuntimeAdapterRejectsSymlinkEscape|TestRuntimeAdapterEditRejectsAmbiguousMatch|TestRuntimeAdapterEditRejectsBaseHashMismatch|TestRuntimeAdapterEditRejectsBinaryFile)$' \
  -count=1

echo "-> File Owner: durable apply, replay proof, stale-revision rejection"
run_go_test ./internal/modules/blocks/file/biz \
  -run '^(TestRuntimeAdapterPatchApplyFileWriteWritesOnce|TestRuntimeAdapterPatchApplyFileWriteRejectsExistingTargetWithoutBaseHash|TestRuntimeAdapterPatchApplyFileWriteConfirmsSealedReplayAfterReceiptLoss|TestRuntimeAdapterPatchApplyReplayProofCoversAllDurableVariants|TestRuntimeAdapterPatchApplyRejectsBaseHashMismatch|TestRuntimeAdapterPatchApplyUnifiedDiffSuccess|TestRuntimeAdapterPatchApplyUnifiedDiffRejectsStaleContext)$' \
  -count=1

echo "-> SM-3/SM-4: sealed patch variants and restart/replay convergence"
run_go_test ./internal/modules/kernel/orchestrator/biz \
  -run '^(TestRuntimeFileBackedPatchVariantMatrix|TestRuntimeFileBackedPatchReplayProofRestartsAfterOwnerReceiptCASFailure|TestRuntimeFileBackedPatchVariantRestartAfterOwnerReceipt)$' \
  -count=1

echo "-> display-only projection: persisted patch evidence remains queryable"
run_go_test ./internal/modules/application/workorchd/biz/appservice \
  -run '^(TestListRunPatchesWithSessionPaginatesProjectedPatches|TestReadRunSnapshotWithBackendReturnsExecutionEvidence)$' \
  -count=1
run_go_test ./internal/services/workorch/service/cli \
  -run '^TestPatchesForRunFiltersWorkspaceSnapshotPatches$' \
  -count=1

echo "workspace file / diff / patch verification passed"
