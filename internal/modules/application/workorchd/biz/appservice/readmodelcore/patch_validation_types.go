package readmodelcore

type Patch struct {
	ID               string           `json:"id"`
	WorkspaceID      string           `json:"workspaceId"`
	RunID            string           `json:"runId"`
	StepID           string           `json:"stepId,omitempty"`
	ToolCallID       string           `json:"toolCallId,omitempty"`
	ApprovalID       string           `json:"approvalId,omitempty"`
	RevertApprovalID string           `json:"revertApprovalId,omitempty"`
	PatchType        string           `json:"patchType"`
	TargetRef        string           `json:"targetRef,omitempty"`
	Summary          string           `json:"summary,omitempty"`
	State            string           `json:"state"`
	AddedLines       int              `json:"addedLines,omitempty"`
	DeletedLines     int              `json:"deletedLines,omitempty"`
	BeforeHash       string           `json:"beforeHash,omitempty"`
	AfterHash        string           `json:"afterHash,omitempty"`
	AffectedFiles    []string         `json:"affectedFiles,omitempty"`
	RiskLevel        string           `json:"riskLevel,omitempty"`
	RiskReasons      []string         `json:"riskReasons,omitempty"`
	Diagnostics      []map[string]any `json:"diagnostics,omitempty"`
	CreatedAt        string           `json:"createdAt,omitempty"`
	AppliedAt        string           `json:"appliedAt,omitempty"`
	RevertedAt       string           `json:"revertedAt,omitempty"`
}

type ValidationResult struct {
	ID              string   `json:"id"`
	WorkspaceID     string   `json:"workspaceId"`
	RunID           string   `json:"runId"`
	StepID          string   `json:"stepId,omitempty"`
	ValidationKind  string   `json:"validationKind"`
	TargetRef       string   `json:"targetRef,omitempty"`
	State           string   `json:"state"`
	Summary         string   `json:"summary,omitempty"`
	EvidenceRef     string   `json:"evidenceRef,omitempty"`
	EvidenceQuality string   `json:"evidenceQuality,omitempty"`
	EvidenceReason  string   `json:"evidenceReason,omitempty"`
	ZeroWork        bool     `json:"zeroWork,omitempty"`
	EvidenceTags    []string `json:"evidenceTags,omitempty"`
	StartedAt       string   `json:"startedAt,omitempty"`
	EndedAt         string   `json:"endedAt,omitempty"`
}
