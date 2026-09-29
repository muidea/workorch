package events

import (
	"fmt"
	"strings"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

// Begin seals pre-admission input without creating an admission candidate,
// resolving a Decision ticket, starting a Run or granting a physical call.
type BeginDefinitionSpecificationCommand struct {
	Candidate                 workspacemodel.DefinitionAdmissionCandidate
	Resources                 []workspacemodel.GoalSpecificationResource
	Dimensions                []workspacemodel.GoalSpecificationDimension
	ExpectedWorkspaceRevision uint64
	ExpectedConfigurationHash string
}

func (c BeginDefinitionSpecificationCommand) Normalized() BeginDefinitionSpecificationCommand {
	c.Candidate = c.Candidate.Normalized()
	seed := (workspacemodel.GoalSpecification{Resources: c.Resources, Dimensions: c.Dimensions}).Normalized()
	c.Resources, c.Dimensions = seed.Resources, seed.Dimensions
	c.ExpectedConfigurationHash = strings.TrimSpace(c.ExpectedConfigurationHash)
	return c
}
func (c BeginDefinitionSpecificationCommand) Validate() error {
	c = c.Normalized()
	if c.Candidate.Specification != nil || c.Candidate.CompilationRef != nil || c.ExpectedWorkspaceRevision == 0 || c.ExpectedConfigurationHash == "" || c.Candidate.Validate() != nil || workspacemodel.ValidateDefinitionSpecificationInputs(c.Resources, c.Dimensions) != nil {
		return fmt.Errorf("invalid pre-seal specification compilation input")
	}
	return nil
}

type BeginDefinitionSpecificationResult struct {
	WorkspaceID          common.WorkspaceID
	CandidateHash        string
	CompilationInputHash string
	GoalRevision         string
	DoDRevision          string
	Snapshot             workspacemodel.DefinitionAdmissionCandidate
	Resources            []workspacemodel.GoalSpecificationResource
	Dimensions           []workspacemodel.GoalSpecificationDimension
	WorkspaceRevision    uint64
}

func (r BeginDefinitionSpecificationResult) ValidateFor(command BeginDefinitionSpecificationCommand) error {
	command = command.Normalized()
	if err := command.Validate(); err != nil {
		return err
	}
	if r.WorkspaceID != command.Candidate.WorkspaceID || r.CandidateHash != command.Candidate.CandidateHash ||
		r.Snapshot.Validate() != nil || r.Snapshot.CandidateHash != r.CandidateHash ||
		r.GoalRevision == "" || r.DoDRevision == "" || r.WorkspaceRevision == 0 ||
		r.CompilationInputHash != workspacemodel.DefinitionSpecificationCompilationInputHash(command.Candidate.CandidateHash, command.Resources, command.Dimensions) ||
		r.CompilationInputHash != workspacemodel.DefinitionSpecificationCompilationInputHash(r.Snapshot.CandidateHash, r.Resources, r.Dimensions) {
		return fmt.Errorf("specification begin result does not bind requested input")
	}
	return nil
}

type ReserveDefinitionSpecificationRequestCommand struct {
	Identity workspacemodel.DefinitionSpecificationRequestIdentity
}

func (c ReserveDefinitionSpecificationRequestCommand) Validate() error { return c.Identity.Validate() }

type DefinitionSpecificationRequestDisposition string

const (
	DefinitionSpecificationPermitted        DefinitionSpecificationRequestDisposition = "permitted"
	DefinitionSpecificationAlreadyReserved  DefinitionSpecificationRequestDisposition = "already_reserved"
	DefinitionSpecificationAlreadyCompleted DefinitionSpecificationRequestDisposition = "already_completed"
)

// PermitID is nonempty only on a fresh, durably persisted reservation.
// Duplicate reserved requests never obtain a second physical-call permit.
type DefinitionSpecificationRequestResult struct {
	Identity    workspacemodel.DefinitionSpecificationRequestIdentity
	Disposition DefinitionSpecificationRequestDisposition
	PermitID    string
	Completed   *workspacemodel.DefinitionSpecificationRequestCompletion
	Consumed    uint8
	Remaining   uint8
}

func (r DefinitionSpecificationRequestResult) ValidateFor(identity workspacemodel.DefinitionSpecificationRequestIdentity) error {
	if identity.Validate() != nil || r.Identity != identity.Normalized() || r.Consumed == 0 ||
		r.Consumed > workspacemodel.DefinitionSpecificationRequestLimit ||
		r.Remaining != workspacemodel.DefinitionSpecificationRequestLimit-r.Consumed {
		return fmt.Errorf("specification request result identity or budget mismatch")
	}
	switch r.Disposition {
	case DefinitionSpecificationPermitted:
		if r.PermitID != identity.PermitID() || r.Completed != nil {
			return fmt.Errorf("specification permit result is invalid")
		}
	case DefinitionSpecificationAlreadyReserved:
		if r.PermitID != "" || r.Completed != nil {
			return fmt.Errorf("reserved replay cannot grant another permit")
		}
	case DefinitionSpecificationAlreadyCompleted:
		if r.PermitID != "" || r.Completed == nil {
			return fmt.Errorf("completed replay requires a saved result without a permit")
		}
	default:
		return fmt.Errorf("unknown specification request disposition")
	}
	return nil
}

func (r DefinitionSpecificationRequestResult) Clone() DefinitionSpecificationRequestResult {
	if r.Completed != nil {
		c := r.Completed.Clone()
		r.Completed = &c
	}
	return r
}

type CompleteDefinitionSpecificationRequestCommand struct {
	Identity   workspacemodel.DefinitionSpecificationRequestIdentity
	PermitID   string
	Completion workspacemodel.DefinitionSpecificationRequestCompletion
}

func (c CompleteDefinitionSpecificationRequestCommand) Validate() error {
	if c.Identity.Validate() != nil || strings.TrimSpace(c.PermitID) != c.Identity.PermitID() {
		return fmt.Errorf("completion requires exact reserved permit")
	}
	return nil
}

type QueryDefinitionSpecificationRequestCommand struct {
	Identity workspacemodel.DefinitionSpecificationRequestIdentity
}

func (c QueryDefinitionSpecificationRequestCommand) Validate() error { return c.Identity.Validate() }
