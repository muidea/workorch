package workorchprojectioncontract

import "github.com/muidea/workorch/internal/pkg/workorcheventpayload"

// Decision is the complete owner-neutral projection of a decision ticket.
type Decision struct {
	ID                      string                       `json:"id"`
	WorkspaceID             string                       `json:"workspaceId"`
	RunID                   string                       `json:"runId"`
	StepID                  string                       `json:"stepId,omitempty"`
	Reason                  string                       `json:"reason"`
	DecisionKind            string                       `json:"decisionKind,omitempty"`
	Question                string                       `json:"question,omitempty"`
	Context                 workorcheventpayload.Payload `json:"context,omitempty"`
	FailureClass            string                       `json:"failureClass,omitempty"`
	FailureSignature        string                       `json:"failureSignature,omitempty"`
	AllowedRecoveryActions  []string                     `json:"allowedRecoveryActions,omitempty"`
	DefinitionClarification *DefinitionClarification     `json:"definitionClarification,omitempty"`
	State                   string                       `json:"state"`
	ResolutionID            string                       `json:"resolutionId,omitempty"`
	ResolutionType          string                       `json:"resolutionType,omitempty"`
	ResolutionPayload       workorcheventpayload.Payload `json:"resolutionPayload,omitempty"`
	ContinuationAction      string                       `json:"continuationAction,omitempty"`
	ResumeRunID             string                       `json:"resumeRunId,omitempty"`
	SuccessorRunID          string                       `json:"successorRunId,omitempty"`
	AutoGovernance          *AutoGovernanceSummary       `json:"autoGovernance,omitempty"`
	ResolvedAt              string                       `json:"resolvedAt,omitempty"`
	ResolvedBy              string                       `json:"resolvedBy,omitempty"`
	CreatedAt               string                       `json:"createdAt,omitempty"`
	UpdatedAt               string                       `json:"updatedAt,omitempty"`
}

// DefinitionClarification is the bounded, owner-neutral read projection of a
// Decision-owned clarification binding. It is immutable display/navigation
// data: clients must not turn it into a generic Decision resolution or Runtime
// continuation, nor derive replacement admission from its display fields.
type DefinitionClarification struct {
	Class                       string   `json:"class"`
	BindingState                string   `json:"bindingState"`
	OpenBindingHash             string   `json:"openBindingHash,omitempty"`
	RequiredAction              string   `json:"requiredAction"`
	CandidateID                 string   `json:"candidateId,omitempty"`
	CandidateRevision           uint64   `json:"candidateRevision,omitempty"`
	CandidateHash               string   `json:"candidateHash,omitempty"`
	ViolationOrGapCode          string   `json:"violationOrGapCode"`
	CanonicalSignatureVersion   string   `json:"canonicalSignatureVersion,omitempty"`
	CanonicalSignature          string   `json:"canonicalSignature,omitempty"`
	CanonicalSignatureHash      string   `json:"canonicalSignatureHash,omitempty"`
	OffendingRefs               []string `json:"offendingRefs,omitempty"`
	DefinitionRevision          string   `json:"definitionRevision,omitempty"`
	StartupInputHash            string   `json:"startupInputHash,omitempty"`
	AuthoritySnapshotID         string   `json:"authoritySnapshotId,omitempty"`
	ClarificationDispatchID     string   `json:"clarificationDispatchId,omitempty"`
	FocusRef                    string   `json:"focusRef,omitempty"`
	LogicalRequestID            string   `json:"logicalRequestId,omitempty"`
	SemanticDocumentHash        string   `json:"semanticDocumentHash,omitempty"`
	ResolutionID                string   `json:"resolutionId,omitempty"`
	ResolutionBindingHash       string   `json:"resolutionBindingHash,omitempty"`
	ReplacementIntentID         string   `json:"replacementIntentId,omitempty"`
	ReplacementIntentHash       string   `json:"replacementIntentHash,omitempty"`
	SuccessorDefinitionRevision string   `json:"successorDefinitionRevision,omitempty"`
	SuccessorStartupInputHash   string   `json:"successorStartupInputHash,omitempty"`
}

// Approval is the complete owner-neutral projection of an approval ticket.
type Approval struct {
	ID                string                       `json:"id"`
	WorkspaceID       string                       `json:"workspaceId"`
	RunID             string                       `json:"runId"`
	StepID            string                       `json:"stepId,omitempty"`
	Reason            string                       `json:"reason"`
	Action            string                       `json:"action,omitempty"`
	Context           workorcheventpayload.Payload `json:"context,omitempty"`
	State             string                       `json:"state"`
	ResolutionID      string                       `json:"resolutionId,omitempty"`
	ResolutionContext workorcheventpayload.Payload `json:"resolutionContext,omitempty"`
	ResolvedByKind    string                       `json:"resolvedByKind,omitempty"`
	AutoGovernance    *AutoGovernanceSummary       `json:"autoGovernance,omitempty"`
	ResolvedAt        string                       `json:"resolvedAt,omitempty"`
	ResolvedBy        string                       `json:"resolvedBy,omitempty"`
	CreatedAt         string                       `json:"createdAt,omitempty"`
	UpdatedAt         string                       `json:"updatedAt,omitempty"`
}

// AutoGovernanceSummary is the immutable projection of automated governance.
type AutoGovernanceSummary struct {
	TraceID           string   `json:"traceId"`
	Action            string   `json:"action"`
	Status            string   `json:"status"`
	RiskLevel         string   `json:"riskLevel,omitempty"`
	PolicyMatched     string   `json:"policyMatched,omitempty"`
	PolicyBlocked     string   `json:"policyBlocked,omitempty"`
	Confidence        float64  `json:"confidence,omitempty"`
	Reason            string   `json:"reason,omitempty"`
	EvidenceRefs      []string `json:"evidenceRefs,omitempty"`
	PromptFingerprint string   `json:"promptFingerprint,omitempty"`
	OccurredAt        string   `json:"occurredAt,omitempty"`
}

func CloneDecision(item Decision) Decision {
	item.Context = item.Context.Clone()
	item.AllowedRecoveryActions = append([]string(nil), item.AllowedRecoveryActions...)
	item.DefinitionClarification = CloneDefinitionClarification(item.DefinitionClarification)
	item.ResolutionPayload = item.ResolutionPayload.Clone()
	item.AutoGovernance = cloneAutoGovernanceSummary(item.AutoGovernance)
	return item
}

func CloneDefinitionClarification(item *DefinitionClarification) *DefinitionClarification {
	if item == nil {
		return nil
	}
	ret := *item
	ret.OffendingRefs = append([]string(nil), item.OffendingRefs...)
	return &ret
}

func CloneDecisions(items []Decision) []Decision {
	if items == nil {
		return nil
	}
	ret := make([]Decision, 0, len(items))
	for _, item := range items {
		ret = append(ret, CloneDecision(item))
	}
	return ret
}

func CloneApproval(item Approval) Approval {
	item.Context = item.Context.Clone()
	item.ResolutionContext = item.ResolutionContext.Clone()
	item.AutoGovernance = cloneAutoGovernanceSummary(item.AutoGovernance)
	return item
}

func CloneApprovals(items []Approval) []Approval {
	if items == nil {
		return nil
	}
	ret := make([]Approval, 0, len(items))
	for _, item := range items {
		ret = append(ret, CloneApproval(item))
	}
	return ret
}

func cloneAutoGovernanceSummary(item *AutoGovernanceSummary) *AutoGovernanceSummary {
	if item == nil {
		return nil
	}
	ret := *item
	ret.EvidenceRefs = append([]string(nil), item.EvidenceRefs...)
	return &ret
}
