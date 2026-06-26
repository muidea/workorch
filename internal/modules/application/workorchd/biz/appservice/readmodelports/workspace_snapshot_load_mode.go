package readmodelports

type workspaceSnapshotLoadMode string

const (
	workspaceSnapshotLoadDetail     workspaceSnapshotLoadMode = "detail"
	workspaceSnapshotLoadProjection workspaceSnapshotLoadMode = "projection"
)

type workspaceSnapshotLoadOptions struct {
	mode                    workspaceSnapshotLoadMode
	includeLLMUsagePayloads bool
	projectionRunLoadMode   runSnapshotLoadMode
}

func workspaceSnapshotDetailLoadOptions() workspaceSnapshotLoadOptions {
	return workspaceSnapshotLoadOptions{
		mode:                    workspaceSnapshotLoadDetail,
		includeLLMUsagePayloads: true,
		projectionRunLoadMode:   runSnapshotLoadProjection,
	}
}

func workspaceSnapshotProjectionLoadOptions() workspaceSnapshotLoadOptions {
	return workspaceSnapshotLoadOptions{
		mode:                    workspaceSnapshotLoadProjection,
		includeLLMUsagePayloads: false,
		projectionRunLoadMode:   runSnapshotLoadViewCompact,
	}
}
