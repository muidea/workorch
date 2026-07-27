package readmodelports

import (
	"context"
	"errors"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func workspaceSnapshotBaseLoadTasks(ctx context.Context, readers WorkspaceSnapshotReaders, workspaceID common.WorkspaceID, cachedRunSnapshot *readmodelcore.RunSnapshot, options workspaceSnapshotLoadOptions, models *workspaceSnapshotModels) []func() error {
	tasks := []func() error{
		namedLoadTask("workspaceSnapshot.runs", func() error {
			var err error
			models.Runs, err = readers.Runs.ListByWorkspaceID(ctx, workspaceID)
			return err
		}),
		namedLoadTask("workspaceSnapshot.results", func() error {
			var err error
			models.Results, err = readers.Results.ListByWorkspace(ctx, workspaceID)
			return err
		}),
		namedLoadTask("workspaceSnapshot.eventRecords", func() error {
			var err error
			models.EventRecords, err = readers.EventRecords.List(ctx, readmodelcore.WorkspaceEventRecordOwnerQuery(workspaceID, readmodelcore.DefaultEventRecordLimit))
			return err
		}),
	}
	if options.includeLLMUsagePayloads {
		tasks = append(tasks, namedLoadTask("workspaceSnapshot.llmUsageEventRecords", func() error {
			var err error
			models.LLMUsageEventRecords, err = readers.EventRecords.List(ctx, readmodelcore.WorkspaceLLMUsageEventRecordOwnerQuery(workspaceID, readmodelcore.DefaultLLMUsageEventRecordLimit))
			return err
		}))
	}
	tasks = append(tasks,
		namedLoadTask("workspaceSnapshot.artifacts", func() error {
			var err error
			models.Artifacts, err = readers.Artifacts.ListByWorkspace(ctx, workspaceID)
			return err
		}),
		namedLoadTask("workspaceSnapshot.tickets", func() error {
			var err error
			models.Tickets, err = readers.Tickets.ListByWorkspaceID(ctx, workspaceID)
			return err
		}),
		namedLoadTask("workspaceSnapshot.approvals", func() error {
			var err error
			models.ApprovalsRaw, err = readers.Approvals.ListByWorkspaceID(ctx, workspaceID)
			return err
		}),
	)
	tasks = appendWorkspaceSnapshotDefinitionLoadTasks(ctx, tasks, readers, workspaceID, models)
	tasks = append(tasks, loadWorkspaceSnapshotWorkspaceTask(ctx, readers, workspaceID, cachedRunSnapshot, models))
	return tasks
}

func loadWorkspaceSnapshotWorkspaceTask(ctx context.Context, readers WorkspaceSnapshotReaders, workspaceID common.WorkspaceID, cachedRunSnapshot *readmodelcore.RunSnapshot, models *workspaceSnapshotModels) func() error {
	return namedLoadTask("workspaceSnapshot.workspace."+string(workspaceID), func() error {
		item, err := readers.Workspaces.Get(ctx, workspaceID)
		if err == nil {
			models.Workspace = item
			return nil
		}
		if errors.Is(err, common.ErrNotFound) {
			if cached := cachedWorkspaceModel(workspaceID, cachedRunSnapshot); cached != nil {
				models.Workspace = cached
				return nil
			}
		}
		return err
	})
}

func cachedWorkspaceModel(workspaceID common.WorkspaceID, snapshot *readmodelcore.RunSnapshot) *workspacemodel.Workspace {
	if snapshot == nil || snapshot.Workspace.ID != string(workspaceID) {
		return nil
	}
	return &workspacemodel.Workspace{
		ID:                   common.WorkspaceID(snapshot.Workspace.ID),
		Title:                snapshot.Workspace.Title,
		Description:          snapshot.Workspace.Description,
		DeliverableDirectory: snapshot.Workspace.DeliverableDirectory,
		InternalDirectory:    snapshot.Workspace.InternalDirectory,
		State:                common.WorkspaceState(snapshot.Workspace.State),
		ActiveRunID:          common.RunID(snapshot.Workspace.ActiveRunID),
	}
}

func appendWorkspaceSnapshotDefinitionLoadTasks(ctx context.Context, tasks []func() error, readers WorkspaceSnapshotReaders, workspaceID common.WorkspaceID, models *workspaceSnapshotModels) []func() error {
	if readers.Goals != nil {
		tasks = append(tasks, namedLoadTask("workspaceSnapshot.goals", func() error {
			var err error
			models.Goals, err = readers.Goals.ListByWorkspaceID(ctx, workspaceID)
			return err
		}))
	}
	if readers.DoDs != nil {
		tasks = append(tasks, namedLoadTask("workspaceSnapshot.dods", func() error {
			var err error
			models.DoDs, err = readers.DoDs.ListByWorkspaceID(ctx, workspaceID)
			return err
		}))
	}
	return tasks
}
