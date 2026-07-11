package readmodelports

import (
	eventstreamevents "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/events"
)

type OwnerReaders struct {
	Workspaces            WorkspaceReader
	Goals                 GoalReader
	DoDs                  DoDReader
	Runs                  RunReader
	Steps                 StepReader
	Results               ResultReader
	Artifacts             ArtifactReader
	ArtifactContinuations ArtifactContinuationReader
	CoverageWorkItems     CoverageWorkItemReader
	EventRecords          EventRecordReader
	MemoryRecalls         MemoryRecallTraceReader
	ToolCalls             ToolCallReader
	MCPCapabilities       MCPCapabilityReader
	Patches               PatchReader
	Validations           ValidationResultReader
	Checkpoints           RuntimeCheckpointReader
	AutoGovernanceTraces  AutoGovernanceTraceReader
	Tickets               DecisionTicketReader
	Resolutions           DecisionResolutionReader
	Approvals             ApprovalReader
}

func (r OwnerReaders) RunSnapshotReaders(published []eventstreamevents.EventRecordData) RunSnapshotReaders {
	return RunSnapshotReaders{
		Workspaces:            r.Workspaces,
		Runs:                  r.Runs,
		Steps:                 r.Steps,
		ToolCalls:             r.ToolCalls,
		Patches:               r.Patches,
		Validations:           r.Validations,
		Checkpoints:           r.Checkpoints,
		Results:               r.Results,
		Artifacts:             r.Artifacts,
		ArtifactContinuations: r.ArtifactContinuations,
		CoverageWorkItems:     r.CoverageWorkItems,
		EventRecords:          r.EventRecords,
		MemoryRecalls:         r.MemoryRecalls,
		Tickets:               r.Tickets,
		Resolutions:           r.Resolutions,
		Approvals:             r.Approvals,
		AutoGovernanceTraces:  r.AutoGovernanceTraces,
		MCPCapabilities:       r.MCPCapabilities,
		Published:             published,
	}
}

func (r OwnerReaders) WorkspaceSnapshotReaders(published []eventstreamevents.EventRecordData) WorkspaceSnapshotReaders {
	return WorkspaceSnapshotReaders{
		Workspaces:            r.Workspaces,
		Goals:                 r.Goals,
		DoDs:                  r.DoDs,
		Runs:                  r.Runs,
		Results:               r.Results,
		Artifacts:             r.Artifacts,
		ArtifactContinuations: r.ArtifactContinuations,
		CoverageWorkItems:     r.CoverageWorkItems,
		EventRecords:          r.EventRecords,
		Patches:               r.Patches,
		Tickets:               r.Tickets,
		Resolutions:           r.Resolutions,
		Approvals:             r.Approvals,
		AutoGovernanceTraces:  r.AutoGovernanceTraces,
		RunSnapshotReaders:    r.RunSnapshotReaders(published),
	}
}

func (r OwnerReaders) RunStatusReaders() RunStatusReaders {
	return RunStatusReaders{
		Runs:        r.Runs,
		Steps:       r.Steps,
		Checkpoints: r.Checkpoints,
		Tickets:     r.Tickets,
		Approvals:   r.Approvals,
	}
}
