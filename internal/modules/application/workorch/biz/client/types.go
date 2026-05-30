package client

// Workspace resource types (matches workorchd HTTP JSON format with lowercase camelCase tags).
type Workspace struct {
	ID                   string            `json:"id"`
	Title                string            `json:"title"`
	Description          string            `json:"description,omitempty"`
	DeliverableDirectory string            `json:"deliverableDirectory,omitempty"`
	State                string            `json:"state"`
	ActiveRunID          string            `json:"activeRunId,omitempty"`
	TemplateID           string            `json:"templateId,omitempty"`
	ProfileID            string            `json:"profileId,omitempty"`
	DefaultSoulID        string            `json:"defaultSoulId,omitempty"`
	SoulBindings         map[string]string `json:"soulBindings,omitempty"`
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
	ID               string `json:"id"`
	WorkspaceID      string `json:"workspaceId"`
	RunID            string `json:"runId"`
	StepID           string `json:"stepId,omitempty"`
	ToolCallID       string `json:"toolCallId,omitempty"`
	ApprovalID       string `json:"approvalId,omitempty"`
	RevertApprovalID string `json:"revertApprovalId,omitempty"`
	PatchType        string `json:"patchType"`
	TargetRef        string `json:"targetRef,omitempty"`
	Summary          string `json:"summary,omitempty"`
	State            string `json:"state"`
	AddedLines       int    `json:"addedLines,omitempty"`
	DeletedLines     int    `json:"deletedLines,omitempty"`
	BeforeHash       string `json:"beforeHash,omitempty"`
	AfterHash        string `json:"afterHash,omitempty"`
	CreatedAt        string `json:"createdAt,omitempty"`
	AppliedAt        string `json:"appliedAt,omitempty"`
	RevertedAt       string `json:"revertedAt,omitempty"`
}

type ValidationResult struct {
	ID              string   `json:"id"`
	WorkspaceID     string   `json:"workspaceId"`
	RunID           string   `json:"runId"`
	StepID          string   `json:"stepId,omitempty"`
	ValidationKind  string   `json:"validationKind"`
	TargetRef       string   `json:"targetRef,omitempty"`
	State           string   `json:"state"`
	Summary         string   `json:"summary,omitempty"`
	EvidenceRef     string   `json:"evidenceRef,omitempty"`
	EvidenceQuality string   `json:"evidenceQuality,omitempty"`
	EvidenceReason  string   `json:"evidenceReason,omitempty"`
	ZeroWork        bool     `json:"zeroWork,omitempty"`
	EvidenceTags    []string `json:"evidenceTags,omitempty"`
	StartedAt       string   `json:"startedAt,omitempty"`
	EndedAt         string   `json:"endedAt,omitempty"`
}

// Decision & Approval types.
type Decision struct {
	ID             string                 `json:"id"`
	WorkspaceID    string                 `json:"workspaceId"`
	RunID          string                 `json:"runId"`
	StepID         string                 `json:"stepId,omitempty"`
	Reason         string                 `json:"reason"`
	DecisionKind   string                 `json:"decisionKind,omitempty"`
	Question       string                 `json:"question,omitempty"`
	Context        map[string]any         `json:"context,omitempty"`
	State          string                 `json:"state"`
	ResolutionID   string                 `json:"resolutionId,omitempty"`
	ResolutionType string                 `json:"resolutionType,omitempty"`
	AutoGovernance *AutoGovernanceSummary `json:"autoGovernance,omitempty"`
}

type Approval struct {
	ID             string                 `json:"id"`
	WorkspaceID    string                 `json:"workspaceId"`
	RunID          string                 `json:"runId"`
	StepID         string                 `json:"stepId,omitempty"`
	Reason         string                 `json:"reason"`
	Action         string                 `json:"action,omitempty"`
	Context        map[string]any         `json:"context,omitempty"`
	State          string                 `json:"state"`
	AutoGovernance *AutoGovernanceSummary `json:"autoGovernance,omitempty"`
}

type AutoGovernanceSummary struct {
	TraceID           string   `json:"traceId"`
	Action            string   `json:"action"`
	Status            string   `json:"status"`
	RiskLevel         string   `json:"riskLevel,omitempty"`
	PolicyMatched     string   `json:"policyMatched,omitempty"`
	PolicyBlocked     string   `json:"policyBlocked,omitempty"`
	Confidence        float64  `json:"confidence,omitempty"`
	Reason            string   `json:"reason,omitempty"`
	EvidenceRefs      []string `json:"evidenceRefs,omitempty"`
	PromptFingerprint string   `json:"promptFingerprint,omitempty"`
	OccurredAt        string   `json:"occurredAt,omitempty"`
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
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Description      string            `json:"description,omitempty"`
	GoalText         string            `json:"goalText,omitempty"`
	DoDItems         []string          `json:"doDItems,omitempty"`
	DefaultSoulID    string            `json:"defaultSoulId,omitempty"`
	SoulBindings     map[string]string `json:"soulBindings,omitempty"`
	EvidencePolicies []map[string]any  `json:"evidencePolicies,omitempty"`
}

// Event types.
type EventRecord struct {
	ID             string         `json:"id"`
	Version        string         `json:"version"`
	Topic          string         `json:"topic"`
	OccurredAt     string         `json:"occurredAt,omitempty"`
	SourceModule   string         `json:"sourceModule,omitempty"`
	SubjectType    string         `json:"subjectType,omitempty"`
	SubjectID      string         `json:"subjectId,omitempty"`
	WorkspaceID    string         `json:"workspaceId,omitempty"`
	RunID          string         `json:"runId,omitempty"`
	StepID         string         `json:"stepId,omitempty"`
	Payload        map[string]any `json:"payload,omitempty"`
	PayloadSummary map[string]any `json:"payloadSummary,omitempty"`
	PayloadRef     string         `json:"payloadRef,omitempty"`
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

type RecoveryPlan struct {
	Mode               string `json:"mode"`
	NextAction         string `json:"nextAction,omitempty"`
	RunID              string `json:"runId,omitempty"`
	StepID             string `json:"stepId,omitempty"`
	BlockingTicketID   string `json:"blockingTicketId,omitempty"`
	BlockingApprovalID string `json:"blockingApprovalId,omitempty"`
	CheckpointID       string `json:"checkpointId,omitempty"`
	FailureClass       string `json:"failureClass,omitempty"`
	FailureSignature   string `json:"failureSignature,omitempty"`
	Reason             string `json:"reason,omitempty"`
}

type RepairLoop struct {
	State                    string         `json:"state"`
	AttemptCount             int            `json:"attemptCount"`
	LastAttempt              int            `json:"lastAttempt,omitempty"`
	LastRepairStepID         string         `json:"lastRepairStepId,omitempty"`
	LastFailureStepID        string         `json:"lastFailureStepId,omitempty"`
	LastFailureClass         string         `json:"lastFailureClass,omitempty"`
	LastFailureSignature     string         `json:"lastFailureSignature,omitempty"`
	LastFailureTargetScope   string         `json:"lastFailureTargetScope,omitempty"`
	LastFailureAt            string         `json:"lastFailureAt,omitempty"`
	StructuredFailureSignals map[string]any `json:"structuredFailureSignals,omitempty"`
	RepairInputCompaction    map[string]any `json:"repairInputCompaction,omitempty"`
	LastRepairInputBytes     int            `json:"lastRepairInputBytes,omitempty"`
	RepeatedFailureReason    string         `json:"repeatedFailureReason,omitempty"`
	NextAction               string         `json:"nextAction,omitempty"`
	Escalated                bool           `json:"escalated"`
	EscalationReason         string         `json:"escalationReason,omitempty"`
	EscalationTicketID       string         `json:"escalationTicketId,omitempty"`
	EvidenceRefs             []string       `json:"evidenceRefs,omitempty"`
}

type TaskProgress struct {
	WorkspaceID        string       `json:"workspaceId"`
	RunID              string       `json:"runId,omitempty"`
	State              string       `json:"state"`
	Summary            string       `json:"summary,omitempty"`
	BlockingTicketID   string       `json:"blockingTicketId,omitempty"`
	BlockingApprovalID string       `json:"blockingApprovalId,omitempty"`
	Recovery           RecoveryPlan `json:"recovery"`
	RepairLoop         RepairLoop   `json:"repairLoop"`
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
	Satisfied            bool                `json:"satisfied"`
	Items                []DoDAssessmentItem `json:"items,omitempty"`
	GoalCoverage         *DoDCoverage        `json:"goalCoverage,omitempty"`
	AcceptanceDimensions *DoDCoverage        `json:"acceptanceDimensions,omitempty"`
	DeliverableCoverage  *DoDCoverage        `json:"deliverableCoverage,omitempty"`
	Summary              string              `json:"summary,omitempty"`
}

type DoDCoverage struct {
	Satisfied bool              `json:"satisfied"`
	Items     []DoDCoverageItem `json:"items,omitempty"`
	Summary   string            `json:"summary,omitempty"`
}

type DoDCoverageItem struct {
	ID           string   `json:"id,omitempty"`
	Deliverable  string   `json:"deliverable,omitempty"`
	Name         string   `json:"name,omitempty"`
	Path         string   `json:"path,omitempty"`
	Goal         string   `json:"goal,omitempty"`
	GoalItem     string   `json:"goalItem,omitempty"`
	Item         string   `json:"item,omitempty"`
	Requirement  string   `json:"requirement,omitempty"`
	State        string   `json:"state"`
	EvidenceRefs []string `json:"evidenceRefs,omitempty"`
	ArtifactRefs []string `json:"artifactRefs,omitempty"`
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

type ArtifactContinuation struct {
	ID                string         `json:"id"`
	WorkspaceID       string         `json:"workspaceId"`
	RunID             string         `json:"runId"`
	RootStepID        string         `json:"rootStepId,omitempty"`
	ArtifactID        string         `json:"artifactId"`
	TargetRef         string         `json:"targetRef"`
	Operation         string         `json:"operation,omitempty"`
	State             string         `json:"state"`
	ExpectedSequence  int            `json:"expectedSequence,omitempty"`
	ReceivedSequences []int          `json:"receivedSequences,omitempty"`
	FinalReceived     bool           `json:"finalReceived,omitempty"`
	BytesWritten      int64          `json:"bytesWritten,omitempty"`
	RollingHash       string         `json:"rollingHash,omitempty"`
	LastChunkHash     string         `json:"lastChunkHash,omitempty"`
	CoverageTargets   []string       `json:"coverageTargets,omitempty"`
	EvidenceRefs      []string       `json:"evidenceRefs,omitempty"`
	FailureReason     string         `json:"failureReason,omitempty"`
	CreatedAt         string         `json:"createdAt,omitempty"`
	UpdatedAt         string         `json:"updatedAt,omitempty"`
	Progress          map[string]any `json:"progress,omitempty"`
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

type MemoryRecallHit struct {
	ID              string  `json:"id"`
	Summary         string  `json:"summary,omitempty"`
	VectorScore     float64 `json:"vectorScore,omitempty"`
	VectorDimension int     `json:"vectorDimension,omitempty"`
}

type MemoryRecallTrace struct {
	ID                  string            `json:"id"`
	WorkspaceID         string            `json:"workspaceId"`
	RunID               string            `json:"runId"`
	StepID              string            `json:"stepId,omitempty"`
	GraphNodeID         string            `json:"graphNodeId,omitempty"`
	Stage               string            `json:"stage,omitempty"`
	Source              string            `json:"source"`
	Policy              string            `json:"policy,omitempty"`
	Status              string            `json:"status"`
	ProviderID          string            `json:"providerId,omitempty"`
	Operation           string            `json:"operation,omitempty"`
	QueryTextPreview    string            `json:"queryTextPreview,omitempty"`
	QueryTextSha256     string            `json:"queryTextSha256,omitempty"`
	MinScore            float64           `json:"minScore,omitempty"`
	Limit               int               `json:"limit,omitempty"`
	EmbeddingModel      string            `json:"embeddingModel,omitempty"`
	VectorDimension     int               `json:"vectorDimension,omitempty"`
	RetrievedMemoryIDs  []string          `json:"retrievedMemoryIds,omitempty"`
	RetrievedMemoryHits []MemoryRecallHit `json:"retrievedMemoryHits,omitempty"`
	Reason              string            `json:"reason,omitempty"`
	OccurredAt          string            `json:"occurredAt,omitempty"`
	EvidenceRef         string            `json:"evidenceRef,omitempty"`
}

// Snapshot is the full workspace/run snapshot returned by the read model.
type WorkspaceSnapshot struct {
	Workspace             Workspace              `json:"workspace"`
	Outcome               RunOutcome             `json:"outcome"`
	TaskProgress          TaskProgress           `json:"taskProgress"`
	Goals                 []Goal                 `json:"goals,omitempty"`
	DoDs                  []DoD                  `json:"dods,omitempty"`
	Runs                  []Run                  `json:"runs,omitempty"`
	Results               []Result               `json:"results,omitempty"`
	Artifacts             []Artifact             `json:"artifacts,omitempty"`
	ArtifactContinuations []ArtifactContinuation `json:"artifactContinuations,omitempty"`
	Decisions             []Decision             `json:"decisions,omitempty"`
	Approvals             []Approval             `json:"approvals,omitempty"`
	Steps                 []Step                 `json:"steps,omitempty"`
	ToolCalls             []ToolCall             `json:"toolCalls,omitempty"`
	Patches               []Patch                `json:"patches,omitempty"`
	ValidationResults     []ValidationResult     `json:"validationResults,omitempty"`
	RuntimeCheckpoints    []RuntimeCheckpoint    `json:"runtimeCheckpoints,omitempty"`
	MemoryRecallTraces    []MemoryRecallTrace    `json:"memoryRecallTraces,omitempty"`
	LLMUsageTraces        []LLMUsageTrace        `json:"llmUsageTraces,omitempty"`
	LLMUsageSummary       LLMUsageSummary        `json:"llmUsageSummary"`
	EventRecords          []EventRecord          `json:"eventRecords,omitempty"`
}

type RunSnapshot struct {
	Workspace               Workspace              `json:"workspace"`
	Run                     Run                    `json:"run"`
	Outcome                 RunOutcome             `json:"outcome"`
	TaskProgress            TaskProgress           `json:"taskProgress"`
	RepairLoop              RepairLoop             `json:"repairLoop"`
	Steps                   []Step                 `json:"steps,omitempty"`
	ToolCalls               []ToolCall             `json:"toolCalls,omitempty"`
	Patches                 []Patch                `json:"patches,omitempty"`
	ValidationResults       []ValidationResult     `json:"validationResults,omitempty"`
	RuntimeCheckpoints      []RuntimeCheckpoint    `json:"runtimeCheckpoints,omitempty"`
	Results                 []Result               `json:"results,omitempty"`
	Artifacts               []Artifact             `json:"artifacts,omitempty"`
	ArtifactContinuations   []ArtifactContinuation `json:"artifactContinuations,omitempty"`
	Decisions               []Decision             `json:"decisions,omitempty"`
	Approvals               []Approval             `json:"approvals,omitempty"`
	MemoryRecallTraces      []MemoryRecallTrace    `json:"memoryRecallTraces,omitempty"`
	LLMUsageTraces          []LLMUsageTrace        `json:"llmUsageTraces,omitempty"`
	LLMUsageSummary         LLMUsageSummary        `json:"llmUsageSummary"`
	CoverageWorkItems       []map[string]any       `json:"coverageWorkItems,omitempty"`
	CoverageWorkItemContext map[string]any         `json:"coverageWorkItemContext,omitempty"`
	EventRecords            []EventRecord          `json:"eventRecords,omitempty"`
}

type Activation struct {
	Workspace Workspace         `json:"workspace"`
	Run       Run               `json:"run"`
	Snapshot  WorkspaceSnapshot `json:"snapshot"`
}

type RunObservability struct {
	Run             Run             `json:"run"`
	Outcome         RunOutcome      `json:"outcome"`
	TaskProgress    TaskProgress    `json:"taskProgress"`
	RepairLoop      RepairLoop      `json:"repairLoop"`
	LLMUsageTraces  []LLMUsageTrace `json:"llmUsageTraces,omitempty"`
	LLMUsageSummary LLMUsageSummary `json:"llmUsageSummary"`
	Timeline        []TimelineEntry `json:"timeline,omitempty"`
}

type LLMUsageTrace struct {
	ID            string         `json:"id"`
	WorkspaceID   string         `json:"workspaceId"`
	RunID         string         `json:"runId"`
	StepID        string         `json:"stepId,omitempty"`
	ProviderID    string         `json:"providerId,omitempty"`
	Provider      string         `json:"provider,omitempty"`
	Model         string         `json:"model,omitempty"`
	Operation     string         `json:"operation,omitempty"`
	InputTokens   int            `json:"inputTokens,omitempty"`
	OutputTokens  int            `json:"outputTokens,omitempty"`
	TotalTokens   int            `json:"totalTokens,omitempty"`
	Estimated     bool           `json:"estimated"`
	UsageSource   string         `json:"usageSource,omitempty"`
	RequestBytes  int            `json:"requestBytes,omitempty"`
	ResponseBytes int            `json:"responseBytes,omitempty"`
	Streaming     bool           `json:"streaming"`
	ContextBudget map[string]any `json:"contextBudget,omitempty"`
	CacheUsage    map[string]any `json:"cacheUsage,omitempty"`
	StartedAt     string         `json:"startedAt,omitempty"`
	CompletedAt   string         `json:"completedAt,omitempty"`
	DurationMs    int            `json:"durationMs,omitempty"`
	Status        string         `json:"status,omitempty"`
	FailureClass  string         `json:"failureClass,omitempty"`
	OccurredAt    string         `json:"occurredAt,omitempty"`
	EvidenceRef   string         `json:"evidenceRef,omitempty"`
}

type LLMUsageSummary struct {
	EntryTotal              int                              `json:"entryTotal"`
	InputTokens             int                              `json:"inputTokens"`
	OutputTokens            int                              `json:"outputTokens"`
	TotalTokens             int                              `json:"totalTokens"`
	EstimatedEntries        int                              `json:"estimatedEntries"`
	ProviderNativeEntries   int                              `json:"providerNativeEntries"`
	FailedEntries           int                              `json:"failedEntries"`
	CacheReadTokens         int                              `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens        int                              `json:"cacheWriteTokens,omitempty"`
	CacheCreationTokens     int                              `json:"cacheCreationTokens,omitempty"`
	CacheableInputTokens    int                              `json:"cacheableInputTokens,omitempty"`
	NonCacheableInputTokens int                              `json:"nonCacheableInputTokens,omitempty"`
	CacheReadInputRatio     float64                          `json:"cacheReadInputRatio,omitempty"`
	CacheableInputRatio     float64                          `json:"cacheableInputRatio,omitempty"`
	NonCacheableInputRatio  float64                          `json:"nonCacheableInputRatio,omitempty"`
	CacheEstimatedEntries   int                              `json:"cacheEstimatedEntries,omitempty"`
	CacheProviderEntries    int                              `json:"cacheProviderEntries,omitempty"`
	ProviderModelBreakdown  []LLMUsageProviderModelBreakdown `json:"providerModelBreakdown,omitempty"`
}

type LLMUsageProviderModelBreakdown struct {
	ProviderID          string  `json:"providerId,omitempty"`
	Provider            string  `json:"provider,omitempty"`
	Model               string  `json:"model,omitempty"`
	EntryTotal          int     `json:"entryTotal"`
	InputTokens         int     `json:"inputTokens"`
	OutputTokens        int     `json:"outputTokens"`
	TotalTokens         int     `json:"totalTokens"`
	CacheReadTokens     int     `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens    int     `json:"cacheWriteTokens,omitempty"`
	CacheReadInputRatio float64 `json:"cacheReadInputRatio,omitempty"`
	EstimatedEntries    int     `json:"estimatedEntries"`
	FailedEntries       int     `json:"failedEntries"`
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
	ID                string                   `json:"id"`
	Provider          string                   `json:"provider"`
	BaseURL           string                   `json:"baseUrl,omitempty"`
	APIKey            string                   `json:"apiKey,omitempty"`
	Model             string                   `json:"model,omitempty"`
	Timeout           string                   `json:"timeout,omitempty"`
	Disabled          bool                     `json:"disabled"`
	Ready             bool                     `json:"ready"`
	Reason            string                   `json:"reason,omitempty"`
	Default           bool                     `json:"default"`
	APIKeyConfigured  bool                     `json:"apiKeyConfigured"`
	ChatCompletions   bool                     `json:"chatCompletions"`
	Streaming         bool                     `json:"streaming"`
	NonChatOperations []string                 `json:"nonChatOperations,omitempty"`
	FailoverEnabled   bool                     `json:"failoverEnabled"`
	FailoverProviders []string                 `json:"failoverProviders,omitempty"`
	InferenceProfile  ProviderInferenceProfile `json:"inferenceProfile,omitempty"`
	ContextProfile    ProviderContextProfile   `json:"contextProfile,omitempty"`
}

type ProviderInferenceProfile struct {
	Temperature          *float64 `json:"temperature,omitempty"`
	MaxOutputTokens      int      `json:"maxOutputTokens,omitempty"`
	ReasoningEffort      string   `json:"reasoningEffort,omitempty"`
	ThinkingBudgetTokens int      `json:"thinkingBudgetTokens,omitempty"`
}

type LLMModelCapabilityConfig struct {
	ID                  string `json:"id"`
	Provider            string `json:"provider,omitempty"`
	ModelPattern        string `json:"modelPattern"`
	ContextWindowTokens int    `json:"contextWindowTokens"`
}

type ProviderContextProfile struct {
	ProfileMode             string  `json:"profileMode,omitempty"`
	CapabilitySource        string  `json:"capabilitySource,omitempty"`
	ContextWindowTokens     int     `json:"contextWindowTokens,omitempty"`
	MaxInputTokens          int     `json:"maxInputTokens,omitempty"`
	ReservedOutputTokens    int     `json:"reservedOutputTokens,omitempty"`
	SafetyRatio             float64 `json:"safetyRatio,omitempty"`
	ContextUtilizationRatio float64 `json:"contextUtilizationRatio,omitempty"`
	BytesHardLimit          int     `json:"bytesHardLimit,omitempty"`
	MaxRequestBytes         int     `json:"maxRequestBytes,omitempty"`
	EstimatedTokensPerByte  float64 `json:"estimatedTokensPerByte,omitempty"`
}

type ServiceConfiguration struct {
	Server struct {
		Addr                  string `json:"addr"`
		ReadHeaderTimeout     string `json:"readHeaderTimeout"`
		ReadTimeout           string `json:"readTimeout"`
		WriteTimeout          string `json:"writeTimeout"`
		IdleTimeout           string `json:"idleTimeout"`
		ShutdownTimeout       string `json:"shutdownTimeout"`
		MaxConcurrentRuns     int64  `json:"maxConcurrentRuns"`
		DefaultDialogLanguage string `json:"defaultDialogLanguage,omitempty"`
	} `json:"server"`
	Repository struct {
		Backend string `json:"backend"`
		DataDir string `json:"dataDir"`
	} `json:"repository"`
	Runtime struct {
		DefaultSoulID            string `json:"defaultSoulId,omitempty"`
		MaxRuntimeRepairAttempts int    `json:"maxRuntimeRepairAttempts"`
		MaxProviderRetryAttempts int    `json:"maxProviderRetryAttempts"`
		ShellCommandTimeout      string `json:"shellCommandTimeout"`
		LLMOutput                struct {
			MaxPatchContentBytes               int `json:"maxPatchContentBytes"`
			MaxGraphBytes                      int `json:"maxGraphBytes"`
			MaxGraphFileWrites                 int `json:"maxGraphFileWrites"`
			SchemaInvalidLargePreviewThreshold int `json:"schemaInvalidLargePreviewThreshold"`
			MaxArtifactChunkBytes              int `json:"maxArtifactChunkBytes"`
			PreviousChunkTailBytes             int `json:"previousChunkTailBytes"`
			MaxChunkRetryAttempts              int `json:"maxChunkRetryAttempts"`
			MaxArtifactPlanAttempts            int `json:"maxArtifactPlanAttempts"`
			MaxArtifactReplanAttempts          int `json:"maxArtifactReplanAttempts"`
		} `json:"llmOutput"`
	} `json:"runtime"`
	LLM struct {
		DefaultProviderID          string                     `json:"defaultProviderId"`
		DefaultEmbeddingProviderID string                     `json:"defaultEmbeddingProviderId,omitempty"`
		ModelCapabilities          []LLMModelCapabilityConfig `json:"modelCapabilities,omitempty"`
		Providers                  []LLMProviderConfig        `json:"providers"`
	} `json:"llm"`
	WebSearch struct {
		DefaultProviderID string `json:"defaultProviderId,omitempty"`
		Providers         []struct {
			ID               string            `json:"id"`
			Provider         string            `json:"provider"`
			SearchEndpoint   string            `json:"searchEndpoint,omitempty"`
			FetchEndpoint    string            `json:"fetchEndpoint,omitempty"`
			Headers          map[string]string `json:"headers,omitempty"`
			Timeout          string            `json:"timeout,omitempty"`
			MaxResults       int               `json:"maxResults,omitempty"`
			MaxFetchBytes    int64             `json:"maxFetchBytes,omitempty"`
			MaxRedirects     int               `json:"maxRedirects,omitempty"`
			AllowDomains     []string          `json:"allowDomains,omitempty"`
			DenyDomains      []string          `json:"denyDomains,omitempty"`
			Disabled         bool              `json:"disabled"`
			Ready            bool              `json:"ready"`
			Reason           string            `json:"reason,omitempty"`
			Default          bool              `json:"default"`
			APIKeyConfigured bool              `json:"apiKeyConfigured"`
		} `json:"providers"`
	} `json:"webSearch"`
	Governance struct {
		AutoTicket struct {
			Enabled       bool    `json:"enabled"`
			MinConfidence float64 `json:"minConfidence"`
			MaxPerRun     int     `json:"maxPerRun"`
		} `json:"autoTicket"`
		AutoApproval struct {
			Enabled             bool     `json:"enabled"`
			MinConfidence       float64  `json:"minConfidence"`
			MaxPerRun           int      `json:"maxPerRun"`
			AllowReadOnly       bool     `json:"allowReadOnly"`
			AllowWorkspaceWrite bool     `json:"allowWorkspaceWrite"`
			RequireHumanFor     []string `json:"requireHumanFor,omitempty"`
		} `json:"autoApproval"`
		Budget struct {
			MaxConsecutiveFailures int `json:"maxConsecutiveFailures"`
			MaxRepeatedSamePrompt  int `json:"maxRepeatedSamePrompt"`
		} `json:"budget"`
	} `json:"governance"`
}
