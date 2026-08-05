package workorchprojectioncontract

type ToolCall struct {
	ID          string         `json:"id"`
	WorkspaceID string         `json:"workspaceId"`
	RunID       string         `json:"runId"`
	StepID      string         `json:"stepId,omitempty"`
	ToolName    string         `json:"toolName"`
	Arguments   map[string]any `json:"arguments,omitempty"`
	State       string         `json:"state"`
	StartedAt   string         `json:"startedAt,omitempty"`
	EndedAt     string         `json:"endedAt,omitempty"`
	ExitSummary string         `json:"exitSummary,omitempty"`
	Output      map[string]any `json:"output,omitempty"`
	Error       string         `json:"error,omitempty"`
}

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

type Artifact struct {
	ID                 string         `json:"id"`
	WorkspaceID        string         `json:"workspaceId"`
	RunID              string         `json:"runId,omitempty"`
	ResultID           string         `json:"resultId,omitempty"`
	StepID             string         `json:"stepId,omitempty"`
	ToolCallID         string         `json:"toolCallId,omitempty"`
	PatchID            string         `json:"patchId,omitempty"`
	ValidationResultID string         `json:"validationResultId,omitempty"`
	ArtifactType       string         `json:"artifactType"`
	Category           string         `json:"category,omitempty"`
	Name               string         `json:"name,omitempty"`
	Summary            string         `json:"summary,omitempty"`
	URI                string         `json:"uri,omitempty"`
	ContentType        string         `json:"contentType,omitempty"`
	State              string         `json:"state"`
	EvidenceRefs       []string       `json:"evidenceRefs,omitempty"`
	Metadata           map[string]any `json:"metadata,omitempty"`
	CreatedAt          string         `json:"createdAt,omitempty"`
	UpdatedAt          string         `json:"updatedAt,omitempty"`
}

type ArtifactContinuation struct {
	ID                string         `json:"id"`
	WorkspaceID       string         `json:"workspaceId"`
	RunID             string         `json:"runId"`
	RootStepID        string         `json:"rootStepId,omitempty"`
	ArtifactID        string         `json:"artifactId"`
	TargetRef         string         `json:"targetRef"`
	Operation         string         `json:"operation,omitempty"`
	State             string         `json:"state"`
	ExpectedSequence  int            `json:"expectedSequence,omitempty"`
	ReceivedSequences []int          `json:"receivedSequences,omitempty"`
	FinalReceived     bool           `json:"finalReceived,omitempty"`
	BytesWritten      int64          `json:"bytesWritten,omitempty"`
	RollingHash       string         `json:"rollingHash,omitempty"`
	LastChunkHash     string         `json:"lastChunkHash,omitempty"`
	CoverageTargets   []string       `json:"coverageTargets,omitempty"`
	EvidenceRefs      []string       `json:"evidenceRefs,omitempty"`
	FailureReason     string         `json:"failureReason,omitempty"`
	CreatedAt         string         `json:"createdAt,omitempty"`
	UpdatedAt         string         `json:"updatedAt,omitempty"`
	Progress          map[string]any `json:"progress,omitempty"`
}

func CloneArtifacts(items []Artifact) []Artifact {
	if items == nil {
		return nil
	}
	ret := make([]Artifact, 0, len(items))
	for _, item := range items {
		item.EvidenceRefs = append([]string(nil), item.EvidenceRefs...)
		item.Metadata = cloneJSONMap(item.Metadata)
		ret = append(ret, item)
	}
	return ret
}

func CloneArtifactContinuations(items []ArtifactContinuation) []ArtifactContinuation {
	if items == nil {
		return nil
	}
	ret := make([]ArtifactContinuation, 0, len(items))
	for _, item := range items {
		item.ReceivedSequences = append([]int(nil), item.ReceivedSequences...)
		item.CoverageTargets = append([]string(nil), item.CoverageTargets...)
		item.EvidenceRefs = append([]string(nil), item.EvidenceRefs...)
		item.Progress = cloneJSONMap(item.Progress)
		ret = append(ret, item)
	}
	return ret
}

func ClonePatches(items []Patch) []Patch {
	if items == nil {
		return nil
	}
	ret := make([]Patch, 0, len(items))
	for _, item := range items {
		item.AffectedFiles = append([]string(nil), item.AffectedFiles...)
		item.RiskReasons = append([]string(nil), item.RiskReasons...)
		if item.Diagnostics != nil {
			diagnostics := make([]map[string]any, 0, len(item.Diagnostics))
			for _, diagnostic := range item.Diagnostics {
				diagnostics = append(diagnostics, cloneJSONMap(diagnostic))
			}
			item.Diagnostics = diagnostics
		}
		ret = append(ret, item)
	}
	return ret
}

func cloneJSONMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = cloneJSONValue(value)
	}
	return ret
}

func cloneJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneJSONMap(typed)
	case []any:
		ret := make([]any, len(typed))
		for index, item := range typed {
			ret[index] = cloneJSONValue(item)
		}
		return ret
	case []map[string]any:
		return cloneJSONMaps(typed)
	case []string:
		return append([]string(nil), typed...)
	case []int:
		return append([]int(nil), typed...)
	case []float64:
		return append([]float64(nil), typed...)
	case []bool:
		return append([]bool(nil), typed...)
	case map[string]string:
		ret := make(map[string]string, len(typed))
		for key, item := range typed {
			ret[key] = item
		}
		return ret
	case map[string]int:
		ret := make(map[string]int, len(typed))
		for key, item := range typed {
			ret[key] = item
		}
		return ret
	default:
		return typed
	}
}

type RuntimeCheckpoint struct {
	ID                 string         `json:"id"`
	WorkspaceID        string         `json:"workspaceId"`
	RunID              string         `json:"runId"`
	CheckpointType     string         `json:"checkpointType"`
	CurrentStepID      string         `json:"currentStepId,omitempty"`
	RunStateSnapshot   string         `json:"runStateSnapshot"`
	BlockingTicketID   string         `json:"blockingTicketId,omitempty"`
	BlockingApprovalID string         `json:"blockingApprovalId,omitempty"`
	ThreadRef          string         `json:"threadRef,omitempty"`
	OccurredAt         string         `json:"occurredAt,omitempty"`
	RestorableUntil    string         `json:"restorableUntil,omitempty"`
	Data               map[string]any `json:"data,omitempty"`
}
