package events

const (
	TopicAcquireRuntimeCapabilities                   = "/shell/runtime-prerequisite/acquire-capabilities"
	TopicAcquireRuntimeOperationAdmissibility         = "/shell/runtime-prerequisite/acquire-operation-admissibility"
	TopicProbeRuntimeOperationFeasibility             = "/shell/runtime-operation/probe-feasibility"
	TopicProbeRuntimeSemanticProofFeasibility         = "/shell/runtime-semantic/probe-proof-feasibility"
	TopicProbeRuntimeOwnerFactProspectiveSatisfaction = "/shell/runtime-owner-fact/probe-prospective-satisfaction"
	TopicQueryRuntimeCWIReconciliation                = "/shell/reconciliation/query-cwi"
	TopicExecuteOperation                             = "/shell/runtime-operation/execute"
)
