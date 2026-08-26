package models

import (
	"github.com/muidea/workorch/internal/pkg/llmaudit"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type RuntimeCandidateRequest struct {
	DocumentKind                runtimecontract.RuntimeProviderDocumentKind
	RequestIdentity             runtimecontract.RuntimeCandidateRequestIdentity
	ExpectedDocument            *runtimecontract.RuntimeExpectedDocumentContract
	Decision                    runtimecontract.RuntimeDecision
	Transaction                 runtimecontract.AuthorityTransaction
	CandidateAttempt            int
	PreviousFailure             *runtimecontract.RuntimeCandidateFailure
	StablePrefixHash            string
	CandidateManifest           runtimecontract.RuntimeCandidateContractManifest
	DefinitionContext           runtimecontract.RuntimeDefinitionContext
	CandidateContext            runtimecontract.RuntimeCandidateContext
	SemanticManifest            runtimecontract.RuntimeSemanticPlanManifest
	SemanticContext             runtimecontract.RuntimeSemanticPlanRequestContext
	PreviousSemanticRejection   *runtimecontract.RuntimeOriginalDiagnostic
	SemanticCorrectionDirective *runtimecontract.RuntimeSemanticCorrectionDirective
}

// RuntimeCandidateGenerationResult is the typed provider outcome for a Runtime candidate
// request. It carries the raw payload, usage, sealed request audit and execution status so
// callers can persist a complete audit without relying on local request mutation.
type RuntimeCandidateGenerationResult struct {
	// RequestID correlates completion, usage and context-budget audit for one provider call.
	RequestID string `json:"requestId,omitempty"`
	// Payload is the raw candidate JSON body returned by the provider (empty on failure).
	Payload []byte `json:"payload,omitempty"`
	// Usage is provider-reported token usage when available.
	Usage *llmaudit.ProviderUsage `json:"usage,omitempty"`
	// Audit is the sealed request audit snapshot (target, revisions, budget, submitted status).
	Audit *ProviderRequestAudit `json:"audit,omitempty"`
	// Status is completed | failed.
	Status string `json:"status,omitempty"`
	// FailureClass classifies provider/schema/internal failures for audit.
	FailureClass string `json:"failureClass,omitempty"`
	// FailureReason is a redacted human-readable failure summary.
	FailureReason string `json:"failureReason,omitempty"`
	// ReachedProviderLimit records an explicit provider max-output stop. Payload
	// is retained only long enough for bounded framing audit and is never
	// published as partial document content. The Runtime owner may reissue the
	// same sealed expected document as a new physical call.
	ReachedProviderLimit bool `json:"reachedProviderLimit,omitempty"`
	// PrerequisiteStale is present only when the LLM owner has confirmed that
	// a persisted prerequisite observation no longer represents the sealed
	// provider target. It is not a provider failure and cannot produce a
	// candidate.
	PrerequisiteStale *runtimecontract.RuntimePrerequisiteStale `json:"prerequisiteStale,omitempty"`
}

// ProviderRequestAudit is the sealed, secret-free audit surface for one provider request.
type ProviderRequestAudit struct {
	RequestID             string                              `json:"requestId,omitempty"`
	LogicalRequestID      string                              `json:"logicalRequestId,omitempty"`
	PhysicalCallID        string                              `json:"physicalCallId,omitempty"`
	WorkspaceID           string                              `json:"workspaceId,omitempty"`
	RunID                 string                              `json:"runId,omitempty"`
	StepID                string                              `json:"stepId,omitempty"`
	Target                runtimecontract.ProviderModelTarget `json:"target"`
	Operation             string                              `json:"operation,omitempty"`
	CapabilityRevision    string                              `json:"capabilityRevision,omitempty"`
	ConfigurationRevision string                              `json:"configurationRevision,omitempty"`
	StablePrefixHash      string                              `json:"stablePrefixHash,omitempty"`
	Kind                  string                              `json:"kind,omitempty"`
	APISurface            string                              `json:"apiSurface,omitempty"`
	Provider              string                              `json:"provider,omitempty"`
	PhysicalResponseMode  llmaudit.PhysicalResponseMode       `json:"physicalResponseMode"`
	ContextBudget         *ProviderContextBudget              `json:"contextBudget,omitempty"`
	ProviderSubmitted     bool                                `json:"providerSubmitted"`
	Status                string                              `json:"status,omitempty"`
	FailureClass          string                              `json:"failureClass,omitempty"`
	StartedAt             string                              `json:"startedAt,omitempty"`
	CompletedAt           string                              `json:"completedAt,omitempty"`
	DurationMs            int                                 `json:"durationMs,omitempty"`
	RequestBytes          int                                 `json:"requestBytes,omitempty"`
	ResponseBytes         int                                 `json:"responseBytes,omitempty"`
}

// RuntimeCandidateDocumentObservation is the provider-boundary observation
// used inside the LLM owner before it publishes its concrete EventHub fact.
type RuntimeCandidateDocumentObservation struct {
	Framing runtimecontract.RuntimeProviderDocumentFraming `json:"framing"`
	Content []byte                                         `json:"-"`
}

func (o RuntimeCandidateDocumentObservation) Normalized() RuntimeCandidateDocumentObservation {
	o.Framing = o.Framing.Normalized()
	o.Content = append([]byte(nil), o.Content...)
	return o
}

// Runtime candidate response returned by the LLM boundary. A complete provider
// document remains opaque here: only the Orchestrator-owned SM-3 evaluator may
// decode or admit it after durable persistence.
type RuntimeCandidateResponse struct {
	WorkspaceID          string
	RunID                string
	Transaction          runtimecontract.AuthorityTransaction
	DocumentKind         runtimecontract.RuntimeProviderDocumentKind
	RequestIdentity      runtimecontract.RuntimeCandidateRequestIdentity `json:"requestIdentity"`
	ExpectedDocumentHash string                                          `json:"expectedDocumentHash,omitempty"`
	// DocumentOutcome is the LLM owner's complete, mutually exclusive response
	// fact. It does not select recovery, correction, admission, or any Runtime
	// transition.
	DocumentOutcome *RuntimeCandidateDocumentObservation `json:"documentOutcome,omitempty"`
	// Audit is present for successful and failed provider requests when generation ran.
	Audit *ProviderRequestAudit `json:"audit,omitempty"`
	// Usage is present when the provider reported token usage.
	Usage *llmaudit.ProviderUsage `json:"usage,omitempty"`
	// PrerequisiteStale preserves an owner-confirmed pre-candidate freshness
	// outcome. Orchestrator owns the resulting invalidation and reacquisition.
	PrerequisiteStale *runtimecontract.RuntimePrerequisiteStale `json:"prerequisiteStale,omitempty"`
}

const (
	RuntimeCandidateStatusCompleted = "completed"
	RuntimeCandidateStatusFailed    = "failed"
)
