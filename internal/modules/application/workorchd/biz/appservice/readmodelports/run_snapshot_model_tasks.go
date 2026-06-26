package readmodelports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func appendRunSnapshotObservabilityTasks(ctx context.Context, tasks []func() error, readers RunSnapshotReaders, item *runmodel.Run, loadMode runSnapshotLoadMode, models *runSnapshotModels) []func() error {
	tasks = append(tasks, namedLoadTask("runSnapshot.eventRecords", func() error {
		var err error
		models.EventRecords, err = readers.EventRecords.List(ctx, runSnapshotEventRecordQuery(item.ID, loadMode))
		return err
	}))
	if loadMode != runSnapshotLoadCompact {
		tasks = append(tasks, namedLoadTask("runSnapshot.llmUsageEventRecords", func() error {
			var err error
			models.LLMUsageEventRecords, err = readers.EventRecords.List(ctx, readmodelcore.RunLLMUsageEventRecordRepositoryQuery(item.ID, readmodelcore.DefaultLLMUsageEventRecordLimit))
			return err
		}))
	}
	tasks = append(tasks, namedLoadTask("runSnapshot.memoryRecalls", func() error {
		var err error
		if readers.MemoryRecalls != nil {
			models.MemoryRecallTraces, err = readers.MemoryRecalls.List(ctx, readmodelcore.RunMemoryRecallTraceRepositoryQuery(item.WorkspaceID, item.ID))
		}
		return err
	}))
	return tasks
}

func runSnapshotEventRecordQuery(runID common.RunID, loadMode runSnapshotLoadMode) repository.EventRecordQuery {
	switch loadMode {
	case runSnapshotLoadObservability:
		return readmodelcore.RunObservabilityEventRecordRepositoryQuery(runID, readmodelcore.DefaultEventRecordLimit)
	default:
		return readmodelcore.RunProjectionEventRecordRepositoryQuery(runID, readmodelcore.DefaultEventRecordLimit)
	}
}

func appendRunSnapshotGovernanceTasks(ctx context.Context, tasks []func() error, readers RunSnapshotReaders, item *runmodel.Run, models *runSnapshotModels) []func() error {
	tasks = append(tasks, namedLoadTask("runSnapshot.decisions", func() error {
		var err error
		models.Decisions, err = DecisionsForRun(ctx, readers.Tickets, readers.Resolutions, readers.AutoGovernanceTraces, item)
		return err
	}))
	tasks = append(tasks, namedLoadTask("runSnapshot.approvals", func() error {
		var err error
		models.Approvals, err = ApprovalsForRun(ctx, readers.Approvals, readers.AutoGovernanceTraces, item)
		return err
	}))
	return tasks
}

func appendRunSnapshotCapabilityTasks(ctx context.Context, tasks []func() error, readers RunSnapshotReaders, recovery bool, models *runSnapshotModels) []func() error {
	if recovery || readers.MCPCapabilities == nil {
		return tasks
	}
	return append(tasks, namedLoadTask("runSnapshot.mcpCapabilities", func() error {
		var err error
		models.MCPCapabilities, err = readers.MCPCapabilities.List(ctx)
		return err
	}))
}
