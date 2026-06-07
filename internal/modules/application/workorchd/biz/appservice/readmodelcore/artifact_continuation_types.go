package readmodelcore

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
