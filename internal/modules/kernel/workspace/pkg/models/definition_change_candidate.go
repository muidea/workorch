package workspace

import (
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
)

// DefinitionChangeCandidate 复用原创作记录保存待审输入，不是新定义或批准副本。
// 当前准备合同只封存一份候选；更换提案必须由后续明确的修订合同处理。
type DefinitionChangeCandidate struct {
	RequestID                 string
	ExpectedWorkspaceRevision uint64
	RecordedWorkspaceRevision uint64
	TicketID                  common.TicketID
	ProposalRevision          uint64
	ProposalHash              string
	BaseDefinitionRevision    string
	BaseStartupInputHash      string
	BaseInputHash             string
	Record                    DefinitionAuthoringRecord
}

func (c DefinitionChangeCandidate) Clone() DefinitionChangeCandidate {
	c.Record = c.Record.Clone()
	return c
}

func (c DefinitionChangeCandidate) Validate(w *Workspace) error {
	if w == nil || w.DefinitionAuthoring == nil || strings.TrimSpace(c.RequestID) == "" || len(c.RequestID) > 256 || !utf8.ValidString(c.RequestID) || c.TicketID == "" || c.ProposalRevision == 0 || c.ProposalHash == "" ||
		c.ExpectedWorkspaceRevision == 0 || c.ExpectedWorkspaceRevision == ^uint64(0) || c.RecordedWorkspaceRevision != c.ExpectedWorkspaceRevision+1 || c.RecordedWorkspaceRevision > w.Revision ||
		c.BaseInputHash != w.DefinitionAuthoring.InputHash || c.Record.Input.WorkspaceID != w.ID || w.DefinitionAuthoring.Input.Revision == ^uint64(0) || c.Record.Input.Revision != w.DefinitionAuthoring.Input.Revision+1 ||
		c.Record.TemplateSnapshotHash != w.DefinitionAuthoring.TemplateSnapshotHash || c.Record.Validate() != nil ||
		c.Record.Input.TemplateID != w.DefinitionAuthoring.Input.TemplateID || !reflect.DeepEqual(c.Record.Input.ResourceRefs, w.DefinitionAuthoring.Input.ResourceRefs) {
		return common.ErrConflict
	}
	for n, original := range w.DefinitionAuthoring.DerivationRequests {
		if n >= len(c.Record.DerivationRequests) || !reflect.DeepEqual(c.Record.DerivationRequests[n], original) {
			return common.ErrConflict
		}
	}
	for n, original := range w.DefinitionAuthoring.SpecificationRequests {
		if n >= len(c.Record.SpecificationRequests) || !reflect.DeepEqual(c.Record.SpecificationRequests[n], original) {
			return common.ErrConflict
		}
	}
	if !reflect.DeepEqual(c.Record.SaveReceipts, w.DefinitionAuthoring.SaveReceipts) {
		return common.ErrConflict
	}
	for n, original := range w.DefinitionAuthoring.PreparationDiagnostics {
		if n >= len(c.Record.PreparationDiagnostics) || !reflect.DeepEqual(c.Record.PreparationDiagnostics[n], original) {
			return common.ErrConflict
		}
	}
	if w.StartupInput == nil {
		if c.BaseDefinitionRevision != "" || c.BaseStartupInputHash != "" || w.ActivationAdmission != nil {
			return common.ErrConflict
		}
	} else if c.BaseDefinitionRevision != w.StartupInput.DefinitionRevision || c.BaseStartupInputHash != w.StartupInput.StartupInputHash {
		return common.ErrConflict
	}
	return nil
}
