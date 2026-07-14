package run

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

type PatchCoverageTarget struct {
	GoalRequirementID string `json:"goalRequirementId,omitempty"`
	TargetText        string `json:"targetText,omitempty"`
}

type Patch struct {
	ID               common.PatchID
	WorkspaceID      common.WorkspaceID
	RunID            common.RunID
	StepID           common.StepID
	ToolCallID       common.ToolCallID
	ApprovalID       common.ApprovalID
	PatchType        string
	TargetRef        string
	Content          string
	Diff             string
	Summary          string
	CoverageTargets  []PatchCoverageTarget
	State            common.PatchState
	BeforeHash       string
	AfterHash        string
	AffectedFiles    []string
	AddedLines       int
	DeletedLines     int
	RiskLevel        string
	RiskReasons      []string
	Diagnostics      []workorcheventpayload.Payload
	OriginalContent  string
	OriginalExists   bool
	ReverseDiff      string
	RevertApprovalID common.ApprovalID
	RevertedAt       string
	CreatedAt        string
	AppliedAt        string
}
