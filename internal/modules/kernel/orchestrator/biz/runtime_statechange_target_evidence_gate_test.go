package orchestrator

import (
	"testing"

	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
)

func TestFirstRunStateChangeWithNonEmptyManifestAcquiresTargetEvidence(t *testing.T) {
	input := map[string]any{
		gapBaselineKey: map[string]any{
			"nextAction": map[string]any{"kind": "state_change"},
		},
		coverageWorkItemContextKey: map[string]any{
			"currentFocus": map[string]any{
				"id":                    "cwi-primary",
				"source":                "deliverable",
				"sourceScope":           "deliverable",
				"sourceRequirementId":   "source_change",
				"targetText":            "source_change",
				"status":                "open",
				"requiredEvidenceKinds": []string{"state_change", "validation"},
				"missingEvidenceKinds":  []string{"state_change", "validation"},
			},
		},
		stepInputKeyExecutionContextPackage: map[string]any{
			stepInputKeyWorkspaceFileManifest: []map[string]any{{
				"kind": "file",
				"path": "target/resource.txt",
				"size": 12,
			}},
		},
	}

	got := applyFirstRunGapBaselineStateChangingContract(input)

	if got["repairObjective"] != repairObjectiveAcquireFailureEvidence ||
		!boolValue(got["concreteFailureEvidenceRequired"]) ||
		boolValue(got["stateChangingOutputRequired"]) ||
		boolValue(got["emptyWorkspaceEvidenceEscalatedToImplementation"]) {
		t.Fatalf("input = %+v, want non-empty workspace to acquire target evidence before state change", got)
	}
	pkg := mapFromValue(got["repairActionPackage"])
	if !boolValue(pkg["evidenceAcquisitionRequired"]) {
		t.Fatalf("repairActionPackage = %+v, want evidence acquisition required", pkg)
	}
	required := mapFromValue(pkg["requiredOutput"])
	if !boolValue(required["mustProduceConcreteFailureEvidence"]) ||
		boolValue(required["mustIncludeStateChangingNode"]) ||
		!boolValue(required["patchForbiddenUntilConcreteTargetEvidence"]) {
		t.Fatalf("requiredOutput = %+v, want bounded evidence acquisition contract", required)
	}
}

func TestFirstRunStateChangeWithoutManifestKeepsEmptyWorkspaceEscalation(t *testing.T) {
	input := map[string]any{
		gapBaselineKey: map[string]any{
			"nextAction": map[string]any{"kind": "state_change"},
		},
		coverageWorkItemContextKey: map[string]any{
			"currentFocus": map[string]any{
				"id":                    "cwi-primary",
				"source":                "deliverable",
				"sourceScope":           "deliverable",
				"sourceRequirementId":   "source_change",
				"targetText":            "source_change",
				"status":                "open",
				"requiredEvidenceKinds": []string{"state_change"},
				"missingEvidenceKinds":  []string{"state_change"},
			},
		},
	}

	got := applyFirstRunGapBaselineStateChangingContract(input)

	if got["repairObjective"] != repairObjectiveStateChangingOutput ||
		!boolValue(got["stateChangingOutputRequired"]) ||
		!boolValue(got["emptyWorkspaceEvidenceEscalatedToImplementation"]) {
		t.Fatalf("input = %+v, want true empty workspace state-changing escalation", got)
	}
}

func TestStateChangingRepairWithNonEmptyManifestAndNoTargetSnapshotRequiresEvidenceAcquisition(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeStage:      runtimeStageRepair,
		"repairObjective":             repairObjectiveStateChangingOutput,
		"stateChangingOutputRequired": true,
		stepInputKeyExecutionContextPackage: map[string]any{
			stepInputKeyWorkspaceFileManifest: []map[string]any{{
				"kind": "file",
				"path": "target/resource.txt",
				"size": 12,
			}},
		},
	}
	required := stateChangingRepairRequiredOutput()
	pkg := map[string]any{"requiredOutput": required}

	applyConcreteRepairEvidenceOutputContract(input, pkg, required)

	if !boolValue(pkg["evidenceAcquisitionRequired"]) ||
		stringFromMap(pkg, "evidenceAcquisitionReason") != "state_changing_repair_target_evidence_missing" {
		t.Fatalf("repairActionPackage = %+v, want target evidence acquisition gate", pkg)
	}
	gotRequired := mapFromValue(pkg["requiredOutput"])
	if boolValue(gotRequired["mustIncludeStateChangingNode"]) ||
		!boolValue(gotRequired["mustProduceConcreteFailureEvidence"]) ||
		!boolValue(gotRequired["patchForbiddenUntilConcreteTargetEvidence"]) {
		t.Fatalf("requiredOutput = %+v, want evidence acquisition before patch contract", gotRequired)
	}
	if !boolValue(gotRequired["acceptedKindsOnly"]) || !stringSliceContains(stringSliceFromValue(gotRequired["forbiddenKinds"]), "patch/apply") {
		t.Fatalf("requiredOutput = %+v, want bounded evidence-only accepted kinds", gotRequired)
	}
}

func TestDecisionWithVisibleWorkspaceFilesAndCandidateTargetPathShouldRepair(t *testing.T) {
	step := &runmodel.Step{
		ID:   "run-readable-visible/step-1",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
			"repairActionPackage": map[string]any{
				"requiredOutput": stateChangingRepairRequiredOutput(),
			},
		},
	}
	spec := runtimeDecisionBlockSpec{
		Reason:       "missing current target snapshot",
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     "Please provide the current target snapshot.",
		Context: map[string]any{
			"knownValues": map[string]any{
				"visibleWorkspaceFiles": []any{"target/resource.txt"},
			},
			"candidateValues": map[string]any{
				"repairTargetSnapshot.path": []any{"target/resource.txt"},
			},
			"missingFields":       []any{"repairTargetSnapshot.content"},
			"requiresHumanFields": []any{"repairTargetSnapshot.content"},
			"runtimeCanContinue":  false,
		},
	}

	if !knownReadableWorkspaceResourceDecisionSpec(spec) {
		t.Fatalf("knownReadableWorkspaceResourceDecisionSpec = false, want visible workspace files treated as readable resources")
	}
	if !continuableDecisionShouldRepair(step, spec) {
		t.Fatalf("continuableDecisionShouldRepair = false, want runtime-readable target snapshot request converted to repair")
	}
}

func TestDecisionWithProviderDependencyAndReadableResourceStaysGoverned(t *testing.T) {
	step := &runmodel.Step{
		ID:   "run-provider-governed/step-1",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
			"repairActionPackage": map[string]any{
				"requiredOutput": stateChangingRepairRequiredOutput(),
			},
		},
	}
	spec := runtimeDecisionBlockSpec{
		Reason:       "provider configuration required",
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     "Fix the runtime provider configuration before continuing.",
		Context: map[string]any{
			decisionContextKeyScope:          decisionContextScopeWorkorchRuntime,
			decisionContextKeyDependency:     decisionContextDependencyLLMProvider,
			decisionContextKeyRecoveryPolicy: decisionContextRecoveryPolicyConfigOnly,
			"knownValues": map[string]any{
				"visibleWorkspaceFiles": []any{"target/resource.txt"},
			},
			"candidateValues": map[string]any{
				"repairTargetSnapshot.path": []any{"target/resource.txt"},
			},
		},
	}

	if knownReadableWorkspaceResourceDecisionSpec(spec) {
		t.Fatalf("knownReadableWorkspaceResourceDecisionSpec = true, want provider dependency to stay governed")
	}
	if continuableDecisionShouldRepair(step, spec) {
		t.Fatalf("continuableDecisionShouldRepair = true, want provider dependency decision to stay governed")
	}
}
