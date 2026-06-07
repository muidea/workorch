package readmodelports

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

type WorkspaceSnapshotReaders struct {
	Workspaces            readmodelcore.WorkspaceReader
	Goals                 readmodelcore.GoalReader
	DoDs                  readmodelcore.DoDReader
	Runs                  readmodelcore.RunReader
	Results               readmodelcore.ResultReader
	Artifacts             readmodelcore.ArtifactReader
	ArtifactContinuations readmodelcore.ArtifactContinuationReader
	EventRecords          readmodelcore.EventRecordReader
	Patches               readmodelcore.PatchReader
	Tickets               readmodelcore.DecisionTicketReader
	Resolutions           readmodelcore.DecisionResolutionReader
	Approvals             readmodelcore.ApprovalReader
	AutoGovernanceTraces  readmodelcore.AutoGovernanceTraceReader
	RunSnapshotReaders    RunSnapshotReaders
}
