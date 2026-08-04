package workorchruntimecontract

import "github.com/muidea/workorch/internal/pkg/llmaudit"

// ProviderRequestAuditView is the sealed, secret-free audit surface for one provider
// request. It is safe to persist and transport across EventHub boundaries.
type ProviderRequestAuditView struct {
	RequestID             string                 `json:"requestId,omitempty"`
	WorkspaceID           string                 `json:"workspaceId,omitempty"`
	RunID                 string                 `json:"runId,omitempty"`
	StepID                string                 `json:"stepId,omitempty"`
	Target                ProviderModelTarget    `json:"target"`
	Operation             string                 `json:"operation,omitempty"`
	CapabilityRevision    string                 `json:"capabilityRevision,omitempty"`
	ConfigurationRevision string                 `json:"configurationRevision,omitempty"`
	StablePrefixHash      string                 `json:"stablePrefixHash,omitempty"`
	Kind                  string                 `json:"kind,omitempty"`
	APISurface            string                 `json:"apiSurface,omitempty"`
	Provider              string                 `json:"provider,omitempty"`
	ContextBudget         llmaudit.ContextBudget `json:"contextBudget,omitempty"`
	ProviderSubmitted     bool                   `json:"providerSubmitted"`
	Status                string                 `json:"status,omitempty"`
	FailureClass          string                 `json:"failureClass,omitempty"`
	StartedAt             string                 `json:"startedAt,omitempty"`
	CompletedAt           string                 `json:"completedAt,omitempty"`
	DurationMs            int                    `json:"durationMs,omitempty"`
	RequestBytes          int                    `json:"requestBytes,omitempty"`
	ResponseBytes         int                    `json:"responseBytes,omitempty"`
}
