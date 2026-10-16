package orchestrator

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Private admitted startup, then real File/Shell/Run/Result owners and production
// SM-2/3/4 drive. No work, repair receipt, validation or Goal credit is injected.
func TestDriveRuntimeGoalExecutionSourceRealRepairAcrossRestart(t *testing.T) {
	var baseline int64
	for _, cut := range []string{"uninterrupted", "before_behavior_failure", "after_behavior_failure", "after_fresh_validation"} {
		t.Run(cut, func(t *testing.T) {
			calls := runValidationInputAdditionMainline(t, cut, validationInputMainlineOptions{Entrypoint: true, BehaviorRepair: true, HistoricalPurposeMapping: true, GoalExecutionSource: true})
			if cut == "uninterrupted" {
				baseline = calls
			} else if calls != baseline {
				t.Fatalf("Goal source recovery repeated provider calls: %d != %d", calls, baseline)
			}
		})
	}
}

// Independently inspect the exact Shell-owned subject, not fixture paths or a
// previous review. This v2 report proves only ordinary input coverage; it does
// not claim that input execution or the review proves Runtime repair history.
func runtimeGoalReviewedRepairSemanticResult(command llmevents.RequestRuntimeCandidateCommand, sourceTargets ...string) (llmevents.RequestRuntimeCandidateResult, error) {
	return runtimeGoalReviewedRepairSemanticResultForScope(command, nil, sourceTargets...)
}

func runtimeGoalReviewedRepairSemanticResultForScope(command llmevents.RequestRuntimeCandidateCommand, validationScope []string, sourceTargets ...string) (llmevents.RequestRuntimeCandidateResult, error) {
	document := validationInputAdditionMainlineSemanticDocument(command, sourceTargets...)
	for n := range document.Proposals {
		proposal := &document.Proposals[n]
		if len(validationScope) != 0 && proposal.OperationKind == runtimecontract.RuntimeBlockKindManagedValidationSession {
			proposal.Target.ExactTargetRefs = append([]string(nil), validationScope...)
		}
		binding, found := runtimecontract.RuntimeSemanticValidationInputReviewBinding(*proposal, command.SemanticManifest.OwnerCatalogs)
		if !found {
			continue
		}
		if binding.InputReview == nil || binding.InputReview.ProtocolRef != runtimecontract.RuntimeValidationInputReviewProtocolV2 {
			return llmevents.RequestRuntimeCandidateResult{}, fmt.Errorf("Goal repair requires exact v2 input review")
		}
		var inputs []string
		for _, file := range binding.InputReview.Files {
			if strings.Contains(file.Content, `exec.Command("go","build"`) && strings.Contains(file.Content, `"./cmd/delivery"`) && strings.Contains(file.Content, `build.Dir="../../.."`) && strings.Contains(file.Content, `exec.Command(bin).Output()`) && strings.Contains(file.Content, `const expected = "4"`) && strings.Contains(file.Content, `WORKORCH_ASSERTION_V1 code=value_mismatch`) {
				inputs = append(inputs, file.TargetRef)
			}
		}
		if len(inputs) == 0 {
			return llmevents.RequestRuntimeCandidateResult{}, fmt.Errorf("owner subject does not prove the unmodified expected delivery behavior")
		}
		review := &runtimecontract.RuntimeValidationInputReview{SubjectHash: binding.InputReview.Hash}
		for _, check := range binding.RequiredCheckRefs {
			criterion := ""
			for _, item := range command.SemanticManifest.CoverageItems {
				if item.ID != proposal.CoverageWorkItemID {
					continue
				}
				criterion = item.TargetText
				for _, source := range item.SourceObligationChecks {
					if source.CheckRef == check {
						criterion = source.Criterion
					}
				}
			}
			if criterion == "" {
				return llmevents.RequestRuntimeCandidateResult{}, fmt.Errorf("review lost original criterion %q", check)
			}
			reason := "The original input builds and invokes the delivered CLI and compares its output with 4; the owner executes the declared v5 conjunctive recipe. Repair history remains the declared Runtime consumer's responsibility; this isolated proof does not claim project-wide test/race coverage."
			review.Checks = append(review.Checks, runtimecontract.RuntimeValidationInputCheckReview{RequiredCheckRef: check, Conclusion: runtimecontract.RuntimeValidationInputConsistent, Reason: reason, InputTargetRefs: inputs, Obligations: []runtimecontract.RuntimeValidationInputObligationReview{{CriterionText: criterion, Finding: runtimecontract.RuntimeValidationInputCovered, Reason: reason, InputTargetRefs: inputs}}})
		}
		if err := review.ValidateAgainstBinding(binding); err != nil {
			return llmevents.RequestRuntimeCandidateResult{}, err
		}
		proposal.ValidationInputReview = review
	}
	payload, err := runtimecontract.EncodeRuntimeSemanticPlanPayloadWithManifest(command.SemanticManifest, document)
	if err != nil {
		return llmevents.RequestRuntimeCandidateResult{}, err
	}
	return runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity, ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument), DocumentOutcome: runtimeCompleteDocumentOutcome(command.ExpectedDocument, payload)}), nil
}

func TestDriveRuntimeConfirmedRepairFiniteValidationScopeAcrossRestart(t *testing.T) {
	var baseline int64
	for _, cut := range []string{"uninterrupted", "before_behavior_failure", "after_behavior_failure", "after_fresh_validation"} {
		t.Run(cut, func(t *testing.T) {
			calls := runValidationInputAdditionMainline(t, cut, validationInputMainlineOptions{Entrypoint: true, BehaviorRepair: true, HistoricalPurposeMapping: true, GoalExecutionSource: true, WideValidationScope: true})
			if cut == "uninterrupted" {
				baseline = calls
			} else if calls != baseline {
				t.Fatalf("finite validation scope recovery repeated provider calls: %d != %d", calls, baseline)
			}
		})
	}
}

func runtimeTestWithGoalRepairSource(t *testing.T, bootstrap runtimecontract.RuntimeBootstrap) runtimecontract.RuntimeBootstrap {
	t.Helper()
	i := bootstrap.Definition.SourceObligations.Clone()
	rule := runtimecontract.RuntimeLineageAssertionRule{Ref: "goal-original-repair", Kind: runtimecontract.RuntimeLineageRepairThenFreshValidation, ConsumptionPoint: runtimecontract.RuntimeProofPhaseExecution, ExecutionSourceContractRef: runtimecontract.RuntimeGoalSourceExecutionProofV2, RepairHistoryContractRef: runtimecontract.RuntimeConfirmedBindingRepairProofV1}
	i.ExecutionProofMappings = []runtimecontract.RuntimeExecutionProofMapping{{ContractRef: rule.ExecutionSourceContractRef, ConsumerItemRef: "original-goal", RuleRef: rule.Ref, SourceItemRef: "cwi-2", ConsumerGoalText: bootstrap.Definition.GoalDefinition}}
	i = i.WithHash()
	bootstrap.Definition.SourceObligations = &i
	bootstrap.Activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements = append(bootstrap.Activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements, runtimecontract.RuntimeProfileEvidenceRequirement{
		ID: "goal", Scope: "goal", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, LineageAssertions: []runtimecontract.RuntimeLineageAssertionRule{rule},
	})
	var err error
	bootstrap.CoverageItems, err = runtimecontract.CompileRuntimeEvidencePurposeMappingCoverage(i, bootstrap.CoverageItems)
	if err != nil {
		t.Fatal(err)
	}
	for n := range bootstrap.CoverageItems {
		item := &bootstrap.CoverageItems[n]
		var err error
		item.SourceObligationChecks, err = runtimecontract.CompileRuntimeSourceObligationChecks(i, *item)
		if err != nil {
			t.Fatal(err)
		}
		item.GoalExecutionAssertions, err = runtimecontract.CompileRuntimeGoalExecutionAssertions(&i, *item, bootstrap.Activation.ExecutionContract.ProfileContracts)
		if err != nil {
			t.Fatal(err)
		}
	}
	bootstrap = runtimeTestBootstrapWithBindings(bootstrap)
	if err := bootstrap.Validate(); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(bootstrap)
	if err != nil {
		t.Fatal(err)
	}
	var reload runtimecontract.RuntimeBootstrap
	if json.Unmarshal(payload, &reload) != nil || reload.Validate() != nil || !reflect.DeepEqual(bootstrap.DefinitionSeed, reload.DefinitionSeed) {
		t.Fatal("sealed Goal source did not survive bootstrap reload")
	}
	return bootstrap
}

func runtimeGoalAssertOriginalRepairConsumer(t *testing.T, record runtimecontract.RuntimeStateRecord) {
	t.Helper()
	sources := runtimecontract.RuntimeLineageAssertionSources{
		Definition: record.Definition, Facts: record.Facts, Work: record.ImplementationWork,
		Semantic: record.SemanticPlanning, Active: record.ActiveDispatch, Completed: record.CompletedDispatches,
		Receipts: record.OperationReceipts, Governance: record.OperationGovernanceBindings,
		Budget: record.ExecutionBudget, EvidenceSequence: record.EvidenceSequence,
		Resolutions: record.GovernanceResolutions, Attempts: record.CandidateAttempts,
		Sessions: record.CandidateRequestSessions, ProcessedEventIDs: record.ProcessedEventIDs,
		ProofLineage: record.CoverageProofLineage, Confirmations: record.RepairSurfaceConfirmations,
		Reconciliation: record.Reconciliation,
	}
	var binding runtimecontract.CoverageEvidenceBinding
	for _, dispatch := range record.CompletedDispatches {
		if dispatch.Candidate.Validation == nil || dispatch.RepairCommitment != nil {
			continue
		}
		for _, candidate := range dispatch.CoverageBindings {
			if candidate.CoverageWorkItemID == "cwi-2" && len(candidate.LineageAssertions) != 0 {
				binding = candidate
			}
		}
	}
	if binding.ID == "" || len(binding.LineageAssertions) != 1 || binding.LineageAssertions[0].SourceScope != "goal" || binding.LineageAssertions[0].Rule.ExecutionSourceContractRef != runtimecontract.RuntimeGoalSourceExecutionProofV2 {
		t.Fatal("ordinary real validation lost exact original Goal consumer")
	}
	if _, err := runtimecontract.CompileRuntimeBindingValidationInputReview(binding, record.SemanticPlanning); err != nil {
		t.Fatalf("fresh ordinary binding lost independent exact review: %v", err)
	}
	if !runtimecontract.RuntimeExecutionLineageAssertionsSatisfied(binding, sources, sources.Facts.EvidenceRevision) {
		t.Fatal("actual owner repair did not satisfy original Goal consumer")
	}
	payload, err := json.Marshal(sources)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"missing original mapping", "missing receipt", "duplicate receipt", "stale receipt fence", "missing original review", "missing confirmation", "duplicate confirmation", "missing commitment", "coordinated source hash", "premature cutoff", "effect acquired credit"} {
		t.Run(scenario, func(t *testing.T) {
			var changed runtimecontract.RuntimeLineageAssertionSources
			if err := json.Unmarshal(payload, &changed); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "missing original mapping":
				changed.Definition.SourceObligations.ExecutionProofMappings = nil
			case "missing receipt":
				changed.Receipts = changed.Receipts[1:]
			case "duplicate receipt":
				changed.Receipts = append(changed.Receipts, changed.Receipts[0])
			case "stale receipt fence":
				changed.Receipts[0].Fence++
			case "missing original review":
				runtimeGoalRemoveOriginalInputReviews(&changed.Semantic)
			case "missing confirmation":
				changed.Confirmations = nil
			case "duplicate confirmation":
				changed.Confirmations = append(changed.Confirmations, changed.Confirmations[0])
			case "missing commitment", "coordinated source hash":
				for n := range changed.Completed {
					dispatch := &changed.Completed[n]
					if dispatch.RepairCommitment == nil {
						continue
					}
					if scenario == "missing commitment" {
						dispatch.RepairCommitment, dispatch.RepairCommitmentHash = nil, ""
					} else {
						dispatch.RepairCommitment.SourceEvidenceFactHash = strings.Repeat("f", 64)
						dispatch.RepairCommitmentHash = runtimecontract.StableRuntimeDispatchRepairCommitmentHash(*dispatch.RepairCommitment)
					}
				}
			case "premature cutoff":
				if runtimecontract.RuntimeExecutionLineageAssertionsSatisfied(binding, changed, "runtime-evidence-00000000000000000002") {
					t.Fatal("future repair/validation counted at original failure cutoff")
				}
				return
			case "effect acquired credit":
				for n := range changed.Facts.Evidence.Accepted {
					if changed.Facts.Evidence.Accepted[n].Kind == runtimecontract.EvidenceKindStateChange && len(changed.Facts.Evidence.Accepted[n].CoverageWorkItemIDs) == 0 {
						changed.Facts.Evidence.Accepted[n].CoverageWorkItemIDs = []common.CoverageWorkItemID{binding.CoverageWorkItemID}
					}
				}
			}
			if runtimecontract.RuntimeExecutionLineageAssertionsSatisfied(binding, changed, changed.Facts.EvidenceRevision) {
				t.Fatal("incomplete or stale original owner proof acquired Goal credit")
			}
		})
	}
}

func runtimeGoalRemoveOriginalInputReviews(ledger *runtimecontract.RuntimeSemanticPlanningLedger) {
	for n := range ledger.BatchDocuments {
		for p := range ledger.BatchDocuments[n].Proposals {
			ledger.BatchDocuments[n].Proposals[p].ValidationInputReview = nil
		}
	}
	for n := range ledger.History {
		runtimeGoalRemoveOriginalInputReviews(&ledger.History[n].Snapshot)
	}
}
