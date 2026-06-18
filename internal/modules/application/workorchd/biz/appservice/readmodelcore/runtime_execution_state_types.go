package readmodelcore

type RuntimeExecutionState struct {
	StepID                string         `json:"stepId,omitempty"`
	StepKind              string         `json:"stepKind,omitempty"`
	StepState             string         `json:"stepState,omitempty"`
	RequestID             string         `json:"requestId,omitempty"`
	QueueKey              string         `json:"queueKey,omitempty"`
	RuntimeRecoveryPolicy map[string]any `json:"runtimeRecoveryPolicy,omitempty"`
	RecoverySafety        string         `json:"recoverySafety,omitempty"`
	AcceptedAt            string         `json:"acceptedAt,omitempty"`
	LastProgressAt        string         `json:"lastProgressAt,omitempty"`
	LastProgressEvent     string         `json:"lastProgressEvent,omitempty"`
	LastProgressStage     string         `json:"lastProgressStage,omitempty"`
	LastProgressSummary   string         `json:"lastProgressSummary,omitempty"`
	WorkerID              string         `json:"workerId,omitempty"`
	WorkerStartedAt       string         `json:"workerStartedAt,omitempty"`
	WorkerHeartbeatAt     string         `json:"workerHeartbeatAt,omitempty"`
	WorkerStoppedAt       string         `json:"workerStoppedAt,omitempty"`
	WorkerLostAt          string         `json:"workerLostAt,omitempty"`
	DoneAt                string         `json:"doneAt,omitempty"`
	FailedAt              string         `json:"failedAt,omitempty"`
	FinalizedAt           string         `json:"finalizedAt,omitempty"`
	FinalizeEvent         string         `json:"finalizeEvent,omitempty"`
	Pending               bool           `json:"pending,omitempty"`
	Gap                   string         `json:"gap,omitempty"`
}
