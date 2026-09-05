package runtime

import (
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestSealedDispatchToRuntimeRecordPreservesRawAuthorityProjectionShape(t *testing.T) {
	repair := runtimecontract.RuntimeDispatchRepairCommitment{
		Purpose:             runtimecontract.RuntimeDispatchRepairPurpose(" confirmed_validation_repair "),
		RepairOwnerModuleID: " blocks/synthetic-repair ",
		OriginDispatchID:    " dispatch-origin ",
		Closure: runtimecontract.RuntimeRepairClosure{
			CoverageWorkItemIDs:   []common.CoverageWorkItemID{"cwi-2", "cwi-1", "cwi-1"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation, runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation},
			GroundedTargetRefs:    []string{"other.go", "main.go", "main.go"},
		},
	}
	authority := runtimecontract.RuntimeAssessmentAuthorityInput{
		SourceText: " definition source ",
		Focus: runtimecontract.CoverageFocus{
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-2", "cwi-1", "cwi-1"},
			TargetRefs:          []string{"other.go", "main.go", "main.go"},
		},
	}
	adjudication := runtimecontract.RuntimeAssessmentAdjudication{
		DispatchID:          " dispatch-assessment ",
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-2", "cwi-1", "cwi-1"},
		Proofs: []runtimecontract.RuntimeAssessmentAdjudicationProof{
			{CoverageWorkItemID: "cwi-2", EvidenceKind: runtimecontract.EvidenceKindValidation, EvidenceFactID: "evidence-2", EvidenceFactHash: "hash-2"},
			{CoverageWorkItemID: "cwi-1", EvidenceKind: runtimecontract.EvidenceKindValidation, EvidenceFactID: "evidence-1", EvidenceFactHash: "hash-1"},
			{CoverageWorkItemID: "cwi-1", EvidenceKind: runtimecontract.EvidenceKindValidation, EvidenceFactID: "evidence-1", EvidenceFactHash: "hash-1"},
		},
	}
	binding := runtimecontract.CoverageEvidenceBinding{
		ID: " binding-1 ", TargetRefs: []string{"other.go", "main.go", "main.go"},
	}
	targetAuthorization := runtimecontract.RuntimeDispatchOperationTargetAuthorization{
		OperationID: " operation-1 ",
		Target: runtimecontract.RuntimeOperationTargetAuthorization{
			Policy:          runtimecontract.RuntimeOperationTargetPolicyExactTarget,
			ExactTargetRefs: []string{"other.go", "main.go", "main.go"},
		},
	}
	recipe := runtimecontract.RuntimeValidationRecipe{
		ID: " recipe-1 ", Command: " go ", Args: []string{" test ", " ./... "},
	}
	dispatch := SealedDispatch{
		CoverageBindings:              []runtimecontract.CoverageEvidenceBinding{binding},
		OperationTargetAuthorizations: []runtimecontract.RuntimeDispatchOperationTargetAuthorization{targetAuthorization},
		ValidationRecipes:             []runtimecontract.RuntimeValidationRecipe{recipe},
		RepairCommitment:              &repair,
		AssessmentAuthorityInput:      &authority,
		AssessmentAdjudication:        &adjudication,
	}
	record := sealedDispatchToRuntimeRecord(dispatch)
	if !reflect.DeepEqual(record.CoverageBindings, dispatch.CoverageBindings) ||
		!reflect.DeepEqual(record.OperationTargetAuthorizations, dispatch.OperationTargetAuthorizations) ||
		!reflect.DeepEqual(record.ValidationRecipes, dispatch.ValidationRecipes) {
		t.Fatalf("existing dispatch authority raw shape was normalized during conversion: got=%+v", record)
	}
	if !reflect.DeepEqual(record.RepairCommitment, dispatch.RepairCommitment) {
		t.Fatalf("repair commitment raw shape was normalized during conversion: got=%+v want=%+v", record.RepairCommitment, dispatch.RepairCommitment)
	}
	if !reflect.DeepEqual(record.AssessmentAuthorityInput, dispatch.AssessmentAuthorityInput) {
		t.Fatalf("assessment authority raw shape was normalized during conversion: got=%+v want=%+v", record.AssessmentAuthorityInput, dispatch.AssessmentAuthorityInput)
	}
	if !reflect.DeepEqual(record.AssessmentAdjudication, dispatch.AssessmentAdjudication) {
		t.Fatalf("assessment adjudication raw shape was normalized during conversion: got=%+v want=%+v", record.AssessmentAdjudication, dispatch.AssessmentAdjudication)
	}

	dispatch.RepairCommitment.Closure.RequiredEvidenceKinds[0] = "mutated"
	dispatch.AssessmentAuthorityInput.Focus.TargetRefs[0] = "mutated"
	dispatch.AssessmentAdjudication.Proofs[0].EvidenceFactHash = "mutated"
	dispatch.CoverageBindings[0].TargetRefs[0] = "mutated"
	dispatch.OperationTargetAuthorizations[0].Target.ExactTargetRefs[0] = "mutated"
	dispatch.ValidationRecipes[0].Args[0] = "mutated"
	if record.CoverageBindings[0].TargetRefs[0] == "mutated" ||
		record.OperationTargetAuthorizations[0].Target.ExactTargetRefs[0] == "mutated" ||
		record.ValidationRecipes[0].Args[0] == "mutated" ||
		record.RepairCommitment.Closure.RequiredEvidenceKinds[0] == "mutated" ||
		record.AssessmentAuthorityInput.Focus.TargetRefs[0] == "mutated" ||
		record.AssessmentAdjudication.Proofs[0].EvidenceFactHash == "mutated" {
		t.Fatal("raw dispatch authority conversion retained mutable source aliases")
	}
}

func TestRuntimeAggregateFreshLoadRejectsRawDispatchProjectionLaundering(t *testing.T) {
	stateFacts := baseDispatchFacts()
	stateFacts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
	stateDefinition := dispatchDefinition()
	stateDefinition.CapabilityContract = []string{runtimecontract.RuntimeBlockKindPatchApply}
	stateAggregate := RuntimeAggregate{StateRevision: 1, Definition: stateDefinition, Facts: stateFacts}
	runtimeTestEnsureCurrentFocusBindings(t, &stateAggregate)
	stateRepository := &durableRuntimeStateRepository{aggregate: stateAggregate}
	stateBaseline := sealCandidateForTest(t, stateRepository, stateChangeCandidate())
	if err := stateBaseline.Validate(); err != nil {
		t.Fatalf("state-change raw projection baseline: %v", err)
	}

	for _, test := range []struct {
		name      string
		wantError string
		mutate    func(*SealedDispatch)
	}{
		{
			name:      "coverage binding",
			wantError: "coverage evidence binding projection is not canonical",
			mutate: func(dispatch *SealedDispatch) {
				binding := &dispatch.CoverageBindings[0]
				binding.TargetRefs = append(binding.TargetRefs, binding.TargetRefs[0])
				binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(*binding)
				dispatch.CoverageBindingHash = runtimecontract.StableRuntimeCoverageEvidenceBindingSnapshotHash(dispatch.CoverageBindings)
			},
		},
		{
			name:      "target authorization",
			wantError: "target authorization projection is not canonical",
			mutate: func(dispatch *SealedDispatch) {
				authorization := &dispatch.OperationTargetAuthorizations[0]
				authorization.Target.ExactTargetRefs = append(authorization.Target.ExactTargetRefs, authorization.Target.ExactTargetRefs[0])
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			tampered := cloneRuntimeAggregateForMCPProvenanceTest(t, stateBaseline)
			test.mutate(tampered.ActiveDispatch)
			tampered.ActiveDispatch.DispatchAuthorityHash = runtimecontract.StableRuntimeDispatchAuthorityHash(
				sealedDispatchToRuntimeRecord(*tampered.ActiveDispatch),
			)
			if err := tampered.Validate(); err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("raw %s laundering error = %v", test.name, err)
			}
		})
	}

	validationRepository := &durableRuntimeStateRepository{aggregate: validationAggregate(t)}
	validationBaseline := sealCandidateForTest(t, validationRepository, validationCandidate())
	if err := validationBaseline.Validate(); err != nil {
		t.Fatalf("validation raw projection baseline: %v", err)
	}
	tamperedRecipe := cloneRuntimeAggregateForMCPProvenanceTest(t, validationBaseline)
	tamperedRecipe.ActiveDispatch.ValidationRecipes[0].ID = " " + tamperedRecipe.ActiveDispatch.ValidationRecipes[0].ID + " "
	tamperedRecipe.ActiveDispatch.DispatchAuthorityHash = runtimecontract.StableRuntimeDispatchAuthorityHash(
		sealedDispatchToRuntimeRecord(*tamperedRecipe.ActiveDispatch),
	)
	if err := tamperedRecipe.Validate(); err == nil || !strings.Contains(err.Error(), "canonical raw recipe fields") {
		t.Fatalf("raw validation recipe laundering error = %v", err)
	}
}
