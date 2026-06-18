package readmodelcore

type RunStatus struct {
	ID                   string                 `json:"id"`
	WorkspaceID          string                 `json:"workspaceId"`
	State                string                 `json:"state"`
	CurrentStepID        string                 `json:"currentStepId,omitempty"`
	CurrentStepKind      string                 `json:"currentStepKind,omitempty"`
	CurrentStepState     string                 `json:"currentStepState,omitempty"`
	StepCount            int                    `json:"stepCount"`
	DecisionCount        int                    `json:"decisionCount"`
	ApprovalCount        int                    `json:"approvalCount"`
	BlockingTicketID     string                 `json:"blockingTicketId,omitempty"`
	BlockingApprovalID   string                 `json:"blockingApprovalId,omitempty"`
	FailureReason        string                 `json:"failureReason,omitempty"`
	RuntimeExecution     *RuntimeExecutionState `json:"runtimeExecution,omitempty"`
	SchedulerDiagnostics *SchedulerDiagnostics  `json:"schedulerDiagnostics,omitempty"`
}
