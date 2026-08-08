package workorchruntimecontract

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

type ArtifactContinuation struct {
	ID            common.ArtifactContinuationID `json:"id"`
	WorkspaceID   common.WorkspaceID            `json:"workspaceId"`
	RunID         common.RunID                  `json:"runId"`
	RootStepID    common.StepID                 `json:"rootStepId,omitempty"`
	CurrentStepID common.StepID                 `json:"currentStepId,omitempty"`
	ArtifactID    string                        `json:"artifactId"`
	TargetRef     string                        `json:"targetRef"`
	Operation     string                        `json:"operation,omitempty"`
	OutputMode    string                        `json:"outputMode,omitempty"`
	// Artifact chunking is audit/storage metadata and cannot select Runtime recovery.
	// These fields bind stored raw chunks to their sealed authority contract;
	// artifact storage never becomes a second action authority.
	AuthorityTransaction AuthorityTransaction             `json:"authorityTransaction,omitempty"`
	CandidateAttempt     int                              `json:"candidateAttempt,omitempty"`
	StablePrefixHash     string                           `json:"stablePrefixHash,omitempty"`
	MaxChunks            int                              `json:"maxChunks,omitempty"`
	TailBytes            int                              `json:"tailBytes,omitempty"`
	CoverageTargets      []string                         `json:"coverageTargets,omitempty"`
	State                common.ArtifactContinuationState `json:"state"`
	ExpectedSequence     int                              `json:"expectedSequence,omitempty"`
	ReceivedSequences    []int                            `json:"receivedSequences,omitempty"`
	ReceivedChunkHashes  map[int]string                   `json:"receivedChunkHashes,omitempty"`
	FinalReceived        bool                             `json:"finalReceived,omitempty"`
	BytesWritten         int64                            `json:"bytesWritten,omitempty"`
	RollingHash          string                           `json:"rollingHash,omitempty"`
	LastChunkHash        string                           `json:"lastChunkHash,omitempty"`
	PreviousChunkTail    string                           `json:"previousChunkTail,omitempty"`
	RetryCounters        map[string]int                   `json:"retryCounters,omitempty"`
	EvidenceRefs         []string                         `json:"evidenceRefs,omitempty"`
	CreatedAt            string                           `json:"createdAt,omitempty"`
	UpdatedAt            string                           `json:"updatedAt,omitempty"`
	Metadata             workorcheventpayload.Payload     `json:"metadata,omitempty"`
}

type ArtifactContinuationChunk struct {
	ContinuationID    common.ArtifactContinuationID `json:"continuationId"`
	Sequence          int                           `json:"sequence"`
	ArtifactID        string                        `json:"artifactId"`
	TargetRef         string                        `json:"targetRef"`
	ContentHash       string                        `json:"contentHash,omitempty"`
	Bytes             int64                         `json:"bytes,omitempty"`
	PreviousChunkTail string                        `json:"previousChunkTail,omitempty"`
	CurrentChunkTail  string                        `json:"currentChunkTail,omitempty"`
	LLMCallRefs       []string                      `json:"llmCallRefs,omitempty"`
	StepRefs          []common.StepID               `json:"stepRefs,omitempty"`
	Validation        workorcheventpayload.Payload  `json:"validation,omitempty"`
	CreatedAt         string                        `json:"createdAt,omitempty"`
	Metadata          workorcheventpayload.Payload  `json:"metadata,omitempty"`
}

type ArtifactContinuationAssembledMetadata struct {
	ContinuationID    common.ArtifactContinuationID `json:"continuationId"`
	TargetRef         string                        `json:"targetRef"`
	Bytes             int64                         `json:"bytes,omitempty"`
	RollingHash       string                        `json:"rollingHash,omitempty"`
	SourceChunkRefs   []string                      `json:"sourceChunkRefs,omitempty"`
	ApplyEvidenceRefs []string                      `json:"applyEvidenceRefs,omitempty"`
	CreatedAt         string                        `json:"createdAt,omitempty"`
	Metadata          workorcheventpayload.Payload  `json:"metadata,omitempty"`
}
