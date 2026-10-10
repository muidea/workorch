package events

import (
	"fmt"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	definitioncontract "github.com/muidea/workorch/internal/pkg/workorchdefinitioncontract"
)

const definitionClarificationSignatureVersion = "workspace-definition-clarification-v1"

func (c DefinitionAdmissionIntentCommand) Normalized() DefinitionAdmissionIntentCommand {
	c.Candidate = c.Candidate.Normalized()
	if c.ClarificationTicket != nil {
		fact := c.ClarificationTicket.Normalized()
		c.ClarificationTicket = &fact
	}
	if c.ClarificationResolution != nil {
		fact := c.ClarificationResolution.Normalized()
		c.ClarificationResolution = &fact
	}
	return c
}

func (c DefinitionAdmissionIntentCommand) Validate() error {
	c = c.Normalized()
	if err := c.Candidate.Validate(); err != nil {
		return fmt.Errorf("definition admission intent candidate: %w", err)
	}
	if c.CurrentTemplate && (c.ExpectedWorkspaceRevision == 0 || c.ExpectedConfigurationHash == "") {
		return fmt.Errorf("current-template admission requires Workspace revision and configuration fences")
	}
	if c.ClarificationTicket != nil && c.ClarificationResolution != nil {
		return fmt.Errorf("definition admission intent accepts at most one Decision-owner fact")
	}
	if c.ClarificationTicket != nil {
		if err := c.ClarificationTicket.Validate(); err != nil {
			return err
		}
	}
	if c.ClarificationResolution != nil {
		if err := c.ClarificationResolution.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (b DefinitionAdmissionClarificationBinding) Normalized() DefinitionAdmissionClarificationBinding {
	b.Candidate = b.Candidate.Normalized()
	b.AdmissionResult = b.AdmissionResult.Normalized()
	return b
}

func (b DefinitionAdmissionClarificationBinding) Clone() DefinitionAdmissionClarificationBinding {
	b = b.Normalized()
	b.Candidate = b.Candidate.Clone()
	b.AdmissionResult.OffendingRefs = append([]string(nil), b.AdmissionResult.OffendingRefs...)
	return b
}

func (b DefinitionAdmissionClarificationBinding) Validate() error {
	b = b.Normalized()
	if err := b.AdmissionResult.ValidateFor(b.Candidate); err != nil {
		return fmt.Errorf("definition admission clarification binding: %w", err)
	}
	if b.AdmissionResult.Disposition != workspacemodel.DefinitionAdmissionClarificationRequired {
		return fmt.Errorf("definition admission clarification binding is not blocking")
	}
	return nil
}

// DecisionRequestIdentity deterministically projects the exact Workspace-owned
// clarification binding into an owner-neutral immutable request identity. The
// Decision owner wraps this value in its own concrete EventHub command.
func (b DefinitionAdmissionClarificationBinding) DecisionRequestIdentity() (definitioncontract.WorkspaceDefinitionClarificationRequestIdentity, error) {
	b = b.Normalized()
	if err := b.Validate(); err != nil {
		return definitioncontract.WorkspaceDefinitionClarificationRequestIdentity{}, err
	}
	action := definitioncontract.DefinitionClarificationActionRewriteAllOf
	switch b.AdmissionResult.ViolationCode {
	case workspacemodel.DefinitionAdmissionProfileContractInvalid,
		workspacemodel.DefinitionAdmissionProfilePrerequisiteUnresolved:
		action = definitioncontract.DefinitionClarificationActionFixTemplateProfile
	}
	command := definitioncontract.WorkspaceDefinitionClarificationRequestIdentity{
		SchemaVersion:             definitioncontract.DefinitionClarificationSchemaVersionCurrent,
		WorkspaceID:               b.Candidate.WorkspaceID,
		CandidateID:               b.Candidate.CandidateID,
		CandidateRevision:         b.Candidate.CandidateRevision,
		CandidateHash:             b.Candidate.CandidateHash,
		ViolationCode:             b.AdmissionResult.ViolationCode,
		OffendingRefs:             append([]string(nil), b.AdmissionResult.OffendingRefs...),
		CanonicalSignatureVersion: definitionClarificationSignatureVersion,
		CanonicalSignature:        b.AdmissionResult.CanonicalViolation,
		RequiredActionKind:        action,
	}
	command.CanonicalSignatureHash = definitioncontract.CanonicalDefinitionClarificationSignatureHash("workspace-signature", command.CanonicalSignatureVersion, command.CanonicalSignature)
	command.TicketID = definitioncontract.StableWorkspaceDefinitionClarificationTicketID(command)
	if err := command.Validate(); err != nil {
		return definitioncontract.WorkspaceDefinitionClarificationRequestIdentity{}, err
	}
	return command, nil
}

func (f WorkspaceDefinitionClarificationTicketFact) Normalized() WorkspaceDefinitionClarificationTicketFact {
	f.RequestIdentity = f.RequestIdentity.Normalized()
	f.TicketBinding = f.TicketBinding.Normalized()
	return f
}

func (f WorkspaceDefinitionClarificationTicketFact) Validate() error {
	f = f.Normalized()
	if err := f.TicketBinding.ValidateFor(f.RequestIdentity); err != nil {
		return fmt.Errorf("workspace definition clarification ticket fact: %w", err)
	}
	return nil
}

func (f WorkspaceDefinitionClarificationTicketFact) ValidateFor(binding DefinitionAdmissionClarificationBinding) error {
	f = f.Normalized()
	binding = binding.Normalized()
	if err := f.Validate(); err != nil {
		return err
	}
	if err := binding.Validate(); err != nil {
		return err
	}
	expected, err := binding.DecisionRequestIdentity()
	if err != nil {
		return err
	}
	if !f.RequestIdentity.EqualNormalized(expected) {
		return fmt.Errorf("workspace definition clarification ticket fact does not bind the outstanding candidate")
	}
	return nil
}

func (f WorkspaceDefinitionClarificationResolutionFact) Normalized() WorkspaceDefinitionClarificationResolutionFact {
	f.ResolutionIdentity = f.ResolutionIdentity.Normalized()
	f.ResolutionBinding = f.ResolutionBinding.Normalized()
	return f
}

func (f WorkspaceDefinitionClarificationResolutionFact) Validate() error {
	f = f.Normalized()
	if err := f.ResolutionBinding.ValidateFor(f.ResolutionIdentity); err != nil {
		return fmt.Errorf("workspace definition clarification resolution fact: %w", err)
	}
	return nil
}

func (f WorkspaceDefinitionClarificationResolutionFact) ValidateFor(prior DefinitionAdmissionClarificationBinding, candidate workspacemodel.DefinitionAdmissionCandidate, admissionResult workspacemodel.DefinitionAdmissionResult, startup workspacemodel.WorkspaceStartupInputContract, admission workspacemodel.WorkspaceActivationAdmission) error {
	f = f.Normalized()
	prior = prior.Normalized()
	candidate = candidate.Normalized()
	admissionResult = admissionResult.Normalized()
	startup = startup.Normalized()
	admission = admission.Normalized()
	if err := f.Validate(); err != nil {
		return err
	}
	if err := prior.Validate(); err != nil {
		return err
	}
	if err := admissionResult.ValidateFor(candidate); err != nil {
		return err
	}
	expected := definitioncontract.WorkspaceDefinitionClarificationAdmissionResolutionIdentity{
		TicketID: admission.BlockingTicketID, WorkspaceID: candidate.WorkspaceID,
		OldCandidateID: prior.Candidate.CandidateID, OldCandidateRevision: prior.Candidate.CandidateRevision, OldCandidateHash: prior.Candidate.CandidateHash,
		NewCandidateID: candidate.CandidateID, NewCandidateRevision: candidate.CandidateRevision, NewCandidateHash: candidate.CandidateHash,
		AdmissionResultHash: admissionResult.AdmissionResultHash, DefinitionRevision: startup.DefinitionRevision, StartupInputHash: startup.StartupInputHash,
	}.Normalized()
	if f.ResolutionIdentity != expected {
		return fmt.Errorf("workspace definition clarification resolution fact does not bind the pending admission")
	}
	return nil
}

// ValidateForEligible validates an exact replay after Workspace has already
// persisted the resolution identity. The original old-candidate binding was
// checked on the pending -> eligible transition; replay must match both the
// current candidate/startup scope and the persisted Decision result fence.
func (f WorkspaceDefinitionClarificationResolutionFact) ValidateForEligible(candidate workspacemodel.DefinitionAdmissionCandidate, admissionResult workspacemodel.DefinitionAdmissionResult, startup workspacemodel.WorkspaceStartupInputContract, admission workspacemodel.WorkspaceActivationAdmission) error {
	f = f.Normalized()
	candidate = candidate.Normalized()
	admissionResult = admissionResult.Normalized()
	startup = startup.Normalized()
	admission = admission.Normalized()
	if err := f.Validate(); err != nil {
		return err
	}
	command := f.ResolutionIdentity
	result := f.ResolutionBinding
	if err := admissionResult.ValidateFor(candidate); err != nil {
		return err
	}
	if command.TicketID != admission.BlockingTicketID || command.WorkspaceID != candidate.WorkspaceID || command.NewCandidateID != candidate.CandidateID || command.NewCandidateRevision != candidate.CandidateRevision || command.NewCandidateHash != candidate.CandidateHash || command.AdmissionResultHash != admissionResult.AdmissionResultHash || command.DefinitionRevision != startup.DefinitionRevision || command.StartupInputHash != startup.StartupInputHash || result.ResolutionID != admission.ClarificationResolutionID || result.ResolutionBindingHash != admission.ClarificationResolutionBindingHash {
		return fmt.Errorf("workspace definition clarification resolution replay does not bind the eligible admission")
	}
	return nil
}

func (r DefinitionAdmissionIntentResult) Normalized() DefinitionAdmissionIntentResult {
	r.Candidate = r.Candidate.Normalized()
	r.AdmissionResult = r.AdmissionResult.Normalized()
	if r.PriorClarification != nil {
		binding := r.PriorClarification.Clone()
		r.PriorClarification = &binding
	}
	if r.StartupInput != nil {
		startup := r.StartupInput.Clone()
		r.StartupInput = &startup
	}
	if r.ActivationAdmission != nil {
		admission := r.ActivationAdmission.Normalized()
		r.ActivationAdmission = &admission
	}
	return r
}

func (r DefinitionAdmissionIntentResult) Validate() error {
	r = r.Normalized()
	if err := r.AdmissionResult.ValidateFor(r.Candidate); err != nil {
		return fmt.Errorf("definition admission intent result: %w", err)
	}
	switch r.CandidateDisposition {
	case DefinitionAdmissionCandidateCreated, DefinitionAdmissionCandidateDuplicate:
	default:
		return fmt.Errorf("definition admission intent result candidate disposition is invalid")
	}
	if r.PriorClarification != nil {
		if err := r.PriorClarification.Validate(); err != nil {
			return err
		}
		if r.PriorClarification.Candidate.WorkspaceID != r.Candidate.WorkspaceID || r.PriorClarification.Candidate.CandidateID != r.Candidate.CandidateID || r.PriorClarification.Candidate.CandidateRevision >= r.Candidate.CandidateRevision {
			return fmt.Errorf("definition admission prior clarification scope is invalid")
		}
	}
	if (r.StartupInput == nil) != (r.ActivationAdmission == nil) {
		return fmt.Errorf("definition admission intent result startup and activation facts must be paired")
	}
	if r.StartupInput != nil {
		if err := r.StartupInput.ValidateCandidate(r.Candidate); err != nil {
			return err
		}
		if err := r.ActivationAdmission.ValidateCandidate(*r.StartupInput, r.Candidate); err != nil {
			return err
		}
	}
	switch r.Disposition {
	case DefinitionAdmissionIntentClarificationRequired:
		if r.AdmissionResult.Disposition != workspacemodel.DefinitionAdmissionClarificationRequired || r.StartupInput != nil {
			return fmt.Errorf("clarification-required admission outcome is invalid")
		}
	case DefinitionAdmissionIntentClarificationTicketRequired:
		if r.AdmissionResult.Disposition != workspacemodel.DefinitionAdmissionAdmittedAllOf || r.PriorClarification == nil || r.StartupInput != nil {
			return fmt.Errorf("clarification-ticket-required admission outcome is invalid")
		}
	case DefinitionAdmissionIntentClarificationResolutionRequired:
		if r.AdmissionResult.Disposition != workspacemodel.DefinitionAdmissionAdmittedAllOf || r.PriorClarification == nil || r.ActivationAdmission == nil || r.ActivationAdmission.State != workspacemodel.WorkspaceActivationPendingClarificationResolution {
			return fmt.Errorf("clarification-resolution-required admission outcome is invalid")
		}
	case DefinitionAdmissionIntentEligible:
		if r.AdmissionResult.Disposition != workspacemodel.DefinitionAdmissionAdmittedAllOf || r.ActivationAdmission == nil || r.ActivationAdmission.State != workspacemodel.WorkspaceActivationEligible {
			return fmt.Errorf("eligible admission outcome is invalid")
		}
	default:
		return fmt.Errorf("definition admission intent disposition is invalid")
	}
	return nil
}

func (r DefinitionAdmissionIntentResult) ValidateFor(candidate workspacemodel.DefinitionAdmissionCandidate) error {
	r = r.Normalized()
	candidate = candidate.Normalized()
	if err := r.Validate(); err != nil {
		return err
	}
	if r.Candidate.CandidateID != candidate.CandidateID || r.Candidate.CandidateRevision != candidate.CandidateRevision || r.Candidate.CandidateHash != candidate.CandidateHash || r.Candidate.WorkspaceID != candidate.WorkspaceID {
		return fmt.Errorf("definition admission intent result does not bind the submitted candidate")
	}
	return nil
}
