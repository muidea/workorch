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
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	workspacemodule "github.com/muidea/workorch/internal/modules/kernel/workspace"
	"github.com/muidea/workorch/internal/pkg/common"
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
	Adapter         string
	ValidationCount int
	ProtocolRef     string
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
	if adapter != shellcommon.ValidationRecipeAdapterIsolatedEntrypoint {
		t.Fatal("default template does not enable reviewed isolated inputs")
	}
	if protocol != runtimecontract.RuntimeValidationInputReviewProtocolV2 {
		t.Fatal("default template does not seal the obligation-review protocol")
	}
	options := validationInputReviewMainlineOptions{Adapter: adapter, ValidationCount: 1, ProtocolRef: protocol}
	for _, mode := range []string{"consistent", "conflict", "clarify", "exhausted", "malformed", "reject"} {
		t.Run(mode, func(t *testing.T) { runValidationInputReviewMainline(t, mode, "", options) })
	}
	t.Run("multiple_cwi", func(t *testing.T) {
		runValidationInputReviewMainline(t, "consistent", "", validationInputReviewMainlineOptions{Adapter: adapter, ValidationCount: 3, ProtocolRef: protocol})
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
	bootstrap.Activation.EvidencePolicies[0].RecipeAdapterRef = options.Adapter
	bootstrap.Activation.EvidencePolicies[0].RecipeInputReviewProtocolRef = options.ProtocolRef
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
	runtimeTestPrepareNativeValidationThroughOwners(t, bootstrap, directory)
	requirements, err := runtimecontract.CompileRuntimeValidationProofRequirements(bootstrap.Definition.Revision, bootstrap.CoverageItems, bootstrap.DefinitionFocuses)
	if err != nil || len(requirements) != options.ValidationCount {
		t.Fatal(err)
	}
	initialFiles := map[string]string{
		"subject.go":           "package fixture\nfunc DeliveredValue() int { return 2 }\n",
		"cmd/delivery/main.go": "package main\nimport (\"fmt\"; subject \"example.com/workorch-runtime-native-fixture\")\nfunc main(){fmt.Print(subject.DeliveredValue())}\n",
		"historical_test.go":   "package fixture\n// Immutable prior input; not selected by the versioned adapter.\n",
	}
	inputTargets := map[common.CoverageWorkItemID]string{}
	for _, requirement := range requirements {
		native := "TestWorkOrchCWI_" + strings.ToUpper(strings.TrimPrefix(requirement.RequiredCheckRefs[0], "workorch-cwi-check-"))
		parent, suffix := "../..", "_InputV3"
		if options.Adapter == shellcommon.ValidationRecipeAdapterIsolatedEntrypoint {
			parent, suffix = "../../..", "_InputV4"
		}
		native += suffix
		inputTarget := "workorch_validation/v3/" + strings.ToLower(native) + "_test.go"
		if options.Adapter == shellcommon.ValidationRecipeAdapterIsolatedEntrypoint {
			inputTarget = "workorch_validation/v4/" + strings.ToLower(native) + "/input_test.go"
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
	results := memory.NewResultRepository()
	if err := resultOwner.Configure(resultbiz.Dependencies{OwnerRepositories: testResultOwnerRepositories(results, memory.NewArtifactRepository(), memory.NewResultWorkspaceDeletionRepository()), EventHub: hub}); err != nil {
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
	var reviewCalls, validationCalls int
	var firstReviewManifest string
	provider := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	provider.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		calls.Add(1)
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		if command.DocumentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan {
			document := validationInputAdditionMainlineSemanticDocument(command)
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
					if binding.InputReview.ProtocolRef == runtimecontract.RuntimeValidationInputReviewProtocolV2 {
						item := &review.Checks[len(review.Checks)-1]
						finding := runtimecontract.RuntimeValidationInputCovered
						if conclusion == runtimecontract.RuntimeValidationInputConflict {
							finding = runtimecontract.RuntimeValidationInputConflicting
						} else if conclusion == runtimecontract.RuntimeValidationInputInconclusive {
							finding = runtimecontract.RuntimeValidationInputUnknown
						}
						for _, coverage := range command.SemanticManifest.CoverageItems {
							if coverage.ID == proposal.CoverageWorkItemID {
								item.Obligations = []runtimecontract.RuntimeValidationInputObligationReview{{CriterionText: coverage.TargetText, Finding: finding, Reason: item.Reason, InputTargetRefs: item.InputTargetRefs}}
							}
						}
						if mode == "omission" {
							item.Reason = "The input invokes the delivered program but does not check the required documentation."
							item.Obligations = []runtimecontract.RuntimeValidationInputObligationReview{
								{CriterionText: "invoke the delivered program", Finding: runtimecontract.RuntimeValidationInputCovered, Reason: "The input builds and invokes the delivery process.", InputTargetRefs: item.InputTargetRefs},
								{CriterionText: "and document its output", Finding: runtimecontract.RuntimeValidationInputMissing, Reason: "No documentation assertion exists in the reviewed input.", InputTargetRefs: item.InputTargetRefs},
							}
						}
					}
				}
				proposal.ValidationInputReview = review
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
			oldText, newText := "return 2", "return 4"
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
	if cut != "" {
		crash = &runtimeSaveCrashRepository{delegate: runtimeStateRepository{hub: hub}, after: true, marker: cut, match: func(a runtime.RuntimeAggregate) bool {
			switch cut {
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
			}
			return false
		}}
		if err := module.ConfigureRuntimeRepository(crash); err != nil {
			t.Fatal(err)
		}
	}
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
	success := mode == "consistent" || mode == "clarify"
	if (completed.Decision.State == runtimecontract.ConvergenceStateComplete) != success {
		t.Fatalf("mode=%s state=%s terminal=%+v failure=%+v", mode, completed.Decision.State, completed.Decision.Terminal, completed.Facts.Failure)
	}
	if success && validationCalls != options.ValidationCount || !success && validationCalls != 0 {
		t.Fatalf("unexpected validation calls: %d", validationCalls)
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
		if err != nil || string(content) != initialFiles[ref] {
			t.Fatal("review path rewrote existing input")
		}
	}
	if cut != "" && !crash.fired {
		t.Fatalf("restart cut not reached: %s", cut)
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
	wantRun, wantResult, wantCoverage := common.RunStateFailed, common.ResultStateRejected, 1
	if success {
		wantRun, wantResult, wantCoverage = common.RunStateCompleted, common.ResultStateAccepted, options.ValidationCount+1
	}
	closeouts, err := results.ListByRun(ctx, "run-v2")
	if err != nil || stored.State != wantRun || len(closeouts) != 1 || closeouts[0].State != wantResult || closeouts[0].RuntimeCloseout == nil || len(closeouts[0].RuntimeCloseout.SatisfiedCoverageWorkItemIDs) != wantCoverage {
		t.Fatalf("review/validation/acceptance conflated: run=%s result=%+v err=%v", stored.State, closeouts, err)
	}
	if mode == "conflict" || mode == "omission" || mode == "exhausted" {
		diagnostic := closeouts[0].RuntimeCloseout.OriginalDiagnostic
		if diagnostic == nil || len(diagnostic.ValidationInputReviews) != 1 || !strings.Contains(strings.Join(closeouts[0].SummaryDetail.KnownLimitations, "\n"), diagnostic.ValidationInputReviews[0].Report.Checks[0].Reason) {
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
