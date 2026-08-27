package llm

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeLLMOwnerCommandsRequireExactOrchestratorSource(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	provider := &blockingSubmittedCandidateProvider{}
	owner, err := New(hub, nil, Dependencies{CandidateProvider: provider})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		topic   string
		payload any
		handler func(magicevent.Event, magicevent.Result)
	}{
		{name: "provider ceiling", topic: llmevents.TopicAcquireProviderCeiling, payload: llmevents.AcquireProviderCeilingCommand{}, handler: owner.handleProviderCeilingPrerequisite},
		{name: "candidate preflight", topic: llmevents.TopicPreflightRuntimeCandidate, payload: llmevents.PreflightRuntimeCandidateCommand{}, handler: owner.handleRuntimeCandidatePreflight},
		{name: "candidate request", topic: llmevents.TopicRequestRuntimeCandidate, payload: llmevents.RequestRuntimeCandidateCommand{}, handler: owner.handleRuntimeCandidate},
	}
	for _, tc := range tests {
		t.Run(tc.name+" nil event", func(t *testing.T) {
			result := magicevent.NewResult(tc.topic, "", llmevents.CommandDestination)
			tc.handler(nil, result)
			_, gotErr := result.Get()
			if gotErr == nil || !strings.Contains(gotErr.Error(), "exact Orchestrator source") {
				t.Fatalf("nil-event error = %v", gotErr)
			}
		})
		for _, source := range []string{"application/workorchd", " kernel/orchestrator "} {
			t.Run(tc.name+" wrong source "+source, func(t *testing.T) {
				event := magicevent.NewEventWithContext(tc.topic, source, llmevents.CommandDestination, magicevent.NewHeader(), ctx, tc.payload)
				result := magicevent.NewResult(tc.topic, source, llmevents.CommandDestination)
				tc.handler(event, result)
				_, gotErr := result.Get()
				if gotErr == nil || !strings.Contains(gotErr.Error(), "exact Orchestrator source") {
					t.Fatalf("wrong-source error = %v", gotErr)
				}
			})
		}
	}
	if provider.calls.Load() != 0 {
		t.Fatalf("wrong-source commands reached candidate provider %d times", provider.calls.Load())
	}
}

type blockingSubmittedCandidateProvider struct {
	calls       atomic.Int32
	startedOnce sync.Once
	started     chan struct{}
	release     chan struct{}
}

func (p *blockingSubmittedCandidateProvider) GenerateCandidate(ctx context.Context, request llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
	p.calls.Add(1)
	if p.started != nil {
		p.startedOnce.Do(func() { close(p.started) })
	}
	if p.release != nil {
		select {
		case <-p.release:
		case <-ctx.Done():
			return llmmodels.RuntimeCandidateGenerationResult{}, ctx.Err()
		}
	}
	providerRequest, err := runtimeProviderRequest(request)
	if err != nil {
		return llmmodels.RuntimeCandidateGenerationResult{}, err
	}
	budget := cloneProviderContextBudget(providerRequest.ContextBudget)
	if budget != nil {
		budget.ProviderSubmitted = true
	}
	now := time.Now().UTC()
	audit := sealedAuditFromRequest(
		request,
		request.RequestIdentity.PhysicalCallID,
		budget,
		now,
		now,
		true,
		llmmodels.RuntimeCandidateStatusFailed,
		string(runtimecontract.RuntimeCandidateFailureClassProviderTransient),
		0,
	)
	audit.RequestBytes = max(audit.RequestBytes, 1)
	return llmmodels.RuntimeCandidateGenerationResult{
		RequestID:     request.RequestIdentity.PhysicalCallID,
		Audit:         audit,
		Status:        llmmodels.RuntimeCandidateStatusFailed,
		FailureClass:  string(runtimecontract.RuntimeCandidateFailureClassProviderTransient),
		FailureReason: "typed provider transport failure",
	}, &ProviderTransportError{Code: "network_timeout"}
}

func TestRuntimePhysicalSubmissionGuardRejectsConcurrentAndDuplicateIdentity(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	provider := &blockingSubmittedCandidateProvider{started: make(chan struct{}), release: make(chan struct{})}
	owner, err := New(hub, nil, Dependencies{CandidateProvider: provider})
	if err != nil {
		t.Fatal(err)
	}
	request := runtimeSemanticRequestWithExpectedDocument(t)
	command := runtimeCandidateCommandFromRequest(request)
	event := func(command llmevents.RequestRuntimeCandidateCommand) magicevent.Event {
		return magicevent.NewEventWithContext(
			llmevents.TopicRequestRuntimeCandidate,
			runtimecontract.RuntimeModuleOrchestrator,
			llmevents.CommandDestination,
			magicevent.NewHeader(),
			ctx,
			command,
		)
	}

	firstResult := magicevent.NewResult(llmevents.TopicRequestRuntimeCandidate, runtimecontract.RuntimeModuleOrchestrator, llmevents.CommandDestination)
	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		owner.handleRuntimeCandidate(event(command), firstResult)
	}()
	select {
	case <-provider.started:
	case <-time.After(2 * time.Second):
		t.Fatal("first physical submission did not start")
	}

	concurrentResult := magicevent.NewResult(llmevents.TopicRequestRuntimeCandidate, runtimecontract.RuntimeModuleOrchestrator, llmevents.CommandDestination)
	owner.handleRuntimeCandidate(event(command), concurrentResult)
	if _, gotErr := concurrentResult.Get(); gotErr == nil || !strings.Contains(gotErr.Error(), "already reserved or submitted") {
		t.Fatalf("concurrent duplicate error = %v", gotErr)
	}
	if provider.calls.Load() != 1 {
		t.Fatalf("concurrent duplicate provider calls = %d, want 1", provider.calls.Load())
	}

	close(provider.release)
	select {
	case <-firstDone:
	case <-time.After(2 * time.Second):
		t.Fatal("first physical submission did not finish")
	}
	if _, gotErr := firstResult.Get(); gotErr != nil {
		t.Fatalf("first physical result error = %v", gotErr)
	}

	duplicateResult := magicevent.NewResult(llmevents.TopicRequestRuntimeCandidate, runtimecontract.RuntimeModuleOrchestrator, llmevents.CommandDestination)
	owner.handleRuntimeCandidate(event(command), duplicateResult)
	if _, gotErr := duplicateResult.Get(); gotErr == nil || !strings.Contains(gotErr.Error(), "already reserved or submitted") {
		t.Fatalf("completed duplicate error = %v", gotErr)
	}
	if provider.calls.Load() != 1 {
		t.Fatalf("completed duplicate provider calls = %d, want 1", provider.calls.Load())
	}

	different := command
	different.RequestIdentity.PhysicalCallID = runtimecontract.StableRuntimeCandidatePhysicalCallID(different.RequestIdentity.LogicalRequestID, 2)
	different.RequestIdentity.Sequence = 2
	differentResult := magicevent.NewResult(llmevents.TopicRequestRuntimeCandidate, runtimecontract.RuntimeModuleOrchestrator, llmevents.CommandDestination)
	owner.handleRuntimeCandidate(event(different), differentResult)
	if _, gotErr := differentResult.Get(); gotErr != nil {
		t.Fatalf("different physical identity error = %v", gotErr)
	}
	if provider.calls.Load() != 2 {
		t.Fatalf("different physical identity provider calls = %d, want 2", provider.calls.Load())
	}
}

func TestRuntimePhysicalSubmissionGuardIsBoundedFailClosedAndClearable(t *testing.T) {
	guard := newRuntimePhysicalSubmissionGuard(1)
	first := runtimePhysicalSubmissionIdentity{WorkspaceID: "ws-1", RunID: "run-1", LogicalRequestID: "logical-1", Sequence: 1, PhysicalCallID: "physical-1"}
	second := runtimePhysicalSubmissionIdentity{WorkspaceID: "ws-1", RunID: "run-1", LogicalRequestID: "logical-2", Sequence: 1, PhysicalCallID: "physical-2"}
	if err := guard.reserve(first); err != nil {
		t.Fatal(err)
	}
	if err := guard.reserve(second); err == nil || !isRuntimePhysicalSubmissionGuardError(err) || !errors.Is(err, common.ErrConflict) {
		t.Fatalf("bounded capacity error = %v", err)
	}
	guard.releaseIfProvenUnsubmitted(first, unsubmittedRuntimeGeneration(first))
	if err := guard.reserve(second); err != nil {
		t.Fatalf("exact unsubmitted fact did not release guard entry: %v", err)
	}
	guard.clear()
	if err := guard.reserve(second); err != nil {
		t.Fatalf("clear did not reset owner-local guard: %v", err)
	}
}

func unsubmittedRuntimeGeneration(identity runtimePhysicalSubmissionIdentity) llmmodels.RuntimeCandidateGenerationResult {
	failureClass := string(runtimecontract.RuntimeCandidateFailureClassProviderConfiguration)
	return llmmodels.RuntimeCandidateGenerationResult{
		RequestID:     identity.PhysicalCallID,
		Status:        llmmodels.RuntimeCandidateStatusFailed,
		FailureClass:  failureClass,
		FailureReason: "pre-dispatch configuration failure",
		Audit: &llmmodels.ProviderRequestAudit{
			RequestID:            identity.PhysicalCallID,
			LogicalRequestID:     identity.LogicalRequestID,
			PhysicalCallID:       identity.PhysicalCallID,
			WorkspaceID:          identity.WorkspaceID,
			RunID:                identity.RunID,
			PhysicalResponseMode: llmaudit.PhysicalResponseModeNonStreaming,
			Status:               llmmodels.RuntimeCandidateStatusFailed,
			FailureClass:         failureClass,
			ContextBudget:        &llmmodels.ProviderContextBudget{},
		},
	}
}

func runtimeSemanticRequestWithExpectedDocument(t *testing.T) llmmodels.RuntimeCandidateRequest {
	t.Helper()
	request := runtimeSemanticProviderTestRequest(t)
	expected := runtimecontract.RuntimeExpectedDocumentContract{
		SchemaVersion:            runtimecontract.RuntimeExpectedDocumentSchemaCurrent,
		DocumentKind:             request.DocumentKind,
		PlanStep:                 request.SemanticContext.PlanStep,
		ManifestHash:             request.SemanticManifest.Hash,
		DefinitionSeedHash:       request.SemanticManifest.DefinitionSeedHash,
		LogicalRequestID:         request.RequestIdentity.LogicalRequestID,
		StablePrefixHash:         request.StablePrefixHash,
		BatchID:                  request.SemanticContext.BatchID,
		BatchSequence:            request.SemanticContext.BatchSequence,
		BatchCoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), request.SemanticContext.BatchCoverageWorkItemIDs...),
		CoverageOrderHash:        "coverage-order-runtime-owner-guard",
		MaxOutputTokens:          request.SemanticContext.ProviderCeiling.EffectiveMaxOutputTokens,
		MaxRecoveryAttempts:      2,
	}
	expected.Hash = runtimecontract.StableRuntimeExpectedDocumentHash(expected)
	request.ExpectedDocument = &expected
	return request
}

func runtimeCandidateCommandFromRequest(request llmmodels.RuntimeCandidateRequest) llmevents.RequestRuntimeCandidateCommand {
	workspaceID, runID := runtimeRequestScope(request)
	return llmevents.RequestRuntimeCandidateCommand{
		WorkspaceID:                    common.WorkspaceID(workspaceID),
		RunID:                          common.RunID(runID),
		DocumentKind:                   request.DocumentKind,
		RequestIdentity:                request.RequestIdentity,
		ExpectedDocument:               request.ExpectedDocument,
		Decision:                       request.Decision,
		Transaction:                    request.Transaction,
		CandidateAttempt:               request.CandidateAttempt,
		CandidateCorrectionObservation: request.CandidateCorrectionObservation,
		CandidateCorrection:            request.CandidateCorrection,
		StablePrefixHash:               request.StablePrefixHash,
		CandidateManifest:              request.CandidateManifest,
		DefinitionContext:              request.DefinitionContext,
		CandidateContext:               request.CandidateContext,
		SemanticManifest:               request.SemanticManifest,
		SemanticContext:                request.SemanticContext,
		SemanticCorrectionObservation:  request.SemanticCorrectionObservation,
		SemanticCorrectionDirective:    request.SemanticCorrectionDirective,
	}
}
