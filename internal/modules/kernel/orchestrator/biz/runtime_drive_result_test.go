package orchestrator

import (
	"testing"

	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

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
