package orchestrator

import (
	"fmt"
	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	"reflect"
	"strings"
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

func TestDriveRuntimeInputReviewRevisionRepeatedStateExhaustion(t *testing.T) {
	baseline := runValidationInputReviewMainline(t, "input_revision_cycle", "")
	if got := runValidationInputReviewMainline(t, "input_revision_cycle", "after_input_revision_receipt"); got != baseline {
		t.Fatalf("restart reset physical state guard: %d != %d", got, baseline)
	}
}

func configureRuntimeInputRevisionMainline(b *runtimecontract.RuntimeBootstrap) {
	p := &runtimecontract.RuntimeImplementationWorkPolicy{ProtocolRef: runtimecontract.RuntimeImplementationWorkProtocolV1, InputRevisionProtocolRef: runtimecontract.RuntimeValidationInputRevisionProtocolV1, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		AllowedOperations: []runtimecontract.RuntimeOperationVariantRef{{Kind: runtimecontract.StepKindFileEdit}}, AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope}, TargetSelectorRef: runtimecontract.RuntimeImplementationWorkFileWorkspaceDeliverableSelectorRef, TargetSelectorOwnerRef: "profile/revision", TargetSelectorSubjectRef: "original-input", MaxRepeatedWorkStates: 2, MaxTargetsPerRound: 1, RequireFreshObservation: true, RequireFreshValidation: true, ProtectValidationInputs: true}
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
					if !w.EffectAvailable {
						d.ImplementationPurpose = runtimecontract.RuntimeImplementationWorkPurposeStalled
						d.ImplementationIntent.SelectedRequirementRefs = nil
					}
				} else if d.ImplementationIntent != nil {
					d.ImplementationIntent.SelectedRequirementRefs = nil
					for i := range d.ImplementationIntent.RequirementReviews {
						d.ImplementationIntent.RequirementReviews[i].WorkRequired = false
					}
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

func runtimeInputRevisionMainlineCandidate(c llmevents.RequestRuntimeCandidateCommand, target string, modes ...string) runtimecontract.RuntimeCandidate {
	before, after := `!="5"`, `!="4"`
	var reviewedInput string
	if v := c.CandidateManifest.ImplementationWorkingBinding.InputRevision; v != nil {
		for _, f := range v.Binding.InputReview.Files {
			if f.TargetRef == target {
				reviewedInput = f.Content
			}
		}
	}
	if len(modes) != 0 && (modes[0] == "input_revision_long" || modes[0] == "input_revision_budget") {
		for n := 1; n <= 12; n++ {
			if strings.Contains(reviewedInput, runtimeLongInputRevisionMarker(n)) {
				before, after = runtimeLongInputRevisionMarker(n), runtimeLongInputRevisionAssertion(n)
				break
			}
		}
	}
	if len(modes) != 0 && modes[0] == "input_revision_cycle" {
		if !strings.Contains(reviewedInput, before) {
			before, after = after, before
		}
	}
	if len(modes) != 0 && modes[0] == "input_revision_omission" {
		before, after = `strings.TrimSpace(string(out))==""`, `strings.TrimSpace(string(out))!="4"`
	}
	return runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{ID: "revise-input", Kind: runtimecontract.StepKindFileEdit, TargetRefs: []string{target}, RequiredOperationRef: c.CandidateManifest.RequiredOperationSequence[0].RequirementID, Parameters: []runtimecontract.OperationParameter{{Name: "oldText", String: &before}, {Name: "newText", String: &after}}}}}}}
}

func assertRuntimeInputRevisionMainline(t *testing.T, a runtime.RuntimeAggregate, reviews int, mode string) {
	t.Helper()
	if mode == "input_revision_budget" {
		if a.Decision.Terminal == nil || a.Decision.Terminal.Code != "execution_budget_exhausted" {
			t.Fatalf("lost resource budget terminal: %+v", a.Decision.Terminal)
		}
		effects := 0
		for _, r := range a.ImplementationWork.Rounds {
			if r.CompletedReceiptSetHash != "" {
				effects++
			}
		}
		if effects != 2 || len(a.OperationReceipts) != 3 || reviews != 3 {
			t.Fatalf("budget reset or granted credit: effects=%d receipts=%d reviews=%d", effects, len(a.OperationReceipts), reviews)
		}
		if len(a.ImplementationWork.Rounds) != 3 || a.ImplementationWork.Rounds[2].CompletedReceiptSetHash != "" || a.ImplementationWork.Rounds[2].FailedReceiptHash != "" {
			t.Fatal("budget stop fabricated round completion")
		}
		if a.ActiveDispatch != nil {
			t.Fatal("budget stop retained executable dispatch")
		}
		for _, mutate := range []func(*runtime.RuntimeAggregate){
			func(b *runtime.RuntimeAggregate) { b.Facts.Failure = nil },
			func(b *runtime.RuntimeAggregate) {
				b.Facts.Failure.Class = runtimecontract.FailureClassRuntimeContractMisalignment
			},
			func(b *runtime.RuntimeAggregate) {
				for i := range b.ExecutionBudget.Policy.Dimensions {
					if b.ExecutionBudget.Policy.Dimensions[i].Dimension == runtimecontract.RuntimeBudgetDimensionCapabilityOperation {
						b.ExecutionBudget.Policy.Dimensions[i].Limit++
					}
				}
				b.ExecutionBudget.Policy = b.ExecutionBudget.Policy.Normalized()
			},
			func(b *runtime.RuntimeAggregate) {
				for i := range b.CompletedDispatches {
					if b.CompletedDispatches[i].ID == b.ImplementationWork.Rounds[2].DispatchID {
						b.CompletedDispatches[i].OperationExecutions = nil
					}
				}
			},
			func(b *runtime.RuntimeAggregate) {
				for i := range b.CompletedDispatches {
					if b.CompletedDispatches[i].ID == b.ImplementationWork.Rounds[2].DispatchID {
						e := &b.CompletedDispatches[i].OperationExecutions[0]
						e.State = runtimecontract.OperationExecutionClaimed
						e.Fence = 1
						e.ClaimedAt = runtimecontract.RuntimeOperationClaimMarkerInitial
						if err := e.Validate(); err != nil {
							t.Fatalf("claim negative fixture is malformed: %v", err)
						}
					}
				}
			},
		} {
			bad := a.CloneRaw()
			mutate(&bad)
			record := runtimeStateRecordFromAggregate(bad)
			if err := runtimecontract.ValidateRuntimeImplementationWorkState(record.Definition, record.Facts, record.ImplementationWork, record.SemanticPlanning, record.ActiveDispatch, record.CompletedDispatches, record.OperationReceipts, record.OperationGovernanceBindings, record.ExecutionBudget); err == nil {
				t.Fatal("work-state guard accepted missing failure or a valid physical claim")
			}
			if bad.Validate() == nil {
				t.Fatal("unclaimed budget stop accepted missing failure or physical claim")
			}
		}
		for _, c := range a.Facts.Coverage {
			if c.ID == "cwi-2" && c.State == runtimecontract.RuntimeCoverageItemStateSatisfied {
				t.Fatal("input revision gained acceptance credit")
			}
		}
		return
	}
	wantRounds := 1
	if mode == "input_revision_long" {
		wantRounds = 12
		if a.SemanticPlanning.HistoryArchive == nil || len(a.SemanticPlanning.HistoryArchive.Segments) == 0 {
			t.Fatal("long revision did not exercise complete history rollover")
		}
	}
	if mode == "input_revision_cycle" {
		wantRounds = 4
		if a.Decision.Terminal == nil || a.Decision.Terminal.Code != "implementation_work_repeated_state_exhausted" {
			t.Fatalf("cycle lost exact terminal: %+v", a.Decision.Terminal)
		}
	}
	if reviews != wantRounds+1 || len(a.ImplementationWork.Rounds) != wantRounds || len(a.GovernanceRequests) != 0 {
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
	for mutationIndex, mutate := range []func(*runtime.RuntimeAggregate){
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
			if archive := b.SemanticPlanning.HistoryArchive; archive != nil {
				for i := range archive.Segments {
					for j := range archive.Segments[i] {
						if archive.Segments[i][j].Reason == runtimecontract.RuntimeSemanticPlanningRetirementInputRevision {
							archive.Segments[i][j].Reason = runtimecontract.RuntimeSemanticPlanningRetirementInputReviewInformation
						}
					}
				}
			}
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
			t.Fatal("fresh load trusted forged revision", mutationIndex)
		}
		if record := runtimeStateRecordFromAggregate(bad); record.Validate() == nil {
			t.Fatal("persisted DTO trusted forged revision")
		}
	}
	if !reflect.DeepEqual(r.ScopeRequest.InputRevision.Report, r.WorkingBinding.InputRevision.Report) {
		t.Fatal("work lost rejected original report")
	}
}

// Missing behavior assertions, original source conjuncts and future delivery
// consumers use the same bounded revision lane as conflicts. The File effect
// must precede a fresh subject/review and ordinary Shell validation.
func TestDriveRuntimeCompositeCoverageOmissionRevisionAndRecovery(t *testing.T) {
	options := validationInputReviewMainlineOptions{Adapter: shellcommon.ValidationRecipeAdapterProjectConjunction, ValidationCount: 1, SourceObligations: true, DeliveryGuard: true, ReverseProofConsumerRefs: true}
	baseline := runValidationInputReviewMainline(t, "input_revision_omission", "", options)
	for _, cut := range []string{"after_review_document", "after_input_revision_retirement", "after_input_revision_receipt", "after_review_binding", "after_project_receipt"} {
		t.Run(cut, func(t *testing.T) {
			if got := runValidationInputReviewMainline(t, "input_revision_omission", cut, options); got != baseline {
				t.Fatalf("recovery repeated provider calls: %d != %d", got, baseline)
			}
		})
	}
}

// Twelve distinct additions cross both retired numeric boundaries without
// treating changed bytes as semantic or validation credit. The independent
// review keeps the same broad missing criterion until every assertion exists.
func runtimeLongInputRevisionMarker(n int) string { return fmt.Sprintf("// missing assertion %d\n", n) }
func runtimeLongInputRevisionAssertion(n int) string {
	return fmt.Sprintf("if strings.Repeat(strings.TrimSpace(string(out)), %d) != %q { t.Fatal(\"missing repeated output behavior %d\") }\n", n, strings.Repeat("4", n), n)
}
func runtimeLongInputRevisionMarkers() string {
	var result strings.Builder
	for n := 1; n <= 12; n++ {
		result.WriteString(runtimeLongInputRevisionMarker(n))
	}
	return result.String()
}
func TestDriveRuntimeTwelveInputRevisionsReachFreshValidationAcrossRecovery(t *testing.T) {
	// Recover after the eighth real receipt, then reach twelve, a fresh
	// independent review, ordinary validation, assessment and all owner ACKs.
	// The provider fixture independently rejects duplicate logical calls.
	runValidationInputReviewMainline(t, "input_revision_long", "after_eighth_input_revision_receipt")
}

func TestDriveRuntimeInputRevisionsConsumeOriginalCapabilityBudget(t *testing.T) {
	baseline := runValidationInputReviewMainline(t, "input_revision_budget", "")
	if got := runValidationInputReviewMainline(t, "input_revision_budget", "after_input_revision_receipt"); got != baseline {
		t.Fatalf("restart reset actual budget: %d != %d", got, baseline)
	}
}
