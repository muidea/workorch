package events

import (
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const TopicWorkspaceAuthoringSuccessAuditQuery = "/workspace/definition-authoring/success-byte-audit/query"

type QueryAuthoringSuccessAuditCommand struct {
	WorkspaceID common.WorkspaceID
	PermitID    string
}

func (c QueryAuthoringSuccessAuditCommand) Validate() error {
	if c.WorkspaceID == "" || c.PermitID == "" || len(c.PermitID) > 256 {
		return common.ErrInvalidArgument
	}
	return nil
}

type QueryAuthoringSuccessAuditResult struct {
	WorkspaceID common.WorkspaceID
	PermitID    string
	// Derivation content hash, or the proposal hash targeted by proposal/review.
	CanonicalHash string
	Audit         workspace.DefinitionAuthoringSuccessByteAudit
}

func (r QueryAuthoringSuccessAuditResult) ValidateFor(c QueryAuthoringSuccessAuditCommand) error {
	if c.Validate() != nil || r.WorkspaceID != c.WorkspaceID || r.PermitID != c.PermitID || r.CanonicalHash == "" || r.Audit.Validate() != nil {
		return common.ErrConflict
	}
	return nil
}
func (r QueryAuthoringSuccessAuditResult) Clone() QueryAuthoringSuccessAuditResult {
	r.Audit = r.Audit.Clone()
	return r
}
