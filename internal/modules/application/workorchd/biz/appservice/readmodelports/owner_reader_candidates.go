package readmodelports

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/events"
)

type OwnerReaderCandidates struct {
	Workspaces            readmodelcore.WorkspaceReader
	Goals                 readmodelcore.GoalReader
	DoDs                  readmodelcore.DoDReader
	Runs                  readmodelcore.RunReader
	Steps                 readmodelcore.StepReader
	Results               readmodelcore.ResultReader
	Artifacts             readmodelcore.ArtifactReader
	ArtifactContinuations readmodelcore.ArtifactContinuationReader
	CoverageWorkItems     readmodelcore.CoverageWorkItemReader
	EventRecords          readmodelcore.EventRecordReader
	MemoryRecalls         readmodelcore.MemoryRecallTraceReader
	ToolCalls             readmodelcore.ToolCallReader
	MCPCapabilities       readmodelcore.MCPCapabilityReader
	Patches               readmodelcore.PatchReader
	Validations           readmodelcore.ValidationResultReader
	Checkpoints           readmodelcore.RuntimeCheckpointReader
	AutoGovernanceTraces  readmodelcore.AutoGovernanceTraceReader
	Tickets               readmodelcore.DecisionTicketReader
	Resolutions           readmodelcore.DecisionResolutionReader
	Approvals             readmodelcore.ApprovalReader
}

func (c OwnerReaderCandidates) RunSnapshotReaders(owner OwnerReaders, published []events.WorkorchEvent) RunSnapshotReaders {
	if c.Matches(owner) {
		return owner.RunSnapshotReaders(published)
	}
	return RunSnapshotReaders{
		Workspaces:            c.Workspaces,
		Runs:                  c.Runs,
		Steps:                 c.Steps,
		ToolCalls:             c.ToolCalls,
		Patches:               c.Patches,
		Validations:           c.Validations,
		Checkpoints:           c.Checkpoints,
		Results:               c.Results,
		Artifacts:             c.Artifacts,
		ArtifactContinuations: c.ArtifactContinuations,
		CoverageWorkItems:     c.CoverageWorkItems,
		EventRecords:          c.EventRecords,
		MemoryRecalls:         c.MemoryRecalls,
		Tickets:               c.Tickets,
		Resolutions:           c.Resolutions,
		Approvals:             c.Approvals,
		AutoGovernanceTraces:  c.AutoGovernanceTraces,
		MCPCapabilities:       c.MCPCapabilities,
		Published:             published,
	}
}
