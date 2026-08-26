package governance

import (
	"context"
	"errors"
	"testing"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	governancecommon "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func governanceTestResult(data any, err *cd.Error) magicevent.Result {
	result := magicevent.NewResult("test", "owner", governancecommon.ModuleID)
	result.Set(data, err)
	return result
}

func TestGovernanceOwnerResponseFamiliesFailClosed(t *testing.T) {
	ownerFailure := errors.New("owner unavailable")
	families := []struct {
		name string
		read func(magicevent.Result) error
	}{
		{name: "decision", read: func(result magicevent.Result) error {
			_, err := decisionResponse[decisionevents.GetTicketResult](result, decisionevents.TopicDecisionCommandGetTicket)
			return err
		}},
		{name: "approval", read: func(result magicevent.Result) error {
			_, err := approvalResponse[approvalevents.GetApprovalResult](result, approvalevents.TopicApprovalCommandGet)
			return err
		}},
		{name: "run", read: func(result magicevent.Result) error {
			_, err := runResponse[runevents.GetRunResult](result, runevents.TopicRunCommandGet)
			return err
		}},
		{name: "workspace", read: func(result magicevent.Result) error {
			_, err := workspaceResponse[workspaceevents.GoalResult](result, workspaceevents.TopicWorkspaceCommandGetGoal)
			return err
		}},
		{name: "memory", read: func(result magicevent.Result) error {
			_, err := memoryResponse[memoryevents.ListMemoryByWorkspaceResult](result, memoryevents.TopicMemoryCommandListByWorkspace)
			return err
		}},
	}
	failures := []struct {
		name   string
		result func() magicevent.Result
		check  func(error) bool
	}{
		{name: "nil result", result: func() magicevent.Result { return nil }, check: func(err error) bool { return errors.Is(err, common.ErrInvalidArgument) }},
		{name: "missing subscriber", result: func() magicevent.Result {
			return magicevent.NewResult("test", "governance", "missing-owner")
		}, check: func(err error) bool { return err != nil }},
		{name: "wrong result type", result: func() magicevent.Result { return governanceTestResult("wrong", nil) }, check: func(err error) bool { return errors.Is(err, common.ErrInvalidArgument) }},
		{name: "timeout", result: func() magicevent.Result {
			return governanceTestResult(nil, cd.NewError(cd.Timeout, "injected timeout"))
		}, check: func(err error) bool {
			var eventErr *cd.Error
			return errors.As(err, &eventErr) && eventErr.Code == cd.Timeout
		}},
		{name: "owner error", result: func() magicevent.Result {
			return governanceTestResult(nil, &cd.Error{Code: cd.Unexpected, Message: "injected owner error", Cause: ownerFailure})
		}, check: func(err error) bool { return errors.Is(err, ownerFailure) }},
	}
	for _, family := range families {
		for _, failure := range failures {
			t.Run(family.name+"/"+failure.name, func(t *testing.T) {
				err := family.read(failure.result())
				if !failure.check(err) {
					t.Fatalf("error = %v, want fail-closed %s response", err, failure.name)
				}
			})
		}
	}
}

func TestGovernanceOwnerAdaptersRejectMismatchedIdentity(t *testing.T) {
	workspaceID := common.WorkspaceID("ws-governance-owner-contract")
	runID := common.RunID("run-governance-owner-contract")
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
	payload := workorcheventpayload.New(
		workorcheventpayload.StringField(resolutionPayloadWorkspaceID, string(workspaceID)),
		workorcheventpayload.StringField(resolutionPayloadRunID, string(runID)),
	)
	cases := []struct {
		name string
		call func() error
	}{
		{name: "ticket", call: func() error {
			_, err := (eventTicketReader{send: func(magicevent.Event) magicevent.Result {
				return governanceTestResult(decisionevents.GetTicketResult{Ticket: &decisionmodel.Ticket{ID: "other", WorkspaceID: workspaceID}}, nil)
			}}).Get(ctx, "ticket")
			return err
		}},
		{name: "approval", call: func() error {
			_, err := (eventApprovalReader{send: func(magicevent.Event) magicevent.Result {
				return governanceTestResult(approvalevents.GetApprovalResult{Approval: &approvalmodel.Approval{ID: "approval", WorkspaceID: workspaceID, RunID: "other"}}, nil)
			}}).GetScoped(ctx, workspaceID, runID, "approval")
			return err
		}},
		{name: "run get", call: func() error {
			_, err := (eventRunReader{send: func(magicevent.Event) magicevent.Result {
				return governanceTestResult(runevents.GetRunResult{Run: &runmodel.Run{ID: runID, WorkspaceID: "other"}}, nil)
			}}).Get(ctx, runID)
			return err
		}},
		{name: "run scoped get", call: func() error {
			_, err := (eventRunReader{send: func(magicevent.Event) magicevent.Result {
				return governanceTestResult(runevents.GetRunResult{Run: &runmodel.Run{ID: "other", WorkspaceID: workspaceID}}, nil)
			}}).GetByWorkspaceID(ctx, workspaceID, runID)
			return err
		}},
		{name: "run inventory", call: func() error {
			_, err := (eventRunReader{send: func(magicevent.Event) magicevent.Result {
				return governanceTestResult(runevents.ListRunsResult{Runs: []*runmodel.Run{nil}}, nil)
			}}).List(ctx)
			return err
		}},
		{name: "goal", call: func() error {
			_, err := (eventWorkspaceReader{send: func(magicevent.Event) magicevent.Result {
				return governanceTestResult(workspaceevents.GoalResult{Goal: &workspacemodel.Goal{ID: "other", WorkspaceID: workspaceID}}, nil)
			}}).GetGoal(ctx, "goal")
			return err
		}},
		{name: "dod", call: func() error {
			_, err := (eventWorkspaceReader{send: func(magicevent.Event) magicevent.Result {
				return governanceTestResult(workspaceevents.DoDResult{DoD: &workspacemodel.DoD{ID: "dod", WorkspaceID: "other"}}, nil)
			}}).GetDoD(ctx, "dod")
			return err
		}},
		{name: "memory", call: func() error {
			_, err := (eventMemoryReader{send: func(magicevent.Event) magicevent.Result {
				return governanceTestResult(memoryevents.ListMemoryByWorkspaceResult{Items: []*memorymodel.MemoryItem{{ID: "memory", WorkspaceID: "other"}}}, nil)
			}}).ListByWorkspaceID(ctx, workspaceID)
			return err
		}},
		{name: "decision ensure", call: func() error {
			return eventResolveDecision(func(magicevent.Event) magicevent.Result {
				return governanceTestResult(decisionevents.EnsureResolvedResult{}, nil)
			})(ctx, "ticket", "provide_missing_info", payload)
		}},
		{name: "approval ensure", call: func() error {
			return eventResolveApproval(func(magicevent.Event) magicevent.Result {
				return governanceTestResult(approvalevents.EnsureApprovedResult{}, nil)
			})(ctx, "approval", runID)
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("error = %v, want owner identity conflict", err)
			}
		})
	}
}

func TestGovernanceEventResolversAcceptMatchingOwnerResults(t *testing.T) {
	workspaceID := common.WorkspaceID("ws-governance-resolver-contract")
	runID := common.RunID("run-governance-resolver-contract")
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
	payload := workorcheventpayload.New(
		workorcheventpayload.StringField(resolutionPayloadWorkspaceID, string(workspaceID)),
		workorcheventpayload.StringField(resolutionPayloadRunID, string(runID)),
	)
	resolveDecision := eventResolveDecision(func(event magicevent.Event) magicevent.Result {
		command := event.Data().(decisionevents.EnsureResolvedCommand)
		return governanceTestResult(decisionevents.EnsureResolvedResult{
			Ticket:     &decisionmodel.Ticket{ID: command.TicketID, WorkspaceID: command.WorkspaceID, RunID: command.RunID, State: common.DecisionTicketStateResolved},
			Resolution: &decisionmodel.Resolution{ID: command.ResolutionID, TicketID: command.TicketID, Type: command.Type},
		}, nil)
	})
	if err := resolveDecision(ctx, "ticket", "provide_missing_info", payload); err != nil {
		t.Fatalf("resolve decision: %v", err)
	}
	resolveApproval := eventResolveApproval(func(event magicevent.Event) magicevent.Result {
		command := event.Data().(approvalevents.EnsureApprovedCommand)
		return governanceTestResult(approvalevents.EnsureApprovedResult{Approval: &approvalmodel.Approval{
			ID: command.ApprovalID, WorkspaceID: command.WorkspaceID, RunID: command.RunID, State: common.ApprovalStateApproved,
			ResolutionPrincipal: approvalmodel.ApprovalResolutionPrincipal{Kind: approvalmodel.ApprovalResolutionPrincipalAutoGovernance, SubjectID: governancecommon.ModuleID},
		}}, nil)
	})
	if err := resolveApproval(ctx, "approval", runID); err != nil {
		t.Fatalf("resolve approval: %v", err)
	}
}
