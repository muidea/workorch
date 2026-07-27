package readmodelports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
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
	CoverageWorkItems     readmodelcore.CoverageWorkItemReader
	EventRecords          readmodelcore.EventRecordReader
	MemoryRecalls         readmodelcore.MemoryRecallTraceReader
	Tickets               readmodelcore.DecisionTicketReader
	Resolutions           readmodelcore.DecisionResolutionReader
	Approvals             readmodelcore.ApprovalReader
	AutoGovernanceTraces  readmodelcore.AutoGovernanceTraceReader
	MCPCapabilities       readmodelcore.MCPCapabilityReader
	Published             []eventstreamevents.EventRecordData
}

func RunSnapshot(ctx context.Context, readers RunSnapshotReaders, runID common.RunID) (readmodelcore.RunSnapshot, error) {
	return runSnapshot(ctx, readers, runID, runSnapshotLoadCompact, false)
}

func RunRecoverySnapshot(ctx context.Context, readers RunSnapshotReaders, runID common.RunID) (readmodelcore.RunSnapshot, error) {
	return runSnapshot(ctx, readers, runID, runSnapshotLoadCompact, true)
}

func runSnapshot(ctx context.Context, readers RunSnapshotReaders, runID common.RunID, loadMode runSnapshotLoadMode, recovery bool) (readmodelcore.RunSnapshot, error) {
	item, err := readmodelcore.GetRunFromReader(ctx, readers.Runs, runID)
	if err != nil {
		return readmodelcore.RunSnapshot{}, err
	}
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, item.WorkspaceID)
	models, err := loadRunSnapshotModels(ctx, readers, item, loadMode, recovery)
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
	ret := readmodelprojection.RunSnapshotFromModels(input)
	if loadMode == runSnapshotLoadViewCompact {
		ret = readmodelprojection.ProjectRunSnapshotCompact(ret)
	}
	return ret, nil
}
