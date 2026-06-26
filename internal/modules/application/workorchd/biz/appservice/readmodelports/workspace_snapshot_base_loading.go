package readmodelports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func workspaceSnapshotBaseLoadTasks(ctx context.Context, readers WorkspaceSnapshotReaders, workspaceID common.WorkspaceID, options workspaceSnapshotLoadOptions, models *workspaceSnapshotModels) []func() error {
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
			models.EventRecords, err = readers.EventRecords.List(ctx, readmodelcore.WorkspaceEventRecordRepositoryQuery(workspaceID, readmodelcore.DefaultEventRecordLimit))
			return err
		}),
	}
	if options.includeLLMUsagePayloads {
		tasks = append(tasks, namedLoadTask("workspaceSnapshot.llmUsageEventRecords", func() error {
			var err error
			models.LLMUsageEventRecords, err = readers.EventRecords.List(ctx, readmodelcore.WorkspaceLLMUsageEventRecordRepositoryQuery(workspaceID, readmodelcore.DefaultLLMUsageEventRecordLimit))
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
	if models.Workspace == nil {
		tasks = append(tasks, namedLoadTask("workspaceSnapshot.workspace."+string(workspaceID), func() error {
			var err error
			models.Workspace, err = readers.Workspaces.Get(ctx, workspaceID)
			return err
		}))
	}
	return tasks
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
