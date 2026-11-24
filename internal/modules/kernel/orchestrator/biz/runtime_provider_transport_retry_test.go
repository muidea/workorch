package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func newRuntimeTransportRetrySemanticToOpsFixture(
	t *testing.T,
	maxTransportAttempts int,
	clockFn func() time.Time,
	onRequest func(command llmevents.RequestRuntimeCandidateCommand) (llmevents.RequestRuntimeCandidateResult, error),
) (context.Context, *Biz, *runtimeDriveRepository, string) {
	t.Helper()
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(context.Background()) })

	bootstrap := runtimeSemanticProviderOrderBootstrap()
	bootstrap.Requirements = nil
	bootstrap.RecoveryPolicy.MaxProviderTransportAttempts = maxTransportAttempts
	bootstrap.ExecutionBudgetPolicy = runtimecontract.RuntimeExecutionBudgetPolicyForRuntimePolicies(bootstrap.ConvergencePolicy, bootstrap.RecoveryPolicy)
	bootstrap = runtimeTestBootstrapWithReboundBindings(bootstrap)

	facts, err := bootstrap.InitialFacts("ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	facts.Capabilities = runtimecontract.RuntimeCapabilities{
		StateChangeKinds: []string{runtimecontract.StepKindFileEdit},
	}
	facts.Resources.ProviderCeiling = &runtimecontract.ProviderCeiling{
		Selection:           runtimecontract.ProviderInferenceSelection{Target: facts.Activation.ProviderSelection.Target},
		ContextWindowTokens: 8192, ProviderMaxOutputTokens: 4096, EffectiveMaxOutputTokens: 2048,
		MaxInputTokens: 4096, MaxBytes: 65536, CapabilityRevision: "cap-1", ConfigurationRevision: "config-1",
	}
	fileCatalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "semantic-file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileEdit}, TargetRefs: []string{"target.txt"},
		OperationVariants: []runtimecontract.RuntimeOperationAdmissibilityVariant{{
			Kind:                  runtimecontract.StepKindFileEdit,
			AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExactTarget},
		}},
		ObservedRevision: "file-catalog-v1",
	}
	fileCatalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(fileCatalog)
	facts.Focus.TargetRefs = []string{"target.txt"}
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{fileCatalog}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{runtimeOperationAdmissibilityPrerequisite(fileCatalog)}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{runtimeFileAdmissibilityObservation(fileCatalog)}
	facts.Resources.TargetSnapshots = []runtimecontract.TargetSnapshot{{
		TargetRef: "target.txt", ContentHash: "semantic-target-v1", ContentState: runtimecontract.SnapshotContentStateComplete,
	}}

	repository := &runtimeDriveRepository{}
	controller, err := runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := runtimeTestControllerInitialize(controller, ctx, facts, bootstrap.Definition, bootstrap.Requirements)
	if err != nil {
		t.Fatal(err)
	}
	repository.aggregate = aggregate

	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{
			ID: aggregate.Facts.WorkspaceID, DeliverableDirectory: t.TempDir(), InternalDirectory: t.TempDir(),
		}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })

	fileObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	fileObserver.Subscribe(fileevents.TopicProbeRuntimeSemanticProofFeasibility, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(fileevents.ProbeRuntimeSemanticProofFeasibilityCommand)
		observation, obsErr := fileevents.NewRuntimeSemanticProofFeasibilityResult(
			command, runtimecontract.RuntimeSemanticFeasibilityFeasible, command.OwnerCatalog.ObservedRevision,
			append([]string(nil), command.Proposal.Target.ExactTargetRefs...), nil,
		)
		if obsErr != nil {
			t.Errorf("build semantic proof feasibility: %v", obsErr)
			return
		}
		result.Set(observation, nil)
	})
	fileObserver.Subscribe(fileevents.TopicProbeRuntimeOperationFeasibility, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(fileevents.ProbeRuntimeOperationFeasibilityCommand)
		response, respErr := fileevents.NewProbeRuntimeOperationFeasibilityResult(command, "", nil, nil, nil, nil)
		if respErr != nil {
			t.Errorf("build operation feasibility: %v", respErr)
			return
		}
		result.Set(response, nil)
	})
	t.Cleanup(func() {
		fileObserver.Unsubscribe(fileevents.TopicProbeRuntimeSemanticProofFeasibility)
		fileObserver.Unsubscribe(fileevents.TopicProbeRuntimeOperationFeasibility)
	})

	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		response, respErr := onRequest(command)
		if respErr != nil {
			result.Set(nil, runtimeOperationMagicFailure(respErr))
			return
		}
		result.Set(response, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })

	module := newTestBiz(newRuntimeTestOwner(), hub, &recordingBackgroundRoutine{})
	if err := module.ConfigureRuntimeRepository(repository); err != nil {
		t.Fatal(err)
	}
	module.nowFn = clockFn
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(aggregate.Definition, aggregate.Facts.Activation.ExecutionContract)
	return ctx, module, repository, prefix
}

func encodeTestSemanticDocumentResult(t *testing.T, command llmevents.RequestRuntimeCandidateCommand) llmevents.RequestRuntimeCandidateResult {
	t.Helper()
	document := runtimecontract.RuntimeSemanticPlanDocument{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent,
		ManifestHash:  command.SemanticManifest.Hash, DefinitionSeedHash: command.SemanticManifest.DefinitionSeedHash,
		PlanStep: command.SemanticContext.PlanStep,
	}
	switch command.SemanticContext.PlanStep {
	case runtimecontract.RuntimeSemanticPlanStepCoverageOrder:
		order := make([]common.CoverageWorkItemID, 0, len(command.SemanticManifest.CoverageItems))
		for _, item := range command.SemanticManifest.CoverageItems {
			order = append(order, item.ID)
		}
		document.CoverageOrder = order
	case runtimecontract.RuntimeSemanticPlanStepProofBatch:
		document.BatchID = command.SemanticContext.BatchID
		document.BatchSequence = command.SemanticContext.BatchSequence
		document.BatchCoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), command.SemanticContext.BatchCoverageWorkItemIDs...)
		proposals := make([]runtimecontract.RuntimeSemanticProofProposal, 0, len(document.BatchCoverageWorkItemIDs))
		for _, coverageID := range document.BatchCoverageWorkItemIDs {
			var declRef string
			for _, ob := range command.SemanticManifest.ProofObligations {
				if ob.CoverageWorkItemID == coverageID {
					declRef = runtimecontract.RuntimeSemanticCurrentDeclarationRef(ob)
					break
				}
			}
			proposals = append(proposals, runtimecontract.RuntimeSemanticProofProposal{
				CoverageWorkItemID: coverageID, OwnerModuleID: runtimecontract.RuntimeModuleFile,
				OperationKind: runtimecontract.StepKindFileEdit,
				Target: runtimecontract.RuntimeOperationTargetAuthorization{
					Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"target.txt"},
				},
				RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
				RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
				DeclarationRef:        declRef,
			})
		}
		document.Proposals = proposals
	default:
		t.Fatalf("unexpected semantic plan step %q", command.SemanticContext.PlanStep)
	}
	payload, err := runtimecontract.EncodeRuntimeSemanticPlanPayloadWithManifest(command.SemanticManifest, document)
	if err != nil {
		t.Fatal(err)
	}
	return runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{
		DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
		ExpectedDocumentHash: command.ExpectedDocument.Hash,
		DocumentOutcome:      runtimeCompleteDocumentOutcome(command.ExpectedDocument, payload),
	})
}

func driveUntilNextStopForTransportTest(
	t *testing.T,
	ctx context.Context,
	module *Biz,
	repository *runtimeDriveRepository,
	prefix string,
	runningGeneration uint64,
) (runtimeDriveTransitionOutcome, error) {
	t.Helper()
	cmd := orchestratorevents.DriveRuntimeCommand{
		WorkspaceID:             repository.aggregate.Facts.WorkspaceID,
		RunID:                   repository.aggregate.Facts.RunID,
		RecoveryLeaseGeneration: runningGeneration,
	}
	for step := 0; step < 12; step++ {
		outcome, err := module.driveOneRuntimeTransition(ctx, nil, repository.aggregate, cmd, prefix)
		if err != nil || outcome.result != nil {
			return outcome, err
		}
	}
	t.Fatalf("driveOneRuntimeTransition did not stop or yield within 12 transitions; state=%+v", repository.aggregate)
	return runtimeDriveTransitionOutcome{}, nil
}

func scheduleProviderRetryThroughSM4ForTest(
	t *testing.T,
	ctx context.Context,
	module *Biz,
	repository *runtimeDriveRepository,
	driveErr error,
	fromRunningGeneration uint64,
) runtimecontract.RuntimeDriveLease {
	t.Helper()
	var retryPending *runtimeProviderTransportRetryPendingError
	if !errors.As(driveErr, &retryPending) || retryPending == nil {
		t.Fatalf("drive error = %v, want *runtimeProviderTransportRetryPendingError", driveErr)
	}
	wsID, runID := repository.aggregate.Facts.WorkspaceID, repository.aggregate.Facts.RunID
	driveKey := runtimeDriveKey(wsID, runID)
	if _, accepted := module.beginRuntimeDriveWithToken(driveKey); !accepted {
		t.Fatalf("drive gate not acquired for %s", driveKey)
	}
	t.Cleanup(func() { module.endRuntimeDrive(driveKey) })

	request := scheduledRuntimeDriveRequest{
		workspaceID: wsID, runID: runID, source: "provider_transport_retry_test", leaseGeneration: fromRunningGeneration,
	}
	leaseReq, handled := module.handleScheduledRuntimeDriveFailure(ctx, request, driveErr)
	if !handled || leaseReq == nil {
		t.Fatalf("handleScheduledRuntimeDriveFailure handled=%v req=%+v", handled, leaseReq)
	}
	if leaseReq.Reason != runtimecontract.RuntimeDriveLeaseReasonProviderRetry {
		t.Fatalf("lease Reason = %q, want %q", leaseReq.Reason, runtimecontract.RuntimeDriveLeaseReasonProviderRetry)
	}
	if leaseReq.DueAt != retryPending.DueAt {
		t.Fatalf("lease DueAt = %q, want session TransportRetryDueAt %q", leaseReq.DueAt, retryPending.DueAt)
	}
	if fromRunningGeneration != 0 {
		leaseReq.FromRunningGeneration = fromRunningGeneration
	}
	disp := module.scheduleRuntimeRecoveryLeaseDisposition(ctx, wsID, runID, *leaseReq)
	if disp != runtimeRecoveryLeaseScheduleMounted {
		t.Fatalf("scheduleRuntimeRecoveryLeaseDisposition = %v, want mounted", disp)
	}
	module.endRuntimeDrive(driveKey)

	loaded, err := repository.Load(ctx, wsID, runID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.DriveLease == nil ||
		loaded.DriveLease.State != runtimecontract.RuntimeDriveLeaseScheduled ||
		loaded.DriveLease.Reason != runtimecontract.RuntimeDriveLeaseReasonProviderRetry ||
		loaded.DriveLease.DueAt != retryPending.DueAt {
		t.Fatalf("persisted DriveLease = %+v, want scheduled provider_retry dueAt=%q", loaded.DriveLease, retryPending.DueAt)
	}
	return loaded.DriveLease.Clone()
}

func TestRuntimeProviderTransportRetrySM4SchedulingBackoffAndPerRequestReset(t *testing.T) {
	t.Run("semantic_sm4_lease_and_reset_on_operations", func(t *testing.T) {
		var mu sync.Mutex
		clock := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
		semanticFailures := 0
		var calls []llmevents.RequestRuntimeCandidateCommand

		ctx, module, repository, prefix := newRuntimeTransportRetrySemanticToOpsFixture(
			t, runtimecontract.DefaultMaxProviderTransportAttempts,
			func() time.Time {
				mu.Lock()
				defer mu.Unlock()
				return clock
			},
			func(command llmevents.RequestRuntimeCandidateCommand) (llmevents.RequestRuntimeCandidateResult, error) {
				mu.Lock()
				calls = append(calls, command)
				mu.Unlock()
				if command.DocumentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan &&
					command.SemanticContext.PlanStep == runtimecontract.RuntimeSemanticPlanStepCoverageOrder {
					mu.Lock()
					semanticFailures++
					failNow := semanticFailures <= 2
					mu.Unlock()
					if failNow {
						return runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{
							DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
							ExpectedDocumentHash: command.ExpectedDocument.Hash,
							Feedback: runtimeProviderFeedbackForTest(t, command, runtimecontract.RuntimeCandidateFailure{
								Class: runtimecontract.RuntimeCandidateFailureClassProviderTransient, Code: "provider_http_status_503",
								Signature: "provider-http-status-503/exact", Reason: "upstream 503",
							}),
						}), nil
					}
				}
				if command.DocumentKind == runtimecontract.RuntimeProviderDocumentOperations {
					return runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{
						Transaction: command.Transaction, DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
						ExpectedDocumentHash: command.ExpectedDocument.Hash,
						Feedback: runtimeProviderFeedbackForTest(t, command, runtimecontract.RuntimeCandidateFailure{
							Class: runtimecontract.RuntimeCandidateFailureClassProviderTransient, Code: "provider_http_status_503",
							Signature: "provider-http-status-503/exact", Reason: "upstream 503",
						}),
					}), nil
				}
				return encodeTestSemanticDocumentResult(t, command), nil
			},
		)

		// Semantic failure #1 -> 2s backoff, SM-4 provider_retry lease
		_, err := driveUntilNextStopForTransportTest(t, ctx, module, repository, prefix, 0)
		lease1 := scheduleProviderRetryThroughSM4ForTest(t, ctx, module, repository, err, 0)
		wantDue1 := time.Date(2026, 11, 23, 8, 0, 2, 0, time.UTC).Format(time.RFC3339Nano)
		if lease1.DueAt != wantDue1 || repository.aggregate.Facts.Recovery.ProviderTransportAttempts != 1 {
			t.Fatalf("semantic retry #1 lease=%+v recovery=%+v, want dueAt=%s", lease1, repository.aggregate.Facts.Recovery, wantDue1)
		}

		// Simulate lease wakeup at dueAt -> ClaimRunning -> drive -> semantic failure #2 (4s backoff)
		mu.Lock()
		clock = clock.Add(2 * time.Second)
		nowText := clock.Format(time.RFC3339Nano)
		mu.Unlock()
		claimed1, ok, claimErr := module.runtimeClaimDriveLease(ctx, lease1, nowText)
		if claimErr != nil || !ok {
			t.Fatalf("claim lease1 ok=%v err=%v", ok, claimErr)
		}
		_, err = driveUntilNextStopForTransportTest(t, ctx, module, repository, prefix, claimed1.RecoveryGeneration)
		lease2 := scheduleProviderRetryThroughSM4ForTest(t, ctx, module, repository, err, claimed1.RecoveryGeneration)
		wantDue2 := time.Date(2026, 11, 23, 8, 0, 6, 0, time.UTC).Format(time.RFC3339Nano)
		if lease2.DueAt != wantDue2 || repository.aggregate.Facts.Recovery.ProviderTransportAttempts != 2 {
			t.Fatalf("semantic retry #2 lease=%+v recovery=%+v, want dueAt=%s", lease2, repository.aggregate.Facts.Recovery, wantDue2)
		}

		// Wakeup at due2 -> semantic succeeds -> advances into operations request -> operations failure #1 -> resets to 2s!
		mu.Lock()
		clock = clock.Add(4 * time.Second)
		nowText = clock.Format(time.RFC3339Nano)
		mu.Unlock()
		claimed2, ok, claimErr := module.runtimeClaimDriveLease(ctx, lease2, nowText)
		if claimErr != nil || !ok {
			t.Fatalf("claim lease2 ok=%v err=%v", ok, claimErr)
		}
		_, err = driveUntilNextStopForTransportTest(t, ctx, module, repository, prefix, claimed2.RecoveryGeneration)
		leaseOps1 := scheduleProviderRetryThroughSM4ForTest(t, ctx, module, repository, err, claimed2.RecoveryGeneration)
		wantOpsDue := time.Date(2026, 11, 23, 8, 0, 8, 0, time.UTC).Format(time.RFC3339Nano)
		if leaseOps1.DueAt != wantOpsDue || repository.aggregate.Facts.Recovery.ProviderTransportAttempts != 3 {
			t.Fatalf("operations retry after semantic reset lease=%+v recovery=%+v, want dueAt=%s attempts=3", leaseOps1, repository.aggregate.Facts.Recovery, wantOpsDue)
		}
	})

	t.Run("operations_exponential_backoff_progression_2s_to_60s", func(t *testing.T) {
		ctx := context.Background()
		hub := magicevent.NewHub(16)
		t.Cleanup(func() { hub.Terminate(context.Background()) })
		startRuntimeCandidateFileEditAdmissionOwners(t, hub)

		facts := runtimeCandidateSealFileEditFacts(t)
		state := &inMemoryRuntimeStateRepository{}
		controller, err := runtime.NewRuntimeController(state)
		if err != nil {
			t.Fatal(err)
		}
		initialized, err := runtimeTestControllerInitialize(controller, ctx, facts, runtimeDefinition(), nil)
		if err != nil {
			t.Fatal(err)
		}
		initialized = runtimeTestPersistSealedSemanticAuthority(t, ctx, state, initialized)
		owner := newRuntimeTestOwnerWithEvidence(RuntimeEvidenceRepositories{ArtifactContinuations: memory.NewArtifactContinuationRepository()})
		if err := owner.ConfigureRuntimeRepository(state); err != nil {
			t.Fatal(err)
		}
		clock := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
		module := newTestBiz(owner, hub, &recordingBackgroundRoutine{})
		module.nowFn = func() time.Time { return clock }
		prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(initialized.Definition, initialized.Facts.Activation.ExecutionContract)

		llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
		llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
			command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
			result.Set(runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{
				Transaction: command.Transaction, DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
				ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument),
				Feedback: runtimeProviderFeedbackForTest(t, command, runtimecontract.RuntimeCandidateFailure{
					Class: runtimecontract.RuntimeCandidateFailureClassProviderTransient, Code: "provider_http_status_503",
					Signature: "provider-http-status-503/exact", Reason: "upstream 503",
				}),
			}), nil)
		})
		t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })

		wantDeltas := []time.Duration{
			2 * time.Second,
			4 * time.Second,
			8 * time.Second,
			16 * time.Second,
			32 * time.Second,
			60 * time.Second,
		}
		cmd := orchestratorevents.DriveRuntimeCommand{WorkspaceID: initialized.Facts.WorkspaceID, RunID: initialized.Facts.RunID}
		for idx, wantDelta := range wantDeltas {
			cur, loadErr := state.Load(ctx, cmd.WorkspaceID, cmd.RunID)
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			_, driveErr := module.driveOneRuntimeTransition(ctx, nil, cur, cmd, prefix)
			var pending *runtimeProviderTransportRetryPendingError
			if !errors.As(driveErr, &pending) || pending == nil {
				t.Fatalf("attempt %d err=%v, want *runtimeProviderTransportRetryPendingError", idx+1, driveErr)
			}
			dueAt, parseErr := time.Parse(time.RFC3339Nano, pending.DueAt)
			if parseErr != nil {
				t.Fatal(parseErr)
			}
			if gotDelta := dueAt.Sub(clock); gotDelta != wantDelta {
				t.Fatalf("operations attempt %d backoff=%v, want %v", idx+1, gotDelta, wantDelta)
			}
			leaseReq, handled := module.handleScheduledRuntimeDriveFailure(ctx, scheduledRuntimeDriveRequest{
				workspaceID: cmd.WorkspaceID, runID: cmd.RunID, source: "ops-backoff",
			}, driveErr)
			if !handled || leaseReq == nil || leaseReq.Reason != runtimecontract.RuntimeDriveLeaseReasonProviderRetry || leaseReq.DueAt != pending.DueAt {
				t.Fatalf("attempt %d leaseReq=%+v handled=%v, want provider_retry dueAt=%s", idx+1, leaseReq, handled, pending.DueAt)
			}
			clock = dueAt
		}
	})
}

func TestRuntimeProviderTransportRetryRunWideExhaustionAcrossDistinctRequests(t *testing.T) {
	var mu sync.Mutex
	clock := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	semanticFailed := false
	diagnostic := runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassProviderTransient, Code: "provider_http_status_503",
		Signature: "provider-http-status-503/exact", Reason: "upstream 503 cross-request",
	}

	ctx, module, repository, prefix := newRuntimeTransportRetrySemanticToOpsFixture(
		t, 2,
		func() time.Time {
			mu.Lock()
			defer mu.Unlock()
			return clock
		},
		func(command llmevents.RequestRuntimeCandidateCommand) (llmevents.RequestRuntimeCandidateResult, error) {
			if command.DocumentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan {
				mu.Lock()
				failFirst := !semanticFailed
				semanticFailed = true
				mu.Unlock()
				if failFirst {
					return runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{
						DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
						ExpectedDocumentHash: command.ExpectedDocument.Hash,
						Feedback:             runtimeProviderFeedbackForTest(t, command, diagnostic),
					}), nil
				}
				return encodeTestSemanticDocumentResult(t, command), nil
			}
			return runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{
				Transaction: command.Transaction, DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
				ExpectedDocumentHash: command.ExpectedDocument.Hash,
				Feedback:             runtimeProviderFeedbackForTest(t, command, diagnostic),
			}), nil
		},
	)

	// 1) Semantic-plan fails once -> consumes Run attempt 1/2 -> yields 2s
	_, err := driveUntilNextStopForTransportTest(t, ctx, module, repository, prefix, 0)
	var pending *runtimeProviderTransportRetryPendingError
	if !errors.As(err, &pending) {
		t.Fatalf("expected *runtimeProviderTransportRetryPendingError after semantic transient failure, got %v", err)
	}
	if repository.aggregate.Facts.Recovery.ProviderTransportAttempts != 1 {
		t.Fatalf("attempts = %d, want 1", repository.aggregate.Facts.Recovery.ProviderTransportAttempts)
	}

	// 2) Advance to dueAt -> semantic succeeds -> operations failure #1 -> consumes Run attempt 2/2 -> yields 2s
	mu.Lock()
	clock = clock.Add(2 * time.Second)
	mu.Unlock()
	_, err = driveUntilNextStopForTransportTest(t, ctx, module, repository, prefix, 0)
	if !errors.As(err, &pending) {
		t.Fatalf("expected *runtimeProviderTransportRetryPendingError after first operations failure, got %v", err)
	}
	if repository.aggregate.Facts.Recovery.ProviderTransportAttempts != 2 {
		t.Fatalf("attempts = %d, want 2", repository.aggregate.Facts.Recovery.ProviderTransportAttempts)
	}

	// 3) Advance to dueAt -> operations failure #2 exhausts Run-wide cap (2) despite per-request TransportOutcomes == 2 <= 8
	mu.Lock()
	clock = clock.Add(2 * time.Second)
	mu.Unlock()
	outcome, err := driveUntilNextStopForTransportTest(t, ctx, module, repository, prefix, 0)
	if err != nil || outcome.result == nil {
		t.Fatalf("expected terminal result on Run-wide exhaustion, got outcome=%+v err=%v", outcome, err)
	}
	final := repository.aggregate
	if err := final.Validate(); err != nil {
		t.Fatalf("final aggregate Validate: %v", err)
	}
	if final.Decision.State != runtimecontract.ConvergenceStateFailed ||
		final.Facts.Failure == nil ||
		final.Facts.Failure.Class != runtimecontract.FailureClassProviderTransient ||
		final.Facts.Failure.Code != "provider_transport_exhausted" ||
		final.Facts.Failure.OriginalDiagnostic == nil ||
		final.Facts.Failure.OriginalDiagnostic.OriginalCode != diagnostic.Code ||
		final.Facts.Failure.OriginalDiagnostic.Signature != diagnostic.Signature ||
		final.Facts.Recovery.ProviderTransportAttempts != 2 {
		t.Fatalf("Run-wide exhaustion terminal mismatch: decision=%+v failure=%+v recovery=%+v", final.Decision, final.Facts.Failure, final.Facts.Recovery)
	}
	var opsSession runtimecontract.RuntimeCandidateRequestSession
	for _, s := range final.CandidateRequestSessions {
		if s.DocumentKind == runtimecontract.RuntimeProviderDocumentOperations {
			opsSession = s
			break
		}
	}
	if opsSession.State != runtimecontract.RuntimeCandidateRequestSessionTerminal ||
		opsSession.TransportRetryDueAt != "" ||
		opsSession.TransportOutcomes != 2 {
		t.Fatalf("operations session after Run-wide exhaustion = %+v", opsSession)
	}
}

func TestRuntimeProviderTransportRetryCrashBeforeLeaseSettlesProviderRetryLease(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	startRuntimeCandidateFileEditAdmissionOwners(t, hub)

	facts := runtimeCandidateSealFileEditFacts(t)
	state := &inMemoryRuntimeStateRepository{}
	controller, err := runtime.NewRuntimeController(state)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := runtimeTestControllerInitialize(controller, ctx, facts, runtimeDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	initialized = runtimeTestPersistSealedSemanticAuthority(t, ctx, state, initialized)
	owner := newRuntimeTestOwnerWithEvidence(RuntimeEvidenceRepositories{ArtifactContinuations: memory.NewArtifactContinuationRepository()})
	if err := owner.ConfigureRuntimeRepository(state); err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	module := newTestBiz(owner, hub, &recordingBackgroundRoutine{})
	module.nowFn = func() time.Time { return clock }
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(initialized.Definition, initialized.Facts.Activation.ExecutionContract)

	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		result.Set(runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{
			Transaction: command.Transaction, DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
			ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument),
			Feedback: runtimeProviderFeedbackForTest(t, command, runtimecontract.RuntimeCandidateFailure{
				Class: runtimecontract.RuntimeCandidateFailureClassProviderTransient, Code: "provider_http_status_503",
				Signature: "provider-http-status-503/exact", Reason: "upstream 503",
			}),
		}), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })

	// Persist TransportPending without DriveLease (simulating crash right after TransportPending CAS).
	_, err = module.requestAndSealRuntimeCandidateWithRetry(ctx, initialized, prefix)
	if err != nil {
		t.Fatal(err)
	}
	crashed, err := state.Load(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if crashed.DriveLease != nil || len(crashed.CandidateRequestSessions) != 1 || crashed.CandidateRequestSessions[0].State != runtimecontract.RuntimeCandidateRequestSessionTransportPending {
		t.Fatalf("unexpected pre-lease crashed state: %+v", crashed)
	}
	wantDueAt := crashed.CandidateRequestSessions[0].TransportRetryDueAt

	driveKey := runtimeDriveKey(crashed.Facts.WorkspaceID, crashed.Facts.RunID)
	gateToken, accepted := module.beginRuntimeDriveWithToken(driveKey)
	if !accepted {
		t.Fatal("failed to acquire test drive gate")
	}
	defer module.endRuntimeDrive(driveKey)
	gate, _ := module.currentRuntimeDriveGate(driveKey)
	if gate.Token != gateToken {
		t.Fatalf("unexpected gate token %d != %d", gate.Token, gateToken)
	}
	mounted := module.settleActionableRuntimeWithoutDriveLease(ctx, crashed, runtimeDriveGateSettlementRequest{
		WorkspaceID: crashed.Facts.WorkspaceID, RunID: crashed.Facts.RunID, DriveKey: driveKey,
		GateEpoch: gate.Epoch, GateToken: gate.Token, Source: "crash-before-lease-test",
	})
	if !mounted {
		t.Fatal("settleActionableRuntimeWithoutDriveLease returned false")
	}
	recovered, err := state.Load(ctx, crashed.Facts.WorkspaceID, crashed.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.DriveLease == nil ||
		recovered.DriveLease.State != runtimecontract.RuntimeDriveLeaseScheduled ||
		recovered.DriveLease.Reason != runtimecontract.RuntimeDriveLeaseReasonProviderRetry ||
		recovered.DriveLease.DueAt != wantDueAt {
		t.Fatalf("settled DriveLease = %+v, want provider_retry with dueAt=%s", recovered.DriveLease, wantDueAt)
	}
}

func TestRuntimeProviderTransportRetryEarlyWakeupMakesZeroProviderCallsAndZeroBudgetReserve(t *testing.T) {
	clock := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	providerCalls := 0
	var mu sync.Mutex

	ctx, module, repository, prefix := newRuntimeTransportRetrySemanticToOpsFixture(
		t, runtimecontract.DefaultMaxProviderTransportAttempts,
		func() time.Time {
			mu.Lock()
			defer mu.Unlock()
			return clock
		},
		func(command llmevents.RequestRuntimeCandidateCommand) (llmevents.RequestRuntimeCandidateResult, error) {
			mu.Lock()
			providerCalls++
			mu.Unlock()
			return runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{
				DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
				ExpectedDocumentHash: command.ExpectedDocument.Hash,
				Feedback: runtimeProviderFeedbackForTest(t, command, runtimecontract.RuntimeCandidateFailure{
					Class: runtimecontract.RuntimeCandidateFailureClassProviderTransient, Code: "provider_timeout",
					Signature: "provider-timeout", Reason: "timed out",
				}),
			}), nil
		},
	)

	// 1. Semantic-plan early wakeup before DueAt -> zero calls, zero budget, zero revision bump.
	_, err := driveUntilNextStopForTransportTest(t, ctx, module, repository, prefix, 0)
	var pending *runtimeProviderTransportRetryPendingError
	if !errors.As(err, &pending) {
		t.Fatalf("expected *runtimeProviderTransportRetryPendingError on semantic transient, got %v", err)
	}
	semRev := repository.aggregate.StateRevision
	semAllocs := len(repository.aggregate.ExecutionBudget.Allocations)
	mu.Lock()
	clock = clock.Add(1500 * time.Millisecond)
	mu.Unlock()
	_, err = driveUntilNextStopForTransportTest(t, ctx, module, repository, prefix, 0)
	if !errors.As(err, &pending) {
		t.Fatalf("semantic early wakeup err=%v, want *runtimeProviderTransportRetryPendingError", err)
	}
	if providerCalls != 1 || repository.aggregate.StateRevision != semRev || len(repository.aggregate.ExecutionBudget.Allocations) != semAllocs {
		t.Fatalf("semantic early wakeup mutated state or called provider: calls=%d rev=%d/%d allocs=%d/%d",
			providerCalls, repository.aggregate.StateRevision, semRev, len(repository.aggregate.ExecutionBudget.Allocations), semAllocs)
	}

	// 2. Operations-candidate early wakeup before DueAt -> zero calls, zero budget, zero revision bump.
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	startRuntimeCandidateFileEditAdmissionOwners(t, hub)
	facts := runtimeCandidateSealFileEditFacts(t)
	opsState := &inMemoryRuntimeStateRepository{}
	opsController, err := runtime.NewRuntimeController(opsState)
	if err != nil {
		t.Fatal(err)
	}
	opsInit, err := runtimeTestControllerInitialize(opsController, ctx, facts, runtimeDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	opsInit = runtimeTestPersistSealedSemanticAuthority(t, ctx, opsState, opsInit)
	opsOwner := newRuntimeTestOwnerWithEvidence(RuntimeEvidenceRepositories{ArtifactContinuations: memory.NewArtifactContinuationRepository()})
	if err := opsOwner.ConfigureRuntimeRepository(opsState); err != nil {
		t.Fatal(err)
	}
	opsClock := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	opsModule := newTestBiz(opsOwner, hub, &recordingBackgroundRoutine{})
	opsModule.nowFn = func() time.Time { return opsClock }
	opsPrefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(opsInit.Definition, opsInit.Facts.Activation.ExecutionContract)
	opsCalls := 0
	opsLLM := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	opsLLM.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		opsCalls++
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		result.Set(runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{
			Transaction: command.Transaction, DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
			ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument),
			Feedback: runtimeProviderFeedbackForTest(t, command, runtimecontract.RuntimeCandidateFailure{
				Class: runtimecontract.RuntimeCandidateFailureClassProviderTransient, Code: "provider_timeout",
				Signature: "provider-timeout", Reason: "timed out",
			}),
		}), nil)
	})
	t.Cleanup(func() { opsLLM.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })

	cmd := orchestratorevents.DriveRuntimeCommand{WorkspaceID: opsInit.Facts.WorkspaceID, RunID: opsInit.Facts.RunID}
	_, err = opsModule.driveOneRuntimeTransition(ctx, nil, opsInit, cmd, opsPrefix)
	if !errors.As(err, &pending) {
		t.Fatalf("expected *runtimeProviderTransportRetryPendingError on ops transient, got %v", err)
	}
	persisted, loadErr := opsState.Load(ctx, opsInit.Facts.WorkspaceID, opsInit.Facts.RunID)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	opsRev := persisted.StateRevision
	opsAllocs := len(persisted.ExecutionBudget.Allocations)
	opsClock = opsClock.Add(time.Second)
	_, err = opsModule.driveOneRuntimeTransition(ctx, nil, persisted, cmd, opsPrefix)
	if !errors.As(err, &pending) {
		t.Fatalf("ops early wakeup err=%v, want *runtimeProviderTransportRetryPendingError", err)
	}
	afterEarly, loadErr := opsState.Load(ctx, opsInit.Facts.WorkspaceID, opsInit.Facts.RunID)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if opsCalls != 1 || afterEarly.StateRevision != opsRev || len(afterEarly.ExecutionBudget.Allocations) != opsAllocs {
		t.Fatalf("ops early wakeup mutated state or called provider: calls=%d rev=%d/%d allocs=%d/%d",
			opsCalls, afterEarly.StateRevision, opsRev, len(afterEarly.ExecutionBudget.Allocations), opsAllocs)
	}
}

func TestRuntimeProviderTransportRetryCrashWhileSubmittedResolvesSubmissionUnknown(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	startRuntimeCandidateFileEditAdmissionOwners(t, hub)

	facts := runtimeCandidateSealFileEditFacts(t)
	state := &inMemoryRuntimeStateRepository{}
	controller, err := runtime.NewRuntimeController(state)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := runtimeTestControllerInitialize(controller, ctx, facts, runtimeDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	initialized = runtimeTestPersistSealedSemanticAuthority(t, ctx, state, initialized)
	owner := newRuntimeTestOwnerWithEvidence(RuntimeEvidenceRepositories{ArtifactContinuations: memory.NewArtifactContinuationRepository()})
	if err := owner.ConfigureRuntimeRepository(state); err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	module := newTestBiz(owner, hub, &recordingBackgroundRoutine{})
	module.nowFn = func() time.Time { return clock }
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(initialized.Definition, initialized.Facts.Activation.ExecutionContract)

	providerCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		providerCalls++
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		result.Set(runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{
			Transaction: command.Transaction, DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
			ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument),
			Feedback: runtimeProviderFeedbackForTest(t, command, runtimecontract.RuntimeCandidateFailure{
				Class: runtimecontract.RuntimeCandidateFailureClassProviderTransient, Code: "provider_timeout",
				Signature: "provider-timeout", Reason: "timed out",
			}),
		}), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })

	// Attempt 1 -> TransportPending
	_, err = module.requestAndSealRuntimeCandidateWithRetry(ctx, initialized, prefix)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := state.Load(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	session := pending.CandidateRequestSessions[0]

	// Advance clock to DueAt and submit sequence 2, then simulate crash before provider outcome arrives.
	dueAt, parseErr := time.Parse(time.RFC3339Nano, session.TransportRetryDueAt)
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	clock = dueAt
	call2 := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, session.NextSequence)
	submitted, err := module.runtimeCandidateSession(ctx, pending, runtime.RuntimeInternalEventCandidateRequestSubmitted, session.LogicalRequestID, session.NextSequence, call2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !submitted.CandidateRequestSessions[0].ProviderSubmitted {
		t.Fatalf("sequence 2 submit did not mark ProviderSubmitted=true: %+v", submitted.CandidateRequestSessions[0])
	}

	restartedOwner := newRuntimeTestOwnerWithEvidence(RuntimeEvidenceRepositories{ArtifactContinuations: owner.evidence.ArtifactContinuations})
	if err := restartedOwner.ConfigureRuntimeRepository(state); err != nil {
		t.Fatal(err)
	}
	restarted := newTestBiz(restartedOwner, hub, &recordingBackgroundRoutine{})
	restarted.nowFn = func() time.Time { return clock }

	settled, changed, settleErr := restarted.settleRuntimeSubmittedProviderBeforeObservation(ctx, submitted)
	if settleErr != nil || !changed {
		t.Fatalf("settleRuntimeSubmittedProviderBeforeObservation changed=%v err=%v", changed, settleErr)
	}
	if err := settled.Validate(); err != nil {
		t.Fatalf("settled submission-unknown aggregate Validate: %v", err)
	}
	if providerCalls != 1 ||
		settled.Decision.State != runtimecontract.ConvergenceStateFailed ||
		settled.Facts.Failure == nil ||
		settled.Facts.Failure.Code != "provider_submission_unknown" ||
		settled.CandidateRequestSessions[0].State != runtimecontract.RuntimeCandidateRequestSessionTerminal ||
		settled.CandidateRequestSessions[0].TransportRetryDueAt != "" ||
		settled.CandidateRequestSessions[0].ProviderSubmitted {
		t.Fatalf("submission-unknown after TransportPending submit = calls=%d aggregate=%+v", providerCalls, settled)
	}
	unknownCount := 0
	for _, alloc := range settled.ExecutionBudget.Allocations {
		if alloc.ScopeRef == call2 && alloc.State == runtimecontract.RuntimeBudgetAllocationUnknown {
			unknownCount++
		}
	}
	if unknownCount == 0 {
		t.Fatalf("submitted call2 budget allocations did not settle to unknown: %+v", settled.ExecutionBudget.Allocations)
	}
}

func TestRuntimeProviderTransportRetryDuplicateRecoveryPreservesOriginalDueAt(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	startRuntimeCandidateFileEditAdmissionOwners(t, hub)

	facts := runtimeCandidateSealFileEditFacts(t)
	state := &inMemoryRuntimeStateRepository{}
	controller, err := runtime.NewRuntimeController(state)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := runtimeTestControllerInitialize(controller, ctx, facts, runtimeDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	initialized = runtimeTestPersistSealedSemanticAuthority(t, ctx, state, initialized)
	owner := newRuntimeTestOwnerWithEvidence(RuntimeEvidenceRepositories{ArtifactContinuations: memory.NewArtifactContinuationRepository()})
	if err := owner.ConfigureRuntimeRepository(state); err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(owner, hub, &recordingBackgroundRoutine{})
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(initialized.Definition, initialized.Facts.Activation.ExecutionContract)
	manifest, err := runtime.BuildRuntimeCandidateContractManifestForAggregate(initialized)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := runtimeOperationsExpectedDocument(initialized, manifest, 0)
	if err != nil {
		t.Fatal(err)
	}
	logicalID := runtimecontract.StableRuntimeCandidateRequestSessionID(runtimecontract.RuntimeProviderDocumentOperations, initialized.Facts.WorkspaceID, initialized.Facts.RunID, initialized.Decision.AuthorityTransaction(), prefix, manifest.Hash, "")
	expected.LogicalRequestID = logicalID
	expected.StablePrefixHash = prefix
	expected.Hash = runtimecontract.StableRuntimeExpectedDocumentHash(expected)

	_, session, err := module.runtimeEnsureCandidateRequestWithExpected(ctx, initialized, runtimecontract.RuntimeProviderDocumentOperations, prefix, manifest.Hash, "", expected)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := state.Load(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	session = loaded.CandidateRequestSessions[0]
	call1 := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, session.NextSequence)
	submitted, err := module.runtimeCandidateSession(ctx, loaded, runtime.RuntimeInternalEventCandidateRequestSubmitted, session.LogicalRequestID, session.NextSequence, call1, nil)
	if err != nil {
		t.Fatal(err)
	}

	diagnostic := runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
		OwnerModuleID: runtimecontract.RuntimeModuleLLM,
		OriginalClass: string(runtimecontract.RuntimeCandidateFailureClassProviderTransient),
		OriginalCode:  "provider_http_status_503", OriginalMessage: "upstream 503",
		Signature: "provider-http-status-503/exact",
	}
	outcome := runtimecontract.RuntimeCandidateRequestOutcome{
		Class: runtimecontract.RuntimeCandidateRequestOutcomeProviderTransient,
		Code:  diagnostic.OriginalCode, Signature: diagnostic.Signature,
	}
	audit := runtimeProviderResultAuditForTest(llmevents.RequestRuntimeCandidateCommand{
		WorkspaceID:  initialized.Facts.WorkspaceID,
		RunID:        initialized.Facts.RunID,
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		RequestIdentity: runtimecontract.RuntimeCandidateRequestIdentity{
			LogicalRequestID: session.LogicalRequestID,
			Sequence:         session.NextSequence,
			PhysicalCallID:   call1,
		},
		StablePrefixHash: prefix,
	}, llmevents.RequestRuntimeCandidateResult{})
	audit.Status = "failed"
	settlement, err := runtimeProviderBudgetSettlement(string(initialized.Facts.WorkspaceID), string(initialized.Facts.RunID), session.LogicalRequestID, call1, audit, nil)
	if err != nil {
		t.Fatal(err)
	}
	firstObservedAt := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	first, err := module.runtimeCoordinateProviderRequestRecovery(ctx, submitted, session.LogicalRequestID, call1, runtime.RuntimeProviderRequestRecoveryInput{
		DocumentKind:     runtimecontract.RuntimeProviderDocumentOperations,
		TransportOutcome: &outcome, Message: diagnostic.OriginalMessage, OriginalDiagnostic: &diagnostic,
		ObservedAt: firstObservedAt,
	}, settlement)
	if err != nil {
		t.Fatal(err)
	}
	wantDueAt := time.Date(2026, 11, 23, 8, 0, 2, 0, time.UTC).Format(time.RFC3339Nano)
	if first.CandidateRequestSessions[0].TransportRetryDueAt != wantDueAt {
		t.Fatalf("first TransportRetryDueAt = %q, want %q", first.CandidateRequestSessions[0].TransportRetryDueAt, wantDueAt)
	}

	laterObservedAt := time.Date(2026, 11, 23, 8, 0, 9, 0, time.UTC).Format(time.RFC3339Nano)
	replayed, err := module.runtimeCoordinateProviderRequestRecovery(ctx, first, session.LogicalRequestID, call1, runtime.RuntimeProviderRequestRecoveryInput{
		DocumentKind:     runtimecontract.RuntimeProviderDocumentOperations,
		TransportOutcome: &outcome, Message: diagnostic.OriginalMessage, OriginalDiagnostic: &diagnostic,
		ObservedAt: laterObservedAt,
	}, settlement)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.StateRevision != first.StateRevision || replayed.CandidateRequestSessions[0].TransportRetryDueAt != wantDueAt {
		t.Fatalf("duplicate recovery mutated revision or dueAt: rev=%d/%d dueAt=%q/%q",
			replayed.StateRevision, first.StateRevision, replayed.CandidateRequestSessions[0].TransportRetryDueAt, wantDueAt)
	}
}
