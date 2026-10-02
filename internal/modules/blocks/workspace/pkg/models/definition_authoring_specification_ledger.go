package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"
)

const DefinitionAuthoringSpecificationRequestLimit = 112
const DefinitionAuthoringSpecificationAttemptLimit = 2

// Provider diagnostics are bounded independently from the existing request
// budget. Oversized values are represented by exact byte metadata, never a
// truncated prefix.
const DefinitionAuthoringDiagnosticDocumentLimit = 256 * 1024
const DefinitionAuthoringDiagnosticErrorLimit = 64 * 1024
const definitionAuthoringDiagnosticNotSavedReason = "diagnostic exceeds supported storage boundary"

// DefinitionAuthoringDiagnostic preserves provider/parser failure data without
// making it an authoring or admission authority.
type DefinitionAuthoringDiagnostic struct {
	ParseError       string
	ParseErrorSHA256 string
	ParseErrorBytes  uint64
	ParseErrorSaved  bool
	Payload          []byte
	PayloadSHA256    string
	PayloadBytes     uint64
	PayloadSaved     bool
	NotSavedReason   string
}

func NewDefinitionAuthoringDiagnostic(parseError string, payload []byte) DefinitionAuthoringDiagnostic {
	d := DefinitionAuthoringDiagnostic{ParseErrorBytes: uint64(len(parseError)), PayloadBytes: uint64(len(payload))}
	if parseError != "" {
		h := sha256.Sum256([]byte(parseError))
		d.ParseErrorSHA256 = hex.EncodeToString(h[:])
		if len(parseError) <= DefinitionAuthoringDiagnosticErrorLimit {
			d.ParseError = parseError
			d.ParseErrorSaved = true
		}
	}
	h := sha256.Sum256(payload)
	d.PayloadSHA256 = hex.EncodeToString(h[:])
	if len(payload) <= DefinitionAuthoringDiagnosticDocumentLimit {
		d.Payload = append([]byte{}, payload...)
		d.PayloadSaved = true
	}
	if !d.ParseErrorSaved || !d.PayloadSaved {
		d.NotSavedReason = definitionAuthoringDiagnosticNotSavedReason
	}
	return d
}

func (d DefinitionAuthoringDiagnostic) Clone() DefinitionAuthoringDiagnostic {
	if d.Payload != nil {
		d.Payload = append([]byte{}, d.Payload...)
	}
	return d
}

func (d DefinitionAuthoringDiagnostic) Validate() error {
	if !utf8.ValidString(d.ParseError) {
		return fmt.Errorf("authoring diagnostic error is not UTF-8")
	}
	if d.ParseErrorBytes == 0 || d.ParseErrorSHA256 == "" || len(d.ParseErrorSHA256) != sha256.Size*2 {
		return fmt.Errorf("authoring diagnostic requires non-empty error identity")
	}
	if strings.ToLower(d.ParseErrorSHA256) != d.ParseErrorSHA256 {
		return fmt.Errorf("authoring diagnostic error hash casing invalid")
	}
	if _, err := hex.DecodeString(d.ParseErrorSHA256); err != nil {
		return fmt.Errorf("authoring diagnostic error hash invalid")
	}
	if d.PayloadSHA256 == "" || len(d.PayloadSHA256) != sha256.Size*2 {
		return fmt.Errorf("authoring diagnostic payload hash invalid")
	}
	if strings.ToLower(d.PayloadSHA256) != d.PayloadSHA256 {
		return fmt.Errorf("authoring diagnostic payload hash casing invalid")
	}
	if _, err := hex.DecodeString(d.PayloadSHA256); err != nil {
		return fmt.Errorf("authoring diagnostic payload hash invalid")
	}
	if d.ParseErrorSaved {
		if d.ParseErrorBytes > DefinitionAuthoringDiagnosticErrorLimit || uint64(len(d.ParseError)) != d.ParseErrorBytes {
			return fmt.Errorf("authoring diagnostic error byte count/limit mismatch")
		}
		h := sha256.Sum256([]byte(d.ParseError))
		if hex.EncodeToString(h[:]) != d.ParseErrorSHA256 {
			return fmt.Errorf("authoring diagnostic error hash mismatch")
		}
	} else if d.ParseError != "" || d.ParseErrorBytes <= DefinitionAuthoringDiagnosticErrorLimit {
		return fmt.Errorf("authoring diagnostic error withheld without exceeding limit")
	}
	if d.PayloadSaved {
		if d.PayloadBytes > DefinitionAuthoringDiagnosticDocumentLimit || uint64(len(d.Payload)) != d.PayloadBytes || d.Payload == nil {
			return fmt.Errorf("authoring diagnostic payload byte count/limit mismatch")
		}
		h := sha256.Sum256(d.Payload)
		if hex.EncodeToString(h[:]) != d.PayloadSHA256 {
			return fmt.Errorf("authoring diagnostic payload hash mismatch")
		}
	} else if d.Payload != nil || d.PayloadBytes <= DefinitionAuthoringDiagnosticDocumentLimit {
		return fmt.Errorf("authoring diagnostic payload withheld without exceeding limit")
	}
	if d.ParseErrorSaved && d.PayloadSaved {
		if d.NotSavedReason != "" {
			return fmt.Errorf("saved authoring diagnostic has unsaved reason")
		}
	} else if d.NotSavedReason != definitionAuthoringDiagnosticNotSavedReason {
		return fmt.Errorf("authoring diagnostic not-saved reason mismatch")
	}
	return nil
}

// A specification request records a single possible physical call. Neither a
// reserved request nor a completed review grants admission.
type DefinitionAuthoringSpecificationRequest struct {
	RequestID           string
	DerivationPermitID  string
	Phase               DefinitionSpecificationPhase
	ReviewKind          string
	ProposalHash        string
	ProviderBindingHash string
	PermitID            string
	Proposal            *GoalSpecification
	Review              *GoalSpecificationCompletenessReview
	FailureCode         string
	Diagnostic          *DefinitionAuthoringDiagnostic
}

func (r DefinitionAuthoringSpecificationRequest) Clone() DefinitionAuthoringSpecificationRequest {
	if r.Proposal != nil {
		proposal := r.Proposal.Clone()
		r.Proposal = &proposal
	}
	if r.Review != nil {
		review := *r.Review
		review.EvidenceRefs = append([]string(nil), review.EvidenceRefs...)
		review.RequirementIDs = append([]string(nil), review.RequirementIDs...)
		r.Review = &review
	}
	if r.Diagnostic != nil {
		d := r.Diagnostic.Clone()
		r.Diagnostic = &d
	}
	return r
}

func (r DefinitionAuthoringSpecificationRequest) ExpectedPermitID() string {
	h := sha256.New()
	writeDefinitionHash(h, "workspace-authoring-specification-permit-v1", r.RequestID, r.DerivationPermitID, string(r.Phase), r.ReviewKind, r.ProposalHash, r.ProviderBindingHash)
	return "authoring-spec-permit-sha256:" + hex.EncodeToString(h.Sum(nil))
}

func (r DefinitionAuthoringSpecificationRequest) Validate() error {
	if r.RequestID == "" || len(r.RequestID) > 128 || r.DerivationPermitID == "" || len(r.DerivationPermitID) > 256 || r.ProviderBindingHash == "" || len(r.ProviderBindingHash) > 256 || r.PermitID != r.ExpectedPermitID() {
		return fmt.Errorf("authoring specification request identity invalid")
	}
	switch r.Phase {
	case DefinitionSpecificationProposal:
		if r.ReviewKind != "" || r.ProposalHash != "" {
			return fmt.Errorf("authoring proposal cannot claim a review or proposal hash")
		}
	case DefinitionSpecificationCritic:
		if !GoalSpecificationReviewKind(r.ReviewKind) || r.ProposalHash == "" || len(r.ProposalHash) > 256 {
			return fmt.Errorf("authoring critic requires exact proposal identity")
		}
	default:
		return fmt.Errorf("authoring specification phase invalid")
	}
	if r.FailureCode != "" {
		if r.Diagnostic != nil && (r.FailureCode == "provider_failed" || r.Diagnostic.Validate() != nil) {
			return fmt.Errorf("authoring failure diagnostic invalid")
		}
		if r.FailureCode != "provider_failed" && r.FailureCode != "invalid_proposal" && r.FailureCode != "invalid_review" || r.Proposal != nil || r.Review != nil {
			return fmt.Errorf("authoring specification failure invalid")
		}
		if r.Phase == DefinitionSpecificationProposal && r.FailureCode == "invalid_review" || r.Phase == DefinitionSpecificationCritic && r.FailureCode == "invalid_proposal" {
			return fmt.Errorf("authoring specification failure phase mismatch")
		}
		return nil
	}
	if r.Diagnostic != nil || r.Proposal != nil && r.Review != nil || r.Proposal != nil && (r.Phase != DefinitionSpecificationProposal || len(r.Proposal.Reviews) != 0) || r.Review != nil && (r.Phase != DefinitionSpecificationCritic || r.Review.Kind != r.ReviewKind || r.Review.SpecificationHash != r.ProposalHash) {
		return fmt.Errorf("authoring specification outcome phase mismatch")
	}
	return nil
}

func validateAuthoringSpecificationRequests(record DefinitionAuthoringRecord) error {
	if len(record.SpecificationRequests) > DefinitionAuthoringSpecificationRequestLimit {
		return fmt.Errorf("authoring specification request budget exceeded")
	}
	seen := make(map[string]bool, len(record.SpecificationRequests))
	attempts := map[string]int{}
	type completedProposal struct {
		specification *GoalSpecification
		context       authoringSpecificationContext
		hash          string
	}
	completedProposals := map[string]completedProposal{}
	contexts := map[string]authoringSpecificationContext{}
	for _, request := range record.SpecificationRequests {
		if err := request.Validate(); err != nil {
			return err
		}
		if seen[request.RequestID] {
			return fmt.Errorf("authoring specification request ID repeated")
		}
		seen[request.RequestID] = true
		derivation, err := record.derivationForSpecification(request.DerivationPermitID)
		if err != nil {
			return err
		}
		if request.ProviderBindingHash != derivation.ProviderBindingHash {
			return fmt.Errorf("authoring specification provider binding differs from derivation")
		}
		key := request.DerivationPermitID + "\x00" + string(request.Phase) + "\x00" + request.ReviewKind
		attempts[key]++
		if attempts[key] > DefinitionAuthoringSpecificationAttemptLimit {
			return fmt.Errorf("authoring specification phase attempt budget exceeded")
		}
		if request.Phase == DefinitionSpecificationProposal {
			if request.Proposal != nil {
				context, present := contexts[request.DerivationPermitID]
				if !present {
					context, err = newAuthoringSpecificationContext(derivation.Input, &derivation.Template, *derivation.Proposal, derivation.Resources)
					if err != nil {
						return err
					}
					contexts[request.DerivationPermitID] = context
				}
				if err := context.validate(*request.Proposal); err != nil {
					return fmt.Errorf("authoring specification proposal changed sealed requirements: %w", err)
				}
				hash := request.Proposal.ContentHash()
				completedProposals[request.DerivationPermitID+"\x00"+hash] = completedProposal{specification: request.Proposal, context: context, hash: hash}
			}
			continue
		}
		proposal, present := completedProposals[request.DerivationPermitID+"\x00"+request.ProposalHash]
		if !present {
			return fmt.Errorf("authoring critic lacks preceding completed proposal")
		}
		if request.Review != nil {
			if err := proposal.context.validateReview(*request.Review, *proposal.specification, proposal.hash); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c authoringSpecificationContext) validateReview(review GoalSpecificationCompletenessReview, proposal GoalSpecification, proposalHash string) error {
	seed := proposal.Clone()
	seed.Reviews = nil
	for _, kind := range goalReviewKinds {
		entry := GoalSpecificationCompletenessReview{Kind: kind, Status: GoalReviewUnknown, SpecificationHash: proposalHash}
		if kind == review.Kind {
			entry = review
		}
		seed.Reviews = append(seed.Reviews, entry)
	}
	return seed.Validate(c.goal, c.items)
}

func (r DefinitionAuthoringRecord) derivationForSpecification(permitID string) (DefinitionDerivationRequest, error) {
	for _, request := range r.DerivationRequests {
		if request.PermitID == permitID {
			if request.Proposal == nil || request.FailureCode != "" {
				return DefinitionDerivationRequest{}, fmt.Errorf("authoring specification requires completed derivation")
			}
			return request, nil
		}
	}
	return DefinitionDerivationRequest{}, fmt.Errorf("authoring specification derivation not found")
}

func (r DefinitionAuthoringRecord) completedSpecificationProposal(derivationPermitID, hash string) *GoalSpecification {
	for _, request := range r.SpecificationRequests {
		if request.DerivationPermitID == derivationPermitID && request.Phase == DefinitionSpecificationProposal && request.Proposal != nil && request.Proposal.ContentHash() == hash {
			proposal := request.Proposal.Clone()
			return &proposal
		}
	}
	return nil
}

func (r DefinitionAuthoringRecord) ReserveSpecification(request DefinitionAuthoringSpecificationRequest) (DefinitionAuthoringRecord, DefinitionAuthoringSpecificationRequest, bool, error) {
	if err := r.Validate(); err != nil {
		return DefinitionAuthoringRecord{}, request, false, err
	}
	request.PermitID = request.ExpectedPermitID()
	if err := request.Validate(); err != nil || request.Proposal != nil || request.Review != nil || request.FailureCode != "" {
		return DefinitionAuthoringRecord{}, request, false, fmt.Errorf("authoring specification reservation invalid")
	}
	derivation, err := r.derivationForSpecification(request.DerivationPermitID)
	if err != nil {
		return DefinitionAuthoringRecord{}, request, false, err
	}
	if derivation.Input.InputHash() != r.InputHash || WorkspaceTemplateSnapshotHash(&derivation.Template) != r.TemplateSnapshotHash {
		return DefinitionAuthoringRecord{}, request, false, fmt.Errorf("authoring specification input became stale")
	}
	if request.ProviderBindingHash != derivation.ProviderBindingHash {
		return DefinitionAuthoringRecord{}, request, false, fmt.Errorf("authoring specification provider binding changed after derivation")
	}
	for _, previous := range r.SpecificationRequests {
		if previous.RequestID == request.RequestID {
			if previous.PermitID != request.PermitID {
				return DefinitionAuthoringRecord{}, request, false, fmt.Errorf("authoring specification request identity conflict")
			}
			return r.Clone(), previous.Clone(), false, nil
		}
	}
	if len(r.SpecificationRequests) >= DefinitionAuthoringSpecificationRequestLimit {
		return DefinitionAuthoringRecord{}, request, false, fmt.Errorf("authoring specification request budget exhausted")
	}
	attempts := 0
	for _, previous := range r.SpecificationRequests {
		if previous.DerivationPermitID == request.DerivationPermitID && previous.Phase == request.Phase && previous.ReviewKind == request.ReviewKind {
			attempts++
		}
	}
	if attempts >= DefinitionAuthoringSpecificationAttemptLimit {
		return DefinitionAuthoringRecord{}, request, false, fmt.Errorf("authoring specification phase attempt budget exhausted")
	}
	if request.Phase == DefinitionSpecificationCritic && r.completedSpecificationProposal(request.DerivationPermitID, request.ProposalHash) == nil {
		return DefinitionAuthoringRecord{}, request, false, fmt.Errorf("authoring critic requires completed proposal")
	}
	next := r.Clone()
	next.SpecificationRequests = append(next.SpecificationRequests, request.Clone())
	return next, request, true, nil
}

func (r DefinitionAuthoringRecord) CompleteSpecification(permitID string, proposal *GoalSpecification, review *GoalSpecificationCompletenessReview, failureCode string, diagnostics ...*DefinitionAuthoringDiagnostic) (DefinitionAuthoringRecord, error) {
	if err := r.Validate(); err != nil {
		return DefinitionAuthoringRecord{}, err
	}
	if len(diagnostics) > 1 {
		return DefinitionAuthoringRecord{}, fmt.Errorf("authoring completion has multiple diagnostics")
	}
	var diagnostic *DefinitionAuthoringDiagnostic
	if len(diagnostics) == 1 {
		diagnostic = diagnostics[0]
	}
	for n, previous := range r.SpecificationRequests {
		if previous.PermitID != permitID {
			continue
		}
		completed := previous.Clone()
		completed.Proposal, completed.Review, completed.FailureCode = proposal, review, failureCode
		if diagnostic != nil {
			d := diagnostic.Clone()
			completed.Diagnostic = &d
		}
		completed = completed.Clone()
		outcomes := 0
		if completed.Proposal != nil {
			outcomes++
		}
		if completed.Review != nil {
			outcomes++
		}
		if completed.FailureCode != "" {
			outcomes++
		}
		if outcomes != 1 {
			return DefinitionAuthoringRecord{}, fmt.Errorf("authoring specification completion requires one outcome")
		}
		if err := completed.Validate(); err != nil {
			return DefinitionAuthoringRecord{}, err
		}
		if previous.Proposal != nil || previous.Review != nil || previous.FailureCode != "" {
			if !reflect.DeepEqual(previous, completed) {
				return DefinitionAuthoringRecord{}, fmt.Errorf("authoring specification completion conflict")
			}
			return r.Clone(), nil
		}
		next := r.Clone()
		next.SpecificationRequests[n] = completed
		if err := next.Validate(); err != nil {
			return DefinitionAuthoringRecord{}, err
		}
		return next, nil
	}
	return DefinitionAuthoringRecord{}, fmt.Errorf("authoring specification permit not found")
}
