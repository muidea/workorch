package events

import (
	"fmt"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const TopicWorkspaceAuthoringAdmit = "/workspace/definition-authoring/admit"

// AdmitDefinitionAuthoringCommand carries a reviewed candidate plus current
// Workspace/Template fences. The owner reconstructs its exact reviews and
// independently checks configuration bindings before applying this intent.
type AdmitDefinitionAuthoringCommand struct {
	Candidate                 workspace.DefinitionAdmissionCandidate
	ExpectedWorkspaceRevision uint64
	ExpectedConfigurationHash string
	ClarificationTicket       *WorkspaceDefinitionClarificationTicketFact
	ClarificationResolution   *WorkspaceDefinitionClarificationResolutionFact
}

func (c AdmitDefinitionAuthoringCommand) Validate() error {
	if c.ExpectedWorkspaceRevision == 0 || c.ExpectedConfigurationHash == "" || c.Candidate.AuthoringRef == nil || c.Candidate.CompilationRef != nil {
		return common.ErrInvalidArgument
	}
	if err := c.Candidate.Validate(); err != nil {
		return fmt.Errorf("authoring admission candidate: %w", err)
	}
	if c.ClarificationTicket != nil && c.ClarificationResolution != nil {
		return common.ErrInvalidArgument
	}
	if c.ClarificationTicket != nil && c.ClarificationTicket.Validate() != nil || c.ClarificationResolution != nil && c.ClarificationResolution.Validate() != nil {
		return common.ErrInvalidArgument
	}
	return nil
}

type AdmitDefinitionAuthoringResult struct {
	Candidate           workspace.DefinitionAdmissionCandidate
	AdmissionResult     workspace.DefinitionAdmissionResult
	PriorClarification  *DefinitionAdmissionClarificationBinding
	BlockingTicketID    common.TicketID
	StartupInput        *workspace.WorkspaceStartupInputContract
	ActivationAdmission *workspace.WorkspaceActivationAdmission
	WorkspaceRevision   uint64
}

func (r AdmitDefinitionAuthoringResult) ValidateFor(c AdmitDefinitionAuthoringCommand) error {
	if c.Validate() != nil || r.Candidate.CandidateHash != c.Candidate.CandidateHash || r.WorkspaceRevision < c.ExpectedWorkspaceRevision || r.AdmissionResult.ValidateFor(r.Candidate) != nil {
		return fmt.Errorf("authoring admission result does not match candidate")
	}
	if r.PriorClarification != nil {
		if r.PriorClarification.Validate() != nil || r.PriorClarification.Candidate.WorkspaceID != r.Candidate.WorkspaceID || r.PriorClarification.Candidate.CandidateID != r.Candidate.CandidateID || r.PriorClarification.Candidate.CandidateRevision >= r.Candidate.CandidateRevision {
			return fmt.Errorf("authoring admission prior clarification scope mismatch")
		}
	}
	if c.ClarificationTicket != nil && r.PriorClarification == nil || c.ClarificationResolution != nil && r.PriorClarification == nil {
		return fmt.Errorf("authoring admission Decision fact has no prior clarification")
	}
	if r.AdmissionResult.Disposition == workspace.DefinitionAdmissionAdmittedAllOf {
		if r.BlockingTicketID != "" {
			return fmt.Errorf("admitted authoring result cannot carry a new blocking ticket")
		}
		if r.StartupInput == nil {
			if r.PriorClarification == nil || r.ActivationAdmission != nil || c.ClarificationTicket != nil || c.ClarificationResolution != nil {
				return fmt.Errorf("authoring successor requires prior Decision ticket")
			}
			return nil
		}
		if r.ActivationAdmission == nil || r.ActivationAdmission.ValidateCandidate(*r.StartupInput, r.Candidate) != nil {
			return fmt.Errorf("authoring admission result lacks sealed startup")
		}
		if r.PriorClarification == nil && r.ActivationAdmission.State != workspace.WorkspaceActivationEligible || r.PriorClarification != nil && (r.ActivationAdmission.State != workspace.WorkspaceActivationPendingClarificationResolution && r.ActivationAdmission.State != workspace.WorkspaceActivationEligible || r.ActivationAdmission.BlockingTicketID == "") {
			return fmt.Errorf("authoring admission has invalid clarification state")
		}
		if r.PriorClarification != nil {
			identity, err := r.PriorClarification.DecisionRequestIdentity()
			if err != nil || r.ActivationAdmission.BlockingTicketID != identity.TicketID {
				return fmt.Errorf("authoring admission prior ticket identity mismatch")
			}
		}
		if c.ClarificationTicket != nil && (r.PriorClarification == nil || c.ClarificationTicket.ValidateFor(*r.PriorClarification) != nil || c.ClarificationTicket.TicketBinding.TicketID != r.ActivationAdmission.BlockingTicketID) {
			return fmt.Errorf("authoring admission ticket mismatch")
		}
		if r.ActivationAdmission.State == workspace.WorkspaceActivationEligible && r.ActivationAdmission.BlockingTicketID != "" && c.ClarificationResolution != nil && c.ClarificationResolution.ValidateForEligible(r.Candidate, r.AdmissionResult, *r.StartupInput, *r.ActivationAdmission) != nil {
			return fmt.Errorf("authoring admission resolution replay mismatch")
		}
		if r.ActivationAdmission.State == workspace.WorkspaceActivationPendingClarificationResolution && c.ClarificationResolution != nil {
			return fmt.Errorf("authoring admission cannot acknowledge resolution while pending")
		}
	} else {
		if r.StartupInput != nil || r.ActivationAdmission != nil {
			return fmt.Errorf("blocked authoring admission cannot return startup")
		}
		if r.BlockingTicketID != "" {
			binding := DefinitionAdmissionClarificationBinding{Candidate: r.Candidate, AdmissionResult: r.AdmissionResult}
			identity, err := binding.DecisionRequestIdentity()
			if err != nil || r.BlockingTicketID != identity.TicketID {
				return fmt.Errorf("blocked authoring admission ticket identity mismatch")
			}
		}
	}
	return nil
}
