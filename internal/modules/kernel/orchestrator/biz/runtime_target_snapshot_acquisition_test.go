package orchestrator

import (
	"context"
	"os"
	"testing"

	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	filecommon "github.com/muidea/workorch/internal/modules/blocks/file/pkg/common"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeManagedTargetSnapshotAcquisitionBuildsReadOnlyGraph(t *testing.T) {
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"nextAction": map[string]any{"kind": runtimemodel.NextActionTypeAcquireTargetSnapshot},
		},
		"convergenceContract": map[string]any{
			"state": convergenceStateNeedEvidence,
		},
		"repairActionPackage": map[string]any{
			"repairTargetSet": []string{"target.txt", "cwi-ignored", "deliverable:source_change"},
			"requiredOutput": map[string]any{
				"mustAcquireCurrentTargetSnapshot":   true,
				"mustUseReadOnlySnapshotAcquisition": true,
				"mustIncludeStateChangingNode":       false,
				"acceptedKinds":                      []string{"file/list", "file/read", "git/read"},
			},
		},
	}

	output, ok := runtimeManagedTargetSnapshotAcquisitionOutput(input)
	if !ok {
		t.Fatal("runtimeManagedTargetSnapshotAcquisitionOutput ok = false, want true")
	}
	if output["runtimeManagedTargetSnapshotAcquisition"] != true {
		t.Fatalf("output = %+v, want runtime managed marker", output)
	}
	graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph])
	if !ok || len(graph.Nodes) != 1 {
		t.Fatalf("graph = %+v/%v, want one read target node", graph, ok)
	}
	node := graph.Nodes[0]
	if node.Kind != "file/read" || node.Input["path"] != "target.txt" || node.Input["targetRef"] != "target.txt" {
		t.Fatalf("node = %+v, want read-only target.txt file/read", node)
	}
	if node.Input["runtimeManagedAcquisition"] != true || node.Input["acquisitionKind"] != "target_snapshot" {
		t.Fatalf("node input = %+v, want target snapshot acquisition metadata", node.Input)
	}
	diagnostics := mapSliceFromValue(output["authorityDiagnostics"])
	if len(diagnostics) != 1 || stringFromMap(diagnostics[0], "code") != "target_snapshot_acquisition_requested" {
		t.Fatalf("authorityDiagnostics = %+v, want target_snapshot_acquisition_requested", diagnostics)
	}
}

func TestRuntimeManagedTargetSnapshotAcquisitionUsesCurrentFocusOnly(t *testing.T) {
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"nextAction": map[string]any{"kind": runtimemodel.NextActionTypeAcquireTargetSnapshot},
			"currentFocus": map[string]any{
				"id":         "cwi-current",
				"targetRefs": []string{"current/target.txt"},
			},
		},
		"convergenceContract": map[string]any{
			"state":            convergenceStateNeedEvidence,
			"runtimeStageMode": "target_snapshot_acquisition",
		},
		"repairActionPackage": map[string]any{
			"repairTargetSet": []string{"stale/unrelated.txt"},
			coverageWorkItemContextKey: map[string]any{
				"ledgerSummary": []map[string]any{{
					"id":         "cwi-stale",
					"targetRefs": []string{"stale/from-ledger.txt"},
				}, {
					"id":         "cwi-current",
					"targetRefs": []string{"current/target.txt"},
				}},
			},
			"requiredOutput": map[string]any{
				"mustAcquireCurrentTargetSnapshot":   true,
				"mustUseReadOnlySnapshotAcquisition": true,
				"mustIncludeStateChangingNode":       false,
				"acceptedKinds":                      []string{"file/read"},
			},
		},
	}

	output, ok := runtimeManagedTargetSnapshotAcquisitionOutput(input)
	if !ok {
		t.Fatal("runtimeManagedTargetSnapshotAcquisitionOutput ok = false, want current-focus read graph")
	}
	graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph])
	if !ok || len(graph.Nodes) != 1 {
		t.Fatalf("graph = %+v/%v, want one current focus read", graph, ok)
	}
	if got := graph.Nodes[0].Input["path"]; got != "current/target.txt" {
		t.Fatalf("read path = %v, want current focus target only", got)
	}
}

func TestRuntimeManagedTargetSnapshotAcquisitionDoesNotFallbackToStaleReadableRefsWhenCurrentFocusAbstract(t *testing.T) {
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"nextAction": map[string]any{"kind": runtimemodel.NextActionTypeAcquireTargetSnapshot},
			"currentFocus": map[string]any{
				"id":         "cwi-current",
				"targetRefs": []string{"cwi-current", "source_change"},
			},
		},
		"convergenceContract": map[string]any{
			"state":            convergenceStateNeedEvidence,
			"runtimeStageMode": "target_snapshot_acquisition",
		},
		"repairActionPackage": map[string]any{
			"repairTargetSet": []string{"stale/readable.txt"},
			"requiredOutput": map[string]any{
				"mustAcquireCurrentTargetSnapshot":   true,
				"mustUseReadOnlySnapshotAcquisition": true,
				"mustIncludeStateChangingNode":       false,
			},
		},
	}

	output, ok := runtimeManagedTargetSnapshotAcquisitionOutput(input)
	if !ok {
		t.Fatal("runtimeManagedTargetSnapshotAcquisitionOutput ok = false, want deterministic abstract-ref failure")
	}
	if output["status"] != "failed" || output[failureOutputKeyClass] != failureClassRuntimeContractMisalign {
		t.Fatalf("output = %+v, want contract misalignment failure", output)
	}
	if graph := output[stepOutputKeyExecutionGraph]; graph != nil {
		t.Fatalf("executionGraph = %+v, want no stale readable target fallback", graph)
	}
	refs := stringSliceFromValue(output["abstractTargetRefs"])
	if !stringSliceContains(refs, "cwi-current") || !stringSliceContains(refs, "source_change") ||
		stringSliceContains(refs, "stale/readable.txt") {
		t.Fatalf("abstractTargetRefs = %+v, want only current focus abstract refs", refs)
	}
}

func TestRuntimeManagedTargetSnapshotAcquisitionFailsFastForUnreadableTargetRefs(t *testing.T) {
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"nextAction": map[string]any{"kind": runtimemodel.NextActionTypeAcquireTargetSnapshot},
		},
		"convergenceContract": map[string]any{
			"state": convergenceStateNeedEvidence,
		},
		"repairActionPackage": map[string]any{
			"repairTargetSet": []string{"cwi-001", "deliverable:source_change", "../escape.txt"},
			"requiredOutput": map[string]any{
				"mustAcquireCurrentTargetSnapshot":   true,
				"mustUseReadOnlySnapshotAcquisition": true,
				"mustIncludeStateChangingNode":       false,
			},
		},
	}

	output, ok := runtimeManagedTargetSnapshotAcquisitionOutput(input)
	if !ok {
		t.Fatal("runtimeManagedTargetSnapshotAcquisitionOutput ok = false, want deterministic fail-fast takeover")
	}
	if output["status"] != "failed" ||
		output[failureOutputKeyClass] != failureClassRuntimeContractMisalign ||
		output["deterministicOutcome"] != "fail_fast_internal_contract_gap" {
		t.Fatalf("runtimeManagedTargetSnapshotAcquisitionOutput = %+v, want deterministic fail-fast output", output)
	}
	if graph := output[stepOutputKeyExecutionGraph]; graph != nil {
		t.Fatalf("executionGraph = %+v, want no read graph for unreadable target refs", graph)
	}
	diagnostics := mapSliceFromValue(output["authorityDiagnostics"])
	if len(diagnostics) != 1 || stringFromMap(diagnostics[0], "code") != "target_snapshot_acquisition_failed_abstract_refs" {
		t.Fatalf("authorityDiagnostics = %+v, want target_snapshot_acquisition_failed_abstract_refs", diagnostics)
	}
}

func TestConvergenceDispatchRejectsStateChangeWithoutRequiredTargetSnapshot(t *testing.T) {
	input := map[string]any{
		"convergenceContract": map[string]any{
			"state":                       convergenceStateNeedStateChange,
			"stateChangingOutputRequired": true,
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"mustIncludeStateChangingNode":             true,
				"mustUseCurrentTargetSnapshot":             true,
				"mustPatchDirectlyFromProvidedSnapshot":    true,
				"mustDeclareCoverageTargets":               true,
				"mustBindCoverageFocus":                    true,
				"acceptedKinds":                            []string{"patch/apply"},
				"allowedOutputShapes":                      []string{"executionGraph"},
				"sameGraphReadPatchRule":                   "forbidden",
				"targetSnapshotPrerequisiteBeforeDispatch": true,
			},
		},
		"requiredOutput": map[string]any{
			"mustIncludeStateChangingNode":          true,
			"mustUseCurrentTargetSnapshot":          true,
			"mustPatchDirectlyFromProvidedSnapshot": true,
			"acceptedKinds":                         []string{"patch/apply"},
			"allowedOutputShapes":                   []string{"executionGraph"},
		},
	}

	err := validateConvergenceDispatchRequiredOutputSemantics(input)
	if err == nil {
		t.Fatal("validateConvergenceDispatchRequiredOutputSemantics error = nil, want missing target snapshot preflight")
	}
	diagnostics := mapSliceFromValue(input["authorityDiagnostics"])
	if len(diagnostics) != 1 || stringFromMap(diagnostics[0], "code") != "state_change_missing_target_snapshot_preflight" {
		t.Fatalf("authorityDiagnostics = %+v, want state_change_missing_target_snapshot_preflight", diagnostics)
	}
}

func TestConvergenceDispatchAllowsStateChangeWithExactPatchSafeTargetSnapshot(t *testing.T) {
	required := map[string]any{
		"mustIncludeStateChangingNode":          true,
		"mustUseCurrentTargetSnapshot":          true,
		"mustPatchDirectlyFromProvidedSnapshot": true,
		"acceptedKinds":                         []string{"patch/apply"},
		"allowedOutputShapes":                   []string{"executionGraph"},
	}
	input := map[string]any{
		"convergenceContract": map[string]any{
			"state":                       convergenceStateNeedStateChange,
			"stateChangingOutputRequired": true,
		},
		"repairActionPackage": map[string]any{
			"targetSnapshots": []map[string]any{{
				"targetRef":         "target.txt",
				"contentHash":       "sha256:target",
				"contentComplete":   true,
				"safeForPatch":      true,
				"safeForFileWrite":  true,
				"safeForExactPatch": true,
				"runtimeManaged":    true,
			}},
			"requiredOutput": cloneStepPayload(required),
		},
		"requiredOutput": cloneStepPayload(required),
	}

	if err := validateConvergenceDispatchRequiredOutputSemantics(input); err != nil {
		t.Fatalf("validateConvergenceDispatchRequiredOutputSemantics error = %v, want nil", err)
	}
	if diagnostics := mapSliceFromValue(input["authorityDiagnostics"]); len(diagnostics) != 0 {
		t.Fatalf("authorityDiagnostics = %+v, want none", diagnostics)
	}
}

func TestRuntimeManagedTargetSnapshotAcquisitionFailsFastForCurrentActionAbstractFocusRefs(t *testing.T) {
	required := map[string]any{
		"mustAcquireCurrentTargetSnapshot":   true,
		"mustUseReadOnlySnapshotAcquisition": true,
		"mustIncludeStateChangingNode":       false,
	}
	focus := map[string]any{
		"id":                  "focus-cwi-039",
		"coverageWorkItemIds": []string{"cwi-039"},
		"targetRefs":          []string{"cwi-039", "version_control_snapshot"},
		"structuredTargetRefs": []map[string]any{
			{"targetRef": "deliverable:source_change"},
			{"targetRef": "source_change"},
		},
	}
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"nextAction": map[string]any{"kind": runtimemodel.NextActionTypeAcquireTargetSnapshot},
		},
		"convergenceContract": map[string]any{
			"state": convergenceStateNeedEvidence,
		},
		currentActionContractKey: map[string]any{
			"currentFocus":   focus,
			"requiredOutput": required,
		},
	}

	output, ok := runtimeManagedTargetSnapshotAcquisitionOutput(input)
	if !ok {
		t.Fatal("runtimeManagedTargetSnapshotAcquisitionOutput ok = false, want deterministic fail-fast for abstract focus refs")
	}
	if output["status"] != "failed" ||
		output[failureOutputKeyClass] != failureClassRuntimeContractMisalign ||
		output["deterministicOutcome"] != "fail_fast_internal_contract_gap" {
		t.Fatalf("runtimeManagedTargetSnapshotAcquisitionOutput = %+v, want deterministic fail-fast output", output)
	}
	refs := stringSliceFromValue(output["abstractTargetRefs"])
	for _, want := range []string{"cwi-039", "deliverable:source_change", "source_change", "version_control_snapshot"} {
		if !stringSliceContains(refs, want) {
			t.Fatalf("abstractTargetRefs = %+v, want %q", refs, want)
		}
	}
	if graph := output[stepOutputKeyExecutionGraph]; graph != nil {
		t.Fatalf("executionGraph = %+v, want no read graph for abstract focus refs", graph)
	}
}

func TestRuntimeManagedTargetSnapshotAcquisitionUsesManifestForAbstractFocus(t *testing.T) {
	required := map[string]any{
		"mustAcquireCurrentTargetSnapshot":   true,
		"mustUseReadOnlySnapshotAcquisition": true,
		"mustIncludeStateChangingNode":       false,
	}
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"nextAction": map[string]any{"kind": runtimemodel.NextActionTypeAcquireTargetSnapshot},
			"currentFocus": map[string]any{
				"id":         "cwi-current",
				"targetRefs": []string{"cwi-current", "source_change", "deliverable:source_change"},
			},
		},
		"convergenceContract": map[string]any{
			"state":            convergenceStateNeedEvidence,
			"runtimeStageMode": "target_snapshot_acquisition",
		},
		"repairActionPackage": map[string]any{
			stepInputKeyWorkspaceFileManifest: []map[string]any{
				{"path": "b.txt"},
				{"path": "a.txt"},
				{"path": "c.txt"},
			},
			"requiredOutput": required,
		},
	}

	output, ok := runtimeManagedTargetSnapshotAcquisitionOutput(input)
	if !ok {
		t.Fatal("runtimeManagedTargetSnapshotAcquisitionOutput ok = false, want manifest-backed read-only graph")
	}
	if output["status"] != "completed" || output["runtimeManagedTargetSnapshotAcquisition"] != true {
		t.Fatalf("output = %+v, want completed runtime-managed acquisition", output)
	}
	graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph])
	if !ok || len(graph.Nodes) != 3 {
		t.Fatalf("graph = %+v/%v, want bounded manifest read graph", graph, ok)
	}
	for idx, want := range []string{"a.txt", "b.txt", "c.txt"} {
		node := graph.Nodes[idx]
		if node.Kind != "file/read" || node.Input["path"] != want || node.Input["targetRef"] != want {
			t.Fatalf("node[%d] = %+v, want file/read %s", idx, node, want)
		}
	}
}

func TestRuntimeManagedTargetSnapshotAcquisitionLimitsManifestRefs(t *testing.T) {
	required := map[string]any{
		"mustAcquireCurrentTargetSnapshot":   true,
		"mustUseReadOnlySnapshotAcquisition": true,
		"mustIncludeStateChangingNode":       false,
	}
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"nextAction": map[string]any{"kind": runtimemodel.NextActionTypeAcquireTargetSnapshot},
			"currentFocus": map[string]any{
				"id":         "cwi-current",
				"targetRefs": []string{"source_change"},
			},
		},
		"repairActionPackage": map[string]any{
			stepInputKeyWorkspaceFileManifest: []map[string]any{
				{"path": "target-06.txt"},
				{"path": "target-01.txt"},
				{"path": "target-04.txt"},
				{"path": "target-02.txt"},
				{"path": "target-03.txt"},
				{"path": "target-05.txt"},
				{"path": "target-07.txt"},
			},
			"requiredOutput": required,
		},
	}

	output, ok := runtimeManagedTargetSnapshotAcquisitionOutput(input)
	if !ok {
		t.Fatal("runtimeManagedTargetSnapshotAcquisitionOutput ok = false, want bounded manifest graph")
	}
	graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph])
	if !ok || len(graph.Nodes) != runtimeManagedTargetSnapshotRefLimit {
		t.Fatalf("graph = %+v/%v, want %d bounded manifest reads", graph, ok, runtimeManagedTargetSnapshotRefLimit)
	}
	for idx, want := range []string{"target-01.txt", "target-02.txt", "target-03.txt", "target-04.txt", "target-05.txt"} {
		if got := graph.Nodes[idx].Input["path"]; got != want {
			t.Fatalf("graph node[%d] path = %v, want %s", idx, got, want)
		}
	}
}

func TestRuntimeManagedTargetSnapshotAcquisitionRejectsAbstractCoveragePaths(t *testing.T) {
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"nextAction": map[string]any{"kind": runtimemodel.NextActionTypeAcquireTargetSnapshot},
		},
		"repairActionPackage": map[string]any{
			"repairTargetSet": []string{
				"coverage_map",
				"dod-d6f553ebf8b83f1a-dod-item-001",
				"goal-req-005-dcbcff0f",
				"goal.leaves[4]",
				"user_deliverable",
				"validation_report",
			},
			"requiredOutput": map[string]any{
				"mustAcquireCurrentTargetSnapshot":   true,
				"mustUseReadOnlySnapshotAcquisition": true,
				"mustIncludeStateChangingNode":       false,
			},
		},
	}

	output, ok := runtimeManagedTargetSnapshotAcquisitionOutput(input)
	if !ok {
		t.Fatal("runtimeManagedTargetSnapshotAcquisitionOutput ok = false, want fail-fast for abstract coverage refs")
	}
	if output["status"] != "failed" ||
		output[failureOutputKeyClass] != failureClassRuntimeContractMisalign ||
		output["deterministicOutcome"] != "fail_fast_internal_contract_gap" {
		t.Fatalf("runtimeManagedTargetSnapshotAcquisitionOutput = %+v, want deterministic fail-fast output", output)
	}
	if graph := output[stepOutputKeyExecutionGraph]; graph != nil {
		t.Fatalf("executionGraph = %+v, want no file/read graph for abstract coverage refs", graph)
	}
}

func TestRuntimeManagedTargetSnapshotAcquisitionDispatchesReadOnlyGraphWithoutRootLLM(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	evidenceRepo := memory.NewEvidenceRecordRepository()
	hub := events.NewMemoryHub()
	deliverableDir := t.TempDir()
	if err := os.WriteFile(deliverableDir+"/target.txt", []byte("current target content\n"), 0o644); err != nil {
		t.Fatalf("seed target file: %v", err)
	}

	runtime := NewRuntimeAdapterRegistry()
	llmRuntime := &requestCapturingRuntimeAdapter{output: map[string]any{
		"status": "completed",
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied":   true,
			"summary":     "follow-up completed after target snapshot acquisition",
			"goalOutcome": "follow-up completed after target snapshot acquisition",
			"items": []map[string]any{{
				"item":         "target snapshot acquisition follow-up",
				"state":        "satisfied",
				"evidenceRefs": []string{"target snapshot acquired"},
			}},
		},
	}}
	if err := runtime.Register(llmWorkspaceExecutionStepKind, llmRuntime); err != nil {
		t.Fatalf("register llm adapter: %v", err)
	}
	if err := runtime.Register(filecommon.StepKindRead, filebiz.NewRuntimeAdapter()); err != nil {
		t.Fatalf("register file/read adapter: %v", err)
	}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		memory.NewDecisionTicketRepository(),
		nil,
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{EvidenceRecords: evidenceRepo},
		hub,
	)

	workspaceID := common.WorkspaceID("ws-runtime-managed-target-snapshot")
	runID := common.RunID("run-runtime-managed-target-snapshot")
	rootStepID := common.StepID(runID + "/step-1")
	workspace := &workspacemodel.Workspace{
		ID:                   workspaceID,
		Title:                "runtime managed target snapshot",
		DeliverableDirectory: deliverableDir,
		State:                common.WorkspaceStateActive,
		ActiveRunID:          runID,
	}
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive}
	if err := workspaceRepo.Create(ctx, workspace); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	input := runtimeManagedTargetSnapshotDispatchInput(workspaceID, runID)
	err := orch.dispatchStepWithWorkspace(ctx, run, rootStepID, llmWorkspaceExecutionStepKind, input, workspace)
	if err != nil {
		t.Fatalf("dispatch step: %v", err)
	}
	rootStep, err := stepRepo.Get(ctx, rootStepID)
	if err != nil {
		t.Fatalf("get root step: %v", err)
	}
	if rootStep.State != common.StepStateSucceeded || rootStep.Output["runtimeManagedTargetSnapshotAcquisition"] != true {
		t.Fatalf("root step = %+v, want runtime-managed completed root", rootStep)
	}
	for _, request := range llmRuntime.requests {
		if request.StepID == rootStepID {
			t.Fatalf("root LLM runtime was called: %+v", request)
		}
	}

	steps, err := stepRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	var readStep *runmodel.Step
	for _, item := range steps {
		if item.Kind == filecommon.StepKindRead {
			readStep = item
			break
		}
	}
	if readStep == nil {
		t.Fatalf("steps = %+v, want dispatched file/read graph step", steps)
	}
	if readStep.Input["path"] != "target.txt" || readStep.Input["runtimeManagedAcquisition"] != true {
		t.Fatalf("read step input = %+v, want runtime-managed target.txt read", readStep.Input)
	}
	if readStep.State != common.StepStateSucceeded {
		t.Fatalf("read step state = %s, want completed", readStep.State)
	}
	records, err := evidenceRepo.ListByRun(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), runID)
	if err != nil {
		t.Fatalf("list evidence records: %v", err)
	}
	var snapshotRecord *runtimemodel.EvidenceRecord
	for _, record := range records {
		if record.Kind == runtimemodel.EvidenceKindTargetSnapshot && stringFromMap(record.Metadata, "targetRef") == "target.txt" {
			snapshotRecord = record
			break
		}
	}
	if snapshotRecord == nil {
		t.Fatalf("evidence records = %+v, want accepted target snapshot record for target.txt", records)
	}
	if snapshotRecord.SourceRef == "" || stringFromMap(snapshotRecord.Metadata, "sourceRef") != snapshotRecord.SourceRef {
		t.Fatalf("snapshot metadata sourceRef = %q/%q, want record sourceRef", snapshotRecord.SourceRef, stringFromMap(snapshotRecord.Metadata, "sourceRef"))
	}
	if ids := stringSliceFromValue(snapshotRecord.Metadata["coverageWorkItemIds"]); !stringSliceContains(ids, "cwi-target-snapshot") {
		t.Fatalf("snapshot coverageWorkItemIds = %+v, want current coverage item", ids)
	}
	if ids := snapshotRecord.CoverageItemRefs; !stringSliceContains(ids, "cwi-target-snapshot") {
		t.Fatalf("snapshot CoverageItemRefs = %+v, want current coverage item", ids)
	}
	if refs := stringSliceFromValue(snapshotRecord.Metadata["structuredTargetRefs"]); !stringSliceContains(refs, "target.txt") {
		t.Fatalf("snapshot structuredTargetRefs = %+v, want target.txt", refs)
	}
	if claims := stringSliceFromValue(snapshotRecord.Metadata["observedClaims"]); len(claims) == 0 {
		t.Fatalf("snapshot observedClaims = %+v, want runtime-derived observed claim", claims)
	}
	for _, key := range []string{"contentHash", "contentComplete", "safeForFileWrite"} {
		if _, ok := snapshotRecord.Metadata[key]; !ok {
			t.Fatalf("snapshot metadata missing %s: %+v", key, snapshotRecord.Metadata)
		}
	}
	snapshot := targetSnapshotFromAcceptedEvidenceStep(snapshotRecord, readStep)
	if len(snapshot) == 0 {
		t.Fatalf("targetSnapshotFromAcceptedEvidenceStep returned empty")
	}
	if ids := stringSliceFromValue(snapshot["coverageWorkItemIds"]); !stringSliceContains(ids, "cwi-target-snapshot") {
		t.Fatalf("materialized snapshot coverageWorkItemIds = %+v, want current coverage item", ids)
	}
	if refs := stringSliceFromValue(snapshot["structuredTargetRefs"]); !stringSliceContains(refs, "target.txt") {
		t.Fatalf("materialized snapshot structuredTargetRefs = %+v, want target.txt", refs)
	}
	if claims := stringSliceFromValue(snapshot["observedClaims"]); len(claims) == 0 {
		t.Fatalf("materialized snapshot observedClaims = %+v, want observed claim", claims)
	}
	if !publishedStepState(hub.Published(), rootStepID, common.StepStateSucceeded) {
		t.Fatalf("events = %+v, want completed root step event", hub.Published())
	}
}

func TestRuntimeManagedTargetSnapshotAcquisitionOptionalMissingFileDoesNotAcceptSnapshot(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	evidenceRepo := memory.NewEvidenceRecordRepository()
	hub := events.NewMemoryHub()
	deliverableDir := t.TempDir()

	runtime := NewRuntimeAdapterRegistry()
	llmRuntime := &requestCapturingRuntimeAdapter{output: map[string]any{"status": "completed"}}
	if err := runtime.Register(llmWorkspaceExecutionStepKind, llmRuntime); err != nil {
		t.Fatalf("register llm adapter: %v", err)
	}
	if err := runtime.Register(filecommon.StepKindRead, filebiz.NewRuntimeAdapter()); err != nil {
		t.Fatalf("register file/read adapter: %v", err)
	}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		memory.NewDecisionTicketRepository(),
		nil,
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{EvidenceRecords: evidenceRepo},
		hub,
	)

	workspaceID := common.WorkspaceID("ws-runtime-managed-target-snapshot-missing")
	runID := common.RunID("run-runtime-managed-target-snapshot-missing")
	rootStepID := common.StepID(runID + "/step-1")
	workspace := &workspacemodel.Workspace{
		ID:                   workspaceID,
		Title:                "runtime managed missing target snapshot",
		DeliverableDirectory: deliverableDir,
		State:                common.WorkspaceStateActive,
		ActiveRunID:          runID,
	}
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive}
	if err := workspaceRepo.Create(ctx, workspace); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	input := runtimeManagedTargetSnapshotDispatchInput(workspaceID, runID)
	err := orch.dispatchStepWithWorkspace(ctx, run, rootStepID, llmWorkspaceExecutionStepKind, input, workspace)
	if err != nil {
		t.Fatalf("dispatch step: %v", err)
	}
	steps, err := stepRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	var readStep *runmodel.Step
	for _, item := range steps {
		if item.Kind == filecommon.StepKindRead {
			readStep = item
			break
		}
	}
	if readStep == nil || readStep.State != common.StepStateSucceeded {
		t.Fatalf("read step = %+v, want completed optional file/read", readStep)
	}
	if readStep.Output["exists"] != false {
		t.Fatalf("read output = %+v, want exists=false optional acquisition", readStep.Output)
	}
	records, err := evidenceRepo.ListByRun(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), runID)
	if err != nil {
		t.Fatalf("list evidence records: %v", err)
	}
	for _, record := range records {
		if record.Kind == runtimemodel.EvidenceKindTargetSnapshot {
			t.Fatalf("evidence records = %+v, want no accepted target snapshot for missing optional file", records)
		}
	}
}

func runtimeManagedTargetSnapshotDispatchInput(workspaceID common.WorkspaceID, runID common.RunID) map[string]any {
	required := map[string]any{
		"mustAcquireCurrentTargetSnapshot":   true,
		"mustUseReadOnlySnapshotAcquisition": true,
		"mustIncludeStateChangingNode":       false,
		"acceptedKinds":                      []string{"file/read", "file/list", "git/read"},
		"repairTargetSet":                    []string{"target.txt"},
		"coverageWorkItemIds":                []string{"cwi-target-snapshot"},
		"structuredTargetRefs":               []string{"target.txt"},
		"observedClaims":                     []string{"target snapshot content is available for the current coverage focus"},
	}
	return map[string]any{
		convergenceAuthorityKey: map[string]any{
			"schemaVersion": convergenceAuthoritySchema,
			"workspaceId":   string(workspaceID),
			"runId":         string(runID),
			"currentFocus": map[string]any{
				"id":                   "focus-target-snapshot",
				"coverageWorkItemIds":  []string{"cwi-target-snapshot"},
				"structuredTargetRefs": []map[string]any{{"targetRef": "target.txt"}},
			},
			"nextAction": map[string]any{
				"kind":         runtimemodel.NextActionTypeStateChange,
				"prerequisite": targetSnapshotAcquisitionPrerequisite(),
			},
		},
		"convergenceContract": map[string]any{
			"state":                       convergenceStateNeedStateChange,
			"runtimeStageMode":            "target_snapshot_acquisition",
			"stateChangingOutputRequired": true,
			"prerequisite":                targetSnapshotAcquisitionPrerequisite(),
		},
		"repairActionPackage": map[string]any{
			"requiresCurrentTargetSnapshotForStateChange": true,
			"repairTargetSet": []string{"target.txt"},
			"requiredOutput":  cloneStepPayload(required),
		},
		stepInputKeyStagePacket: map[string]any{
			"requiredOutput": cloneStepPayload(required),
		},
		"requiredOutput": cloneStepPayload(required),
	}
}
