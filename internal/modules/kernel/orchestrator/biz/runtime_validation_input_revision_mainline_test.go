package orchestrator

import (
	"reflect"
	"testing"

	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Actual File/Shell/Run/Result/Workspace owners, scripted provider only. No
// synthetic repair receipt, coverage fact or input mutation is injected.
func TestDriveRuntimeInputReviewRevisionRealEffectAndFreshValidation(t *testing.T) {
	baseline := runValidationInputReviewMainline(t, "input_revision", "")
	for _, cut := range []string{"after_input_revision_retirement", "after_input_revision_receipt"} {
		t.Run(cut, func(t *testing.T) {
			if calls := runValidationInputReviewMainline(t, "input_revision", cut); calls != baseline {
				t.Fatalf("restart repeated provider work: %d != %d", calls, baseline)
			}
		})
	}
}

func TestDriveRuntimeInputReviewRevisionOriginalCWIBudgetExhaustion(t *testing.T) {
	baseline := runValidationInputReviewMainline(t, "input_revision_exhausted", "")
	if got := runValidationInputReviewMainline(t, "input_revision_exhausted", "after_input_revision_receipt"); got != baseline {
		t.Fatalf("restart reset original CWI budget: %d != %d", got, baseline)
	}
}

func configureRuntimeInputRevisionMainline(b *runtimecontract.RuntimeBootstrap) {
	p := &runtimecontract.RuntimeImplementationWorkPolicy{ProtocolRef: runtimecontract.RuntimeImplementationWorkProtocolV1, InputRevisionProtocolRef: runtimecontract.RuntimeValidationInputRevisionProtocolV1, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		AllowedOperations: []runtimecontract.RuntimeOperationVariantRef{{Kind: runtimecontract.StepKindFileEdit}}, AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope}, TargetSelectorRef: runtimecontract.RuntimeImplementationWorkFileWorkspaceDeliverableSelectorRef, TargetSelectorOwnerRef: "profile/revision", TargetSelectorSubjectRef: "original-input", MaxRoundsPerCWI: 2, MaxTargetsPerRound: 1, RequireFreshObservation: true, RequireFreshValidation: true, ProtectValidationInputs: true}
	b.DefinitionFocuses[1].ImplementationWorkPolicy = p
	b.Activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements[1].ImplementationWorkPolicy = p
}

func runtimeInputRevisionMainlineSemanticDocument(d *runtimecontract.RuntimeSemanticPlanDocument, c llmevents.RequestRuntimeCandidateCommand, target string) {
	for _, o := range c.SemanticManifest.ProofObligations {
		if w := o.ImplementationWork; w != nil {
			if w.PurposeChoiceOnly {
				d.ImplementationPurpose = runtimecontract.RuntimeImplementationWorkPurposeTerminal
				d.ImplementationIntent = implementationWorkIntentForTest(c.SemanticManifest)
				if w.Scope.Request.InputRevision != nil {
					d.ImplementationPurpose = runtimecontract.RuntimeImplementationWorkPurposeEffect
				}
				return
			}
			if d.PlanStep == runtimecontract.RuntimeSemanticPlanStepCoverageOrder {
				return
			}
			d.Proposals = []runtimecontract.RuntimeSemanticProofProposal{{CoverageWorkItemID: o.CoverageWorkItemID, OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKind: runtimecontract.StepKindFileEdit, DeclarationRef: runtimecontract.RuntimeSemanticCurrentDeclarationRef(o), RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}, Target: runtimecontract.RuntimeOperationTargetAuthorization{Policy: runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope, OwnerScopeTargetRefs: []string{target}}}}
		}
	}
}

func runtimeInputRevisionMainlineCandidate(c llmevents.RequestRuntimeCandidateCommand, target string) runtimecontract.RuntimeCandidate {
	before, after := `!="5"`, `!="4"`
	return runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{ID: "revise-input", Kind: runtimecontract.StepKindFileEdit, TargetRefs: []string{target}, RequiredOperationRef: c.CandidateManifest.RequiredOperationSequence[0].RequirementID, Parameters: []runtimecontract.OperationParameter{{Name: "oldText", String: &before}, {Name: "newText", String: &after}}}}}}}
}

func assertRuntimeInputRevisionMainline(t *testing.T, a runtime.RuntimeAggregate, reviews int) {
	t.Helper()
	if reviews != 2 || len(a.ImplementationWork.Rounds) != 1 || len(a.GovernanceRequests) != 0 {
		t.Fatalf("revision did not use one bounded real effect and fresh review: reviews=%d rounds=%d", reviews, len(a.ImplementationWork.Rounds))
	}
	r := a.ImplementationWork.Rounds[0]
	if r.CompletedReceiptSetHash == "" || r.ScopeRequest.InputRevision == nil {
		t.Fatal("missing exact real input revision")
	}
	if err := runtimecontract.ValidateRuntimeInputRevisionSource(r.ScopeRequest.InputRevision, a.SemanticPlanning); err != nil {
		t.Fatal(err)
	}
	for _, receipt := range a.OperationReceipts {
		if receipt.OperationID == "revise-input" && (receipt.Evidence.Validation != nil || !receipt.Changed || receipt.BeforeHash == receipt.AfterHash) {
			t.Fatal("revision borrowed terminal credit or did no real work")
		}
	}
	for _, mutate := range []func(*runtime.RuntimeAggregate){
		func(b *runtime.RuntimeAggregate) {
			var sessions []runtimecontract.RuntimeCandidateRequestSession
			for _, session := range b.CandidateRequestSessions {
				if session.LogicalRequestID != r.ScopeRequest.InputRevision.SourceLogicalRequestID {
					sessions = append(sessions, session)
				}
			}
			b.CandidateRequestSessions = sessions
		},
		func(b *runtime.RuntimeAggregate) {
			for i := range b.SemanticPlanning.History {
				if b.SemanticPlanning.History[i].Reason == runtimecontract.RuntimeSemanticPlanningRetirementInputRevision {
					b.SemanticPlanning.History[i].Reason = runtimecontract.RuntimeSemanticPlanningRetirementInputReviewInformation
				}
			}
		},
		func(b *runtime.RuntimeAggregate) {
			b.ImplementationWork.Rounds[0].ScopeRequest.InputRevision.Report.Checks[0].Reason = "forged"
		},
	} {
		bad := runtimeAggregateFromRecord(runtimeStateRecordFromAggregate(a))
		mutate(&bad)
		if bad.Validate() == nil {
			t.Fatal("fresh load trusted forged revision")
		}
		if record := runtimeStateRecordFromAggregate(bad); record.Validate() == nil {
			t.Fatal("persisted DTO trusted forged revision")
		}
	}
	if !reflect.DeepEqual(r.ScopeRequest.InputRevision.Report, r.WorkingBinding.InputRevision.Report) {
		t.Fatal("work lost rejected original report")
	}
}
