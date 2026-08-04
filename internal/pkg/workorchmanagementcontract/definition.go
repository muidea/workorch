// Package workorchmanagementcontract contains owner-neutral JSON DTOs shared
// by the management API producer and its clients. It carries no topic,
// command/result, repository, state transition, or transport behavior.
package workorchmanagementcontract

type Goal struct {
	ID                     string `json:"id"`
	WorkspaceID            string `json:"workspaceId"`
	Text                   string `json:"text"`
	DefinitionRevision     string `json:"definitionRevision,omitempty"`
	PreviousDefinitionHash string `json:"previousDefinitionHash,omitempty"`
	PreviousGoalID         string `json:"previousGoalId,omitempty"`
	LineageState           string `json:"lineageState,omitempty"`
}

type DoD struct {
	ID                     string   `json:"id"`
	WorkspaceID            string   `json:"workspaceId"`
	Items                  []string `json:"items"`
	State                  string   `json:"state"`
	DefinitionRevision     string   `json:"definitionRevision,omitempty"`
	PreviousDefinitionHash string   `json:"previousDefinitionHash,omitempty"`
	PreviousDoDID          string   `json:"previousDoDId,omitempty"`
	LineageState           string   `json:"lineageState,omitempty"`
}

type WorkspaceTemplate struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Description      string            `json:"description,omitempty"`
	GoalText         string            `json:"goalText,omitempty"`
	DoDItems         []string          `json:"doDItems,omitempty"`
	DefaultSoulID    string            `json:"defaultSoulId,omitempty"`
	SoulBindings     map[string]string `json:"soulBindings,omitempty"`
	ContextHints     map[string]any    `json:"contextHints,omitempty"`
	EvidencePolicies []map[string]any  `json:"evidencePolicies,omitempty"`
}

type Soul struct {
	ID                          string   `json:"id"`
	Name                        string   `json:"name"`
	Description                 string   `json:"description,omitempty"`
	BusinessDomains             []string `json:"businessDomains,omitempty"`
	SoulMD                      string   `json:"soulMd"`
	AllowedSkillRefs            []string `json:"allowedSkillRefs,omitempty"`
	AllowedMCPServerRefs        []string `json:"allowedMCPServerRefs,omitempty"`
	AllowedCapabilityRefs       []string `json:"allowedCapabilityRefs,omitempty"`
	AllowedCapabilityCategories []string `json:"allowedCapabilityCategories,omitempty"`
	AllowedToolKinds            []string `json:"allowedToolKinds,omitempty"`
}

type Skill struct {
	ID                     string   `json:"id"`
	Name                   string   `json:"name"`
	Version                string   `json:"version"`
	Description            string   `json:"description,omitempty"`
	TriggerRules           []string `json:"triggerRules,omitempty"`
	Instruction            string   `json:"instruction"`
	ReferenceRefs          []string `json:"referenceRefs,omitempty"`
	RequiredToolKinds      []string `json:"requiredToolKinds,omitempty"`
	RequiredMCPServerRefs  []string `json:"requiredMCPServerRefs,omitempty"`
	RequiredCapabilityRefs []string `json:"requiredCapabilityRefs,omitempty"`
	Constraints            []string `json:"constraints,omitempty"`
	Source                 string   `json:"source,omitempty"`
	Scope                  string   `json:"scope,omitempty"`
	WorkspaceID            string   `json:"workspaceId,omitempty"`
	Status                 string   `json:"status,omitempty"`
	SourceRepository       string   `json:"sourceRepository,omitempty"`
	SourcePath             string   `json:"sourcePath,omitempty"`
	TargetPath             string   `json:"targetPath,omitempty"`
	LocalPath              string   `json:"localPath,omitempty"`
	RepoPath               string   `json:"repoPath,omitempty"`
	GlobalPath             string   `json:"globalPath,omitempty"`
	ProjectPath            string   `json:"projectPath,omitempty"`
	ContentHash            string   `json:"contentHash,omitempty"`
	SourceHash             string   `json:"sourceHash,omitempty"`
	AppliedHash            string   `json:"appliedHash,omitempty"`
	ActualHash             string   `json:"actualHash,omitempty"`
	Agent                  string   `json:"agent,omitempty"`
	ManagedBy              string   `json:"managedBy,omitempty"`
	SourceSkillID          string   `json:"sourceSkillId,omitempty"`
}

type MCPTool struct {
	ServerID string `json:"serverId"`
	Name     string `json:"name"`
}

type MCPResource struct {
	ServerID string `json:"serverId"`
	URI      string `json:"uri"`
}

type MCPPrompt struct {
	ServerID string `json:"serverId"`
	Name     string `json:"name"`
}

type MCPServer struct {
	ID                    string            `json:"id"`
	Name                  string            `json:"name"`
	Transport             string            `json:"transport"`
	Endpoint              string            `json:"endpoint,omitempty"`
	Command               string            `json:"command,omitempty"`
	Args                  []string          `json:"args,omitempty"`
	Env                   map[string]string `json:"env,omitempty"`
	Enabled               bool              `json:"enabled"`
	AllowedTools          []string          `json:"allowedTools,omitempty"`
	AllowedResources      []string          `json:"allowedResources,omitempty"`
	AllowedPrompts        []string          `json:"allowedPrompts,omitempty"`
	DefaultApprovalPolicy string            `json:"defaultApprovalPolicy,omitempty"`
	SessionPolicy         string            `json:"sessionPolicy,omitempty"`
	TimeoutMillis         int               `json:"timeoutMillis,omitempty"`
}

type MCPCapability struct {
	ID               string         `json:"id"`
	ServerID         string         `json:"serverId"`
	Kind             string         `json:"kind"`
	Name             string         `json:"name"`
	DisplayName      string         `json:"displayName"`
	Description      string         `json:"description,omitempty"`
	Category         string         `json:"category,omitempty"`
	Tags             []string       `json:"tags,omitempty"`
	UsageHint        string         `json:"usageHint,omitempty"`
	InputSummary     string         `json:"inputSummary,omitempty"`
	OutputSummary    string         `json:"outputSummary,omitempty"`
	StageHints       []string       `json:"stageHints,omitempty"`
	RiskLevel        string         `json:"riskLevel,omitempty"`
	EffectScope      string         `json:"effectScope,omitempty"`
	IdempotencyClass string         `json:"idempotencyClass,omitempty"`
	Schema           map[string]any `json:"schema,omitempty"`
	URI              string         `json:"uri,omitempty"`
	Enabled          bool           `json:"enabled"`
	ApprovalPolicy   string         `json:"approvalPolicy,omitempty"`
	Status           string         `json:"status,omitempty"`
}

type MemoryItem struct {
	ID                  string   `json:"id"`
	WorkspaceID         string   `json:"workspaceId"`
	RunID               string   `json:"runId,omitempty"`
	SourceType          string   `json:"sourceType"`
	SourceRef           string   `json:"sourceRef,omitempty"`
	MemoryType          string   `json:"memoryType"`
	Content             string   `json:"content,omitempty"`
	Summary             string   `json:"summary"`
	Tags                []string `json:"tags,omitempty"`
	Importance          int      `json:"importance,omitempty"`
	Confidence          int      `json:"confidence,omitempty"`
	Visibility          string   `json:"visibility,omitempty"`
	EmbeddingProviderID string   `json:"embeddingProviderId,omitempty"`
	EmbeddingModel      string   `json:"embeddingModel,omitempty"`
	EmbeddingText       string   `json:"embeddingText,omitempty"`
	VectorReady         bool     `json:"vectorReady"`
	VectorDimension     int      `json:"vectorDimension,omitempty"`
	VectorScore         float64  `json:"vectorScore,omitempty"`
}

type MemoryMutation struct {
	MemoryID            string    `json:"memoryId,omitempty"`
	RunID               string    `json:"runId,omitempty"`
	SourceType          string    `json:"sourceType"`
	SourceRef           string    `json:"sourceRef,omitempty"`
	MemoryType          string    `json:"memoryType"`
	Content             string    `json:"content,omitempty"`
	Summary             string    `json:"summary"`
	Tags                []string  `json:"tags,omitempty"`
	Importance          int       `json:"importance,omitempty"`
	Confidence          int       `json:"confidence,omitempty"`
	Visibility          string    `json:"visibility,omitempty"`
	EmbeddingProviderID string    `json:"embeddingProviderId,omitempty"`
	EmbeddingModel      string    `json:"embeddingModel,omitempty"`
	EmbeddingText       string    `json:"embeddingText,omitempty"`
	Vector              []float64 `json:"vector,omitempty"`
}

type MemorySearchQuery struct {
	WorkspaceID string
	Query       string
	Tags        []string
	Types       []string
	Limit       int
}

type ProviderModelDescriptorView struct {
	ProviderID          string   `json:"providerId"`
	ModelID             string   `json:"modelId"`
	RouteOwner          string   `json:"routeOwner,omitempty"`
	ContextWindowTokens int      `json:"contextWindowTokens,omitempty"`
	MaxOutputTokens     int      `json:"maxOutputTokens,omitempty"`
	Operations          []string `json:"operations,omitempty"`
	CapabilityReady     bool     `json:"capabilityReady"`
	UnavailableReason   string   `json:"unavailableReason,omitempty"`
	CapabilityRevision  string   `json:"capabilityRevision,omitempty"`
}

type ProviderCatalogSummary struct {
	ProviderID   string                        `json:"providerId"`
	State        string                        `json:"state"`
	Revision     string                        `json:"revision,omitempty"`
	FetchedAt    string                        `json:"fetchedAt,omitempty"`
	ExpiresAt    string                        `json:"expiresAt,omitempty"`
	Stale        bool                          `json:"stale"`
	Source       string                        `json:"source,omitempty"`
	ErrorCode    string                        `json:"errorCode,omitempty"`
	ErrorMessage string                        `json:"errorMessage,omitempty"`
	Models       []ProviderModelDescriptorView `json:"models,omitempty"`
}
