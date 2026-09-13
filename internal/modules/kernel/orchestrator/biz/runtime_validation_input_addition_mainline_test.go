package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	filemodule "github.com/muidea/workorch/internal/modules/blocks/file"
	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	resultmodule "github.com/muidea/workorch/internal/modules/blocks/result"
	resultbiz "github.com/muidea/workorch/internal/modules/blocks/result/biz"
	runmodule "github.com/muidea/workorch/internal/modules/blocks/run"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	workspacemodule "github.com/muidea/workorch/internal/modules/blocks/workspace"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	filerepository "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Only the provider is scripted. File/Shell execute real operations, Run owns
// the file-backed CAS, and every semantic/dispatch/assessment transition goes
// through the Orchestrator drive and its private events. No repair binding,
// receipt, or acceptance credit is supplied by the harness.
func TestDriveRuntimeValidationInputAdditionPreservesHistoricalAssessment(t *testing.T) {
	var uninterruptedCalls int64
	for _, cut := range []string{"uninterrupted", "before_repair_seal", "after_repair_seal", "after_input_receipt", "after_fresh_validation"} {
		t.Run(cut, func(t *testing.T) {
			calls := runValidationInputAdditionMainline(t, cut)
			if cut == "uninterrupted" {
				uninterruptedCalls = calls
			} else if uninterruptedCalls != 0 && calls != uninterruptedCalls {
				t.Fatalf("restart consumed another provider call: got=%d uninterrupted=%d", calls, uninterruptedCalls)
			}
		})
	}
}

func TestDriveRuntimeExecutionLimitsPreserveRealOwnerCompletionAcrossRestart(t *testing.T) {
	var baseline int64
	for _, cut := range []string{"uninterrupted", "after_input_receipt"} {
		t.Run(cut, func(t *testing.T) {
			calls := runValidationInputAdditionMainline(t, cut, validationInputMainlineOptions{ExecutionLimits: &runtimecontract.RuntimeExecutionLimits{
				ProtocolRef: runtimecontract.RuntimeExecutionLimitsV1, MaxProviderCalls: 64, MaxCapabilityOperations: 32, MaxProofBatchesPerCWI: 4, MaxDurableDispatchesPerCWI: 2,
			}})
			if calls < 2 || calls > 64 {
				t.Fatalf("physical calls outside declared budget: %d", calls)
			}
			if cut == "uninterrupted" {
				baseline = calls
			} else if calls != baseline {
				t.Fatal("restart consumed additional provider calls")
			}
		})
	}
}

func TestDriveRuntimeProofBudgetExhaustionClosesWithoutAnotherProviderOrEffect(t *testing.T) {
	runValidationInputAdditionMainline(t, "uninterrupted", validationInputMainlineOptions{
		ProofBudgetExhaustion: true,
		ExecutionLimits: &runtimecontract.RuntimeExecutionLimits{
			ProtocolRef: runtimecontract.RuntimeExecutionLimitsV1, MaxProviderCalls: 64, MaxCapabilityOperations: 32, MaxProofBatchesPerCWI: 1,
		},
	})
}

func TestDriveRuntimeDurableDispatchBudgetBoundsAdditionAndRepairAcrossRestart(t *testing.T) {
	for _, cut := range []string{"uninterrupted", "after_input_receipt"} {
		t.Run(cut, func(t *testing.T) {
			runValidationInputAdditionMainline(t, cut, validationInputMainlineOptions{
				Entrypoint: true, BehaviorRepair: true, DurableBudgetExhaustion: true,
				ExecutionLimits: &runtimecontract.RuntimeExecutionLimits{
					ProtocolRef: runtimecontract.RuntimeExecutionLimitsV1, MaxProviderCalls: 64, MaxCapabilityOperations: 32, MaxDurableDispatchesPerCWI: 1,
				},
			})
		})
	}
}

func TestDriveRuntimeDurableDispatchBudgetAllowsAdditionAndRepairWithinLimit(t *testing.T) {
	runValidationInputAdditionMainline(t, "uninterrupted", validationInputMainlineOptions{
		Entrypoint: true, BehaviorRepair: true,
		ExecutionLimits: &runtimecontract.RuntimeExecutionLimits{
			ProtocolRef: runtimecontract.RuntimeExecutionLimitsV1, MaxProviderCalls: 64, MaxCapabilityOperations: 32, MaxDurableDispatchesPerCWI: 2,
		},
	})
}

func TestDriveRuntimeInvalidSupplementClosesBeforeAnotherAcquisition(t *testing.T) {
	var baseline int64
	for _, cut := range []string{"uninterrupted", "after_input_receipt", "after_unavailability_terminal"} {
		t.Run(cut, func(t *testing.T) {
			calls := runValidationInputAdditionMainline(t, cut, validationInputMainlineOptions{InvalidSupplement: true})
			if cut == "uninterrupted" {
				baseline = calls
			} else if baseline != 0 && baseline != calls {
				t.Fatalf("unavailability restart consumed provider budget: %d != %d", calls, baseline)
			}
		})
	}
}

type validationInputMainlineOptions struct {
	ExecutionLimits         *runtimecontract.RuntimeExecutionLimits
	ProofBudgetExhaustion   bool
	DurableBudgetExhaustion bool
	InvalidSupplement       bool
	Entrypoint              bool
	CompileRepair           bool
	BehaviorRepair          bool
	InputProtection         bool
	RepeatInputBypass       bool
}

func TestDriveRuntimeOrdinaryMutationCannotWeakenValidationInputAcrossRestart(t *testing.T) {
	var baseline int64
	for _, cut := range []string{"uninterrupted", "after_ordinary_source_seal", "after_input_receipt"} {
		t.Run(cut, func(t *testing.T) {
			calls := runValidationInputAdditionMainline(t, cut, validationInputMainlineOptions{Entrypoint: true, InputProtection: true})
			if cut == "uninterrupted" {
				baseline = calls
			} else if calls != baseline {
				t.Fatalf("input exclusion restart consumed another provider call: %d != %d", calls, baseline)
			}
		})
	}
}

func TestDriveRuntimeRepeatedInputWeakeningClosesWithoutEffectOrTicket(t *testing.T) {
	var baseline int64
	for _, cut := range []string{"uninterrupted", "after_input_exclusion_terminal"} {
		t.Run(cut, func(t *testing.T) {
			calls := runValidationInputAdditionMainline(t, cut, validationInputMainlineOptions{Entrypoint: true, InputProtection: true, RepeatInputBypass: true})
			if cut == "uninterrupted" {
				baseline = calls
			} else if calls != baseline {
				t.Fatalf("terminal replay consumed provider budget: %d != %d", calls, baseline)
			}
		})
	}
}

func TestDriveRuntimeAssertionFeedbackRepairsBehaviorAcrossRestart(t *testing.T) {
	var baseline int64
	for _, cut := range []string{"uninterrupted", "before_behavior_failure", "after_behavior_failure", "after_fresh_validation"} {
		t.Run(cut, func(t *testing.T) {
			calls := runValidationInputAdditionMainline(t, cut, validationInputMainlineOptions{Entrypoint: true, BehaviorRepair: true})
			if cut == "uninterrupted" {
				baseline = calls
			} else if calls != baseline {
				t.Fatalf("failure feedback restart consumed another provider call: %d != %d", calls, baseline)
			}
		})
	}
}

func TestDriveRuntimeValidationInputCompileRepairPreservesCriteriaAcrossRestart(t *testing.T) {
	var baseline int64
	for _, cut := range []string{"uninterrupted", "before_input_repair_seal", "after_input_repair_seal", "after_input_repair_receipt", "after_fresh_validation"} {
		t.Run(cut, func(t *testing.T) {
			calls := runValidationInputAdditionMainline(t, cut, validationInputMainlineOptions{Entrypoint: true, CompileRepair: true})
			if cut == "uninterrupted" {
				baseline = calls
			} else if calls != baseline {
				t.Fatalf("input repair restart consumed provider budget: %d != %d", calls, baseline)
			}
		})
	}
}

func TestDriveRuntimeEntrypointValidationUsesSameOwnerTransactionsAcrossRestart(t *testing.T) {
	var baseline int64
	for _, cut := range []string{"uninterrupted", "before_repair_seal", "after_repair_seal", "after_input_receipt", "after_fresh_validation"} {
		t.Run(cut, func(t *testing.T) {
			calls := runValidationInputAdditionMainline(t, cut, validationInputMainlineOptions{Entrypoint: true})
			if cut == "uninterrupted" {
				baseline = calls
			} else if calls != baseline {
				t.Fatalf("entrypoint restart consumed extra provider budget: %d != %d", calls, baseline)
			}
		})
	}
}

func runValidationInputAdditionMainline(t *testing.T, cut string, options ...validationInputMainlineOptions) int64 {
	var mode validationInputMainlineOptions
	if len(options) != 0 {
		mode = options[0]
	}
	invalidSupplement := mode.InvalidSupplement
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	deliverable, internalDir := filepath.Join(root, "workspace"), filepath.Join(root, "internal")
	for _, directory := range []string{deliverable, internalDir} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	bootstrap := validationInputAdditionMainlineBootstrap()
	if mode.Entrypoint {
		bootstrap.Activation.EvidencePolicies[0].RecipeAdapterRef = shellcommon.ValidationRecipeAdapterProjectEntrypoint
	}
	if mode.BehaviorRepair {
		convergence := bootstrap.DefinitionFocuses[1].ValidationFailureRepairConvergence.Clone()
		convergence.MaxAttempts = 2 // one input acquisition, one implementation repair
		route := convergence.Routes[0].Clone()
		route.ID, route.RequireInputAddition = "test-failure-to-file-edit", false
		route.TriggerDiagnosticClass, route.TriggerEvidenceTargetType, route.TriggerOwnerDiagnosticCode = runtimecontract.ValidationDiagnosticTestFailed, runtimecontract.ValidationEvidenceTargetBehaviorEvidence, "test_failed"
		route.RepairOperationKinds, route.RepairTargetPolicy = []string{runtimecontract.StepKindFileEdit}, runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope
		route.RepairTargetRole, route.RepairOperationTargetSelectorRef = "deliverable_implementation", "workspace-relative-existing-file-target"
		convergence.Routes = append(convergence.Routes, route)
		bootstrap.DefinitionFocuses[1].ValidationFailureRepairConvergence = &convergence
		bootstrap.Activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements[1].ValidationFailureRepairConvergence = &convergence
		bootstrap = runtimeTestBootstrapWithBindings(bootstrap)
	}
	if mode.CompileRepair {
		bootstrap.Definition.CapabilityContract = append(bootstrap.Definition.CapabilityContract, runtimecontract.RuntimeBlockKindPatchApply)
		convergence := bootstrap.DefinitionFocuses[1].ValidationFailureRepairConvergence.Clone()
		convergence.MaxAttempts = 2
		route := convergence.Routes[0].Clone()
		route.ID, route.RequireInputAddition, route.RequireInputRepair = "input-symbol-case-to-attested-patch", false, true
		route.TriggerDiagnosticClass, route.TriggerEvidenceTargetType, route.TriggerOwnerDiagnosticCode = runtimecontract.ValidationDiagnosticBuildFailed, runtimecontract.ValidationEvidenceTargetValidationInput, "validation_input_symbol_case_mismatch"
		route.RepairOperationKinds, route.RepairTargetPolicy = []string{runtimecontract.RuntimeBlockKindPatchApply}, runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope
		route.RepairTargetRole, route.RepairOperationTargetSelectorRef = "validation_input_maintenance", "workspace-relative-existing-file-target"
		convergence.Routes = append(convergence.Routes, route)
		bootstrap.DefinitionFocuses[1].ValidationFailureRepairConvergence = &convergence
		bootstrap.Activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements[1].ValidationFailureRepairConvergence = &convergence
		for i := range bootstrap.Requirements {
			if bootstrap.Requirements[i].OwnerModuleID == runtimecontract.RuntimeModuleFile {
				bootstrap.Requirements[i].RequiredCapabilities = append(bootstrap.Requirements[i].RequiredCapabilities, runtimecontract.RuntimeBlockKindPatchApply)
			}
		}
		bootstrap = runtimeTestBootstrapWithBindings(bootstrap)
	}
	// Initial fixture only: Shell authors the original native check and File
	// writes it before the Run. The later supplement must come from Runtime.
	if mode.ExecutionLimits != nil {
		bootstrap.Activation.ExecutionContract.ExecutionLimits = mode.ExecutionLimits
		var err error
		bootstrap.ExecutionBudgetPolicy, err = runtimecontract.RuntimeExecutionBudgetPolicyWithLimits(bootstrap.ConvergencePolicy, bootstrap.RecoveryPolicy, mode.ExecutionLimits)
		if err != nil {
			t.Fatal(err)
		}
		bootstrap = runtimeTestBootstrapWithBindings(bootstrap)
	}
	runtimeTestPrepareNativeValidationThroughOwners(t, bootstrap, deliverable)
	if err := os.WriteFile(filepath.Join(deliverable, "subject.go"), []byte("package fixture\nfunc DeliveredValue() int { return 2 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if mode.Entrypoint {
		if err := os.MkdirAll(filepath.Join(deliverable, "cmd", "delivery"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(deliverable, "cmd", "delivery", "main.go"), []byte("package main\nimport (\"fmt\"; subject \"example.com/workorch-runtime-native-fixture\")\nfunc main(){ fmt.Print(subject.DeliveredValue()) }\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	weakInputs := make(map[string][]byte)
	entries, err := os.ReadDir(filepath.Join(deliverable, "workorch_validation"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		path := filepath.Join(deliverable, "workorch_validation", entry.Name())
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		weakInputs[path] = content
	}
	var protectedInput string
	var sourceTargets []string
	if mode.InputProtection {
		protectedInput = filepath.ToSlash(filepath.Join("workorch_validation", entries[0].Name()))
		if err := os.WriteFile(filepath.Join(deliverable, "support.go"), []byte("package fixture\n"), 0644); err != nil {
			t.Fatal(err)
		}
		sourceTargets = []string{"subject.go", "support.go", protectedInput}
		// Reproduce an ordinary implementation scope which also happens to
		// contain an existing native input, as a real workspace manifest does.
		bootstrap.Activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements[0].TargetRefs = sourceTargets
		bootstrap = runtimeTestBootstrapWithBindings(bootstrap)
	}
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	workspaces := filerepository.NewWorkspaceRepository(filepath.Join(root, "workspaces.json"))
	results := filerepository.NewResultRepository(filepath.Join(root, "results.json"))
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2", DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive}); err != nil {
		t.Fatal(err)
	}
	if err := workspaces.Create(ctx, runtimeWorkspaceForPersistedRunTest(t, ctx, runs, "ws-v2", "run-v2", deliverable, internalDir)); err != nil {
		t.Fatal(err)
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
	startRuntimeReceiptTestOwner(t, hub)
	fileOwner := filemodule.New()
	if err := fileOwner.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := fileOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fileOwner.Stop(ctx) })
	startRuntimeManagedValidationTestOwner(t, hub)
	startRuntimeProviderCeilingTestOwner(t, hub)
	resultOwner := resultmodule.New()
	if err := resultOwner.Configure(resultbiz.Dependencies{OwnerRepositories: testResultOwnerRepositories(results, filerepository.NewArtifactRepository(filepath.Join(root, "artifacts.json")), memory.NewResultWorkspaceDeletionRepository()), EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := resultOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resultOwner.Stop(ctx) })

	var providerCalls atomic.Int64
	var inputProviderCall int64
	var addition runtimecontract.RuntimeValidationInputAddition
	var historicalAssessment runtimecontract.RuntimeCandidateContractManifest
	var freshValidation runtime.RuntimeAggregate
	var inputBeforeRepair string
	var observedInputRepair *runtimecontract.RuntimeValidationInputRepair
	var behaviorRepairs int
	var inputBypassAttempts int
	var observedSuccessfulWorkHistory bool
	llm := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llm.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		providerCalls.Add(1)
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		if command.DocumentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan {
			// Reload via the Run owner, including after the tested crash cuts.
			// Session/submission CAS advances the aggregate revision but must not
			// invent CWI acceptance or history in the pre-session observation.
			reader := newRuntimeDriveTestModule(t, hub)
			persisted, loadErr := reader.loadRuntime(ctx, command.WorkspaceID, command.RunID)
			if loadErr != nil {
				t.Error(loadErr)
			} else {
				progress := command.SemanticContext.CoverageProgress
				replayed, progressErr := runtimecontract.BuildRuntimeSemanticCoverageProgress(command.SemanticContext, command.SemanticManifest, persisted.Facts, progress.SourceStateRevision)
				if progressErr != nil || !reflect.DeepEqual(replayed, progress) || progress.SourceStateRevision > persisted.StateRevision {
					t.Errorf("semantic progress diverged from reloaded owner facts: %+v err=%v", replayed, progressErr)
				}
				history, historyErr := runtime.SemanticExecutionHistory(persisted, command.SemanticContext, command.SemanticManifest)
				if historyErr != nil || !reflect.DeepEqual(history, command.SemanticContext.ExecutionHistory) {
					t.Errorf("semantic execution history diverged from reloaded receipts: err=%v", historyErr)
				}
				for _, observation := range history.Receipts {
					if observation.ExpectedEffectScope == "" {
						t.Error("real owner history lost sealed effect expectation")
					}
					if observation.ObservedState == runtimecontract.RuntimeOperationOutcomeRejected && observation.Diagnostic == nil {
						t.Error("real owner rejected receipt lost its diagnostic")
					}
					if observation.ObservedState == runtimecontract.RuntimeOperationOutcomeAccepted && observation.Changed && slices.Contains(observation.BoundCoverageWorkItemIDs, common.CoverageWorkItemID("cwi-1")) {
						observedSuccessfulWorkHistory = true
					}
				}
				budget := command.SemanticContext.ExecutionBudget
				persistedRecord := runtimeStateRecordFromAggregate(persisted)
				durableUsage, durableErr := runtimecontract.RuntimeDurableDispatchUsage(persistedRecord.ActiveDispatch, persistedRecord.CompletedDispatches, persistedRecord.CandidateAttempts)
				if durableErr != nil {
					t.Fatal(durableErr)
				}
				for _, observed := range budget.VisibleDurableDispatches {
					if observed.Used != durableUsage[observed.CoverageWorkItemID] {
						t.Fatalf("semantic durable budget differs from persisted seal lineage: %+v, usage=%v", observed, durableUsage)
					}
				}
				if err := budget.Validate(command.SemanticContext, command.SemanticManifest); err != nil {
					t.Fatalf("semantic request lost budget observation: %v", err)
				}
				view, budgetErr := runtimecontract.BuildRuntimeExecutionBudgetView(persisted.Facts.WorkspaceID, persisted.Facts.RunID, persisted.StateRevision, persisted.ExecutionBudget, "", "")
				if budgetErr != nil {
					t.Fatal(budgetErr)
				}
				for _, observed := range budget.PhysicalDimensions {
					for _, current := range view.Dimensions {
						if current.Dimension != observed.Dimension {
							continue
						}
						// The observation precedes this callback's one submission
						// CAS; all earlier calls and all effects remain accounted.
						increment := int64(0)
						if current.Dimension == runtimecontract.RuntimeBudgetDimensionProviderCall {
							increment = 1
						}
						if current.Confirmed != observed.Confirmed || current.Unknown != observed.Unknown || current.Reserved != observed.Reserved+increment || current.Remaining != observed.Remaining-increment {
							t.Fatalf("semantic budget differs from exact pre-submission owner accounting: observed=%+v current=%+v", observed, current)
						}
					}
				}
				usage := runtimecontract.RuntimeSemanticProofBatchUsage(persisted.SemanticPlanning)
				for _, item := range budget.VisibleProofBatches {
					if item.Used != usage[item.CoverageWorkItemID] {
						t.Fatal("semantic request reset original CWI proof usage")
					}
				}
			}
			if err := runtimecontract.ValidateRuntimeSemanticExecutionFeedback(command.SemanticContext, command.SemanticManifest); err != nil {
				t.Errorf("semantic repair request lost exact causal feedback: %v", err)
			}
			for _, feedback := range command.SemanticContext.ExecutionFeedback {
				if feedback.Diagnostic == nil || feedback.Diagnostic.OwnerModuleID != runtimecontract.RuntimeModuleShell || feedback.Diagnostic.ValidationDiagnostic == nil || feedback.Diagnostic.OriginalCode == "" {
					t.Errorf("real Shell rejection lost owner diagnostic before semantic planning: %+v", feedback.Diagnostic)
				}
				// A native check may pass while delivery-behavior evidence is
				// missing. Preserve the actual dispositions, not invented failures.
				if feedback.Validation == nil || feedback.Validation.RecipeID == "" ||
					len(feedback.Validation.ExecutedChecks)+len(feedback.Validation.PassedChecks)+len(feedback.Validation.FailedChecks)+len(feedback.Validation.SkippedChecks)+len(feedback.Validation.IncompleteChecks) == 0 {
					t.Errorf("real Shell rejection lost typed check observations before semantic planning: %+v", feedback.Validation)
				}
				// Reload through the normal typed owner boundary while the physical
				// request is in flight: feedback must not depend on CAS revision.
				reader := newRuntimeDriveTestModule(t, hub)
				persisted, err := reader.loadRuntime(ctx, command.SemanticContext.WorkspaceID, command.SemanticContext.RunID)
				if err != nil {
					t.Error(err)
					continue
				}
				replayed, err := runtime.SemanticExecutionFeedback(persisted, command.SemanticContext, command.SemanticManifest)
				if err != nil || !reflect.DeepEqual(replayed, command.SemanticContext.ExecutionFeedback) {
					t.Errorf("fresh owner load changed semantic execution feedback: %+v err=%v", replayed, err)
				}
			}
			for _, obligation := range command.SemanticManifest.ProofObligations {
				if obligation.ValidationFailureRepair != nil {
					addition = obligation.ValidationFailureRepair.InputAddition
					if addition == (runtimecontract.RuntimeValidationInputAddition{}) || historicalAssessment.Hash == "" {
						t.Error("repair must follow a typed Shell rejection and the source assessment")
					}
				}
			}
			response, err := validationInputAdditionMainlineSemanticResult(command, sourceTargets...)
			if err != nil {
				t.Errorf("semantic provider fixture: %v", err)
				result.Set(nil, cd.NewError(cd.Unexpected, err.Error()))
				return
			}
			result.Set(response, nil)
			return
		}
		focus := command.Decision.Focus
		coverageID := focus.CoverageWorkItemIDs[0]
		var candidate runtimecontract.RuntimeCandidate
		switch command.Decision.State {
		case runtimecontract.ConvergenceStateNeedStateChange:
			if mode.InputProtection && coverageID == "cwi-1" {
				sealed := command.CandidateManifest
				if len(sealed.MutationInputCatalogs) == 0 || len(sealed.Operations) != 1 || !reflect.DeepEqual(sealed.Operations[0].Target.ExcludedTargetRefs, []string{protectedInput}) {
					t.Error("first ordinary operation did not consume Shell input facts")
				}
				if inputBypassAttempts == 0 || mode.RepeatInputBypass {
					inputBypassAttempts++
					oldText, newText := "return 2", "return 4"
					if mode.RepeatInputBypass {
						oldText, newText = "t.Fatal(", "t.Skip("
					}
					payload, _ := json.Marshal(map[string]any{"operations": []any{map[string]any{
						"kind": runtimecontract.StepKindFileEdit, "targetRefs": []string{protectedInput},
						"parameters": []any{map[string]string{"name": "oldText", "string": oldText}, map[string]string{"name": "newText", "string": newText}},
					}}})
					result.Set(runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{
						Transaction: command.Transaction, DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
						ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument), DocumentOutcome: runtimeCompleteDocumentOutcome(command.ExpectedDocument, payload),
					}), nil)
					return
				}
				if command.CandidateAttempt != 1 || command.CandidateCorrectionObservation == nil {
					t.Error("excluded input did not follow the bounded candidate-correction transaction")
				}
			}
			operation := runtimecontract.RuntimeOperation{ID: "source-change", Kind: runtimecontract.StepKindFileEdit, TargetRefs: []string{"subject.go"}, CoverageWorkItemIDs: []common.CoverageWorkItemID{coverageID}}
			oldText, newText := "return 2", "return 4"
			if mode.BehaviorRepair {
				newText = "return 3" // real implementation defect, not a grader defect
			}
			operation.Parameters = []runtimecontract.OperationParameter{{Name: "oldText", String: &oldText}, {Name: "newText", String: &newText}}
			if coverageID == "cwi-2" && mode.BehaviorRepair && command.CandidateManifest.Operations[0].Kind == runtimecontract.StepKindFileEdit {
				behaviorRepairs++
				operation.ID = "repair-behavior"
				oldText, newText = "return 3", "return 4"
				seen := false
				for _, fact := range command.CandidateContext.Evidence.Rejected {
					if fact.Validation == nil || len(fact.Validation.CheckFailures) == 0 {
						continue
					}
					detail := fact.Validation.CheckFailures[0]
					seen = fact.Validation.FailureDetailProtocolRef == runtimecontract.RuntimeValidationFailureDetailProtocolV2 &&
						detail.CheckRef == addition.NativeCheckRef && detail.TargetRef == addition.TargetRef && detail.ComparisonHash != "" &&
						detail.ReportedAssertion.Code == "value_mismatch" && len(fact.CoverageWorkItemIDs) == 0
				}
				if !seen {
					t.Error("real failed assertion did not reach the candidate as uncredited owner facts")
				}
			} else if coverageID == "cwi-2" && mode.CompileRepair && command.CandidateManifest.Operations[0].InputRepair != nil {
				observedInputRepair = command.CandidateManifest.Operations[0].InputRepair.Clone()
				if observedInputRepair.TargetRef != addition.TargetRef || len(observedInputRepair.Corrections) != 1 {
					t.Error("input repair not tied to original supplemental input")
				}
				oldText, newText, variant := "!=workOrchExpected", "!=workorchExpected", "text_replace"
				operation.ID, operation.Kind, operation.TargetRefs = "repair-input-compile", runtimecontract.RuntimeBlockKindPatchApply, []string{observedInputRepair.TargetRef}
				operation.Parameters = []runtimecontract.OperationParameter{{Name: "patchType", String: &variant}, {Name: "oldText", String: &oldText}, {Name: "newText", String: &newText}}
			} else if coverageID == "cwi-2" {
				content := fmt.Sprintf("package workorch_validation\nimport (\"testing\"; subject \"example.com/workorch-runtime-native-fixture\")\nfunc %s(t *testing.T) { const expected = 4; type response struct { value int }; read := func() response { return response{subject.DeliveredValue()} }; if read().value != expected { t.Fatal(\"wrong delivered behavior\") } }\n", addition.NativeCheckRef)
				if mode.Entrypoint {
					content = fmt.Sprintf(`package workorch_validation
import ("testing"; "os/exec"; "path/filepath"; "strings")
const expected = "4"
func assertDelivery(t *testing.T) {
 t.Helper(); bin:=filepath.Join(t.TempDir(),"delivery")
 build:=exec.Command("go","build","-buildvcs=false","-o",bin,"./cmd/delivery"); build.Dir=".."
 if output,err:=build.CombinedOutput(); err!=nil { t.Fatalf("build: %%v %%s",err,output) }
 output,err:=exec.Command(bin).Output(); if err!=nil || strings.TrimSpace(string(output))!=expected { t.Fatal("wrong delivered behavior") }
}
func %s(t *testing.T) { assertDelivery(t) }
`, addition.NativeCheckRef)
				}
				if mode.BehaviorRepair {
					content = strings.ReplaceAll(content, "wrong delivered behavior", "WORKORCH_ASSERTION_V1 code=value_mismatch expected=equal observed=different")
				}
				if addition.ContentContractRef == "" || addition.ContentRequirements == "" {
					t.Error("supplement generation lost the receiving owner's content contract")
				}
				if invalidSupplement {
					content += "const expected = 4\nvar fixtureValue = 4\ntype response struct { Value int }\nfunc helper() int { return expected }\n"
				}
				if mode.CompileRepair {
					content = strings.ReplaceAll(content, "const expected", "const workorchExpected")
					content = strings.ReplaceAll(content, "!=expected", "!=workOrchExpected")
					inputBeforeRepair = content
				}
				inputProviderCall = providerCalls.Load()
				operation.ID, operation.Kind, operation.TargetRefs = "create-input-addition", runtimecontract.StepKindFileWrite, []string{addition.TargetRef}
				operation.Parameters = []runtimecontract.OperationParameter{{Name: "content", String: &content}}
			}
			candidate = runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: operation}}}}
		case runtimecontract.ConvergenceStateNeedValidation:
			if addition.TargetRef != "" {
				current, loadErr := newRuntimeDriveTestModule(t, hub).loadRuntime(ctx, "ws-v2", "run-v2")
				if loadErr != nil {
					t.Errorf("reload fresh validation: %v", loadErr)
				}
				if !mode.BehaviorRepair || freshValidation.StateRevision == 0 {
					freshValidation = current
				}
			}
			candidate = runtimeManagedShellValidationCandidate("validate-behavior", coverageID, focus.TargetRefs[0], "behavior-validation")
		case runtimecontract.ConvergenceStateNeedAssessment:
			if coverageID == "cwi-1" {
				historicalAssessment = command.CandidateManifest.Clone()
			}
			candidate = runtimeSatisfiedAssessmentCandidate(command, "exact owner evidence satisfies this coverage item")
		default:
			result.Set(nil, cd.NewError(cd.Unexpected, fmt.Sprintf("unexpected mainline action %s", command.Decision.State)))
			return
		}
		result.Set(runtimeCandidateResultForTest(t, command, candidate), nil)
	})
	t.Cleanup(func() { llm.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })

	module := newRuntimeDriveTestModule(t, hub)
	command := orchestratorevents.DriveRuntimeCommand{WorkspaceID: "ws-v2", RunID: "run-v2"}
	if cut != "uninterrupted" {
		crash := &runtimeSaveCrashRepository{delegate: runtimeStateRepository{hub: hub}, after: cut != "before_repair_seal" && cut != "before_input_repair_seal" && cut != "before_behavior_failure", marker: cut, match: func(aggregate runtime.RuntimeAggregate) bool {
			if cut == "after_input_exclusion_terminal" {
				return aggregate.Decision.Terminal != nil && aggregate.Decision.Terminal.Code == "candidate_retry_exhausted"
			}
			if cut == "after_ordinary_source_seal" {
				return aggregate.ActiveDispatch != nil && aggregate.ActiveDispatch.Candidate.StateChange != nil && reflect.DeepEqual(aggregate.ActiveDispatch.Candidate.StateChange.Operations[0].Operation.TargetRefs, []string{"subject.go"})
			}
			if cut == "before_behavior_failure" || cut == "after_behavior_failure" {
				for _, fact := range aggregate.Facts.Evidence.Rejected {
					if fact.Validation != nil && len(fact.Validation.CheckFailures) != 0 {
						return true
					}
				}
				return false
			}
			if cut == "before_input_repair_seal" || cut == "after_input_repair_seal" {
				return aggregate.ActiveDispatch != nil && aggregate.ActiveDispatch.RepairCommitment != nil && aggregate.ActiveDispatch.RepairCommitment.InputRepair != nil
			}
			if cut == "after_input_repair_receipt" {
				return observedInputRepair != nil && aggregate.ActiveDispatch == nil && aggregate.Facts.RepairDirective != nil && aggregate.Facts.RepairDirective.RequiredState == runtimecontract.ConvergenceStateNeedValidation
			}
			if cut == "after_unavailability_terminal" {
				return aggregate.Decision.Terminal != nil && aggregate.Decision.Terminal.Code == "owner_fact_convergence_exhausted"
			}
			if cut == "after_input_receipt" {
				return addition.TargetRef != "" && aggregate.ActiveDispatch == nil && aggregate.Facts.Focus.ID == "semantic-pending/cwi-2" && aggregate.Facts.RepairDirective != nil && aggregate.Facts.RepairDirective.RequiredState == runtimecontract.ConvergenceStateNeedValidation
			}
			if cut == "after_fresh_validation" {
				return addition.TargetRef != "" && aggregate.ActiveDispatch == nil && aggregate.Decision.State == runtimecontract.ConvergenceStateNeedAssessment
			}
			if aggregate.SemanticPlanning.Manifest == nil || aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningSealed {
				return false
			}
			for _, obligation := range aggregate.SemanticPlanning.Manifest.ProofObligations {
				if obligation.ValidationFailureRepair != nil {
					return true
				}
			}
			return false
		}}
		if err := module.ConfigureRuntimeRepository(crash); err != nil {
			t.Fatal(err)
		}
		func() {
			defer func() {
				if recovered := recover(); recovered != cut {
					t.Fatalf("expected %s process-loss cut, got %v", cut, recovered)
				}
			}()
			// Model process loss inside the owned drive. Do not run the synchronous
			// ingress's graceful gate-exit handoff after the injected CAS crash.
			response, driveErr := module.driveRuntimeLocked(ctx, command)
			t.Fatalf("did not reach repair seal cut: response=%+v err=%v", response, driveErr)
		}()
		cutAggregate, err := newRuntimeDriveTestModule(t, hub).loadRuntime(ctx, "ws-v2", "run-v2")
		if err != nil || cutAggregate.Validate() != nil {
			t.Fatalf("repair seal cut is not restart-valid: load=%v validate=%v", err, cutAggregate.Validate())
		}
		if cut == "after_ordinary_source_seal" {
			if cutAggregate.ActiveDispatch == nil || len(cutAggregate.ActiveDispatch.OperationTargetAuthorizations) == 0 || len(cutAggregate.ActiveDispatch.OperationTargetAuthorizations[0].Target.ExcludedTargetRefs) == 0 {
				t.Fatal("sealed dispatch lost input exclusions across process loss")
			}
		} else if cut == "after_input_exclusion_terminal" {
			if len(cutAggregate.OperationReceipts) != 0 || cutAggregate.ActiveDispatch != nil {
				t.Fatal("rejected ordinary input mutation produced an operation")
			}
		} else if _, err := os.Stat(filepath.Join(deliverable, addition.TargetRef)); cut == "before_repair_seal" || cut == "after_repair_seal" {
			if !os.IsNotExist(err) {
				t.Fatalf("repair seal performed an effect before dispatch: %v", err)
			}
		} else if err != nil {
			t.Fatalf("committed repair lost its durable effect: %v", err)
		}
		if historicalAssessment.Hash != "" {
			if _, err := runtimecontract.ResolveRuntimeAssessmentPurposePreimage(cutAggregate.SemanticPlanning, historicalAssessment.Bindings); err != nil {
				t.Fatalf("repair seal invalidated historical assessment: %v", err)
			}
		}
		module = newRuntimeDriveTestModule(t, hub)
	}
	completed, err := module.driveRuntime(ctx, command)
	if mode.ProofBudgetExhaustion || mode.DurableBudgetExhaustion {
		var exhausted *runtimecontract.RuntimeBudgetExhaustedError
		var durableExhausted *runtimecontract.RuntimeDurableDispatchBudgetExhaustedError
		if mode.DurableBudgetExhaustion {
			if !errors.As(err, &durableExhausted) || durableExhausted.CoverageWorkItemID != "cwi-2" {
				t.Fatalf("expected same-CWI durable seal exhaustion: %v", err)
			}
		} else if !errors.As(err, &exhausted) || exhausted.Dimension != runtimecontract.RuntimeBudgetDimensionSemanticGeneration {
			t.Fatalf("expected proof budget exhaustion before provider: %v", err)
		}
		before, loadErr := module.loadRuntime(ctx, "ws-v2", "run-v2")
		if loadErr != nil {
			t.Fatal(loadErr)
		}
		if mode.ProofBudgetExhaustion && (before.SemanticPlanning.ActiveBatchID != "" || len(before.SemanticPlanning.Batches) != 0 || runtimecontract.RuntimeSemanticProofBatchUsage(before.SemanticPlanning)["cwi-2"] != 1) {
			t.Fatal("exhaustion created another proof batch or reset original CWI usage")
		}
		if mode.DurableBudgetExhaustion {
			if before.ActiveDispatch != nil {
				t.Fatal("exhausted candidate created a dispatch")
			}
			usage, usageErr := runtimecontract.RuntimeDurableDispatchUsage(nil, runtimeStateRecordFromAggregate(before).CompletedDispatches, before.CandidateAttempts)
			if usageErr != nil || usage["cwi-2"] != 1 {
				t.Fatalf("addition and repair did not share original CWI budget: %v %v", usage, usageErr)
			}
		}
		calls := providerCalls.Load()
		// Exercise the production SM-4 error boundary and SM-2 terminal reducer,
		// followed by the ordinary Result -> Workspace -> Run owner handoff.
		retry, handled := module.handleScheduledRuntimeDriveFailure(ctx, scheduledRuntimeDriveRequest{workspaceID: "ws-v2", runID: "run-v2", source: "proof-budget-test"}, err)
		if !handled || retry != nil {
			t.Fatal("proof exhaustion left a retry or unhandled failure")
		}
		after, loadErr := module.loadRuntime(ctx, "ws-v2", "run-v2")
		if loadErr != nil {
			t.Fatal(loadErr)
		}
		if calls != providerCalls.Load() || !reflect.DeepEqual(before.CandidateRequestSessions, after.CandidateRequestSessions) || !reflect.DeepEqual(before.OperationReceipts, after.OperationReceipts) || !reflect.DeepEqual(before.ExecutionBudget, after.ExecutionBudget) {
			t.Fatal("budget terminal created another session, allocation, provider call or effect")
		}
		completed, err = runtimeTerminalDriveResult(after), nil
	}
	wantState, wantRun, wantResult, wantSatisfied := runtimecontract.ConvergenceStateComplete, common.RunStateCompleted, common.ResultStateAccepted, 2
	if mode.ProofBudgetExhaustion || mode.DurableBudgetExhaustion {
		wantState, wantRun, wantResult, wantSatisfied = runtimecontract.ConvergenceStateFailed, common.RunStateFailed, common.ResultStateRejected, 1
	}
	if invalidSupplement {
		wantState, wantRun, wantResult, wantSatisfied = runtimecontract.ConvergenceStateFailed, common.RunStateFailed, common.ResultStateRejected, 1
	}
	if mode.RepeatInputBypass {
		wantState, wantRun, wantResult, wantSatisfied = runtimecontract.ConvergenceStateFailed, common.RunStateFailed, common.ResultStateRejected, 0
	}
	if err != nil || completed.State != wantState {
		persisted, _ := module.loadRuntime(ctx, "ws-v2", "run-v2")
		t.Fatalf("input addition did not complete: result=%+v err=%v failure=%+v semantic=%s focusTargets=%v repair=%+v", completed, err, persisted.Facts.Failure, persisted.SemanticPlanning.Phase, persisted.Facts.Focus.TargetRefs, persisted.Facts.Repair)
	}
	if err := module.driveAndProjectRuntimeLockedWithCommand(ctx, command); err != nil {
		t.Fatalf("deliver completed Runtime through owner commands: %v", err)
	}
	// A fresh repository instance must read the same valid aggregate from disk.
	persisted, err := filerepository.NewRunRepository(filepath.Join(root, "runs.json")).GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.RuntimeState == nil {
		t.Fatalf("reload completed Run: %v", err)
	}
	aggregate := runtimeAggregateFromRecord(*persisted.RuntimeState)
	completed = runtimeTerminalDriveResult(aggregate)
	if err := aggregate.Validate(); err != nil {
		t.Fatalf("completed aggregate invariant: %v", err)
	}
	if persisted.State != wantRun || !runtimeTerminalDeliveryFullyAcknowledged(aggregate) || !reflect.DeepEqual(persisted.RuntimeBootstrap, initialRun.RuntimeBootstrap) {
		t.Fatalf("terminal delivery incomplete or immutable startup changed: run=%s acknowledged=%t bootstrapUnchanged=%t", persisted.State, runtimeTerminalDeliveryFullyAcknowledged(aggregate), reflect.DeepEqual(persisted.RuntimeBootstrap, initialRun.RuntimeBootstrap))
	}
	if mode.ExecutionLimits != nil {
		if !runtimecontract.SameRuntimeExecutionLimits(aggregate.Facts.Activation.ExecutionContract.ExecutionLimits, mode.ExecutionLimits) {
			t.Fatal("Run lost its sealed execution limits")
		}
		if !observedSuccessfulWorkHistory {
			t.Fatal("next semantic request never observed completed source work")
		}
		if err := runtimecontract.ValidateRuntimeExecutionLimitsBinding(aggregate.ExecutionBudget.Policy, mode.ExecutionLimits); err != nil {
			t.Fatal(err)
		}
		view, err := runtimecontract.BuildRuntimeExecutionBudgetView(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.StateRevision, aggregate.ExecutionBudget, "", "")
		if err != nil {
			t.Fatal(err)
		}
		for _, dimension := range view.Dimensions {
			if dimension.Dimension == runtimecontract.RuntimeBudgetDimensionProviderCall && dimension.Confirmed+dimension.Unknown != providerCalls.Load() {
				t.Fatalf("real provider invocations differ from shared budget: %+v calls=%d", dimension, providerCalls.Load())
			}
			if dimension.Dimension == runtimecontract.RuntimeBudgetDimensionCapabilityOperation && dimension.Confirmed != int64(len(aggregate.OperationReceipts)) {
				t.Fatalf("real owner receipts differ from operation budget: %+v receipts=%d", dimension, len(aggregate.OperationReceipts))
			}
		}
	}
	items, err := results.ListByRun(ctx, "run-v2")
	if err != nil || len(items) != 1 || items[0].State != wantResult || items[0].RuntimeCloseout == nil || len(items[0].RuntimeCloseout.SatisfiedCoverageWorkItemIDs) != wantSatisfied {
		t.Fatalf("Result owner did not persist the judged two-CWI closeout: %+v %v", items, err)
	}
	workspace, err := workspaces.Get(ctx, "ws-v2")
	if err != nil || workspace.ActiveRunID != "" {
		t.Fatalf("Workspace owner did not settle its active Run: %+v %v", workspace, err)
	}
	if mode.RepeatInputBypass {
		if inputBypassAttempts != 2 || aggregate.Decision.Terminal == nil || aggregate.Decision.Terminal.Code != "candidate_retry_exhausted" || len(aggregate.OperationReceipts) != 0 || completed.BlockingTicketID != "" || completed.BlockingApprovalID != "" {
			t.Fatalf("input bypass did not close through bounded SM-3 rejection: attempts=%d result=%+v", inputBypassAttempts, completed)
		}
		for _, item := range aggregate.Facts.Coverage {
			if item.State == runtimecontract.RuntimeCoverageItemStateSatisfied {
				t.Fatal("rejected ordinary input mutation granted coverage credit")
			}
		}
		for path, before := range weakInputs {
			after, err := os.ReadFile(path)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("rejected input weakening changed historical bytes")
			}
		}
		return providerCalls.Load()
	}
	if mode.ProofBudgetExhaustion || mode.DurableBudgetExhaustion {
		if aggregate.Decision.Terminal == nil || aggregate.Decision.Terminal.Code != "execution_budget_exhausted" || completed.BlockingTicketID != "" || completed.BlockingApprovalID != "" {
			t.Fatalf("proof exhaustion was not a bounded budget terminal: %+v", completed)
		}
		for path, before := range weakInputs {
			after, err := os.ReadFile(path)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("proof exhaustion changed original validation input")
			}
		}
		fresh := newRuntimeDriveTestModule(t, hub)
		calls, revision := providerCalls.Load(), aggregate.StateRevision
		if err := fresh.driveAndProjectRuntimeLockedWithCommand(ctx, command); err != nil {
			t.Fatal(err)
		}
		replayed, err := fresh.loadRuntime(ctx, "ws-v2", "run-v2")
		if err != nil || calls != providerCalls.Load() || revision != replayed.StateRevision {
			t.Fatal("terminal reload/replay created new work")
		}
		return calls
	}
	for _, item := range aggregate.Facts.Coverage {
		if invalidSupplement && item.ID == "cwi-2" {
			if item.State == runtimecontract.RuntimeCoverageItemStateSatisfied {
				t.Fatal("invalid supplemental input gained coverage credit")
			}
			continue
		}
		if item.State != runtimecontract.RuntimeCoverageItemStateSatisfied {
			t.Fatalf("CWI not settled by exact evidence: %+v", item)
		}
	}
	acceptedValidation, rejectedValidation, createdInputs := 0, 0, 0
	for _, receipt := range aggregate.OperationReceipts {
		if receipt.Evidence.Validation != nil {
			if mode.Entrypoint && receipt.Evidence.Validation.ObservationProtocolRef != "shell/go-test-json/workspace-behavior/v4" {
				t.Fatal("entrypoint receipt lost its sealed owner protocol")
			}
			if receipt.Outcome == runtimecontract.RuntimeOperationOutcomeAccepted {
				acceptedValidation++
			} else {
				rejectedValidation++
			}
		}
		if reflect.DeepEqual(receipt.TargetRefs, []string{addition.TargetRef}) && receipt.Changed {
			createdInputs++
		}
	}
	wantAccepted := 1
	if invalidSupplement {
		wantAccepted = 0
	}
	wantRejected, wantChanges := 1, 1
	if mode.CompileRepair {
		wantRejected, wantChanges = 2, 2
	}
	if mode.BehaviorRepair {
		wantRejected = 2
		if behaviorRepairs != 1 {
			t.Fatal("feedback did not produce exactly one implementation repair")
		}
	}
	if acceptedValidation != wantAccepted || rejectedValidation != wantRejected || createdInputs != wantChanges || addition.TargetRef == "" {
		t.Fatalf("unexpected effect/validation counts: accepted=%d rejected=%d input=%d addition=%+v", acceptedValidation, rejectedValidation, createdInputs, addition)
	}
	if mode.InputProtection {
		if inputBypassAttempts != 1 {
			t.Fatal("ordinary input mutation did not consume exactly one rejected candidate")
		}
		for _, receipt := range aggregate.OperationReceipts {
			for _, target := range receipt.TargetRefs {
				if target == protectedInput && receipt.Changed {
					t.Fatal("ordinary candidate acquired input mutation credit")
				}
			}
		}
	}
	if mode.CompileRepair {
		content, err := os.ReadFile(filepath.Join(deliverable, addition.TargetRef))
		if err != nil || string(content) != strings.ReplaceAll(inputBeforeRepair, "!=workOrchExpected", "!=workorchExpected") {
			t.Fatal("input repair changed acceptance criteria or unrelated bytes")
		}
		if observedInputRepair == nil {
			t.Fatal("input compilation error bypassed independently sealed repair")
		}
		for _, dispatch := range aggregate.CompletedDispatches {
			if dispatch.RepairCommitment == nil || dispatch.RepairCommitment.InputRepair == nil {
				continue
			}
			if !observedInputRepair.MatchesReplayProof(dispatch.OperationExecutions[0].ReplayProof) {
				t.Fatal("lost exact content repair proof")
			}
		}
	}
	for path, before := range weakInputs {
		after, err := os.ReadFile(path)
		if err != nil || !reflect.DeepEqual(before, after) {
			t.Fatalf("old native input was rewritten: %s err=%v", path, err)
		}
	}
	preimage, err := runtimecontract.ResolveRuntimeAssessmentPurposePreimage(aggregate.SemanticPlanning, historicalAssessment.Bindings)
	if err != nil || preimage.SemanticManifestHash != historicalAssessment.AssessmentPurposeManifestHash {
		t.Fatalf("historical assessment lost exact immutable preimage: %+v %v", preimage, err)
	}
	if invalidSupplement {
		terminal := aggregate.Decision.Terminal
		if terminal == nil || terminal.Code != "owner_fact_convergence_exhausted" || terminal.OriginalDiagnostic == nil ||
			terminal.OriginalDiagnostic.OriginalCode != "native_check_supplement_contract_invalid" ||
			!reflect.DeepEqual(terminal.OriginalDiagnostic.TargetRefs, []string{addition.TargetRef}) ||
			!reflect.DeepEqual(items[0].RuntimeCloseout.OriginalDiagnostic, terminal.OriginalDiagnostic) {
			t.Fatalf("bounded terminal/Result lost exact Shell cause: terminal=%+v result=%+v", terminal, items[0].RuntimeCloseout)
		}
		if providerCalls.Load() != inputProviderCall || freshValidation.StateRevision != 0 {
			t.Fatalf("invalid supplement opened another provider request: %d != %d", providerCalls.Load(), inputProviderCall)
		}
		for _, scenario := range []string{"missing cause", "changed cause", "stale catalog", "missing owner fact", "duplicate owner fact"} {
			t.Run(scenario, func(t *testing.T) {
				changed := runtimeAggregateFromRecord(runtimeStateRecordFromAggregate(aggregate))
				switch scenario {
				case "missing cause":
					changed.Decision.Terminal.OriginalDiagnostic = nil
				case "changed cause":
					changed.Decision.Terminal.OriginalDiagnostic.OriginalCode = "other"
				default:
					for ci := range changed.Facts.Resources.OperationAdmissibilities {
						catalog := &changed.Facts.Resources.OperationAdmissibilities[ci]
						for si := range catalog.ValidationSurfaces {
							surface := &catalog.ValidationSurfaces[si]
							if len(surface.ProofRequirementUnavailabilities) == 0 {
								continue
							}
							switch scenario {
							case "stale catalog":
								surface.ProofRequirementUnavailabilities[0].OriginalDiagnostic.ObservedRevision = "stale"
							case "missing owner fact":
								surface.ProofRequirementUnavailabilities = nil
							case "duplicate owner fact":
								surface.ProofRequirementUnavailabilities = append(surface.ProofRequirementUnavailabilities, surface.ProofRequirementUnavailabilities[0])
							}
						}
					}
				}
				if changed.Validate() == nil {
					t.Fatal("fresh load accepted invalid terminal owner cause")
				}
			})
		}
		for range 2 {
			replayed, replayErr := newRuntimeDriveTestModule(t, hub).driveRuntime(ctx, command)
			if replayErr != nil || replayed.StateRevision != completed.StateRevision || providerCalls.Load() != inputProviderCall {
				t.Fatalf("terminal restart repeated transition/provider: %+v %v", replayed, replayErr)
			}
		}
		return providerCalls.Load()
	}
	if freshValidation.StateRevision == 0 || freshValidation.Facts.RepairDirective == nil || len(freshValidation.Facts.RepairDirective.TargetRefs) != 0 {
		t.Fatal("fresh validation lacks a target-free, restart-valid repair continuation")
	}
	for _, item := range freshValidation.Facts.Coverage {
		if item.ID == "cwi-2" && item.State == runtimecontract.RuntimeCoverageItemStateSatisfied {
			t.Fatal("input creation granted terminal coverage before fresh validation")
		}
	}
	nativeBindings := 0
	wantProtocol := "shell/go-test-json/workspace-behavior/v3"
	if mode.Entrypoint {
		wantProtocol = "shell/go-test-json/workspace-behavior/v4"
	}
	for _, binding := range freshValidation.Facts.Focus.CoverageEvidenceBindings {
		for _, proof := range binding.Proofs {
			for _, check := range proof.ValidationCheckAttestations {
				if check.RequiredCheckRef == addition.RequiredCheckRef && check.NativeCheckRef == addition.OriginalNativeCheckRef && check.SupplementalNativeCheckRef == addition.NativeCheckRef && check.ObservationProtocolRef == wantProtocol {
					nativeBindings++
				}
			}
		}
	}
	if nativeBindings != 1 {
		t.Fatalf("fresh Shell binding did not retain original + supplemental all-of checks: %d", nativeBindings)
	}
	if cut == "uninterrupted" {
		assertSemanticRepairContinuationTamperRejected(t, freshValidation)
		if mode.CompileRepair {
			assertInputRepairArchiveTamperRejected(t, aggregate)
		}
	}
	callsBefore := providerCalls.Load()
	if mode.BehaviorRepair && cut == "uninterrupted" {
		// Read-only owner re-observation of the real two-CWI mainline after
		// source repair + fresh v4 validation + file-backed restart. This does
		// not drive a terminal Run or manufacture another acceptance credit.
		request := runtimeReconciliationRequestForRun(persisted, aggregate, "verify-completed-proof-context", 0)
		batch := module.queryRuntimeReconciliationBatch(ctx, request, aggregate, deliverable, internalDir, nil)
		if err := batch.ValidateForRequest(request); err != nil {
			t.Fatalf("reloaded two-CWI reconciliation batch: %v", err)
		}
		observed := false
		for _, observation := range batch.Observations {
			if observation.CoverageWorkItemID == "cwi-2" && observation.OwnerModuleID == runtimecontract.RuntimeModuleShell {
				observed = true
				if observation.Status != runtimecontract.RuntimeCWIReconciliationObservationSatisfied {
					t.Fatalf("fresh accepted v4 proof became unverifiable after restart: %+v", observation)
				}
			}
		}
		if !observed || providerCalls.Load() != callsBefore {
			t.Fatal("reconciliation omitted the validation owner or invoked the provider")
		}
		after, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
		if err != nil || !reflect.DeepEqual(after.RuntimeState, persisted.RuntimeState) {
			t.Fatal("owner observation rewrote the terminal aggregate")
		}
	}
	for range 2 {
		replayed, err := newRuntimeDriveTestModule(t, hub).driveRuntime(ctx, command)
		if err != nil || replayed.StateRevision != completed.StateRevision || providerCalls.Load() != callsBefore {
			t.Fatalf("terminal restart replay changed state or provider budget: %+v %v calls=%d", replayed, err, providerCalls.Load())
		}
	}
	return callsBefore
}

func assertInputRepairArchiveTamperRejected(t *testing.T, completed runtime.RuntimeAggregate) {
	t.Helper()
	for _, scenario := range []string{"missing_commitment", "changed_commitment", "missing_replay_proof", "changed_replay_postimage", "missing_manifest_input", "changed_manifest_input", "changed_source_diagnostic"} {
		t.Run(scenario, func(t *testing.T) {
			changed := runtimeAggregateFromRecord(runtimeStateRecordFromAggregate(completed))
			found := false
			for index := range changed.CompletedDispatches {
				dispatch := &changed.CompletedDispatches[index]
				if dispatch.RepairCommitment == nil || dispatch.RepairCommitment.InputRepair == nil {
					continue
				}
				found = true
				switch scenario {
				case "missing_commitment":
					dispatch.RepairCommitment.InputRepair = nil
				case "changed_commitment":
					dispatch.RepairCommitment.InputRepair.AfterHash = strings.Repeat("c", 64)
				case "missing_replay_proof":
					dispatch.OperationExecutions[0].ReplayProof = nil
				case "changed_replay_postimage":
					dispatch.OperationExecutions[0].ReplayProof.AfterHash = strings.Repeat("c", 64)
				case "missing_manifest_input", "changed_manifest_input":
					for i := range changed.CandidateRequestSessions {
						manifest := changed.CandidateRequestSessions[i].CandidateManifest
						if manifest == nil || manifest.Hash != dispatch.CandidateManifestHash {
							continue
						}
						if scenario == "missing_manifest_input" {
							manifest.Operations[0].InputRepair = nil
						} else {
							manifest.Operations[0].InputRepair.Corrections[0].After = "other"
						}
					}
				case "changed_source_diagnostic":
					for i := range changed.OperationReceipts {
						receipt := &changed.OperationReceipts[i]
						if receipt.ExecutionKey == dispatch.RepairCommitment.SourceReceiptExecutionKey {
							receipt.Feedback.OriginalDiagnostic.ValidationDiagnostic.InputRepair.BeforeHash = strings.Repeat("c", 64)
						}
					}
				}
				break
			}
			if !found {
				t.Fatal("no input repair dispatch")
			}
			if err := changed.Validate(); err == nil {
				t.Fatal("tampered input repair archive accepted on fresh load")
			}
		})
	}
}

func assertSemanticRepairContinuationTamperRejected(t *testing.T, fresh runtime.RuntimeAggregate) {
	t.Helper()
	for _, scenario := range []string{"missing_directive", "restored_targets", "changed_barrier", "missing_receipt", "duplicate_receipt", "stale_receipt", "missing_evidence", "missing_dispatch", "missing_semantic_preimage"} {
		t.Run(scenario, func(t *testing.T) {
			changed := runtimeAggregateFromRecord(runtimeStateRecordFromAggregate(fresh))
			barrier := changed.Facts.RepairDirective.SupersedesEvidenceRevision
			var evidence runtimecontract.EvidenceFact
			for _, fact := range changed.Facts.Evidence.Accepted {
				if fact.Revision == barrier {
					evidence = fact
				}
			}
			switch scenario {
			case "missing_directive":
				changed.Facts.RepairDirective = nil
			case "restored_targets":
				changed.Facts.RepairDirective.TargetRefs = append([]string(nil), changed.Facts.RepairDirective.Closure.GroundedTargetRefs...)
			case "changed_barrier":
				changed.Facts.RepairDirective.SupersedesEvidenceRevision = changed.Facts.RepairDirective.Closure.BaselineEvidenceRevision
			case "missing_evidence":
				for index, fact := range changed.Facts.Evidence.Accepted {
					if fact.ID == evidence.ID {
						changed.Facts.Evidence.Accepted = append(changed.Facts.Evidence.Accepted[:index], changed.Facts.Evidence.Accepted[index+1:]...)
						break
					}
				}
			case "missing_dispatch":
				for index, dispatch := range changed.CompletedDispatches {
					if dispatch.ID == evidence.DispatchID {
						changed.CompletedDispatches = append(changed.CompletedDispatches[:index], changed.CompletedDispatches[index+1:]...)
						break
					}
				}
			case "missing_semantic_preimage":
				for index, generation := range changed.SemanticPlanning.History {
					if generation.Snapshot.Manifest != nil && generation.Snapshot.Manifest.ProofObligations[0].ValidationFailureRepair != nil {
						changed.SemanticPlanning.History = append(changed.SemanticPlanning.History[:index], changed.SemanticPlanning.History[index+1:]...)
						break
					}
				}
			default:
				for index, receipt := range changed.OperationReceipts {
					if receipt.OperationID != evidence.OperationID {
						continue
					}
					switch scenario {
					case "missing_receipt":
						changed.OperationReceipts = append(changed.OperationReceipts[:index], changed.OperationReceipts[index+1:]...)
					case "duplicate_receipt":
						changed.OperationReceipts = append(changed.OperationReceipts, receipt)
					case "stale_receipt":
						changed.OperationReceipts[index].Fence++
					}
					break
				}
			}
			if err := changed.Validate(); err == nil {
				t.Fatal("invalid fresh-validation continuation accepted")
			}
		})
	}
}

func validationInputAdditionMainlineBootstrap() runtimecontract.RuntimeBootstrap {
	bootstrap := runtimeBootstrap()
	bootstrap.Definition.CapabilityContract = []string{runtimecontract.StepKindFileWrite, runtimecontract.StepKindFileEdit, runtimecontract.RuntimeBlockKindManagedValidationSession}
	bootstrap.Activation.EvidencePolicies = []runtimecontract.RuntimeEvidencePolicy{{ID: "behavior-validation", AppliesTo: runtimecontract.StepKindShellCommand, RecipeDeclared: true, RecipeAdapterRef: "shell/project-native-validation/v1", RecipeTimeoutMillis: 60_000}}
	convergence := runtimecontract.RuntimeValidationFailureRepairConvergence{ID: "behavior-input-repair", RequireFreshValidation: true, MaxAttempts: 1, Routes: []runtimecontract.RuntimeValidationFailureRepairRoute{{
		ID: "missing-behavior-to-new-validation-input", TriggerOwnerModuleID: runtimecontract.RuntimeModuleShell, TriggerOperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
		TriggerDiagnosticClass: runtimecontract.ValidationDiagnosticOutputInconclusive, TriggerEvidenceTargetType: runtimecontract.ValidationEvidenceTargetValidationOutput, TriggerOwnerDiagnosticCode: "validation_workspace_behavior_unobserved",
		RepairEvidenceKind: runtimecontract.EvidenceKindStateChange, RepairOwnerModuleID: runtimecontract.RuntimeModuleFile, RepairOperationKinds: []string{runtimecontract.StepKindFileWrite},
		RepairTargetPolicy: runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget, RepairTargetRole: "validation_input_supplement",
		RepairDeclarationTargetSelectorRef: "file/workspace_deliverable", RepairDeclarationTargetSelectorOwnerRef: "mainline-profile", RepairDeclarationTargetSelectorSubjectRef: "behavior-input",
		RepairOperationTargetSelectorRef: "workspace-relative-new-file-target", RequireInputAddition: true,
	}}}
	bootstrap.DefinitionFocuses = []runtimecontract.RuntimeDefinitionFocus{
		{ID: "focus-source", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, AcceptedStepKinds: []string{runtimecontract.StepKindFileEdit}, TargetRole: "deliverable", RequireTargetGrounding: true, RequireAssessment: true},
		{ID: "focus-behavior", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-2"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, TargetRole: "deliverable", RequireTargetGrounding: true, RequireExecutableBehaviorEvidence: true, RequireAssessment: true, ValidationFailureRepairConvergence: &convergence},
	}
	bootstrap.Activation.ExecutionContract.ProfileContracts = []runtimecontract.RuntimeProfileContract{{ID: "mainline-profile", EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{
		{ID: "cwi-1", Scope: "test", TargetRole: "deliverable", TargetRefs: []string{"subject.go"}, RequireTargetGrounding: true, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, AcceptedStepKinds: []string{runtimecontract.StepKindFileEdit}},
		{ID: "cwi-2", Scope: "dodItem", TargetRole: "deliverable", TargetRefs: []string{"subject.go"}, RequireTargetGrounding: true, RequireExecutableBehaviorEvidence: true, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, ValidationRecipeID: "behavior-validation", ValidationFailureRepairConvergence: &convergence},
	}}}
	bootstrap.Slices = []runtimecontract.RuntimeAcceptanceSlice{{ID: "slice-mainline", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1", "cwi-2"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}, CompletionPolicy: runtimecontract.RuntimeAcceptanceSliceCompletionAll, ResultPolicy: runtimecontract.RuntimeAcceptanceSliceResultAllowed}}
	bootstrap.CoverageItems = []runtimecontract.RuntimeCoverageItem{{ID: "cwi-1", SliceID: "slice-mainline", Order: 1}, {ID: "cwi-2", SliceID: "slice-mainline", SourceScope: "dodItem", Order: 2, PrerequisiteCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}}}
	bootstrap.Requirements = []runtimecontract.RuntimeRequirement{
		{Kind: runtimecontract.PrerequisiteKindProviderCeiling, OwnerModuleID: runtimecontract.RuntimeModuleLLM, MinimumBytes: 64 * 1024, Reason: "semantic planning provider ceiling"},
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleFile, CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{runtimecontract.StepKindFileWrite, runtimecontract.StepKindFileEdit}},
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleShell, CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}},
		{Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleFile, RequiredCapabilities: []string{runtimecontract.StepKindFileWrite, runtimecontract.StepKindFileEdit}, Reason: "File inventory"},
		{Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleShell, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, Reason: "Shell inventory"},
	}
	runtimeTestDeclareExactManagedValidationOwnerFact(&bootstrap, "cwi-2", "behavior-validation", false)
	return runtimeTestBootstrapWithBindings(bootstrap)
}

func validationInputAdditionMainlineSemanticResult(command llmevents.RequestRuntimeCandidateCommand, sourceTargets ...string) (llmevents.RequestRuntimeCandidateResult, error) {
	document := validationInputAdditionMainlineSemanticDocument(command, sourceTargets...)
	payload, err := runtimecontract.EncodeRuntimeSemanticPlanPayloadWithManifest(command.SemanticManifest, document)
	if err != nil {
		return llmevents.RequestRuntimeCandidateResult{}, err
	}
	return runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity, ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument), DocumentOutcome: runtimeCompleteDocumentOutcome(command.ExpectedDocument, payload)}), nil
}

func validationInputAdditionMainlineSemanticDocument(command llmevents.RequestRuntimeCandidateCommand, sourceTargets ...string) runtimecontract.RuntimeSemanticPlanDocument {
	document := runtimecontract.RuntimeSemanticPlanDocument{SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: command.SemanticManifest.Hash, DefinitionSeedHash: command.SemanticManifest.DefinitionSeedHash, PlanStep: command.SemanticContext.PlanStep}
	if document.PlanStep == runtimecontract.RuntimeSemanticPlanStepCoverageOrder {
		for _, item := range command.SemanticManifest.CoverageItems {
			document.CoverageOrder = append(document.CoverageOrder, item.ID)
		}
	} else {
		document.BatchID, document.BatchSequence = command.SemanticContext.BatchID, command.SemanticContext.BatchSequence
		document.BatchCoverageWorkItemIDs = command.SemanticContext.BatchCoverageWorkItemIDs
		for _, coverageID := range document.BatchCoverageWorkItemIDs {
			for _, obligation := range command.SemanticManifest.ProofObligations {
				if obligation.CoverageWorkItemID != coverageID {
					continue
				}
				proposal := runtimecontract.RuntimeSemanticProofProposal{CoverageWorkItemID: coverageID, OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKind: runtimecontract.StepKindFileEdit,
					Target:                runtimecontract.RuntimeOperationTargetAuthorization{Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"subject.go"}},
					RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}, DeclarationRef: runtimecontract.RuntimeSemanticCurrentDeclarationRef(obligation)}
				if coverageID == "cwi-1" && len(sourceTargets) != 0 {
					proposal.Target.ExactTargetRefs = append([]string(nil), sourceTargets...)
				}
				if repair := obligation.ValidationFailureRepair; repair != nil {
					proposal.OperationKind = runtimecontract.StepKindFileWrite
					proposal.Target = runtimecontract.RuntimeOperationTargetAuthorization{Policy: repair.RepairTargetPolicy, OwnerSelectorRefs: []string{repair.RepairOperationTargetSelectorRef}, ProspectiveConstraintRefs: repair.RepairConstraintRefs, ProspectiveTargetRefs: []string{repair.InputAddition.TargetRef}}
				} else if obligation.ValidationProofRequirement != nil {
					proposal.OwnerModuleID, proposal.OperationKind = runtimecontract.RuntimeModuleShell, runtimecontract.RuntimeBlockKindManagedValidationSession
					proposal.RequiredEvidenceKinds, proposal.RequiredEffectScopes = []string{runtimecontract.EvidenceKindValidation}, []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectEphemeralSession}
					proposal.ValidationRecipeID = "behavior-validation"
				}
				document.Proposals = append(document.Proposals, proposal)
			}
		}
	}
	return document
}
