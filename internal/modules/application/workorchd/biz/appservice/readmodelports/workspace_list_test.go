package readmodelports

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"

	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type workspaceListWorkspaceReaderFunc func(context.Context) ([]*workspacemodel.Workspace, error)

func (fn workspaceListWorkspaceReaderFunc) List(ctx context.Context) ([]*workspacemodel.Workspace, error) {
	return fn(ctx)
}

type workspaceListRunReaderFunc func(context.Context) ([]*runmodel.Run, error)

func (fn workspaceListRunReaderFunc) List(ctx context.Context) ([]*runmodel.Run, error) {
	return fn(ctx)
}

type workspaceListResultReaderFunc func(context.Context, common.WorkspaceID) ([]*resultmodel.Result, error)

func (fn workspaceListResultReaderFunc) ListByWorkspace(ctx context.Context, workspaceID common.WorkspaceID) ([]*resultmodel.Result, error) {
	return fn(ctx, workspaceID)
}

type workspaceListEventRecordReaderFunc func(context.Context, eventstreamevents.EventRecordQuery) ([]*eventstreammodel.EventRecord, error)

func (fn workspaceListEventRecordReaderFunc) List(ctx context.Context, query eventstreamevents.EventRecordQuery) ([]*eventstreammodel.EventRecord, error) {
	return fn(ctx, query)
}

func TestListProjectedWorkspacesUsesLatestRunProjection(t *testing.T) {
	ctx := context.Background()
	workspaceID := common.WorkspaceID("ws-001")
	items, err := ListProjectedWorkspaces(
		ctx,
		workspaceListWorkspaceReaderFunc(func(context.Context) ([]*workspacemodel.Workspace, error) {
			return []*workspacemodel.Workspace{{
				ID:          workspaceID,
				Title:       "workspace",
				State:       common.WorkspaceStateActive,
				ActiveRunID: common.RunID("run-002"),
			}}, nil
		}),
		workspaceListRunReaderFunc(func(context.Context) ([]*runmodel.Run, error) {
			return []*runmodel.Run{
				{
					ID:          common.RunID("run-001"),
					WorkspaceID: workspaceID,
					State:       common.RunStateCompleted,
				},
				{
					ID:               common.RunID("run-002"),
					WorkspaceID:      workspaceID,
					State:            common.RunStateBlockedOnDecision,
					BlockingTicketID: common.TicketID("ticket-001"),
				},
			}, nil
		}),
		workspaceListResultReaderFunc(func(context.Context, common.WorkspaceID) ([]*resultmodel.Result, error) {
			return nil, nil
		}),
		workspaceListEventRecordReaderFunc(func(_ context.Context, query eventstreamevents.EventRecordQuery) ([]*eventstreammodel.EventRecord, error) {
			if query.Limit != readmodelcore.DefaultEventRecordLimit {
				t.Fatalf("event query limit = %d, want %d", query.Limit, readmodelcore.DefaultEventRecordLimit)
			}
			return []*eventstreammodel.EventRecord{{
				WorkspaceID: workspaceID,
				RunID:       common.RunID("run-002"),
				Topic:       runevents.TopicRunStateChanged,
				OccurredAt:  "2026-06-05T00:00:00Z",
			}}, nil
		}),
	)
	if err != nil {
		t.Fatalf("ListProjectedWorkspaces returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one workspace, got %d", len(items))
	}
	if items[0].State != string(common.WorkspaceStateActive) {
		t.Fatalf("expected projected active state, got %q", items[0].State)
	}
	if items[0].ActiveRunID != "run-002" {
		t.Fatalf("expected active run projection, got %q", items[0].ActiveRunID)
	}
	if items[0].BlockingTicketID != "ticket-001" {
		t.Fatalf("expected blocking ticket projection, got %q", items[0].BlockingTicketID)
	}
}
