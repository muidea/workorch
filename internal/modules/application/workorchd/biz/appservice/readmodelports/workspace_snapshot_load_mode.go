package readmodelports

type workspaceSnapshotLoadMode string

const (
	workspaceSnapshotLoadSummary       workspaceSnapshotLoadMode = "summary"
	workspaceSnapshotLoadDetail        workspaceSnapshotLoadMode = "detail"
	workspaceSnapshotLoadObservability workspaceSnapshotLoadMode = "observability"
)

type workspaceSnapshotLoadOptions struct {
	mode                         workspaceSnapshotLoadMode
	includeLLMUsagePayloads      bool
	includeRunDerivedCollections bool
	includeProjectionRunSnapshot bool
	projectionRunLoadMode        runSnapshotLoadMode
}

func workspaceSnapshotObservabilityLoadOptions() workspaceSnapshotLoadOptions {
	return workspaceSnapshotLoadOptions{
		mode:                         workspaceSnapshotLoadObservability,
		includeLLMUsagePayloads:      true,
		includeRunDerivedCollections: true,
		includeProjectionRunSnapshot: true,
		projectionRunLoadMode:        runSnapshotLoadProjection,
	}
}

func workspaceSnapshotDetailLoadOptions() workspaceSnapshotLoadOptions {
	return workspaceSnapshotLoadOptions{
		mode:                         workspaceSnapshotLoadDetail,
		includeLLMUsagePayloads:      false,
		includeRunDerivedCollections: false,
		includeProjectionRunSnapshot: true,
		projectionRunLoadMode:        runSnapshotLoadViewCompact,
	}
}

func workspaceSnapshotSummaryLoadOptions() workspaceSnapshotLoadOptions {
	return workspaceSnapshotLoadOptions{
		mode:                         workspaceSnapshotLoadSummary,
		includeLLMUsagePayloads:      false,
		includeRunDerivedCollections: false,
		includeProjectionRunSnapshot: false,
		projectionRunLoadMode:        runSnapshotLoadViewCompact,
	}
}
