#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

GOCACHE="${GOCACHE:-/tmp/workorch-go-build}"
export GOCACHE

echo "=== workspace file / diff / patch verification ==="

echo "-> FDP-E2E-04/07/08: governed file list/read/edit safety"
go test ./internal/modules/blocks/file/biz \
  -run 'TestRuntimeAdapter(ReadReturnsContentAndHash|ListSkipsGeneratedDirectories|RejectsPathEscape|RejectsSymlinkEscape|EditReturnsPatchPreviewWithoutWriting|EditRejectsAmbiguousMatch|EditRejectsBaseHashMismatch|EditUsesExplicitOccurrenceWithoutWriting|EditRejectsBinaryFile)' \
  -count=1

echo "-> FDP-E2E-01/02/03/05/06/07: patch apply, approval, conflict, revert"
go test ./internal/modules/kernel/orchestrator/biz \
  -run 'Test(RuntimeOrchestrator_SoftwareDevCodeAgentRepairsTestFailureAndDeliversDiff|RuntimeOrchestrator_StartRunDispatchesPatchApplyGraphNode|RuntimeOrchestrator_StartRunAppliesRuntimePatchEvidence|RuntimeOrchestrator_StartRunAppliesUnifiedDiffPatchEvidence|RuntimeOrchestrator_StartRunPersistsUnifiedDiffPatchEvidenceWithFileBackend|RuntimeOrchestrator_ResultArtifactsIncludePatchDiff|RuntimeOrchestrator_ResultManifestListsOnlyPatchTargets|RuntimeOrchestratorPublishesPatchPreviewEvent|RuntimeOrchestratorApplyRuntimePatchesPublishesAppliedAndFailedEvents|RuntimeOrchestratorRevertsAppliedPatch|RuntimeOrchestratorRevertRejectsHashDrift|RuntimeOrchestratorBlocksSensitivePatchUntilApproval|RuntimeOrchestratorAllowsSensitivePatchWithApprovedApprovalID|ApplyWorkspacePatchRejectsBaseHashMismatch|PatchPreviewDoesNotWriteFile|WorkspaceTargetPathRejectsSymlinkEscape|PatchApprovalReasonsRequireApprovalForLargeLockfileChange|PatchApprovalReasonsAllowSmallLockfileChange|PatchRevertApprovalReasonsRequireApprovalForSensitiveCreatedFile)' \
  -count=1

echo "-> FDP-E2E-05/06: patch events and read-model projection"
go test ./internal/pkg/events ./internal/modules/blocks/eventstream/biz ./internal/modules/application/workorchd/biz/appservice \
  -run 'Test(DecodeWorkorchEventReturnsTypedPatchEvent|DecodePatchPayloadRoundTrip|ServicePersistsPatchEventsFromHub|MapRunObservabilityProjectsPatchEventsIntoTimeline|ReadRunSnapshotWithBackendReturnsExecutionEvidence|ReadWorkspaceSnapshotWithRunSnapshotReusesProjectionSections)' \
  -count=1

echo "-> FDP-E2E-05/07: TUI and CLI patch evidence projection"
go test ./internal/services/workorch/service/tui ./internal/services/workorch/service/cli \
  -run 'Test(ModelWorkspaceContextPrefersFinalSummaryArtifactOutput|ModelWorkspaceContextHydratesFreshSnapshot|ModelResultArtifacts|ModelResultArtifactsHiddenWhenResultContainsArtifactIndex|PatchesForRunFiltersWorkspaceSnapshotPatches|NonEmptyApprovalIDPrefersApplyThenRevertApproval)$' \
  -count=1

echo "workspace file / diff / patch verification passed"
