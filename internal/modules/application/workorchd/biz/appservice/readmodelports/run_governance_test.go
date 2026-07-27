package readmodelports

import (
	"context"
	"testing"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestDecisionsForRunSelectsBlockingTicketAndProjectsResolution(t *testing.T) {
	t.Parallel()

	run := &runmodel.Run{
		ID:               "run-governance",
		WorkspaceID:      "ws-governance",
		BlockingTicketID: "ticket-blocking",
	}
	tickets := []*decisionmodel.Ticket{
		{ID: "ticket-other", WorkspaceID: run.WorkspaceID, RunID: run.ID, State: common.DecisionTicketStateOpen},
		{ID: "ticket-blocking", WorkspaceID: run.WorkspaceID, RunID: run.ID, State: common.DecisionTicketStateResolved},
		{ID: "ticket-wrong-run", WorkspaceID: run.WorkspaceID, RunID: "run-other", State: common.DecisionTicketStateResolved},
	}
	resolution := &decisionmodel.Resolution{
		ID:                 "resolution-blocking",
		TicketID:           "ticket-blocking",
		Type:               "provide_missing_info",
		ContinuationAction: "replan",
		ResumeRunID:        "run-governance",
		SuccessorRunID:     "run-governance-next",
	}

	got, err := DecisionsForRun(
		context.Background(),
		fakeTicketReader{items: tickets},
		fakeResolutionReader{items: map[common.TicketID]*decisionmodel.Resolution{"ticket-blocking": resolution}},
		nil,
		run,
	)
	if err != nil {
		t.Fatalf("DecisionsForRun: %v", err)
	}
	if got.SelectedTicket == nil || got.SelectedTicket.ID != "ticket-blocking" {
		t.Fatalf("selected ticket = %+v, want ticket-blocking", got.SelectedTicket)
	}
	if got.SelectedResolution == nil || got.SelectedResolution.ID != "resolution-blocking" {
		t.Fatalf("selected resolution = %+v, want resolution-blocking", got.SelectedResolution)
	}
	if len(got.Items) != 2 {
		t.Fatalf("items len = %d, want 2 same-run decisions", len(got.Items))
	}
	if got.Items[1].ResolutionID != "resolution-blocking" || got.Items[1].ResolutionType != "provide_missing_info" {
		t.Fatalf("blocking decision projection = %+v, want resolution fields", got.Items[1])
	}
	if got.Items[1].ContinuationAction != "replan" || got.Items[1].ResumeRunID != "run-governance" || got.Items[1].SuccessorRunID != "run-governance-next" {
		t.Fatalf("blocking decision projection = %+v, want resolution recovery refs", got.Items[1])
	}
}

func TestApprovalsForRunSelectsBlockingApproval(t *testing.T) {
	t.Parallel()

	run := &runmodel.Run{
		ID:                 "run-approval-governance",
		WorkspaceID:        "ws-approval-governance",
		BlockingApprovalID: "approval-blocking",
	}
	approvals := []*approvalmodel.Approval{
		{ID: "approval-other", WorkspaceID: run.WorkspaceID, RunID: run.ID, State: common.ApprovalStatePending},
		{ID: "approval-blocking", WorkspaceID: run.WorkspaceID, RunID: run.ID, State: common.ApprovalStateApproved},
		{ID: "approval-wrong-run", WorkspaceID: run.WorkspaceID, RunID: "run-other", State: common.ApprovalStateApproved},
	}

	got, err := ApprovalsForRun(context.Background(), fakeApprovalListReader{items: approvals}, nil, run)
	if err != nil {
		t.Fatalf("ApprovalsForRun: %v", err)
	}
	if got.SelectedApproval == nil || got.SelectedApproval.ID != "approval-blocking" {
		t.Fatalf("selected approval = %+v, want approval-blocking", got.SelectedApproval)
	}
	if len(got.Items) != 2 {
		t.Fatalf("items len = %d, want 2 same-run approvals", len(got.Items))
	}
	if got.Items[1].ID != "approval-blocking" || got.Items[1].State != string(common.ApprovalStateApproved) {
		t.Fatalf("blocking approval projection = %+v, want approved approval", got.Items[1])
	}
}

func TestDecisionsForWorkspaceItemsFiltersWorkspaceAndProjectsResolution(t *testing.T) {
	t.Parallel()

	workspaceID := common.WorkspaceID("ws-governance-items")
	items := []*decisionmodel.Ticket{
		{ID: "ticket-workspace-open", WorkspaceID: workspaceID, RunID: "run-001", State: common.DecisionTicketStateOpen},
		{ID: "ticket-workspace-resolved", WorkspaceID: workspaceID, RunID: "run-002", State: common.DecisionTicketStateResolved},
		{ID: "ticket-other-workspace", WorkspaceID: "ws-other", RunID: "run-003", State: common.DecisionTicketStateResolved},
	}
	resolution := &decisionmodel.Resolution{
		ID:       "resolution-workspace",
		TicketID: "ticket-workspace-resolved",
		Type:     "provide_missing_info",
	}

	got, err := DecisionsForWorkspaceItems(
		context.Background(),
		fakeResolutionReader{items: map[common.TicketID]*decisionmodel.Resolution{"ticket-workspace-resolved": resolution}},
		nil,
		workspaceID,
		items,
	)
	if err != nil {
		t.Fatalf("DecisionsForWorkspaceItems: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("items len = %d, want 2 workspace decisions", len(got))
	}
	if got[1].ID != "ticket-workspace-resolved" || got[1].ResolutionID != "resolution-workspace" {
		t.Fatalf("resolved decision = %+v, want workspace resolution projection", got[1])
	}
}

func TestApprovalsForWorkspaceItemsFiltersWorkspace(t *testing.T) {
	t.Parallel()

	workspaceID := common.WorkspaceID("ws-approval-items")
	items := []*approvalmodel.Approval{
		{ID: "approval-workspace-pending", WorkspaceID: workspaceID, RunID: "run-001", State: common.ApprovalStatePending},
		{ID: "approval-workspace-approved", WorkspaceID: workspaceID, RunID: "run-002", State: common.ApprovalStateApproved},
		{ID: "approval-other-workspace", WorkspaceID: "ws-other", RunID: "run-003", State: common.ApprovalStateApproved},
	}

	got, err := ApprovalsForWorkspaceItems(context.Background(), nil, workspaceID, items)
	if err != nil {
		t.Fatalf("ApprovalsForWorkspaceItems: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("items len = %d, want 2 workspace approvals", len(got))
	}
	if got[1].ID != "approval-workspace-approved" || got[1].State != string(common.ApprovalStateApproved) {
		t.Fatalf("approved approval = %+v, want workspace approval projection", got[1])
	}
}

type fakeTicketReader struct {
	items []*decisionmodel.Ticket
}

func (f fakeTicketReader) Get(_ context.Context, id common.TicketID) (*decisionmodel.Ticket, error) {
	for _, item := range f.items {
		if item != nil && item.ID == id {
			return item, nil
		}
	}
	return nil, common.ErrNotFound
}

func (f fakeTicketReader) List(context.Context) ([]*decisionmodel.Ticket, error) {
	return append([]*decisionmodel.Ticket(nil), f.items...), nil
}

func (f fakeTicketReader) ListByRunID(_ context.Context, runID common.RunID) ([]*decisionmodel.Ticket, error) {
	ret := make([]*decisionmodel.Ticket, 0, len(f.items))
	for _, item := range f.items {
		if item != nil && item.RunID == runID {
			ret = append(ret, item)
		}
	}
	return ret, nil
}

func (f fakeTicketReader) ListByWorkspaceID(_ context.Context, workspaceID common.WorkspaceID) ([]*decisionmodel.Ticket, error) {
	ret := make([]*decisionmodel.Ticket, 0, len(f.items))
	for _, item := range f.items {
		if item != nil && item.WorkspaceID == workspaceID {
			ret = append(ret, item)
		}
	}
	return ret, nil
}

type fakeResolutionReader struct {
	items map[common.TicketID]*decisionmodel.Resolution
}

func (f fakeResolutionReader) GetByTicketID(_ context.Context, id common.TicketID) (*decisionmodel.Resolution, error) {
	if item := f.items[id]; item != nil {
		return item, nil
	}
	return nil, common.ErrNotFound
}

type fakeApprovalListReader struct {
	items []*approvalmodel.Approval
}

func (f fakeApprovalListReader) Get(_ context.Context, id common.ApprovalID) (*approvalmodel.Approval, error) {
	for _, item := range f.items {
		if item != nil && item.ID == id {
			return item, nil
		}
	}
	return nil, common.ErrNotFound
}

func (f fakeApprovalListReader) List(context.Context) ([]*approvalmodel.Approval, error) {
	return append([]*approvalmodel.Approval(nil), f.items...), nil
}

func (f fakeApprovalListReader) ListByRunID(_ context.Context, runID common.RunID) ([]*approvalmodel.Approval, error) {
	ret := make([]*approvalmodel.Approval, 0, len(f.items))
	for _, item := range f.items {
		if item != nil && item.RunID == runID {
			ret = append(ret, item)
		}
	}
	return ret, nil
}

func (f fakeApprovalListReader) ListByWorkspaceID(_ context.Context, workspaceID common.WorkspaceID) ([]*approvalmodel.Approval, error) {
	ret := make([]*approvalmodel.Approval, 0, len(f.items))
	for _, item := range f.items {
		if item != nil && item.WorkspaceID == workspaceID {
			ret = append(ret, item)
		}
	}
	return ret, nil
}
