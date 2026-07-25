package workorchruntimecontract

import (
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeCandidateFailureDiagnosticForFeasibilityUsesOnlySealedFacts(t *testing.T) {
	target := "directory"
	content := "replacement"
	candidate := RuntimeCandidate{StateChange: &StateChangeCandidate{Operations: []StateChangingOperation{{Operation: RuntimeOperation{
		ID: "edit-directory", Kind: StepKindFileEdit, TargetRefs: []string{target}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"coverage"},
		Parameters: []OperationParameter{{Name: "targetRef", String: &target}, {Name: "oldText", String: &content}, {Name: "newText", String: &content}},
	}}}}}
	decision := RuntimeDecision{Action: RuntimeActionProduceStateChange, RequiredOutput: RequiredOutputContract{
		Kind: RequiredOutputKindStateChange, AcceptedKinds: []string{StepKindFileEdit}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"coverage"},
	}}
	manifest, err := BuildRuntimeCandidateContractManifest(decision, nil)
	if err != nil {
		t.Fatal(err)
	}
	admissibility := RuntimeOperationAdmissibility{OwnerModuleID: RuntimeModuleFile, OperationKinds: []string{StepKindFileEdit}, TargetRefs: []string{target, "replacement.txt"}}.Normalized()
	diagnostic := RuntimeCandidateFailureDiagnosticForFeasibility(&FailureFact{
		Class: FailureClassCandidateContractFailure, Code: "operation_target_not_regular", Signature: "target-not-regular",
		Message: "owner-private directory detail must not become correction input", TargetRefs: []string{target},
	}, candidate.StateChange.Operations[0].Operation, manifest, admissibility, candidate)
	if diagnostic == nil {
		t.Fatal("missing diagnostic")
	}
	if diagnostic.OperationID != "edit-directory" || diagnostic.Path != "$.operations[id=edit-directory].targetRefs" || diagnostic.Actual != target || diagnostic.ManifestHash != manifest.Hash || diagnostic.RejectedShapeHash != StableRuntimeCandidateSemanticShape(candidate) {
		t.Fatalf("diagnostic identity=%+v", diagnostic)
	}
	if len(diagnostic.Allowed) != 1 || diagnostic.Allowed[0] != "replacement.txt" {
		t.Fatalf("diagnostic allowed=%+v", diagnostic.Allowed)
	}
	if len(diagnostic.Objectives) != 1 || diagnostic.Objectives[0] != RuntimeCandidateCorrectionReplaceTarget {
		t.Fatalf("diagnostic objectives=%+v", diagnostic.Objectives)
	}
	if rendered := diagnostic.Normalized(); strings.Contains(strings.Join(append(append([]string{}, rendered.Allowed...), rendered.Actual, rendered.Path), "|"), "owner-private") {
		t.Fatalf("owner message leaked into diagnostic: %+v", diagnostic)
	}
}

func TestRuntimeCandidateFailureDiagnosticForFeasibilityHasNoTargetObjectiveWithoutAlternative(t *testing.T) {
	target := "directory"
	candidate := RuntimeCandidate{StateChange: &StateChangeCandidate{Operations: []StateChangingOperation{{Operation: RuntimeOperation{ID: "edit-directory", Kind: StepKindFileEdit, TargetRefs: []string{target}}}}}}
	decision := RuntimeDecision{Action: RuntimeActionProduceStateChange, RequiredOutput: RequiredOutputContract{Kind: RequiredOutputKindStateChange, AcceptedKinds: []string{StepKindFileEdit}}}
	manifest, err := BuildRuntimeCandidateContractManifest(decision, nil)
	if err != nil {
		t.Fatal(err)
	}
	diagnostic := RuntimeCandidateFailureDiagnosticForFeasibility(&FailureFact{Class: FailureClassCandidateContractFailure, Code: "operation_target_not_regular", Signature: "target-not-regular", Message: "directory", TargetRefs: []string{target}}, candidate.StateChange.Operations[0].Operation, manifest, RuntimeOperationAdmissibility{OwnerModuleID: RuntimeModuleFile, OperationKinds: []string{StepKindFileEdit}, TargetRefs: []string{target}}, candidate)
	if diagnostic == nil || len(diagnostic.Allowed) != 0 || len(diagnostic.Objectives) != 0 {
		t.Fatalf("target without alternative became a correction instruction: %+v", diagnostic)
	}
}

func TestRuntimeCandidateFeasibilityCorrectionRoundTripsSameManifest(t *testing.T) {
	initialTarget := "directory"
	replacementTarget := "replacement.txt"
	content := "replacement"
	initial := RuntimeCandidate{StateChange: &StateChangeCandidate{Operations: []StateChangingOperation{{Operation: RuntimeOperation{
		ID: "edit-directory", Kind: StepKindFileEdit, TargetRefs: []string{initialTarget}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"coverage"},
		Parameters: []OperationParameter{{Name: "path", String: &initialTarget}, {Name: "oldText", String: &content}, {Name: "newText", String: &content}},
	}}}}}
	decision := RuntimeDecision{Action: RuntimeActionProduceStateChange, RequiredOutput: RequiredOutputContract{
		Kind: RequiredOutputKindStateChange, AcceptedKinds: []string{StepKindFileEdit}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"coverage"},
	}}
	manifest, err := BuildRuntimeCandidateContractManifest(decision, nil)
	if err != nil {
		t.Fatal(err)
	}
	diagnostic := RuntimeCandidateFailureDiagnosticForFeasibility(
		&FailureFact{Class: FailureClassCandidateContractFailure, Code: "operation_target_not_regular", Signature: "target-not-regular", TargetRefs: []string{initialTarget}},
		initial.StateChange.Operations[0].Operation,
		manifest,
		RuntimeOperationAdmissibility{OwnerModuleID: RuntimeModuleFile, OperationKinds: []string{StepKindFileEdit}, TargetRefs: []string{initialTarget, replacementTarget}},
		initial,
	)
	if diagnostic == nil || len(diagnostic.Objectives) != 1 || diagnostic.Objectives[0] != RuntimeCandidateCorrectionReplaceTarget || len(diagnostic.Allowed) != 1 || diagnostic.Allowed[0] != replacementTarget {
		t.Fatalf("feasibility correction diagnostic = %+v", diagnostic)
	}
	corrected := RuntimeCandidate{StateChange: &StateChangeCandidate{Operations: []StateChangingOperation{{Operation: RuntimeOperation{
		ID: "edit-replacement", Kind: StepKindFileEdit, TargetRefs: []string{replacementTarget}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"coverage"},
		Parameters: []OperationParameter{{Name: "path", String: &replacementTarget}, {Name: "oldText", String: &content}, {Name: "newText", String: &content}},
	}}}}}
	payload, err := EncodeRuntimeCandidatePayloadWithManifest(manifest, corrected)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeRuntimeCandidatePayloadWithManifest(manifest, payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeCandidateAgainstManifest(manifest, decoded); err != nil {
		t.Fatal(err)
	}
	if got, want := StableRuntimeCandidateHash(decoded), StableRuntimeCandidateHash(corrected); got != want || StableRuntimeCandidateSemanticShape(decoded) == StableRuntimeCandidateSemanticShape(initial) {
		t.Fatalf("same-manifest correction did not preserve corrected structure: decoded=%+v payload=%s", decoded, payload)
	}
}
