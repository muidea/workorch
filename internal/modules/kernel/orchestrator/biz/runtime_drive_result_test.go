package orchestrator

import (
	"testing"

	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeTerminalDeliveryWaitsForCurrentSemanticAuthority(t *testing.T) {
	aggregate := runtime.RuntimeAggregate{
		StateRevision: 2,
		Facts: runtimecontract.RuntimeFacts{
			Focus: runtimecontract.CoverageFocus{ID: "semantic-pending/cwi-2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-2"}},
			Coverage: []runtimecontract.RuntimeCoverageItemFact{{
				ID: "cwi-2", State: runtimecontract.RuntimeCoverageItemStateOpen,
				CWIConvergenceState: runtimecontract.RuntimeCWIConvergenceStatePlanning,
			}},
		},
		Decision: runtimecontract.RuntimeDecision{
			State: runtimecontract.ConvergenceStateFailed,
			Terminal: &runtimecontract.RuntimeTerminalDecision{
				Code: "target_grounding_unresolved", Summary: "binding-free focus has no target yet",
			},
		},
		SemanticPlanning: runtimecontract.RuntimeSemanticPlanningLedger{Phase: runtimecontract.RuntimeSemanticPlanningRequired},
	}
	if runtimeTerminalDeliveryReady(aggregate) {
		t.Fatal("binding-free reducer terminal escaped before semantic re-seal")
	}
	if outcome, err := finishRuntimeConvergenceTransition(aggregate, 1, "before-catalog-retirement"); err != nil || outcome.result != nil {
		t.Fatalf("provisional terminal produced an SM-4 result: outcome=%+v err=%v", outcome, err)
	}

	aggregate.Decision.Terminal.Code = "runtime_contract_defect"
	if !runtimeTerminalDeliveryReady(aggregate) {
		t.Fatal("authoritative non-grounding terminal was incorrectly reopened for semantic planning")
	}
	aggregate.Decision.Terminal.Code = "target_grounding_unresolved"

	aggregate.SemanticPlanning.Phase = runtimecontract.RuntimeSemanticPlanningSealed
	if !runtimeTerminalDeliveryReady(aggregate) {
		t.Fatal("sealed semantic terminal was not delivery-ready")
	}
	if outcome, err := finishRuntimeConvergenceTransition(aggregate, 1, "before-semantic-seal"); err != nil || outcome.result == nil || outcome.result.Terminal == nil {
		t.Fatalf("sealed terminal did not produce its SM-4 result: outcome=%+v err=%v", outcome, err)
	}

	aggregate.SemanticPlanning.Phase = runtimecontract.RuntimeSemanticPlanningFailed
	if !runtimeTerminalDeliveryReady(aggregate) {
		t.Fatal("semantic transaction failure was not delivery-ready")
	}
}

func TestRuntimeAdministrativeTerminationIsStartupAndDriveTerminalSuccessor(t *testing.T) {
	aggregate := runtime.RuntimeAggregate{
		StateRevision: 12,
		Facts: runtimecontract.RuntimeFacts{
			WorkspaceID: "ws-admin-terminal", RunID: "run-admin-terminal",
			DefinitionRevision: "definition-admin-terminal", EvidenceRevision: "evidence-admin-terminal",
		},
		Decision: runtimecontract.RuntimeDecision{
			State: runtimecontract.ConvergenceStateNeedStateChange,
			AuthoritySnapshot: runtimecontract.AuthoritySnapshot{
				ID: "authority-admin-terminal", WorkspaceID: "ws-admin-terminal", RunID: "run-admin-terminal",
			},
		},
		Termination: &runtimecontract.RuntimeTerminationRecord{
			Kind: runtimecontract.RuntimeTerminationCancelled, RequestedBy: "blocks/run", Reason: "operator cancel",
			Revision: 12, RequestedAt: "2026-08-30T00:00:00Z",
		},
	}
	if !runtimeTerminalDeliveryReady(aggregate) {
		t.Fatal("persisted administrative termination was not classified as a startup terminal-delivery successor")
	}
	result := runtimeTerminalDriveResult(aggregate)
	if result.AdministrativeTermination == nil || result.Terminal != nil || result.AdministrativeTermination.Kind != runtimecontract.RuntimeTerminationCancelled {
		t.Fatalf("administrative terminal drive result rebuilt convergence authority: %+v", result)
	}
	if err := validateRuntimeDriveResult(result); err != nil {
		t.Fatalf("administrative terminal drive result rejected: %v", err)
	}
}

func TestRuntimeDriveResultDerivesItsOnlyValidDispositionFromSealedFacts(t *testing.T) {
	base := orchestratorevents.DriveRuntimeResult{
		WorkspaceID: "ws-drive-result", RunID: "run-drive-result",
		StateRevision: 1, AuthoritySnapshotID: "authority-drive-result",
	}
	tests := []struct {
		name    string
		mutate  func(*orchestratorevents.DriveRuntimeResult)
		wantErr bool
	}{
		{name: "terminal complete", mutate: func(r *orchestratorevents.DriveRuntimeResult) {
			r.State = runtimecontract.ConvergenceStateComplete
			r.Terminal = &runtimecontract.RuntimeTerminalDecision{Code: "complete", Summary: "done"}
		}},
		{name: "terminal failed", mutate: func(r *orchestratorevents.DriveRuntimeResult) {
			r.State = runtimecontract.ConvergenceStateFailed
			r.Terminal = &runtimecontract.RuntimeTerminalDecision{Code: "failed", Summary: "failed"}
		}},
		{name: "governed wait", mutate: func(r *orchestratorevents.DriveRuntimeResult) {
			r.State = runtimecontract.ConvergenceStateNeedGovernance
			r.BlockingTicketID = common.TicketID("ticket-drive-result")
		}},
		{name: "recoverable wait", mutate: func(r *orchestratorevents.DriveRuntimeResult) {
			r.State = runtimecontract.ConvergenceStateNeedPrerequisite
			r.RecoverableWait = &runtimecontract.RuntimeRecoverableWaitView{
				WaitID: "wait-drive-result", Status: runtimecontract.RuntimeRecoverableWaitWaiting, StateRevision: r.StateRevision,
			}
		}},
		{name: "terminal state without detail", wantErr: true, mutate: func(r *orchestratorevents.DriveRuntimeResult) {
			r.State = runtimecontract.ConvergenceStateComplete
		}},
		{name: "governance state without blocker", wantErr: true, mutate: func(r *orchestratorevents.DriveRuntimeResult) {
			r.State = runtimecontract.ConvergenceStateNeedGovernance
		}},
		{name: "ordinary nonterminal state", wantErr: true, mutate: func(r *orchestratorevents.DriveRuntimeResult) {
			r.State = runtimecontract.ConvergenceStateNeedStateChange
		}},
		{name: "terminal and blocker", wantErr: true, mutate: func(r *orchestratorevents.DriveRuntimeResult) {
			r.State = runtimecontract.ConvergenceStateFailed
			r.Terminal = &runtimecontract.RuntimeTerminalDecision{Code: "failed", Summary: "failed"}
			r.BlockingTicketID = common.TicketID("ticket-drive-result")
		}},
		{name: "recoverable and governance wait", wantErr: true, mutate: func(r *orchestratorevents.DriveRuntimeResult) {
			r.State = runtimecontract.ConvergenceStateNeedGovernance
			r.BlockingTicketID = common.TicketID("ticket-drive-result")
			r.RecoverableWait = &runtimecontract.RuntimeRecoverableWaitView{
				WaitID: "wait-drive-result", Status: runtimecontract.RuntimeRecoverableWaitWaiting, StateRevision: r.StateRevision,
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := base
			test.mutate(&result)
			err := validateRuntimeDriveResult(result)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateRuntimeDriveResult() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}
