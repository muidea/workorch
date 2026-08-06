// Package workorchmanagementcontract contains owner-neutral JSON DTOs shared
// by the management API producer and its clients. It carries no topic,
// command/result, repository, state transition, or transport behavior.
package workorchmanagementcontract

// WorkspaceCreateRequest is the single owner-neutral workspace creation
// request shared by protocol clients, process ports and the application
// ingress. Workspace identity remains owner-generated.
type WorkspaceCreateRequest struct {
	Title                string `json:"title"`
	Description          string `json:"description,omitempty"`
	DeliverableDirectory string `json:"deliverableDirectory,omitempty"`
	TemplateID           string `json:"templateId,omitempty"`
}

// WorkspaceExecutionCommandRequest contains only user-controlled scheduling
// inputs. Goal/DoD, provider, soul and capability inputs are compiled from the
// current Workspace owner state and therefore never enter this wire contract.
type WorkspaceExecutionCommandRequest struct {
	Priority  int    `json:"priority,omitempty"`
	RequestID string `json:"requestId,omitempty"`
	Reason    string `json:"reason,omitempty"`
	Source    string `json:"source,omitempty"`
	Intent    string `json:"intent,omitempty"`
}

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

func CloneGoals(items []Goal) []Goal {
	return append([]Goal(nil), items...)
}

func CloneDoD(item DoD) DoD {
	item.Items = append([]string(nil), item.Items...)
	return item
}

func CloneDoDs(items []DoD) []DoD {
	if items == nil {
		return nil
	}
	ret := make([]DoD, 0, len(items))
	for _, item := range items {
		ret = append(ret, CloneDoD(item))
	}
	return ret
}

// WorkspaceSoulBindings is the owner-neutral management projection used by
// the HTTP producer and protocol clients. It carries no workspace mutation or
// routing authority.
type WorkspaceSoulBindings struct {
	WorkspaceID     string            `json:"workspaceId,omitempty"`
	DefaultSoulID   string            `json:"defaultSoulId,omitempty"`
	Bindings        map[string]string `json:"bindings,omitempty"`
	PriorityPolicy  []string          `json:"priorityPolicy,omitempty"`
	AvailableStages []string          `json:"availableStages,omitempty"`
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

// SoulMutation contains only caller-controlled Soul configuration. Owner read
// entities are never accepted as write commands.
type SoulMutation struct {
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

// SkillMutation excludes lifecycle, managed-source and observed hash fields
// that are produced by the Skill owner.
type SkillMutation struct {
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

type MCPServerMutation struct {
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

type MCPCapabilityMutation struct {
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

type ProviderModelDescriptorView struct {
	ProviderID          string   `json:"providerId"`
	ModelID             string   `json:"modelId"`
	RouteOwner          string   `json:"routeOwner,omitempty"`
	ContextWindowTokens int      `json:"contextWindowTokens,omitempty"`
	MaxOutputTokens     int      `json:"maxOutputTokens,omitempty"`
	SupportedEndpoints  []string `json:"supportedEndpoints,omitempty"`
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
