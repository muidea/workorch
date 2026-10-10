package events

import (
	"fmt"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const TopicWorkspaceAuthoringConfirm = "/workspace/definition-authoring/confirm"

// ConfirmDefinitionAuthoringCommand names one exact, reviewed, persisted result.
// Neither caller-supplied specification content nor a caller-selected state is
// accepted. ExpectedWorkspaceRevision applies only to the first preparation;
// exact retries continue the existing durable journal after a lost response.
type ConfirmDefinitionAuthoringCommand struct {
	WorkspaceID               common.WorkspaceID
	InputHash                 string
	DerivationPermitID        string
	ProposalHash              string
	SpecificationHash         string
	ExpectedWorkspaceRevision uint64
}

func (c ConfirmDefinitionAuthoringCommand) Validate() error {
	if c.WorkspaceID == "" || c.InputHash == "" || len(c.InputHash) > 256 || c.DerivationPermitID == "" || len(c.DerivationPermitID) > 256 || c.ProposalHash == "" || len(c.ProposalHash) > 256 || c.SpecificationHash == "" || len(c.SpecificationHash) > 256 || c.ExpectedWorkspaceRevision == 0 {
		return common.ErrInvalidArgument
	}
	return nil
}

// Materialization alone is not an eligible Workspace. Admission still requires
// the exact profile-backed candidate and the owner compiler.
type ConfirmDefinitionAuthoringResult struct {
	WorkspaceID       common.WorkspaceID
	InputHash         string
	ProposalHash      string
	SpecificationHash string
	Goal              workspace.Goal
	DoD               workspace.DoD
	WorkspaceRevision uint64
}

func (r ConfirmDefinitionAuthoringResult) ValidateFor(c ConfirmDefinitionAuthoringCommand) error {
	if c.Validate() != nil || r.WorkspaceID != c.WorkspaceID || r.InputHash != c.InputHash || r.ProposalHash != c.ProposalHash || r.SpecificationHash != c.SpecificationHash || r.Goal.WorkspaceID != c.WorkspaceID || r.DoD.WorkspaceID != c.WorkspaceID || r.Goal.ID == "" || r.DoD.ID == "" || r.WorkspaceRevision == 0 {
		return fmt.Errorf("authoring confirmation result identity mismatch")
	}
	return nil
}
