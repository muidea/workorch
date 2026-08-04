package models

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

// ToolCall, Patch and ValidationResult are Orchestrator-owned evidence
// persistence models. Run owns lifecycle and opaque aggregate persistence; it
// does not own Runtime execution evidence.
type ToolCall struct {
	ID          common.ToolCallID
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	StepID      common.StepID
	ToolName    string
	Arguments   workorcheventpayload.Payload
	State       common.ToolCallState
	StartedAt   string
	EndedAt     string
	ExitSummary string
	Output      workorcheventpayload.Payload
	Error       string
}

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

func (t ToolCall) Clone() ToolCall {
	t.Arguments = t.Arguments.Clone()
	t.Output = t.Output.Clone()
	return t
}

func (p Patch) Clone() Patch {
	if p.CoverageTargets != nil {
		p.CoverageTargets = append(make([]PatchCoverageTarget, 0, len(p.CoverageTargets)), p.CoverageTargets...)
	}
	if p.AffectedFiles != nil {
		p.AffectedFiles = append(make([]string, 0, len(p.AffectedFiles)), p.AffectedFiles...)
	}
	if p.RiskReasons != nil {
		p.RiskReasons = append(make([]string, 0, len(p.RiskReasons)), p.RiskReasons...)
	}
	if p.Diagnostics != nil {
		diagnostics := p.Diagnostics
		p.Diagnostics = make([]workorcheventpayload.Payload, len(diagnostics))
		for idx := range diagnostics {
			p.Diagnostics[idx] = diagnostics[idx].Clone()
		}
	}
	return p
}

func (v ValidationResult) Clone() ValidationResult {
	if v.EvidenceTags != nil {
		v.EvidenceTags = append(make([]string, 0, len(v.EvidenceTags)), v.EvidenceTags...)
	}
	return v
}

func CloneToolCalls(items []*ToolCall) []*ToolCall {
	if items == nil {
		return nil
	}
	cloned := make([]*ToolCall, len(items))
	for idx, item := range items {
		if item != nil {
			value := item.Clone()
			cloned[idx] = &value
		}
	}
	return cloned
}

func ClonePatches(items []*Patch) []*Patch {
	if items == nil {
		return nil
	}
	cloned := make([]*Patch, len(items))
	for idx, item := range items {
		if item != nil {
			value := item.Clone()
			cloned[idx] = &value
		}
	}
	return cloned
}

func CloneValidationResults(items []*ValidationResult) []*ValidationResult {
	if items == nil {
		return nil
	}
	cloned := make([]*ValidationResult, len(items))
	for idx, item := range items {
		if item != nil {
			value := item.Clone()
			cloned[idx] = &value
		}
	}
	return cloned
}
