package workorchclient

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// Workspace resource types (matches workorchd HTTP JSON format with lowercase camelCase tags).
type WorkspaceListProjection struct {
	Projection projectioncontract.ProjectionStatus `json:"projection"`
	Workspaces []projectioncontract.Workspace      `json:"workspaces"`
}

// Run resource types.
// Decision & Approval types.
type EventRecordQuery struct {
	WorkspaceID    string
	RunID          string
	Topic          string
	SubjectType    string
	SubjectID      string
	CursorAfterID  string
	Limit          int
	IncludePayload bool
}

// Snapshot is the full workspace/run snapshot returned by the read model.
type WorkspaceSnapshotProjection struct {
	Projection projectioncontract.ProjectionStatus  `json:"projection"`
	Snapshot   projectioncontract.WorkspaceSnapshot `json:"snapshot"`
}

type WorkspaceRuntimeViewProjection struct {
	Service     string                                  `json:"service"`
	Backend     string                                  `json:"backend,omitempty"`
	DataRoot    string                                  `json:"dataRoot,omitempty"`
	Projection  projectioncontract.ProjectionStatus     `json:"projection"`
	RuntimeView projectioncontract.WorkspaceRuntimeView `json:"runtimeView"`
	Error       string                                  `json:"error,omitempty"`
}

type TimelineEntry struct {
	Source     string         `json:"source"`
	Type       string         `json:"type"`
	ID         string         `json:"id,omitempty"`
	Topic      string         `json:"topic,omitempty"`
	RunState   string         `json:"runState,omitempty"`
	StepID     string         `json:"stepId,omitempty"`
	StepState  string         `json:"stepState,omitempty"`
	Summary    string         `json:"summary,omitempty"`
	OccurredAt string         `json:"occurredAt,omitempty"`
	Data       map[string]any `json:"data,omitempty"`
	Output     map[string]any `json:"output,omitempty"`
}

// Workspace soul bindings.
type WorkspaceSoulBindings struct {
	WorkspaceID     string            `json:"workspaceId,omitempty"`
	DefaultSoulID   string            `json:"defaultSoulId,omitempty"`
	Bindings        map[string]string `json:"bindings,omitempty"`
	PriorityPolicy  []string          `json:"priorityPolicy,omitempty"`
	AvailableStages []string          `json:"availableStages,omitempty"`
}

// Configuration types.
// Keep legacy provider profile structs for usage/trace DTOs that still project them.
type ProviderCacheCapability struct {
	Surface             string   `json:"surface,omitempty"`
	Mode                string   `json:"mode,omitempty"`
	PromptCacheKey      bool     `json:"promptCacheKey,omitempty"`
	MessageCacheControl bool     `json:"messageCacheControl,omitempty"`
	UsageFields         []string `json:"usageFields,omitempty"`
}

type ProviderInferenceProfile struct {
	Temperature          *float64 `json:"temperature,omitempty"`
	MaxOutputTokens      int      `json:"maxOutputTokens,omitempty"`
	ReasoningEffort      string   `json:"reasoningEffort,omitempty"`
	ThinkingBudgetTokens int      `json:"thinkingBudgetTokens,omitempty"`
}

type ProviderContextProfile struct {
	CapabilitySource        string  `json:"capabilitySource,omitempty"`
	ContextWindowTokens     int     `json:"contextWindowTokens,omitempty"`
	MaxInputTokens          int     `json:"maxInputTokens,omitempty"`
	ReservedOutputTokens    int     `json:"reservedOutputTokens,omitempty"`
	ContextUtilizationRatio float64 `json:"contextUtilizationRatio,omitempty"`
	MaxRequestBytes         int     `json:"maxRequestBytes,omitempty"`
	EstimatedTokensPerByte  float64 `json:"estimatedTokensPerByte,omitempty"`
}
