package orchestrator

import (
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestConvergenceStateMachineMapsDispatchableNextActions(t *testing.T) {
	tests := []struct {
		nextAction string
		wantState  string
	}{
		{nextAction: runtimemodel.NextActionTypeAcquireTargetSnapshot, wantState: convergenceStateNeedEvidence},
		{nextAction: runtimemodel.NextActionTypeStateChange, wantState: convergenceStateNeedStateChange},
		{nextAction: runtimemodel.NextActionTypeValidate, wantState: convergenceStateNeedValidation},
		{nextAction: runtimemodel.NextActionTypeAssessment, wantState: convergenceStateNeedAssessment},
		{nextAction: runtimemodel.NextActionTypeRepair, wantState: convergenceStateNeedEvidence},
		{nextAction: runtimemodel.NextActionTypeRequestDecision, wantState: convergenceStateNeedGovernance},
		{nextAction: runtimemodel.NextActionTypeRequestApproval, wantState: convergenceStateNeedGovernance},
		{nextAction: "produce_state_change", wantState: convergenceStateNeedStateChange},
		{nextAction: "produce_validation_evidence", wantState: convergenceStateNeedValidation},
		{nextAction: "complete_coverage_assessment", wantState: convergenceStateNeedAssessment},
		{nextAction: "acquire_evidence", wantState: convergenceStateNeedEvidence},
	}
	for _, tt := range tests {
		t.Run(tt.nextAction, func(t *testing.T) {
			got, ok := convergenceContractStateForNextAction(tt.nextAction)
			if !ok {
				t.Fatalf("convergenceContractStateForNextAction(%q) ok=false", tt.nextAction)
			}
			if got != tt.wantState {
				t.Fatalf("convergenceContractStateForNextAction(%q) = %q, want %q", tt.nextAction, got, tt.wantState)
			}
		})
	}
}

func TestConvergenceDispatchStateMachineRejectsForbiddenCombinations(t *testing.T) {
	tests := []struct {
		name       string
		state      string
		nextAction string
		required   map[string]any
		want       string
	}{
		{
			name:       "state-change-next-action-cannot-use-validation-contract",
			state:      convergenceStateNeedValidation,
			nextAction: runtimemodel.NextActionTypeStateChange,
			required:   concreteValidationEvidenceRequiredOutput(nil),
			want:       "contract_state_mismatch_next_action",
		},
		{
			name:       "validation-next-action-cannot-use-state-change-contract",
			state:      convergenceStateNeedStateChange,
			nextAction: runtimemodel.NextActionTypeValidate,
			required:   stateChangingRepairRequiredOutput(),
			want:       "contract_state_mismatch_next_action",
		},
		{
			name:       "validation-cannot-accept-git-write",
			state:      convergenceStateNeedValidation,
			nextAction: runtimemodel.NextActionTypeValidate,
			required: map[string]any{
				"mustIncludeStateChangingNode":          false,
				"mustProduceConcreteValidationEvidence": true,
				"acceptedKinds":                         []string{runtimemodel.StepKindGitWrite},
			},
			want: "need_validation is not validation-only",
		},
		{
			name:       "assessment-cannot-accept-state-changing-output",
			state:      convergenceStateNeedAssessment,
			nextAction: runtimemodel.NextActionTypeAssessment,
			required:   stateChangingRepairRequiredOutput(),
			want:       "need_assessment is not assessment-only",
		},
		{
			name:       "evidence-acquisition-cannot-accept-patch",
			state:      convergenceStateNeedEvidence,
			nextAction: runtimemodel.NextActionTypeRepair,
			required: map[string]any{
				"mustIncludeStateChangingNode": true,
				"acceptedKinds":                []string{"patch/apply"},
			},
			want: "need_evidence exposes state-changing output",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConvergenceDispatchRequiredOutputSemantics(stateMachineDispatchInput(tt.state, tt.nextAction, tt.required))
			if err == nil {
				t.Fatalf("validateConvergenceDispatchRequiredOutputSemantics error = nil, want %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want to contain %q", err.Error(), tt.want)
			}
		})
	}
}

func TestConvergenceDispatchStateMachineAllowsCanonicalShapes(t *testing.T) {
	targetSnapshotRequired := evidenceAcquisitionRequiredOutput(nil)
	targetSnapshotRequired["mustAcquireCurrentTargetSnapshot"] = true
	targetSnapshotRequired["mustUseReadOnlySnapshotAcquisition"] = true
	targetSnapshotRequired["mustIncludeStateChangingNode"] = false

	tests := []struct {
		name       string
		state      string
		nextAction string
		required   map[string]any
	}{
		{
			name:       "target-snapshot",
			state:      convergenceStateNeedEvidence,
			nextAction: runtimemodel.NextActionTypeAcquireTargetSnapshot,
			required:   targetSnapshotRequired,
		},
		{
			name:       "state-change",
			state:      convergenceStateNeedStateChange,
			nextAction: runtimemodel.NextActionTypeStateChange,
			required:   stateChangingRepairRequiredOutput(),
		},
		{
			name:       "validation",
			state:      convergenceStateNeedValidation,
			nextAction: runtimemodel.NextActionTypeValidate,
			required:   concreteValidationEvidenceRequiredOutput(nil),
		},
		{
			name:       "assessment",
			state:      convergenceStateNeedAssessment,
			nextAction: runtimemodel.NextActionTypeAssessment,
			required:   assessmentRequiredOutputForAuthorityFocus(map[string]any{"id": "cwi-1"}),
		},
		{
			name:       "evidence",
			state:      convergenceStateNeedEvidence,
			nextAction: runtimemodel.NextActionTypeRepair,
			required:   evidenceAcquisitionRequiredOutput(nil),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateConvergenceDispatchRequiredOutputSemantics(stateMachineDispatchInput(tt.state, tt.nextAction, tt.required)); err != nil {
				t.Fatalf("validateConvergenceDispatchRequiredOutputSemantics error = %v", err)
			}
		})
	}
}

func TestCurrentAuthoritySnapshotLineageSealsDerivedMirrors(t *testing.T) {
	input := sealCurrentAuthoritySnapshotLineage(stateMachineDispatchInput(
		convergenceStateNeedStateChange,
		runtimemodel.NextActionTypeStateChange,
		stateChangingRepairRequiredOutput(),
	))
	lineage := mapFromValue(input["currentAuthoritySnapshot"])
	id := stringFromMap(lineage, "id")
	if id == "" {
		t.Fatalf("currentAuthoritySnapshot = %+v, want id", lineage)
	}
	for label, value := range map[string]map[string]any{
		"convergenceContract":    mapFromValue(input["convergenceContract"]),
		"repairActionPackage":    mapFromValue(input["repairActionPackage"]),
		"currentActionContract":  mapFromValue(input[currentActionContractKey]),
		"stagePacket":            mapFromValue(input[stepInputKeyStagePacket]),
		"stagePacket.nextAction": mapFromValue(mapFromValue(input[stepInputKeyStagePacket])["nextAction"]),
	} {
		if got := stringFromMap(value, "authoritySnapshotId"); got != id {
			t.Fatalf("%s authoritySnapshotId = %q, want %q; value=%+v", label, got, id, value)
		}
	}
	if err := validateCurrentAuthoritySnapshotLineage(input); err != nil {
		t.Fatalf("validateCurrentAuthoritySnapshotLineage error = %v", err)
	}
	packet := mapFromValue(input[stepInputKeyStagePacket])
	packet["authoritySnapshotId"] = "auth-stale"
	input[stepInputKeyStagePacket] = packet
	err := validateCurrentAuthoritySnapshotLineage(input)
	if err == nil {
		t.Fatal("validateCurrentAuthoritySnapshotLineage error = nil, want mismatch")
	}
	if !strings.Contains(err.Error(), "authority snapshot lineage mismatch") {
		t.Fatalf("error = %v, want authority snapshot mismatch", err)
	}
}

func stateMachineDispatchInput(state string, nextAction string, required map[string]any) map[string]any {
	return map[string]any{
		convergenceAuthorityKey: map[string]any{
			"schemaVersion": convergenceAuthoritySchema,
			"nextAction":    map[string]any{"kind": nextAction},
			"currentFocus": map[string]any{
				"id":                  "cwi-1",
				"coverageWorkItemIds": []string{"cwi-1"},
			},
		},
		"convergenceContract": map[string]any{
			"state": state,
		},
		"requiredOutput": cloneStepPayload(required),
		"repairActionPackage": map[string]any{
			"requiredOutput": cloneStepPayload(required),
		},
		currentActionContractKey: map[string]any{
			"nextAction":     map[string]any{"kind": nextAction},
			"requiredOutput": cloneStepPayload(required),
		},
		stepInputKeyStagePacket: map[string]any{
			"nextAction":     map[string]any{"kind": nextAction, "requiredOutput": cloneStepPayload(required)},
			"requiredOutput": cloneStepPayload(required),
			"workspaceId":    string(common.WorkspaceID("ws-state-machine")),
			"runId":          string(common.RunID("run-state-machine")),
		},
	}
}
