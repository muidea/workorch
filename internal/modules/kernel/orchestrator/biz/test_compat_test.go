package orchestrator

import (
	"sync"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// orchestratorbiz keeps moved package-local tests readable without importing
// their own package through its old root path.
var orchestratorbiz orchestratorBizTestShim

type orchestratorBizTestShim struct{}

// Module keeps moved legacy test names readable. Production lifecycle wiring
// uses Biz exclusively.
type Module = Biz

var runtimeV2CandidateRequestContentTestStores sync.Map

// runtimeV2CandidateRequestContentTestStore gives all Biz instances wired to
// one test EventHub the same opaque content repository. The production
// equivalent is process wiring to the durable repository; this helper keeps
// restart fixtures from replacing that repository while leaving raw provider
// documents subject to the normal C4/C5 persist-and-reload path.
func runtimeV2CandidateRequestContentTestStore(hub magicevent.Hub) repository.ArtifactContinuationRepository {
	if hub == nil {
		return memory.NewArtifactContinuationRepository()
	}
	store, _ := runtimeV2CandidateRequestContentTestStores.LoadOrStore(hub, memory.NewArtifactContinuationRepository())
	return store.(repository.ArtifactContinuationRepository)
}

// ensureRuntimeCandidatePreflightTestResponder makes test fakes state their
// rendered-request contract explicitly. Production always uses the LLM Block
// handler; this responder never calls a provider and only exists on test hubs
// that otherwise mock the candidate command directly.
func ensureRuntimeCandidatePreflightTestResponder(hub magicevent.Hub) {
	if hub == nil {
		return
	}
	observer := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorPreflightRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command, ok := event.Data().(orchestratorevents.PreflightRuntimeCandidateCommand)
		if !ok {
			return
		}
		result.Set(orchestratorevents.PreflightRuntimeCandidateResult{Transaction: command.Transaction, Ready: true}, nil)
	})
}

func newTestBiz(owner *RuntimeOrchestrator, hub magicevent.Hub, background magictask.BackgroundRoutine) *Biz {
	ensureRuntimeCandidatePreflightTestResponder(hub)
	// C4/C5 fixtures need the same opaque content carrier supplied by
	// production wiring. This is only test setup: a complete scripted provider
	// document still must persist and be decoded via the request-session path.
	if owner != nil && owner.evidence.ArtifactContinuations == nil {
		owner.evidence.ArtifactContinuations = runtimeV2CandidateRequestContentTestStore(hub)
	}
	return &Biz{Base: basebiz.New(ModuleID, hub, background), bizPtr: owner}
}

func (orchestratorBizTestShim) NewRuntimeOrchestrator(
	runs repository.RunRepository,
	workspaces repository.WorkspaceRepository,
	tickets repository.DecisionTicketRepository,
	approvals repository.ApprovalRepository,
) *RuntimeOrchestrator {
	return NewRuntimeOrchestrator(runs, workspaces, tickets, approvals)
}

// runtimeV2CandidateResultForTest is the only scripted-provider success
// builder for the orchestrator package. It deliberately encodes the provider
// wire schema, rather than marshaling RuntimeCandidate: RuntimeCandidate is
// an internal DTO and its field layout is not the provider response contract.
// C5 must consume DocumentContent just like production does; Candidate remains
// only as an optional assertion aid for legacy test bodies.
func runtimeV2CandidateResultForTest(t *testing.T, command orchestratorevents.RequestRuntimeCandidateCommand, candidate runtimecontract.RuntimeCandidate) orchestratorevents.RequestRuntimeCandidateResult {
	t.Helper()
	document, err := runtimecontract.EncodeRuntimeCandidatePayloadWithManifest(command.CandidateManifest, candidate)
	if err != nil {
		// Never Fatal inside an EventHub observer: that leaves hub.Send blocked.
		// Surface the encode/membership failure as a typed complete rejection.
		return orchestratorevents.RequestRuntimeCandidateResult{
			Transaction: command.Transaction, LogicalRequestID: command.LogicalRequestID,
			RequestSequence: command.RequestSequence, PhysicalCallID: command.PhysicalCallID,
			Failure: &runtimecontract.RuntimeCandidateFailure{
				Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "candidate_contract_invalid",
				Signature: "candidate-contract-invalid", Reason: err.Error(),
			},
		}
	}
	return orchestratorevents.RequestRuntimeCandidateResult{
		Transaction: command.Transaction, LogicalRequestID: command.LogicalRequestID,
		RequestSequence: command.RequestSequence, PhysicalCallID: command.PhysicalCallID,
		Candidate: candidate, DocumentContent: document,
	}
}

func runtimeV2CandidateDocumentForTest(t *testing.T, manifest runtimecontract.RuntimeCandidateContractManifest, candidate runtimecontract.RuntimeCandidate) []byte {
	t.Helper()
	document, err := runtimecontract.EncodeRuntimeCandidatePayloadWithManifest(manifest, candidate)
	if err != nil {
		t.Fatalf("encode test provider document from sealed manifest: %v", err)
	}
	return document
}
