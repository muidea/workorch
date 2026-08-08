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
	// ReachedProviderLimit indicates that the provider returned a bounded,
	// usable prefix but stopped at its output limit. Payload must be preserved
	// so the Runtime owner can continue the same logical document.
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

// Runtime candidate response returned by the LLM boundary. A complete provider
// document remains opaque here: only the Orchestrator-owned SM-3 evaluator may
// decode or admit it after durable persistence.
type RuntimeCandidateResponse struct {
	Transaction          runtimecontract.AuthorityTransaction
	DocumentKind         runtimecontract.RuntimeProviderDocumentKind
	RequestIdentity      runtimecontract.RuntimeCandidateRequestIdentity `json:"requestIdentity"`
	ExpectedDocumentHash string                                          `json:"expectedDocumentHash,omitempty"`
	// DocumentContent carries a complete provider document only across the
	// synchronous LLM-owner EventHub exchange. It is
	// intentionally excluded from JSON/audit projection; Orchestrator must
	// durably store it before any session or candidate transition consumes it.
	DocumentContent []byte `json:"-"`
	// DocumentOutcome is the LLM owner's bounded framing observation. It does
	// not select recovery, correction, admission, or any Runtime transition.
	DocumentOutcome *runtimecontract.RuntimeProviderDocumentOutcome `json:"documentOutcome,omitempty"`
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
