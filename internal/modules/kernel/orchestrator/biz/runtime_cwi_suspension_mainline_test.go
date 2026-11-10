package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	filemodule "github.com/muidea/workorch/internal/modules/blocks/file"
	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	resultmodule "github.com/muidea/workorch/internal/modules/blocks/result"
	resultbiz "github.com/muidea/workorch/internal/modules/blocks/result/biz"
	runmodule "github.com/muidea/workorch/internal/modules/blocks/run"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	workspacemodule "github.com/muidea/workorch/internal/modules/kernel/workspace"
	"github.com/muidea/workorch/internal/pkg/common"
	filerepository "github.com/muidea/workorch/internal/pkg/repository/file"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Real File/Shell/Run/Workspace/Result; scripted proposals are not live model
// verification. The ordinary predecessor is explicitly declared before sealing;
// the previous state-change-only fixture is not silently upgraded.
func TestDriveRuntimeCWISuspensionRevalidatesOrdinaryPredecessorWithinOriginalBudget(t *testing.T) {
	var baseline int64
	for _, cut := range []string{"uninterrupted", "before_suspend", "after_suspend", "before_resume", "after_resume"} {
		t.Run(cut, func(t *testing.T) {
			calls := runRuntimeCWISuspensionMainline(t, cut)
			if baseline == 0 {
				baseline = calls
			} else if calls != baseline {
				t.Fatalf("restart changed provider usage: got=%d baseline=%d", calls, baseline)
			}
		})
	}
}

func TestDriveRuntimeCWISuspensionMultipleConsumersAndRecovery(t *testing.T) {
	var baseline int64
	for _, cut := range []string{"uninterrupted", "after_suspend", "after_sources", "before_consumer", "after_consumer", "after_resume"} {
		t.Run(cut, func(t *testing.T) {
			calls := runRuntimeCWISuspensionMainline(t, cut, false, true)
			if baseline == 0 {
				baseline = calls
			} else if calls != baseline {
				t.Fatalf("multi-consumer recovery repeated provider work: %d != %d", calls, baseline)
			}
		})
	}
}

func runRuntimeCWISuspensionMainline(t *testing.T, cut string, historical ...bool) int64 {
	mapped := len(historical) > 0 && historical[0]
	multiple := len(historical) > 1 && historical[1]
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	processStage := os.Getenv("WORKORCH_CWI_SUSPENSION_STAGE")
	if processStage != "" {
		root = os.Getenv("WORKORCH_CWI_SUSPENSION_ROOT")
		if !filepath.IsAbs(root) || (processStage != "seed" && processStage != "resume") {
			t.Fatal("invalid isolated process fixture scope")
		}
	}
	deliverable, internalDir := filepath.Join(root, "workspace"), filepath.Join(root, "internal")
	for _, directory := range []string{deliverable, internalDir} {
		if err := os.MkdirAll(directory, 0755); err != nil {
			t.Fatal(err)
		}
	}
	bootstrap := validationInputAdditionMainlineBootstrap()
	bootstrap.CoverageItems[0].TargetText = "独立普通验收：DeliveredValue() 必须至少为 2"
	bootstrap.CoverageItems[1].TargetText = "修改 subject.go，并用独立普通证明验证 DeliveredValue() 至少为 2"
	bootstrap.DefinitionFocuses[0].RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	bootstrap.DefinitionFocuses[0].AcceptedStepKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
	bootstrap.DefinitionFocuses[1].RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}
	bootstrap.DefinitionFocuses[1].AcceptedStepKinds = []string{runtimecontract.StepKindFileEdit, runtimecontract.RuntimeBlockKindManagedValidationSession}
	bootstrap.DefinitionFocuses[1].ValidationFailureRepairConvergence = nil
	bootstrap.Activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements[0].RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	bootstrap.Activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements[0].AcceptedStepKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
	bootstrap.Activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements[0].ValidationRecipeID = "behavior-validation"
	bootstrap.Activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements[1].RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}
	bootstrap.Activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements[1].AcceptedStepKinds = []string{runtimecontract.StepKindFileEdit, runtimecontract.RuntimeBlockKindManagedValidationSession}
	bootstrap.Activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements[1].ValidationFailureRepairConvergence = nil
	runtimeTestDeclareExactManagedValidationOwnerFact(&bootstrap, "cwi-1", "behavior-validation", false)
	if multiple {
		id := common.CoverageWorkItemID("cwi-extra")
		focus := bootstrap.DefinitionFocuses[0].Clone()
		focus.ID, focus.AssessmentID, focus.EvidenceDeclarationRef = "extra-ordinary", string(id), ""
		focus.CoverageWorkItemIDs = []common.CoverageWorkItemID{id}
		bootstrap.DefinitionFocuses = append(bootstrap.DefinitionFocuses, focus)
		declaration := bootstrap.Activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements[0]
		declaration.ID = string(id)
		bootstrap.Activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements = append(bootstrap.Activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements, declaration)
		item := bootstrap.CoverageItems[0]
		item.ID, item.Order = id, 2
		item.PrerequisiteCoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-1"}
		bootstrap.CoverageItems[1].Order = 3
		bootstrap.CoverageItems[1].PrerequisiteCoverageWorkItemIDs = append(bootstrap.CoverageItems[1].PrerequisiteCoverageWorkItemIDs, id)
		bootstrap.CoverageItems = append(bootstrap.CoverageItems, item)
		bootstrap.Slices[0].CoverageWorkItemIDs = append(bootstrap.Slices[0].CoverageWorkItemIDs, id)
		runtimeTestDeclareExactManagedValidationOwnerFact(&bootstrap, id, "behavior-validation", false)
	}
	limits := &runtimecontract.RuntimeExecutionLimits{ProtocolRef: runtimecontract.RuntimeExecutionLimitsV1, MaxProviderCalls: 64, MaxCapabilityOperations: 32, MaxDurableDispatchesPerCWI: 2}
	bootstrap.Activation.ExecutionContract.ExecutionLimits = limits
	var err error
	bootstrap.ExecutionBudgetPolicy, err = runtimecontract.RuntimeExecutionBudgetPolicyWithLimits(bootstrap.ConvergencePolicy, bootstrap.RecoveryPolicy, limits)
	if err != nil {
		t.Fatal(err)
	}
	if mapped {
		bootstrap = withHistoricalSuspensionSource(t, bootstrap)
	}
	bootstrap = runtimeTestBootstrapWithBindings(bootstrap)
	if processStage != "resume" {
		runtimeTestPrepareNativeValidationThroughOwners(t, bootstrap, deliverable, func(preparation runtimecontract.RuntimeValidationProofPreparation) string {
			var source strings.Builder
			source.WriteString("package workorch_validation\nimport (\"testing\"; subject \"example.com/workorch-runtime-native-fixture\")\n")
			for _, check := range preparation.CheckAttestations {
				condition := "subject.DeliveredValue() < 2"
				if mapped {
					condition += " || subject.HistoricalValue() != 1"
				}
				fmt.Fprintf(&source, "func %s(t *testing.T) { if %s { t.Fatal(\"actual current behavior below sealed minimum\") } }\n", check.NativeCheckRef, condition)
			}
			return source.String()
		})
		if err := os.WriteFile(filepath.Join(deliverable, "subject.go"), []byte("package fixture\nfunc DeliveredValue() int { return 2 }\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if mapped {
			if err := os.WriteFile(filepath.Join(deliverable, "history.go"), []byte("package fixture\nfunc HistoricalValue() int { return 0 }\n"), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	workspaces := filerepository.NewWorkspaceRepository(filepath.Join(root, "workspaces.json"))
	results := filerepository.NewResultRepository(filepath.Join(root, "results.json"))
	if processStage != "resume" {
		if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2", DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive}); err != nil {
			t.Fatal(err)
		}
		if err := workspaces.Create(ctx, runtimeWorkspaceForPersistedRunTest(t, ctx, runs, "ws-v2", "run-v2", deliverable, internalDir)); err != nil {
			t.Fatal(err)
		}
	}
	initialRun, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	workspaceOwner := workspacemodule.New()
	if err := workspaceOwner.Configure(workspaceOwnerTestDependencies(hub, workspaces, workspaces, memory.NewGoalRepository(), memory.NewDoDRepository(), memory.NewWorkspaceTemplateRepository())); err != nil {
		t.Fatal(err)
	}
	if err := workspaceOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workspaceOwner.Stop(ctx) })
	fileOwner := filemodule.New()
	if err := fileOwner.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := fileOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fileOwner.Stop(ctx) })
	startRuntimeReceiptTestOwner(t, hub)
	startRuntimeManagedValidationTestOwner(t, hub)
	startRuntimeProviderCeilingTestOwner(t, hub)
	resultOwner := resultmodule.New()
	if err := resultOwner.Configure(resultbiz.Dependencies{OwnerRepositories: testResultOwnerRepositories(results, memory.NewArtifactRepository(), memory.NewResultWorkspaceDeletionRepository()), EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := resultOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resultOwner.Stop(ctx) })
	var calls atomic.Int64
	llm := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	if err := llm.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		calls.Add(1)
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		if command.DocumentKind == runtimecontract.RuntimeProviderDocumentOperations && len(command.Decision.Focus.CoverageWorkItemIDs) == 1 && command.Decision.Focus.CoverageWorkItemIDs[0] == "cwi-2" {
			persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil || persisted.RuntimeState == nil {
				t.Errorf("candidate has no committed source: %v", err)
				return
			}
			facts := persisted.RuntimeState.Facts
			for _, current := range facts.Coverage {
				if current.ID != "cwi-2" {
					continue
				}
				for _, id := range current.PrerequisiteCoverageWorkItemIDs {
					for _, dependency := range facts.Coverage {
						if dependency.ID == id && !runtimecontract.RuntimeCWIPrerequisiteReadyForConsumer(facts, dependency, current.ID) {
							t.Errorf("blocked successor requested a provider candidate: %s", id)
							return
						}
					}
				}
			}
		}
		if command.DocumentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan {
			document := validationInputAdditionMainlineSemanticDocument(command)
			if mapped {
				for i := range document.Proposals {
					if document.Proposals[i].CoverageWorkItemID == "cwi-history" {
						document.Proposals[i].Target.ExactTargetRefs = []string{"history.go"}
					}
				}
			}
			if document.PlanStep == runtimecontract.RuntimeSemanticPlanStepProofBatch {
				for _, id := range document.BatchCoverageWorkItemIDs {
					if id != "cwi-2" {
						continue
					}
					ref, err := runtimeTestSemanticCurrentDeclarationRefForCoverage(command.SemanticManifest, id)
					if err != nil {
						t.Error(err)
						return
					}
					document.Proposals = append(document.Proposals, runtimecontract.RuntimeSemanticProofProposal{CoverageWorkItemID: id, OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKind: runtimecontract.StepKindFileEdit, Target: runtimecontract.RuntimeOperationTargetAuthorization{Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"subject.go"}}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}, DeclarationRef: ref})
				}
			}
			payload, err := runtimecontract.EncodeRuntimeSemanticPlanPayloadWithManifest(command.SemanticManifest, document)
			if err != nil {
				t.Error(err)
				return
			}
			result.Set(runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity, ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument), DocumentOutcome: runtimeCompleteDocumentOutcome(command.ExpectedDocument, payload)}), nil)
			return
		}
		id := command.Decision.Focus.CoverageWorkItemIDs[0]
		var candidate runtimecontract.RuntimeCandidate
		switch command.Decision.State {
		case runtimecontract.ConvergenceStateNeedStateChange:
			if id == "cwi-history" && mapped {
				oldText, newText := "return 0", "return 1"
				candidate = runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{ID: "original-historical-effect", Kind: runtimecontract.StepKindFileEdit, TargetRefs: []string{"history.go"}, CoverageWorkItemIDs: []common.CoverageWorkItemID{id}, Parameters: []runtimecontract.OperationParameter{{Name: "oldText", String: &oldText}, {Name: "newText", String: &newText}}}}}}}
				break
			}
			if id != "cwi-2" {
				t.Error("predecessor revalidation attempted an effect")
				return
			}
			oldText, newText := "return 2", "return 3"
			candidate = runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{ID: "single-successor-effect", Kind: runtimecontract.StepKindFileEdit, TargetRefs: []string{"subject.go"}, CoverageWorkItemIDs: []common.CoverageWorkItemID{id}, Parameters: []runtimecontract.OperationParameter{{Name: "oldText", String: &oldText}, {Name: "newText", String: &newText}}}}}}}
		case runtimecontract.ConvergenceStateNeedValidation:
			candidate = runtimeManagedShellValidationCandidate("validate-current", id, "subject.go", "behavior-validation")
		case runtimecontract.ConvergenceStateNeedAssessment:
			candidate = runtimeSatisfiedAssessmentCandidate(command, "actual current ordinary proof")
		default:
			t.Errorf("unexpected action %s", command.Decision.State)
			return
		}
		payload := runtimeCandidateDocumentForTest(t, command.CandidateManifest, candidate)
		result.Set(runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity, ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument), DocumentOutcome: runtimeCompleteDocumentOutcome(command.ExpectedDocument, payload)}), nil)
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = llm.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })
	if processStage != "resume" {
		runtimeTestPersistBootstrappedPreSemanticFixture(t, ctx, hub, "ws-v2", "run-v2", bootstrap)
	}
	owner := newRuntimeDriveTestModule(t, hub)
	command := orchestratorevents.DriveRuntimeCommand{WorkspaceID: "ws-v2", RunID: "run-v2"}
	if cut != "uninterrupted" && processStage != "resume" {
		crash := &runtimeSaveCrashRepository{delegate: runtimeStateRepository{hub: hub}, after: strings.HasPrefix(cut, "after_") || cut == "before_effect", marker: cut, match: func(a runtime.RuntimeAggregate) bool {
			if cut == "before_effect" {
				return a.ActiveDispatch == nil && a.Facts.Focus.CoverageWorkItemIDs[0] == "cwi-2" && len(a.Facts.Focus.CoverageEvidenceBindings) == 1 && a.Decision.State == runtimecontract.ConvergenceStateNeedStateChange
			}
			if len(a.Facts.CWISuspensions) != 1 {
				return false
			}
			resumed := !a.Facts.CWISuspensions[0].Active()
			if cut == "after_sources" {
				return len(a.Facts.CWISuspensions[0].Consumers) == 2
			}
			if strings.HasSuffix(cut, "consumer") {
				pause := a.Facts.CWISuspensions[0]
				completed := 0
				for _, consumer := range pause.Consumers {
					if consumer.ResumeBinding != nil {
						completed++
					}
				}
				return pause.Active() && completed == 1
			}
			return strings.HasSuffix(cut, "resume") == resumed
		}}
		if err := owner.ConfigureRuntimeRepository(crash); err != nil {
			t.Fatal(err)
		}
		func() {
			defer func() {
				if recovered := recover(); recovered != cut {
					t.Fatalf("expected %s crash, got=%v", cut, recovered)
				}
				if processStage == "seed" {
					os.Exit(crossCWIProcessCutExit) // Durable CAS; no owner cleanup or in-process continuation.
				}
			}()
			response, err := owner.driveRuntimeLocked(ctx, command)
			t.Fatalf("crash cut not reached: response=%+v err=%v", response, err)
		}()
		if !crash.fired {
			t.Fatal("crash did not fire")
		}
		if cut == "before_effect" {
			command.ReconciliationRequestID = "observe-external-selector-change"
			// A real external metadata change before the successor effect.
			// The Shell owner must observe this exact selector; no owner
			// result, authority, criterion or operation is manufactured.
			path := filepath.Join(deliverable, "subject.go")
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, info.Mode().Perm()^0040); err != nil {
				t.Fatal(err)
			}
		}
		// Independent owner repository object reloads the exact JSON record;
		// drive/controller is reconstructed. This is not an OS-process test.
		persisted, err := filerepository.NewRunRepository(filepath.Join(root, "runs.json")).GetByWorkspaceID(ctx, "ws-v2", "run-v2")
		if err != nil || persisted.RuntimeState == nil {
			t.Fatalf("reload crash preimage: %v", err)
		}
		if err := runtimeAggregateFromRecord(*persisted.RuntimeState).Validate(); err != nil {
			t.Fatal(err)
		}
		owner = newRuntimeDriveTestModule(t, hub)
	}
	completed, err := owner.driveRuntime(ctx, command)
	if err != nil || completed.State != runtimecontract.ConvergenceStateComplete {
		a, _ := owner.loadRuntime(ctx, "ws-v2", "run-v2")
		t.Fatalf("suspension mainline did not close: %v state=%s focus=%s failure=%+v pauses=%+v", err, completed.State, a.Facts.Focus.ID, a.Facts.Failure, a.Facts.CWISuspensions)
	}
	if err := owner.driveAndProjectRuntimeLockedWithCommand(ctx, command); err != nil {
		t.Fatal(err)
	}
	persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	a := runtimeAggregateFromRecord(*persisted.RuntimeState)
	if err := a.Validate(); err != nil {
		t.Fatal(err)
	}
	wantPauses := 1
	if cut == "before_effect" {
		wantPauses = 2
	}
	if len(a.Facts.CWISuspensions) != wantPauses || a.Facts.CWISuspensions[0].Active() || !runtimeTerminalDeliveryFullyAcknowledged(a) {
		t.Fatalf("missing exact pause/resume/ACK: %+v", a.Facts.CWISuspensions)
	}
	if multiple {
		pause := a.Facts.CWISuspensions[0]
		if len(pause.Consumers) != 2 {
			t.Fatal("independent consumers missing", pause.Consumers)
		}
		for _, consumer := range pause.Consumers {
			if consumer.ResumeBinding == nil || consumer.ResumeAssessmentID == "" || consumer.ResumeObservationCount == 0 {
				t.Fatal("consumer restored without its own ordinary proof", consumer)
			}
		}
	}
	changed := 0
	for _, receipt := range a.OperationReceipts {
		if receipt.Changed && runtimecontract.RuntimeEffectIsDurable(receipt.ObservedEffectScope) {
			changed++
		}
	}
	wantChanges := 1
	if mapped {
		wantChanges++
	}
	if changed != wantChanges || !reflect.DeepEqual(persisted.RuntimeBootstrap, initialRun.RuntimeBootstrap) {
		t.Fatalf("suspension replayed effect or changed sealed startup: durableChanges=%d startupSame=%t", changed, reflect.DeepEqual(persisted.RuntimeBootstrap, initialRun.RuntimeBootstrap))
	}
	if mapped {
		for _, row := range a.Facts.Coverage {
			if row.ID == "cwi-history" && (row.HistoricalBehavior == nil || len(row.HistoricalAcceptanceProofs) != wantPauses+1 || row.State != runtimecontract.RuntimeCoverageItemStateSatisfied) {
				t.Fatalf("historical acceptance did not revalidate through its consumer: %+v", row)
			}
		}
	}
	runtimeAssertCWISuspensionArchiveTamperRejected(t, a)
	before, count := persisted.RuntimeState.Clone(), calls.Load()
	for range 2 {
		if _, err := newRuntimeDriveTestModule(t, hub).driveRuntime(ctx, command); err != nil {
			t.Fatal(err)
		}
	}
	after, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || calls.Load() != count || !reflect.DeepEqual(after.RuntimeState, &before) {
		t.Fatal("duplicate terminal changed receipt/budget/provider")
	}
	if cut == "uninterrupted" {
		runtimeAssertReconciliationFormalResultReads(t, ctx, hub, persisted, a, deliverable)
		unchanged, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
		if err != nil || calls.Load() != count || !reflect.DeepEqual(unchanged.RuntimeState, &before) {
			t.Fatal("formal result query changed aggregate/provider/budget/receipts")
		}
	}
	return count
}

func runtimeAssertCWISuspensionArchiveTamperRejected(t *testing.T, original runtime.RuntimeAggregate) {
	t.Helper()
	for _, scenario := range []string{"protocol", "future_pause", "missing_evidence_revision", "future_evidence_revision", "dropped_effect", "duplicate_pause", "replayed_pause_source", "effect_source", "predecessor_source", "frozen_binding", "frozen_focus", "reopen_source", "missing_resume", "old_credit", "missing_assessment", "missing_reads", "missing_original_reads"} {
		t.Run("reject_"+scenario, func(t *testing.T) {
			a := original.CloneRaw()
			s := &a.Facts.CWISuspensions[0]
			switch scenario {
			case "protocol":
				s.ProtocolRef = "unknown"
			case "future_pause":
				s.SuspendedAtStateRevision = a.StateRevision + 1
			case "missing_evidence_revision":
				s.SuspendedEvidenceRevision = ""
			case "future_evidence_revision":
				s.SuspendedEvidenceRevision = a.Facts.EvidenceRevision + "~"
			case "dropped_effect":
				for i := range a.Facts.CWISuspensions {
					if a.Facts.CWISuspensions[i].EffectEvidenceID != "" {
						a.Facts.CWISuspensions[i].EffectEvidenceID = ""
						break
					}
				}
			case "duplicate_pause":
				a.Facts.CWISuspensions = append(a.Facts.CWISuspensions, s.CloneRaw())
			case "replayed_pause_source":
				copy := s.CloneRaw()
				copy.SuspendedAtStateRevision++
				a.Facts.CWISuspensions = append(a.Facts.CWISuspensions, copy)
			case "effect_source":
				s.EffectEvidenceID = "missing"
			case "predecessor_source":
				s.Consumers[0].PredecessorID = "missing"
			case "frozen_binding":
				s.FrozenBinding.BindingHash = "forged"
			case "frozen_focus":
				s.FrozenFocus.TargetRefs = []string{"forged"}
			case "reopen_source":
				s.Consumers[0].ReopenSource.TriggerRef = "forged"
			case "missing_resume":
				s.Consumers[0].ResumeBinding = nil
			case "old_credit":
				old := s.Consumers[0].ReopenSource.PreviousBindingRef
				s.Consumers[0].ResumeBinding = &old
			case "missing_assessment":
				s.Consumers[0].ResumeAssessmentID = "missing"
			case "missing_reads":
				s.Consumers[0].ResumeObservationCount = 0
			case "missing_original_reads":
				a.Facts.EvidenceApplicabilityObservations = nil
			}
			if a.Validate() == nil {
				t.Fatal("coordinated archive source drift was accepted")
			}
			if reflect.DeepEqual(a.Facts.CWISuspensions, original.Facts.CWISuspensions) && scenario != "missing_original_reads" {
				t.Fatal("raw clone aliased source")
			}
		})
	}
}

func TestDriveRuntimeMappedDependencyBeforeSuccessorEffect(t *testing.T) {
	runRuntimeCWISuspensionMainline(t, "before_effect", true)
}

func TestDriveRuntimeMappedDependencyContinuationAndRecovery(t *testing.T) {
	var baseline int64
	for _, cut := range []string{"uninterrupted", "before_suspend", "after_suspend", "before_resume", "after_resume"} {
		t.Run(cut, func(t *testing.T) {
			calls := runRuntimeCWISuspensionMainline(t, cut, true)
			if baseline == 0 {
				baseline = calls
			} else if baseline != calls {
				t.Fatalf("recovery repeated provider work: %d != %d", calls, baseline)
			}
		})
	}
}

// A fresh three-CWI contract: real historical File effect, independent ordinary
// acceptance, and a successor whose edit invalidates that exact acceptance.
func withHistoricalSuspensionSource(t *testing.T, b runtimecontract.RuntimeBootstrap) runtimecontract.RuntimeBootstrap {
	t.Helper()
	history := runtimecontract.RuntimeCoverageItem{ID: "cwi-history", SliceID: "slice-mainline", SourceScope: "dodItem", SourceID: "cwi-history", TargetText: "通过原 File 操作将 HistoricalValue 从 0 改为 1；当前正确性由 cwi-1 独立验收", Order: 1}
	for i := range b.CoverageItems {
		b.CoverageItems[i].Order++
		b.CoverageItems[i].SourceScope = "dodItem"
		b.CoverageItems[i].SourceID = string(b.CoverageItems[i].ID)
		b.CoverageItems[i].PrerequisiteCoverageWorkItemIDs = append(b.CoverageItems[i].PrerequisiteCoverageWorkItemIDs, history.ID)
	}
	b.CoverageItems = append([]runtimecontract.RuntimeCoverageItem{history}, b.CoverageItems...)
	b.DefinitionFocuses = append([]runtimecontract.RuntimeDefinitionFocus{{ID: "focus-history", CoverageWorkItemIDs: []common.CoverageWorkItemID{history.ID}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, AcceptedStepKinds: []string{runtimecontract.StepKindFileEdit}, TargetRole: "deliverable", RequireTargetGrounding: true}}, b.DefinitionFocuses...)
	profile := &b.Activation.ExecutionContract.ProfileContracts[0]
	profile.EvidenceRequirements = append([]runtimecontract.RuntimeProfileEvidenceRequirement{{ID: "cwi-history", Scope: "dodItem", TargetRole: "deliverable", TargetRefs: []string{"history.go"}, RequireTargetGrounding: true, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, AcceptedStepKinds: []string{runtimecontract.StepKindFileEdit}}}, profile.EvidenceRequirements...)
	for i := range profile.EvidenceRequirements {
		profile.EvidenceRequirements[i].Scope = "dodItem"
	}
	b.Slices[0].CoverageWorkItemIDs = append([]common.CoverageWorkItemID{history.ID}, b.Slices[0].CoverageWorkItemIDs...)
	inventory := runtimecontract.RuntimeSourceObligationInventory{SchemaVersion: runtimecontract.RuntimeSourceObligationInventorySchemaCurrent, CandidateHash: "historical-suspension-candidate", SpecificationHash: "historical-suspension-specification", DefinitionRevision: b.Definition.Revision, StartupInputHash: b.Definition.StartupInputHash, Sources: []runtimecontract.RuntimeDefinitionSource{{CandidateItemRef: "goal", ItemRef: "formal-goal", SourceScope: "goal", SourceID: "goal", Text: b.Definition.GoalDefinition}}, EvidencePurposeMappings: []runtimecontract.RuntimeEvidencePurposeMapping{{ContractRef: runtimecontract.RuntimeCrossCWIEvidencePurpose, HistoricalItemRef: "cwi-history", AcceptanceItemRef: "cwi-1"}}}
	for _, item := range b.CoverageItems {
		id := string(item.ID)
		inventory.Sources = append(inventory.Sources, runtimecontract.RuntimeDefinitionSource{CandidateItemRef: id, ItemRef: "formal/" + id, SourceScope: item.SourceScope, SourceID: item.SourceID, Text: item.TargetText})
		inventory.Requirements = append(inventory.Requirements, runtimecontract.RuntimeSourceRequirement{ID: "requirement/" + id, DimensionID: "dimension/" + id, Required: true, SourceKind: "USER_EXPLICIT", SourceItemRefs: []string{id}, Text: item.TargetText})
		inventory.Obligations = append(inventory.Obligations, runtimecontract.RuntimeSourceObligation{ID: "obligation/" + id, RequirementID: "requirement/" + id, DoDItemRefs: []string{id}, Text: item.TargetText})
		inventory.Verifications = append(inventory.Verifications, runtimecontract.RuntimeSourceVerification{ID: "verification/" + id, ObligationID: "obligation/" + id, Declaration: item.TargetText, ProfileID: profile.ID, EvidenceRequirementID: item.SourceID, Scope: item.SourceScope})
	}
	inventory = inventory.WithHash()
	if err := inventory.Validate(); err != nil {
		t.Fatal(err)
	}
	b.Definition.SourceObligations = &inventory
	items, err := runtimecontract.CompileRuntimeEvidencePurposeMappingCoverage(inventory, b.CoverageItems)
	if err != nil {
		t.Fatal(err)
	}
	for i := range items {
		items[i].SourceObligationChecks, err = runtimecontract.CompileRuntimeSourceObligationChecks(inventory, items[i])
		if err != nil {
			t.Fatal(err)
		}
	}
	b.CoverageItems = items
	return b
}
