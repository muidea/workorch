package events

const (
	TopicAcquireRuntimeCapabilities           = "/mcp/runtime-prerequisite/acquire-capabilities"
	TopicAcquireRuntimeOperationAdmissibility = "/mcp/runtime-prerequisite/acquire-operation-admissibility"
	TopicProbeRuntimeOperationFeasibility     = "/mcp/runtime-operation/probe-feasibility"
	TopicProbeRuntimeSemanticProofFeasibility = "/mcp/runtime-semantic/probe-proof-feasibility"
	TopicQueryRuntimeCWIReconciliation        = "/mcp/reconciliation/query-cwi"
	TopicExecuteOperation                     = "/mcp/runtime-operation/execute"
)
