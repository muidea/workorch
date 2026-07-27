package readmodelports

import eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"

func (c OwnerReaderCandidates) WorkspaceSnapshotReaders(owner OwnerReaders, published []eventstreamevents.EventRecordData) WorkspaceSnapshotReaders {
	if c.Matches(owner) {
		return owner.WorkspaceSnapshotReaders(published)
	}
	return WorkspaceSnapshotReaders{
		Workspaces:            c.Workspaces,
		Goals:                 c.Goals,
		DoDs:                  c.DoDs,
		Runs:                  c.Runs,
		Results:               c.Results,
		Artifacts:             c.Artifacts,
		ArtifactContinuations: c.ArtifactContinuations,
		CoverageWorkItems:     c.CoverageWorkItems,
		EventRecords:          c.EventRecords,
		Patches:               c.Patches,
		Tickets:               c.Tickets,
		Resolutions:           c.Resolutions,
		Approvals:             c.Approvals,
		AutoGovernanceTraces:  c.AutoGovernanceTraces,
		RunSnapshotReaders:    c.RunSnapshotReaders(owner, published),
	}
}
