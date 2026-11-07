package orchestrator

import (
	"os"
	"path/filepath"
	"testing"

	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const inputRevisionDeliveryDependency = "DELIVERY.txt"

// The first two revisions create and correct a delivery dependency. The reviewed
// input remains contradictory until a third real effect fixes it. No
// effect may stand in for fresh independent review, validation or assessment.
func TestDriveRuntimeInputRevisionDeliveryDependencyAndRecovery(t *testing.T) {
	options := validationInputReviewMainlineOptions{Adapter: shellcommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic, ValidationCount: 1, ProtocolRef: rc.RuntimeValidationInputReviewProtocol}
	baseline := runValidationInputReviewMainline(t, "input_revision_dependency", "", options)
	for _, cut := range []string{"after_input_revision_retirement", "after_input_revision_receipt"} {
		t.Run(cut, func(t *testing.T) {
			if got := runValidationInputReviewMainline(t, "input_revision_dependency", cut, options); got != baseline {
				t.Fatalf("dependency recovery repeated provider work: %d != %d", got, baseline)
			}
		})
	}
}

func configureRuntimeInputDependencyMainline(b *rc.RuntimeBootstrap) {
	p := b.DefinitionFocuses[1].ImplementationWorkPolicy
	p.AllowedOperations = append(p.AllowedOperations, rc.RuntimeOperationVariantRef{Kind: rc.StepKindFileWrite})
	p.AllowedTargetPolicies = append(p.AllowedTargetPolicies, rc.RuntimeOperationTargetPolicyProspectiveOwnerTarget)
	p.ProspectiveTargetAllOfConstraintRefs = []string{rc.RuntimeImplementationWorkConstraintWorkspaceRelativePath, rc.RuntimeImplementationWorkConstraintNewTargetOwnerSelector}
	b.CoverageItems[1].TargetText += "; the delivered workspace note must contain the documented value 4"
}

func runtimeInputDependencyContent(c llmevents.RequestRuntimeCandidateCommand) (string, bool) {
	for _, o := range c.SemanticManifest.ProofObligations {
		if w := o.ImplementationWork; w != nil && w.Scope.Content != nil {
			for _, s := range w.Scope.Content.Snapshots {
				if s.TargetRef == inputRevisionDeliveryDependency {
					return s.Content, true
				}
			}
			return "", false
		}
	}
	return "", false
}

func runtimeInputDependencySemanticDocument(d *rc.RuntimeSemanticPlanDocument, c llmevents.RequestRuntimeCandidateCommand) {
	content, exists := runtimeInputDependencyContent(c)
	if exists && content == "4\n" {
		return
	}
	for _, o := range c.SemanticManifest.ProofObligations {
		if w := o.ImplementationWork; w != nil && w.PurposeChoiceOnly && d.ImplementationPurpose == rc.RuntimeImplementationWorkPurposeEffect {
			d.ImplementationIntent.DependencyTargetRefs = []string{inputRevisionDeliveryDependency}
		}
		if w := o.ImplementationWork; w != nil && !w.PurposeChoiceOnly && d.PlanStep == rc.RuntimeSemanticPlanStepProofBatch {
			d.Proposals = []rc.RuntimeSemanticProofProposal{{CoverageWorkItemID: o.CoverageWorkItemID, OwnerModuleID: rc.RuntimeModuleFile, OperationKind: rc.StepKindFileWrite,
				DeclarationRef: rc.RuntimeSemanticCurrentDeclarationRef(o), RequiredEvidenceKinds: []string{rc.EvidenceKindStateChange}, RequiredEffectScopes: []rc.RuntimeEffectScope{rc.RuntimeEffectWorkspaceDurable},
				Target: rc.RuntimeOperationTargetAuthorization{Policy: rc.RuntimeOperationTargetPolicyProspectiveOwnerTarget, ProspectiveTargetRefs: []string{inputRevisionDeliveryDependency}, OwnerSelectorRefs: []string{"workspace-relative-new-file-target"}, ProspectiveConstraintRefs: w.Scope.Request.Policy.ProspectiveTargetAllOfConstraintRefs}}}
			if exists {
				d.Proposals[0].OperationKind = rc.StepKindFileEdit
				d.Proposals[0].Target = rc.RuntimeOperationTargetAuthorization{Policy: rc.RuntimeOperationTargetPolicyExistingOwnerScope, OwnerScopeTargetRefs: []string{inputRevisionDeliveryDependency}}
			}
		}
	}
}

func runtimeInputDependencyCandidate(c llmevents.RequestRuntimeCandidateCommand) (rc.RuntimeCandidate, bool) {
	for _, p := range c.CandidateManifest.ImplementationWorkingBinding.Proposals {
		if p.OperationKind == rc.StepKindFileWrite {
			body := "3\n"
			return rc.RuntimeCandidate{StateChange: &rc.StateChangeCandidate{Operations: []rc.StateChangingOperation{{Operation: rc.RuntimeOperation{ID: "create-delivery-dependency", Kind: rc.StepKindFileWrite, TargetRefs: []string{inputRevisionDeliveryDependency}, RequiredOperationRef: c.CandidateManifest.RequiredOperationSequence[0].RequirementID, Parameters: []rc.OperationParameter{{Name: "content", String: &body}}}}}}}, true
		}
		if refs := rc.RuntimeSemanticProposalTargetRefs(p); len(refs) == 1 && refs[0] == inputRevisionDeliveryDependency {
			before, after := "3", "4"
			return rc.RuntimeCandidate{StateChange: &rc.StateChangeCandidate{Operations: []rc.StateChangingOperation{{Operation: rc.RuntimeOperation{ID: "correct-delivery-dependency", Kind: rc.StepKindFileEdit, TargetRefs: refs, RequiredOperationRef: c.CandidateManifest.RequiredOperationSequence[0].RequirementID, Parameters: []rc.OperationParameter{{Name: "oldText", String: &before}, {Name: "newText", String: &after}}}}}}}, true
		}
	}
	return rc.RuntimeCandidate{}, false
}

func assertRuntimeInputDependencyMainline(t *testing.T, a runtime.RuntimeAggregate, directory string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(directory, inputRevisionDeliveryDependency))
	if err != nil || string(b) != "4\n" {
		t.Fatal("missing delivery was not created through its File owner", err)
	}
	if len(a.ImplementationWork.Rounds) != 3 {
		t.Fatal("dependency or input effect did not complete")
	}
	first := a.ImplementationWork.Rounds[0]
	if first.ScopeRequest.InputRevision == nil || len(first.WorkingBinding.Proposals) != 1 || first.WorkingBinding.Proposals[0].OperationKind != rc.StepKindFileWrite {
		t.Fatal("dependency creation lost the original conflict/policy source")
	}
	for _, r := range a.ImplementationWork.Rounds[:2] {
		if len(r.WorkingBinding.IntentBinding.Intent.DependencyTargetRefs) != 1 || r.WorkingBinding.IntentBinding.Intent.DependencyTargetRefs[0] != inputRevisionDeliveryDependency {
			t.Fatal("selected dependency lost before effect")
		}
		if len(r.ScopeRequest.InputRevision.Report.Checks[0].DependencyTargetRefs) != 1 {
			t.Fatal("original independent dependency finding lost")
		}
	}
	for _, attempt := range a.CandidateAttempts {
		for _, e := range attempt.Feasibilities {
			if e.OperationID == "create-delivery-dependency" || e.OperationID == "correct-delivery-dependency" {
				if e.InputStructure != nil {
					t.Fatal("delivery dependency was checked as validation-input code")
				}
			}
		}
	}
}

func TestDriveRuntimeDependencyScopeMisclassificationUsesBoundedCorrection(t *testing.T) {
	options := validationInputReviewMainlineOptions{Adapter: shellcommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic, ValidationCount: 1, ProtocolRef: rc.RuntimeValidationInputReviewProtocol, DependencyScopeErrorOnce: true}
	baseline := runValidationInputReviewMainline(t, "input_revision_dependency", "", options)
	if got := runValidationInputReviewMainline(t, "input_revision_dependency", "after_input_revision_retirement", options); got != baseline {
		t.Fatalf("recovery repeated rejected or corrected request: %d != %d", got, baseline)
	}
}
