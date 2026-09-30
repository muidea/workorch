package workspace

import (
	"fmt"
	"reflect"

	"github.com/muidea/workorch/internal/pkg/common"
)

// DefinitionAuthoringConfirmedRevision retains a prior materialized journal
// together with the record that supplied its exact derivation and six reviews.
// Neither the history nor an unconfirmed successor can authorize startup.
type DefinitionAuthoringConfirmedRevision struct {
	Record       DefinitionAuthoringRecord
	Confirmation DefinitionAuthoringConfirmation
}

func (r DefinitionAuthoringConfirmedRevision) Clone() DefinitionAuthoringConfirmedRevision {
	r.Record = r.Record.Clone()
	r.Confirmation = r.Confirmation.Clone()
	return r
}

func (w *Workspace) ValidateDefinitionAuthoringHistory() error {
	if w == nil {
		return common.ErrInvalidArgument
	}
	if len(w.DefinitionAuthoringHistory) == 0 {
		return nil
	}
	if w.DefinitionAuthoring == nil || len(w.DefinitionAuthoringHistory) > len(w.DefinitionAdmissionCandidates) || len(w.DefinitionAuthoringHistory) > DefinitionDerivationRequestLimit {
		return fmt.Errorf("authoring history lacks current draft or confirmed candidate")
	}
	for index, archived := range w.DefinitionAuthoringHistory {
		if archived.Record.Validate() != nil || archived.Confirmation.Phase != DefinitionAuthoringConfirmationMaterialized || archived.Confirmation.Validate(&archived.Record, w.ID) != nil {
			return fmt.Errorf("authoring history %d has invalid sealed revision", index)
		}
		if index > 0 && archived.Record.Input.Revision <= w.DefinitionAuthoringHistory[index-1].Record.Input.Revision {
			return fmt.Errorf("authoring history input revisions are not increasing")
		}
		if index > 0 && archived.Confirmation.ValidateSuccessor(w.DefinitionAuthoringHistory[index-1].Confirmation) != nil {
			return fmt.Errorf("authoring history formal revisions are not contiguous")
		}
		if index == 0 && archived.Confirmation.FormalSequence > 1 || index > 0 && archived.Confirmation.FormalSequence != uint64(index+1) {
			return fmt.Errorf("authoring history has invalid formal sequence")
		}
		candidate := w.DefinitionAdmissionCandidates[index]
		ref, err := ConfirmedAuthoringAdmissionRef(&archived.Record, archived.Confirmation, w.ID)
		if err != nil || candidate.Validate() != nil || candidate.AuthoringRef == nil || *candidate.AuthoringRef != ref || candidate.CandidateRevision != uint64(index+1) || (index > 0 && candidate.CandidateID != w.DefinitionAdmissionCandidates[0].CandidateID) {
			return fmt.Errorf("authoring history %d differs from immutable admission candidate", index)
		}
		goal, dod, specification, err := ConfirmedAuthoringCandidateInputs(&archived.Record, archived.Confirmation, w.ID)
		if err != nil || candidate.GoalInput != goal || !reflect.DeepEqual(candidate.DoDInputs, DefinitionAdmissionCandidate{DoDInputs: dod}.Normalized().DoDInputs) || candidate.Specification == nil || !reflect.DeepEqual(candidate.Specification.Normalized(), specification.Normalized()) {
			return fmt.Errorf("authoring history %d differs from reviewed definitions", index)
		}
	}
	last := w.DefinitionAuthoringHistory[len(w.DefinitionAuthoringHistory)-1]
	if w.DefinitionAuthoring.Input.Revision <= last.Record.Input.Revision {
		return fmt.Errorf("authoring successor does not advance input revision")
	}
	return nil
}
