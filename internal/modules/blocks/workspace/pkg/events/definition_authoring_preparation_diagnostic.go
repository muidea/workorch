package events

import (
	"reflect"

	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const TopicWorkspacePreparationDiagnosticRecord = "/workspace/definition-authoring/preparation-diagnostic/record"
const TopicWorkspacePreparationDiagnosticQuery = "/workspace/definition-authoring/preparation-diagnostic/query"

// Cause 仅供 detached owner-local typed 校验。常规 JSON 回执只公开 metadata；
// 原始 bytes 必须通过独立 scoped query 获取。
type DefinitionAuthoringPreparationDiagnosticMetadata struct {
	DiagnosticID              string
	WorkspaceID               common.WorkspaceID
	InputHash                 string
	TemplateSnapshotHash      string
	ObservedWorkspaceRevision uint64
	Stage                     workspace.DefinitionAuthoringPreparationStage
	TargetRequestID           string
	DerivationPermitID        string
	ReviewKind                string
	ProposalHash              string
	PhaseCode                 string
	ResourceRef               string
	ProviderBindingHash       string
	Cause                     []byte `json:"-"`
	CauseSHA256               string
	CauseBytes                uint64
	CauseSaved                bool
	NotSavedReason            string
}

func (d DefinitionAuthoringPreparationDiagnosticMetadata) Model() workspace.DefinitionAuthoringPreparationDiagnostic {
	return workspace.DefinitionAuthoringPreparationDiagnostic{
		DiagnosticID: d.DiagnosticID, WorkspaceID: d.WorkspaceID, InputHash: d.InputHash, TemplateSnapshotHash: d.TemplateSnapshotHash,
		ObservedWorkspaceRevision: d.ObservedWorkspaceRevision, Stage: d.Stage, TargetRequestID: d.TargetRequestID,
		DerivationPermitID: d.DerivationPermitID, ReviewKind: d.ReviewKind, ProposalHash: d.ProposalHash,
		PhaseCode: d.PhaseCode, ResourceRef: d.ResourceRef, ProviderBindingHash: d.ProviderBindingHash,
		Cause: d.Cause, CauseSHA256: d.CauseSHA256, CauseBytes: d.CauseBytes, CauseSaved: d.CauseSaved, NotSavedReason: d.NotSavedReason,
	}.Clone()
}
func (d DefinitionAuthoringPreparationDiagnosticMetadata) Clone() DefinitionAuthoringPreparationDiagnosticMetadata {
	if d.Cause != nil {
		d.Cause = append([]byte{}, d.Cause...)
	}
	return d
}
func (d DefinitionAuthoringPreparationDiagnosticMetadata) Validate() error {
	return d.Model().Validate()
}

type RecordPreparationDiagnosticCommand struct {
	Diagnostic workspace.DefinitionAuthoringPreparationDiagnostic
}

func (c RecordPreparationDiagnosticCommand) Validate() error { return c.Diagnostic.Validate() }
func (c RecordPreparationDiagnosticCommand) DeepClone() RecordPreparationDiagnosticCommand {
	c.Diagnostic = c.Diagnostic.Clone()
	return c
}

type RecordPreparationDiagnosticResult struct {
	WorkspaceID       common.WorkspaceID
	WorkspaceRevision uint64
	Diagnostic        DefinitionAuthoringPreparationDiagnosticMetadata
	Duplicate         bool
}

func (r RecordPreparationDiagnosticResult) DeepClone() RecordPreparationDiagnosticResult {
	r.Diagnostic = r.Diagnostic.Clone()
	return r
}
func (r RecordPreparationDiagnosticResult) ValidateFor(c RecordPreparationDiagnosticCommand) error {
	if c.Validate() != nil || r.Diagnostic.Validate() != nil || r.WorkspaceID != c.Diagnostic.WorkspaceID || r.Diagnostic.WorkspaceID != r.WorkspaceID || r.WorkspaceRevision == 0 || r.Diagnostic.ObservedWorkspaceRevision > r.WorkspaceRevision {
		return common.ErrConflict
	}
	actual, expected := r.Diagnostic.Model(), c.Diagnostic.Clone()
	if r.Duplicate {
		// 重放可观察较新 Workspace revision，但不能重写首份 observation。
		expected.ObservedWorkspaceRevision = actual.ObservedWorkspaceRevision
	} else if c.Diagnostic.ObservedWorkspaceRevision == ^uint64(0) || r.WorkspaceRevision != c.Diagnostic.ObservedWorkspaceRevision+1 {
		return common.ErrConflict
	}
	if !reflect.DeepEqual(actual, expected) {
		return common.ErrConflict
	}
	return nil
}

type QueryPreparationDiagnosticCommand struct {
	WorkspaceID  common.WorkspaceID
	DiagnosticID string
}

func (c QueryPreparationDiagnosticCommand) Validate() error {
	if c.WorkspaceID == "" || c.DiagnosticID == "" || len(c.DiagnosticID) > 256 {
		return common.ErrInvalidArgument
	}
	return nil
}

type QueryPreparationDiagnosticResult struct {
	WorkspaceID common.WorkspaceID
	Diagnostic  workspace.DefinitionAuthoringPreparationDiagnostic
}

func (r QueryPreparationDiagnosticResult) DeepClone() QueryPreparationDiagnosticResult {
	r.Diagnostic = r.Diagnostic.Clone()
	return r
}
func (r QueryPreparationDiagnosticResult) ValidateFor(c QueryPreparationDiagnosticCommand) error {
	if c.Validate() != nil || r.WorkspaceID != c.WorkspaceID || r.Diagnostic.WorkspaceID != c.WorkspaceID || r.Diagnostic.DiagnosticID != c.DiagnosticID || r.Diagnostic.Validate() != nil {
		return common.ErrConflict
	}
	return nil
}
