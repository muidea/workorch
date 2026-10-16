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

func runRuntimeCWISuspensionMainline(t *testing.T, cut string) int64 {
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
	limits := &runtimecontract.RuntimeExecutionLimits{ProtocolRef: runtimecontract.RuntimeExecutionLimitsV1, MaxProviderCalls: 64, MaxCapabilityOperations: 32, MaxDurableDispatchesPerCWI: 2}
	bootstrap.Activation.ExecutionContract.ExecutionLimits = limits
	var err error
	bootstrap.ExecutionBudgetPolicy, err = runtimecontract.RuntimeExecutionBudgetPolicyWithLimits(bootstrap.ConvergencePolicy, bootstrap.RecoveryPolicy, limits)
	if err != nil {
		t.Fatal(err)
	}
	bootstrap = runtimeTestBootstrapWithBindings(bootstrap)
	if processStage != "resume" {
		runtimeTestPrepareNativeValidationThroughOwners(t, bootstrap, deliverable, func(preparation runtimecontract.RuntimeValidationProofPreparation) string {
			var source strings.Builder
			source.WriteString("package workorch_validation\nimport (\"testing\"; subject \"example.com/workorch-runtime-native-fixture\")\n")
			for _, check := range preparation.CheckAttestations {
				fmt.Fprintf(&source, "func %s(t *testing.T) { if subject.DeliveredValue() < 2 { t.Fatal(\"actual current behavior below sealed minimum\") } }\n", check.NativeCheckRef)
			}
			return source.String()
		})
		if err := os.WriteFile(filepath.Join(deliverable, "subject.go"), []byte("package fixture\nfunc DeliveredValue() int { return 2 }\n"), 0644); err != nil {
			t.Fatal(err)
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
		if command.DocumentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan {
			document := validationInputAdditionMainlineSemanticDocument(command)
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
		crash := &runtimeSaveCrashRepository{delegate: runtimeStateRepository{hub: hub}, after: strings.HasPrefix(cut, "after_"), marker: cut, match: func(a runtime.RuntimeAggregate) bool {
			if len(a.Facts.CWISuspensions) != 1 {
				return false
			}
			resumed := a.Facts.CWISuspensions[0].ResumeBinding != nil
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
	if len(a.Facts.CWISuspensions) != 1 || a.Facts.CWISuspensions[0].ResumeBinding == nil || !runtimeTerminalDeliveryFullyAcknowledged(a) {
		t.Fatalf("missing exact pause/resume/ACK: %+v", a.Facts.CWISuspensions)
	}
	changed := 0
	for _, receipt := range a.OperationReceipts {
		if receipt.Changed && runtimecontract.RuntimeEffectIsDurable(receipt.ObservedEffectScope) {
			changed++
		}
	}
	if changed != 1 || !reflect.DeepEqual(persisted.RuntimeBootstrap, initialRun.RuntimeBootstrap) {
		t.Fatalf("suspension replayed effect or changed sealed startup: durableChanges=%d startupSame=%t", changed, reflect.DeepEqual(persisted.RuntimeBootstrap, initialRun.RuntimeBootstrap))
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
	return count
}

func runtimeAssertCWISuspensionArchiveTamperRejected(t *testing.T, original runtime.RuntimeAggregate) {
	t.Helper()
	for _, scenario := range []string{"protocol", "future_pause", "duplicate_pause", "replayed_pause_source", "effect_source", "predecessor_source", "frozen_binding", "frozen_focus", "reopen_source", "missing_resume", "old_credit", "missing_assessment", "missing_reads", "missing_original_reads"} {
		t.Run("reject_"+scenario, func(t *testing.T) {
			a := original.CloneRaw()
			s := &a.Facts.CWISuspensions[0]
			switch scenario {
			case "protocol":
				s.ProtocolRef = "unknown"
			case "future_pause":
				s.SuspendedAtStateRevision = a.StateRevision + 1
			case "duplicate_pause":
				a.Facts.CWISuspensions = append(a.Facts.CWISuspensions, s.CloneRaw())
			case "replayed_pause_source":
				copy := s.CloneRaw()
				copy.SuspendedAtStateRevision++
				a.Facts.CWISuspensions = append(a.Facts.CWISuspensions, copy)
			case "effect_source":
				s.EffectEvidenceID = "missing"
			case "predecessor_source":
				s.PredecessorID = "missing"
			case "frozen_binding":
				s.FrozenBinding.BindingHash = "forged"
			case "frozen_focus":
				s.FrozenFocus.TargetRefs = []string{"forged"}
			case "reopen_source":
				s.ReopenSource.TriggerRef = "forged"
			case "missing_resume":
				s.ResumeBinding = nil
			case "old_credit":
				old := s.ReopenSource.PreviousBindingRef
				s.ResumeBinding = &old
			case "missing_assessment":
				s.ResumeAssessmentID = "missing"
			case "missing_reads":
				s.ResumeObservationCount = 0
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
