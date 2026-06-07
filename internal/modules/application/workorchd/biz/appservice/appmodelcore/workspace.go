package appmodelcore

type Workspace struct {
	ID                   string            `json:"id"`
	Title                string            `json:"title"`
	Description          string            `json:"description"`
	DeliverableDirectory string            `json:"deliverableDirectory"`
	InternalDirectory    string            `json:"-"`
	State                string            `json:"state"`
	ActiveRunID          string            `json:"activeRunId,omitempty"`
	BlockingTicketID     string            `json:"blockingTicketId,omitempty"`
	BlockingApprovalID   string            `json:"blockingApprovalId,omitempty"`
	TemplateID           string            `json:"templateId,omitempty"`
	ProfileID            string            `json:"profileId,omitempty"`
	DefaultSoulID        string            `json:"defaultSoulId,omitempty"`
	SoulBindings         map[string]string `json:"soulBindings,omitempty"`
	ContextHints         map[string]any    `json:"contextHints,omitempty"`
	EvidencePolicies     []map[string]any  `json:"evidencePolicies,omitempty"`
}

type CreateWorkspaceCommand struct {
	Title                string `json:"title"`
	Description          string `json:"description,omitempty"`
	DeliverableDirectory string `json:"deliverableDirectory,omitempty"`
	TemplateID           string `json:"templateId,omitempty"`
}

type ActivateWorkspaceExecutionCommand struct {
	GoalID                string            `json:"goalId,omitempty"`
	DoDID                 string            `json:"doDId,omitempty"`
	LLMProviderID         string            `json:"llmProviderId,omitempty"`
	SoulID                string            `json:"soulId,omitempty"`
	SoulBindings          map[string]string `json:"soulBindings,omitempty"`
	SkillIDs              []string          `json:"skillIds,omitempty"`
	MCPServerIDs          []string          `json:"mcpServerIds,omitempty"`
	CapabilityIDs         []string          `json:"capabilityIds,omitempty"`
	MemoryIDs             []string          `json:"memoryIds,omitempty"`
	DialogLanguage        string            `json:"dialogLanguage,omitempty"`
	Priority              int               `json:"priority,omitempty"`
	ExecutionContextPatch map[string]any    `json:"executionContextPatch,omitempty"`
}

type DeleteWorkspaceCommand struct {
	Purge bool `json:"purge,omitempty"`
}
