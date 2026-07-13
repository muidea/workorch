package workorchruntimecontract

import "github.com/muidea/workorch/internal/pkg/common"

const EvidenceRecordSchema = "workorch.evidence_record.v1"

const (
	EvidenceKindStateChange    = "state_change"
	EvidenceKindGovernance     = "governance"
	EvidenceKindValidation     = "validation"
	EvidenceKindAssessment     = "assessment"
	EvidenceKindArtifact       = "artifact"
	EvidenceKindTargetSnapshot = "target_snapshot"
	EvidenceKindDecision       = "decision"
	EvidenceKindApproval       = "approval"
	EvidenceKindDiagnostic     = "diagnostic"
)

const (
	EvidenceStatusAccepted   = "accepted"
	EvidenceStatusRejected   = "rejected"
	EvidenceStatusSuperseded = "superseded"
)

type EvidenceRecord struct {
	ID                 common.EvidenceRecordID   `json:"id"`
	WorkspaceID        common.WorkspaceID        `json:"workspaceId"`
	RunID              common.RunID              `json:"runId"`
	StepID             common.StepID             `json:"stepId,omitempty"`
	DefinitionRevision string                    `json:"definitionRevision,omitempty"`
	Kind               string                    `json:"kind"`
	Status             string                    `json:"status"`
	SourceType         string                    `json:"sourceType,omitempty"`
	SourceRef          string                    `json:"sourceRef"`
	SubjectType        string                    `json:"subjectType,omitempty"`
	SubjectID          string                    `json:"subjectId,omitempty"`
	CoverageItemRefs   []string                  `json:"coverageItemRefs,omitempty"`
	Supersedes         []common.EvidenceRecordID `json:"supersedes,omitempty"`
	CreatedAt          string                    `json:"createdAt,omitempty"`
	UpdatedAt          string                    `json:"updatedAt,omitempty"`
	Metadata           map[string]any            `json:"metadata,omitempty"`
}
