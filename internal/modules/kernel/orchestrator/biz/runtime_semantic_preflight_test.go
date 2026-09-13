package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	llmmodule "github.com/muidea/workorch/internal/modules/blocks/llm"
	llmbiz "github.com/muidea/workorch/internal/modules/blocks/llm/biz"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Do not install the generic always-accepted preflight test responder: these
// tests exercise the real typed preflight boundary and its durable cut points.
func newRuntimeSemanticPreflightModule(t *testing.T, hub magicevent.Hub, repo runtime.RuntimeStateRepository) *Biz {
	t.Helper()
	module := New()
	if err := module.Configure(Dependencies{
		Evidence: memory.NewOrchestratorOwnerRepositorySet(), CommandHub: runtimeTestHubWithRunRetirementFallback(hub),
		BackgroundRoutine: &recordingBackgroundRoutine{executeAsync: true},
	}); err != nil {
		t.Fatal(err)
	}
	module.evidence.ArtifactContinuations = runtimeCandidateRequestContentTestStore(hub)
	if err := module.ConfigureRuntimeRepository(repo); err != nil {
		t.Fatal(err)
	}
	return module
}

func TestRuntimeSemanticLatePreflightCannotReopenTerminatedIntent(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	repo, aggregate := runtimeSemanticPreflightFixture(t)
	module := newRuntimeSemanticPreflightModule(t, hub, repo)
	started, err := module.driveRuntimeSemanticPlanning(ctx, aggregate)
	if err != nil {
		t.Fatal(err)
	}
	preflights, providers := 0, 0
	var terminated runtime.RuntimeAggregate
	observer := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	observer.Subscribe(llmevents.TopicPreflightRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		preflights++
		var terminateErr error
		terminated, terminateErr = module.failRuntimeSemanticContract(ctx, started, "semantic_request_superseded", "request terminated while preflight was pending", nil)
		if terminateErr != nil {
			t.Errorf("terminate request intent: %v", terminateErr)
			result.Set(nil, nil)
			return
		}
		command := event.Data().(llmevents.PreflightRuntimeCandidateCommand)
		result.Set(llmevents.PreflightRuntimeCandidateResult{DocumentKind: command.DocumentKind, Feedback: runtimePreflightFeedbackForTest(t, command, "")}, nil)
	})
	observer.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(_ magicevent.Event, _ magicevent.Result) { providers++ })
	if _, err := module.driveRuntimeSemanticPlanning(ctx, started); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("late successful preflight did not conflict: %v", err)
	}
	if preflights != 1 || providers != 0 || terminated.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningFailed ||
		len(repo.aggregate.CandidateRequestSessions) != 0 || repo.aggregate.StateRevision != terminated.StateRevision ||
		!reflect.DeepEqual(started.ExecutionBudget.Clone(), repo.aggregate.ExecutionBudget.Clone()) {
		t.Fatal("late preflight opened a session, allocation, or provider call")
	}
	restarted := newRuntimeSemanticPreflightModule(t, hub, repo)
	if _, err := restarted.driveRuntimeSemanticPlanning(ctx, terminated); err != nil || preflights != 1 || providers != 0 {
		t.Fatalf("restart replayed the terminated request: %v", err)
	}
}

func TestRuntimeSemanticRenderedPreflightRejectsBeforeSession(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	repo, aggregate := runtimeSemanticPreflightFixture(t)
	aggregate.Facts.Resources.ProviderCeiling.ContextWindowTokens = 128000
	aggregate.Facts.Resources.ProviderCeiling.MaxInputTokens = 65536
	aggregate.Facts.Resources.ProviderCeiling.MaxBytes = 128
	repo.aggregate = aggregate
	providers := 0
	llm := llmmodule.New()
	if err := llm.Configure(llmbiz.Dependencies{
		EventHub: hub, LoadLLMConfig: testLLMConfigurationSnapshot,
		CandidateProvider: eventCandidateProviderFunc(func(_ context.Context, request llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
			providers++
			return eventGenerationResult(request, nil, errors.New("unexpected provider call"))
		}),
	}); err != nil {
		t.Fatal(err)
	}
	if err := llm.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = llm.Stop(ctx) })
	module := newRuntimeSemanticPreflightModule(t, hub, repo)
	started, err := module.driveRuntimeSemanticPlanning(ctx, aggregate)
	if err != nil {
		t.Fatal(err)
	}
	// Establish that this exact semantic contract renders successfully with a
	// sufficient byte allowance, so rejection cannot pass due to a bad fixture.
	controlCeiling := *started.Facts.Resources.ProviderCeiling
	controlCeiling.MaxBytes = 96 * 1024
	controlContext := runtimecontract.RuntimeSemanticPlanRequestContext{
		WorkspaceID: started.Facts.WorkspaceID, RunID: started.Facts.RunID, Definition: started.Definition,
		DefinitionSeed: started.Facts.DefinitionSeed, Activation: started.Facts.Activation, ProviderCeiling: &controlCeiling,
		PlanStep: runtimecontract.RuntimeSemanticPlanStepCoverageOrder,
	}
	controlContext.CoverageProgress, err = runtimecontract.BuildRuntimeSemanticCoverageProgress(controlContext, *started.SemanticPlanning.Manifest, started.Facts, started.StateRevision)
	if err != nil {
		t.Fatal(err)
	}
	controlContext.ExecutionHistory, err = runtime.SemanticExecutionHistory(started, controlContext, *started.SemanticPlanning.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	controlContext.ExecutionBudget, err = runtime.SemanticExecutionBudget(started, controlContext, *started.SemanticPlanning.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	control, err := PreflightRuntimeCandidate(ctx, hub, llmevents.PreflightRuntimeCandidateCommand{
		WorkspaceID: started.Facts.WorkspaceID, RunID: started.Facts.RunID,
		DocumentKind:     runtimecontract.RuntimeProviderDocumentSemanticPlan,
		StablePrefixHash: started.SemanticPlanning.StablePrefixHash, SemanticManifest: started.SemanticPlanning.Manifest.Clone(),
		SemanticContext: controlContext,
	})
	if err != nil || control.Feedback.StableCategory != runtimecontract.RuntimeFeedbackCategoryAccepted {
		t.Fatalf("semantic rendering control failed: err=%v feedback=%+v", err, control.Feedback)
	}
	failed, err := module.driveRuntimeSemanticPlanning(ctx, started)
	if err != nil {
		t.Fatal(err)
	}
	if providers != 0 || len(failed.CandidateRequestSessions) != 0 || failed.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningFailed ||
		failed.Facts.Failure == nil || failed.Facts.Failure.OriginalDiagnostic == nil || failed.Facts.Failure.OriginalDiagnostic.OwnerModuleID != runtimecontract.RuntimeModuleLLM ||
		!reflect.DeepEqual(started.ExecutionBudget, failed.ExecutionBudget) {
		t.Fatalf("rendered rejection did not close without physical work: providers=%d sessions=%d failure=%+v", providers, len(failed.CandidateRequestSessions), failed.Facts.Failure)
	}
}

type semanticPreflightCutRepository struct {
	runtime.RuntimeStateRepository
	match        func(runtime.RuntimeAggregate) bool
	after, fired bool
	cut          error
}

func (r *semanticPreflightCutRepository) Load(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (runtime.RuntimeAggregate, error) {
	value, err := r.RuntimeStateRepository.Load(ctx, workspaceID, runID)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	// Model the detached values returned by opaque Run persistence. A shallow
	// in-memory slice would let a rejected CAS modify the stored session.
	encoded, err := json.Marshal(value)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	var detached runtime.RuntimeAggregate
	err = json.Unmarshal(encoded, &detached)
	return detached, err
}

func (r *semanticPreflightCutRepository) Save(ctx context.Context, aggregate runtime.RuntimeAggregate, revision uint64) error {
	if r.fired || !r.match(aggregate) {
		return r.RuntimeStateRepository.Save(ctx, aggregate, revision)
	}
	r.fired = true
	if r.after {
		if err := r.RuntimeStateRepository.Save(ctx, aggregate, revision); err != nil {
			return err
		}
	}
	return r.cut
}

func TestRuntimeSemanticPreflightSessionRestartMatrix(t *testing.T) {
	for _, boundary := range []string{"prepared", "submitted", "document_complete", "recovery_pending"} {
		for _, after := range []bool{false, true} {
			t.Run(boundary+map[bool]string{false: "/before_cas", true: "/response_lost"}[after], func(t *testing.T) {
				ctx := context.Background()
				hub := magicevent.NewHub(16)
				t.Cleanup(func() { hub.Terminate(ctx) })
				repo, aggregate := runtimeSemanticPreflightFixture(t)
				cut := &semanticPreflightCutRepository{RuntimeStateRepository: repo, after: after, cut: errors.New("injected CAS boundary interruption")}
				cut.match = func(a runtime.RuntimeAggregate) bool {
					if len(a.CandidateRequestSessions) != 1 {
						return false
					}
					s := a.CandidateRequestSessions[0]
					switch boundary {
					case "prepared":
						return s.State == runtimecontract.RuntimeCandidateRequestSessionPrepared && !s.ProviderSubmitted
					case "submitted":
						return s.State == runtimecontract.RuntimeCandidateRequestSessionPrepared && s.ProviderSubmitted
					case "recovery_pending":
						return s.State == runtimecontract.RuntimeCandidateRequestSessionRecoveryPending
					default:
						return s.State == runtimecontract.RuntimeCandidateRequestSessionDocumentComplete
					}
				}
				module := newRuntimeSemanticPreflightModule(t, hub, cut)
				preflights, providers := 0, 0
				observer := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
				observer.Subscribe(llmevents.TopicPreflightRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
					preflights++
					if len(repo.aggregate.CandidateRequestSessions) != 0 {
						t.Error("preflight ran after session CAS")
					}
					command := event.Data().(llmevents.PreflightRuntimeCandidateCommand)
					result.Set(llmevents.PreflightRuntimeCandidateResult{DocumentKind: command.DocumentKind, Feedback: runtimePreflightFeedbackForTest(t, command, "")}, nil)
				})
				observer.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
					providers++
					command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
					session, found := runtimeCandidateRequestSession(repo.aggregate, command.RequestIdentity.LogicalRequestID)
					if !found || !session.ProviderSubmitted {
						t.Error("provider ran before submitted CAS")
					}
					if boundary == "recovery_pending" && providers == 1 {
						result.Set(runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{
							DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
							ExpectedDocumentHash: command.ExpectedDocument.Hash, DocumentOutcome: runtimeProviderFinalOutputMissingOutcome(command.ExpectedDocument),
						}), nil)
						return
					}
					document := runtimecontract.RuntimeSemanticPlanDocument{
						SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent,
						ManifestHash:  command.SemanticManifest.Hash, DefinitionSeedHash: command.SemanticManifest.DefinitionSeedHash,
						PlanStep:      runtimecontract.RuntimeSemanticPlanStepCoverageOrder,
						CoverageOrder: []common.CoverageWorkItemID{command.SemanticManifest.CoverageItems[0].ID},
					}
					payload, err := runtimecontract.EncodeRuntimeSemanticPlanPayloadWithManifest(command.SemanticManifest, document)
					if err != nil {
						t.Error(err)
						return
					}
					result.Set(runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{
						DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
						ExpectedDocumentHash: command.ExpectedDocument.Hash, DocumentOutcome: runtimeCompleteDocumentOutcome(command.ExpectedDocument, payload),
					}), nil)
				})
				started, err := module.driveRuntimeSemanticPlanning(ctx, aggregate)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := module.driveRuntimeSemanticPlanning(ctx, started); !errors.Is(err, cut.cut) || !cut.fired {
					t.Fatalf("did not hit cut point: fired=%t err=%v", cut.fired, err)
				}
				reloaded, err := cut.Load(ctx, started.Facts.WorkspaceID, started.Facts.RunID)
				if err != nil {
					t.Fatal(err)
				}
				if err := reloaded.Validate(); err != nil {
					t.Fatalf("cut point cannot reload: %v", err)
				}
				restarted := newRuntimeSemanticPreflightModule(t, hub, cut)
				resumed, err := restarted.driveRuntimeSemanticPlanning(ctx, reloaded)
				if err != nil {
					t.Fatal(err)
				}
				wantPreflights := 1
				if boundary == "prepared" && !after {
					wantPreflights = 2
				}
				unknown := boundary == "submitted" && after || (boundary == "document_complete" || boundary == "recovery_pending") && !after
				wantProviders := 1
				if boundary == "submitted" && after {
					wantProviders = 0
				}
				if boundary == "recovery_pending" && after {
					wantProviders = 2
				}
				if preflights != wantPreflights || providers != wantProviders || len(resumed.CandidateRequestSessions) != 1 || len(resumed.CandidateAttempts) != 0 || resumed.ActiveDispatch != nil {
					t.Fatalf("restart duplicated work: preflights=%d/%d providers=%d/%d sessions=%d", preflights, wantPreflights, providers, wantProviders, len(resumed.CandidateRequestSessions))
				}
				if unknown {
					if resumed.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningFailed || resumed.Facts.Failure == nil || resumed.Facts.Failure.Class != runtimecontract.FailureClassProviderSubmissionUnknown {
						t.Fatalf("unsettled call replayed: %+v", resumed.SemanticPlanning)
					}
				} else if resumed.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningOrderAccepted {
					t.Fatalf("complete document not admitted: %+v", resumed.SemanticPlanning)
				}
				if err := resumed.Validate(); err != nil {
					t.Fatal(err)
				}
				physicalIDs := map[string]bool{}
				for _, consumption := range resumed.ExecutionBudget.Consumptions {
					if consumption.Unit == "call" {
						physicalIDs[consumption.PhysicalConsumptionID] = true
					}
				}
				wantCalls := wantProviders
				if boundary == "submitted" && after {
					wantCalls = 1
				}
				if len(physicalIDs) != wantCalls {
					t.Fatalf("physical budget duplicated/lost: calls=%d want=%d", len(physicalIDs), wantCalls)
				}
			})
		}
	}
}

func TestRuntimeSemanticPreparedSessionRejectsIdentityDrift(t *testing.T) {
	for _, field := range []string{"manifest", "prefix", "correction", "expected_document", "transaction"} {
		t.Run(field, func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(16)
			t.Cleanup(func() { hub.Terminate(ctx) })
			repo, aggregate := runtimeSemanticPreflightFixture(t)
			cut := &semanticPreflightCutRepository{RuntimeStateRepository: repo, after: true, cut: errors.New("prepared response lost"),
				match: func(a runtime.RuntimeAggregate) bool { return len(a.CandidateRequestSessions) == 1 },
			}
			module := newRuntimeSemanticPreflightModule(t, hub, cut)
			preflights, providers := 0, 0
			observer := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
			observer.Subscribe(llmevents.TopicPreflightRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
				preflights++
				command := event.Data().(llmevents.PreflightRuntimeCandidateCommand)
				result.Set(llmevents.PreflightRuntimeCandidateResult{DocumentKind: command.DocumentKind, Feedback: runtimePreflightFeedbackForTest(t, command, "")}, nil)
			})
			observer.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(_ magicevent.Event, _ magicevent.Result) { providers++ })
			started, err := module.driveRuntimeSemanticPlanning(ctx, aggregate)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := module.driveRuntimeSemanticPlanning(ctx, started); !errors.Is(err, cut.cut) {
				t.Fatal(err)
			}
			session := &repo.aggregate.CandidateRequestSessions[0]
			switch field {
			case "manifest":
				session.ManifestHash = "drifted-manifest"
			case "prefix":
				session.StablePrefixHash = "drifted-prefix"
			case "correction":
				session.CorrectionHash = "drifted-correction"
			case "expected_document":
				session.ExpectedDocument.MaxOutputTokens++
				session.ExpectedDocument.Hash = runtimecontract.StableRuntimeExpectedDocumentHash(*session.ExpectedDocument)
			case "transaction":
				session.Transaction.DefinitionRevision = "drifted-definition"
			}
			beforeRevision, beforeBudget := repo.aggregate.StateRevision, repo.aggregate.ExecutionBudget.Clone()
			reloaded, err := cut.Load(ctx, started.Facts.WorkspaceID, started.Facts.RunID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := newRuntimeSemanticPreflightModule(t, hub, cut).driveRuntimeSemanticPlanning(ctx, reloaded); err == nil {
				t.Fatal("drifted session was accepted")
			}
			if preflights != 1 || providers != 0 || repo.aggregate.StateRevision != beforeRevision || !reflect.DeepEqual(beforeBudget, repo.aggregate.ExecutionBudget.Clone()) {
				t.Fatalf("drift replay caused work: preflights=%d providers=%d", preflights, providers)
			}
		})
	}
}

func runtimeSemanticPreflightFixture(t *testing.T) (*runtimeDriveRepository, runtime.RuntimeAggregate) {
	t.Helper()
	bootstrap := runtimeBootstrap()
	facts, err := bootstrap.InitialFacts("ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	repo := &runtimeDriveRepository{}
	controller, err := runtime.NewRuntimeController(repo)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := runtimeTestControllerInitialize(controller, context.Background(), facts, bootstrap.Definition, bootstrap.Requirements)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Facts.Resources.ProviderCeiling = &runtimecontract.ProviderCeiling{
		Selection:           runtimecontract.ProviderInferenceSelection{Target: aggregate.Facts.Activation.ProviderSelection.Target},
		ContextWindowTokens: 8192, ProviderMaxOutputTokens: 4096, EffectiveMaxOutputTokens: 2048,
		MaxInputTokens: 4096, MaxBytes: 65536, CapabilityRevision: "cap-1", ConfigurationRevision: "config-1",
	}
	catalog := runtimecontract.RuntimeOperationAdmissibility{PrerequisiteID: "semantic-file", OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKinds: []string{runtimecontract.StepKindFileEdit}, TargetRefs: []string{"target.txt"}, ObservedRevision: "file-revision-1"}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	aggregate.Facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	repo.aggregate = aggregate
	return repo, aggregate
}

func TestRuntimeSemanticPreflightRejectsBeforeSession(t *testing.T) {
	for _, invalidResult := range []bool{false, true} {
		t.Run(map[bool]string{false: "typed_rejection", true: "invalid_result"}[invalidResult], func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(16)
			t.Cleanup(func() { hub.Terminate(ctx) })
			repo, aggregate := runtimeSemanticPreflightFixture(t)
			module := newRuntimeSemanticPreflightModule(t, hub, repo)
			preflights, providers := 0, 0
			observer := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
			observer.Subscribe(llmevents.TopicPreflightRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
				preflights++
				if invalidResult {
					result.Set("invalid typed result", nil)
					return
				}
				command := event.Data().(llmevents.PreflightRuntimeCandidateCommand)
				result.Set(llmevents.PreflightRuntimeCandidateResult{DocumentKind: command.DocumentKind, Feedback: runtimePreflightFeedbackForTest(t, command, "protected semantic context exceeds rendered budget")}, nil)
			})
			observer.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(_ magicevent.Event, _ magicevent.Result) { providers++ })
			started, err := module.driveRuntimeSemanticPlanning(ctx, aggregate)
			if err != nil {
				t.Fatal(err)
			}
			failed, err := module.driveRuntimeSemanticPlanning(ctx, started)
			if err != nil {
				t.Fatalf("preflight did not seal semantic failure: %v", err)
			}
			if preflights != 1 || providers != 0 || len(failed.CandidateRequestSessions) != 0 || len(failed.CandidateAttempts) != 0 || failed.ActiveDispatch != nil ||
				failed.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningFailed || failed.Decision.Terminal == nil || failed.Facts.Failure == nil || failed.Facts.Failure.Class != runtimecontract.FailureClassRuntimeContractMisalignment ||
				!reflect.DeepEqual(started.ExecutionBudget, failed.ExecutionBudget) {
				t.Fatalf("preflight opened work or lost failure: preflights=%d providers=%d sessions=%d phase=%s failure=%+v", preflights, providers, len(failed.CandidateRequestSessions), failed.SemanticPlanning.Phase, failed.Facts.Failure)
			}
			restarted := newRuntimeSemanticPreflightModule(t, hub, repo)
			reloaded, err := repo.Load(ctx, failed.Facts.WorkspaceID, failed.Facts.RunID)
			if err != nil {
				t.Fatal(err)
			}
			replayed, err := restarted.driveRuntimeSemanticPlanning(ctx, reloaded)
			if err != nil || replayed.StateRevision != failed.StateRevision || providers != 0 {
				t.Fatalf("restart repeated preflight failure: revision=%d err=%v", replayed.StateRevision, err)
			}
		})
	}
}

func TestRuntimeSemanticBatchAndCorrectionPreflightRejectBeforeNewSession(t *testing.T) {
	for _, correction := range []bool{false, true} {
		t.Run(map[bool]string{false: "proof_batch", true: "correction"}[correction], func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(16)
			t.Cleanup(func() { hub.Terminate(ctx) })
			repo, aggregate := runtimeSemanticPreflightFixture(t)
			module := newRuntimeSemanticPreflightModule(t, hub, repo)
			reject, preflights, providers := false, 0, 0
			observer := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
			observer.Subscribe(llmevents.TopicPreflightRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
				preflights++
				command := event.Data().(llmevents.PreflightRuntimeCandidateCommand)
				reason := ""
				if reject {
					reason = "protected semantic batch context omitted"
				}
				result.Set(llmevents.PreflightRuntimeCandidateResult{DocumentKind: command.DocumentKind, Feedback: runtimePreflightFeedbackForTest(t, command, reason)}, nil)
			})
			observer.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
				providers++
				command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
				payload := []byte(`{}`)
				if command.SemanticContext.PlanStep == runtimecontract.RuntimeSemanticPlanStepCoverageOrder {
					document := runtimecontract.RuntimeSemanticPlanDocument{
						SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: command.SemanticManifest.Hash,
						DefinitionSeedHash: command.SemanticManifest.DefinitionSeedHash, PlanStep: runtimecontract.RuntimeSemanticPlanStepCoverageOrder,
						CoverageOrder: []common.CoverageWorkItemID{command.SemanticManifest.CoverageItems[0].ID},
					}
					var err error
					payload, err = runtimecontract.EncodeRuntimeSemanticPlanPayloadWithManifest(command.SemanticManifest, document)
					if err != nil {
						t.Error(err)
						return
					}
				}
				result.Set(runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{
					DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity, ExpectedDocumentHash: command.ExpectedDocument.Hash,
					DocumentOutcome: runtimeCompleteDocumentOutcome(command.ExpectedDocument, payload),
				}), nil)
			})
			wanted := runtimecontract.RuntimeSemanticPlanningProofBatchRequestOpen
			if correction {
				wanted = runtimecontract.RuntimeSemanticPlanningCorrectionRequired
			}
			for steps := 0; aggregate.SemanticPlanning.Phase != wanted && steps < 8; steps++ {
				var err error
				aggregate, err = module.driveRuntimeSemanticPlanning(ctx, aggregate)
				if err != nil {
					t.Fatal(err)
				}
			}
			if aggregate.SemanticPlanning.Phase != wanted {
				t.Fatalf("did not reach request intent: %s", aggregate.SemanticPlanning.Phase)
			}
			if _, exists := runtimeCandidateRequestSession(aggregate, aggregate.SemanticPlanning.LogicalRequestID); exists {
				t.Fatal("intent created session before preflight")
			}
			beforeBudget := aggregate.ExecutionBudget.Clone()
			beforeSessions, beforeProviders, beforePreflights := len(aggregate.CandidateRequestSessions), providers, preflights
			reject = true
			failed, err := newRuntimeSemanticPreflightModule(t, hub, repo).driveRuntimeSemanticPlanning(ctx, aggregate)
			if err != nil {
				t.Fatal(err)
			}
			if failed.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningFailed || len(failed.CandidateRequestSessions) != beforeSessions || providers != beforeProviders || preflights != beforePreflights+1 ||
				!reflect.DeepEqual(failed.ExecutionBudget.Clone(), beforeBudget) || failed.ActiveDispatch != nil || len(failed.CandidateAttempts) != 0 {
				t.Fatalf("rejection opened new work: sessions=%d/%d providers=%d/%d preflights=%d/%d phase=%s", len(failed.CandidateRequestSessions), beforeSessions, providers, beforeProviders, preflights, beforePreflights+1, failed.SemanticPlanning.Phase)
			}
			if err := failed.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRuntimeSemanticPreflightFailureCASRestart(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(map[bool]string{false: "before_cas", true: "response_lost"}[after], func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(16)
			t.Cleanup(func() { hub.Terminate(ctx) })
			repo, aggregate := runtimeSemanticPreflightFixture(t)
			cut := &semanticPreflightCutRepository{RuntimeStateRepository: repo, after: after, cut: errors.New("semantic failure CAS interrupted"),
				match: func(a runtime.RuntimeAggregate) bool {
					return a.SemanticPlanning.Phase == runtimecontract.RuntimeSemanticPlanningFailed
				},
			}
			module := newRuntimeSemanticPreflightModule(t, hub, cut)
			preflights, providers := 0, 0
			observer := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
			observer.Subscribe(llmevents.TopicPreflightRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
				preflights++
				command := event.Data().(llmevents.PreflightRuntimeCandidateCommand)
				persisted, err := cut.Load(ctx, command.WorkspaceID, command.RunID)
				if err != nil {
					t.Error(err)
				} else {
					progress, err := runtimecontract.BuildRuntimeSemanticCoverageProgress(command.SemanticContext, command.SemanticManifest, persisted.Facts, persisted.StateRevision)
					if err != nil || !reflect.DeepEqual(progress, command.SemanticContext.CoverageProgress) {
						t.Errorf("preflight did not use current owner progress across restart: %+v err=%v", progress, err)
					}
				}
				result.Set(llmevents.PreflightRuntimeCandidateResult{DocumentKind: command.DocumentKind, Feedback: runtimePreflightFeedbackForTest(t, command, "protected context missing")}, nil)
			})
			observer.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(_ magicevent.Event, _ magicevent.Result) { providers++ })
			started, err := module.driveRuntimeSemanticPlanning(ctx, aggregate)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := module.driveRuntimeSemanticPlanning(ctx, started); !errors.Is(err, cut.cut) {
				t.Fatalf("did not reach failure CAS: %v", err)
			}
			reloaded, err := cut.Load(ctx, started.Facts.WorkspaceID, started.Facts.RunID)
			if err != nil {
				t.Fatal(err)
			}
			restarted := newRuntimeSemanticPreflightModule(t, hub, cut)
			failed, err := restarted.driveRuntimeSemanticPlanning(ctx, reloaded)
			if err != nil {
				t.Fatal(err)
			}
			wantPreflights := 2
			if after {
				wantPreflights = 1
			}
			if preflights != wantPreflights || providers != 0 || len(failed.CandidateRequestSessions) != 0 || failed.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningFailed ||
				failed.Facts.Failure == nil || failed.Facts.Failure.OriginalDiagnostic == nil || failed.Facts.Failure.OriginalDiagnostic.OriginalCode != "candidate_context_integrity" ||
				!reflect.DeepEqual(started.ExecutionBudget.Clone(), failed.ExecutionBudget.Clone()) {
				t.Fatalf("failure restart lost exact outcome: preflights=%d providers=%d failure=%+v", preflights, providers, failed.Facts.Failure)
			}
			duplicate, err := restarted.driveRuntimeSemanticPlanning(ctx, failed)
			if err != nil || duplicate.StateRevision != failed.StateRevision || preflights != wantPreflights || providers != 0 {
				t.Fatalf("terminal replay duplicated work: %v", err)
			}
			if err := duplicate.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
