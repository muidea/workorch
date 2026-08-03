package events

const (
	TopicAcquireRuntimeCapabilities           = "/ssh/runtime-prerequisite/acquire-capabilities"
	TopicAcquireRuntimeOperationAdmissibility = "/ssh/runtime-prerequisite/acquire-operation-admissibility"
	TopicProbeRuntimeOperationFeasibility     = "/ssh/runtime-operation/probe-feasibility"
	TopicProbeRuntimeSemanticProofFeasibility = "/ssh/runtime-semantic/probe-proof-feasibility"
	TopicExecuteOperation                     = "/ssh/runtime-operation/execute"
)
