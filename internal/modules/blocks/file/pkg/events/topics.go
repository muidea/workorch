package events

const (
	PrerequisiteCommandDestination = "/runtime/prerequisite"

	TopicAcquireRuntimeCapabilities           = "/file/runtime-prerequisite/acquire-capabilities"
	TopicAcquireRuntimeOperationAdmissibility = "/file/runtime-prerequisite/acquire-operation-admissibility"
	TopicProbeRuntimeOperationFeasibility     = "/file/runtime-operation/probe-feasibility"
	TopicProbeRuntimeSemanticProofFeasibility = "/file/runtime-semantic/probe-proof-feasibility"
	TopicQueryRuntimeCWIReconciliation        = "/file/reconciliation/query-cwi"
	TopicAcquireTargetSnapshot                = "/file/runtime-prerequisite/acquire-target-snapshot"
	TopicAcquireRuntimeTargetSelection        = "/file/runtime-prerequisite/acquire-target-selection"
	TopicAcquireWorkspaceManifest             = "/file/runtime-prerequisite/acquire-workspace-manifest"
	TopicConfirmRuntimeRepairSurface          = "/file/runtime-query/confirm-repair-surface"

	TopicExecuteFileListOperation     = "/file/runtime-operation/list"
	TopicExecuteFileReadOperation     = "/file/runtime-operation/read"
	TopicExecuteFileEditOperation     = "/file/runtime-operation/edit"
	TopicExecuteFileWriteOperation    = "/file/runtime-operation/write"
	TopicExecutePatchApplyOperation   = "/file/runtime-operation/patch-apply"
	TopicExecutePatchPreviewOperation = "/file/runtime-operation/patch-preview"
)
