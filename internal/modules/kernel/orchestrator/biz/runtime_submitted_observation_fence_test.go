package orchestrator

import (
	"context"
	"errors"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// The interruption is after the submitted marker but before physical provider
// I/O. This proves durable responsibility recovery, not a remote call outcome.
func TestRuntimeSubmittedObservationFenceSemanticRecovery(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	repo, aggregate := runtimeSemanticPreflightFixture(t)
	cut := &semanticPreflightCutRepository{
		RuntimeStateRepository: repo, after: true, cut: errors.New("submitted marker response lost"),
		match: func(a runtime.RuntimeAggregate) bool {
			return len(a.CandidateRequestSessions) == 1 && a.CandidateRequestSessions[0].ProviderSubmitted
		},
	}
	module := newRuntimeSemanticPreflightModule(t, hub, cut)
	preflights, providers := 0, 0
	observer := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	observer.Subscribe(llmevents.TopicPreflightRuntimeCandidate, func(ev magicevent.Event, result magicevent.Result) {
		preflights++
		command := ev.Data().(llmevents.PreflightRuntimeCandidateCommand)
		result.Set(llmevents.PreflightRuntimeCandidateResult{DocumentKind: command.DocumentKind, Feedback: runtimePreflightFeedbackForTest(t, command, "")}, nil)
	})
	observer.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(_ magicevent.Event, result magicevent.Result) {
		providers++
		result.Set(nil, nil)
	})
	started, err := module.driveRuntimeSemanticPlanning(ctx, aggregate)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = module.driveRuntimeSemanticPlanning(ctx, started); !errors.Is(err, cut.cut) || !cut.fired {
		t.Fatalf("submitted cut not reached: fired=%t err=%v", cut.fired, err)
	}
	before, err := cut.Load(ctx, started.Facts.WorkspaceID, started.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if err := before.Validate(); err != nil {
		t.Fatal(err)
	}
	restarted := newRuntimeSemanticPreflightModule(t, hub, cut)
	outcome, err := restarted.driveOneRuntimeTransition(ctx, nil, before, orchestratorevents.DriveRuntimeCommand{}, "")
	if err != nil {
		t.Fatal(err)
	}
	after := outcome.aggregate
	if err := after.Validate(); err != nil {
		t.Fatal(err)
	}
	if after.StateRevision != before.StateRevision+1 || preflights != 1 || providers != 0 ||
		after.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningFailed ||
		after.Facts.Failure == nil || after.Facts.Failure.Code != "semantic_contract_failed" ||
		after.CandidateRequestSessions[0].LastOutcome.Code != "semantic_provider_submission_unknown" ||
		after.Facts.Failure.Class != runtimecontract.FailureClassProviderSubmissionUnknown ||
		after.CandidateRequestSessions[0].State != runtimecontract.RuntimeCandidateRequestSessionTerminal {
		t.Fatalf("semantic submission responsibility not closed exactly once: revision=%d preflights=%d providers=%d failure=%+v", after.StateRevision, preflights, providers, after.Facts.Failure)
	}
	unknownCalls := 0
	for _, consumption := range after.ExecutionBudget.Consumptions {
		if consumption.Unit == "call" && consumption.PhysicalConsumptionID == before.CandidateRequestSessions[0].LastPhysicalCallID && consumption.UnknownAmount == 1 && consumption.ConfirmedAmount == 0 {
			unknownCalls++
		}
	}
	if unknownCalls != 1 ||
		!reflect.DeepEqual(before.OperationReceipts, after.OperationReceipts) ||
		!reflect.DeepEqual(before.Facts.Evidence, after.Facts.Evidence) ||
		!reflect.DeepEqual(before.Reconciliation, after.Reconciliation) {
		t.Fatal("submission settlement lost budget responsibility or changed unrelated facts")
	}
	// The original unknown settlement is independently checked by the aggregate
	// budget invariant; duplicate recovery must not create another consumption.
	duplicate, changed, err := restarted.settleRuntimeSubmittedProviderBeforeObservation(ctx, after)
	if err != nil || changed || !reflect.DeepEqual(after, duplicate) || !reflect.DeepEqual(repo.aggregate, after) {
		t.Fatalf("duplicate recovery mutated settled responsibility: changed=%t err=%v", changed, err)
	}
}

// Forged/nonmatching inventories are predicate tests only: they cannot gain
// owner I/O or CAS access. They do not prove a historical late-settlement lane.
func TestRuntimeSubmittedObservationFenceRejectsAmbiguousAuthorityBeforeIO(t *testing.T) {
	for _, variant := range []string{"historical", "multiple", "multiple_with_historical"} {
		t.Run(variant, func(t *testing.T) {
			_, aggregate := runtimeSemanticPreflightFixture(t)
			session := runtimecontract.RuntimeCandidateRequestSession{
				ProviderSubmitted: true, State: runtimecontract.RuntimeCandidateRequestSessionPrepared,
				Transaction: aggregate.Decision.AuthorityTransaction(),
			}
			aggregate.CandidateRequestSessions = []runtimecontract.RuntimeCandidateRequestSession{session}
			switch variant {
			case "historical":
				aggregate.CandidateRequestSessions[0].Transaction.AuthorityInputID += "-historical"
			case "multiple":
				aggregate.CandidateRequestSessions = append(aggregate.CandidateRequestSessions, session)
			case "multiple_with_historical":
				session.Transaction.AuthorityInputID += "-historical"
				aggregate.CandidateRequestSessions = append(aggregate.CandidateRequestSessions, session)
			}
			before := aggregate.CloneRaw()
			// No configured controller or Hub: any attempt to access persistence
			// or another owner before this guard would fail the test.
			_, changed, err := (&Biz{}).settleRuntimeSubmittedProviderBeforeObservation(context.Background(), aggregate)
			if !errors.Is(err, common.ErrConflict) || changed || !reflect.DeepEqual(before, aggregate) {
				t.Fatalf("nonunique/current authority escaped zero-I/O guard: changed=%t err=%v", changed, err)
			}
		})
	}
}
