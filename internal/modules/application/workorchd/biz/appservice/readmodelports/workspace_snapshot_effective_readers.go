package readmodelports

func effectiveWorkspaceRunSnapshotReaders(readers WorkspaceSnapshotReaders) RunSnapshotReaders {
	ret := readers.RunSnapshotReaders
	if ret.Workspaces == nil {
		ret.Workspaces = readers.Workspaces
	}
	if ret.Runs == nil {
		ret.Runs = readers.Runs
	}
	if ret.Patches == nil {
		ret.Patches = readers.Patches
	}
	if ret.Results == nil {
		ret.Results = readers.Results
	}
	if ret.Artifacts == nil {
		ret.Artifacts = readers.Artifacts
	}
	if ret.ArtifactContinuations == nil {
		ret.ArtifactContinuations = readers.ArtifactContinuations
	}
	if ret.EventRecords == nil {
		ret.EventRecords = readers.EventRecords
	}
	if ret.Tickets == nil {
		ret.Tickets = readers.Tickets
	}
	if ret.Resolutions == nil {
		ret.Resolutions = readers.Resolutions
	}
	if ret.Approvals == nil {
		ret.Approvals = readers.Approvals
	}
	if ret.AutoGovernanceTraces == nil {
		ret.AutoGovernanceTraces = readers.AutoGovernanceTraces
	}
	return ret
}
