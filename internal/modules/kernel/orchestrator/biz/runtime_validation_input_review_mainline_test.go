package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	decisionmodule "github.com/muidea/workorch/internal/modules/blocks/decision"
	decisionbiz "github.com/muidea/workorch/internal/modules/blocks/decision/biz"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	filemodule "github.com/muidea/workorch/internal/modules/blocks/file"
	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	resultmodule "github.com/muidea/workorch/internal/modules/blocks/result"
	resultbiz "github.com/muidea/workorch/internal/modules/blocks/result/biz"
	runmodule "github.com/muidea/workorch/internal/modules/blocks/run"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	workspacemodule "github.com/muidea/workorch/internal/modules/kernel/workspace"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	filerepository "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchdefaults"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Only provider responses and the user's factual ticket answer are scripted.
// Shell publishes real input bytes and executes the delivery process, Decision
// owns tickets, Run persists CAS, and the harness supplies no binding/credit.
func TestDriveRuntimeValidationInputReviewMainline(t *testing.T) {
	for _, mode := range []string{"consistent", "conflict", "clarify", "exhausted", "malformed", "reject"} {
		t.Run(mode, func(t *testing.T) { runValidationInputReviewMainline(t, mode, "") })
	}
}

func TestDriveRuntimeValidationInputReviewRestart(t *testing.T) {
	baseline := runValidationInputReviewMainline(t, "clarify", "")
	for _, cut := range []string{"after_review_document", "after_review_classification", "after_ticket_binding", "after_information_commit", "after_review_binding"} {
		t.Run(cut, func(t *testing.T) {
			if calls := runValidationInputReviewMainline(t, "clarify", cut); calls != baseline {
				t.Fatalf("restart consumed another provider call: %d != %d", calls, baseline)
			}
		})
	}
}

type validationInputReviewMainlineOptions struct {
	Adapter                            string
	ValidationCount                    int
	ProtocolRef                        string
	SourceObligations                  bool
	DeliveryGuard                      bool
	TerminalMaterialGuards             bool
	SnapshotInputSources               bool
	TerminalSnapshot                   bool
	AdditionalTerminalSnapshotConsumer bool
	SubmittedSettlementFault           string
}

func TestDriveRuntimeInputReviewV2OmissionAndRestart(t *testing.T) {
	options := validationInputReviewMainlineOptions{Adapter: shellcommon.ValidationRecipeAdapterIsolatedEntrypoint, ValidationCount: 1, ProtocolRef: runtimecontract.RuntimeValidationInputReviewProtocolV2}
	// The shipped-default mainline below covers v2 consistency, contradiction,
	// clarification and its recovery cuts. Exercise the additional omission
	// branch here without repeating those expensive real-owner scenarios.
	var baseline int64
	var baselineOnce sync.Once
	baselineCalls := func(t *testing.T) int64 {
		t.Helper()
		// A filtered restart subtest must still establish the uninterrupted
		// reference. Never skip the budget assertion when a sibling did not run.
		baselineOnce.Do(func() { baseline = runValidationInputReviewMainline(t, "omission", "", options) })
		return baseline
	}
	t.Run("omission", func(t *testing.T) { baselineCalls(t) })
	for _, cut := range []string{"after_review_document", "after_review_failure"} {
		t.Run(cut, func(t *testing.T) {
			expected := baselineCalls(t)
			if calls := runValidationInputReviewMainline(t, "omission", cut, options); calls != expected {
				t.Fatalf("restart repeated a completed review request: got=%d uninterrupted=%d", calls, expected)
			}
		})
	}
}

func TestDriveRuntimeIsolatedInputReviewMainline(t *testing.T) {
	// Selection comes from the shipped declaration, not a test-only opt-in.
	templates, err := workorchdefaults.BuiltinWorkspaceTemplateDefaults()
	if err != nil {
		t.Fatal(err)
	}
	adapter, protocol := "", ""
	for _, template := range templates {
		if template.ID == "software-dev" {
			for _, policy := range template.EvidencePolicies {
				if ref, ok := policy["recipeAdapterRef"].(string); ok {
					adapter = ref
				}
				if ref, ok := policy["recipeInputReviewProtocolRef"].(string); ok {
					protocol = ref
				}
			}
		}
	}
	if adapter != shellcommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic {
		t.Fatal("default template does not enable reviewed conjunctive inputs")
	}
	if protocol != runtimecontract.RuntimeValidationInputReviewProtocolV3 {
		t.Fatal("default template does not seal the obligation-review protocol")
	}
	options := validationInputReviewMainlineOptions{Adapter: adapter, ValidationCount: 1, ProtocolRef: protocol}
	for _, mode := range []string{"consistent", "conflict", "clarify", "exhausted", "malformed", "reject"} {
		t.Run(mode, func(t *testing.T) { runValidationInputReviewMainline(t, mode, "", options) })
	}
	t.Run("multiple_cwi", func(t *testing.T) {
		multiple := validationInputReviewMainlineOptions{Adapter: adapter, ValidationCount: 3, ProtocolRef: protocol}
		baseline := runValidationInputReviewMainline(t, "consistent", "", multiple)
		t.Run("receipt-reply-loss", func(t *testing.T) {
			if got := runValidationInputReviewMainline(t, "consistent", "after_project_receipt", multiple); got != baseline {
				t.Fatalf("multiple CWI recovery repeated provider calls: %d != %d", got, baseline)
			}
		})
	})
	baseline := runValidationInputReviewMainline(t, "clarify", "", options)
	for _, cut := range []string{"after_review_document", "after_review_classification", "after_ticket_binding", "after_information_commit", "after_review_binding"} {
		t.Run(cut, func(t *testing.T) {
			if got := runValidationInputReviewMainline(t, "clarify", cut, options); got != baseline {
				t.Fatalf("restart repeated provider call: %d != %d", got, baseline)
			}
		})
	}
}

func runValidationInputReviewMainline(t *testing.T, mode, cut string, requested ...validationInputReviewMainlineOptions) int64 {
	t.Helper()
	options := validationInputReviewMainlineOptions{Adapter: shellcommon.ValidationRecipeAdapterVersionedEntrypoint, ValidationCount: 1}
	if len(requested) != 0 {
		options = requested[0]
	}
	projectAdapter := options.Adapter == shellcommon.ValidationRecipeAdapterProjectConjunction || options.Adapter == shellcommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic
	projectBehaviorRepair := mode == "project_behavior_failure"
	if projectBehaviorRepair && options.Adapter != shellcommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic {
		t.Fatal("multi-source repair fixture requires explicitly selected v8")
	}
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	directory, internalDir := filepath.Join(root, "workspace"), filepath.Join(root, "internal")
	for _, path := range []string{directory, internalDir} {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
	}
	bootstrap := validationInputAdditionMainlineBootstrap()
	if strings.HasPrefix(mode, "input_revision") {
		configureRuntimeInputRevisionMainline(&bootstrap)
		if mode == "input_revision_exhausted" {
			bootstrap.DefinitionFocuses[1].ImplementationWorkPolicy.MaxRoundsPerCWI = 1
		}
	}
	bootstrap.Activation.EvidencePolicies[0].RecipeAdapterRef = options.Adapter
	bootstrap.Activation.EvidencePolicies[0].RecipeInputReviewProtocolRef = options.ProtocolRef
	if projectBehaviorRepair {
		convergence := bootstrap.DefinitionFocuses[1].ValidationFailureRepairConvergence.Clone()
		route := convergence.Routes[0].Clone()
		route.ID, route.RequireInputAddition = "multi-source-failure-to-confirmed-file-edit", false
		route.TriggerDiagnosticClass, route.TriggerEvidenceTargetType, route.TriggerOwnerDiagnosticCode = runtimecontract.ValidationDiagnosticTestFailed, runtimecontract.ValidationEvidenceTargetBehaviorEvidence, "test_failed"
		route.RepairOperationKinds, route.RepairTargetPolicy = []string{runtimecontract.StepKindFileEdit}, runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope
		route.RepairTargetRole, route.RepairOperationTargetSelectorRef = "deliverable_implementation", "workspace-relative-existing-file-target"
		convergence.Routes = []runtimecontract.RuntimeValidationFailureRepairRoute{route}
		bootstrap.DefinitionFocuses[1].ValidationFailureRepairConvergence = &convergence
		bootstrap.Activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements[1].ValidationFailureRepairConvergence = &convergence
	}
	if options.TerminalSnapshot {
		bootstrap = runtimeReviewedGitSnapshotBootstrap(bootstrap, options.AdditionalTerminalSnapshotConsumer)
	}
	if options.DeliveryGuard || options.TerminalMaterialGuards {
		rules := []runtimecontract.RuntimeLineageAssertionRule{{Ref: "original-delivery", Kind: runtimecontract.RuntimeLineageDeliveryAcknowledgements, ConsumptionPoint: runtimecontract.RuntimeProofPhasePostTerminalDelivery}}
		if options.TerminalMaterialGuards {
			rules = append(rules,
				runtimecontract.RuntimeLineageAssertionRule{Ref: "formal-result-index", Kind: runtimecontract.RuntimeLineageTerminalResultIndex, ConsumptionPoint: runtimecontract.RuntimeProofPhasePostTerminalDelivery, TerminalContractRef: runtimecontract.RuntimeTerminalProofContractV1},
				runtimecontract.RuntimeLineageAssertionRule{Ref: "original-validation-receipt", Kind: runtimecontract.RuntimeLineageTerminalOwnerReceipt, ConsumptionPoint: runtimecontract.RuntimeProofPhasePostTerminalDelivery, TerminalContractRef: runtimecontract.RuntimeTerminalProofContractV1, ReceiptSourceScope: "dodItem", ReceiptSourceID: "cwi-2", OwnerModuleID: runtimecontract.RuntimeModuleShell, OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession, RequiredEffectScope: runtimecontract.RuntimeEffectEphemeralSession})
		}
		bootstrap.Activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements[1].LineageAssertions = rules
		bootstrap.DefinitionFocuses[1].LineageAssertions = rules
	}
	if mode == "omission" {
		bootstrap.CoverageItems[1].TargetText = "invoke the delivered program and document its output"
	}
	for index := 3; index <= options.ValidationCount+1; index++ {
		id := common.CoverageWorkItemID(fmt.Sprintf("cwi-%d", index))
		focus := bootstrap.DefinitionFocuses[1].Clone()
		focus.ID, focus.CoverageWorkItemIDs, focus.EvidenceDeclarationRef = "focus-"+string(id), []common.CoverageWorkItemID{id}, ""
		focus.AssessmentID = string(id)
		bootstrap.DefinitionFocuses = append(bootstrap.DefinitionFocuses, focus)
		declaration := runtimecontract.NormalizeRuntimeProfileContracts(bootstrap.Activation.ExecutionContract.ProfileContracts)[0].EvidenceRequirements[1]
		declaration.ID = string(id)
		bootstrap.Activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements = append(bootstrap.Activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements, declaration)
		bootstrap.CoverageItems = append(bootstrap.CoverageItems, runtimecontract.RuntimeCoverageItem{ID: id, SliceID: "slice-mainline", SourceScope: "dodItem", Order: index, PrerequisiteCoverageWorkItemIDs: []common.CoverageWorkItemID{common.CoverageWorkItemID(fmt.Sprintf("cwi-%d", index-1))}})
		bootstrap.Slices[0].CoverageWorkItemIDs = append(bootstrap.Slices[0].CoverageWorkItemIDs, id)
		runtimeTestDeclareExactManagedValidationOwnerFact(&bootstrap, id, "behavior-validation", false)
	}
	bootstrap.Definition.CapabilityContract = append(bootstrap.Definition.CapabilityContract, runtimecontract.RuntimeBlockKindDecisionRequest)
	bootstrap.Requirements = append(bootstrap.Requirements, runtimecontract.RuntimeRequirement{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleDecision, CapabilityClass: runtimecontract.RuntimeCapabilityClassGovernance, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindDecisionRequest}})
	bootstrap = runtimeTestBootstrapWithBindings(bootstrap)
	if projectBehaviorRepair {
		bootstrap = runtimeTestWithHistoricalPurposeMapping(t, bootstrap, "cwi-1", "cwi-2")
	}
	if options.SourceObligations {
		bootstrap = withSharedSourceObligationBootstrap(t, bootstrap)
	}
	preparations := runtimeTestPrepareNativeValidationThroughOwners(t, bootstrap, directory)
	requirements, err := runtimecontract.CompileRuntimeValidationProofRequirements(bootstrap.Definition.Revision, bootstrap.CoverageItems, bootstrap.DefinitionFocuses)
	if err != nil || len(requirements) != options.ValidationCount {
		t.Fatal(err)
	}
	initialFiles := map[string]string{
		"subject.go":           "package fixture\nfunc DeliveredValue() int { return 2 }\n",
		"cmd/delivery/main.go": "package main\nimport (\"fmt\"; subject \"example.com/workorch-runtime-native-fixture\")\nfunc main(){fmt.Print(subject.DeliveredValue())}\n",
		"historical_test.go":   "package fixture\n// Immutable prior input; not selected by the versioned adapter.\n",
	}
	if mode == "project_history_failure" {
		initialFiles["historical_test.go"] = "package fixture\nimport \"testing\"\nfunc TestHistorical(t *testing.T){t.Fatal(\"原工程判据失败\")}\n"
	}
	projectExecutionCounter := filepath.Join(root, "project-executions")
	// 成功合取的 ordinary/race 两阶段均执行原工程；已知 ordinary 失败则不进入 race。
	projectExecutionCount := strings.Repeat("executed\n", 2*options.ValidationCount)
	if mode == "project_history_failure" {
		projectExecutionCount = "executed\n"
		if options.Adapter == shellcommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic {
			projectExecutionCount += "executed\n" // real independent attribution attempt; still no repair authority
		}
	}
	if projectAdapter {
		outcome := ""
		if mode == "project_history_failure" {
			outcome = `t.Fatal("原工程判据失败")`
		} else if projectBehaviorRepair {
			outcome = `if DeliveredValue()!=4 { t.Fatal("WORKORCH_ASSERTION_V1 code=value_mismatch expected=4 actual=3") }`
		}
		initialFiles["historical_test.go"] = fmt.Sprintf(`package fixture
import ("os"; "testing")
func TestHistorical(t *testing.T) {
 f,err:=os.OpenFile(%q,os.O_CREATE|os.O_WRONLY|os.O_APPEND,0600); if err!=nil { t.Fatal(err) }
 if _,err:=f.WriteString("executed\n"); err!=nil { t.Fatal(err) }; if err:=f.Close(); err!=nil { t.Fatal(err) }
 %s
}
`, projectExecutionCounter, outcome)
		if projectBehaviorRepair {
			projectExecutionCount = "executed\nexecuted\nexecuted\nexecuted\n"
		}
	}
	inputTargets := map[common.CoverageWorkItemID]string{}
	for _, requirement := range requirements {
		preparation := preparations[requirement.Hash]
		if len(preparation.CheckAttestations) != 1 && !options.SourceObligations && !projectBehaviorRepair {
			t.Fatal("single-criterion fixture lost its exact owner mapping")
		}
		native, inputTarget := preparation.CheckAttestations[0].NativeCheckRef, preparation.TargetRef
		parent := "../.."
		if options.Adapter == shellcommon.ValidationRecipeAdapterIsolatedEntrypoint || options.Adapter == shellcommon.ValidationRecipeAdapterConjunctiveEntrypoint || projectAdapter {
			parent = "../../.."
		}
		if _, err := os.Stat(filepath.Join(directory, inputTarget)); err != nil {
			t.Fatalf("owner did not prepare exact input: %v", err)
		}
		inputTargets[requirement.CoverageWorkItemID] = inputTarget
		initialFiles[inputTarget] = fmt.Sprintf(`package workorch_validation
import ("testing"; "os/exec"; "path/filepath"; "strings")
func %s(t *testing.T) {
 bin:=filepath.Join(t.TempDir(),"delivery")
 build:=exec.Command("go","build","-buildvcs=false","-o",bin,"./cmd/delivery"); build.Dir=%q
 if out,err:=build.CombinedOutput(); err!=nil { t.Fatalf("build: %%v %%s",err,out) }
 out,err:=exec.Command(bin).Output(); if err!=nil || strings.TrimSpace(string(out))!="4" { t.Fatal("wrong delivered behavior") }
}

`, native, parent)
		if projectBehaviorRepair {
			initialFiles[inputTarget] = strings.Replace(initialFiles[inputTarget], "wrong delivered behavior", "WORKORCH_ASSERTION_V1 code=value_mismatch expected=4 actual=3", 1)
		}
		if options.SourceObligations || projectBehaviorRepair {
			body := initialFiles[inputTarget]
			var source strings.Builder
			for index, check := range preparation.CheckAttestations {
				part := strings.Replace(body, native, check.NativeCheckRef, 1)
				if mode == "source_B_failure" && check.RequiredCheckRef == requirement.SourceObligationChecks[1].CheckRef {
					part = strings.Replace(part, `!="4"`, `!="5"`, 1)
				}
				if index != 0 {
					part = part[strings.Index(part, "func "):]
				}
				source.WriteString(part)
			}
			initialFiles[inputTarget] = source.String()
		}
	}
	if strings.HasPrefix(mode, "input_revision") {
		for _, target := range inputTargets {
			initialFiles[target] = strings.Replace(initialFiles[target], `!="4"`, `!="5"`, 1)
		}
	}
	for ref, content := range initialFiles {
		path := filepath.Join(directory, ref)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	workspaces := filerepository.NewWorkspaceRepository(filepath.Join(root, "workspaces.json"))
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2", DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive}); err != nil {
		t.Fatal(err)
	}
	if err := workspaces.Create(ctx, runtimeWorkspaceForPersistedRunTest(t, ctx, runs, "ws-v2", "run-v2", directory, internalDir)); err != nil {
		t.Fatal(err)
	}
	var runRepository repository.RunRepository = runs
	var workspaceRepository repository.WorkspaceRepository = workspaces
	var interruptedWorkspace *failAfterWorkspaceUpdateRepository
	var interruptedRun *failAfterRunUpdateRepository
	if cut == "after_workspace_persistence" {
		interruptedWorkspace = &failAfterWorkspaceUpdateRepository{WorkspaceRepository: workspaces}
		workspaceRepository = interruptedWorkspace
	}
	if cut == "after_run_persistence" {
		interruptedRun = &failAfterRunUpdateRepository{RunRepository: runs}
		runRepository = interruptedRun
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runOwnerTestDependencies(runRepository, nil, nil, hub)); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	workspaceOwner := workspacemodule.New()
	if err := workspaceOwner.Configure(workspaceOwnerTestDependencies(hub, workspaceRepository, workspaces, memory.NewGoalRepository(), memory.NewDoDRepository(), memory.NewWorkspaceTemplateRepository())); err != nil {
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
	shellOwner := startRuntimeManagedValidationTestOwner(t, hub)
	if options.TerminalSnapshot {
		startRuntimeReviewedGitSnapshotOwner(t, hub, root)
	}
	startRuntimeProviderCeilingTestOwner(t, hub)
	resultOwner := resultmodule.New()
	var results repository.ResultRepository = memory.NewResultRepository()
	resultRepositories := testResultOwnerRepositories(results, memory.NewArtifactRepository(), memory.NewResultWorkspaceDeletionRepository())
	var interruptedResults *failAfterResultCreateRepository
	deliveryCut := cut == "after_result_persistence" || cut == "after_workspace_persistence" || cut == "after_run_persistence"
	materialCut := cut == "before_material_seal" || cut == "after_material_seal"
	if deliveryCut {
		results = filerepository.NewResultRepository(filepath.Join(root, "results.json"))
		resultRepository := results
		if cut == "after_result_persistence" {
			interruptedResults = &failAfterResultCreateRepository{ResultRepository: results}
			resultRepository = interruptedResults
		}
		resultRepositories = testResultOwnerRepositories(resultRepository, filerepository.NewArtifactRepository(filepath.Join(root, "artifacts.json")), memory.NewResultWorkspaceDeletionRepository())
	}
	if err := resultOwner.Configure(resultbiz.Dependencies{OwnerRepositories: resultRepositories, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := resultOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resultOwner.Stop(ctx) })
	tickets := memory.NewDecisionTicketRepository()
	decisionOwner := decisionmodule.New()
	if err := decisionOwner.Configure(decisionbiz.Dependencies{OwnerRepositories: testDecisionOwnerRepositories(tickets, memory.NewDecisionResolutionRepository(), memory.NewDecisionWorkspaceDeletionRepository()), EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := decisionOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = decisionOwner.Stop(ctx) })

	var calls atomic.Int64
	behaviorRepairs := 0
	var reviewCalls, validationCalls int
	var firstReviewManifest string
	var snapshotPaths []string
	if options.TerminalSnapshot {
		snapshotPaths = runtimeReviewedGitSnapshotPaths(t, directory)
	}
	provider := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	provider.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		calls.Add(1)
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		if command.DocumentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan {
			document := validationInputAdditionMainlineSemanticDocument(command)
			if strings.HasPrefix(mode, "input_revision") {
				runtimeInputRevisionMainlineSemanticDocument(&document, command, inputTargets["cwi-2"])
			}
			if options.TerminalSnapshot {
				runtimeReviewedGitSnapshotSemanticDocument(&document, command)
			}
			for i := range document.Proposals {
				proposal := &document.Proposals[i]
				binding, found := runtimecontract.RuntimeSemanticValidationInputReviewBinding(*proposal, command.SemanticManifest.OwnerCatalogs)
				if !found {
					continue
				}
				reviewCalls++
				if firstReviewManifest == "" {
					firstReviewManifest = command.SemanticManifest.Hash
				}
				conclusion := runtimecontract.RuntimeValidationInputConsistent
				if strings.HasPrefix(mode, "input_revision") {
					for _, file := range binding.InputReview.Files {
						if strings.Contains(file.Content, `!="5"`) || mode == "input_revision_exhausted" {
							conclusion = runtimecontract.RuntimeValidationInputConflict
						}
					}
				}
				if mode == "conflict" || mode == "omission" {
					conclusion = runtimecontract.RuntimeValidationInputConflict
				}
				if mode == "exhausted" || mode == "reject" || (mode == "clarify" && len(command.SemanticManifest.InputReviewInformation) == 0) {
					conclusion = runtimecontract.RuntimeValidationInputInconclusive
				}
				if len(command.SemanticManifest.InputReviewInformation) != 0 {
					info := command.SemanticManifest.InputReviewInformation[0]
					if info.SubjectHash != binding.InputReview.Hash || info.Summary != "The input invokes the delivery binary built from the workspace source." || command.SemanticManifest.Hash == firstReviewManifest {
						t.Error("ticket facts did not form a new exact semantic request")
					}
				}
				review := &runtimecontract.RuntimeValidationInputReview{SubjectHash: binding.InputReview.Hash}
				for _, check := range binding.RequiredCheckRefs {
					review.Checks = append(review.Checks, runtimecontract.RuntimeValidationInputCheckReview{RequiredCheckRef: check, Conclusion: conclusion, Reason: "Confirm whether this input invokes the delivered program without bypassing the original requirement.", InputTargetRefs: []string{inputTargets[proposal.CoverageWorkItemID]}})
					if binding.InputReview.ProtocolRef != runtimecontract.RuntimeValidationInputReviewProtocol {
						item := &review.Checks[len(review.Checks)-1]
						finding := runtimecontract.RuntimeValidationInputCovered
						if conclusion == runtimecontract.RuntimeValidationInputConflict {
							finding = runtimecontract.RuntimeValidationInputConflicting
						} else if conclusion == runtimecontract.RuntimeValidationInputInconclusive {
							finding = runtimecontract.RuntimeValidationInputUnknown
						}
						for _, coverage := range command.SemanticManifest.CoverageItems {
							if coverage.ID == proposal.CoverageWorkItemID {
								criterion := coverage.TargetText
								for _, source := range coverage.SourceObligationChecks {
									if source.CheckRef == check {
										criterion = source.Criterion
									}
								}
								item.Obligations = []runtimecontract.RuntimeValidationInputObligationReview{{CriterionText: criterion, Finding: finding, Reason: item.Reason, InputTargetRefs: item.InputTargetRefs}}
							}
						}
						if mode == "omission" && strings.HasPrefix(check, "workorch-obligation-check-") {
							// Source A/B have their own behavioral criteria, not the
							// shared CWI's additional documentation requirement.
							item.Conclusion = runtimecontract.RuntimeValidationInputConsistent
							for i := range item.Obligations {
								item.Obligations[i].Finding = runtimecontract.RuntimeValidationInputCovered
							}
						} else if mode == "omission" {
							item.Reason = "The input invokes the delivered program but does not check the required documentation."
							item.Obligations = []runtimecontract.RuntimeValidationInputObligationReview{
								{CriterionText: "invoke the delivered program", Finding: runtimecontract.RuntimeValidationInputCovered, Reason: "The input builds and invokes the delivery process.", InputTargetRefs: item.InputTargetRefs},
								{CriterionText: "and document its output", Finding: runtimecontract.RuntimeValidationInputMissing, Reason: "No documentation assertion exists in the reviewed input.", InputTargetRefs: item.InputTargetRefs},
							}
						}
						if binding.InputReview.ProtocolRef == runtimecontract.RuntimeValidationInputReviewProtocolV3 {
							for i := range item.Obligations {
								item.Obligations[i].ProofConsumerRefs = runtimecontract.RuntimeValidationProofConsumerRefs(*binding.InputReview)
							}
						}
					}
				}
				proposal.ValidationInputReview = review
				for _, source := range binding.InputReview.DiagnosticSources {
					// Scripted independent reviewer consumes the exact owner corpus.
					// Native names alone are not evidence of compatible assertions.
					body, supplied := initialFiles[source.InputTargetRef]
					matched := false
					for _, input := range binding.InputReview.Files {
						if input.TargetRef == source.InputTargetRef && input.Content == body {
							matched = true
						}
					}
					if !supplied || !matched || !strings.Contains(body, "func "+source.NativeCaseRef[strings.LastIndex(source.NativeCaseRef, "/")+1:]+"(") {
						result.Set(nil, cd.NewError(cd.Unexpected, "diagnostic source is not an exact supplied test input"))
						return
					}
					review.DiagnosticSources = append(review.DiagnosticSources, runtimecontract.RuntimeValidationDiagnosticSourceReview{SourceRef: source.SourceRef, Conclusion: conclusion, Reason: "The exact supplied fixture asserts the delivered value or preserves the original independent project constraint, without replacing the original criterion.", InputTargetRefs: []string{source.InputTargetRef}})
				}
			}
			payload, err := runtimecontract.EncodeRuntimeSemanticPlanPayloadWithManifest(command.SemanticManifest, document)
			if err != nil {
				result.Set(nil, cd.NewError(cd.Unexpected, err.Error()))
				return
			}
			if mode == "malformed" && reviewCalls > 0 {
				payload = []byte(strings.ReplaceAll(string(payload), `"consistent"`, `"approved"`))
			}
			result.Set(runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity, ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument), DocumentOutcome: runtimeCompleteDocumentOutcome(command.ExpectedDocument, payload)}), nil)
			return
		}
		var candidate runtimecontract.RuntimeCandidate
		switch command.Decision.State {
		case runtimecontract.ConvergenceStateNeedStateChange:
			if options.TerminalSnapshot && command.Decision.Focus.CoverageWorkItemIDs[0] == "cwi-git" {
				candidate = runtimeGitStateCandidateForCoverage(requiredOperationRefs(command.Decision.RequiredOutput.RequiredOperationSequence), "cwi-git", "workspace-repository")
				for i := range candidate.StateChange.Operations {
					for j := range candidate.StateChange.Operations[i].Operation.Parameters {
						parameter := &candidate.StateChange.Operations[i].Operation.Parameters[j]
						if parameter.Name == "pathspecs" {
							parameter.StringList = append([]string(nil), snapshotPaths...)
						}
					}
				}
				break
			}
			if strings.HasPrefix(mode, "input_revision") && command.CandidateManifest.ImplementationWorkingBinding != nil {
				candidate = runtimeInputRevisionMainlineCandidate(command, inputTargets["cwi-2"])
				break
			}
			oldText, newText := "return 2", "return 4"
			if projectBehaviorRepair {
				newText = "return 3"
				if command.Decision.Focus.CoverageWorkItemIDs[0] == "cwi-2" {
					oldText, newText = "return 3", "return 4"
					behaviorRepairs++
				}
			}
			candidate.StateChange = &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{ID: "deliver", Kind: runtimecontract.StepKindFileEdit, TargetRefs: []string{"subject.go"}, CoverageWorkItemIDs: command.Decision.Focus.CoverageWorkItemIDs, Parameters: []runtimecontract.OperationParameter{{Name: "oldText", String: &oldText}, {Name: "newText", String: &newText}}}}}}
		case runtimecontract.ConvergenceStateNeedValidation:
			validationCalls++
			candidate = runtimeManagedShellValidationCandidate("validate", command.Decision.Focus.CoverageWorkItemIDs[0], "subject.go", "behavior-validation")
		case runtimecontract.ConvergenceStateNeedAssessment:
			candidate = runtimeSatisfiedAssessmentCandidate(command, "exact owner evidence satisfies coverage")
		default:
			result.Set(nil, cd.NewError(cd.Unexpected, "unexpected provider action "+string(command.Decision.State)))
			return
		}
		result.Set(runtimeCandidateResultForTest(t, command, candidate), nil)
	})
	t.Cleanup(func() { provider.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })
	evidenceRecords := memory.NewEvidenceRecordRepository()
	newModule := func() *Biz {
		owner := newRuntimeTestOwner()
		if err := owner.ConfigureRuntimeRepository(runtimeStateRepository{hub: hub}); err != nil {
			t.Fatal(err)
		}
		module := newTestBiz(owner, hub, &recordingBackgroundRoutine{})
		module.evidenceRecords = evidenceRecords
		return module
	}
	module := newModule()
	observer := magicevent.NewSimpleObserver(ModuleID, hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceTicket, func(event magicevent.Event, result magicevent.Result) {
		module.handleRuntimeGovernanceTicketResolution(event, result)
	})
	observer.Subscribe(orchestratorevents.TopicOrchestratorCommandRecordDecisionEvidence, func(event magicevent.Event, result magicevent.Result) {
		module.handleDecisionEvidenceCommand(event, result)
	})
	t.Cleanup(func() {
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceTicket)
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommandRecordDecisionEvidence)
	})
	var crash *runtimeSaveCrashRepository
	if cut != "" && !deliveryCut && !materialCut {
		crash = &runtimeSaveCrashRepository{delegate: runtimeStateRepository{hub: hub}, after: cut != "before_shell_applicability", marker: cut, match: func(a runtime.RuntimeAggregate) bool {
			switch cut {
			case "after_assessment_submission":
				for _, session := range a.CandidateRequestSessions {
					if a.Decision.State == runtimecontract.ConvergenceStateNeedAssessment && containsRuntimeCoverageID(a.Facts.Focus.CoverageWorkItemIDs, "cwi-2") && session.Transaction == a.Decision.AuthorityTransaction() && session.ProviderSubmitted && session.State != runtimecontract.RuntimeCandidateRequestSessionDocumentComplete && session.State != runtimecontract.RuntimeCandidateRequestSessionTerminal {
						return true
					}
				}
				return false
			case "before_shell_applicability", "after_shell_applicability":
				for _, value := range a.Facts.EvidenceApplicabilityObservations {
					if value.CurrentDependencies != nil && len(value.CurrentDependencies.Entries) != 0 && strings.HasPrefix(value.CurrentDependencies.Entries[0].OwnerInstanceID, runtimecontract.RuntimeModuleShell+"/") {
						return true
					}
				}
				return false
			case "after_input_revision_retirement":
				for _, g := range a.SemanticPlanning.History {
					if g.Reason == runtimecontract.RuntimeSemanticPlanningRetirementInputRevision {
						return true
					}
				}
				return false
			case "after_input_revision_receipt":
				for _, r := range a.ImplementationWork.Rounds {
					if r.ScopeRequest.InputRevision != nil && r.CompletedReceiptSetHash != "" {
						return true
					}
				}
				return false
			case "after_review_document":
				return a.SemanticPlanning.Phase == runtimecontract.RuntimeSemanticPlanningProofBatchDocumentAccepted && len(a.SemanticPlanning.BatchDocuments) != 0 && a.SemanticPlanning.BatchDocuments[0].Proposals[0].ValidationInputReview != nil
			case "after_review_classification":
				return a.Facts.Failure != nil && a.Facts.Failure.Code == "validation_input_review_inconclusive"
			case "after_review_failure":
				return a.SemanticPlanning.Phase == runtimecontract.RuntimeSemanticPlanningFailed && a.Facts.Failure != nil && a.Facts.Failure.OriginalDiagnostic != nil && len(a.Facts.Failure.OriginalDiagnostic.ValidationInputReviews) != 0
			case "after_ticket_binding":
				return len(a.GovernanceRequests) == 1
			case "after_information_commit":
				return false // cut on the calling thread after Decision's reply
			case "after_review_binding":
				return a.SemanticPlanning.Phase == runtimecontract.RuntimeSemanticPlanningSealed && a.Facts.Focus.CoverageWorkItemIDs[0] == "cwi-2"
			case "after_project_receipt":
				return a.ActiveDispatch != nil && a.ActiveDispatch.Candidate.Validation != nil && a.ActiveDispatch.State == runtime.DispatchStateSealed
			case "after_repair_receipt":
				return a.ActiveDispatch != nil && a.ActiveDispatch.RepairCommitment != nil && a.ActiveDispatch.Candidate.StateChange != nil && a.ActiveDispatch.State == runtime.DispatchStateSealed
			}
			return false
		}}
		if err := module.ConfigureRuntimeRepository(crash); err != nil {
			t.Fatal(err)
		}
	}
	var receiptBeforeRecovery *runtime.RuntimeAggregate
	var originalProjectReceipt runtimecontract.RuntimeOperationReceipt
	var originalRepairReceipt *runtimecontract.RuntimeOperationReceipt
	var sealedDefinition runtimecontract.RuntimeDefinitionContext
	var resolved bool
	var completed runtime.RuntimeAggregate
	for iteration := 0; iteration < 8; iteration++ {
		panicked := false
		func() {
			defer func() {
				if value := recover(); value != nil {
					if value != cut || cut == "" {
						panic(value)
					}
					panicked = true
				}
			}()
			if !resolved {
				_, err = module.driveRuntimeLocked(ctx, orchestratorevents.DriveRuntimeCommand{WorkspaceID: "ws-v2", RunID: "run-v2"})
				if err != nil {
					t.Fatalf("drive %s: %v", mode, err)
				}
			}
			a, err := module.loadRuntime(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			if sealedDefinition.Revision == "" {
				sealedDefinition = a.Definition.Clone()
			}
			if a.ActiveDispatch != nil && a.ActiveDispatch.Candidate.Governance != nil {
				if validationCalls != 0 || len(a.GovernanceRequests) != 1 || reviewCalls != 1 {
					t.Fatal("inconclusive review leaked execution or repeated ticket")
				}
				request := a.GovernanceRequests[0]
				ticket, err := tickets.Get(ctx, request.TicketID)
				if err != nil || ticket.FailureClass != runtimecontract.RuntimeValidationInputReviewFailureClass {
					t.Fatalf("review ticket: %v", err)
				}
				resolutionType := decisioncommon.ResolutionTypeProvideMissingInfo
				if mode == "reject" {
					resolutionType = decisioncommon.ResolutionTypeRejectCurrentPath
				}
				command := decisionevents.EnsureResolvedCommand{ResolutionID: "review-information", TicketID: request.TicketID, WorkspaceID: "ws-v2", RunID: "run-v2", StepID: common.StepID("runtime/" + request.DispatchID), DecisionKind: "runtime_governance", Type: resolutionType, Payload: mustRuntimeGovernancePayload(t, map[string]any{"summary": "The input invokes the delivery binary built from the workspace source."})}
				if resolutionType == decisioncommon.ResolutionTypeProvideMissingInfo {
					for _, summary := range []string{"", " ", strings.Repeat("x", runtimecontract.RuntimeValidationInputReviewMaxReasonBytes+1)} {
						bad := command
						bad.Payload = mustRuntimeGovernancePayload(t, map[string]any{"summary": summary})
						reply := hub.Send(magicevent.NewEventWithContext(decisionevents.TopicDecisionCommandEnsureResolved, "kernel/governance", decisionevents.CommandDestination, magicevent.NewHeader(), ctx, bad))
						if reply == nil || reply.Error() == nil {
							t.Fatal("incomplete ticket facts accepted")
						}
					}
					unchanged, err := module.loadRuntime(ctx, "ws-v2", "run-v2")
					if err != nil || unchanged.StateRevision != a.StateRevision {
						t.Fatal("invalid ticket facts changed Runtime")
					}
				}
				response := hub.Send(magicevent.NewEventWithContext(decisionevents.TopicDecisionCommandEnsureResolved, "kernel/governance", decisionevents.CommandDestination, magicevent.NewHeader(), ctx, command))
				if response == nil || response.Error() != nil {
					if response == nil {
						t.Fatal("missing ticket resolution reply")
					}
					t.Fatalf("ticket resolution: %v", response.Error())
				}
				resolved = true
				continued, err := module.loadRuntime(ctx, "ws-v2", "run-v2")
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(a.Facts.Coverage, continued.Facts.Coverage) || validationCalls != 0 {
					t.Fatal("ticket resolution granted coverage or validation credit")
				}
				if resolutionType == decisioncommon.ResolutionTypeProvideMissingInfo && continued.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningRequired {
					t.Fatal("ticket bypassed fresh semantic request")
				}
				duplicate := hub.Send(magicevent.NewEventWithContext(decisionevents.TopicDecisionCommandEnsureResolved, "kernel/governance", decisionevents.CommandDestination, magicevent.NewHeader(), ctx, command))
				if duplicate == nil || duplicate.Error() != nil {
					t.Fatal("exact ticket resolution replay failed")
				}
				replayed, err := module.loadRuntime(ctx, "ws-v2", "run-v2")
				if err != nil || replayed.StateRevision != continued.StateRevision {
					t.Fatal("duplicate ticket resolution mutated Runtime")
				}
				if cut == "after_information_commit" && crash != nil && !crash.fired {
					crash.fired = true
					panic(cut)
				}
			}
			_, err = module.driveRuntimeLocked(ctx, orchestratorevents.DriveRuntimeCommand{WorkspaceID: "ws-v2", RunID: "run-v2"})
			if err != nil {
				t.Fatalf("redrive %s: %v", mode, err)
			}
			completed, err = module.loadRuntime(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
		}()
		if panicked {
			module = newModule()
			loaded, err := module.loadRuntime(ctx, "ws-v2", "run-v2")
			if err != nil || loaded.Validate() != nil {
				t.Fatalf("restart invalid: %v", err)
			}
			if cut == "after_repair_receipt" {
				if loaded.ActiveDispatch == nil || loaded.ActiveDispatch.RepairCommitment == nil || len(loaded.ActiveDispatch.OperationExecutions) != 1 {
					t.Fatal("未到达唯一原 confirmed repair dispatch")
				}
				key := loaded.ActiveDispatch.OperationExecutions[0].ExecutionKey
				func() {
					defer func() {
						if recovered := recover(); recovered != "runtime-test-crash-after-owner-receipt" {
							t.Fatalf("真实 File receipt 响应丢失切点未到达: %v", recovered)
						}
					}()
					_, _ = module.ExecuteRuntimeDispatch(ctx, "ws-v2", "run-v2", panicAfterOwnerReceiptExecutor{delegate: eventRuntimeOperationExecutor{hub: hub}})
					t.Fatal("File accepted receipt 后未中断")
				}()
				before, err := module.loadRuntime(ctx, "ws-v2", "run-v2")
				if err != nil || before.Validate() != nil {
					t.Fatal("真实 repair receipt 后 aggregate 无效", err)
				}
				if before.ActiveDispatch == nil || len(before.ActiveDispatch.OperationExecutions) != 1 || before.ActiveDispatch.OperationExecutions[0].ExecutionKey != key {
					t.Fatal("repair receipt 未保留原 execution key 与真实 claim fence")
				}
				receipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(before.OperationReceipts, key, before.ActiveDispatch.OperationExecutions[0].Fence)
				if !found || receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || !receipt.Changed {
					t.Fatal("未封存真实唯一 accepted File receipt")
				}
				originalRepairReceipt = &receipt
				if err := fileOwner.Stop(ctx); err != nil {
					t.Fatal(err)
				}
				fileOwner = filemodule.New()
				if err := fileOwner.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
					t.Fatal(err)
				}
				if err := fileOwner.Start(); err != nil {
					t.Fatal(err)
				}
				module = newModule()
				reopened, err := filerepository.NewRunRepository(filepath.Join(root, "runs.json")).GetByWorkspaceID(ctx, "ws-v2", "run-v2")
				if err != nil || reopened.RuntimeState == nil || !reflect.DeepEqual(runtimeStateRecordFromAggregate(before), *reopened.RuntimeState) {
					t.Fatal("File/Run owner 重建改变原 repair receipt 或预算")
				}
			}
			if cut == "before_shell_applicability" || cut == "after_shell_applicability" {
				if err := shellOwner.Stop(ctx); err != nil {
					t.Fatal(err)
				}
				shellOwner = startRuntimeManagedValidationTestOwner(t, hub)
				module = newModule()
				reopened, err := filerepository.NewRunRepository(filepath.Join(root, "runs.json")).GetByWorkspaceID(ctx, "ws-v2", "run-v2")
				if err != nil || reopened.RuntimeState == nil || !reflect.DeepEqual(runtimeStateRecordFromAggregate(loaded), *reopened.RuntimeState) {
					t.Fatal("owner/repository rebuild changed original applicability transaction")
				}
			}
			if cut == "after_assessment_submission" {
				original := loaded.CloneRaw()
				if _, required, err := runtime.NextRuntimeEvidenceApplicabilityRequest(loaded); err != nil || required {
					t.Fatalf("unsettled submission allowed fresh dependency query: %v", err)
				}
				run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
				if err != nil {
					t.Fatal(err)
				}
				// Seal a genuine new SM-4 reconciliation generation without yet
				// sampling owners or rewriting the submitted SM-3 authority.
				resumed, changed, err := module.reconcileRuntimeWorkspaceWithCommand(ctx, run, loaded, orchestratorevents.DriveRuntimeCommand{ReconciliationRequestID: "submitted-restart-current-read"})
				if err != nil || !changed || resumed.Reconciliation.RequestHash == loaded.Reconciliation.RequestHash {
					t.Fatalf("Resume generation did not seal: %v", err)
				}
				var settlementFault *runtimeSubmittedSettlementFaultRepository
				if options.SubmittedSettlementFault != "" {
					settlementFault = &runtimeSubmittedSettlementFaultRepository{delegate: runtimeStateRepository{hub: hub}, afterCAS: options.SubmittedSettlementFault == "after_cas"}
					if err := module.ConfigureRuntimeRepository(settlementFault); err != nil {
						t.Fatal(err)
					}
					if _, err := module.driveOneRuntimeTransition(ctx, run, resumed, orchestratorevents.DriveRuntimeCommand{}, ""); err == nil || !settlementFault.fired {
						t.Fatal("submitted settlement CAS fault was not reached")
					}
					module = newModule()
					resumed, err = module.loadRuntime(ctx, "ws-v2", "run-v2")
					if err != nil || resumed.Validate() != nil {
						t.Fatalf("settlement reload failed: %v", err)
					}
				}
				outcome, err := module.driveOneRuntimeTransition(ctx, run, resumed, orchestratorevents.DriveRuntimeCommand{}, "")
				if err != nil {
					t.Fatalf("submitted settlement was displaced by current-read debt: %v", err)
				}
				settled := outcome.aggregate
				if settled.Decision.Terminal == nil || settled.Decision.Terminal.Code != "provider_submission_unknown" || !reflect.DeepEqual(settled.OperationReceipts, original.OperationReceipts) || !reflect.DeepEqual(settled.Facts.EvidenceApplicabilityObservations, original.Facts.EvidenceApplicabilityObservations) || !reflect.DeepEqual(settled.Facts.Evidence, original.Facts.Evidence) || !reflect.DeepEqual(settled.Facts.Coverage, original.Facts.Coverage) {
					t.Fatal("fresh observation/reconciliation displaced original submission or invented credit")
				}
				unknown := 0
				for _, consumption := range settled.ExecutionBudget.Consumptions {
					if consumption.UnknownAmount > 0 {
						unknown++
					}
				}
				if unknown == 0 {
					t.Fatal("submission settlement discarded reserved unknown cost")
				}
				// Exact terminal re-drive is a zero-write no-op. Owner receipts,
				// unknown costs and observation history cannot be regenerated.
				duplicate, err := module.driveOneRuntimeTransition(ctx, run, settled, orchestratorevents.DriveRuntimeCommand{}, "")
				if err != nil || !reflect.DeepEqual(duplicate.aggregate, settled) {
					t.Fatal("duplicate submitted settlement changed original terminal")
				}
			}
			if cut == "after_project_receipt" {
				if loaded.ActiveDispatch == nil || loaded.ActiveDispatch.Candidate.Validation == nil || len(loaded.ActiveDispatch.OperationExecutions) != 1 {
					t.Fatal("未到达原 v6 validation sealed dispatch")
				}
				func() {
					defer func() {
						if recovered := recover(); recovered != "runtime-test-crash-after-owner-receipt" {
							t.Fatalf("实际 receipt 后响应丢失未到达: %v", recovered)
						}
					}()
					_, _ = module.ExecuteRuntimeDispatch(ctx, "ws-v2", "run-v2", panicAfterOwnerReceiptExecutor{delegate: eventRuntimeOperationExecutor{hub: hub}})
					t.Fatal("实际 Shell receipt 后未中断")
				}()
				before, err := module.loadRuntime(ctx, "ws-v2", "run-v2")
				if err != nil {
					t.Fatal(err)
				}
				if options.Adapter == shellcommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic {
					expectation := projectConjunctionExpectation{ProtocolRef: "shell/go-test-json/project-conjunction-behavior/v3"}
					if projectBehaviorRepair {
						expectation.RepairSources = len(preparations[requirements[0].Hash].CheckAttestations) + 1
					}
					originalProjectReceipt = assertProjectReceiptReplyLossBoundary(t, before, mode != "project_history_failure" && !projectBehaviorRepair, expectation)
				} else {
					originalProjectReceipt = assertProjectReceiptReplyLossBoundary(t, before, mode != "project_history_failure")
				}
				receiptBeforeRecovery = &before
				counter, err := os.ReadFile(projectExecutionCounter)
				expectedCount := projectExecutionCount
				if options.ValidationCount > 1 {
					expectedCount = "executed\nexecuted\n" // first receipt, remaining CWIs are not dispatched yet
				}
				if projectBehaviorRepair {
					expectedCount = "executed\nexecuted\n"
				}
				if err != nil || string(counter) != expectedCount {
					t.Fatalf("receipt 切点未取得唯一原工程实际执行事实: %q / %v", counter, err)
				}
				if err := shellOwner.Stop(ctx); err != nil {
					t.Fatal(err)
				}
				shellOwner = startRuntimeManagedValidationTestOwner(t, hub)
				module = newModule()
				reopened, err := filerepository.NewRunRepository(filepath.Join(root, "runs.json")).GetByWorkspaceID(ctx, "ws-v2", "run-v2")
				if err != nil || reopened.RuntimeState == nil || !reflect.DeepEqual(runtimeStateRecordFromAggregate(before), *reopened.RuntimeState) {
					t.Fatal("独立 Run repository 重载改变原 receipt／预算前像")
				}
			}
			continue
		}
		if completed.Decision.Terminal != nil {
			break
		}
	}
	if completed.Decision.Terminal == nil {
		t.Fatalf("review mainline did not converge: %s / %s", completed.Decision.State, completed.SemanticPlanning.Phase)
	}
	if !reflect.DeepEqual(sealedDefinition, completed.Definition) {
		t.Fatal("review or ticket rewrote sealed startup definition")
	}
	submissionUnknown := cut == "after_assessment_submission"
	success := (mode == "consistent" || mode == "clarify" || mode == "input_revision" || projectBehaviorRepair) && !submissionUnknown
	if (completed.Decision.State == runtimecontract.ConvergenceStateComplete) != success {
		for _, receipt := range completed.OperationReceipts {
			if receipt.Outcome == runtimecontract.RuntimeOperationOutcomeRejected {
				t.Logf("rejected owner receipt: feedback=%+v validation=%+v", receipt.Feedback, receipt.Evidence.Validation)
				if receipt.Feedback.OriginalDiagnostic != nil {
					t.Logf("owner diagnostic: %+v", *receipt.Feedback.OriginalDiagnostic)
				}
				if receipt.Evidence.Validation != nil && receipt.Evidence.Validation.ExecutionConjunction != nil {
					t.Logf("owner stages: %+v", receipt.Evidence.Validation.ExecutionConjunction.Stages)
					origin, originErr := runtimecontract.RuntimeOperationExecutionKeyOrigin(completed.Facts.WorkspaceID, completed.Facts.RunID, receipt.ExecutionKey, receipt.OperationID)
					if originErr == nil {
						for _, stage := range receipt.Evidence.Validation.ExecutionConjunction.Stages {
							command := shellevents.QueryRuntimeValidationRawOutputCommand{WorkspaceID: completed.Facts.WorkspaceID, RunID: completed.Facts.RunID, DispatchID: origin, OperationID: receipt.OperationID, ExecutionKey: receipt.ExecutionKey, Fence: receipt.Fence, SourceReceiptHash: runtimecontract.StableRuntimeOperationReceiptHash(receipt), StageID: stage.Identity.ID, Stream: "stderr"}
							response := hub.Send(magicevent.NewEventWithContext(shellevents.TopicQueryRuntimeValidationRawOutput, ModuleID, runtimecontract.RuntimeModuleShell, magicevent.NewHeader(), ctx, command))
							if response != nil {
								data, readErr := response.Get()
								if output, ok := data.(shellevents.QueryRuntimeValidationRawOutputResult); ok && readErr == nil && output.ValidateFor(command) == nil {
									t.Logf("owner stage %s original stderr: %s", stage.Identity.ID, output.Raw)
								}
							}
						}
					}
				}
			}
		}

		t.Fatalf("mode=%s state=%s terminal=%+v failure=%+v", mode, completed.Decision.State, completed.Decision.Terminal, completed.Facts.Failure)
	}
	expectedValidations := options.ValidationCount
	if projectBehaviorRepair {
		expectedValidations++
	}
	if success && validationCalls != expectedValidations || !success && !submissionUnknown && mode != "source_B_failure" && mode != "project_history_failure" && validationCalls != 0 || (mode == "source_B_failure" || submissionUnknown) && validationCalls != 1 {
		t.Fatalf("unexpected validation calls: %d; state=%s terminal=%+v failure=%+v evidenceRevision=%s observations=%+v", validationCalls, completed.Decision.State, completed.Decision.Terminal, completed.Facts.Failure, completed.Facts.EvidenceRevision, completed.Facts.EvidenceApplicabilityObservations)
	}
	if mode == "project_history_failure" && (validationCalls != 2 || completed.Facts.Failure == nil || completed.Facts.Failure.Code != "candidate_authority_exhausted" || len(completed.Facts.Evidence.Rejected) != 1) {
		// 这里计数的是 provider candidate 请求：第二个相同候选必须在 owner I/O 前被原拒绝重放门禁终止。
		// 实际 receipt/阶段次数由 assertProjectConjunctionPersistedOwnerOutputs 独立反证。
		t.Fatal("original project failure retried owner execution or lost bounded candidate rejection")
	}
	if options.SourceObligations {
		if success || mode == "source_B_failure" {
			assertSharedSourceObligationOwnerObservation(t, completed, requirements[0], mode == "source_B_failure")
		}
		if success {
			assertActualShellAssessmentProofDependencies(t, runtimeStateRecordFromAggregate(completed))
		}
	}
	if (mode == "conflict" || mode == "omission" || mode == "malformed") && len(completed.GovernanceRequests) != 0 {
		t.Fatal("conflict/format error created clarification ticket")
	}
	if mode == "clarify" || mode == "exhausted" {
		if reviewCalls != 2 || len(completed.GovernanceRequests) != 1 || len(completed.GovernanceResolutions) != 1 {
			t.Fatalf("unbounded review/ticket: reviews=%d tickets=%d", reviewCalls, len(completed.GovernanceRequests))
		}
	}
	if mode == "clarify" && cut == "" {
		for _, mutate := range []func(*runtime.RuntimeAggregate){
			func(a *runtime.RuntimeAggregate) { a.GovernanceResolutions[0].Summary = "forged approval" },
			func(a *runtime.RuntimeAggregate) {
				for i := range a.SemanticPlanning.History {
					if a.SemanticPlanning.History[i].InputReviewResolutionID != "" {
						a.SemanticPlanning.History[i].InputReviewResolutionID = "stale"
					}
				}
			},
			func(a *runtime.RuntimeAggregate) {
				a.SemanticPlanning.Manifest.InputReviewInformation = nil
				a.SemanticPlanning.Manifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(*a.SemanticPlanning.Manifest)
			},
		} {
			bad := runtimeAggregateFromRecord(runtimeStateRecordFromAggregate(completed))
			mutate(&bad)
			if err := bad.Validate(); err == nil {
				t.Fatal("review resolution/preimage tamper admitted after reload")
			}
		}
	}
	for ref := range initialFiles {
		if ref == "subject.go" {
			continue
		}
		content, err := os.ReadFile(filepath.Join(directory, ref))
		expectedInput := initialFiles[ref]
		if strings.HasPrefix(mode, "input_revision") && ref == inputTargets["cwi-2"] {
			expectedInput = strings.Replace(expectedInput, `!="5"`, `!="4"`, 1)
		}
		if err != nil || string(content) != expectedInput {
			t.Fatal("review path rewrote existing input")
		}
	}
	if cut != "" && !deliveryCut && !materialCut && !crash.fired {
		t.Fatalf("restart cut not reached: %s", cut)
	}
	if strings.HasPrefix(mode, "input_revision") {
		assertRuntimeInputRevisionMainline(t, completed, reviewCalls)
	}
	if projectBehaviorRepair && behaviorRepairs != 1 {
		t.Fatal("same-binding implementation repair was missing or repeated")
	}
	if projectBehaviorRepair {
		assertMultiFailureConfirmedRepairFacts(t, runtimeStateRecordFromAggregate(completed))
		if originalRepairReceipt != nil {
			current, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(completed.OperationReceipts, originalRepairReceipt.ExecutionKey, originalRepairReceipt.Fence)
			if !found || !runtimecontract.SameRuntimeOperationReceipt(current, *originalRepairReceipt) {
				t.Fatal("恢复改写了原 accepted repair receipt")
			}
		}
	}
	if projectAdapter && (success || mode == "project_history_failure") {
		if options.TerminalSnapshot && success {
			projectExecutionCount = runtimeReviewedGitProjectExecutionCount(t, runtimeStateRecordFromAggregate(completed))
		} else if options.ValidationCount > 1 && success {
			// Each later CWI requires fresh currentness of the already settled
			// predecessors. These owner observations do not create new receipts
			// or credit; their exact per-CWI request identities remain unique.
			wanted := make(map[common.CoverageWorkItemID]int)
			for i := 2; i <= options.ValidationCount; i++ {
				wanted[common.CoverageWorkItemID(fmt.Sprintf("cwi-%d", i))] = options.ValidationCount + 1 - i
			}
			projectExecutionCount = runtimeReviewedProjectExecutionCount(t, runtimeStateRecordFromAggregate(completed), options.ValidationCount, wanted)
		}
		// Replay/raw-read requires the original resource owners. Check before a
		// delivery cut replaces those owners with a separate Hub.
		if err := shellOwner.Stop(ctx); err != nil {
			t.Fatal(err)
		}
		shellOwner = startRuntimeManagedValidationTestOwner(t, hub)
		reopened, err := filerepository.NewRunRepository(filepath.Join(root, "runs.json")).GetByWorkspaceID(ctx, "ws-v2", "run-v2")
		if err != nil || reopened.RuntimeState == nil || !reflect.DeepEqual(runtimeStateRecordFromAggregate(completed), *reopened.RuntimeState) {
			t.Fatal("reopened Run lost original persisted conjunction")
		}
		protocol := "shell/go-test-json/project-conjunction-behavior/v1"
		if options.Adapter == shellcommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic {
			protocol = "shell/go-test-json/project-conjunction-behavior/v3"
		}
		expectation := projectConjunctionExpectation{ProtocolRef: protocol, ValidationReceipts: options.ValidationCount}
		if projectBehaviorRepair {
			expectation.RepairSources = len(preparations[requirements[0].Hash].CheckAttestations) + 1
		}
		assertProjectConjunctionPersistedOwnerOutputs(t, ctx, hub, module, *reopened.RuntimeState, success, expectation)
		counter, err := os.ReadFile(projectExecutionCounter)
		if err != nil || string(counter) != projectExecutionCount {
			t.Fatalf("恢复或 duplicate 重复执行原工程判据: %q / %v", counter, err)
		}
		if cut == "after_project_receipt" {
			if receiptBeforeRecovery == nil {
				t.Fatal("未观察到实际 receipt 响应丢失边界")
			}
			if projectBehaviorRepair {
				current, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(reopened.RuntimeState.OperationReceipts, originalProjectReceipt.ExecutionKey, originalProjectReceipt.Fence)
				if !found || !runtimecontract.SameRuntimeOperationReceipt(current, originalProjectReceipt) {
					t.Fatal("repair recovery rewrote original rejected receipt")
				}
				assertProjectReceiptBudgetConsumption(t, runtimeAggregateFromRecord(*reopened.RuntimeState), current)
			} else {
				additional := 0
				if success {
					additional = options.ValidationCount - 1 // only the remaining declared CWIs may produce new receipts
				}
				assertProjectReceiptPreservedAfterRecovery(t, *receiptBeforeRecovery, runtimeAggregateFromRecord(*reopened.RuntimeState), originalProjectReceipt, additional)
			}
		}
	}
	if materialCut {
		before := completed.CloneRaw()
		crash = &runtimeSaveCrashRepository{delegate: runtimeStateRepository{hub: hub}, after: cut == "after_material_seal", marker: cut, match: func(a runtime.RuntimeAggregate) bool { return a.TerminalMaterialSeal != nil }}
		if err := module.ConfigureRuntimeRepository(crash); err != nil {
			t.Fatal(err)
		}
		func() {
			defer func() {
				if recovered := recover(); recovered != cut {
					t.Fatalf("material seal CAS cut not reached: %v", recovered)
				}
			}()
			proofs, err := module.observeRuntimeTerminalSnapshotProofs(ctx, completed)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = module.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventTerminalMaterialSealRequired, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: completed.Facts.WorkspaceID, RunID: completed.Facts.RunID}, Delivery: &runtime.RuntimeInternalDeliveryPayload{TerminalSnapshotProofs: proofs}})
			t.Fatal("material seal CAS cut did not interrupt")
		}()
		if !crash.fired {
			t.Fatal("material seal crash wrapper did not fire")
		}
		reloadedRun, err := filerepository.NewRunRepository(filepath.Join(root, "runs.json")).GetByWorkspaceID(ctx, "ws-v2", "run-v2")
		if err != nil || reloadedRun.RuntimeState == nil {
			t.Fatalf("material seal fresh Run load: %v", err)
		}
		reloaded := runtimeAggregateFromRecord(*reloadedRun.RuntimeState)
		if err := reloaded.Validate(); err != nil || len(reloaded.DeliveryAcknowledgements) != 0 || (reloaded.TerminalMaterialSeal != nil) != (cut == "after_material_seal") {
			t.Fatalf("material cut lost original seal/no-ACK distinction: %v", err)
		}
		withoutSeal := reloaded.CloneRaw()
		withoutSeal.TerminalMaterialSeal, withoutSeal.StateRevision = nil, before.StateRevision
		if !reflect.DeepEqual(before, withoutSeal) {
			t.Fatal("material seal CAS changed authority, provider budget or original receipts")
		}
		items, err := results.ListByRun(ctx, "run-v2")
		if err != nil || len(items) != 0 {
			t.Fatal("material seal CAS executed Result I/O before returning")
		}
		module = newModule()
	}
	if deliveryCut {
		fired := func() bool {
			switch cut {
			case "after_result_persistence":
				return interruptedResults.fired
			case "after_workspace_persistence":
				return interruptedWorkspace.fired
			default:
				return interruptedRun.fired
			}
		}
		stopResources := func() {
			if err := workspaceOwner.Stop(ctx); err != nil {
				t.Fatal(err)
			}
			if err := runOwner.Stop(ctx); err != nil {
				t.Fatal(err)
			}
		}
		module, results = restartSourceObligationTerminalDelivery(t, ctx, module, completed, resultOwner, stopResources, fired, cut, root)
	}
	command := orchestratorevents.DriveRuntimeCommand{WorkspaceID: "ws-v2", RunID: "run-v2"}
	for range 2 {
		if err := module.driveAndProjectRuntimeLockedWithCommand(ctx, command); err != nil {
			t.Fatal(err)
		}
	}
	stored, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || stored.RuntimeState == nil || !runtimeTerminalDeliveryFullyAcknowledged(runtimeAggregateFromRecord(*stored.RuntimeState)) {
		t.Fatalf("terminal owner delivery incomplete: %v", err)
	}
	if (options.DeliveryGuard || options.TerminalMaterialGuards) && success {
		bindings, err := runtimecontract.RuntimeDeliveryLineageBindingsFromState(*stored.RuntimeState)
		if err != nil || len(bindings) != 1 {
			t.Fatalf("delivery source: %v %+v", err, bindings)
		}
		state, err := runtimecontract.EvaluateRuntimeDeliveryLineageAssertionFromState(*stored.RuntimeState, bindings[0])
		if err != nil || state != runtimecontract.RuntimeDeliveryLineageSatisfied {
			t.Fatalf("delivery guard not independently consumed: %s %v", state, err)
		}
		if options.TerminalMaterialGuards {
			assertTerminalMaterialConsumerTamper(t, *stored.RuntimeState, bindings[0])
		}
	}
	if options.TerminalMaterialGuards && !success {
		bindings, err := runtimecontract.RuntimeDeliveryLineageBindingsFromState(*stored.RuntimeState)
		if err != nil || len(bindings) != 1 {
			t.Fatalf("failed terminal obligation source: %v", err)
		}
		state, err := runtimecontract.EvaluateRuntimeDeliveryLineageAssertionFromState(*stored.RuntimeState, bindings[0])
		if err != nil || state != runtimecontract.RuntimeDeliveryLineageUnsatisfied {
			t.Fatalf("failed delivery invented success-material satisfaction: %s %v", state, err)
		}
	}
	if options.SourceObligations && success {
		assertActualShellAssessmentProofDependencies(t, *stored.RuntimeState)
	}
	if options.SnapshotInputSources && success {
		assertRuntimeReviewedInputSnapshotSources(t, runtimeAggregateFromRecord(*stored.RuntimeState))
	}
	if options.TerminalSnapshot && success {
		assertRuntimeTerminalSnapshotProof(t, *stored.RuntimeState, directory)
		counter, err := os.ReadFile(projectExecutionCounter)
		if err != nil || string(counter) != projectExecutionCount {
			t.Fatalf("terminal retry repeated original/reconciliation project checks: %q / %v", counter, err)
		}
	}
	wantRun, wantResult, wantCoverage := common.RunStateFailed, common.ResultStateRejected, 1
	if success {
		wantRun, wantResult, wantCoverage = common.RunStateCompleted, common.ResultStateAccepted, options.ValidationCount+1
		if options.TerminalSnapshot {
			wantCoverage++
		}
	}
	closeouts, err := results.ListByRun(ctx, "run-v2")
	if err != nil || stored.State != wantRun || len(closeouts) != 1 || closeouts[0].State != wantResult || closeouts[0].RuntimeCloseout == nil || len(closeouts[0].RuntimeCloseout.SatisfiedCoverageWorkItemIDs) != wantCoverage {
		t.Fatalf("review/validation/acceptance conflated: run=%s result=%+v err=%v", stored.State, closeouts, err)
	}
	if options.SourceObligations {
		closeout := closeouts[0].RuntimeCloseout
		if closeout.SourceObligations == nil || closeout.SourceObligations.Hash != bootstrap.Definition.SourceObligations.Hash {
			t.Fatal("terminal delivery omitted immutable source inventory")
		}
		members := 0
		for _, outcome := range closeout.RequirementOutcomes {
			for _, source := range outcome.SourceObligations {
				members++
				if success && (source.AssessmentState != runtimecontract.RuntimeRequirementSatisfied || source.CurrentBindingRef == nil || len(source.AcceptedEvidenceRefs) == 0) {
					t.Fatal("source outcome has no current accepted proof")
				}
				if !success && (source.AssessmentState == runtimecontract.RuntimeRequirementSatisfied || len(source.AcceptedEvidenceRefs) != 0) {
					t.Fatal("failed B restored source credit from A")
				}
			}
		}
		if members != 2 || closeout.ValidateRaw() != nil {
			t.Fatal("Result lost complete independently bound source outcomes")
		}
	}
	if mode == "conflict" || mode == "omission" || mode == "exhausted" {
		diagnostic := closeouts[0].RuntimeCloseout.OriginalDiagnostic
		if diagnostic == nil || len(diagnostic.ValidationInputReviews) != 1 || !strings.Contains(strings.Join(closeouts[0].SummaryDetail.KnownLimitations, "\n"), diagnostic.ValidationInputReviews[0].Report.Checks[0].Reason) {
			t.Logf("review terminal source: failure=%+v diagnostic=%+v limitations=%+v", completed.Facts.Failure, diagnostic, closeouts[0].SummaryDetail.KnownLimitations)
			t.Fatal("Result omitted the original input review reason or exact report identity")
		}
		if mode == "omission" && diagnostic.OriginalCode != "validation_input_review_coverage_incomplete" {
			t.Fatal("typed omission was conflated with contradiction")
		}
		for _, mutate := range []func(*runtimecontract.RuntimeOriginalDiagnostic){
			func(d *runtimecontract.RuntimeOriginalDiagnostic) { d.ValidationInputReviews = nil },
			func(d *runtimecontract.RuntimeOriginalDiagnostic) {
				d.ValidationInputReviews[0].Report.Checks[0].Reason = "forged review"
			},
			func(d *runtimecontract.RuntimeOriginalDiagnostic) {
				d.ValidationInputReviews[0].ReportHash = "runtime-input-review-report-forged"
			},
		} {
			bad := runtimeAggregateFromRecord(runtimeStateRecordFromAggregate(completed))
			mutate(bad.Facts.Failure.OriginalDiagnostic)
			if bad.Validate() == nil {
				t.Fatal("fresh-load accepted a diagnostic without exact report provenance")
			}
		}
	}
	return calls.Load()
}
