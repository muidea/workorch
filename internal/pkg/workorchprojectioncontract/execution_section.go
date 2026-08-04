package workorchprojectioncontract

type ExecutionSection struct {
	ID                 string   `json:"id"`
	WorkspaceID        string   `json:"workspaceId"`
	RunID              string   `json:"runId"`
	Title              string   `json:"title"`
	State              string   `json:"state"`
	GraphRootStepID    string   `json:"graphRootStepId,omitempty"`
	GraphNodeID        string   `json:"graphNodeId,omitempty"`
	StepID             string   `json:"stepId,omitempty"`
	StepKind           string   `json:"stepKind,omitempty"`
	RelatedDoDItems    []string `json:"relatedDoDItems,omitempty"`
	PlannedStepRefs    []string `json:"plannedStepRefs,omitempty"`
	ActualEvidenceRefs []string `json:"actualEvidenceRefs,omitempty"`
	DecisionRefs       []string `json:"decisionRefs,omitempty"`
	ApprovalRefs       []string `json:"approvalRefs,omitempty"`
	ToolCallRefs       []string `json:"toolCallRefs,omitempty"`
	PatchRefs          []string `json:"patchRefs,omitempty"`
	ValidationRefs     []string `json:"validationRefs,omitempty"`
	ResultRefs         []string `json:"resultRefs,omitempty"`
	ArtifactRefs       []string `json:"artifactRefs,omitempty"`
	MemoryRecallRefs   []string `json:"memoryRecallRefs,omitempty"`
	MemoryIDs          []string `json:"memoryIds,omitempty"`
	Summary            string   `json:"summary,omitempty"`
}
