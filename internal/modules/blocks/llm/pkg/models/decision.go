package models

type ProviderRequest struct {
	WorkspaceID         string                 `json:"workspaceId"`
	RunID               string                 `json:"runId"`
	StepID              string                 `json:"stepId"`
	Kind                string                 `json:"kind"`
	Model               string                 `json:"model"`
	SystemLocale        string                 `json:"systemLocale,omitempty"`
	DialogLanguage      string                 `json:"dialogLanguage,omitempty"`
	Input               map[string]any         `json:"input,omitempty"`
	Schema              string                 `json:"schema"`
	RuntimeCapabilities RuntimeCapabilities    `json:"runtimeCapabilities,omitempty"`
	TraceRefs           map[string]string      `json:"traceRefs,omitempty"`
	Config              RedactedProviderConfig `json:"config"`
	PromptContext       *PromptContext         `json:"promptContext,omitempty"`
	ContextBudget       *ProviderContextBudget `json:"contextBudget,omitempty"`
	Repair              *ProviderRepairRequest `json:"repair,omitempty"`
}

type PromptContext struct {
	SchemaVersion string                 `json:"schemaVersion,omitempty"`
	Sections      []PromptSection        `json:"sections,omitempty"`
	Turns         []PromptTurn           `json:"turns,omitempty"`
	Normalization *NormalizationManifest `json:"normalization,omitempty"`
}

type PromptSection struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name,omitempty"`
	Role               string   `json:"role,omitempty"`
	Policy             string   `json:"policy,omitempty"`
	Cacheability       string   `json:"cacheability,omitempty"`
	Hash               string   `json:"hash,omitempty"`
	SourceRefs         []string `json:"sourceRefs,omitempty"`
	DefinitionRevision string   `json:"definitionRevision,omitempty"`
	DisplayOrder       int      `json:"displayOrder,omitempty"`
	Content            string   `json:"content,omitempty"`
	Normalized         bool     `json:"normalized,omitempty"`
	Protected          bool     `json:"protected,omitempty"`
}

type PromptTurn struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	SectionID string `json:"sectionId,omitempty"`
}

type NormalizationManifest struct {
	NormalizedSections   []string `json:"normalizedSections,omitempty"`
	ProtectedSections    []string `json:"protectedSections,omitempty"`
	EstimatedTokensSaved int      `json:"estimatedTokensSaved,omitempty"`
	SemanticLoss         bool     `json:"semanticLoss"`
}

type ProviderContextBudget struct {
	MaxBytes              int                    `json:"maxBytes"`
	ActualBytes           int                    `json:"actualBytes"`
	MaxTokens             int                    `json:"maxTokens,omitempty"`
	EstimatedInputTokens  int                    `json:"estimatedInputTokens,omitempty"`
	EstimatedOutputTokens int                    `json:"estimatedOutputTokens,omitempty"`
	ActualTokens          int                    `json:"actualTokens,omitempty"`
	ShrinkLevel           int                    `json:"shrinkLevel,omitempty"`
	Truncated             bool                   `json:"truncated"`
	Rules                 []string               `json:"rules,omitempty"`
	SourceRefs            []string               `json:"sourceRefs,omitempty"`
	IncludedSections      []string               `json:"includedSections,omitempty"`
	NormalizedSections    []string               `json:"normalizedSections,omitempty"`
	CompressedSections    []string               `json:"compressedSections,omitempty"`
	OmittedSections       []string               `json:"omittedSections,omitempty"`
	GoalDoDCompressed     bool                   `json:"goalDoDCompressed,omitempty"`
	OverBudget            bool                   `json:"overBudget,omitempty"`
	P0Overflow            bool                   `json:"p0Overflow,omitempty"`
	ProfileMode           string                 `json:"profileMode,omitempty"`
	CapabilitySource      string                 `json:"capabilitySource,omitempty"`
	ContextUtilization    float64                `json:"contextUtilizationRatio,omitempty"`
	MaxRequestBytes       int                    `json:"maxRequestBytes,omitempty"`
	StageBudgetRatio      float64                `json:"stageBudgetRatio,omitempty"`
	StageBudgetSource     string                 `json:"stageBudgetSource,omitempty"`
	TracePolicy           string                 `json:"tracePolicy,omitempty"`
	PromptCaptured        bool                   `json:"promptCaptured"`
	Estimator             string                 `json:"estimator,omitempty"`
	RedactionApplied      bool                   `json:"redactionApplied,omitempty"`
	RedactedPaths         []string               `json:"redactedPaths,omitempty"`
	ProtectedFields       []string               `json:"protectedFields,omitempty"`
	IntegrityWarnings     []string               `json:"integrityWarnings,omitempty"`
	ContextGateAction     string                 `json:"contextGateAction,omitempty"`
	ContextGateReason     string                 `json:"contextGateReason,omitempty"`
	CachePlan             *ProviderCachePlan     `json:"cachePlan,omitempty"`
	Profile               ProviderContextProfile `json:"profile,omitempty"`
}

type ProviderCachePlan struct {
	SchemaVersion           string                       `json:"schemaVersion,omitempty"`
	Strategy                string                       `json:"strategy,omitempty"`
	CacheKey                string                       `json:"cacheKey,omitempty"`
	StablePrefixHash        string                       `json:"stablePrefixHash,omitempty"`
	WorkspaceDefinitionHash string                       `json:"workspaceDefinitionHash,omitempty"`
	CapabilityContractHash  string                       `json:"capabilityContractHash,omitempty"`
	StageBundleHash         string                       `json:"stageBundleHash,omitempty"`
	VolatileTailHash        string                       `json:"volatileTailHash,omitempty"`
	LargeRefsHash           string                       `json:"largeRefsHash,omitempty"`
	SectionHashes           []ProviderCacheSectionHash   `json:"sectionHashes,omitempty"`
	CacheBreakers           []ProviderCacheBreaker       `json:"cacheBreakers,omitempty"`
	Canonicalization        ProviderCanonicalizationPlan `json:"canonicalization,omitempty"`
	ObservedRequestShape    ProviderObservedRequestShape `json:"observedRequestShape,omitempty"`
	AdapterHint             ProviderAdapterCacheHint     `json:"adapterHint,omitempty"`
}

type ProviderCacheSectionHash struct {
	SectionID          string   `json:"sectionId"`
	Name               string   `json:"name,omitempty"`
	Cacheability       string   `json:"cacheability,omitempty"`
	Hash               string   `json:"hash,omitempty"`
	SourceRefs         []string `json:"sourceRefs,omitempty"`
	DefinitionRevision string   `json:"definitionRevision,omitempty"`
}

type ProviderCacheBreaker struct {
	SectionID string `json:"sectionId,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

type ProviderCanonicalizationPlan struct {
	SectionOrderPolicy string   `json:"sectionOrderPolicy,omitempty"`
	SectionOrder       []string `json:"sectionOrder,omitempty"`
	OrderedDataPolicy  string   `json:"orderedDataPolicy,omitempty"`
	MapKeyPolicy       string   `json:"mapKeyPolicy,omitempty"`
	ProviderMapping    string   `json:"providerMapping,omitempty"`
}

type ProviderObservedRequestShape struct {
	ProviderFacingMode  string `json:"providerFacingMode,omitempty"`
	TurnsHaveSectionIDs bool   `json:"turnsHaveSectionIds,omitempty"`
	TypedEventTape      bool   `json:"typedEventTape,omitempty"`
}

type ProviderAdapterCacheHint struct {
	Enabled          bool     `json:"enabled"`
	Mode             string   `json:"mode,omitempty"`
	ProviderMapping  string   `json:"providerMapping,omitempty"`
	CacheablePrefix  []string `json:"cacheablePrefix,omitempty"`
	VolatileSections []string `json:"volatileSections,omitempty"`
	CacheBreakers    []string `json:"cacheBreakers,omitempty"`
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
	InputTokenPrice         float64 `json:"inputTokenPrice,omitempty"`
	OutputTokenPrice        float64 `json:"outputTokenPrice,omitempty"`
	Currency                string  `json:"currency,omitempty"`
}

type ProviderUsage struct {
	InputTokens  int `json:"inputTokens,omitempty"`
	OutputTokens int `json:"outputTokens,omitempty"`
	TotalTokens  int `json:"totalTokens,omitempty"`
}

type ProviderCompletionResult struct {
	Payload []byte         `json:"payload,omitempty"`
	Usage   *ProviderUsage `json:"usage,omitempty"`
}

type ProviderRepairRequest struct {
	Attempt        int    `json:"attempt"`
	OriginalOutput string `json:"originalOutput"`
	ParseError     string `json:"parseError"`
}

type RuntimeCapabilities struct {
	BlockKinds  []RuntimeBlockCapability `json:"blockKinds,omitempty"`
	ServiceAPIs []ServiceAPICapability   `json:"serviceAPIs,omitempty"`
	ShellPolicy ShellRuntimePolicy       `json:"shellPolicy,omitempty"`
}

type RuntimeBlockCapability struct {
	Kind        string         `json:"kind"`
	Description string         `json:"description,omitempty"`
	Input       map[string]any `json:"input,omitempty"`
}

type ServiceAPICapability struct {
	Method       string `json:"method"`
	PathTemplate string `json:"pathTemplate"`
	Description  string `json:"description,omitempty"`
	ReadOnly     bool   `json:"readOnly,omitempty"`
	AllowQuery   bool   `json:"allowQuery,omitempty"`
}

type ShellRuntimePolicy struct {
	Enabled                 bool     `json:"enabled"`
	LocalServiceHTTP        string   `json:"localServiceHttp,omitempty"`
	UnavailableCommands     []string `json:"unavailableCommands,omitempty"`
	AllowedLocalServiceHost []string `json:"allowedLocalServiceHost,omitempty"`
}

type LLMExecutionDecision struct {
	ThoughtSummary   string               `json:"thoughtSummary,omitempty"`
	Plan             []string             `json:"plan,omitempty"`
	ExecutionGraph   *BlockExecutionGraph `json:"executionGraph,omitempty"`
	NextAction       *LLMAction           `json:"nextAction,omitempty"`
	RequiredDecision *RequiredDecision    `json:"requiredDecision,omitempty"`
	RequiredApproval *RequiredApproval    `json:"requiredApproval,omitempty"`
	PatchRequests    []PatchRequest       `json:"patchRequests,omitempty"`
	MCPToolCalls     []MCPToolCallRequest `json:"mcpToolCalls,omitempty"`
	ShellCommands    []ShellCommand       `json:"shellCommands,omitempty"`
	MemoryWrites     []MemoryWriteRequest `json:"memoryWrites,omitempty"`
	DoDAssessment    *DoDAssessment       `json:"dodAssessment,omitempty"`
	CompletionState  string               `json:"completionState"`
}

type BlockExecutionGraph struct {
	Nodes []BlockExecutionNode `json:"nodes"`
}

type BlockExecutionNode struct {
	ID        string         `json:"id"`
	Kind      string         `json:"kind"`
	DependsOn []string       `json:"dependsOn,omitempty"`
	Branch    string         `json:"branch,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
}

type LLMAction struct {
	Kind    string         `json:"kind"`
	Summary string         `json:"summary,omitempty"`
	Input   map[string]any `json:"input,omitempty"`
}

type RequiredDecision struct {
	Reason       string         `json:"reason"`
	DecisionKind string         `json:"decisionKind,omitempty"`
	Question     string         `json:"question,omitempty"`
	Context      map[string]any `json:"context,omitempty"`
}

type RequiredApproval struct {
	Reason  string         `json:"reason"`
	Action  string         `json:"action,omitempty"`
	Context map[string]any `json:"context,omitempty"`
}

type PatchRequest struct {
	PatchType       string `json:"patchType,omitempty"`
	Operation       string `json:"operation,omitempty"`
	TargetRef       string `json:"targetRef"`
	Content         string `json:"content,omitempty"`
	Diff            string `json:"diff,omitempty"`
	Summary         string `json:"summary,omitempty"`
	BaseHash        string `json:"baseHash,omitempty"`
	ApprovalID      string `json:"approvalId,omitempty"`
	CoverageTargets any    `json:"coverageTargets,omitempty"`
}

type MCPToolCallRequest struct {
	ServerID string         `json:"serverId"`
	Tool     string         `json:"tool"`
	Input    map[string]any `json:"input,omitempty"`
	Summary  string         `json:"summary,omitempty"`
}

type ShellCommand struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	Summary string   `json:"summary,omitempty"`
}

type MemoryWriteRequest struct {
	MemoryType string   `json:"memoryType"`
	Summary    string   `json:"summary"`
	Content    string   `json:"content,omitempty"`
	Tags       []string `json:"tags,omitempty"`
}

type DoDAssessment struct {
	Satisfied             bool                `json:"satisfied"`
	Items                 []DoDAssessmentItem `json:"items,omitempty"`
	GoalCoverage          *DoDCoverage        `json:"goalCoverage,omitempty"`
	AcceptanceDimensions  *DoDCoverage        `json:"acceptanceDimensions,omitempty"`
	DeliverableCoverage   *DoDCoverage        `json:"deliverableCoverage,omitempty"`
	Summary               string              `json:"summary,omitempty"`
	GoalOutcome           string              `json:"goalOutcome,omitempty"`
	ImplementationSummary string              `json:"implementationSummary,omitempty"`
	VerificationSummary   string              `json:"verificationSummary,omitempty"`
	KnownLimitations      []string            `json:"knownLimitations,omitempty"`
	ResidualRisks         []string            `json:"residualRisks,omitempty"`
	NextActions           []string            `json:"nextActions,omitempty"`
}

type DoDAssessmentItem struct {
	Item         string   `json:"item"`
	State        string   `json:"state"`
	EvidenceRefs []string `json:"evidenceRefs,omitempty"`
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
	Reason       string   `json:"reason,omitempty"`
	Summary      string   `json:"summary,omitempty"`
}
