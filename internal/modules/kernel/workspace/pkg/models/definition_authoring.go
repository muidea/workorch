package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
)

func (r DefinitionAuthoringRecord) CurrentAuthoringSummary() (state, failurePhase, failureCode string, reviewCount int) {
	input := r.Input.Normalized()
	if r.ReadCommittedRecord() != nil || input.InputHash() != r.InputHash {
		return "", "", "", 0
	}
	state = "draft"
	var derivation *DefinitionDerivationRequest
	for n := len(r.DerivationRequests) - 1; n >= 0; n-- {
		request := &r.DerivationRequests[n]
		if request.Input.InputHash() != r.InputHash || WorkspaceTemplateSnapshotHash(&request.Template) != r.TemplateSnapshotHash {
			continue
		}
		derivation = request
		break
	}
	if derivation != nil {
		state = "deriving"
		switch {
		case derivation.FailureCode != "":
			if _, retryable := r.DerivationRetryOrdinalForCurrent(); !retryable {
				state, failurePhase, failureCode = "failed", "derivation", derivation.FailureCode
			}
		case derivation.Proposal == nil:
			state = "outcome_unresolved"
		default:
			state = "derived"
			proposal, propFailed, propUnresolved, propRetryable, propErr := r.SettledProposalState(derivation.PermitID)
			if propErr != nil {
				return "", "", "", 0
			}
			switch {
			case propUnresolved:
				state = "outcome_unresolved"
			case propFailed != nil:
				if propRetryable {
					if proposal != nil {
						state = "reviewing"
					}
				} else {
					state, failurePhase, failureCode = "failed", "proposal", propFailed.FailureCode
				}
			case proposal != nil:
				proposalHash := proposal.ContentHash()
				state = "reviewing"
				reviewFailed, reviewUnresolved, reviewBlocked := false, false, false
				for _, kind := range goalReviewKinds {
					review, cFailed, cUnresolved, cRetryable, cErr := r.SettledCriticReview(derivation.PermitID, proposalHash, kind)
					if cErr != nil {
						return "", "", "", 0
					}
					if cUnresolved {
						reviewUnresolved = true
						continue
					}
					if cFailed != nil {
						if !cRetryable {
							reviewFailed = true
							if failureCode == "" {
								failurePhase, failureCode = "review", cFailed.FailureCode
							}
						}
						continue
					}
					if review != nil {
						reviewCount++
						if review.Status != GoalReviewPass {
							reviewBlocked = true
						}
					}
				}
				switch {
				case reviewUnresolved:
					state = "outcome_unresolved"
				case reviewFailed:
					state = "failed"
				case reviewBlocked:
					if reviewCount == len(goalReviewKinds) {
						state = "review_blocked"
						latest := r.LatestSpecificationRequest(derivation.PermitID)
						if latest != nil && latest.PreviousProposalHash == "" {
							if _, err := r.SpecificationFeedback(derivation.PermitID, proposalHash); err == nil {
								state = "reviewing"
							}
						}
					}
				default:
					if !hasCurrentPreparationDiagnostic(r, derivation) {
						if reviewCount == len(goalReviewKinds) {
							state = "reviewed"
							for _, verification := range proposal.Verifications {
								if verification.Status == GoalVerificationUnverified {
									state = "review_blocked"
								}
							}
						}
					}
				}
			}
		}
	}
	// 未完成许可的优先级高于已知失败，与详细创作视图保持一致。
	for _, request := range r.DerivationRequests {
		if request.Input.InputHash() == r.InputHash && WorkspaceTemplateSnapshotHash(&request.Template) == r.TemplateSnapshotHash && request.FailureCode == "" && request.Proposal == nil {
			return "outcome_unresolved", "", "", reviewCount
		}
	}
	if derivation != nil && derivation.Proposal != nil {
		for _, request := range r.SpecificationRequests {
			if request.DerivationPermitID == derivation.PermitID && request.FailureCode == "" && request.Proposal == nil && request.Review == nil {
				return "outcome_unresolved", "", "", reviewCount
			}
		}
	}
	for _, diagnostic := range r.PreparationDiagnostics {
		if !preparationDiagnosticCurrentForSummary(r, derivation, diagnostic) {
			continue
		}
		if state == "draft" || state == "deriving" || state == "derived" || state == "reviewing" || state == "review_blocked" {
			state, failurePhase, failureCode = "preparation_failed", string(diagnostic.Stage), diagnostic.PhaseCode
		}
	}
	return state, failurePhase, failureCode, reviewCount
}

func hasCurrentPreparationDiagnostic(record DefinitionAuthoringRecord, derivation *DefinitionDerivationRequest) bool {
	for _, diagnostic := range record.PreparationDiagnostics {
		if preparationDiagnosticCurrentForSummary(record, derivation, diagnostic) {
			return true
		}
	}
	return false
}

func (record DefinitionAuthoringRecord) PreparationDiagnosticCurrent(derivation *DefinitionDerivationRequest, diagnostic DefinitionAuthoringPreparationDiagnostic) bool {
	return preparationDiagnosticCurrentForSummary(record, derivation, diagnostic)
}

func preparationDiagnosticCurrentForSummary(record DefinitionAuthoringRecord, derivation *DefinitionDerivationRequest, diagnostic DefinitionAuthoringPreparationDiagnostic) bool {
	if diagnostic.InputHash != record.InputHash || diagnostic.TemplateSnapshotHash != record.TemplateSnapshotHash {
		return false
	}
	for _, request := range record.DerivationRequests {
		if request.Input.InputHash() == record.InputHash && WorkspaceTemplateSnapshotHash(&request.Template) == record.TemplateSnapshotHash && request.Proposal == nil && request.FailureCode == "" {
			return false
		}
	}
	if derivation == nil {
		return diagnostic.Stage == DefinitionAuthoringPreparationDerivation
	}
	if derivation.FailureCode != "" {
		if _, retryable := record.DerivationRetryOrdinalForCurrent(); retryable {
			return diagnostic.Stage == DefinitionAuthoringPreparationDerivation && diagnostic.TargetRequestID == record.NextDerivationRequestID()
		}
		return false
	}
	if derivation.Proposal == nil || diagnostic.DerivationPermitID != derivation.PermitID {
		return false
	}
	for _, request := range record.SpecificationRequests {
		if request.DerivationPermitID == derivation.PermitID && request.Proposal == nil && request.Review == nil && request.FailureCode == "" {
			return false
		}
	}
	proposal, propFailed, propUnresolved, propRetryable, propErr := record.SettledProposalState(derivation.PermitID)
	if propErr != nil || propUnresolved {
		return false
	}
	if propFailed != nil {
		if !propRetryable {
			return false
		}
		slot := record.SpecificationSlotRequests(derivation.PermitID, DefinitionSpecificationProposal, "", "", propFailed.PreviousProposalHash, propFailed.FeedbackHash)
		baseID := SpecificationRequestID(derivation.PermitID, "", "", propFailed.FeedbackHash)
		return diagnostic.Stage == DefinitionAuthoringPreparationProposal && diagnostic.TargetRequestID == AuthoringRetryRequestID(baseID, len(slot))
	}
	if proposal == nil {
		return diagnostic.Stage == DefinitionAuthoringPreparationProposal && diagnostic.TargetRequestID == SpecificationRequestID(derivation.PermitID, "", "", "")
	}
	proposalHash := proposal.ContentHash()
	for _, kind := range goalReviewKinds {
		review, cFailed, cUnresolved, cRetryable, cErr := record.SettledCriticReview(derivation.PermitID, proposalHash, kind)
		if cErr != nil || cUnresolved {
			return false
		}
		if cFailed != nil {
			if !cRetryable {
				return false
			}
			slot := record.SpecificationSlotRequests(derivation.PermitID, DefinitionSpecificationCritic, kind, proposalHash, "", "")
			baseID := SpecificationRequestID(derivation.PermitID, kind, proposalHash, "")
			return diagnostic.Stage == DefinitionAuthoringPreparationReview && diagnostic.ReviewKind == kind && diagnostic.ProposalHash == proposalHash && diagnostic.TargetRequestID == AuthoringRetryRequestID(baseID, len(slot))
		}
		if review == nil {
			return diagnostic.Stage == DefinitionAuthoringPreparationReview && diagnostic.ReviewKind == kind && diagnostic.ProposalHash == proposalHash && diagnostic.TargetRequestID == SpecificationRequestID(derivation.PermitID, kind, proposalHash, "")
		}
	}
	latest := record.LatestSpecificationRequest(derivation.PermitID)
	if latest != nil && latest.PreviousProposalHash == "" {
		feedback, err := record.SpecificationFeedback(derivation.PermitID, proposalHash)
		if err == nil {
			return diagnostic.Stage == DefinitionAuthoringPreparationProposal && diagnostic.TargetRequestID == SpecificationRequestID(derivation.PermitID, "", "", feedback.ContentHash())
		}
	}
	return false
}

// DefinitionAuthoringInput 是尚未派生 DoD 的创作输入，不可作为准入候选。
// 资源仅含用户引用；可用性和快照必须由能力 owner 另行提供。
type DefinitionAuthoringInput struct {
	Version                uint32
	WorkspaceID            common.WorkspaceID
	Revision               uint64
	GoalText               string
	AcceptanceRequirements []string
	TemplateID             string
	ResourceRefs           []string
}

type DefinitionAuthoringRecord struct {
	Input                  DefinitionAuthoringInput
	InputHash              string
	TemplateSnapshotHash   string
	TemplateSnapshot       WorkspaceTemplate
	DerivationRequests     []DefinitionDerivationRequest
	SpecificationRequests  []DefinitionAuthoringSpecificationRequest
	SaveReceipts           []DefinitionAuthoringSaveReceipt
	PreparationDiagnostics []DefinitionAuthoringPreparationDiagnostic
}

func (r DefinitionAuthoringRecord) Clone() DefinitionAuthoringRecord {
	r.PreparationDiagnostics = append([]DefinitionAuthoringPreparationDiagnostic(nil), r.PreparationDiagnostics...)
	for n := range r.PreparationDiagnostics {
		r.PreparationDiagnostics[n] = r.PreparationDiagnostics[n].Clone()
	}
	r.SaveReceipts = append([]DefinitionAuthoringSaveReceipt(nil), r.SaveReceipts...)
	r.Input = r.Input.Clone()
	r.TemplateSnapshot = r.TemplateSnapshot.Clone()
	r.DerivationRequests = append([]DefinitionDerivationRequest(nil), r.DerivationRequests...)
	for n := range r.DerivationRequests {
		r.DerivationRequests[n] = r.DerivationRequests[n].Clone()
	}
	r.SpecificationRequests = append([]DefinitionAuthoringSpecificationRequest(nil), r.SpecificationRequests...)
	for n := range r.SpecificationRequests {
		r.SpecificationRequests[n] = r.SpecificationRequests[n].Clone()
	}
	return r
}

func (r DefinitionAuthoringRecord) Validate() error { return r.validate(false) }

// ValidateView validates only the canonical/metadata projection. It is not raw
// persistence validation and cannot authorize an owner mutation.
func (r DefinitionAuthoringRecord) ValidateView() error { return r.validate(true) }
func (r DefinitionAuthoringRecord) validate(view bool) error {
	if err := r.Input.Validate(); err != nil {
		return err
	}
	if r.InputHash != r.Input.InputHash() || r.TemplateSnapshot.ID != r.Input.Normalized().TemplateID || r.TemplateSnapshotHash != WorkspaceTemplateSnapshotHash(&r.TemplateSnapshot) {
		return fmt.Errorf("authoring record identity mismatch")
	}
	if len(r.SaveReceipts) > DefinitionAuthoringSaveReceiptLimit {
		return fmt.Errorf("authoring save receipt limit exceeded")
	}
	seen := make(map[string]bool, len(r.SaveReceipts))
	for _, receipt := range r.SaveReceipts {
		if receipt.Validate() != nil || seen[receipt.RequestID] {
			return fmt.Errorf("authoring save receipt identity mismatch")
		}
		seen[receipt.RequestID] = true
	}
	if err := validateDerivationRequestsMode(r.DerivationRequests, r.Input, view); err != nil {
		return err
	}
	if err := validatePreparationDiagnosticsMode(r, view); err != nil {
		return err
	}
	return validateAuthoringSpecificationRequestsMode(r, view)
}

const DefinitionAuthoringInputVersionCurrent uint32 = 1

func (i DefinitionAuthoringInput) Clone() DefinitionAuthoringInput {
	i.AcceptanceRequirements = append([]string(nil), i.AcceptanceRequirements...)
	i.ResourceRefs = append([]string(nil), i.ResourceRefs...)
	return i
}

func (i DefinitionAuthoringInput) Normalized() DefinitionAuthoringInput {
	i = i.Clone()
	i.GoalText = strings.TrimSpace(i.GoalText)
	i.TemplateID = strings.TrimSpace(i.TemplateID)
	for n := range i.AcceptanceRequirements {
		i.AcceptanceRequirements[n] = strings.TrimSpace(i.AcceptanceRequirements[n])
	}
	for n := range i.ResourceRefs {
		i.ResourceRefs[n] = strings.TrimSpace(i.ResourceRefs[n])
	}
	return i
}

func (i DefinitionAuthoringInput) Validate() error {
	i = i.Normalized()
	if i.Version != DefinitionAuthoringInputVersionCurrent || i.WorkspaceID == "" || i.Revision == 0 {
		return fmt.Errorf("definition authoring input requires version, workspace and revision")
	}
	validText := func(value string, limit int) bool {
		return value != "" && len(value) <= limit && utf8.ValidString(value) && !strings.ContainsRune(value, 0)
	}
	if !validText(string(i.WorkspaceID), 256) || !validText(i.GoalText, 32*1024) || !validText(i.TemplateID, 256) {
		return fmt.Errorf("definition authoring input has invalid goal or scope")
	}
	if len(i.AcceptanceRequirements) > 128 || len(i.ResourceRefs) > DefinitionAuthoringResourceLimit {
		return fmt.Errorf("definition authoring input exceeds item bounds")
	}
	total := len(i.GoalText) + len(i.TemplateID) + len(i.WorkspaceID)
	for _, requirement := range i.AcceptanceRequirements {
		if !validText(requirement, 4096) {
			return fmt.Errorf("definition authoring acceptance requirement is empty or invalid")
		}
		total += len(requirement)
	}
	seen := map[string]bool{}
	for _, ref := range i.ResourceRefs {
		if !validText(ref, 1024) || seen[ref] {
			return fmt.Errorf("definition authoring resource reference is invalid or duplicated")
		}
		seen[ref] = true
		total += len(ref)
	}
	if total > 128*1024 {
		return fmt.Errorf("definition authoring input exceeds byte budget")
	}
	return nil
}

// InputHash 包含 revision，防止 A→B→A 的旧确认绑定新输入。
// 输入顺序亦被封存，不能通过排序或去重静默丢弃用户要求。
func (i DefinitionAuthoringInput) InputHash() string {
	i = i.Normalized()
	h := sha256.New()
	writeDefinitionHash(h, "workspace-definition-authoring-v1", fmt.Sprint(i.Version), string(i.WorkspaceID), fmt.Sprint(i.Revision), i.GoalText, i.TemplateID)
	writeDefinitionHash(h, fmt.Sprint(len(i.AcceptanceRequirements)))
	writeDefinitionHash(h, i.AcceptanceRequirements...)
	writeDefinitionHash(h, fmt.Sprint(len(i.ResourceRefs)))
	writeDefinitionHash(h, i.ResourceRefs...)
	return "definition-authoring-sha256:" + hex.EncodeToString(h.Sum(nil))
}
