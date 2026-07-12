package orchestrator

import (
	"strings"
	"testing"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestAttachCurrentActionContractFromAuthority(t *testing.T) {
	run := &runModel.Run{ID: "run-001", WorkspaceID: "ws-current-action"}
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"nextAction":   map[string]any{"kind": "state_change", "reason": "need state change"},
			"currentFocus": map[string]any{"id": "cwi-001", "coverageWorkItemIds": []any{"cwi-001"}},
		},
		"convergenceContract": map[string]any{
			"state":                       convergenceStateNeedStateChange,
			"stateChangingOutputRequired": true,
		},
		"stateChangingOutputRequired": true,
		stepInputKeyStagePacket: map[string]any{
			"schemaVersion": "workorch.stage_packet.v1",
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"mustIncludeStateChangingNode": true,
				"coverageWorkItemIds":          []any{"cwi-001"},
				"acceptedKinds":                []any{"patch/apply"},
			},
		},
	}

	got := attachCurrentActionContractToInput(run, common.StepID("run-001/step-001"), input)
	contract := mapFromValue(got[currentActionContractKey])
	if contract["schemaVersion"] != currentActionContractSchema ||
		contract["workspaceId"] != string(run.WorkspaceID) ||
		contract["runId"] != string(run.ID) ||
		contract["stepId"] != "run-001/step-001" {
		t.Fatalf("currentActionContract identity = %+v", contract)
	}
	if next := mapFromValue(contract["nextAction"]); next["kind"] != "state_change" {
		t.Fatalf("nextAction = %+v, want state_change", next)
	}
	if required := mapFromValue(contract["requiredOutput"]); required["mustIncludeStateChangingNode"] != true ||
		!stringSliceContains(stringSliceFromValue(required["coverageWorkItemIds"]), "cwi-001") {
		t.Fatalf("requiredOutput = %+v, want state-changing cwi-001 contract", required)
	}
	if err := validateConvergenceDispatchInput(got); err != nil {
		t.Fatalf("validateConvergenceDispatchInput: %v", err)
	}
}

func TestAttachCurrentActionContractSurfacesReadyTargetSnapshotRefs(t *testing.T) {
	run := &runModel.Run{ID: "run-current-action-target-snapshot", WorkspaceID: "ws-current-action-target-snapshot"}
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"nextAction":   map[string]any{"kind": runtimemodel.NextActionTypeStateChange},
			"currentFocus": map[string]any{"id": "cwi-target", "coverageWorkItemIds": []any{"cwi-target"}},
		},
		"convergenceContract": map[string]any{
			"state":                       convergenceStateNeedStateChange,
			"stateChangingOutputRequired": true,
		},
		"stateChangingOutputRequired": true,
		"repairActionPackage": map[string]any{
			"targetSnapshots": []map[string]any{{
				"targetRef":         "target.txt",
				"content":           "current content must stay out of currentActionContract",
				"contentHash":       "sha256:target",
				"contentComplete":   true,
				"safeForPatch":      true,
				"safeForFileWrite":  true,
				"safeForExactPatch": true,
				"runtimeManaged":    true,
				"evidenceRef":       "ev-target-snapshot",
			}},
			"requiredOutput": map[string]any{
				"mustIncludeStateChangingNode": true,
				"acceptedKinds":                []any{"patch/apply"},
			},
		},
	}

	got := attachCurrentActionContractToInput(run, common.StepID("run-current-action-target-snapshot/step-001"), input)
	snapshots := mapSliceFromValue(mapFromValue(got[currentActionContractKey])["targetSnapshots"])
	if len(snapshots) != 1 || snapshots[0]["targetRef"] != "target.txt" || snapshots[0]["contentHash"] != "sha256:target" {
		t.Fatalf("currentActionContract.targetSnapshots = %+v, want compact ready snapshot ref", snapshots)
	}
	if snapshots[0]["content"] != nil {
		t.Fatalf("currentActionContract.targetSnapshots = %+v, must not carry full content", snapshots)
	}
}

func TestValidateConvergenceDispatchInputRejectsCurrentActionContractDrift(t *testing.T) {
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"nextAction": map[string]any{"kind": "state_change"},
		},
		"convergenceContract": map[string]any{
			"state":                       convergenceStateNeedStateChange,
			"stateChangingOutputRequired": true,
		},
		"stateChangingOutputRequired": true,
		stepInputKeyStagePacket:       map[string]any{"schemaVersion": "workorch.stage_packet.v1"},
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"mustIncludeStateChangingNode": true,
				"acceptedKinds":                []any{"patch/apply"},
			},
		},
		currentActionContractKey: map[string]any{
			"nextAction":     map[string]any{"kind": "validate"},
			"requiredOutput": map[string]any{"mustIncludeStateChangingNode": true, "acceptedKinds": []any{"patch/apply"}},
		},
	}

	err := validateConvergenceDispatchInput(input)
	if err == nil || !strings.Contains(err.Error(), "currentActionContract nextAction=validate expected=state_change") {
		t.Fatalf("validateConvergenceDispatchInput error = %v, want currentActionContract drift", err)
	}
}

func TestCurrentActionValidationRequiredOutputRemovesMCPWhenNoServerDeclared(t *testing.T) {
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"nextAction":   map[string]any{"kind": "validate"},
			"currentFocus": map[string]any{"id": "cwi-validate", "coverageWorkItemIds": []any{"cwi-validate"}},
		},
		"convergenceContract": map[string]any{
			"state": convergenceStateNeedValidation,
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"mustProduceConcreteValidationEvidence": true,
				"acceptedKinds":                         []any{"shell/command", "mcp/tool_call", managedValidationSessionNodeKind},
			},
		},
	}

	required := activeRequiredOutputForCurrentAction(input)
	if stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "mcp/tool_call") {
		t.Fatalf("requiredOutput.acceptedKinds = %+v, want mcp/tool_call removed without declared server", required["acceptedKinds"])
	}
	if !stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "shell/command") ||
		!stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), managedValidationSessionNodeKind) {
		t.Fatalf("requiredOutput.acceptedKinds = %+v, want shell and managed validation retained", required["acceptedKinds"])
	}
	if stringFromMap(required, "mcpToolCallUnavailableReason") == "" {
		t.Fatalf("requiredOutput = %+v, want unavailable MCP reason", required)
	}
}

func TestCurrentActionValidationRequiredOutputKeepsDeclaredMCPServers(t *testing.T) {
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"nextAction":   map[string]any{"kind": "validate"},
			"currentFocus": map[string]any{"id": "cwi-validate", "coverageWorkItemIds": []any{"cwi-validate"}},
		},
		"convergenceContract": map[string]any{
			"state": convergenceStateNeedValidation,
		},
		"runtimeCapabilities": map[string]any{
			"mcpServers": []map[string]any{{"id": "semantic-mcp"}},
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"mustProduceConcreteValidationEvidence": true,
				"acceptedKinds":                         []any{"shell/command", "mcp/tool_call"},
			},
		},
	}

	required := activeRequiredOutputForCurrentAction(input)
	if !stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "mcp/tool_call") {
		t.Fatalf("requiredOutput.acceptedKinds = %+v, want declared mcp/tool_call retained", required["acceptedKinds"])
	}
	if ids := stringSliceFromValue(required["allowedMCPServerIds"]); len(ids) != 1 || ids[0] != "semantic-mcp" {
		t.Fatalf("requiredOutput.allowedMCPServerIds = %+v, want [semantic-mcp]", ids)
	}
}

func TestCurrentActionValidationRequiredOutputKeepsMCPFromExecutionContextCapabilityIDs(t *testing.T) {
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"nextAction":   map[string]any{"kind": runtimemodel.NextActionTypeValidate},
			"currentFocus": map[string]any{"id": "cwi-validate", "coverageWorkItemIds": []any{"cwi-validate"}},
		},
		"convergenceContract": map[string]any{"state": convergenceStateNeedValidation},
		"executionContext": map[string]any{
			"capabilityIds": []string{"semantic-mcp:tool:lsp_hover"},
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"mustProduceConcreteValidationEvidence": true,
				"acceptedKinds":                         []any{"shell/command", "mcp/tool_call", managedValidationSessionNodeKind},
			},
		},
	}

	required := activeRequiredOutputForCurrentAction(input)
	if !stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), runtimemodel.StepKindMCPToolCall) {
		t.Fatalf("requiredOutput.acceptedKinds = %+v, want mcp/tool_call from executionContext capabilityIds", required["acceptedKinds"])
	}
	if ids := stringSliceFromValue(required["allowedMCPServerIds"]); len(ids) != 1 || ids[0] != "semantic-mcp" {
		t.Fatalf("requiredOutput.allowedMCPServerIds = %+v, want [semantic-mcp]", ids)
	}
}

func TestCurrentActionValidationRequiredOutputReportsCapabilityGapWhenMCPUnavailable(t *testing.T) {
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"nextAction":   map[string]any{"kind": runtimemodel.NextActionTypeValidate},
			"currentFocus": map[string]any{"id": "cwi-validate", "coverageWorkItemIds": []any{"cwi-validate"}},
		},
		"convergenceContract": map[string]any{"state": convergenceStateNeedValidation},
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"mustProduceConcreteValidationEvidence": true,
				"acceptedKinds":                         []any{"shell/command", "mcp/tool_call", managedValidationSessionNodeKind},
			},
		},
	}

	required := activeRequiredOutputForCurrentAction(input)
	if !boolValue(required["mustResolveCapabilityGap"]) ||
		!stringSliceContains(stringSliceFromValue(required["missingKinds"]), runtimemodel.StepKindMCPToolCall) {
		t.Fatalf("requiredOutput = %+v, want explicit MCP capability gap", required)
	}
	if stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), runtimemodel.StepKindMCPToolCall) {
		t.Fatalf("requiredOutput.acceptedKinds = %+v, want mcp/tool_call removed", required["acceptedKinds"])
	}
}

func TestSynchronizeCurrentRequiredOutputMirrorsUsesCurrentContractBeforeStaleMirrors(t *testing.T) {
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"nextAction":   map[string]any{"kind": "validate"},
			"currentFocus": map[string]any{"id": "cwi-validate", "coverageWorkItemIds": []any{"cwi-validate"}},
		},
		"convergenceContract": map[string]any{
			"state":        convergenceStateNeedValidation,
			"runtimeStage": runtimeStageVerification,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"coverageWorkItemIds":                   []any{"cwi-validate"},
					"mustProduceConcreteValidationEvidence": true,
					"acceptedKinds":                         []any{"validation/managed_session"},
				},
			},
		},
		stepInputKeyStagePacket: map[string]any{
			"schemaVersion": "workorch.stage_packet.v1",
			"nextAction": map[string]any{
				"type": runtimemodel.NextActionTypeValidate,
				"requiredOutput": map[string]any{
					"coverageWorkItemIds":          []any{"cwi-stale"},
					"mustIncludeStateChangingNode": true,
					"acceptedKinds":                []any{"patch/apply"},
				},
			},
			"requiredOutput": map[string]any{
				"coverageWorkItemIds":          []any{"cwi-stale"},
				"mustIncludeStateChangingNode": true,
				"acceptedKinds":                []any{"patch/apply"},
			},
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"coverageWorkItemIds":          []any{"cwi-stale"},
				"mustIncludeStateChangingNode": true,
				"acceptedKinds":                []any{"patch/apply"},
			},
		},
		"requiredOutput": map[string]any{
			"coverageWorkItemIds":          []any{"cwi-stale"},
			"mustIncludeStateChangingNode": true,
			"acceptedKinds":                []any{"patch/apply"},
		},
	}

	run := &runModel.Run{ID: "run-required-output-authority", WorkspaceID: "ws-required-output-authority"}
	got := synchronizeCurrentRequiredOutputMirrors(input)
	got = attachCurrentActionContractToInput(run, "run-required-output-authority/step-001", got)
	required := mapFromValue(got["requiredOutput"])
	if boolValue(required["mustIncludeStateChangingNode"]) ||
		!boolValue(required["mustProduceConcreteValidationEvidence"]) ||
		!stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "validation/managed_session") ||
		stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("requiredOutput = %+v, want validation contract from convergenceContract before stale mirrors", required)
	}
	for label, mirror := range map[string]map[string]any{
		"repairActionPackage.requiredOutput":    mapFromValue(mapFromValue(got["repairActionPackage"])["requiredOutput"]),
		"stagePacket.requiredOutput":            mapFromValue(mapFromValue(got[stepInputKeyStagePacket])["requiredOutput"]),
		"stagePacket.nextAction.requiredOutput": mapFromValue(mapFromValue(mapFromValue(got[stepInputKeyStagePacket])["nextAction"])["requiredOutput"]),
		"currentActionContract.requiredOutput":  mapFromValue(mapFromValue(got[currentActionContractKey])["requiredOutput"]),
	} {
		if stableCoverageJSON(mirror) != stableCoverageJSON(required) {
			t.Fatalf("%s = %+v, want synchronized requiredOutput %+v", label, mirror, required)
		}
	}
}

func TestRequiredOutputSeedIgnoresStaleTargetSnapshotMirrorForStateChange(t *testing.T) {
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"nextAction":   map[string]any{"kind": runtimemodel.NextActionTypeStateChange},
			"currentFocus": map[string]any{"id": "cwi-current", "coverageWorkItemIds": []any{"cwi-current"}},
		},
		"convergenceContract": map[string]any{
			"state":                       convergenceStateNeedStateChange,
			"stateChangingOutputRequired": true,
		},
		"stateChangingOutputRequired": true,
		"requiredOutput": map[string]any{
			"mustAcquireCurrentTargetSnapshot":   true,
			"mustUseReadOnlySnapshotAcquisition": true,
			"acceptedKinds":                      []any{"target_snapshot"},
			"forbiddenKinds":                     []any{"patch/apply"},
		},
		currentActionContractKey: map[string]any{
			"prerequisite": targetSnapshotAcquisitionPrerequisite(),
			"requiredOutput": map[string]any{
				"mustAcquireCurrentTargetSnapshot":   true,
				"mustUseReadOnlySnapshotAcquisition": true,
				"acceptedKinds":                      []any{"target_snapshot"},
				"forbiddenKinds":                     []any{"patch/apply"},
			},
		},
	}

	required := activeRequiredOutputForCurrentAction(input)
	if boolValue(required["mustAcquireCurrentTargetSnapshot"]) ||
		boolValue(required["mustUseReadOnlySnapshotAcquisition"]) ||
		stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "target_snapshot") {
		t.Fatalf("requiredOutput = %+v, want current state-changing contract instead of stale target snapshot mirror", required)
	}
	if !requiredOutputHasAllowedStateChangingKind(required) {
		t.Fatalf("requiredOutput = %+v, want state-changing accepted kind", required)
	}
	if stringSliceContains(stringSliceFromValue(required["forbiddenKinds"]), "patch/apply") {
		t.Fatalf("requiredOutput.forbiddenKinds = %+v, want patch/apply not forbidden by stale mirror", required["forbiddenKinds"])
	}
}

func TestCurrentRequiredOutputIgnoresOldFailedStepLineage(t *testing.T) {
	required := concreteValidationEvidenceRequiredOutputWithScratch(
		map[string]any{
			"coverageWorkItemIds":  []string{"cwi-current"},
			"structuredTargetRefs": []string{"cwi-current"},
		},
		nil,
		nil,
	)
	staleFailedRequired := map[string]any{
		"mustIncludeStateChangingNode": true,
		"acceptedKinds":                []any{"patch/apply"},
		"coverageWorkItemIds":          []any{"cwi-stale"},
	}
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"schemaVersion": convergenceAuthoritySchema,
			"currentFocus": map[string]any{
				"id":                  "cwi-current",
				"coverageWorkItemIds": []string{"cwi-current"},
			},
			"nextAction": map[string]any{
				"kind":           runtimemodel.NextActionTypeValidate,
				"requiredOutput": required,
			},
		},
		"convergenceContract": map[string]any{"state": convergenceStateNeedValidation},
		"repairActionPackage": map[string]any{
			"requiredOutput": required,
		},
		"currentFailure": map[string]any{
			"failedStepId": "run-old/failed-step",
			"failedStepInput": map[string]any{
				"requiredOutput": staleFailedRequired,
			},
			"failedStepOutput": map[string]any{
				"requiredOutput": staleFailedRequired,
			},
		},
		"diagnosticLineage": map[string]any{
			"oldFailedStep": map[string]any{
				"input":  map[string]any{"requiredOutput": staleFailedRequired},
				"output": map[string]any{"requiredOutput": staleFailedRequired},
			},
		},
		stepInputKeyProcessEvidenceTimeline: []map[string]any{{
			"stepId":         "run-old/failed-step",
			"requiredOutput": staleFailedRequired,
		}},
		stepInputKeyStagePacket: map[string]any{
			"schemaVersion": "workorch.stage_packet.v1",
			"nextAction":    map[string]any{"type": runtimemodel.NextActionTypeValidate},
		},
	}
	run := &runModel.Run{ID: "run-current-required-output-lineage", WorkspaceID: "ws-current-required-output-lineage"}
	got := synchronizeCurrentRequiredOutputMirrors(input)
	got = attachCurrentActionContractToInput(run, "run-current-required-output-lineage/step-001", got)

	for label, mirror := range map[string]map[string]any{
		"requiredOutput":                       mapFromValue(got["requiredOutput"]),
		"repairActionPackage.requiredOutput":   mapFromValue(mapFromValue(got["repairActionPackage"])["requiredOutput"]),
		"stagePacket.requiredOutput":           mapFromValue(mapFromValue(got[stepInputKeyStagePacket])["requiredOutput"]),
		"currentActionContract.requiredOutput": mapFromValue(mapFromValue(got[currentActionContractKey])["requiredOutput"]),
	} {
		if boolValue(mirror["mustIncludeStateChangingNode"]) ||
			stringSliceContains(stringSliceFromValue(mirror["acceptedKinds"]), "patch/apply") ||
			!boolValue(mirror["mustProduceConcreteValidationEvidence"]) {
			t.Fatalf("%s = %+v, want current validation requiredOutput without old failed state-change contract", label, mirror)
		}
		if !stringSliceContains(stringSliceFromValue(mirror["coverageWorkItemIds"]), "cwi-current") ||
			stringSliceContains(stringSliceFromValue(mirror["coverageWorkItemIds"]), "cwi-stale") {
			t.Fatalf("%s.coverageWorkItemIds = %+v, want current focus only", label, mirror["coverageWorkItemIds"])
		}
	}
	if gotFailure := mapFromValue(got["currentFailure"]); len(mapFromValue(gotFailure["failedStepInput"])) == 0 {
		t.Fatalf("currentFailure = %+v, want old failed input preserved as diagnostic lineage", gotFailure)
	}
	if err := validateConvergenceDispatchInput(got); err != nil {
		t.Fatalf("validateConvergenceDispatchInput: %v; input=%+v", err, got)
	}
}

func TestCurrentAuthorityRenderedRequiredOutputIgnoresValidationMirrorForStateChange(t *testing.T) {
	validationOnly := concreteValidationEvidenceRequiredOutputWithScratch(
		map[string]any{
			"coverageWorkItemIds":  []string{"cwi-state-change"},
			"structuredTargetRefs": []string{"cwi-state-change", "source_change"},
		},
		nil,
		nil,
	)
	input := map[string]any{
		"currentAuthorityRequiredOutputMirrorsCanonical": true,
		convergenceAuthorityKey: map[string]any{
			"schemaVersion": convergenceAuthoritySchema,
			"currentFocus": map[string]any{
				"id":                    "cwi-state-change",
				"coverageWorkItemIds":   []string{"cwi-state-change"},
				"requiredEvidenceKinds": []string{"validation", "state_change"},
				"missingEvidenceKinds":  []string{"validation", "state_change"},
			},
			"nextAction": map[string]any{
				"kind": runtimemodel.NextActionTypeStateChange,
			},
		},
		"convergenceContract": map[string]any{
			"state":                       convergenceStateNeedStateChange,
			"stateChangingOutputRequired": true,
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": validationOnly,
		},
		"requiredOutput": validationOnly,
		stepInputKeyStagePacket: map[string]any{
			"schemaVersion":   "workorch.stage_packet.v1",
			"requiredOutput":  validationOnly,
			"nextAction":      map[string]any{"type": runtimemodel.NextActionTypeStateChange, "requiredOutput": validationOnly},
			"coverageFocusId": "focus-cwi-state-change",
		},
	}

	required := activeRequiredOutputForCurrentActionContract(input)

	if !boolValue(required["mustIncludeStateChangingNode"]) ||
		!stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") ||
		stringSliceContains(stringSliceFromValue(required["forbiddenKinds"]), "patch/apply") ||
		boolValue(required["mustProduceConcreteValidationEvidence"]) {
		t.Fatalf("requiredOutput = %+v, want rebuilt state-changing output instead of stale validation mirror", required)
	}
}

func TestCurrentActionContractDoesNotFallbackToRunAuthority(t *testing.T) {
	run := &runModel.Run{
		ID:          "run-current-action-stale",
		WorkspaceID: "ws-current-action-stale",
		ExecutionContext: map[string]any{
			convergenceAuthorityKey: map[string]any{
				"nextAction": map[string]any{"kind": "state_change"},
			},
		},
	}
	input := map[string]any{
		"convergenceContract": map[string]any{
			"state":                       convergenceStateNeedStateChange,
			"runtimeStageMode":            "target_snapshot_acquisition",
			"stateChangingOutputRequired": true,
			"prerequisite":                targetSnapshotAcquisitionPrerequisite(),
		},
		stepInputKeyStagePacket: map[string]any{"schemaVersion": "workorch.stage_packet.v1"},
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"mustAcquireCurrentTargetSnapshot":   true,
				"mustUseReadOnlySnapshotAcquisition": true,
				"acceptedKinds":                      []any{"target_snapshot"},
				"forbiddenKinds":                     []any{"patch/apply"},
			},
		},
	}

	got := attachCurrentActionContractToInput(run, common.StepID("run-current-action-stale/step-001"), input)
	contract := mapFromValue(got[currentActionContractKey])
	next := mapFromValue(contract["nextAction"])
	if next["kind"] != "acquire_target_snapshot" {
		t.Fatalf("currentActionContract.nextAction = %+v, want acquire_target_snapshot preflight action", next)
	}
	if prerequisite := mapFromValue(contract["prerequisite"]); prerequisite["kind"] != "acquire_target_snapshot" {
		t.Fatalf("currentActionContract.prerequisite = %+v, want target snapshot prerequisite", prerequisite)
	}
}

func TestValidateConvergenceDispatchInputRejectsReadOnlyStateChangeWithoutPrerequisite(t *testing.T) {
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"nextAction": map[string]any{"kind": "state_change"},
		},
		"convergenceContract": map[string]any{
			"state":            convergenceStateNeedStateChange,
			"runtimeStageMode": "target_snapshot_acquisition",
		},
		stepInputKeyStagePacket: map[string]any{
			"schemaVersion": "workorch.stage_packet.v1",
			"requiredOutput": map[string]any{
				"mustAcquireCurrentTargetSnapshot":   true,
				"mustUseReadOnlySnapshotAcquisition": true,
				"acceptedKinds":                      []any{"target_snapshot"},
				"forbiddenKinds":                     []any{"patch/apply"},
			},
		},
		"requiredOutput": map[string]any{
			"mustAcquireCurrentTargetSnapshot":   true,
			"mustUseReadOnlySnapshotAcquisition": true,
			"acceptedKinds":                      []any{"target_snapshot"},
			"forbiddenKinds":                     []any{"patch/apply"},
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"mustAcquireCurrentTargetSnapshot":   true,
				"mustUseReadOnlySnapshotAcquisition": true,
				"acceptedKinds":                      []any{"target_snapshot"},
				"forbiddenKinds":                     []any{"patch/apply"},
			},
		},
		currentActionContractKey: map[string]any{
			"nextAction": map[string]any{"kind": "state_change"},
			"requiredOutput": map[string]any{
				"mustAcquireCurrentTargetSnapshot":   true,
				"mustUseReadOnlySnapshotAcquisition": true,
				"acceptedKinds":                      []any{"target_snapshot"},
				"forbiddenKinds":                     []any{"patch/apply"},
			},
		},
	}

	err := validateConvergenceDispatchInput(input)
	if err == nil || !strings.Contains(err.Error(), "state_change action lacks state-changing output contract") {
		t.Fatalf("validateConvergenceDispatchInput error = %v, want read-only state_change without prerequisite rejection", err)
	}
}

func TestGraphLimitAssessmentOnlySkipsTargetSnapshotRealignment(t *testing.T) {
	run := &runModel.Run{
		ID:          "run-graph-limit-local-assessment",
		WorkspaceID: "ws-graph-limit-local-assessment",
		ExecutionContext: map[string]any{
			convergenceAuthorityKey: map[string]any{
				"nextAction": map[string]any{"kind": "state_change"},
			},
		},
	}
	step := &runModel.Step{
		ID:    "run-graph-limit-local-assessment/step-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyAssessmentOnly:     true,
			stepInputKeyGraphLimitFollowUp: true,
			stepInputKeyRuntimeStage:       runtimeStageVerification,
			stepInputKeyRuntimeStageMode:   "strict_convergence",
			"assessmentOnlyReason":         "graph_follow_up_expansion_limit_exceeded",
			"convergenceContract":          map[string]any{"state": convergenceStateNeedAssessment, "runtimeStageMode": "strict_convergence"},
			stepInputKeyStagePacket:        map[string]any{"schemaVersion": "workorch.stage_packet.v1"},
			"repairActionPackage":          map[string]any{"requiredOutput": assessmentRequiredOutputForAuthorityFocus(nil)},
			currentActionContractKey:       map[string]any{"nextAction": map[string]any{"kind": "assessment"}},
		},
	}

	realignLLMStepInputTargetSnapshotAuthority(run, step)

	contract := mapFromValue(step.Input["convergenceContract"])
	if stringFromMap(contract, "state") != convergenceStateNeedAssessment {
		t.Fatalf("convergenceContract = %+v, want assessment-only preserved", contract)
	}
	required := activeRequiredOutputForCurrentAction(step.Input)
	if boolValue(required["mustAcquireCurrentTargetSnapshot"]) {
		t.Fatalf("requiredOutput = %+v, want no target snapshot acquisition", required)
	}
}

func TestRuntimeManagedTargetSnapshotAbstractRefsFailFast(t *testing.T) {
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"nextAction": map[string]any{"kind": "acquire_target_snapshot"},
		},
		"convergenceContract": map[string]any{
			"state":            convergenceStateNeedEvidence,
			"runtimeStageMode": "target_snapshot_acquisition",
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"mustAcquireCurrentTargetSnapshot":   true,
				"mustUseReadOnlySnapshotAcquisition": true,
				"targetRefs":                         []any{"cwi-001", "source_change"},
				"acceptedKinds":                      []any{"target_snapshot"},
			},
		},
	}

	output, ok := runtimeManagedTargetSnapshotAcquisitionOutput(input)
	if !ok {
		t.Fatalf("runtimeManagedTargetSnapshotAcquisitionOutput ok = false, want fail-fast output")
	}
	if output["status"] != "failed" ||
		output[failureOutputKeyClass] != failureClassRuntimeContractMisalign ||
		output["deterministicOutcome"] != "fail_fast_internal_contract_gap" {
		t.Fatalf("output = %+v, want deterministic runtime contract failure", output)
	}
	if graph := output[stepOutputKeyExecutionGraph]; graph != nil {
		t.Fatalf("executionGraph = %+v, want no read-only graph for abstract refs", graph)
	}
}

func TestSynchronizeCurrentRequiredOutputMirrorsUsesCurrentActionCanonicalContract(t *testing.T) {
	canonical := stateChangingRepairRequiredOutput()
	staleValidation := map[string]any{
		"mustProduceConcreteValidationEvidence": true,
		"acceptedKinds":                         []string{"shell/command", managedValidationSessionNodeKind},
		"forbiddenKinds":                        []string{"patch/apply", "git/write"},
	}
	input := map[string]any{
		"convergenceContract": map[string]any{
			"state":                       convergenceStateNeedStateChange,
			"stateChangingOutputRequired": true,
		},
		"stateChangingOutputRequired": true,
		currentActionContractKey: map[string]any{
			"nextAction":     map[string]any{"kind": runtimemodel.NextActionTypeStateChange},
			"requiredOutput": cloneStepPayload(canonical),
		},
		"requiredOutput": cloneStepPayload(staleValidation),
		"repairActionPackage": map[string]any{
			"requiredOutput": cloneStepPayload(staleValidation),
		},
		stepInputKeyStagePacket: map[string]any{
			"nextAction": map[string]any{
				"kind":           runtimemodel.NextActionTypeStateChange,
				"requiredOutput": cloneStepPayload(staleValidation),
			},
			"requiredOutput": cloneStepPayload(staleValidation),
		},
	}

	got := synchronizeCurrentRequiredOutputMirrors(input)
	for label, required := range map[string]map[string]any{
		"requiredOutput":                        mapFromValue(got["requiredOutput"]),
		"repairActionPackage.requiredOutput":    mapFromValue(mapFromValue(got["repairActionPackage"])["requiredOutput"]),
		"stagePacket.requiredOutput":            mapFromValue(mapFromValue(got[stepInputKeyStagePacket])["requiredOutput"]),
		"stagePacket.nextAction.requiredOutput": mapFromValue(mapFromValue(mapFromValue(got[stepInputKeyStagePacket])["nextAction"])["requiredOutput"]),
		"currentActionContract.requiredOutput":  mapFromValue(mapFromValue(got[currentActionContractKey])["requiredOutput"]),
	} {
		if !requiredOutputRequiresStateChanging(required) ||
			!requiredOutputHasAllowedStateChangingKind(required) ||
			boolValue(required["mustProduceConcreteValidationEvidence"]) {
			t.Fatalf("%s = %+v, want current-action canonical state-changing requiredOutput", label, required)
		}
	}
}

func TestConvergenceDispatchSemanticsPrefersCurrentActionRequiredOutput(t *testing.T) {
	canonical := stateChangingRepairRequiredOutput()
	staleValidation := map[string]any{
		"mustProduceConcreteValidationEvidence": true,
		"acceptedKinds":                         []string{"shell/command", managedValidationSessionNodeKind},
		"forbiddenKinds":                        []string{"patch/apply", "git/write"},
	}
	input := map[string]any{
		"convergenceContract": map[string]any{
			"state":                       convergenceStateNeedStateChange,
			"stateChangingOutputRequired": true,
		},
		"stateChangingOutputRequired": true,
		currentActionContractKey: map[string]any{
			"nextAction":     map[string]any{"kind": runtimemodel.NextActionTypeStateChange},
			"requiredOutput": cloneStepPayload(canonical),
		},
		"repairActionPackage": map[string]any{
			"repairObjective": repairObjectiveStateChangingOutput,
			"requiredOutput":  cloneStepPayload(staleValidation),
		},
		stepInputKeyStagePacket: map[string]any{
			"nextAction":     map[string]any{"kind": runtimemodel.NextActionTypeStateChange},
			"requiredOutput": cloneStepPayload(staleValidation),
		},
		"requiredOutput": cloneStepPayload(staleValidation),
	}

	sources := map[string]map[string]any{
		"currentActionContract.requiredOutput": mapFromValue(mapFromValue(input[currentActionContractKey])["requiredOutput"]),
		"repairActionPackage.requiredOutput":   mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"]),
		"stagePacket.requiredOutput":           mapFromValue(mapFromValue(input[stepInputKeyStagePacket])["requiredOutput"]),
		"requiredOutput":                       mapFromValue(input["requiredOutput"]),
	}
	if selected := firstRequiredOutputForSemantics(sources); !requiredOutputRequiresStateChanging(selected) {
		t.Fatalf("firstRequiredOutputForSemantics() = %+v, want current-action state-changing contract", selected)
	}

	input = synchronizeCurrentRequiredOutputMirrors(input)
	if err := validateConvergenceDispatchRequiredOutputSemantics(input); err != nil {
		t.Fatalf("validateConvergenceDispatchRequiredOutputSemantics() error = %v after canonical synchronization; input=%+v", err, input)
	}
}
