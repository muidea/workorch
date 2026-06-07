package readmodelports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type RunSnapshotReaders struct {
	Workspaces            readmodelcore.WorkspaceReader
	Runs                  readmodelcore.RunReader
	Steps                 readmodelcore.StepReader
	ToolCalls             readmodelcore.ToolCallReader
	Patches               readmodelcore.PatchReader
	Validations           readmodelcore.ValidationResultReader
	Checkpoints           readmodelcore.RuntimeCheckpointReader
	Results               readmodelcore.ResultReader
	Artifacts             readmodelcore.ArtifactReader
	ArtifactContinuations readmodelcore.ArtifactContinuationReader
	EventRecords          readmodelcore.EventRecordReader
	MemoryRecalls         readmodelcore.MemoryRecallTraceReader
	Tickets               readmodelcore.DecisionTicketReader
	Resolutions           readmodelcore.DecisionResolutionReader
	Approvals             readmodelcore.ApprovalReader
	AutoGovernanceTraces  readmodelcore.AutoGovernanceTraceReader
	MCPCapabilities       readmodelcore.MCPCapabilityReader
	Published             []events.WorkorchEvent
}

func RunSnapshot(ctx context.Context, readers RunSnapshotReaders, runID common.RunID) (readmodelcore.RunSnapshot, error) {
	return runSnapshot(ctx, readers, runID, false)
}

func RunRecoverySnapshot(ctx context.Context, readers RunSnapshotReaders, runID common.RunID) (readmodelcore.RunSnapshot, error) {
	return runSnapshot(ctx, readers, runID, true)
}

func runSnapshot(ctx context.Context, readers RunSnapshotReaders, runID common.RunID, recovery bool) (readmodelcore.RunSnapshot, error) {
	item, err := readmodelcore.GetRunFromReader(ctx, readers.Runs, runID)
	if err != nil {
		return readmodelcore.RunSnapshot{}, err
	}
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, item.WorkspaceID)
	models, err := loadRunSnapshotModels(ctx, readers, item, recovery)
	if err != nil {
		return readmodelcore.RunSnapshot{}, err
	}

	input := models.projectionInput(item)
	if recovery {
		return readmodelprojection.RunRecoverySnapshotFromModels(input), nil
	}
	input.SelectedTicket = models.Decisions.SelectedTicket
	input.SelectedResolution = models.Decisions.SelectedResolution
	input.SelectedApproval = models.Approvals.SelectedApproval
	input.PublishedEvents = readers.Published
	return readmodelprojection.RunSnapshotFromModels(input), nil
}
