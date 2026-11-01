package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"unicode/utf8"
)

// Project design is a proposal under an existing obligation, never another
// acceptance item or a claim that implementation/validation already happened.
type GoalSpecificationDesignDecision struct {
	CandidateItemRef  string
	Decision          string
	VerificationBasis string
}

const (
	AuthoringIssueCoverage      = "coverage_gap"
	AuthoringIssueDesign        = "design_gap"
	AuthoringIssueVerification  = "verification_gap"
	AuthoringIssueSource        = "source_conflict"
	AuthoringIssueClarification = "user_clarification"
	AuthoringIssueCapability    = "capability_unavailable"
)

type GoalSpecificationReviewIssue struct {
	Class          string
	Detail         string
	RequirementIDs []string
	EvidenceRefs   []string
}

func cloneReviewIssues(in []GoalSpecificationReviewIssue) []GoalSpecificationReviewIssue {
	out := slices.Clone(in)
	for i := range out {
		out[i].RequirementIDs = slices.Clone(out[i].RequirementIDs)
		out[i].EvidenceRefs = slices.Clone(out[i].EvidenceRefs)
	}
	return out
}

func (c goalSpecificationReviewContext) validateIssues(r GoalSpecificationCompletenessReview) error {
	if len(r.Issues) > 32 || r.Status == GoalReviewPass && len(r.Issues) != 0 {
		return fmt.Errorf("authoring review issue cardinality/status mismatch")
	}
	for _, issue := range r.Issues {
		switch issue.Class {
		case AuthoringIssueCoverage, AuthoringIssueDesign, AuthoringIssueVerification, AuthoringIssueSource, AuthoringIssueClarification, AuthoringIssueCapability:
		default:
			return fmt.Errorf("authoring review issue class invalid")
		}
		if strings.TrimSpace(issue.Detail) == "" || len(issue.Detail) > 4096 || !utf8.ValidString(issue.Detail) || strings.ContainsRune(issue.Detail, 0) || len(issue.RequirementIDs) == 0 || len(issue.EvidenceRefs) == 0 ||
			!specRefsExist(normalizedSpecRefs(issue.RequirementIDs), func(ref string) bool {
				_, ok := c.requirements[ref]
				return ok && slices.Contains(r.RequirementIDs, ref)
			}) ||
			!specRefsExist(normalizedSpecRefs(issue.EvidenceRefs), func(ref string) bool { return c.evidence[ref] && slices.Contains(r.EvidenceRefs, ref) }) {
			return fmt.Errorf("authoring review issue lacks exact sources")
		}
	}
	return nil
}

type DefinitionAuthoringIssueResolution struct {
	IssueRef string
	Change   string
}

type DefinitionAuthoringFeedback struct {
	PreviousProposalHash string
	Reviews              []GoalSpecificationCompletenessReview
}

func (f DefinitionAuthoringFeedback) ContentHash() string {
	raw, _ := json.Marshal(f)
	h := sha256.Sum256(raw)
	return "authoring-feedback-sha256:" + hex.EncodeToString(h[:])
}

// Feedback is owner-derived from all six settled, independent original reviews.
// It grants one bounded corrective proposal, never sampling or execution credit.
func (r DefinitionAuthoringRecord) SpecificationFeedback(derivation, hash string) (DefinitionAuthoringFeedback, error) {
	f := DefinitionAuthoringFeedback{PreviousProposalHash: hash}
	if r.completedSpecificationProposal(derivation, hash) == nil {
		return f, fmt.Errorf("feedback proposal not found")
	}
	blocked := false
	for _, kind := range goalReviewKinds {
		var review *GoalSpecificationCompletenessReview
		for _, request := range r.SpecificationRequests {
			if request.DerivationPermitID != derivation || request.ProposalHash != hash || request.Phase != DefinitionSpecificationCritic || request.ReviewKind != kind {
				continue
			}
			if review != nil || request.Review == nil || request.FailureCode != "" {
				return f, fmt.Errorf("feedback reviews unresolved or duplicated")
			}
			v := request.Clone().Review
			review = v
		}
		if review == nil {
			return f, fmt.Errorf("feedback requires all six reviews")
		}
		if r.completedSpecificationProposal(derivation, hash).ValidateReviewCoverage(*review) != nil {
			return f, fmt.Errorf("feedback review omitted required scope")
		}
		if review.Status != GoalReviewPass {
			blocked = true
			if len(review.Issues) == 0 {
				return f, fmt.Errorf("feedback requires classified issues")
			}
			for _, issue := range review.Issues {
				switch issue.Class {
				case AuthoringIssueCoverage, AuthoringIssueDesign, AuthoringIssueVerification, AuthoringIssueSource:
				default:
					return f, fmt.Errorf("feedback requires user clarification or unavailable capability")
				}
			}
		}
		f.Reviews = append(f.Reviews, *review)
	}
	if !blocked {
		return f, fmt.Errorf("passing candidate cannot be resampled")
	}
	return f, nil
}

func (r DefinitionAuthoringRecord) LatestSpecificationRequest(derivation string) *DefinitionAuthoringSpecificationRequest {
	var latest *DefinitionAuthoringSpecificationRequest
	for _, q := range r.SpecificationRequests {
		if q.DerivationPermitID == derivation && q.Phase == DefinitionSpecificationProposal {
			v := q.Clone()
			latest = &v
		}
	}
	return latest
}

func (r DefinitionAuthoringRecord) validateSpecificationSuccessor(q DefinitionAuthoringSpecificationRequest) error {
	previous := r.LatestSpecificationRequest(q.DerivationPermitID)
	if previous == nil {
		if q.Mapping != nil && len(q.Mapping.IssueResolutions) != 0 {
			return fmt.Errorf("first proposal cannot claim issue resolutions")
		}
		if q.PreviousProposalHash != "" || q.FeedbackHash != "" {
			return fmt.Errorf("first authoring proposal cannot claim feedback")
		}
		return nil
	}
	if previous.Proposal == nil || previous.PreviousProposalHash != "" || previous.Proposal.ContentHash() != q.PreviousProposalHash || q.FeedbackHash == "" {
		return fmt.Errorf("authoring revision requires the latest completed first proposal")
	}
	f, err := r.SpecificationFeedback(q.DerivationPermitID, q.PreviousProposalHash)
	if err != nil {
		return err
	}
	if f.ContentHash() != q.FeedbackHash {
		return fmt.Errorf("authoring feedback identity mismatch")
	}
	if q.Proposal != nil {
		if q.Mapping == nil {
			return fmt.Errorf("revision lacks mapping")
		}
		if err := validateRevisionChanges(f, *q.Mapping, *previous.Proposal, *q.Proposal); err != nil {
			return err
		}
	}
	if q.Proposal != nil && q.Proposal.ContentHash() == q.PreviousProposalHash {
		return fmt.Errorf("authoring revision did not change the rejected candidate")
	}
	return nil
}

// ValidateSpecificationRevisionCandidate evaluates the proposed result against
// the immutable ledger prefix, before the owner settles success or known failure.
func (r DefinitionAuthoringRecord) ValidateSpecificationRevisionCandidate(permit string, mapping DefinitionAuthoringMapping, proposal GoalSpecification) error {
	for i, request := range r.SpecificationRequests {
		if request.PermitID != permit {
			continue
		}
		if request.Phase != DefinitionSpecificationProposal {
			return fmt.Errorf("revision result is not a proposal")
		}
		prefix := r
		prefix.SpecificationRequests = r.SpecificationRequests[:i]
		request.Mapping, request.Proposal = &mapping, &proposal
		return prefix.validateSpecificationSuccessor(request)
	}
	return fmt.Errorf("revision permit not found")
}

// SpecificationRequestID is shared by request preparation and diagnostic scope.
// It is an idempotency identity, not a grant of provider or execution authority.
func SpecificationRequestID(derivation, kind, proposal, feedback string) string {
	if kind != "" {
		suffix := derivation
		if proposal != "" {
			suffix = strings.TrimPrefix(proposal, "goal-specification-sha256:")
		}
		return "review/" + kind + "/" + suffix
	}
	if feedback != "" {
		return "revise/" + strings.TrimPrefix(feedback, "authoring-feedback-sha256:")
	}
	return "proposal/" + derivation
}

func validateDesignDecisions(decisions []GoalSpecificationDesignDecision, items []DefinitionCandidateItem) error {
	if len(decisions) > len(items) {
		return fmt.Errorf("project design exceeds candidate scope")
	}
	seen := map[string]bool{}
	for _, d := range decisions {
		exists := false
		for _, item := range items {
			if item.CandidateItemRef == d.CandidateItemRef {
				exists = true
				break
			}
		}
		if !exists || seen[d.CandidateItemRef] || strings.TrimSpace(d.Decision) == "" || strings.TrimSpace(d.VerificationBasis) == "" || len(d.Decision) > 4096 || len(d.VerificationBasis) > 4096 || !utf8.ValidString(d.Decision) || !utf8.ValidString(d.VerificationBasis) || strings.ContainsRune(d.Decision, 0) || strings.ContainsRune(d.VerificationBasis, 0) {
			return fmt.Errorf("project design lacks unique original obligation or concrete decision/basis")
		}
		seen[d.CandidateItemRef] = true
	}
	return nil
}

// Issue refs are positional identities in the immutable, fixed-kind feedback,
// not model-authored identifiers or execution evidence.
func (f DefinitionAuthoringFeedback) IssueRefs() []string {
	refs := []string{}
	for _, review := range f.Reviews {
		for i := range review.Issues {
			refs = append(refs, fmt.Sprintf("%s/issue/%06d", review.Kind, i+1))
		}
	}
	return refs
}
func validateRevisionChanges(f DefinitionAuthoringFeedback, m DefinitionAuthoringMapping, previous, next GoalSpecification) error {
	refs := f.IssueRefs()
	if len(m.IssueResolutions) != len(refs) {
		return fmt.Errorf("revision must respond to every original issue")
	}
	seen := map[string]bool{}
	for _, resolution := range m.IssueResolutions {
		if !slices.Contains(refs, resolution.IssueRef) || seen[resolution.IssueRef] || strings.TrimSpace(resolution.Change) == "" || len(resolution.Change) > 4096 || !utf8.ValidString(resolution.Change) || strings.ContainsRune(resolution.Change, 0) {
			return fmt.Errorf("revision issue response invalid")
		}
		seen[resolution.IssueRef] = true
	}
	// A wording-only requirement regrouping cannot authorize six new votes.
	if reflect.DeepEqual(previous.Verifications, next.Verifications) && reflect.DeepEqual(previous.DesignDecisions, next.DesignDecisions) && reflect.DeepEqual(previous.ExecutionProofMappings, next.ExecutionProofMappings) && reflect.DeepEqual(previous.EvidencePurposeMappings, next.EvidencePurposeMappings) {
		return fmt.Errorf("revision has no corrective semantic change")
	}
	return nil
}
