package readmodelcore

type WorkspaceSnapshot struct {
	Workspace               Workspace               `json:"workspace"`
	Run                     *Run                    `json:"run,omitempty"`
	Step                    *StepView               `json:"step,omitempty"`
	Result                  *Result                 `json:"result,omitempty"`
	Outcome                 WorkspaceOutcome        `json:"outcome"`
	TaskProgress            TaskProgress            `json:"taskProgress"`
	RunHeartbeat            RunHeartbeat            `json:"runHeartbeat,omitempty"`
	RuntimeExecution        *RuntimeExecutionState  `json:"runtimeExecution,omitempty"`
	RunCommand              *RunCommandStatus       `json:"runCommand,omitempty"`
	SchedulerDiagnostics    *SchedulerDiagnostics   `json:"schedulerDiagnostics,omitempty"`
	ContextBundle           *ContextBundle          `json:"contextBundle,omitempty"`
	CoverageSummary         map[string]any          `json:"coverageSummary,omitempty"`
	GapBaseline             *GapBaseline            `json:"gapBaseline,omitempty"`
	Acceptance              AcceptanceProjection    `json:"acceptance"`
	TaskDecompositions      []TaskDecomposition     `json:"taskDecompositions,omitempty"`
	TaskPlans               []TaskPlan              `json:"taskPlans,omitempty"`
	ExecutionSections       []ExecutionSection      `json:"executionSections,omitempty"`
	MemoryRecallTraces      []MemoryRecallTrace     `json:"memoryRecallTraces,omitempty"`
	Goals                   []Goal                  `json:"goals,omitempty"`
	DoDs                    []DoD                   `json:"dods,omitempty"`
	Runs                    []Run                   `json:"runs,omitempty"`
	Results                 []Result                `json:"results,omitempty"`
	Artifacts               []Artifact              `json:"artifacts,omitempty"`
	ArtifactContinuations   []ArtifactContinuation  `json:"artifactContinuations,omitempty"`
	Patches                 []Patch                 `json:"patches,omitempty"`
	WebSources              []WebSourceTrace        `json:"webSources,omitempty"`
	LLMContextBudgets       []LLMContextBudgetTrace `json:"llmContextBudgets,omitempty"`
	LLMContextBudgetSummary LLMContextBudgetSummary `json:"llmContextBudgetSummary"`
	LLMUsageTraces          []LLMUsageTrace         `json:"llmUsageTraces,omitempty"`
	LLMUsageSummary         LLMUsageSummary         `json:"llmUsageSummary"`
	EventRecords            []EventRecord           `json:"eventRecords,omitempty"`
	Decisions               []Decision              `json:"decisions,omitempty"`
	Approvals               []Approval              `json:"approvals,omitempty"`
}

type RunCommandStatus struct {
	WorkspaceID        string `json:"workspaceId,omitempty"`
	RunID              string `json:"runId,omitempty"`
	State              string `json:"state,omitempty"`
	Intent             string `json:"intent,omitempty"`
	Reason             string `json:"reason,omitempty"`
	RequestID          string `json:"requestId,omitempty"`
	IdempotencyKey     string `json:"idempotencyKey,omitempty"`
	Source             string `json:"source,omitempty"`
	DefinitionRevision string `json:"definitionRevision,omitempty"`
	DecisionID         string `json:"decisionId,omitempty"`
	AcceptedAt         string `json:"acceptedAt,omitempty"`
	RejectedAt         string `json:"rejectedAt,omitempty"`
	Error              string `json:"error,omitempty"`
	Revision           int64  `json:"revision,omitempty"`
}

type WorkspaceExecutionActivation struct {
	Workspace Workspace         `json:"workspace"`
	Run       Run               `json:"run"`
	Snapshot  WorkspaceSnapshot `json:"snapshot"`
}
