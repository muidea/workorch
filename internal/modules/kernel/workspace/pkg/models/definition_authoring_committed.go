package workspace

import (
	"fmt"
	"maps"
	"math"
	"reflect"
	"slices"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

// ReadCommittedConfirmation authenticates an owner-committed confirmation's
// source links and materialized after-images. It never compiles the mapping,
// derives DoD, or re-adjudicates the six semantic reviews. New commands and
// independent external admission must continue to use Validate.
func (c DefinitionAuthoringConfirmation) ReadCommitted(record *DefinitionAuthoringRecord, workspaceID common.WorkspaceID) error {
	return c.readCommitted(record, workspaceID, false)
}

func (c DefinitionAuthoringConfirmation) readCommitted(record *DefinitionAuthoringRecord, workspaceID common.WorkspaceID, recordAlreadyVerified bool) error {
	if record == nil || workspaceID == "" || record.Input.WorkspaceID != workspaceID || c.InputHash != record.InputHash || c.TemplateSnapshotHash != record.TemplateSnapshotHash || c.ConfigurationHash == "" || (c.Phase != DefinitionAuthoringConfirmationPrepared && c.Phase != DefinitionAuthoringConfirmationMaterialized) {
		return common.ErrConflict
	}
	if !recordAlreadyVerified && (record.InputHash != record.Input.InputHash() || record.TemplateSnapshotHash != WorkspaceTemplateSnapshotHash(&record.TemplateSnapshot)) {
		return common.ErrConflict
	}
	var derivation *DefinitionDerivationRequest
	for i := range record.DerivationRequests {
		d := &record.DerivationRequests[i]
		if d.PermitID == c.DerivationPermitID {
			if derivation != nil {
				return common.ErrConflict
			}
			derivation = d
		}
	}
	if derivation == nil || derivation.Proposal == nil || derivation.FailureCode != "" || derivation.ResourceSnapshotHash != c.ResourceSnapshotHash {
		return common.ErrConflict
	}
	if !sameDefinitionAuthoringInput(&derivation.Input, &record.Input) && derivation.Input.InputHash() != c.InputHash {
		return common.ErrConflict
	}
	if !sameWorkspaceTemplate(&derivation.Template, &record.TemplateSnapshot) && WorkspaceTemplateSnapshotHash(&derivation.Template) != c.TemplateSnapshotHash {
		return common.ErrConflict
	}
	var proposal *GoalSpecification
	for i := range record.SpecificationRequests {
		r := &record.SpecificationRequests[i]
		if r.DerivationPermitID == c.DerivationPermitID && r.Proposal != nil && r.Proposal.ContentHash() == c.ProposalHash {
			if proposal != nil || r.FailureCode != "" {
				return common.ErrConflict
			}
			p := r.Proposal.Clone()
			proposal = &p
		}
	}
	if proposal == nil {
		return common.ErrConflict
	}
	for _, kind := range goalReviewKinds {
		review, failed, unresolved, _, reviewErr := record.SettledCriticReview(c.DerivationPermitID, c.ProposalHash, kind)
		if reviewErr != nil || unresolved || failed != nil || review == nil {
			return common.ErrConflict
		}
		proposal.Reviews = append(proposal.Reviews, *review)
	}
	if proposal.AdmissionHash() != c.SpecificationHash {
		return common.ErrConflict
	}
	sequence := c.FormalSequence
	if sequence == 0 && c.PreviousInputHash == "" {
		sequence = 1
	}
	if sequence == 0 || sequence == 1 && c.PreviousInputHash != "" || sequence > 1 && (c.PreviousInputHash == "" || c.PreviousInputHash == c.InputHash) {
		return common.ErrConflict
	}
	id := StableDefinitionAuthoringConfirmationID(workspaceID, c.InputHash, c.DerivationPermitID, c.SpecificationHash)
	if c.Goal.ID != common.GoalID("authoring-goal-"+id) || c.DoD.ID != common.DoDID("authoring-dod-"+id) || c.Goal.WorkspaceID != workspaceID || c.DoD.WorkspaceID != workspaceID || c.Goal.Text != strings.TrimSpace(record.Input.GoalText) || c.Goal.LineageState != "active" || c.DoD.LineageState != "active" || c.DoD.State != common.DoDStateReady || len(c.DoD.Items) == 0 || c.Goal.DefinitionRevision != authoringFormalRevisionAt("goal", sequence, c.Goal.Text) || c.DoD.DefinitionRevision != authoringFormalRevisionAt("dod", sequence, strings.Join(c.DoD.Items, "\n")) {
		return common.ErrConflict
	}
	return nil
}

// CommittedAdmissionRef reads the source identity already committed by this owner.
func (c DefinitionAuthoringConfirmation) CommittedAdmissionRef(record *DefinitionAuthoringRecord, id common.WorkspaceID) (DefinitionAuthoringAdmissionRef, error) {
	if err := c.ReadCommitted(record, id); err != nil {
		return DefinitionAuthoringAdmissionRef{}, err
	}
	return DefinitionAuthoringAdmissionRef{InputHash: c.InputHash, DerivationPermitID: c.DerivationPermitID, ProposalHash: c.ProposalHash, SpecificationHash: c.SpecificationHash}, nil
}

// ReadCommittedItems projects the retained source order without re-running
// derivation or semantic review. The complete source remains in the owner.
func (c DefinitionAuthoringConfirmation) ReadCommittedItems(record *DefinitionAuthoringRecord, id common.WorkspaceID) ([]DefinitionCandidateItem, error) {
	if err := c.ReadCommitted(record, id); err != nil {
		return nil, err
	}
	for _, d := range record.DerivationRequests {
		if d.PermitID == c.DerivationPermitID {
			items := d.Proposal.candidateItems(d.Input, &d.Template)
			if len(items) != len(c.DoD.Items) {
				return nil, common.ErrConflict
			}
			for i := range items {
				if items[i].Text != c.DoD.Items[i] {
					return nil, common.ErrConflict
				}
			}
			return items, nil
		}
	}
	return nil, common.ErrConflict
}

// This is an identity/content association check, not a second admission compiler.
func readCommittedCandidateConfirmation(candidate DefinitionAdmissionCandidate, c DefinitionAuthoringConfirmation) error {
	ref := DefinitionAuthoringAdmissionRef{InputHash: c.InputHash, DerivationPermitID: c.DerivationPermitID, ProposalHash: c.ProposalHash, SpecificationHash: c.SpecificationHash}
	if candidate.SchemaVersion != DefinitionAdmissionCandidateSchemaVersionCurrent || candidate.AdmissionCompilerVersion != DefinitionAdmissionCompilerVersionCurrent || candidate.WorkspaceID != c.Goal.WorkspaceID || candidate.CandidateHash != candidate.computeHash() || candidate.AuthoringRef == nil || *candidate.AuthoringRef != ref || candidate.Specification == nil || candidate.Specification.AdmissionHash() != c.SpecificationHash || candidate.GoalInput.Text != c.Goal.Text || candidate.GoalInput.SourceIdentity != "goal:"+string(c.Goal.ID)+"@"+c.Goal.DefinitionRevision || len(candidate.DoDInputs) != len(c.DoD.Items) {
		return common.ErrConflict
	}
	texts := make(map[string]int, len(candidate.DoDInputs))
	for _, item := range candidate.DoDInputs {
		if item.SourceIdentity != "dod:"+string(c.DoD.ID)+"@"+c.DoD.DefinitionRevision {
			return common.ErrConflict
		}
		texts[item.Text]++
	}
	for _, text := range c.DoD.Items {
		texts[text]--
	}
	for _, count := range texts {
		if count != 0 {
			return common.ErrConflict
		}
	}
	return nil
}

// ReadCommittedAuthoring checks the formal lineage without reconstructing any
// historical proof graph. The repository still checks original byte integrity.
func (w *Workspace) ReadCommittedAuthoring() error     { return w.readCommittedAuthoring(false) }
func (w *Workspace) ReadCommittedAuthoringView() error { return w.readCommittedAuthoring(true) }
func (w *Workspace) readCommittedAuthoring(view bool) error {
	if w == nil {
		return common.ErrInvalidArgument
	}
	if err := ValidateDefinitionAdmissionResultTransition(nil, w); err != nil {
		return err
	}
	if w.DefinitionAuthoring != nil && w.DefinitionAuthoring.readCommittedRecord(view) != nil {
		return common.ErrConflict
	}
	if w.DefinitionChangeCandidate != nil && w.DefinitionChangeCandidate.Record.readCommittedRecord(view) != nil {
		return common.ErrConflict
	}
	for i := range w.DefinitionAuthoringHistory {
		h := &w.DefinitionAuthoringHistory[i]
		if h.Record.readCommittedRecord(view) != nil {
			return common.ErrConflict
		}
		if h.Confirmation.Phase != DefinitionAuthoringConfirmationMaterialized || h.Confirmation.readCommitted(&h.Record, w.ID, true) != nil || i >= len(w.DefinitionAdmissionCandidates) || w.DefinitionAdmissionCandidates[i].CandidateRevision != uint64(i+1) || readCommittedCandidateConfirmation(w.DefinitionAdmissionCandidates[i], h.Confirmation) != nil {
			return common.ErrConflict
		}
		if i > 0 && (h.Confirmation.ValidateSuccessor(w.DefinitionAuthoringHistory[i-1].Confirmation) != nil || h.Record.Input.Revision <= w.DefinitionAuthoringHistory[i-1].Record.Input.Revision) {
			return common.ErrConflict
		}
	}
	if len(w.DefinitionAuthoringHistory) > 0 && (w.DefinitionAuthoring == nil || w.DefinitionAuthoring.Input.Revision <= w.DefinitionAuthoringHistory[len(w.DefinitionAuthoringHistory)-1].Record.Input.Revision) {
		return common.ErrConflict
	}
	if c := w.DefinitionAuthoringConfirmation; c != nil {
		if c.readCommitted(w.DefinitionAuthoring, w.ID, w.DefinitionAuthoring != nil) != nil {
			return common.ErrConflict
		}
		sequence := c.FormalSequence
		if sequence == 0 && c.PreviousInputHash == "" {
			sequence = 1
		}
		if sequence != uint64(len(w.DefinitionAuthoringHistory)+1) {
			return common.ErrConflict
		}
		if len(w.DefinitionAdmissionCandidates) == len(w.DefinitionAuthoringHistory)+1 && readCommittedCandidateConfirmation(w.DefinitionAdmissionCandidates[len(w.DefinitionAuthoringHistory)], *c) != nil {
			return common.ErrConflict
		}
		if len(w.DefinitionAuthoringHistory) > 0 && c.ValidateSuccessor(w.DefinitionAuthoringHistory[len(w.DefinitionAuthoringHistory)-1].Confirmation) != nil {
			return common.ErrConflict
		}
	}
	return nil
}

// ReadCommittedActivation consumes StartupInput/ActivationAdmission, the
// existing formal admission product. It authenticates identity and source
// associations and does not rerun admission/authoring compilers.
func (w *Workspace) ReadCommittedActivation() (DefinitionAdmissionCandidate, bool, error) {
	return w.readCommittedActivation(false)
}
func (w *Workspace) ReadCommittedActivationView() (DefinitionAdmissionCandidate, bool, error) {
	return w.readCommittedActivation(true)
}
func (w *Workspace) readCommittedActivation(view bool) (DefinitionAdmissionCandidate, bool, error) {
	if w == nil {
		return DefinitionAdmissionCandidate{}, false, common.ErrInvalidArgument
	}
	if err := w.readCommittedAuthoring(view); err != nil {
		return DefinitionAdmissionCandidate{}, false, err
	}
	if w.StartupInput == nil && w.ActivationAdmission == nil {
		return DefinitionAdmissionCandidate{}, false, nil
	}
	if w.StartupInput == nil || w.ActivationAdmission == nil {
		return DefinitionAdmissionCandidate{}, false, common.ErrConflict
	}
	a, s := w.ActivationAdmission, w.StartupInput
	if a.WorkspaceID != w.ID || s.WorkspaceID != w.ID || a.Validate(*s) != nil || s.Validate() != nil {
		return DefinitionAdmissionCandidate{}, false, common.ErrConflict
	}
	var selected *DefinitionAdmissionCandidate
	for i := range w.DefinitionAdmissionCandidates {
		c := &w.DefinitionAdmissionCandidates[i]
		if c.CandidateID == a.CandidateID && c.CandidateRevision == a.CandidateRevision {
			if selected != nil || c.SchemaVersion != DefinitionAdmissionCandidateSchemaVersionCurrent || c.AdmissionCompilerVersion != DefinitionAdmissionCompilerVersionCurrent || c.WorkspaceID != w.ID || c.CandidateHash != a.CandidateHash || c.CandidateHash != c.computeHash() {
				return DefinitionAdmissionCandidate{}, false, common.ErrConflict
			}
			selected = c
		}
	}
	if selected == nil {
		return DefinitionAdmissionCandidate{}, false, common.ErrConflict
	}
	if journal := w.DefinitionAuthoringConfirmation; journal != nil {
		if journal.Phase != DefinitionAuthoringConfirmationMaterialized || readCommittedCandidateConfirmation(*selected, *journal) != nil {
			return DefinitionAdmissionCandidate{}, false, common.ErrConflict
		}
	}
	return selected.Clone(), true, nil
}

// ValidateCommittedPreparationDefinition verifies the retained formal source
// associations, never recompiling a historical manifest or readiness result.
func ValidateCommittedPreparationDefinition(w *Workspace, a WorkspacePreparationAttempt) error {
	if a.DefinitionCompilation == nil {
		return nil
	}
	if a.DefinitionCompilation.ValidateFor(a) != nil {
		return common.ErrConflict
	}
	found := false
	for _, e := range w.PreparationSealEvidence {
		if e.Ref != a.SealEvidenceRef {
			continue
		}
		if found || e.ContentHash == "" || e.ContentHash != e.Hash() || !reflect.DeepEqual(e.Before, e.After) || e.Before.WorkspaceID != w.ID || e.Before.AuthoringRef != a.Binding.AuthoringRef {
			return common.ErrConflict
		}
		found = true
	}
	if !found {
		return fmt.Errorf("%w: committed preparation definition has no original seal", common.ErrConflict)
	}
	return nil
}

// ReadCommittedRecord checks the saved source identities and complete raw
// documents. Per-request and per-review semantic conclusions stay committed.
func (r DefinitionAuthoringRecord) ReadCommittedRecord() error { return r.readCommittedRecord(false) }
func (r DefinitionAuthoringRecord) ReadCommittedView() error   { return r.readCommittedRecord(true) }
func (r DefinitionAuthoringRecord) readCommittedRecord(view bool) error {
	inputHash := r.Input.InputHash()
	templateHash := WorkspaceTemplateSnapshotHash(&r.TemplateSnapshot)
	if r.InputHash != inputHash || r.TemplateSnapshotHash != templateHash || r.Input.Validate() != nil || len(r.DerivationRequests) > DefinitionDerivationRequestLimit || len(r.SpecificationRequests) > DefinitionAuthoringSpecificationRequestLimit {
		return common.ErrConflict
	}
	seen := map[string]bool{}
	for _, d := range r.DerivationRequests {
		dInputHash := inputHash
		if !sameDefinitionAuthoringInput(&d.Input, &r.Input) {
			dInputHash = d.Input.InputHash()
		}
		dTemplateHash := templateHash
		if !sameWorkspaceTemplate(&d.Template, &r.TemplateSnapshot) {
			dTemplateHash = WorkspaceTemplateSnapshotHash(&d.Template)
		}
		if d.Input.WorkspaceID != r.Input.WorkspaceID || d.Template.ID != d.Input.TemplateID || d.PermitID == "" || seen[d.PermitID] || d.PermitID != d.expectedPermitIDWithHashes(dInputHash, dTemplateHash) || d.ResourceSnapshotHash != DefinitionAuthoringResourcesHash(d.Resources) {
			return common.ErrConflict
		}
		seen[d.PermitID] = true
		if d.SuccessAudit != nil {
			if view {
				if d.SuccessAudit.ValidateMetadata() != nil {
					return common.ErrConflict
				}
			} else if d.SuccessAudit.Validate() != nil {
				return common.ErrConflict
			}
		}
		if (d.Proposal != nil) != (d.SuccessAudit != nil) || d.Proposal != nil && d.FailureCode != "" {
			return common.ErrConflict
		}
		if d.Diagnostic != nil {
			if view {
				if d.Diagnostic.ValidateMetadata() != nil {
					return common.ErrConflict
				}
			} else if d.Diagnostic.Validate() != nil {
				return common.ErrConflict
			}
		}
	}
	derivations := map[string]DefinitionDerivationRequest{}
	for _, d := range r.DerivationRequests {
		derivations[d.PermitID] = d
	}
	seen = map[string]bool{}
	for _, s := range r.SpecificationRequests {
		d, found := derivations[s.DerivationPermitID]
		if !found || d.Proposal == nil || d.FailureCode != "" || s.WireContractRef != d.WireContractRef || s.ProviderBindingHash != d.ProviderBindingHash {
			return common.ErrConflict
		}
		if s.PermitID == "" || seen[s.PermitID] || s.PermitID != s.ExpectedPermitID() {
			return common.ErrConflict
		}
		seen[s.PermitID] = true
		if s.Proposal != nil && (s.Mapping == nil || s.SuccessAudit == nil || s.CompletionIdentity != MappingCompletionIdentity(s.PermitID, *s.Mapping, *s.SuccessAudit) || s.FailureCode != "") {
			return common.ErrConflict
		}
		if s.SuccessAudit != nil {
			if view {
				if s.SuccessAudit.ValidateMetadata() != nil {
					return common.ErrConflict
				}
			} else if s.SuccessAudit.Validate() != nil {
				return common.ErrConflict
			}
		}
		if s.ReviewAudit != nil {
			if view {
				if s.ReviewAudit.ValidateMetadata() != nil {
					return common.ErrConflict
				}
			} else if s.ReviewAudit.Validate() != nil {
				return common.ErrConflict
			}
		}
		if (s.Review != nil) != (s.ReviewAudit != nil) || s.Review != nil && (s.FailureCode != "" || s.Review.SpecificationHash != s.ProposalHash || s.Review.Kind != s.ReviewKind) {
			return common.ErrConflict
		}
		if s.Diagnostic != nil {
			if view {
				if s.Diagnostic.ValidateMetadata() != nil {
					return common.ErrConflict
				}
			} else if s.Diagnostic.Validate() != nil {
				return common.ErrConflict
			}
		}
	}
	return nil
}

func ValidateDefinitionAdmissionResultTransition(current, next *Workspace) error {
	if next == nil {
		return common.ErrInvalidArgument
	}
	if current != nil && len(current.DefinitionAdmissionCandidates) > 0 && (len(next.DefinitionAdmissionCandidates) < len(current.DefinitionAdmissionCandidates) || !reflect.DeepEqual(current.DefinitionAdmissionCandidates, next.DefinitionAdmissionCandidates[:len(current.DefinitionAdmissionCandidates)])) {
		return common.ErrConflict
	}
	if current != nil && len(current.DefinitionAdmissionResults) > 0 && (len(next.DefinitionAdmissionResults) < len(current.DefinitionAdmissionResults) || !reflect.DeepEqual(current.DefinitionAdmissionResults, next.DefinitionAdmissionResults[:len(current.DefinitionAdmissionResults)])) {
		return common.ErrConflict
	}
	seen := map[string]bool{}
	for _, r := range next.DefinitionAdmissionResults {
		key := fmt.Sprintf("%s/%d", r.CandidateID, r.CandidateRevision)
		if seen[key] {
			return common.ErrConflict
		}
		seen[key] = true
		var c *DefinitionAdmissionCandidate
		for i := range next.DefinitionAdmissionCandidates {
			candidate := &next.DefinitionAdmissionCandidates[i]
			if candidate.CandidateID == r.CandidateID && candidate.CandidateRevision == r.CandidateRevision {
				if c != nil {
					return common.ErrConflict
				}
				c = candidate
			}
		}
		if c == nil || c.WorkspaceID != next.ID || c.CandidateHash != c.computeHash() || r.ValidateBinding(*c) != nil {
			return common.ErrConflict
		}
	}
	return nil
}

// ValidateCommittedAuthoringTransition protects original completed request
// outcomes at the existing storage CAS boundary. It does not adjudicate their
// semantics again. The private owner reducer alone accepts new completions.
func ValidateCommittedAuthoringTransition(current, next *Workspace) error {
	if next == nil {
		return common.ErrInvalidArgument
	}
	if current == nil {
		return nil
	}
	if len(next.DefinitionAuthoringHistory) < len(current.DefinitionAuthoringHistory) {
		return common.ErrConflict
	}
	for i := range current.DefinitionAuthoringHistory {
		if !reflect.DeepEqual(current.DefinitionAuthoringHistory[i], next.DefinitionAuthoringHistory[i]) {
			return common.ErrConflict
		}
	}
	// The confirmation after-images are immutable after prepare. The only
	// in-place transition is prepared -> materialized; a successor archives the
	// complete original journal before opening its separate input transaction.
	if c := current.DefinitionAuthoringConfirmation; c != nil {
		found := false
		if n := next.DefinitionAuthoringConfirmation; n != nil && n.InputHash == c.InputHash {
			expected := c.Clone()
			if c.Phase == DefinitionAuthoringConfirmationPrepared && n.Phase == DefinitionAuthoringConfirmationMaterialized {
				expected.Phase = n.Phase
			}
			found = reflect.DeepEqual(expected, *n)
		}
		if !found {
			for _, h := range next.DefinitionAuthoringHistory {
				if reflect.DeepEqual(h.Confirmation, *c) {
					found = true
					break
				}
			}
		}
		if !found {
			return common.ErrConflict
		}
	}
	derivations := map[string]DefinitionDerivationRequest{}
	specifications := map[string]DefinitionAuthoringSpecificationRequest{}
	for _, record := range committedAuthoringRecords(next) {
		for _, request := range record.DerivationRequests {
			if prior, found := derivations[request.PermitID]; found && !reflect.DeepEqual(prior, request) {
				return common.ErrConflict
			}
			derivations[request.PermitID] = request
		}
		for _, request := range record.SpecificationRequests {
			if prior, found := specifications[request.PermitID]; found && !reflect.DeepEqual(prior, request) {
				return common.ErrConflict
			}
			specifications[request.PermitID] = request
		}
	}
	for _, record := range committedAuthoringRecords(current) {
		for _, request := range record.DerivationRequests {
			after, found := derivations[request.PermitID]
			if !found {
				return common.ErrConflict
			}
			if request.Proposal == nil && request.FailureCode == "" {
				after.Proposal, after.FailureCode, after.Diagnostic, after.SuccessAudit = nil, "", nil, nil
			}
			if !reflect.DeepEqual(request, after) {
				return common.ErrConflict
			}
		}
		for _, request := range record.SpecificationRequests {
			after, found := specifications[request.PermitID]
			if !found {
				return common.ErrConflict
			}
			if request.Proposal == nil && request.Review == nil && request.FailureCode == "" {
				after.Proposal, after.Review, after.FailureCode, after.Diagnostic = nil, nil, "", nil
				after.Mapping, after.SuccessAudit, after.ReviewAudit, after.CompletionIdentity = nil, nil, nil, ""
			}
			if !reflect.DeepEqual(request, after) {
				return common.ErrConflict
			}
		}
	}
	return nil
}

func committedAuthoringRecords(w *Workspace) []*DefinitionAuthoringRecord {
	var records []*DefinitionAuthoringRecord
	if w.DefinitionAuthoring != nil {
		records = append(records, w.DefinitionAuthoring)
	}
	if w.DefinitionChangeCandidate != nil {
		records = append(records, &w.DefinitionChangeCandidate.Record)
	}
	for i := range w.DefinitionAuthoringHistory {
		records = append(records, &w.DefinitionAuthoringHistory[i].Record)
	}
	return records
}

// ReadCommittedAdmissionSource translates this owner's formal confirmation into
// its existing candidate representation. It does not decide readiness or rejudge reviews.
func (c DefinitionAuthoringConfirmation) ReadCommittedAdmissionSource(record *DefinitionAuthoringRecord, id common.WorkspaceID) (DefinitionCandidateItem, []DefinitionCandidateItem, GoalSpecification, DefinitionAuthoringAdmissionRef, error) {
	ref, err := c.CommittedAdmissionRef(record, id)
	if err != nil || c.Phase != DefinitionAuthoringConfirmationMaterialized {
		return DefinitionCandidateItem{}, nil, GoalSpecification{}, DefinitionAuthoringAdmissionRef{}, common.ErrConflict
	}
	items, err := c.ReadCommittedItems(record, id)
	if err != nil {
		return DefinitionCandidateItem{}, nil, GoalSpecification{}, DefinitionAuthoringAdmissionRef{}, err
	}
	var spec GoalSpecification
	for _, r := range record.SpecificationRequests {
		if r.Proposal != nil && r.DerivationPermitID == c.DerivationPermitID && r.Proposal.ContentHash() == c.ProposalHash {
			spec = r.Proposal.Clone()
		}
	}
	for _, kind := range goalReviewKinds {
		for _, r := range record.SpecificationRequests {
			if r.DerivationPermitID == c.DerivationPermitID && r.ProposalHash == c.ProposalHash && r.ReviewKind == kind && r.Review != nil {
				spec.Reviews = append(spec.Reviews, *r.Clone().Review)
			}
		}
	}
	for i := range items {
		items[i].SourceIdentity = "dod:" + string(c.DoD.ID) + "@" + c.DoD.DefinitionRevision
	}
	goal := DefinitionCandidateItem{CandidateItemRef: DefinitionAuthoringGoalItemRef, Text: c.Goal.Text, SourceIdentity: "goal:" + string(c.Goal.ID) + "@" + c.Goal.DefinitionRevision}
	return goal, items, spec, ref, nil
}

// CommittedDerivation selects a completed owner result, preserving its full source.
func (r DefinitionAuthoringRecord) CommittedDerivation(permit string) (DefinitionDerivationRequest, error) {
	if err := r.ReadCommittedRecord(); err != nil {
		return DefinitionDerivationRequest{}, err
	}
	for _, d := range r.DerivationRequests {
		if d.PermitID == permit && d.Proposal != nil && d.FailureCode == "" {
			return d.Clone(), nil
		}
	}
	return DefinitionDerivationRequest{}, common.ErrConflict
}

func sameDefinitionAuthoringInput(a, b *DefinitionAuthoringInput) bool {
	return a.Version == b.Version &&
		a.WorkspaceID == b.WorkspaceID &&
		a.Revision == b.Revision &&
		a.GoalText == b.GoalText &&
		a.TemplateID == b.TemplateID &&
		slices.Equal(a.AcceptanceRequirements, b.AcceptanceRequirements) &&
		slices.Equal(a.ResourceRefs, b.ResourceRefs)
}

func sameWorkspaceTemplate(a, b *WorkspaceTemplate) bool {
	if a.Builtin != b.Builtin ||
		a.ID != b.ID ||
		a.Name != b.Name ||
		a.Description != b.Description ||
		a.GoalText != b.GoalText ||
		a.DefaultSoulID != b.DefaultSoulID ||
		!slices.Equal(a.ParentTemplateIDs, b.ParentTemplateIDs) ||
		!slices.Equal(a.EffectiveSources, b.EffectiveSources) ||
		!slices.Equal(a.DoDItems, b.DoDItems) ||
		!maps.Equal(a.SoulBindings, b.SoulBindings) ||
		!samePayloadExact(a.ContextHints, b.ContextHints) ||
		len(a.EvidencePolicies) != len(b.EvidencePolicies) {
		return false
	}
	for i := range a.EvidencePolicies {
		if !samePayloadExact(a.EvidencePolicies[i], b.EvidencePolicies[i]) {
			return false
		}
	}
	return true
}

func samePayloadExact(a, b workorcheventpayload.Payload) bool {
	return samePayloadFieldsExact(a.Fields, b.Fields)
}

func samePayloadFieldsExact(a, b []workorcheventpayload.Field) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name || !samePayloadValueExact(a[i].Value, b[i].Value) {
			return false
		}
	}
	return true
}

func samePayloadValueExact(a, b workorcheventpayload.Value) bool {
	if a.Kind != b.Kind || a.String != b.String || a.Bool != b.Bool || a.Int != b.Int || a.Uint != b.Uint || math.Float64bits(a.Float) != math.Float64bits(b.Float) {
		return false
	}
	if !samePayloadFieldsExact(a.Object, b.Object) || len(a.Array) != len(b.Array) {
		return false
	}
	for i := range a.Array {
		if !samePayloadValueExact(a.Array[i], b.Array[i]) {
			return false
		}
	}
	return true
}
