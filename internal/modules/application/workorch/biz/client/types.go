package client

// Workspace resource types (matches workorchd HTTP JSON format with lowercase camelCase tags).
type Workspace struct {
	ID               string            `json:"id"`
	Title            string            `json:"title"`
	Description      string            `json:"description,omitempty"`
	WorkingDirectory string            `json:"workingDirectory,omitempty"`
	State            string            `json:"state"`
	ActiveRunID      string            `json:"activeRunId,omitempty"`
	DefaultSoulID    string            `json:"defaultSoulId,omitempty"`
	SoulBindings     map[string]string `json:"soulBindings,omitempty"`
}

type Goal struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspaceId"`
	Text        string `json:"text"`
}

type DoD struct {
	ID          string   `json:"id"`
	WorkspaceID string   `json:"workspaceId"`
	Items       []string `json:"items"`
	State       string   `json:"state"`
}

// Run resource types.
type Run struct {
	ID                 string         `json:"id"`
	WorkspaceID        string         `json:"workspaceId"`
	GoalID             string         `json:"goalId,omitempty"`
	DoDID              string         `json:"doDId,omitempty"`
	InitialStepKind    string         `json:"initialStepKind,omitempty"`
	ExecutionContext   map[string]any `json:"executionContext,omitempty"`
	State              string         `json:"state"`
	Priority           int            `json:"priority,omitempty"`
	BlockingTicketID   string         `json:"blockingTicketId,omitempty"`
	BlockingApprovalID string         `json:"blockingApprovalId,omitempty"`
	FailureReason      string         `json:"failureReason,omitempty"`
}

type Step struct {
	ID     string         `json:"id"`
	RunID  string         `json:"runId"`
	Kind   string         `json:"kind"`
	State  string         `json:"state"`
	Input  map[string]any `json:"input,omitempty"`
	Output map[string]any `json:"output,omitempty"`
}

type ToolCall struct {
	ID          string         `json:"id"`
	WorkspaceID string         `json:"workspaceId"`
	RunID       string         `json:"runId"`
	StepID      string         `json:"stepId,omitempty"`
	ToolName    string         `json:"toolName"`
	Arguments   map[string]any `json:"arguments,omitempty"`
	State       string         `json:"state"`
	StartedAt   string         `json:"startedAt,omitempty"`
	EndedAt     string         `json:"endedAt,omitempty"`
	ExitSummary string         `json:"exitSummary,omitempty"`
	Output      map[string]any `json:"output,omitempty"`
	Error       string         `json:"error,omitempty"`
}

type Patch struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspaceId"`
	RunID       string `json:"runId"`
	StepID      string `json:"stepId,omitempty"`
	ToolCallID  string `json:"toolCallId,omitempty"`
	PatchType   string `json:"patchType"`
	TargetRef   string `json:"targetRef,omitempty"`
	Summary     string `json:"summary,omitempty"`
	State       string `json:"state"`
	CreatedAt   string `json:"createdAt,omitempty"`
	AppliedAt   string `json:"appliedAt,omitempty"`
}

type ValidationResult struct {
	ID             string `json:"id"`
	WorkspaceID    string `json:"workspaceId"`
	RunID          string `json:"runId"`
	StepID         string `json:"stepId,omitempty"`
	ValidationKind string `json:"validationKind"`
	TargetRef      string `json:"targetRef,omitempty"`
	State          string `json:"state"`
	Summary        string `json:"summary,omitempty"`
	EvidenceRef    string `json:"evidenceRef,omitempty"`
	StartedAt      string `json:"startedAt,omitempty"`
	EndedAt        string `json:"endedAt,omitempty"`
}

// Decision & Approval types.
type Decision struct {
	ID             string         `json:"id"`
	WorkspaceID    string         `json:"workspaceId"`
	RunID          string         `json:"runId"`
	StepID         string         `json:"stepId,omitempty"`
	Reason         string         `json:"reason"`
	DecisionKind   string         `json:"decisionKind,omitempty"`
	Question       string         `json:"question,omitempty"`
	Context        map[string]any `json:"context,omitempty"`
	State          string         `json:"state"`
	ResolutionID   string         `json:"resolutionId,omitempty"`
	ResolutionType string         `json:"resolutionType,omitempty"`
}

type Approval struct {
	ID          string         `json:"id"`
	WorkspaceID string         `json:"workspaceId"`
	RunID       string         `json:"runId"`
	StepID      string         `json:"stepId,omitempty"`
	Reason      string         `json:"reason"`
	Action      string         `json:"action,omitempty"`
	Context     map[string]any `json:"context,omitempty"`
	State       string         `json:"state"`
}

// Soul & Skill types.
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
}

// MCP types.
type MCPServer struct {
	ID                    string   `json:"id"`
	Name                  string   `json:"name"`
	Transport             string   `json:"transport"`
	Endpoint              string   `json:"endpoint,omitempty"`
	Enabled               bool     `json:"enabled"`
	AllowedTools          []string `json:"allowedTools,omitempty"`
	AllowedResources      []string `json:"allowedResources,omitempty"`
	AllowedPrompts        []string `json:"allowedPrompts,omitempty"`
	DefaultApprovalPolicy string   `json:"defaultApprovalPolicy,omitempty"`
	SessionPolicy         string   `json:"sessionPolicy,omitempty"`
	TimeoutMillis         int      `json:"timeoutMillis,omitempty"`
}

type MCPCapability struct {
	ID             string         `json:"id"`
	ServerID       string         `json:"serverId"`
	Kind           string         `json:"kind"`
	Name           string         `json:"name"`
	DisplayName    string         `json:"displayName"`
	Description    string         `json:"description,omitempty"`
	Category       string         `json:"category,omitempty"`
	Tags           []string       `json:"tags,omitempty"`
	UsageHint      string         `json:"usageHint,omitempty"`
	InputSummary   string         `json:"inputSummary,omitempty"`
	OutputSummary  string         `json:"outputSummary,omitempty"`
	StageHints     []string       `json:"stageHints,omitempty"`
	RiskLevel      string         `json:"riskLevel,omitempty"`
	Schema         map[string]any `json:"schema,omitempty"`
	URI            string         `json:"uri,omitempty"`
	Enabled        bool           `json:"enabled"`
	ApprovalPolicy string         `json:"approvalPolicy,omitempty"`
	Status         string         `json:"status,omitempty"`
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

// Memory type.
type MemoryItem struct {
	ID                  string    `json:"id"`
	WorkspaceID         string    `json:"workspaceId"`
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
	VectorReady         bool      `json:"vectorReady"`
	VectorDimension     int       `json:"vectorDimension,omitempty"`
	VectorScore         float64   `json:"vectorScore,omitempty"`
	Vector              []float64 `json:"vector,omitempty"`
}

type MemorySearchQuery struct {
	WorkspaceID string
	Query       string
	Tags        []string
	Types       []string
	Limit       int
}

// Workspace template type.
type WorkspaceTemplate struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Description   string            `json:"description,omitempty"`
	GoalText      string            `json:"goalText,omitempty"`
	DoDItems      []string          `json:"doDItems,omitempty"`
	DefaultSoulID string            `json:"defaultSoulId,omitempty"`
	SoulBindings  map[string]string `json:"soulBindings,omitempty"`
}

// Event types.
type EventRecord struct {
	ID           string         `json:"id"`
	Version      string         `json:"version"`
	Topic        string         `json:"topic"`
	OccurredAt   string         `json:"occurredAt,omitempty"`
	SourceModule string         `json:"sourceModule,omitempty"`
	SubjectType  string         `json:"subjectType,omitempty"`
	SubjectID    string         `json:"subjectId,omitempty"`
	WorkspaceID  string         `json:"workspaceId,omitempty"`
	RunID        string         `json:"runId,omitempty"`
	StepID       string         `json:"stepId,omitempty"`
	Payload      map[string]any `json:"payload,omitempty"`
}

// Snapshot / Status / Observability types.
type RunStatus struct {
	ID                 string `json:"id"`
	WorkspaceID        string `json:"workspaceId"`
	State              string `json:"state"`
	CurrentStepID      string `json:"currentStepId,omitempty"`
	CurrentStepKind    string `json:"currentStepKind,omitempty"`
	CurrentStepState   string `json:"currentStepState,omitempty"`
	StepCount          int    `json:"stepCount"`
	DecisionCount      int    `json:"decisionCount"`
	ApprovalCount      int    `json:"approvalCount"`
	BlockingTicketID   string `json:"blockingTicketId,omitempty"`
	BlockingApprovalID string `json:"blockingApprovalId,omitempty"`
	FailureReason      string `json:"failureReason,omitempty"`
}

type RunOutcome struct {
	WorkspaceID  string   `json:"workspaceId,omitempty"`
	State        string   `json:"state"`
	RunID        string   `json:"runId,omitempty"`
	ResultID     string   `json:"resultId,omitempty"`
	Accepted     bool     `json:"accepted"`
	Terminal     bool     `json:"terminal"`
	Summary      string   `json:"summary,omitempty"`
	Reason       string   `json:"reason,omitempty"`
	EvidenceRefs []string `json:"evidenceRefs,omitempty"`
	ArtifactRefs []string `json:"artifactRefs,omitempty"`
}

type DoDAssessmentItem struct {
	Item          string   `json:"item"`
	State         string   `json:"state"`
	EvidenceRefs  []string `json:"evidenceRefs,omitempty"`
	FailureReason string   `json:"failureReason,omitempty"`
	Confidence    float64  `json:"confidence"`
	SourceNote    string   `json:"sourceNote,omitempty"`
}

type DoDAssessment struct {
	Satisfied bool                `json:"satisfied"`
	Items     []DoDAssessmentItem `json:"items,omitempty"`
	Summary   string              `json:"summary,omitempty"`
}

type ResultSummary struct {
	GoalOutcome           string              `json:"goalOutcome"`
	DoDItems              []DoDAssessmentItem `json:"doDItems,omitempty"`
	ImplementationSummary string              `json:"implementationSummary,omitempty"`
	VerificationSummary   string              `json:"verificationSummary,omitempty"`
	EvidenceRefs          []string            `json:"evidenceRefs,omitempty"`
	ArtifactRefs          []string            `json:"artifactRefs,omitempty"`
	KnownLimitations      []string            `json:"knownLimitations,omitempty"`
	ResidualRisks         []string            `json:"residualRisks,omitempty"`
	NextActions           []string            `json:"nextActions,omitempty"`
}

type Result struct {
	ID            string        `json:"id"`
	WorkspaceID   string        `json:"workspaceId"`
	RunID         string        `json:"runId"`
	GoalID        string        `json:"goalId,omitempty"`
	DoDID         string        `json:"doDId,omitempty"`
	State         string        `json:"state"`
	Title         string        `json:"title,omitempty"`
	Summary       string        `json:"summary,omitempty"`
	SourceType    string        `json:"sourceType,omitempty"`
	SourceRef     string        `json:"sourceRef,omitempty"`
	EvidenceRefs  []string      `json:"evidenceRefs,omitempty"`
	ArtifactRefs  []string      `json:"artifactRefs,omitempty"`
	DoDAssessment DoDAssessment `json:"doDAssessment,omitempty"`
	SummaryDetail ResultSummary `json:"summaryDetail,omitempty"`
	CreatedAt     string        `json:"createdAt,omitempty"`
	UpdatedAt     string        `json:"updatedAt,omitempty"`
}

type Artifact struct {
	ID                 string         `json:"id"`
	WorkspaceID        string         `json:"workspaceId"`
	RunID              string         `json:"runId,omitempty"`
	ResultID           string         `json:"resultId,omitempty"`
	StepID             string         `json:"stepId,omitempty"`
	ToolCallID         string         `json:"toolCallId,omitempty"`
	PatchID            string         `json:"patchId,omitempty"`
	ValidationResultID string         `json:"validationResultId,omitempty"`
	ArtifactType       string         `json:"artifactType"`
	Category           string         `json:"category,omitempty"`
	Name               string         `json:"name,omitempty"`
	Summary            string         `json:"summary,omitempty"`
	URI                string         `json:"uri,omitempty"`
	ContentType        string         `json:"contentType,omitempty"`
	State              string         `json:"state"`
	EvidenceRefs       []string       `json:"evidenceRefs,omitempty"`
	Metadata           map[string]any `json:"metadata,omitempty"`
	CreatedAt          string         `json:"createdAt,omitempty"`
	UpdatedAt          string         `json:"updatedAt,omitempty"`
}

type RuntimeCheckpoint struct {
	ID                 string         `json:"id"`
	WorkspaceID        string         `json:"workspaceId"`
	RunID              string         `json:"runId"`
	CheckpointType     string         `json:"checkpointType"`
	CurrentStepID      string         `json:"currentStepId,omitempty"`
	RunStateSnapshot   string         `json:"runStateSnapshot"`
	BlockingTicketID   string         `json:"blockingTicketId,omitempty"`
	BlockingApprovalID string         `json:"blockingApprovalId,omitempty"`
	ThreadRef          string         `json:"threadRef,omitempty"`
	OccurredAt         string         `json:"occurredAt,omitempty"`
	RestorableUntil    string         `json:"restorableUntil,omitempty"`
	Data               map[string]any `json:"data,omitempty"`
}

// Snapshot is the full workspace/run snapshot returned by the read model.
type WorkspaceSnapshot struct {
	Workspace          Workspace           `json:"workspace"`
	Outcome            RunOutcome          `json:"outcome"`
	Runs               []Run               `json:"runs,omitempty"`
	Results            []Result            `json:"results,omitempty"`
	Artifacts          []Artifact          `json:"artifacts,omitempty"`
	Decisions          []Decision          `json:"decisions,omitempty"`
	Approvals          []Approval          `json:"approvals,omitempty"`
	Steps              []Step              `json:"steps,omitempty"`
	ToolCalls          []ToolCall          `json:"toolCalls,omitempty"`
	Patches            []Patch             `json:"patches,omitempty"`
	ValidationResults  []ValidationResult  `json:"validationResults,omitempty"`
	RuntimeCheckpoints []RuntimeCheckpoint `json:"runtimeCheckpoints,omitempty"`
	EventRecords       []EventRecord       `json:"eventRecords,omitempty"`
}

type RunSnapshot struct {
	Workspace          Workspace           `json:"workspace"`
	Run                Run                 `json:"run"`
	Outcome            RunOutcome          `json:"outcome"`
	Steps              []Step              `json:"steps,omitempty"`
	ToolCalls          []ToolCall          `json:"toolCalls,omitempty"`
	Patches            []Patch             `json:"patches,omitempty"`
	ValidationResults  []ValidationResult  `json:"validationResults,omitempty"`
	RuntimeCheckpoints []RuntimeCheckpoint `json:"runtimeCheckpoints,omitempty"`
	Results            []Result            `json:"results,omitempty"`
	Artifacts          []Artifact          `json:"artifacts,omitempty"`
	Decisions          []Decision          `json:"decisions,omitempty"`
	Approvals          []Approval          `json:"approvals,omitempty"`
	EventRecords       []EventRecord       `json:"eventRecords,omitempty"`
}

type Activation struct {
	Workspace Workspace         `json:"workspace"`
	Run       Run               `json:"run"`
	Snapshot  WorkspaceSnapshot `json:"snapshot"`
}

type RunObservability struct {
	Run      Run             `json:"run"`
	Outcome  RunOutcome      `json:"outcome"`
	Timeline []TimelineEntry `json:"timeline,omitempty"`
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
type LLMProviderConfig struct {
	ID                string   `json:"id"`
	Provider          string   `json:"provider"`
	BaseURL           string   `json:"baseUrl,omitempty"`
	APIKey            string   `json:"apiKey,omitempty"`
	Model             string   `json:"model,omitempty"`
	Timeout           string   `json:"timeout,omitempty"`
	Disabled          bool     `json:"disabled"`
	Ready             bool     `json:"ready"`
	Reason            string   `json:"reason,omitempty"`
	Default           bool     `json:"default"`
	APIKeyConfigured  bool     `json:"apiKeyConfigured"`
	ChatCompletions   bool     `json:"chatCompletions"`
	Streaming         bool     `json:"streaming"`
	NonChatOperations []string `json:"nonChatOperations,omitempty"`
}

type ServiceConfiguration struct {
	Server struct {
		Addr              string `json:"addr"`
		ReadHeaderTimeout string `json:"readHeaderTimeout"`
		ReadTimeout       string `json:"readTimeout"`
		WriteTimeout      string `json:"writeTimeout"`
		IdleTimeout       string `json:"idleTimeout"`
		ShutdownTimeout   string `json:"shutdownTimeout"`
		MaxConcurrentRuns int64  `json:"maxConcurrentRuns"`
	} `json:"server"`
	Repository struct {
		Backend string `json:"backend"`
		DataDir string `json:"dataDir"`
	} `json:"repository"`
	Runtime struct {
		DefaultSoulID string `json:"defaultSoulId,omitempty"`
	} `json:"runtime"`
	LLM struct {
		DefaultProviderID          string              `json:"defaultProviderId"`
		DefaultEmbeddingProviderID string              `json:"defaultEmbeddingProviderId,omitempty"`
		Providers                  []LLMProviderConfig `json:"providers"`
	} `json:"llm"`
}
