package candidateadmission

import (
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRejectedShapeReplayDistinguishesFullAndPartialReplay(t *testing.T) {
	first := runtimecontract.RuntimeOperation{ID: "first", Kind: runtimecontract.StepKindFileWrite, TargetRefs: []string{"main.go"}}
	second := runtimecontract.RuntimeOperation{ID: "second", Kind: runtimecontract.StepKindFileRead, TargetRefs: []string{"main.go"}}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: first}, {Operation: second}}}}
	shape := runtimecontract.RuntimeRejectedOperationShape{Operation: first}
	if finding, ok := RejectedShapeReplay(candidate, []runtimecontract.RuntimeRejectedOperationShape{shape}); !ok || finding.Code != CodePartialReplay {
		t.Fatalf("partial replay = %+v/%t", finding, ok)
	}
	full := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: first}}}}
	if finding, ok := RejectedShapeReplay(full, []runtimecontract.RuntimeRejectedOperationShape{shape}); !ok || finding.Code != CodeRejectedShape {
		t.Fatalf("full replay = %+v/%t", finding, ok)
	}
}

func TestNoProgressAndSemanticReplayArePureShapeFences(t *testing.T) {
	operation := runtimecontract.RuntimeOperation{ID: "write", Kind: runtimecontract.StepKindFileWrite, BindingRefs: []string{"binding-generation-1"}, TargetRefs: []string{"main.go"}}
	successor := operation
	successor.ID = "write-successor"
	successor.BindingRefs = []string{"binding-generation-2"}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: successor}}}}
	shape := runtimecontract.RuntimeRejectedOperationShape{Operation: operation}
	if finding, ok := NoProgressReplay(candidate, []runtimecontract.RuntimeRejectedOperationShape{shape}); !ok || finding.Code != CodeNoProgressShape {
		t.Fatalf("no-progress replay = %+v/%t", finding, ok)
	}
	wanted := runtimecontract.StableRuntimeCandidateSemanticShape(candidate)
	if finding, ok := CandidateSemanticReplay(candidate, []string{wanted}); !ok || finding.Code != CodeRejectedCandidate {
		t.Fatalf("semantic replay = %+v/%t", finding, ok)
	}
	if _, ok := CandidateSemanticReplay(candidate, []string{"different"}); ok {
		t.Fatal("different semantic shape was rejected")
	}
}

func TestOperationsDoesNotExposeAssessmentOrGovernanceAsReplaySurface(t *testing.T) {
	if got := Operations(runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{}}); got != nil {
		t.Fatalf("assessment operations = %+v", got)
	}
	if got := Operations(runtimecontract.RuntimeCandidate{Governance: &runtimecontract.GovernanceCandidate{}}); got != nil {
		t.Fatalf("governance operations = %+v", got)
	}
}

func TestEvaluateUsesOneStableStructuralThenReplayOrder(t *testing.T) {
	facts, decision, manifest, candidate := evaluatorFixture(t)
	input := EvaluationInput{
		Manifest:                manifest,
		Decision:                decision,
		Facts:                   facts,
		Candidate:               candidate,
		RejectedOperationShapes: []runtimecontract.RuntimeRejectedOperationShape{{Operation: candidate.StateChange.Operations[0].Operation}},
	}
	before := input

	invalid := candidate
	invalid.StateChange = &runtimecontract.StateChangeCandidate{Operations: append([]runtimecontract.StateChangingOperation(nil), candidate.StateChange.Operations...)}
	invalid.StateChange.Operations[0].Operation.TargetRefs = []string{"outside.go"}
	invalid, err := runtimecontract.DeriveRuntimeCandidateOperationAuthority(manifest, invalid)
	if err != nil {
		t.Fatal(err)
	}
	input.Candidate = invalid
	input.RejectedOperationShapes = []runtimecontract.RuntimeRejectedOperationShape{{Operation: invalid.StateChange.Operations[0].Operation}}
	if failure := Evaluate(input); failure == nil || failure.Code != CodeInvalid || failure.Diagnostic == nil || failure.Diagnostic.Path == "" {
		t.Fatalf("wire/target violation did not precede replay: %+v", failure)
	}

	input.Candidate = candidate
	input.RejectedOperationShapes = before.RejectedOperationShapes
	if failure := Evaluate(input); failure == nil || failure.Code != CodeRejectedShape || failure.Diagnostic == nil ||
		failure.Diagnostic.ManifestHash != manifest.Hash || failure.Diagnostic.RejectedShapeHash != runtimecontract.StableRuntimeCandidateSemanticShape(candidate) {
		t.Fatalf("valid structural replay = %+v", failure)
	}

	input.RejectedOperationShapes = nil
	if failure := Evaluate(input); failure != nil {
		t.Fatalf("valid candidate rejected: %+v", failure)
	}
	if !reflect.DeepEqual(before.Manifest, input.Manifest) || !reflect.DeepEqual(before.Decision, input.Decision) || !reflect.DeepEqual(before.Facts, input.Facts) {
		t.Fatal("candidate evaluator mutated authority input")
	}
}

func TestEvaluateRepairClosurePrecedesReplay(t *testing.T) {
	facts, decision, _, candidate := evaluatorFixture(t)
	closure := runtimecontract.RuntimeRepairClosure{
		GapSignature:             "gap-v1",
		BaselineEvidenceRevision: facts.EvidenceRevision,
		CoverageWorkItemIDs:      facts.Focus.CoverageWorkItemIDs,
		RequiredEvidenceKinds:    []string{runtimecontract.EvidenceKindStateChange},
		GroundedTargetRefs:       []string{"other.go"},
	}
	closure.ID = runtimecontract.StableRuntimeRepairClosureID(closure)
	directive := &runtimecontract.RuntimeRepairDirective{
		ID: "repair-v1", FocusID: facts.Focus.ID,
		RequiredState: runtimecontract.ConvergenceStateNeedStateChange,
		Closure:       &closure,
	}
	decision.RequiredOutput.RepairClosureID = closure.ID
	var err error
	decision, err = runtimecontract.SealRuntimeDecision(facts, decision, 0)
	if err != nil {
		t.Fatal(err)
	}
	facts.RepairDirective = directive
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(decision, runtimecontract.RuntimeCandidateManifestOwnerFacts{})
	if err != nil {
		t.Fatal(err)
	}
	candidate, err = runtimecontract.DeriveRuntimeCandidateOperationAuthority(manifest, candidate)
	if err != nil {
		t.Fatal(err)
	}
	failure := Evaluate(EvaluationInput{
		Manifest: manifest, Decision: decision, Facts: facts, Candidate: candidate,
		RejectedOperationShapes: []runtimecontract.RuntimeRejectedOperationShape{{Operation: candidate.StateChange.Operations[0].Operation}},
	})
	if failure == nil || failure.Code != CodeRepairClosureMismatch || failure.Diagnostic == nil ||
		!reflect.DeepEqual(failure.Diagnostic.Allowed, []string{"other.go"}) {
		t.Fatalf("repair closure did not precede replay: %+v", failure)
	}
}

func TestEvaluateRejectsDistinctDurableOperationsForSameOwnerTarget(t *testing.T) {
	facts, decision, manifest, candidate := evaluatorFixture(t)
	second := candidate.StateChange.Operations[0].Operation
	second.ID = ""
	second.Parameters = append([]runtimecontract.OperationParameter(nil), second.Parameters...)
	for index := range second.Parameters {
		if second.Parameters[index].Name == "content" {
			value := "second update\n"
			second.Parameters[index].String = &value
		}
	}
	candidate.StateChange.Operations = append(candidate.StateChange.Operations, runtimecontract.StateChangingOperation{Operation: second})
	var err error
	candidate, err = runtimecontract.DeriveRuntimeCandidateOperationAuthority(manifest, candidate)
	if err != nil {
		t.Fatal(err)
	}
	secondOperationID := candidate.StateChange.Operations[1].Operation.ID

	failure := Evaluate(EvaluationInput{Manifest: manifest, Decision: decision, Facts: facts, Candidate: candidate})
	if failure == nil || failure.Code != CodeOwnerTargetEffectOverlap || failure.Diagnostic == nil ||
		failure.Diagnostic.OperationID != secondOperationID || failure.Diagnostic.Actual != runtimecontract.RuntimeModuleFile+"|main.go" ||
		failure.Diagnostic.OperationSetViolation == nil ||
		!reflect.DeepEqual(failure.Diagnostic.Objectives, []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionConsolidateOperationSet}) {
		t.Fatalf("same owner/target durable overlap = %+v", failure)
	}
}

func TestEvaluateAppliesExecutableContractBeforeOwnerProbe(t *testing.T) {
	facts, decision, manifest, candidate := evaluatorFixture(t)
	patchType := "text_replace"
	unchanged := "same"
	operation := candidate.StateChange.Operations[0].Operation
	operation.Parameters = []runtimecontract.OperationParameter{
		{Name: "patchType", String: &patchType},
		{Name: "oldText", String: &unchanged},
		{Name: "newText", String: &unchanged},
	}
	candidate.StateChange.Operations[0].Operation = operation
	var err error
	candidate, err = runtimecontract.DeriveRuntimeCandidateOperationAuthority(manifest, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtimecontract.ValidateRuntimeCandidateAgainstManifest(manifest, candidate); err != nil {
		t.Fatalf("fixture must cross the provider-selectable manifest fence first: %v", err)
	}

	failure := Evaluate(EvaluationInput{Manifest: manifest, Decision: decision, Facts: facts, Candidate: candidate})
	if failure == nil || failure.Code != CodeInvalid || !strings.Contains(failure.Reason, "distinct") {
		t.Fatalf("executable contract defect escaped pre-probe admission: %+v", failure)
	}
}

func TestManifestOperationSetPolicyAllowsDistinctTargetsAndNonStateChangeDocuments(t *testing.T) {
	_, _, manifest, candidate := evaluatorFixture(t)
	first := candidate.StateChange.Operations[0].Operation
	second := first
	second.ID = "patch-2"
	second.TargetRefs = []string{"other.go"}
	operations := []runtimecontract.RuntimeOperation{first, second}
	if violation := runtimecontract.RuntimeCandidateOperationSetViolationForManifest(manifest, operations); violation != nil {
		t.Fatalf("distinct durable targets rejected: %+v", violation)
	}

	manifest.State = runtimecontract.ConvergenceStateNeedValidation
	manifest.OperationSetPolicy = runtimecontract.RuntimeCandidateOperationSetPolicy{}
	if violation := runtimecontract.RuntimeCandidateOperationSetViolationForManifest(manifest, []runtimecontract.RuntimeOperation{first, first}); violation != nil {
		t.Fatalf("non-state-change document applied durable operation-set policy: %+v", violation)
	}
}

func TestManifestOperationSetPolicyAllowsExactOwnerRequiredSequence(t *testing.T) {
	_, _, manifest, candidate := evaluatorFixture(t)
	first := candidate.StateChange.Operations[0].Operation
	first.RequiredOperationRef = "prepare"
	second := first
	second.ID = "patch-2"
	second.RequiredOperationRef = "finalize"
	second.DependsOn = []string{first.ID}
	candidate.StateChange.Operations = []runtimecontract.StateChangingOperation{{Operation: first}, {Operation: second}}
	manifest.RequiredOperationSequence = []runtimecontract.RuntimeRequiredOperationVariant{
		{RequirementID: "prepare", Kind: first.Kind, VariantID: "text_replace"},
		{RequirementID: "finalize", Kind: second.Kind, VariantID: "text_replace", DependsOnRequirementIDs: []string{"prepare"}},
	}
	operations := Operations(candidate)
	if violation := runtimecontract.RuntimeCandidateOperationSetViolationForManifest(manifest, operations); violation != nil {
		t.Fatalf("owner-required same-target sequence rejected: %+v", violation)
	}

	second.RequiredOperationRef = ""
	candidate.StateChange.Operations[1].Operation = second
	if violation := runtimecontract.RuntimeCandidateOperationSetViolationForManifest(manifest, Operations(candidate)); violation == nil {
		t.Fatal("optional same-target operation escaped manifest operation-set gate")
	}
}

func TestMissingValidationFailureUsesDetachedProofProjection(t *testing.T) {
	facts, decision, _, candidate := evaluatorFixture(t)
	failure := MissingValidationFailure(facts, decision, candidate, ValidationGapFacts{
		MissingEvidenceKinds:       []string{runtimecontract.EvidenceKindValidation},
		HasAcceptedStateChange:     true,
		LatestFailureStateChanging: true,
	})
	if failure == nil || failure.Code != CodeActionEvidenceMismatch {
		t.Fatalf("missing validation failure = %+v", failure)
	}
	facts.RepairDirective = &runtimecontract.RuntimeRepairDirective{FocusID: facts.Focus.ID, RequiredState: runtimecontract.ConvergenceStateNeedStateChange}
	if failure := MissingValidationFailure(facts, decision, candidate, ValidationGapFacts{
		MissingEvidenceKinds:       []string{runtimecontract.EvidenceKindValidation},
		HasAcceptedStateChange:     true,
		LatestFailureStateChanging: true,
	}); failure != nil {
		t.Fatalf("explicit state-change repair directive rejected: %+v", failure)
	}
}

func evaluatorFixture(t *testing.T) (runtimecontract.RuntimeFacts, runtimecontract.RuntimeDecision, runtimecontract.RuntimeCandidateContractManifest, runtimecontract.RuntimeCandidate) {
	t.Helper()
	binding := runtimecontract.CoverageEvidenceBinding{
		ID:                 "binding/test/cwi-1",
		CoverageWorkItemID: "cwi-1",
		FocusID:            "focus-1",
		DefinitionRevision: "definition-1",
		TargetRefs:         []string{"main.go"},
		DeclarationRef:     "declaration/test/cwi-1",
		Proofs: []runtimecontract.CoverageEvidenceProof{{
			EvidenceKind:          runtimecontract.EvidenceKindStateChange,
			RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
			AllowedOperationKinds: []string{runtimecontract.RuntimeBlockKindPatchApply},
		}},
	}
	binding = runtimecontract.NormalizeCoverageEvidenceBindings([]runtimecontract.CoverageEvidenceBinding{binding})[0]
	focus := runtimecontract.CoverageFocus{
		ID:                       "focus-1",
		CoverageWorkItemIDs:      []common.CoverageWorkItemID{"cwi-1"},
		RequiredEvidenceKinds:    []string{runtimecontract.EvidenceKindStateChange},
		TargetRefs:               []string{"main.go"},
		CoverageEvidenceBindings: []runtimecontract.CoverageEvidenceBinding{binding},
	}
	decision := runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange,
		Focus: focus,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds:         []string{runtimecontract.RuntimeBlockKindPatchApply},
			AcceptedEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
			RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
			CoverageWorkItemIDs:   []common.CoverageWorkItemID{"cwi-1"},
			TargetRefs:            []string{"main.go"},
		},
	}
	facts := runtimecontract.RuntimeFacts{
		WorkspaceID: "workspace-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: "evidence-1",
		Focus: focus,
		Coverage: []runtimecontract.RuntimeCoverageItemFact{{
			ID: "cwi-1", State: runtimecontract.RuntimeCoverageItemStateOpen,
		}},
		Activation: runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{
			Target: runtimecontract.ProviderModelTarget{ProviderID: "provider-1", ModelID: "model-1"},
		}, ExecutionContract: runtimecontract.RuntimeExecutionContract{StateChangePolicy: runtimecontract.RuntimeStateChangeAllowedButNotRequired}},
	}
	facts.ConvergencePolicy = runtimecontract.DefaultRuntimeConvergencePolicy()
	facts.RecoveryPolicy = runtimecontract.DefaultRuntimeRecoveryPolicy()
	facts.ExecutionBudgetPolicy = runtimecontract.DefaultRuntimeExecutionBudgetPolicy()
	var err error
	decision, err = runtimecontract.SealRuntimeDecision(facts, decision, 0)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(decision, runtimecontract.RuntimeCandidateManifestOwnerFacts{})
	if err != nil {
		t.Fatal(err)
	}
	patchType, content := "file_write", "updated\n"
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "patch-1", Kind: runtimecontract.RuntimeBlockKindPatchApply,
		BindingRefs: []string{binding.ID}, TargetRefs: []string{"main.go"},
		Parameters: []runtimecontract.OperationParameter{{Name: "patchType", String: &patchType}, {Name: "content", String: &content}},
	}}}}}
	candidate, err = runtimecontract.DeriveRuntimeCandidateOperationAuthority(manifest, candidate)
	if err != nil {
		t.Fatal(err)
	}
	return facts, decision, manifest, candidate
}
