package events

import (
	"fmt"
	"reflect"
	"strings"

	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type SaveDefinitionAuthoringCommand struct {
	Input                     workspace.DefinitionAuthoringInput
	ExpectedWorkspaceRevision uint64
	DeliverableDirectory      *string
}

func (c SaveDefinitionAuthoringCommand) Validate() error {
	if err := c.Input.Validate(); err != nil {
		return err
	}
	if c.DeliverableDirectory != nil && (strings.TrimSpace(*c.DeliverableDirectory) == "" || len(*c.DeliverableDirectory) > 32*1024 || strings.ContainsRune(*c.DeliverableDirectory, 0)) {
		return fmt.Errorf("authoring directory is invalid")
	}
	if c.ExpectedWorkspaceRevision == 0 {
		return fmt.Errorf("authoring save requires workspace revision")
	}
	return nil
}

type SaveDefinitionAuthoringResult struct {
	Record workspace.DefinitionAuthoringRecord
}

type QueryDefinitionAuthoringCommand struct {
	WorkspaceID common.WorkspaceID
}

type QueryDefinitionAuthoringResult struct {
	WorkspaceID                 common.WorkspaceID
	WorkspaceRevision           uint64
	Record                      *workspace.DefinitionAuthoringRecord
	History                     []workspace.DefinitionAuthoringConfirmedRevision
	Confirmation                *workspace.DefinitionAuthoringConfirmation
	PriorClarification          *DefinitionAdmissionClarificationBinding
	AdmissionCandidate          *workspace.DefinitionAdmissionCandidate
	AdmissionResult             *workspace.DefinitionAdmissionResult
	ActivationAdmission         *workspace.WorkspaceActivationAdmission
	DeliverableDirectory        string
	CurrentTemplateSnapshotHash string
	CurrentTemplateAvailable    bool
}

func (r QueryDefinitionAuthoringResult) ValidateFor(c QueryDefinitionAuthoringCommand) error {
	if c.WorkspaceID == "" || r.WorkspaceID != c.WorkspaceID || r.WorkspaceRevision == 0 || r.CurrentTemplateAvailable != (r.CurrentTemplateSnapshotHash != "") {
		return fmt.Errorf("authoring query result scope is invalid")
	}
	if r.Record != nil {
		if err := r.Record.Validate(); err != nil {
			return err
		}
		if r.Record.Input.WorkspaceID != c.WorkspaceID {
			return fmt.Errorf("authoring query record scope mismatch")
		}
	}
	if r.Confirmation != nil {
		if r.Record == nil || r.Confirmation.Validate(r.Record, c.WorkspaceID) != nil {
			return fmt.Errorf("authoring query confirmation lacks exact record")
		}
	}
	if len(r.History) > 0 {
		if r.Record == nil || r.Record.Input.Revision <= r.History[len(r.History)-1].Record.Input.Revision {
			return fmt.Errorf("authoring query lacks successor input")
		}
		previousRevision := uint64(0)
		for index, archived := range r.History {
			if archived.Record.Validate() != nil || archived.Record.Input.Revision <= previousRevision || archived.Confirmation.Phase != workspace.DefinitionAuthoringConfirmationMaterialized || archived.Confirmation.Validate(&archived.Record, c.WorkspaceID) != nil {
				return fmt.Errorf("authoring query history is invalid")
			}
			previousRevision = archived.Record.Input.Revision
			if index == 0 && archived.Confirmation.FormalSequence > 1 || index > 0 && archived.Confirmation.FormalSequence != uint64(index+1) {
				return fmt.Errorf("authoring query history formal sequence is invalid")
			}
			if index > 0 && archived.Confirmation.ValidateSuccessor(r.History[index-1].Confirmation) != nil {
				return fmt.Errorf("authoring query history lineage is invalid")
			}
		}
		if r.Confirmation != nil && r.Confirmation.ValidateSuccessor(r.History[len(r.History)-1].Confirmation) != nil {
			return fmt.Errorf("authoring query confirmation lineage is invalid")
		}
	}
	if r.PriorClarification != nil {
		if r.PriorClarification.Validate() != nil || r.PriorClarification.Candidate.WorkspaceID != c.WorkspaceID || r.PriorClarification.Candidate.CandidateRevision > uint64(len(r.History)) {
			return fmt.Errorf("authoring query prior clarification is invalid")
		}
	}
	if r.AdmissionCandidate != nil {
		if r.Confirmation == nil || r.Confirmation.Phase != workspace.DefinitionAuthoringConfirmationMaterialized || r.AdmissionCandidate.Validate() != nil || r.AdmissionCandidate.AuthoringRef == nil || r.AdmissionCandidate.WorkspaceID != c.WorkspaceID || r.AdmissionResult == nil || r.AdmissionResult.ValidateFor(*r.AdmissionCandidate) != nil {
			return fmt.Errorf("authoring query admission lacks reviewed source")
		}
		if r.ActivationAdmission == nil && r.AdmissionResult.Disposition != workspace.DefinitionAdmissionClarificationRequired && r.PriorClarification == nil {
			return fmt.Errorf("authoring query has an admitted candidate without prior clarification")
		}
		if r.PriorClarification != nil && (r.PriorClarification.Candidate.CandidateID != r.AdmissionCandidate.CandidateID || r.PriorClarification.Candidate.CandidateRevision >= r.AdmissionCandidate.CandidateRevision) {
			return fmt.Errorf("authoring query prior clarification does not precede candidate")
		}
		if r.AdmissionCandidate.CandidateRevision != uint64(len(r.History)+1) {
			return fmt.Errorf("authoring query candidate revision differs from history")
		}
		ref, err := workspace.ConfirmedAuthoringAdmissionRef(r.Record, *r.Confirmation, c.WorkspaceID)
		if err != nil || *r.AdmissionCandidate.AuthoringRef != ref {
			return fmt.Errorf("authoring query admission source mismatch")
		}
		goal, dod, spec, err := workspace.ConfirmedAuthoringCandidateInputs(r.Record, *r.Confirmation, c.WorkspaceID)
		if err != nil || r.AdmissionCandidate.GoalInput != goal || !reflect.DeepEqual(r.AdmissionCandidate.DoDInputs, workspace.DefinitionAdmissionCandidate{DoDInputs: dod}.Normalized().DoDInputs) || r.AdmissionCandidate.Specification == nil || !reflect.DeepEqual(r.AdmissionCandidate.Specification.Normalized(), spec.Normalized()) {
			return fmt.Errorf("authoring query admission reviews mismatch")
		}
		if r.ActivationAdmission != nil && (r.AdmissionResult.Disposition != workspace.DefinitionAdmissionAdmittedAllOf || r.ActivationAdmission.CandidateHash != r.AdmissionCandidate.CandidateHash || r.ActivationAdmission.State != workspace.WorkspaceActivationEligible && r.ActivationAdmission.State != workspace.WorkspaceActivationPendingClarificationResolution || r.ActivationAdmission.State == workspace.WorkspaceActivationPendingClarificationResolution && r.PriorClarification == nil) {
			return fmt.Errorf("authoring query admission has invalid activation")
		}
		if r.ActivationAdmission != nil && r.PriorClarification != nil {
			identity, err := r.PriorClarification.DecisionRequestIdentity()
			if err != nil || r.ActivationAdmission.BlockingTicketID != identity.TicketID {
				return fmt.Errorf("authoring query activation does not bind prior ticket")
			}
		}
	} else if r.AdmissionResult != nil || r.ActivationAdmission != nil {
		return fmt.Errorf("authoring query has admission without candidate")
	}
	return nil
}
