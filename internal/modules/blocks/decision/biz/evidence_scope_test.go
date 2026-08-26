package decision

import (
	"context"
	"errors"
	"testing"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	filerepository "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestDecisionEvidenceIdentityScopesSameResolutionAcrossBackends(t *testing.T) {
	for _, testCase := range []struct {
		name string
		new  func(*testing.T) repository.EvidenceRecordRepository
	}{
		{name: "memory", new: func(*testing.T) repository.EvidenceRecordRepository { return memory.NewEvidenceRecordRepository() }},
		{name: "file", new: func(t *testing.T) repository.EvidenceRecordRepository {
			return filerepository.NewEvidenceRecordRepository(t.TempDir())
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			evidence := testCase.new(t)
			hub := magicevent.NewHub(16)
			t.Cleanup(func() { hub.Terminate(ctx) })
			commands := make([]orchestratorevents.RecordDecisionEvidenceCommand, 0, 4)
			observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
			observer.Subscribe(orchestratorevents.TopicOrchestratorCommandRecordDecisionEvidence, func(event magicevent.Event, result magicevent.Result) {
				command, ok := event.Data().(orchestratorevents.RecordDecisionEvidenceCommand)
				if !ok {
					result.Set(nil, &cd.Error{Code: cd.IllegalParam, Message: common.ErrInvalidArgument.Error(), Cause: common.ErrInvalidArgument})
					return
				}
				commands = append(commands, command)
				record := &runtimecontract.EvidenceRecord{
					ID: command.EvidenceRecordID, WorkspaceID: command.WorkspaceID, RunID: command.RunID, StepID: command.StepID,
					Kind: runtimecontract.EvidenceKindDecision, Status: runtimecontract.EvidenceStatusAccepted,
					SourceType: "decision_resolution", SourceRef: string(command.ResolutionID), SubjectType: "decision_ticket", SubjectID: string(command.TicketID),
					CreatedAt: command.Timestamp, UpdatedAt: command.Timestamp,
					Metadata: workorcheventpayload.New(workorcheventpayload.StringField("ticketId", string(command.TicketID))),
				}
				bound := common.ContextWithRuntimeWorkspaceID(event.Context(), command.WorkspaceID)
				if err := repository.CreateOrVerifyImmutableEvidenceRecord(bound, evidence, record); err != nil {
					result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
					return
				}
				result.Set(orchestratorevents.RecordDecisionEvidenceResult{EvidenceRecordID: command.EvidenceRecordID}, nil)
			})
			t.Cleanup(func() { observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommandRecordDecisionEvidence) })

			ownerSet := memory.NewDecisionOwnerRepositorySet()
			owner, err := New(hub, nil, Dependencies{OwnerRepositories: ownerSet.OwnerRepositories(), EventHub: hub})
			if err != nil {
				t.Fatal(err)
			}
			const resolvedAt = "2026-08-26T03:00:00Z"
			for _, scope := range []struct {
				workspace common.WorkspaceID
				run       common.RunID
			}{
				{workspace: "ws-decision-evidence-a", run: "run-decision-evidence-a"},
				{workspace: "ws-decision-evidence-b", run: "run-decision-evidence-b"},
			} {
				ticket := &decisionmodel.Ticket{
					ID: "ticket-shared", WorkspaceID: scope.workspace, RunID: scope.run, StepID: "decision/shared",
					DecisionKind: "operator_input", State: common.DecisionTicketStateResolved, ResolvedAt: resolvedAt,
				}
				resolution := &decisionmodel.Resolution{
					ID: "resolution-shared", TicketID: ticket.ID, ResolvedAt: resolvedAt, Type: "provide_missing_info",
				}
				if err := owner.recordDecisionResolutionEvidence(ctx, ticket, resolution); err != nil {
					t.Fatalf("record scoped evidence workspace=%s: %v", scope.workspace, err)
				}
				if err := owner.recordDecisionResolutionEvidence(ctx, ticket, resolution); err != nil {
					t.Fatalf("exact replay workspace=%s: %v", scope.workspace, err)
				}
			}

			if len(commands) != 4 {
				t.Fatalf("evidence commands = %d, want 4", len(commands))
			}
			if commands[0].EvidenceRecordID != commands[1].EvidenceRecordID || commands[2].EvidenceRecordID != commands[3].EvidenceRecordID {
				t.Fatalf("exact replay identities drifted: %+v", commands)
			}
			if commands[0].EvidenceRecordID == commands[2].EvidenceRecordID {
				t.Fatalf("cross-workspace Decision evidence identity collided: %q", commands[0].EvidenceRecordID)
			}
			for _, command := range []orchestratorevents.RecordDecisionEvidenceCommand{commands[0], commands[2]} {
				stored, err := evidence.Get(common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID), command.EvidenceRecordID)
				if err != nil || stored.WorkspaceID != command.WorkspaceID || stored.RunID != command.RunID || stored.SubjectID != string(command.TicketID) {
					t.Fatalf("stored scoped evidence = %+v err=%v command=%+v", stored, err, command)
				}
			}
			if _, err := evidence.Get(common.ContextWithRuntimeWorkspaceID(ctx, commands[0].WorkspaceID), commands[2].EvidenceRecordID); testCase.name == "memory" && !errors.Is(err, common.ErrNotFound) {
				t.Fatalf("memory cross-workspace lookup = %v, want not found", err)
			}
		})
	}
}
