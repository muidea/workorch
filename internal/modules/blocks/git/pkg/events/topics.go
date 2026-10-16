package events

const (
	TopicAcquireRuntimeCapabilities           = "/git/runtime-prerequisite/acquire-capabilities"
	TopicAcquireRuntimeOperationAdmissibility = "/git/runtime-prerequisite/acquire-operation-admissibility"
	TopicProbeRuntimeOperationFeasibility     = "/git/runtime-operation/probe-feasibility"
	TopicProbeRuntimeSemanticProofFeasibility = "/git/runtime-semantic/probe-proof-feasibility"
	TopicQueryRuntimeCWIReconciliation        = "/git/reconciliation/query-cwi"
	TopicAcquireCapabilityState               = "/git/runtime-prerequisite/acquire-capability-state"
	TopicExecuteOperation                     = "/git/runtime-operation/execute"
	TopicQueryRuntimeCommitSnapshot           = "/git/runtime-operation/query-commit-snapshot"
)
