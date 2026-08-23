package workorchprojectioncontract

type RunObservability RunObservabilityOf[Run]

// RunObservabilityOf is the single structural definition for run
// observability. Application-private read inputs instantiate R with their
// private run view; protocol output uses the canonical Run projection.
type RunObservabilityOf[R any] struct {
	Run                     R                       `json:"run"`
	DisplayOutcome          RunDisplayOutcome       `json:"displayOutcome"`
	GovernanceDisplay       GovernanceDisplay       `json:"governanceDisplay,omitempty"`
	ConsistencyDiagnostic   *ConsistencyDiagnostic  `json:"consistencyDiagnostic,omitempty"`
	Progress                RunProgress             `json:"progress"`
	TaskProgress            TaskProgress            `json:"taskProgress"`
	RepairLoop              RepairLoop              `json:"repairLoop"`
	Acceptance              AcceptanceProjection    `json:"acceptance"`
	TaskDecompositions      []TaskDecomposition     `json:"taskDecompositions,omitempty"`
	TaskPlans               []TaskPlan              `json:"taskPlans,omitempty"`
	ExecutionSections       []ExecutionSection      `json:"executionSections,omitempty"`
	LLMContextBudgets       []LLMContextBudgetTrace `json:"llmContextBudgets,omitempty"`
	LLMContextBudgetSummary LLMContextBudgetSummary `json:"llmContextBudgetSummary"`
	LLMUsageTraces          []LLMUsageTrace         `json:"llmUsageTraces,omitempty"`
	LLMUsageSummary         LLMUsageSummary         `json:"llmUsageSummary"`
	MemoryRecallTraces      []MemoryRecallTrace     `json:"memoryRecallTraces,omitempty"`
	GitOperations           []GitOperationTrace     `json:"gitOperations,omitempty"`
	WebSources              []WebSourceTrace        `json:"webSources,omitempty"`
	ClosureTraces           []ClosureTrace          `json:"closureTraces,omitempty"`
	DoDEvidence             []DoDEvidenceTrace      `json:"doDEvidence,omitempty"`
	ResultQuality           []ResultQualityTrace    `json:"resultQuality,omitempty"`
	Evidence                RuntimeEvidenceSummary  `json:"evidence"`
	LatestCheckpoint        *RuntimeCheckpoint      `json:"latestCheckpoint,omitempty"`
	SelectedCapabilities    []RuntimeCapability     `json:"selectedCapabilities,omitempty"`
	UsedCapabilities        []RuntimeCapability     `json:"usedCapabilities,omitempty"`
	Timeline                []RuntimeTimelineItem   `json:"timeline,omitempty"`
}
