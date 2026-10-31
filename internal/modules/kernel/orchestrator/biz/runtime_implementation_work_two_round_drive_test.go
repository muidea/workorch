package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 使用 private startup fixture，但之后只由 production SM-4 driver、完整 provider 文档和真实 File owner 推进。
// 不手工写 round/binding/candidate/receipt，不宣称公开 activation 或独立 fresh-process。
func TestImplementationWorkProductionDriverSameCWIRealFileTwoRounds(t *testing.T) {
	for _, empty := range []bool{false, true} {
		name := "existing_edit_then_edit"
		if empty {
			name = "empty_create_then_edit"
		}
		t.Run(name, func(t *testing.T) {
			testImplementationWorkProductionDriverSameCWIRealFileTwoRounds(t, empty)
		})
	}
}

func TestImplementationWorkProductionDriverFourTargetsTwoRounds(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(fmt.Sprintf("empty_%t", empty), func(t *testing.T) {
			t.Parallel()
			testImplementationWorkProductionDriverTargets(t, empty, []string{"resource-a", "resource-b", "resource-c", "resource-d"})
		})
	}
}

func TestImplementationWorkProductionDriverCardinalityCorrectionRealFile(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(fmt.Sprintf("empty_%t", empty), func(t *testing.T) {
			testImplementationWorkProductionDriverTargetsWithCardinality(t, empty, []string{"resource-a", "resource-b", "resource-c", "resource-d"}, true)
		})
	}
}

func testImplementationWorkProductionDriverSameCWIRealFileTwoRounds(t *testing.T, empty bool) {
	testImplementationWorkProductionDriverTargets(t, empty, []string{"resource-a"})
}

func configureImplementationWorkDriverTargets(t *testing.T, aggregate *runtime.RuntimeAggregate, hub magicevent.Hub, empty bool, targets []string) {
	t.Helper()
	ctx := context.Background()
	if len(targets) > 1 {
		policy := aggregate.Facts.DefinitionFocuses[0].ImplementationWorkPolicy.Normalized()
		policy.MaxTargetsPerRound = len(targets)
		policy.TargetSelectorCandidateRefs = targets
		aggregate.Facts.DefinitionFocuses[0].ImplementationWorkPolicy = &policy
		aggregate.Facts.Activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements[0].ImplementationWorkPolicy = &policy
		if !empty {
			workspace, err := (workspaceSnapshotReader{hub: hub}).Read(ctx, aggregate.Facts.WorkspaceID)
			if err != nil {
				t.Fatal(err)
			}
			for _, target := range targets[1:] {
				if err := os.WriteFile(filepath.Join(workspace.DeliverableDirectory, target), []byte("original implementation"), 0600); err != nil {
					t.Fatal(err)
				}
			}
		}
		if err := aggregate.Validate(); err != nil {
			t.Fatal(err)
		}
	}
}

func testImplementationWorkProductionDriverTargets(t *testing.T, empty bool, targets []string) {
	testImplementationWorkProductionDriverTargetsWithCardinality(t, empty, targets, false)
}

func testImplementationWorkProductionDriverTargetsWithCardinality(t *testing.T, empty bool, targets []string, injectCardinality bool) {
	t.Helper()
	ctx := context.Background()
	module, repo, aggregate := implementationWorkPurposeDebtDriverFixtureWithEmptyDirectory(t, empty)
	configureImplementationWorkDriverTargets(t, &aggregate, module.EventHub(), empty, targets)
	repo.aggregate = aggregate
	hub := module.EventHub()
	receipts := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	committedReceipts := 0
	receipts.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, module.handleRuntimeOperationReceipt)
	receipts.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		beforeRevision, beforeCount := repo.aggregate.StateRevision, len(repo.aggregate.OperationReceipts)
		module.handleRuntimeOperationReceipt(event, result)
		if len(repo.aggregate.OperationReceipts) == beforeCount+1 {
			committedReceipts++
			if repo.aggregate.StateRevision != beforeRevision+1 {
				t.Errorf("real owner receipt/effect/round/invalidation must use one aggregate CAS: before=%d after=%d", beforeRevision, repo.aggregate.StateRevision)
			}
		}
	})
	t.Cleanup(func() {
		receipts.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
		receipts.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
	})
	provider := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	semanticCalls, operationCalls := 0, 0
	provider.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		if command.DocumentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan {
			semanticCalls++
			manifest := command.SemanticManifest
			document := runtimecontract.RuntimeSemanticPlanDocument{SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: command.SemanticContext.PlanStep}
			if document.PlanStep == runtimecontract.RuntimeSemanticPlanStepCoverageOrder {
				document.CoverageOrder = []common.CoverageWorkItemID{manifest.CoverageItems[0].ID}
				if work := manifest.ProofObligations[0].ImplementationWork; work != nil && work.PurposeChoiceOnly {
					document.ImplementationPurpose = runtimecontract.RuntimeImplementationWorkPurposeEffect
				}
			} else {
				document.BatchID, document.BatchSequence = command.SemanticContext.BatchID, command.SemanticContext.BatchSequence
				document.BatchCoverageWorkItemIDs = command.SemanticContext.BatchCoverageWorkItemIDs
				target := runtimecontract.RuntimeOperationTargetAuthorization{Policy: runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope, OwnerScopeTargetRefs: targets}
				kind := runtimecontract.StepKindFileEdit
				if work := manifest.ProofObligations[0].ImplementationWork; empty && work != nil && work.Ordinal == 1 {
					kind = runtimecontract.StepKindFileWrite
					target = runtimecontract.RuntimeOperationTargetAuthorization{Policy: runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
						OwnerSelectorRefs: []string{"workspace-relative-new-file-target"}, ProspectiveTargetRefs: targets,
						ProspectiveConstraintRefs: work.Scope.Request.Policy.ProspectiveTargetAllOfConstraintRefs}
				}
				document.Proposals = []runtimecontract.RuntimeSemanticProofProposal{{CoverageWorkItemID: manifest.CoverageItems[0].ID, OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKind: kind,
					Target: target, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
					RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}, DeclarationRef: runtimecontract.RuntimeSemanticCurrentDeclarationRef(manifest.ProofObligations[0])}}
			}
			payload, err := runtimecontract.EncodeRuntimeSemanticPlanPayloadWithManifest(manifest, document)
			if err != nil {
				t.Error(err)
				result.Set(nil, nil)
				return
			}
			result.Set(runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{DocumentOutcome: runtimeCompleteDocumentOutcome(command.ExpectedDocument, payload)}), nil)
			return
		}
		operationCalls++
		manifest := command.CandidateManifest
		ordinal := operationCalls
		if injectCardinality && operationCalls > 1 {
			ordinal--
		}
		if injectCardinality && operationCalls == 2 {
			if command.CandidateAttempt != 1 || command.CandidateCorrection == nil || command.CandidateCorrection.Witness.Dimension != runtimecontract.RuntimeCandidateCorrectionWitnessParameterCardinality || len(repo.aggregate.OperationReceipts) != 0 {
				t.Error("raw document correction did not retain one attempt and zero partial File effects")
			}
		}
		oldText := "original implementation"
		if ordinal > 1 {
			oldText = fmt.Sprintf("实际工作内容-%d", ordinal-1)
		}
		newText := fmt.Sprintf("实际工作内容-%d", ordinal)
		operations := []runtimecontract.StateChangingOperation{}
		for targetIndex, target := range targets {
			baseHash := ""
			for _, operation := range manifest.Operations {
				for _, binding := range operation.TargetParameterBindings {
					if binding.Name == "baseHash" {
						for _, value := range binding.Values {
							if value.TargetRef == target {
								baseHash = value.Value
							}
						}
					}
				}
			}
			kind := runtimecontract.StepKindFileEdit
			parameters := []runtimecontract.OperationParameter{{Name: "oldText", String: &oldText}, {Name: "newText", String: &newText}, {Name: "baseHash", String: &baseHash}}
			if empty && ordinal == 1 {
				kind = runtimecontract.StepKindFileWrite
				parameters = []runtimecontract.OperationParameter{{Name: "content", String: &newText}}
			}
			requirement := ""
			for _, required := range manifest.RequiredOperationSequence {
				if required.TargetRef == target {
					requirement = required.RequirementID
				}
			}
			operations = append(operations, runtimecontract.StateChangingOperation{Operation: runtimecontract.RuntimeOperation{ID: fmt.Sprintf("work-operation-%d-%d", ordinal, targetIndex), Kind: kind, TargetRefs: []string{target}, RequiredOperationRef: requirement, Parameters: parameters}})
		}
		candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: operations}}
		if injectCardinality && operationCalls == 1 {
			// Supply the original invalid bytes, never a partially decoded DTO.
			response := runtimeCandidateResultForTest(t, command, candidate)
			var document map[string]json.RawMessage
			if response.DocumentOutcome == nil || json.Unmarshal(response.DocumentOutcome.Content, &document) != nil {
				t.Error("missing fixture document")
				result.Set(response, nil)
				return
			}
			var rawOperations []map[string]json.RawMessage
			json.Unmarshal(document["operations"], &rawOperations)
			for i := range rawOperations {
				var parameters []map[string]any
				json.Unmarshal(rawOperations[i]["parameters"], &parameters)
				name := "newText"
				if empty {
					name = "content"
				}
				for _, parameter := range parameters {
					if parameter["name"] == name {
						parameter["string"] = "实际工作内容-"
					}
				}
				parameters = append(parameters, map[string]any{"name": name, "string": "1"})
				rawOperations[i]["parameters"], _ = json.Marshal(parameters)
			}
			document["operations"], _ = json.Marshal(rawOperations)
			body, _ := json.Marshal(document)
			result.Set(runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{DocumentOutcome: runtimeCompleteDocumentOutcome(command.ExpectedDocument, body)}), nil)
			return
		}
		result.Set(runtimeCandidateResultForTest(t, command, candidate), nil)
	})
	t.Cleanup(func() { provider.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })
	originalDefinition := aggregate.Definition
	originalFocus := aggregate.Facts.DefinitionFocuses[0]
	for transition := 0; transition < runtimeDriveTransitionLimit; transition++ {
		outcome, err := module.driveOneRuntimeTransition(ctx, nil, aggregate, orchestratorevents.DriveRuntimeCommand{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(aggregate.Definition, aggregate.Facts.Activation.ExecutionContract))
		if err != nil {
			t.Fatalf("production transition %d phase=%s state=%s: %v", transition, aggregate.SemanticPlanning.Phase, aggregate.Decision.State, err)
		}
		aggregate = outcome.aggregate
		if len(aggregate.ImplementationWork.Rounds) == 2 && aggregate.ImplementationWork.Rounds[1].CompletedReceiptSetHash != "" {
			break
		}
		if outcome.result != nil {
			t.Fatalf("unexpected terminal before two real work receipts: %+v failure=%+v", outcome.result, aggregate.Facts.Failure)
		}
	}
	expectedOperationCalls := 2
	if injectCardinality {
		expectedOperationCalls++
	}
	if len(aggregate.ImplementationWork.Rounds) != 2 || aggregate.ImplementationWork.Rounds[0].CompletedReceiptSetHash == "" || aggregate.ImplementationWork.Rounds[1].CompletedReceiptSetHash == "" || operationCalls != expectedOperationCalls || semanticCalls != 6 || committedReceipts != 2*len(targets) {
		t.Fatalf("two-round production chain incomplete: rounds=%+v semantic=%d operations=%d failure=%+v", aggregate.ImplementationWork.Rounds, semanticCalls, operationCalls, aggregate.Facts.Failure)
	}
	first, second := aggregate.ImplementationWork.Rounds[0], aggregate.ImplementationWork.Rounds[1]
	if first.CoverageWorkItemID != second.CoverageWorkItemID || second.PrecedingReceiptSetHash != first.CompletedReceiptSetHash || first.ScopeObservation.Selection.ObservedRevision == second.ScopeObservation.Selection.ObservedRevision {
		t.Fatal("same CWI second work round did not require fresh owner scope and prior collective receipt")
	}
	if !reflect.DeepEqual(aggregate.Definition, originalDefinition) || !reflect.DeepEqual(aggregate.Facts.DefinitionFocuses[0], originalFocus) || aggregate.Facts.Coverage[0].CurrentBindingRef != nil {
		t.Fatal("working rounds rewrote startup or conferred terminal binding")
	}
	for _, fact := range aggregate.Facts.Evidence.Accepted {
		if len(fact.BindingRefs) != 0 || len(fact.CoverageWorkItemIDs) != 0 || fact.Validation != nil || fact.Quality != "" {
			t.Fatal("real File working receipt acquired quality or terminal validation credit")
		}
	}
	workspace, err := (workspaceSnapshotReader{hub: hub}).Read(ctx, aggregate.Facts.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		content, err := os.ReadFile(filepath.Join(workspace.DeliverableDirectory, target))
		if err != nil || string(content) != "实际工作内容-2" {
			t.Fatalf("real File final content: %q err=%v", content, err)
		}
	}
	var dispatches []runtimecontract.RuntimeDispatchRecord
	for _, value := range aggregate.CompletedDispatches {
		value := value
		dispatches = append(dispatches, runtimeDispatchRecordFromAggregate(value))
	}
	usage, err := runtimecontract.RuntimeDurableDispatchUsage(nil, dispatches, aggregate.CandidateAttempts)
	if err != nil || usage[first.CoverageWorkItemID] != 2 {
		t.Fatalf("two original work seals must consume two original-CWI durable slots: usage=%v err=%v", usage, err)
	}
	if len(dispatches) != 2 {
		t.Fatalf("two durable working dispatches expected: %d", len(dispatches))
	}
	firstAttempts := []runtimecontract.RuntimeCandidateAttemptRecord{}
	for _, attempt := range aggregate.CandidateAttempts {
		if attempt.DispatchID == dispatches[0].ID {
			firstAttempts = append(firstAttempts, attempt)
		}
	}
	limits := &runtimecontract.RuntimeExecutionLimits{ProtocolRef: runtimecontract.RuntimeExecutionLimitsV1, MaxProviderCalls: 20, MaxCapabilityOperations: 20, MaxDurableDispatchesPerCWI: 1}
	var exhausted *runtimecontract.RuntimeDurableDispatchBudgetExhaustedError
	if err := runtimecontract.CheckRuntimeDurableDispatchBudget(limits, nil, dispatches[:1], firstAttempts, dispatches[1]); !errors.As(err, &exhausted) || exhausted.CoverageWorkItemID != first.CoverageWorkItemID {
		t.Fatalf("max1 must reject same-CWI second work seal: %v", err)
	}
	limits.MaxDurableDispatchesPerCWI = 2
	if err := runtimecontract.CheckRuntimeDurableDispatchBudget(limits, nil, dispatches[:1], firstAttempts, dispatches[1]); err != nil {
		t.Fatalf("max2 must retain original two-seal allocation: %v", err)
	}
	archived := append([]runtimecontract.RuntimeDispatchRecord(nil), dispatches...)
	for index := range archived {
		archived[index].State = runtimecontract.RuntimeDispatchStateInvalidated
	}
	if usage, err := runtimecontract.RuntimeDurableDispatchUsage(nil, archived, aggregate.CandidateAttempts); err != nil || usage[first.CoverageWorkItemID] != 2 {
		t.Fatalf("archive or unknown disposition cannot refund durable original seals: %v %v", usage, err)
	}
	if err := runtimecontract.ValidateRuntimeDispatchAdmissionLineage(nil, dispatches, firstAttempts, nil); err == nil {
		t.Fatal("deleting accepted attempt refunded its original durable dispatch")
	}
	altered := append([]runtimecontract.RuntimeCandidateAttemptRecord(nil), aggregate.CandidateAttempts...)
	for index := range altered {
		if altered[index].State == runtimecontract.RuntimeCandidateAttemptStateAccepted {
			altered[index].AdmissionImplementationWorkCommitment = nil
			break
		}
	}
	if _, err := runtimecontract.RuntimeDurableDispatchUsage(nil, dispatches, altered); err == nil {
		t.Fatal("work durable budget borrowed dispatch-self-asserted commitment without accepted source")
	}
	if len(targets) > 1 && !empty {
		assertImplementationWorkChainGuards(t, runtimeStateRecordFromAggregate(aggregate))
	}
	fresh, err := module.runtimeController.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil || fresh.StateRevision != repo.aggregate.StateRevision || len(fresh.ImplementationWork.Rounds) != 2 {
		t.Fatalf("same-process aggregate reopen failed: %v", err)
	}
}
