package models

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

type RuntimeCandidateRequest struct {
	Decision          runtimecontract.RuntimeDecision
	Transaction       runtimecontract.AuthorityTransaction
	CandidateAttempt  int
	PreviousFailure   *runtimecontract.RuntimeCandidateFailure
	StablePrefixHash  string
	DefinitionContext runtimecontract.RuntimeDefinitionContext
	CandidateContext  runtimecontract.RuntimeCandidateContext
}

// RuntimeCandidateGenerationResult is the typed provider outcome for a Runtime V2 candidate
// request. It carries the raw payload, usage, sealed request audit and execution status so
// callers can persist a complete audit without relying on local request mutation.
type RuntimeCandidateGenerationResult struct {
	// RequestID correlates completion, usage and context-budget audit for one provider call.
	RequestID string `json:"requestId,omitempty"`
	// Payload is the raw candidate JSON body returned by the provider (empty on failure).
	Payload []byte `json:"payload,omitempty"`
	// Usage is provider-reported token usage when available.
	Usage *ProviderUsage `json:"usage,omitempty"`
	// Audit is the sealed request audit snapshot (target, revisions, budget, submitted status).
	Audit *ProviderRequestAudit `json:"audit,omitempty"`
	// Status is completed | failed.
	Status string `json:"status,omitempty"`
	// FailureClass classifies provider/schema/internal failures for audit.
	FailureClass string `json:"failureClass,omitempty"`
	// FailureReason is a redacted human-readable failure summary.
	FailureReason string `json:"failureReason,omitempty"`
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

// Runtime candidate response returned across the EventHub boundary after decode/validation.
type RuntimeCandidateResponse struct {
	Transaction runtimecontract.AuthorityTransaction
	Candidate   runtimecontract.RuntimeCandidate
	// Audit is present for successful and failed provider requests when generation ran.
	Audit *ProviderRequestAudit `json:"audit,omitempty"`
	// Usage is present when the provider reported token usage.
	Usage *ProviderUsage `json:"usage,omitempty"`
	// RequestID correlates this result with the persisted usage audit record.
	RequestID string `json:"requestId,omitempty"`
	// Status mirrors generation status (completed | failed) when audit is available.
	Status string `json:"status,omitempty"`
}

const (
	RuntimeCandidateStatusCompleted = "completed"
	RuntimeCandidateStatusFailed    = "failed"
)
