package readmodelports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
)

func appendRunSnapshotObservabilityTasks(ctx context.Context, tasks []func() error, readers RunSnapshotReaders, item *runmodel.Run, models *runSnapshotModels) []func() error {
	tasks = append(tasks, func() error {
		var err error
		models.EventRecords, err = readers.EventRecords.List(ctx, readmodelcore.RunObservabilityEventRecordRepositoryQuery(item.ID, readmodelcore.DefaultEventRecordLimit))
		return err
	})
	tasks = append(tasks, func() error {
		var err error
		models.LLMUsageEventRecords, err = readers.EventRecords.List(ctx, readmodelcore.RunLLMUsageEventRecordRepositoryQuery(item.ID, readmodelcore.DefaultLLMUsageEventRecordLimit))
		return err
	})
	tasks = append(tasks, func() error {
		var err error
		if readers.MemoryRecalls != nil {
			models.MemoryRecallTraces, err = readers.MemoryRecalls.List(ctx, readmodelcore.RunMemoryRecallTraceRepositoryQuery(item.WorkspaceID, item.ID))
		}
		return err
	})
	return tasks
}

func appendRunSnapshotGovernanceTasks(ctx context.Context, tasks []func() error, readers RunSnapshotReaders, item *runmodel.Run, models *runSnapshotModels) []func() error {
	tasks = append(tasks, func() error {
		var err error
		models.Decisions, err = DecisionsForRun(ctx, readers.Tickets, readers.Resolutions, readers.AutoGovernanceTraces, item)
		return err
	})
	tasks = append(tasks, func() error {
		var err error
		models.Approvals, err = ApprovalsForRun(ctx, readers.Approvals, readers.AutoGovernanceTraces, item)
		return err
	})
	return tasks
}

func appendRunSnapshotCapabilityTasks(ctx context.Context, tasks []func() error, readers RunSnapshotReaders, recovery bool, models *runSnapshotModels) []func() error {
	if recovery || readers.MCPCapabilities == nil {
		return tasks
	}
	return append(tasks, func() error {
		var err error
		models.MCPCapabilities, err = readers.MCPCapabilities.List(ctx)
		return err
	})
}
