package run

import "github.com/muidea/workorch/internal/pkg/common"

type ValidationResult struct {
	ID              common.ValidationResultID
	WorkspaceID     common.WorkspaceID
	RunID           common.RunID
	StepID          common.StepID
	ValidationKind  string
	TargetRef       string
	State           common.ValidationResultState
	Summary         string
	EvidenceRef     string
	EvidenceQuality common.EvidenceQuality
	EvidenceReason  string
	ZeroWork        bool
	EvidenceTags    []string
	StartedAt       string
	EndedAt         string
}
