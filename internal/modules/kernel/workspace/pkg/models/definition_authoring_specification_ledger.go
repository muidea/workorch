package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const DefinitionAuthoringSpecificationRequestLimit = 112
const DefinitionAuthoringSpecificationAttemptLimit = 2
const AuthoringProviderTransientRetryLimit = 3

// Provider diagnostics are bounded independently from the existing request
// budget. Oversized values are represented by exact byte metadata, never a
// truncated prefix.
const DefinitionAuthoringDiagnosticDocumentLimit = 256 * 1024
const DefinitionAuthoringDiagnosticErrorLimit = 64 * 1024
const definitionAuthoringDiagnosticNotSavedReason = "diagnostic exceeds supported storage boundary"

// DefinitionAuthoringDiagnostic preserves provider/parser failure data without
// making it an authoring or admission authority.
type DefinitionAuthoringDiagnostic struct {
	ParseError           string
	ParseErrorSHA256     string
	ParseErrorBytes      uint64
	ParseErrorSaved      bool
	Payload              []byte
	PayloadSHA256        string
	PayloadBytes         uint64
	PayloadSaved         bool
	NotSavedReason       string
	ProviderCode         string                         `json:",omitempty"`
	ProviderFailureClass string                         `json:",omitempty"`
	ProviderDocument     *DefinitionProviderObservation `json:",omitempty"`
	ProviderWire         *DefinitionProviderObservation `json:",omitempty"`
	ProviderCause        *DefinitionProviderObservation `json:",omitempty"`
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
	if d.ProviderDocument != nil {
		observation := d.ProviderDocument.Clone()
		d.ProviderDocument = &observation
	}
	if d.ProviderWire != nil {
		observation := d.ProviderWire.Clone()
		d.ProviderWire = &observation
	}
	if d.ProviderCause != nil {
		observation := d.ProviderCause.Clone()
		d.ProviderCause = &observation
	}
	if d.Payload != nil {
		d.Payload = append([]byte{}, d.Payload...)
	}
	return d
}

func (d DefinitionAuthoringDiagnostic) Validate() error {
	if d.ProviderCode != "" || d.ProviderFailureClass != "" || d.ProviderDocument != nil || d.ProviderWire != nil || d.ProviderCause != nil {
		if len(d.ProviderCode) > 128 || d.ProviderDocument == nil || d.ProviderWire == nil || d.ProviderDocument.Validate() != nil || d.ProviderWire.Validate() != nil || d.ProviderCause != nil && (d.ProviderCode == "" || d.ProviderCause.Validate() != nil || !d.ProviderCause.Observed || !d.ProviderCause.Complete) {
			return fmt.Errorf("authoring provider diagnostic invalid")
		}
		if d.ProviderFailureClass != "" && (d.ProviderCode == "" || !validAuthoringProviderFailureClass(d.ProviderFailureClass)) {
			return fmt.Errorf("authoring provider diagnostic invalid")
		}
		if d.ProviderCode == "" && (!d.ProviderDocument.Observed || !d.ProviderDocument.Complete || !d.ProviderDocument.Saved) {
			return fmt.Errorf("parser diagnostic requires a complete saved provider document")
		}
	}
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
	if d.ProviderCode == "" && d.ProviderDocument != nil && (d.ProviderDocument.ObservedBytes != d.PayloadBytes || d.ProviderDocument.SHA256 != d.PayloadSHA256) {
		return fmt.Errorf("parser diagnostic document differs from original payload")
	}
	return nil
}

// A specification request records a single possible physical call. Neither a
// reserved request nor a completed review grants admission.
type DefinitionAuthoringSpecificationRequest struct {
	PreviousProposalHash string                               `json:",omitempty"`
	FeedbackHash         string                               `json:",omitempty"`
	WireContractRef      string                               `json:",omitempty"`
	WireContractHash     string                               `json:",omitempty"`
	ReviewAudit          *DefinitionAuthoringSuccessByteAudit `json:",omitempty"`
	RequestID            string
	DerivationPermitID   string
	Phase                DefinitionSpecificationPhase
	ReviewKind           string
	ProposalHash         string
	ProviderBindingHash  string
	PermitID             string
	Proposal             *GoalSpecification
	Mapping              *DefinitionAuthoringMapping
	SuccessAudit         *DefinitionAuthoringSuccessByteAudit
	CompletionIdentity   string
	Review               *GoalSpecificationCompletenessReview
	FailureCode          string
	Diagnostic           *DefinitionAuthoringDiagnostic
}

func (r DefinitionAuthoringSpecificationRequest) Clone() DefinitionAuthoringSpecificationRequest {
	if r.ReviewAudit != nil {
		a := r.ReviewAudit.Clone()
		r.ReviewAudit = &a
	}
	if r.Mapping != nil {
		m := r.Mapping.Clone()
		r.Mapping = &m
	}
	if r.SuccessAudit != nil {
		a := r.SuccessAudit.Clone()
		r.SuccessAudit = &a
	}
	if r.Proposal != nil {
		proposal := r.Proposal.Clone()
		r.Proposal = &proposal
	}
	if r.Review != nil {
		review := *r.Review
		review.Issues = cloneReviewIssues(review.Issues)
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

	writeDefinitionHash(h, r.WireContractRef, r.WireContractHash)
	if r.PreviousProposalHash != "" || r.FeedbackHash != "" {
		writeDefinitionHash(h, r.PreviousProposalHash, r.FeedbackHash)
	}

	return "authoring-spec-permit-sha256:" + hex.EncodeToString(h.Sum(nil))
}

func (r DefinitionAuthoringSpecificationRequest) Validate() error { return r.validate(false) }

// ValidateView checks a detached metadata/canonical observation, not raw identity.
func (r DefinitionAuthoringSpecificationRequest) ValidateView() error { return r.validate(true) }
func (r DefinitionAuthoringSpecificationRequest) validate(view bool) error {
	phase := "proposal"
	if r.ReviewKind != "" {
		phase = r.ReviewKind
	}
	if err := validateAuthoringWire(r.WireContractRef, r.WireContractHash, phase); err != nil {
		return err
	}
	if r.Review != nil {
		if r.ReviewAudit == nil {
			return fmt.Errorf("owner-bound review lacks original document")
		}
		if view {
			if err := r.ReviewAudit.ValidateMetadata(); err != nil {
				return err
			}
		} else {
			if err := r.ReviewAudit.Validate(); err != nil {
				return err
			}
			v, err := DecodeOwnerBoundReview(r, r.ReviewAudit.Document.Content)
			if err != nil || !reflect.DeepEqual(v, *r.Review) {
				return fmt.Errorf("review differs from original semantic document")
			}
		}
	} else if r.ReviewAudit != nil {
		return fmt.Errorf("unexpected authoring review audit")
	}

	if r.Proposal != nil {
		if r.Mapping == nil || r.SuccessAudit == nil {
			return fmt.Errorf("authoring proposal lacks current mapping/success audit")
		}
		if view {
			if r.SuccessAudit.ValidateMetadata() != nil {
				return fmt.Errorf("authoring success audit metadata invalid")
			}
		} else if r.SuccessAudit.Validate() != nil {
			return fmt.Errorf("authoring success byte audit invalid")
		}
	} else if r.Mapping != nil || r.SuccessAudit != nil {
		return fmt.Errorf("authoring non-proposal contains mapping/success audit")
	}
	if r.Diagnostic != nil {
		if view {
			if r.Diagnostic.ValidateMetadata() != nil {
				return fmt.Errorf("authoring failure metadata invalid")
			}
		} else if r.Diagnostic.Validate() != nil {
			return fmt.Errorf("authoring failure diagnostic invalid")
		}
	}
	if r.RequestID == "" || len(r.RequestID) > 128 || r.DerivationPermitID == "" || len(r.DerivationPermitID) > 256 || r.ProviderBindingHash == "" || len(r.ProviderBindingHash) > 256 || r.PermitID != r.ExpectedPermitID() {
		return fmt.Errorf("authoring specification request identity invalid")
	}
	switch r.Phase {
	case DefinitionSpecificationProposal:
		if r.ReviewKind != "" || r.ProposalHash != "" || (r.PreviousProposalHash == "") != (r.FeedbackHash == "") || len(r.PreviousProposalHash) > 256 || len(r.FeedbackHash) > 256 {
			return fmt.Errorf("authoring proposal cannot claim a review or proposal hash")
		}
	case DefinitionSpecificationCritic:
		if r.PreviousProposalHash != "" || r.FeedbackHash != "" || !GoalSpecificationReviewKind(r.ReviewKind) || r.ProposalHash == "" || len(r.ProposalHash) > 256 {
			return fmt.Errorf("authoring critic requires exact proposal identity")
		}
	default:
		return fmt.Errorf("authoring specification phase invalid")
	}
	if r.CompletionIdentity != "" && (r.Phase != DefinitionSpecificationProposal || r.Proposal == nil && (r.FailureCode != "invalid_proposal" || r.Diagnostic == nil)) {
		return fmt.Errorf("mapping completion identity outcome mismatch")
	}
	if r.Proposal != nil && r.CompletionIdentity != MappingCompletionIdentity(r.PermitID, *r.Mapping, *r.SuccessAudit) {
		return fmt.Errorf("mapping completion identity mismatch")
	}
	if r.FailureCode != "" {
		if r.Diagnostic != nil && (r.FailureCode == "provider_failed" && r.Diagnostic.ProviderCode == "") {
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
	return validateAuthoringSpecificationRequestsMode(record, false)
}
func validateAuthoringSpecificationRequestsMode(record DefinitionAuthoringRecord, view bool) error {
	if len(record.SpecificationRequests) > DefinitionAuthoringSpecificationRequestLimit {
		return fmt.Errorf("authoring specification request budget exceeded")
	}
	seen := make(map[string]bool, len(record.SpecificationRequests))
	attempts := map[string]int{}
	lastInSlot := map[string]DefinitionAuthoringSpecificationRequest{}
	distinctProposals := map[string]int{}
	type completedProposal struct {
		reviews goalSpecificationReviewContext
	}
	completedProposals := map[string]completedProposal{}
	contexts := map[string]authoringSpecificationContext{}
	for index, request := range record.SpecificationRequests {
		if request.Phase == DefinitionSpecificationProposal {
			prefix := record
			prefix.SpecificationRequests = record.SpecificationRequests[:index]
			if err := prefix.validateSpecificationSuccessor(request); err != nil {
				return err
			}
		}
		if err := request.validate(view); err != nil {
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
		if request.WireContractRef != derivation.WireContractRef {
			return fmt.Errorf("authoring wire differs from derivation")
		}
		if request.ProviderBindingHash != derivation.ProviderBindingHash {
			return fmt.Errorf("authoring specification provider binding differs from derivation")
		}
		if request.CompletionIdentity != "" && request.FailureCode != "" && !view {
			if _, _, err := ReparseMappingFailure(request, derivation); err != nil {
				return err
			}
		}
		slotKey := request.DerivationPermitID + "\x00" + string(request.Phase) + "\x00" + request.ReviewKind + "\x00" + request.ProposalHash + "\x00" + request.PreviousProposalHash + "\x00" + request.FeedbackHash
		if prev, exists := lastInSlot[slotKey]; exists {
			if !prev.IsRetryableProviderTransientFailure() {
				return fmt.Errorf("authoring specification phase attempt budget exceeded")
			}
		} else if request.Phase == DefinitionSpecificationProposal {
			distinctProposals[request.DerivationPermitID]++
			if distinctProposals[request.DerivationPermitID] > DefinitionAuthoringSpecificationAttemptLimit {
				return fmt.Errorf("authoring specification phase attempt budget exceeded")
			}
		}
		attempts[slotKey]++
		if attempts[slotKey] > 1+AuthoringProviderTransientRetryLimit {
			return fmt.Errorf("authoring specification phase attempt budget exceeded")
		}
		lastInSlot[slotKey] = request
		if request.Phase == DefinitionSpecificationProposal {
			if request.Proposal != nil {
				if !view {
					if _, err := RecompileDefinitionAuthoringProposal(derivation, request); err != nil {
						return err
					}
				} else {
					compiled, err := CompileDefinitionAuthoringMapping(derivation, *request.Mapping)
					if err != nil || !reflect.DeepEqual(compiled, *request.Proposal) {
						return fmt.Errorf("authoring canonical view differs from sealed mapping")
					}
				}
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
				completedProposals[request.DerivationPermitID+"\x00"+hash] = completedProposal{reviews: authoringProposalReviewContext(context, *request.Proposal, hash)}
			}
			continue
		}
		proposal, present := completedProposals[request.DerivationPermitID+"\x00"+request.ProposalHash]
		if !present {
			return fmt.Errorf("authoring critic lacks preceding completed proposal")
		}
		if request.Review != nil {
			if err := proposal.reviews.validate(*request.Review); err != nil {
				return err
			}
		}
	}
	return nil
}

func validAuthoringProviderFailureClass(class string) bool {
	switch runtimecontract.RuntimeCandidateFailureClass(class) {
	case runtimecontract.RuntimeCandidateFailureClassProviderTransient,
		runtimecontract.RuntimeCandidateFailureClassProviderProtocol,
		runtimecontract.RuntimeCandidateFailureClassProviderAuthentication,
		runtimecontract.RuntimeCandidateFailureClassProviderConfiguration,
		runtimecontract.RuntimeCandidateFailureClassProvider:
		return true
	default:
		return false
	}
}

// AuthoringRetryRequestID derives the deterministic request identity for a bounded
// retry attempt without changing the base transaction identity.
func AuthoringRetryRequestID(base string, retryOrdinal int) string {
	if retryOrdinal <= 0 {
		return base
	}
	return fmt.Sprintf("%s/retry/%d", base, retryOrdinal)
}

func (d *DefinitionAuthoringDiagnostic) IsRetryableProviderTransient() bool {
	return d != nil &&
		d.ProviderFailureClass == string(runtimecontract.RuntimeCandidateFailureClassProviderTransient) &&
		d.ProviderCode != "" &&
		d.ProviderCode != "request_canceled" &&
		d.ProviderCode != "tls_verification_failed"
}

func (r DefinitionDerivationRequest) IsRetryableProviderTransientFailure() bool {
	return r.FailureCode == "provider_failed" && r.Proposal == nil && r.Diagnostic.IsRetryableProviderTransient()
}

func (r DefinitionAuthoringSpecificationRequest) IsRetryableProviderTransientFailure() bool {
	return r.FailureCode == "provider_failed" && r.Proposal == nil && r.Review == nil && r.Diagnostic.IsRetryableProviderTransient()
}

func (r DefinitionAuthoringRecord) CompletedSpecificationProposal(derivationPermitID, hash string) *GoalSpecification {
	return r.completedSpecificationProposal(derivationPermitID, hash)
}

func (r DefinitionAuthoringRecord) SpecificationSlotRequests(derivationPermitID string, phase DefinitionSpecificationPhase, reviewKind, proposalHash, previousProposalHash, feedbackHash string) []DefinitionAuthoringSpecificationRequest {
	var slot []DefinitionAuthoringSpecificationRequest
	for _, request := range r.SpecificationRequests {
		if request.DerivationPermitID != derivationPermitID || request.Phase != phase {
			continue
		}
		if phase == DefinitionSpecificationProposal {
			if request.PreviousProposalHash != previousProposalHash || request.FeedbackHash != feedbackHash {
				continue
			}
		} else {
			if request.ReviewKind != reviewKind || request.ProposalHash != proposalHash {
				continue
			}
		}
		slot = append(slot, request.Clone())
	}
	return slot
}

func (r DefinitionAuthoringRecord) CanRetrySpecificationSlot(slot []DefinitionAuthoringSpecificationRequest) (int, bool) {
	if len(slot) == 0 || len(slot) > AuthoringProviderTransientRetryLimit || len(r.SpecificationRequests) >= DefinitionAuthoringSpecificationRequestLimit {
		return 0, false
	}
	for _, entry := range slot {
		if !entry.IsRetryableProviderTransientFailure() {
			return 0, false
		}
	}
	return len(slot), true
}

func (r DefinitionAuthoringRecord) SettledCriticReview(derivationPermitID, proposalHash, kind string) (*GoalSpecificationCompletenessReview, *DefinitionAuthoringSpecificationRequest, bool, bool, error) {
	slot := r.SpecificationSlotRequests(derivationPermitID, DefinitionSpecificationCritic, kind, proposalHash, "", "")
	if len(slot) == 0 {
		return nil, nil, false, false, nil
	}
	if len(slot) > 1+AuthoringProviderTransientRetryLimit {
		return nil, nil, false, false, fmt.Errorf("authoring critic %s retry budget exceeded", kind)
	}
	for i, entry := range slot {
		isLast := i == len(slot)-1
		if !isLast {
			if !entry.IsRetryableProviderTransientFailure() {
				return nil, nil, false, false, fmt.Errorf("authoring critic %s has superseded non-transient entry", kind)
			}
			continue
		}
		if entry.Review != nil && entry.FailureCode == "" {
			review := *entry.Clone().Review
			return &review, nil, false, false, nil
		}
		if entry.FailureCode != "" {
			failed := entry.Clone()
			_, retryable := r.CanRetrySpecificationSlot(slot)
			return nil, &failed, false, retryable, nil
		}
		return nil, nil, true, false, nil
	}
	return nil, nil, false, false, nil
}

func (r DefinitionAuthoringRecord) SettledProposalState(derivationPermitID string) (*GoalSpecification, *DefinitionAuthoringSpecificationRequest, bool, bool, error) {
	type proposalSlotGroup struct {
		previousHash string
		feedbackHash string
		requests     []DefinitionAuthoringSpecificationRequest
	}
	var groups []proposalSlotGroup
	seenSlot := map[string]bool{}
	for _, request := range r.SpecificationRequests {
		if request.DerivationPermitID != derivationPermitID || request.Phase != DefinitionSpecificationProposal {
			continue
		}
		slotKey := request.PreviousProposalHash + "\x00" + request.FeedbackHash
		if len(groups) == 0 || groups[len(groups)-1].previousHash != request.PreviousProposalHash || groups[len(groups)-1].feedbackHash != request.FeedbackHash {
			if seenSlot[slotKey] {
				return nil, nil, false, false, fmt.Errorf("authoring proposal slot interleaved")
			}
			seenSlot[slotKey] = true
			if len(groups) >= DefinitionAuthoringSpecificationAttemptLimit {
				return nil, nil, false, false, fmt.Errorf("authoring proposal generation budget exceeded")
			}
			groups = append(groups, proposalSlotGroup{previousHash: request.PreviousProposalHash, feedbackHash: request.FeedbackHash})
		}
		idx := len(groups) - 1
		if len(groups[idx].requests) >= 1+AuthoringProviderTransientRetryLimit {
			return nil, nil, false, false, fmt.Errorf("authoring proposal transient retry budget exceeded")
		}
		groups[idx].requests = append(groups[idx].requests, request.Clone())
	}
	if len(groups) == 0 {
		return nil, nil, false, false, nil
	}
	var latestProposal *GoalSpecification
	for gIdx, group := range groups {
		isLastGroup := gIdx == len(groups)-1
		for i, entry := range group.requests {
			isLast := i == len(group.requests)-1
			if !isLast {
				if !entry.IsRetryableProviderTransientFailure() {
					return nil, nil, false, false, fmt.Errorf("authoring proposal has superseded non-transient entry")
				}
				continue
			}
			if entry.Proposal != nil && entry.FailureCode == "" {
				p := entry.Proposal.Clone()
				latestProposal = &p
				if !isLastGroup {
					continue
				}
				return latestProposal, nil, false, false, nil
			}
			if !isLastGroup {
				return nil, nil, false, false, fmt.Errorf("authoring proposal revision lacks completed predecessor")
			}
			if entry.FailureCode != "" {
				failed := entry.Clone()
				_, retryable := r.CanRetrySpecificationSlot(group.requests)
				return latestProposal, &failed, false, retryable, nil
			}
			return latestProposal, nil, true, false, nil
		}
	}
	return latestProposal, nil, false, false, nil
}

// 仅由本次 ledger 中已通过 context.validate 的提案构建，不能替代 body 校验。
func authoringProposalReviewContext(c authoringSpecificationContext, proposal GoalSpecification, proposalHash string) goalSpecificationReviewContext {
	evidence := map[string]bool{c.goal.CandidateItemRef: true}
	for _, item := range c.items {
		evidence[item.CandidateItemRef] = true
	}
	for _, resource := range proposal.Resources {
		evidence[strings.TrimSpace(resource.ID)] = true
	}
	for _, dimension := range proposal.Dimensions {
		evidence[strings.TrimSpace(dimension.ID)] = true
	}
	requirements := make(map[string]GoalSpecificationRequirement, len(proposal.Requirements))
	for _, requirement := range proposal.Requirements {
		id := strings.TrimSpace(requirement.ID)
		evidence[id] = true
		requirements[id] = requirement
	}
	for _, obligation := range proposal.Obligations {
		evidence[strings.TrimSpace(obligation.ID)] = true
	}
	for _, verification := range proposal.Verifications {
		evidence[strings.TrimSpace(verification.ID)] = true
	}
	return goalSpecificationReviewContext{contentHash: proposalHash, evidence: evidence, requirements: requirements}
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
	if err := r.ReadCommittedRecord(); err != nil {
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
	if request.WireContractRef != derivation.WireContractRef {
		return DefinitionAuthoringRecord{}, request, false, fmt.Errorf("authoring wire differs from derivation")
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
		return DefinitionAuthoringRecord{}, DefinitionAuthoringSpecificationRequest{}, false, &DefinitionAuthoringBudgetRefusalError{Stage: preparationStageForSpecification(request.Phase), RequestID: request.RequestID, Budget: "request", Limit: DefinitionAuthoringSpecificationRequestLimit, Cause: fmt.Errorf("authoring specification request budget exhausted")}
	}
	slot := r.SpecificationSlotRequests(request.DerivationPermitID, request.Phase, request.ReviewKind, request.ProposalHash, request.PreviousProposalHash, request.FeedbackHash)
	if len(slot) > 0 {
		if _, retryable := r.CanRetrySpecificationSlot(slot); !retryable {
			limit := uint32(1)
			if request.Phase == DefinitionSpecificationProposal || slot[0].IsRetryableProviderTransientFailure() {
				limit = uint32(1 + AuthoringProviderTransientRetryLimit)
			}
			return DefinitionAuthoringRecord{}, DefinitionAuthoringSpecificationRequest{}, false, &DefinitionAuthoringBudgetRefusalError{Stage: preparationStageForSpecification(request.Phase), RequestID: request.RequestID, Budget: "attempt", Limit: limit, Cause: fmt.Errorf("authoring specification phase attempt budget exhausted")}
		}
	}
	if request.Phase == DefinitionSpecificationProposal {
		if err := r.validateSpecificationSuccessor(request); err != nil {
			return DefinitionAuthoringRecord{}, request, false, err
		}
	}
	if request.Phase == DefinitionSpecificationCritic && (r.completedSpecificationProposal(request.DerivationPermitID, request.ProposalHash) == nil || r.LatestSpecificationRequest(request.DerivationPermitID).Proposal == nil || r.LatestSpecificationRequest(request.DerivationPermitID).Proposal.ContentHash() != request.ProposalHash) {
		return DefinitionAuthoringRecord{}, request, false, fmt.Errorf("authoring critic requires completed proposal")
	}
	next := r.Clone()
	next.SpecificationRequests = append(next.SpecificationRequests, request.Clone())
	return next, request, true, nil
}

func (r DefinitionAuthoringRecord) CompleteSpecification(permitID string, proposal *GoalSpecification, review *GoalSpecificationCompletenessReview, failureCode string, diagnostics ...*DefinitionAuthoringDiagnostic) (DefinitionAuthoringRecord, error) {
	return r.completeSpecification(permitID, proposal, review, failureCode, diagnostics, nil, nil, "")
}
func (r DefinitionAuthoringRecord) CompleteSpecificationMapping(permitID string, mapping DefinitionAuthoringMapping, audit DefinitionAuthoringSuccessByteAudit) (DefinitionAuthoringRecord, error) {
	if err := r.ReadCommittedRecord(); err != nil {
		return DefinitionAuthoringRecord{}, err
	}
	// An exact duplicate consumes the committed outcome before any compiler runs.
	for _, prior := range r.SpecificationRequests {
		if prior.PermitID == permitID && (prior.Proposal != nil || prior.FailureCode != "" || prior.Review != nil) {
			if audit.Validate() != nil || prior.CompletionIdentity != MappingCompletionIdentity(permitID, mapping, audit) {
				return DefinitionAuthoringRecord{}, fmt.Errorf("authoring mapping completion conflict")
			}
			if prior.Proposal != nil && (prior.Mapping == nil || prior.SuccessAudit == nil || !reflect.DeepEqual(*prior.Mapping, mapping) || !reflect.DeepEqual(*prior.SuccessAudit, audit)) {
				return DefinitionAuthoringRecord{}, fmt.Errorf("authoring mapping original differs")
			}
			if prior.FailureCode != "" && (prior.Diagnostic == nil || prior.Diagnostic.ProviderDocument == nil || prior.Diagnostic.ProviderWire == nil || !reflect.DeepEqual(*prior.Diagnostic.ProviderDocument, audit.Document) || !reflect.DeepEqual(*prior.Diagnostic.ProviderWire, audit.Wire)) {
				return DefinitionAuthoringRecord{}, fmt.Errorf("authoring mapping failure source differs")
			}
			return r.Clone(), nil
		}
	}
	if audit.Validate() != nil {
		return DefinitionAuthoringRecord{}, fmt.Errorf("authoring mapping completion audit invalid")
	}
	var err error
	var derivation DefinitionDerivationRequest
	for _, request := range r.SpecificationRequests {
		if request.PermitID == permitID {
			derivation, err = r.derivationForSpecification(request.DerivationPermitID)
			break
		}
	}
	if err != nil || derivation.Proposal == nil {
		return DefinitionAuthoringRecord{}, fmt.Errorf("authoring mapping completion derivation missing")
	}
	decoded, err := DecodeAuthoringMappingFor(derivation, audit.Document.Content)
	if err != nil || !reflect.DeepEqual(decoded, mapping) {
		return DefinitionAuthoringRecord{}, fmt.Errorf("authoring mapping completion raw mismatch")
	}
	proposal, err := CompileDefinitionAuthoringMapping(derivation, mapping)
	if err == nil {
		err = r.ValidateSpecificationRevisionCandidate(permitID, mapping, proposal)
	}
	if err != nil {
		return DefinitionAuthoringRecord{}, &DefinitionAuthoringMappingRejection{Cause: err}
	}
	return r.completeSpecification(permitID, &proposal, nil, "", nil, &mapping, &audit, MappingCompletionIdentity(permitID, mapping, audit))
}
func (r DefinitionAuthoringRecord) completeSpecification(permitID string, proposal *GoalSpecification, review *GoalSpecificationCompletenessReview, failureCode string, diagnostics []*DefinitionAuthoringDiagnostic, mapping *DefinitionAuthoringMapping, audit *DefinitionAuthoringSuccessByteAudit, completionIdentity string) (DefinitionAuthoringRecord, error) {
	if err := r.ReadCommittedRecord(); err != nil {
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
		completed.Mapping, completed.SuccessAudit, completed.CompletionIdentity = mapping, audit, completionIdentity
		if review != nil {
			completed.ReviewAudit = audit
			completed.SuccessAudit = nil
		}
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
		if previous.Proposal != nil || previous.Review != nil || previous.FailureCode != "" {
			if !reflect.DeepEqual(previous, completed) {
				return DefinitionAuthoringRecord{}, fmt.Errorf("authoring specification completion conflict")
			}
			return r.Clone(), nil
		}
		if err := completed.Validate(); err != nil {
			return DefinitionAuthoringRecord{}, err
		}
		next := r.Clone()
		next.SpecificationRequests[n] = completed
		// Only the newly accepted outcome needs semantic admission. Earlier
		// outcomes remain their owner's committed results.
		derivation, err := r.derivationForSpecification(completed.DerivationPermitID)
		if err != nil {
			return DefinitionAuthoringRecord{}, err
		}
		if completed.Review != nil {
			original := r.completedSpecificationProposal(completed.DerivationPermitID, completed.ProposalHash)
			if original == nil {
				return DefinitionAuthoringRecord{}, fmt.Errorf("review lacks original committed proposal")
			}
			context, err := newAuthoringSpecificationContext(derivation.Input, &derivation.Template, *derivation.Proposal, derivation.Resources)
			if err != nil {
				return DefinitionAuthoringRecord{}, err
			}
			if err := authoringProposalReviewContext(context, *original, completed.ProposalHash).validate(*completed.Review); err != nil {
				return DefinitionAuthoringRecord{}, err
			}
		}
		if err := next.ReadCommittedRecord(); err != nil {
			return DefinitionAuthoringRecord{}, err
		}
		return next, nil
	}
	return DefinitionAuthoringRecord{}, fmt.Errorf("authoring specification permit not found")
}

func (r DefinitionAuthoringRecord) CompleteReviewDocument(permit string, review GoalSpecificationCompletenessReview, audit DefinitionAuthoringSuccessByteAudit) (DefinitionAuthoringRecord, error) {
	return r.completeSpecification(permit, nil, &review, "", nil, nil, &audit, "")
}

// DefinitionAuthoringMappingRejection is a known semantic outcome of the sole
// mapping admission, distinct from missing/corrupt source refusal.
type DefinitionAuthoringMappingRejection struct{ Cause error }

func (e *DefinitionAuthoringMappingRejection) Error() string { return e.Cause.Error() }
func (e *DefinitionAuthoringMappingRejection) Unwrap() error { return e.Cause }
