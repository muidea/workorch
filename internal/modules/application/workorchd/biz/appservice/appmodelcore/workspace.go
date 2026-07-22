package appmodelcore

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

type Workspace struct {
	ID                   string                                    `json:"id"`
	Title                string                                    `json:"title"`
	Description          string                                    `json:"description"`
	DeliverableDirectory string                                    `json:"deliverableDirectory"`
	InternalDirectory    string                                    `json:"-"`
	State                string                                    `json:"state"`
	ActiveRunID          string                                    `json:"activeRunId,omitempty"`
	BlockingTicketID     string                                    `json:"blockingTicketId,omitempty"`
	BlockingApprovalID   string                                    `json:"blockingApprovalId,omitempty"`
	TemplateID           string                                    `json:"templateId,omitempty"`
	ProfileID            string                                    `json:"profileId,omitempty"`
	DefaultLLMTarget     *runtimecontract.ProviderModelTarget      `json:"defaultLLMTarget,omitempty"`
	TargetDeclaration    *runtimecontract.RuntimeTargetDeclaration `json:"targetDeclaration,omitempty"`
	DefaultSoulID        string                                    `json:"defaultSoulId,omitempty"`
	SoulBindings         map[string]string                         `json:"soulBindings,omitempty"`
	ContextHints         map[string]any                            `json:"contextHints,omitempty"`
	EvidencePolicies     []map[string]any                          `json:"evidencePolicies,omitempty"`
	ViewModel            WorkspaceViewModel                        `json:"viewModel,omitempty"`
}

type WorkspaceViewModel struct {
	WorkspaceID             string                               `json:"workspaceId,omitempty"`
	State                   string                               `json:"state,omitempty"`
	PromptMode              string                               `json:"promptMode,omitempty"`
	Revision                int64                                `json:"revision,omitempty"`
	StreamGeneration        string                               `json:"streamGeneration,omitempty"`
	GlobalRevision          int64                                `json:"globalRevision,omitempty"`
	WorkspaceRevision       int64                                `json:"workspaceRevision,omitempty"`
	SourceEventRevision     int64                                `json:"sourceEventRevision,omitempty"`
	ProjectedSourceRevision int64                                `json:"projectedSourceRevision,omitempty"`
	UpdatedAt               string                               `json:"updatedAt,omitempty"`
	RebuiltAt               string                               `json:"rebuiltAt,omitempty"`
	Degraded                bool                                 `json:"degraded,omitempty"`
	StaleReason             string                               `json:"staleReason,omitempty"`
	Error                   string                               `json:"error,omitempty"`
	FailedWorkspaceIds      []string                             `json:"failedWorkspaceIds,omitempty"`
	DegradedSince           string                               `json:"degradedSince,omitempty"`
	Sections                map[string]WorkspaceViewModelSection `json:"sections,omitempty"`
}

type WorkspaceViewModelSection struct {
	Name                    string `json:"name"`
	Revision                int64  `json:"revision,omitempty"`
	WorkspaceRevision       int64  `json:"workspaceRevision,omitempty"`
	SourceEventRevision     int64  `json:"sourceEventRevision,omitempty"`
	ProjectedSourceRevision int64  `json:"projectedSourceRevision,omitempty"`
	State                   string `json:"state,omitempty"`
}

type CreateWorkspaceCommand struct {
	Title                string `json:"title"`
	Description          string `json:"description,omitempty"`
	DeliverableDirectory string `json:"deliverableDirectory,omitempty"`
	TemplateID           string `json:"templateId,omitempty"`
}

type ActivateWorkspaceExecutionCommand struct {
	GoalID            string                                    `json:"goalId,omitempty"`
	DoDID             string                                    `json:"doDId,omitempty"`
	LLMTarget         *runtimecontract.ProviderModelTarget      `json:"llmTarget,omitempty"`
	TargetDeclaration *runtimecontract.RuntimeTargetDeclaration `json:"targetDeclaration,omitempty"`
	SoulID            string                                    `json:"soulId,omitempty"`
	SoulBindings      map[string]string                         `json:"soulBindings,omitempty"`
	SkillIDs          []string                                  `json:"skillIds,omitempty"`
	MCPServerIDs      []string                                  `json:"mcpServerIds,omitempty"`
	CapabilityIDs     []string                                  `json:"capabilityIds,omitempty"`
	MemoryIDs         []string                                  `json:"memoryIds,omitempty"`
	DialogLanguage    string                                    `json:"dialogLanguage,omitempty"`
	Priority          int                                       `json:"priority,omitempty"`
	Restart           bool                                      `json:"restart,omitempty"`
}

type DeleteWorkspaceCommand struct {
	Purge bool `json:"purge,omitempty"`
}
