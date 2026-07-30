package events

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

const (
	PayloadKeyTitle                  = "title"
	PayloadKeyDescription            = "description"
	PayloadKeyDeliverableDirectory   = "deliverableDirectory"
	PayloadKeyInternalDirectory      = "internalDirectory"
	PayloadKeyGoalID                 = "goalId"
	PayloadKeyDoDID                  = "doDId"
	PayloadKeyText                   = "text"
	PayloadKeyItems                  = "items"
	PayloadKeyState                  = "state"
	PayloadKeyActiveRunID            = "activeRunId"
	PayloadKeyBlockingTicketID       = "blockingTicketId"
	PayloadKeyBlockingApprovalID     = "blockingApprovalId"
	PayloadKeyFailureReason          = "failureReason"
	PayloadKeyDefinitionRevision     = "definitionRevision"
	PayloadKeyPreviousDefinitionHash = "previousDefinitionHash"
	PayloadKeyPreviousGoalID         = "previousGoalId"
	PayloadKeyPreviousDoDID          = "previousDoDId"
	PayloadKeyLineageState           = "lineageState"
	PayloadKeyKind                   = "kind"
	PayloadKeyCategory               = "category"
	PayloadKeyInput                  = "input"
	PayloadKeyOutput                 = "output"
	PayloadKeyReason                 = "reason"
	PayloadKeyAction                 = "action"
	PayloadKeyIntent                 = "intent"
	PayloadKeyRequestID              = "requestId"
	PayloadKeySource                 = "source"
	PayloadKeyContext                = "context"
	PayloadKeyResolutionID           = "resolutionId"
	PayloadKeyContinuationAction     = "continuationAction"
	PayloadKeyResumeRunID            = "resumeRunId"
	PayloadKeySuccessorRunID         = "successorRunId"
	PayloadKeyDecisionID             = "decisionId"
	PayloadKeyType                   = "type"
	PayloadKeyPayload                = "payload"
	PayloadKeySummary                = "summary"
	PayloadKeySourceType             = "sourceType"
	PayloadKeySourceRef              = "sourceRef"
	PayloadKeyEvidenceRefs           = "evidenceRefs"
	PayloadKeyArtifactRefs           = "artifactRefs"
	PayloadKeyResultID               = "resultId"
	PayloadKeyArtifactID             = "artifactId"
	PayloadKeyArtifactType           = "artifactType"
	PayloadKeyName                   = "name"
	PayloadKeyURI                    = "uri"
	PayloadKeyContentType            = "contentType"
	PayloadKeyMemoryType             = "memoryType"
	PayloadKeyContent                = "content"
	PayloadKeyTags                   = "tags"
	PayloadKeyImportance             = "importance"
	PayloadKeyConfidence             = "confidence"
	PayloadKeyVisibility             = "visibility"
	PayloadKeyTraceID                = "traceId"
	PayloadKeyStage                  = "stage"
	PayloadKeyPolicy                 = "policy"
	PayloadKeyStatus                 = "status"
	PayloadKeyEnabled                = "enabled"
	PayloadKeyQuery                  = "query"
	PayloadKeyStates                 = "states"
	PayloadKeyCompact                = "compact"
	PayloadKeyTopic                  = "topic"
	PayloadKeySubjectType            = "subjectType"
	PayloadKeySubjectID              = "subjectId"
	PayloadKeyCursorAfterID          = "cursorAfterId"
	PayloadKeyOccurredAfter          = "occurredAfter"
	PayloadKeyOccurredBefore         = "occurredBefore"
	PayloadKeyLimit                  = "limit"
	PayloadKeyIncludePayload         = "includePayload"
	PayloadKeyProviderID             = "providerId"
	PayloadKeyOperation              = "operation"
	PayloadKeyItemType               = "itemType"
	PayloadKeyItemID                 = "itemId"
	PayloadKeyConfidenceScore        = "confidence"
	PayloadKeyPolicyMatched          = "policyMatched"
	PayloadKeyPolicyBlocked          = "policyBlocked"
	PayloadKeyBudgetBefore           = "budgetBefore"
	PayloadKeyBudgetAfter            = "budgetAfter"
	PayloadKeyLLMProviderID          = "llmProviderId"
	PayloadKeyLLMModel               = "llmModel"
	PayloadKeyLLMDecisionPreview     = "llmDecisionPreview"
	PayloadKeyPromptFingerprint      = "promptFingerprint"
	PayloadKeyResolvedAt             = "resolvedAt"
	PayloadKeyEscalationReason       = "escalationReason"
	PayloadKeyRetrievedMemoryIDs     = "retrievedMemoryIds"
	PayloadKeyPatchID                = "patchId"
	PayloadKeyStepID                 = "stepId"
	PayloadKeyStep                   = "step"
	PayloadKeySteps                  = "steps"
	PayloadKeyCheckpointID           = "checkpointId"
	PayloadKeyCheckpoint             = "checkpoint"
	PayloadKeyCheckpoints            = "checkpoints"
	PayloadKeyToolCallID             = "toolCallId"
	PayloadKeyPatchType              = "patchType"
	PayloadKeyTargetRef              = "targetRef"
	PayloadKeyAddedLines             = "addedLines"
	PayloadKeyDeletedLines           = "deletedLines"
	PayloadKeyContentHash            = "contentHash"
	PayloadKeyDiffHash               = "diffHash"
	PayloadKeyApprovalID             = "approvalId"
	PayloadKeyApproval               = "approval"
	PayloadKeyRisk                   = "risk"
	PayloadKeyError                  = "error"
	PayloadKeyWorkspaceID            = "workspaceId"
	PayloadKeyRunID                  = "runId"
	PayloadKeyRun                    = "run"
	PayloadKeyRuns                   = "runs"
	PayloadKeyWorkspace              = "workspace"
	PayloadKeyWorkspaces             = "workspaces"
	PayloadKeyModel                  = "model"
	PayloadKeyServerID               = "serverId"
	PayloadKeyServer                 = "server"
	PayloadKeyChunkCount             = "chunkCount"
	PayloadKeyReceivedBytes          = "receivedBytes"
	PayloadKeyElapsedMs              = "elapsedMs"
	PayloadKeyProvider               = "provider"
	PayloadKeyAPISurface             = "apiSurface"
	PayloadKeyInputTokens            = "inputTokens"
	PayloadKeyOutputTokens           = "outputTokens"
	PayloadKeyTotalTokens            = "totalTokens"
	PayloadKeyEstimated              = "estimated"
	PayloadKeyUsageSource            = "usageSource"
	PayloadKeyRequestBytes           = "requestBytes"
	PayloadKeyResponseBytes          = "responseBytes"
	PayloadKeyStreaming              = "streaming"
	PayloadKeyContextBudget          = "contextBudget"
	PayloadKeyCacheUsage             = "cacheUsage"
	PayloadKeyCacheMarkersSent       = "cacheMarkersSent"
	PayloadKeyStartedAt              = "startedAt"
	PayloadKeyCompletedAt            = "completedAt"
	PayloadKeyDurationMs             = "durationMs"
	PayloadKeyFailureClass           = "failureClass"
	PayloadKeyResult                 = "result"
	PayloadKeyArtifact               = "artifact"
	PayloadKeyIdempotencyKey         = "idempotencyKey"
	PayloadKeyProviderRequest        = "providerRequest"
	PayloadKeyProviderCompletion     = "providerCompletion"
	PayloadKeyProviderEmbedding      = "providerEmbedding"
	PayloadKeyTicket                 = "ticket"
	PayloadKeyTickets                = "tickets"
	PayloadKeyResolution             = "resolution"
	PayloadKeyGoal                   = "goal"
	PayloadKeyGoals                  = "goals"
	PayloadKeyDoD                    = "dod"
	PayloadKeyDoDs                   = "dods"
	PayloadKeyTemplateID             = "templateId"
	PayloadKeyWorkspaceTemplate      = "workspaceTemplate"
	PayloadKeyWorkspaceTemplates     = "workspaceTemplates"
	PayloadKeyMemoryItems            = "memoryItems"
	PayloadKeyMemoryItem             = "memoryItem"
	PayloadKeyMemoryID               = "memoryId"
	PayloadKeyMemorySearchQuery      = "memorySearchQuery"
	PayloadKeyMemoryRecallTrace      = "memoryRecallTrace"
	PayloadKeyMemoryRecallTraceQuery = "memoryRecallTraceQuery"
	PayloadKeyMemoryRecallTraces     = "memoryRecallTraces"
	PayloadKeyDeletedCount           = "deletedCount"
	PayloadKeyCommand                = "command"
	PayloadKeySoulID                 = "soulId"
	PayloadKeySoul                   = "soul"
	PayloadKeySouls                  = "souls"
	PayloadKeySkillRefs              = "skillRefs"
	PayloadKeyMCPServerRefs          = "mcpServerRefs"
	PayloadKeyCapabilityRefs         = "capabilityRefs"
	PayloadKeyCapabilityCategories   = "capabilityCategories"
	PayloadKeyToolKinds              = "toolKinds"
	PayloadKeySkillID                = "skillId"
	PayloadKeySkill                  = "skill"
	PayloadKeySkills                 = "skills"
	PayloadKeyServers                = "servers"
	PayloadKeyCapabilityID           = "capabilityId"
	PayloadKeyCapability             = "capability"
	PayloadKeyCapabilities           = "capabilities"
	PayloadKeyCapabilityQuery        = "capabilityQuery"
	PayloadKeyPriority               = "priority"
	PayloadKeyRuntimeStateRevision   = "runtimeStateRevision"
	PayloadKeyRuntimeStateRef        = "runtimeStateRef"
)

type WorkspacePayloadData struct {
	Title                string
	Description          string
	DeliverableDirectory string
	InternalDirectory    string
	State                common.WorkspaceState
	ActiveRunID          common.RunID
}

type RunPayloadData struct {
	GoalID               common.GoalID
	DoDID                common.DoDID
	State                common.RunState
	BlockingTicketID     common.TicketID
	BlockingApprovalID   common.ApprovalID
	FailureReason        string
	RuntimeStateRevision int
	RuntimeStateRef      string
}

type GoalPayloadData struct {
	Text                   string
	DefinitionRevision     string
	PreviousDefinitionHash string
	PreviousGoalID         common.GoalID
	LineageState           string
}

type DoDPayloadData struct {
	Items                  []string
	State                  common.DoDState
	DefinitionRevision     string
	PreviousDefinitionHash string
	PreviousDoDID          common.DoDID
	LineageState           string
}

type DecisionTicketPayloadData struct {
	Reason string
	State  common.DecisionTicketState
}

type DecisionResolutionPayloadData struct {
	DecisionTicketPayloadData
	ResolutionID       common.ResolutionID
	Type               string
	Payload            workorcheventpayload.Payload
	ContinuationAction string
	ResumeRunID        common.RunID
	SuccessorRunID     common.RunID
}

type ApprovalPayloadData struct {
	Reason  string
	Action  string
	Context workorcheventpayload.Payload
	State   common.ApprovalState
}

type ResultPayloadData struct {
	GoalID       common.GoalID
	DoDID        common.DoDID
	State        common.ResultState
	Title        string
	Summary      string
	SourceType   string
	SourceRef    string
	EvidenceRefs []string
	ArtifactRefs []string
}

type ArtifactPayloadData struct {
	ResultID           common.ResultID
	StepID             common.StepID
	ToolCallID         common.ToolCallID
	PatchID            common.PatchID
	ValidationResultID common.ValidationResultID
	ArtifactType       string
	Name               string
	Summary            string
	URI                string
	ContentType        string
	State              common.ArtifactState
	EvidenceRefs       []string
}

type PatchPayloadData struct {
	PatchID      common.PatchID
	StepID       common.StepID
	ToolCallID   common.ToolCallID
	PatchType    string
	TargetRef    string
	Summary      string
	State        common.PatchState
	AddedLines   int
	DeletedLines int
	ContentHash  string
	DiffHash     string
	ApprovalID   common.ApprovalID
	Risk         string
	Error        string
}

type MemoryPayloadData struct {
	MemoryType string
	Summary    string
	Content    string
	SourceType string
	SourceRef  string
	Tags       []string
	Importance int
	Confidence int
	Visibility string
}

type MemoryRecallPayloadData struct {
	TraceID            common.MemoryRecallTraceID
	Stage              string
	Source             string
	Policy             string
	Status             string
	ProviderID         string
	Operation          string
	RetrievedMemoryIDs []common.MemoryID
	Reason             string
}

// LLMUsagePayloadData is the durable event payload for /run/llm_usage/recorded.
// It carries sealed provider/model target, revisions, budget and usage for one request.
type LLMUsagePayloadData struct {
	RequestID             string
	WorkspaceID           string
	RunID                 string
	StepID                string
	ProviderID            string
	Provider              string
	APISurface            string
	Model                 string
	Operation             string
	CapabilityRevision    string
	ConfigurationRevision string
	StablePrefixHash      string
	Kind                  string
	InputTokens           int
	OutputTokens          int
	TotalTokens           int
	Estimated             bool
	UsageSource           string
	RequestBytes          int
	ResponseBytes         int
	Streaming             bool
	ContextBudget         llmaudit.ContextBudget
	CacheUsage            llmaudit.CacheUsage
	StartedAt             string
	CompletedAt           string
	DurationMs            int
	Status                string
	FailureClass          string
	ProviderSubmitted     bool
}

type AutoGovernancePayloadData struct {
	TraceID            common.AutoGovernanceTraceID
	ItemType           string
	ItemID             string
	Action             string
	RiskLevel          string
	Confidence         float64
	PolicyMatched      string
	PolicyBlocked      string
	BudgetBefore       int
	BudgetAfter        int
	PromptFingerprint  string
	LLMProviderID      string
	LLMModel           string
	LLMDecisionPreview string
	Status             string
	Reason             string
	EscalationReason   string
}

type ArtifactContinuationPayloadData struct {
	ContinuationID    common.ArtifactContinuationID
	RunID             common.RunID
	ArtifactID        string
	TargetRef         string
	State             common.ArtifactContinuationState
	ExpectedSequence  int
	FinalReceived     bool
	BytesWritten      int64
	RollingHash       string
	LastChunkHash     string
	ReceivedSequences []int
	CoverageTargets   []string
	EvidenceRefs      []string
	FailureReason     string
}

func (data WorkspacePayloadData) EventPayload() workorcheventpayload.Payload {
	fields := []workorcheventpayload.Field{
		workorcheventpayload.StringField(PayloadKeyState, string(common.CanonicalWorkspaceOwnerState(data.State))),
		workorcheventpayload.StringField(PayloadKeyActiveRunID, string(data.ActiveRunID)),
	}
	fields = appendOptionalPayloadString(fields, PayloadKeyTitle, data.Title)
	fields = appendOptionalPayloadString(fields, PayloadKeyDescription, data.Description)
	fields = appendOptionalPayloadString(fields, PayloadKeyDeliverableDirectory, data.DeliverableDirectory)
	fields = appendOptionalPayloadString(fields, PayloadKeyInternalDirectory, data.InternalDirectory)
	return workorcheventpayload.New(fields...)
}

func (data RunPayloadData) EventPayload() workorcheventpayload.Payload {
	fields := []workorcheventpayload.Field{
		workorcheventpayload.StringField(PayloadKeyState, string(common.CanonicalRunState(data.State))),
		workorcheventpayload.StringField(PayloadKeyBlockingTicketID, string(data.BlockingTicketID)),
		workorcheventpayload.StringField(PayloadKeyBlockingApprovalID, string(data.BlockingApprovalID)),
	}
	fields = appendOptionalPayloadString(fields, PayloadKeyGoalID, string(data.GoalID))
	fields = appendOptionalPayloadString(fields, PayloadKeyDoDID, string(data.DoDID))
	fields = appendOptionalPayloadString(fields, PayloadKeyFailureReason, data.FailureReason)
	if data.RuntimeStateRevision > 0 {
		fields = append(fields, workorcheventpayload.IntField(PayloadKeyRuntimeStateRevision, int64(data.RuntimeStateRevision)))
	}
	fields = appendOptionalPayloadString(fields, PayloadKeyRuntimeStateRef, data.RuntimeStateRef)
	return workorcheventpayload.New(fields...)
}

func (data GoalPayloadData) EventPayload() workorcheventpayload.Payload {
	fields := []workorcheventpayload.Field{workorcheventpayload.StringField(PayloadKeyText, data.Text)}
	fields = appendOptionalPayloadString(fields, PayloadKeyDefinitionRevision, data.DefinitionRevision)
	fields = appendOptionalPayloadString(fields, PayloadKeyPreviousDefinitionHash, data.PreviousDefinitionHash)
	fields = appendOptionalPayloadString(fields, PayloadKeyPreviousGoalID, string(data.PreviousGoalID))
	fields = appendOptionalPayloadString(fields, PayloadKeyLineageState, data.LineageState)
	return workorcheventpayload.New(fields...)
}

func (data DoDPayloadData) EventPayload() workorcheventpayload.Payload {
	fields := []workorcheventpayload.Field{
		workorcheventpayload.StringArrayField(PayloadKeyItems, data.Items),
		workorcheventpayload.StringField(PayloadKeyState, string(data.State)),
	}
	fields = appendOptionalPayloadString(fields, PayloadKeyDefinitionRevision, data.DefinitionRevision)
	fields = appendOptionalPayloadString(fields, PayloadKeyPreviousDefinitionHash, data.PreviousDefinitionHash)
	fields = appendOptionalPayloadString(fields, PayloadKeyPreviousDoDID, string(data.PreviousDoDID))
	fields = appendOptionalPayloadString(fields, PayloadKeyLineageState, data.LineageState)
	return workorcheventpayload.New(fields...)
}

func (data DecisionTicketPayloadData) EventPayload() workorcheventpayload.Payload {
	return workorcheventpayload.New(
		workorcheventpayload.StringField(PayloadKeyReason, data.Reason),
		workorcheventpayload.StringField(PayloadKeyState, string(data.State)),
	)
}

func (data DecisionResolutionPayloadData) EventPayload() workorcheventpayload.Payload {
	fields := data.DecisionTicketPayloadData.EventPayload().Fields
	fields = append(fields,
		workorcheventpayload.StringField(PayloadKeyResolutionID, string(data.ResolutionID)),
		workorcheventpayload.StringField(PayloadKeyType, data.Type),
		workorcheventpayload.ObjectField(PayloadKeyPayload, data.Payload),
	)
	fields = appendOptionalPayloadString(fields, PayloadKeyContinuationAction, data.ContinuationAction)
	fields = appendOptionalPayloadString(fields, PayloadKeyResumeRunID, string(data.ResumeRunID))
	fields = appendOptionalPayloadString(fields, PayloadKeySuccessorRunID, string(data.SuccessorRunID))
	return workorcheventpayload.New(fields...)
}

func (data ApprovalPayloadData) EventPayload() workorcheventpayload.Payload {
	fields := []workorcheventpayload.Field{
		workorcheventpayload.StringField(PayloadKeyReason, data.Reason),
		workorcheventpayload.StringField(PayloadKeyState, string(data.State)),
	}
	fields = appendOptionalPayloadString(fields, PayloadKeyAction, data.Action)
	if len(data.Context.Fields) > 0 {
		fields = append(fields, workorcheventpayload.ObjectField(PayloadKeyContext, data.Context))
	}
	return workorcheventpayload.New(fields...)
}

func DecodeWorkspaceEventPayload(payload workorcheventpayload.Payload) (WorkspacePayloadData, bool) {
	state, ok := payload.String(PayloadKeyState)
	if !ok {
		return WorkspacePayloadData{}, false
	}
	title, ok := optionalEventPayloadString(payload, PayloadKeyTitle)
	if !ok {
		return WorkspacePayloadData{}, false
	}
	description, ok := optionalEventPayloadString(payload, PayloadKeyDescription)
	if !ok {
		return WorkspacePayloadData{}, false
	}
	deliverableDirectory, ok := optionalEventPayloadString(payload, PayloadKeyDeliverableDirectory)
	if !ok {
		return WorkspacePayloadData{}, false
	}
	internalDirectory, ok := optionalEventPayloadString(payload, PayloadKeyInternalDirectory)
	if !ok {
		return WorkspacePayloadData{}, false
	}
	activeRunID, ok := optionalEventPayloadString(payload, PayloadKeyActiveRunID)
	if !ok {
		return WorkspacePayloadData{}, false
	}
	return WorkspacePayloadData{
		Title:                title,
		Description:          description,
		DeliverableDirectory: deliverableDirectory,
		InternalDirectory:    internalDirectory,
		State:                common.CanonicalWorkspaceOwnerState(common.WorkspaceState(state)),
		ActiveRunID:          common.RunID(activeRunID),
	}, true
}

func DecodeRunEventPayload(payload workorcheventpayload.Payload) (RunPayloadData, bool) {
	state, ok := payload.String(PayloadKeyState)
	if !ok {
		return RunPayloadData{}, false
	}
	goalID, ok := optionalEventPayloadString(payload, PayloadKeyGoalID)
	if !ok {
		return RunPayloadData{}, false
	}
	dodID, ok := optionalEventPayloadString(payload, PayloadKeyDoDID)
	if !ok {
		return RunPayloadData{}, false
	}
	blockingTicketID, ok := optionalEventPayloadString(payload, PayloadKeyBlockingTicketID)
	if !ok {
		return RunPayloadData{}, false
	}
	blockingApprovalID, ok := optionalEventPayloadString(payload, PayloadKeyBlockingApprovalID)
	if !ok {
		return RunPayloadData{}, false
	}
	failureReason, ok := optionalEventPayloadString(payload, PayloadKeyFailureReason)
	if !ok {
		return RunPayloadData{}, false
	}
	runtimeStateRevision, ok := optionalEventPayloadInt(payload, PayloadKeyRuntimeStateRevision)
	if !ok {
		return RunPayloadData{}, false
	}
	runtimeStateRef, ok := optionalEventPayloadString(payload, PayloadKeyRuntimeStateRef)
	if !ok {
		return RunPayloadData{}, false
	}
	return RunPayloadData{
		GoalID:               common.GoalID(goalID),
		DoDID:                common.DoDID(dodID),
		State:                common.CanonicalRunState(common.RunState(state)),
		BlockingTicketID:     common.TicketID(blockingTicketID),
		BlockingApprovalID:   common.ApprovalID(blockingApprovalID),
		FailureReason:        failureReason,
		RuntimeStateRevision: runtimeStateRevision,
		RuntimeStateRef:      runtimeStateRef,
	}, true
}

func DecodeDecisionTicketEventPayload(payload workorcheventpayload.Payload) (DecisionTicketPayloadData, bool) {
	reason, ok := payload.String(PayloadKeyReason)
	if !ok {
		return DecisionTicketPayloadData{}, false
	}
	state, ok := payload.String(PayloadKeyState)
	if !ok {
		return DecisionTicketPayloadData{}, false
	}
	return DecisionTicketPayloadData{Reason: reason, State: common.DecisionTicketState(state)}, true
}

func DecodeDecisionResolutionEventPayload(payload workorcheventpayload.Payload) (DecisionResolutionPayloadData, bool) {
	base, ok := DecodeDecisionTicketEventPayload(payload)
	if !ok {
		return DecisionResolutionPayloadData{}, false
	}
	resolutionID, ok := payload.String(PayloadKeyResolutionID)
	if !ok {
		return DecisionResolutionPayloadData{}, false
	}
	resolutionType, ok := payload.String(PayloadKeyType)
	if !ok {
		return DecisionResolutionPayloadData{}, false
	}
	resolutionPayload, ok := optionalEventPayloadObject(payload, PayloadKeyPayload)
	if !ok {
		return DecisionResolutionPayloadData{}, false
	}
	continuationAction, ok := optionalEventPayloadString(payload, PayloadKeyContinuationAction)
	if !ok {
		return DecisionResolutionPayloadData{}, false
	}
	resumeRunID, ok := optionalEventPayloadString(payload, PayloadKeyResumeRunID)
	if !ok {
		return DecisionResolutionPayloadData{}, false
	}
	successorRunID, ok := optionalEventPayloadString(payload, PayloadKeySuccessorRunID)
	if !ok {
		return DecisionResolutionPayloadData{}, false
	}
	return DecisionResolutionPayloadData{
		DecisionTicketPayloadData: base,
		ResolutionID:              common.ResolutionID(resolutionID),
		Type:                      resolutionType,
		Payload:                   resolutionPayload,
		ContinuationAction:        continuationAction,
		ResumeRunID:               common.RunID(resumeRunID),
		SuccessorRunID:            common.RunID(successorRunID),
	}, true
}

func DecodeApprovalEventPayload(payload workorcheventpayload.Payload) (ApprovalPayloadData, bool) {
	reason, ok := payload.String(PayloadKeyReason)
	if !ok {
		return ApprovalPayloadData{}, false
	}
	state, ok := payload.String(PayloadKeyState)
	if !ok {
		return ApprovalPayloadData{}, false
	}
	action, ok := optionalEventPayloadString(payload, PayloadKeyAction)
	if !ok {
		return ApprovalPayloadData{}, false
	}
	context, ok := optionalEventPayloadObject(payload, PayloadKeyContext)
	if !ok {
		return ApprovalPayloadData{}, false
	}
	return ApprovalPayloadData{
		Reason:  reason,
		Action:  action,
		Context: context,
		State:   common.ApprovalState(state),
	}, true
}

func optionalEventPayloadString(payload workorcheventpayload.Payload, key string) (string, bool) {
	value, exists := payload.Field(key)
	if !exists || value.Kind == workorcheventpayload.ValueNull {
		return "", true
	}
	if value.Kind != workorcheventpayload.ValueString {
		return "", false
	}
	return value.String, true
}

func optionalEventPayloadInt(payload workorcheventpayload.Payload, key string) (int, bool) {
	value, exists := payload.Field(key)
	if !exists || value.Kind == workorcheventpayload.ValueNull {
		return 0, true
	}
	parsed, ok := payload.Int64(key)
	if !ok {
		return 0, false
	}
	return int(parsed), true
}

func optionalEventPayloadObject(payload workorcheventpayload.Payload, key string) (workorcheventpayload.Payload, bool) {
	value, exists := payload.Field(key)
	if !exists || value.Kind == workorcheventpayload.ValueNull {
		return workorcheventpayload.Payload{}, true
	}
	return payload.Object(key)
}

func (data ResultPayloadData) EventPayload() workorcheventpayload.Payload {
	fields := []workorcheventpayload.Field{
		workorcheventpayload.StringField(PayloadKeyState, string(data.State)),
		workorcheventpayload.StringArrayField(PayloadKeyEvidenceRefs, data.EvidenceRefs),
		workorcheventpayload.StringArrayField(PayloadKeyArtifactRefs, data.ArtifactRefs),
	}
	fields = appendOptionalPayloadString(fields, PayloadKeyGoalID, string(data.GoalID))
	fields = appendOptionalPayloadString(fields, PayloadKeyDoDID, string(data.DoDID))
	fields = appendOptionalPayloadString(fields, PayloadKeyTitle, data.Title)
	fields = appendOptionalPayloadString(fields, PayloadKeySummary, data.Summary)
	fields = appendOptionalPayloadString(fields, PayloadKeySourceType, data.SourceType)
	fields = appendOptionalPayloadString(fields, PayloadKeySourceRef, data.SourceRef)
	return workorcheventpayload.New(fields...)
}

func (data ArtifactPayloadData) EventPayload() workorcheventpayload.Payload {
	fields := []workorcheventpayload.Field{
		workorcheventpayload.StringField(PayloadKeyArtifactType, data.ArtifactType),
		workorcheventpayload.StringField(PayloadKeyState, string(data.State)),
		workorcheventpayload.StringArrayField(PayloadKeyEvidenceRefs, data.EvidenceRefs),
	}
	fields = appendOptionalPayloadString(fields, PayloadKeyResultID, string(data.ResultID))
	fields = appendOptionalPayloadString(fields, PayloadKeyStepID, string(data.StepID))
	fields = appendOptionalPayloadString(fields, PayloadKeyToolCallID, string(data.ToolCallID))
	fields = appendOptionalPayloadString(fields, PayloadKeyPatchID, string(data.PatchID))
	fields = appendOptionalPayloadString(fields, "validationResultId", string(data.ValidationResultID))
	fields = appendOptionalPayloadString(fields, PayloadKeyName, data.Name)
	fields = appendOptionalPayloadString(fields, PayloadKeySummary, data.Summary)
	fields = appendOptionalPayloadString(fields, PayloadKeyURI, data.URI)
	fields = appendOptionalPayloadString(fields, PayloadKeyContentType, data.ContentType)
	return workorcheventpayload.New(fields...)
}

func (data PatchPayloadData) EventPayload() workorcheventpayload.Payload {
	fields := []workorcheventpayload.Field{
		workorcheventpayload.StringField(PayloadKeyState, string(data.State)),
		workorcheventpayload.StringField(PayloadKeyPatchType, data.PatchType),
		workorcheventpayload.StringField(PayloadKeyTargetRef, data.TargetRef),
		workorcheventpayload.IntField(PayloadKeyAddedLines, int64(data.AddedLines)),
		workorcheventpayload.IntField(PayloadKeyDeletedLines, int64(data.DeletedLines)),
	}
	fields = appendOptionalPayloadString(fields, PayloadKeyPatchID, string(data.PatchID))
	fields = appendOptionalPayloadString(fields, PayloadKeyStepID, string(data.StepID))
	fields = appendOptionalPayloadString(fields, PayloadKeyToolCallID, string(data.ToolCallID))
	fields = appendOptionalPayloadString(fields, PayloadKeySummary, data.Summary)
	fields = appendOptionalPayloadString(fields, PayloadKeyContentHash, data.ContentHash)
	fields = appendOptionalPayloadString(fields, PayloadKeyDiffHash, data.DiffHash)
	fields = appendOptionalPayloadString(fields, PayloadKeyApprovalID, string(data.ApprovalID))
	fields = appendOptionalPayloadString(fields, PayloadKeyRisk, data.Risk)
	fields = appendOptionalPayloadString(fields, PayloadKeyError, data.Error)
	return workorcheventpayload.New(fields...)
}

func (data MemoryPayloadData) EventPayload() workorcheventpayload.Payload {
	fields := []workorcheventpayload.Field{
		workorcheventpayload.StringField(PayloadKeyMemoryType, data.MemoryType),
		workorcheventpayload.StringField(PayloadKeySummary, data.Summary),
		workorcheventpayload.StringField(PayloadKeySourceType, data.SourceType),
		workorcheventpayload.StringArrayField(PayloadKeyTags, data.Tags),
		workorcheventpayload.IntField(PayloadKeyImportance, int64(data.Importance)),
		workorcheventpayload.IntField(PayloadKeyConfidence, int64(data.Confidence)),
		workorcheventpayload.StringField(PayloadKeyVisibility, data.Visibility),
	}
	fields = appendOptionalPayloadString(fields, PayloadKeyContent, data.Content)
	fields = appendOptionalPayloadString(fields, PayloadKeySourceRef, data.SourceRef)
	return workorcheventpayload.New(fields...)
}

func (data MemoryRecallPayloadData) EventPayload() workorcheventpayload.Payload {
	fields := []workorcheventpayload.Field{
		workorcheventpayload.StringField(PayloadKeyTraceID, string(data.TraceID)),
		workorcheventpayload.StringField(PayloadKeyStage, data.Stage),
		workorcheventpayload.StringField(PayloadKeySourceType, data.Source),
		workorcheventpayload.StringField(PayloadKeyPolicy, data.Policy),
		workorcheventpayload.StringField(PayloadKeyStatus, data.Status),
		workorcheventpayload.StringArrayField(PayloadKeyRetrievedMemoryIDs, memoryIDsToStrings(data.RetrievedMemoryIDs)),
	}
	fields = appendOptionalPayloadString(fields, PayloadKeyProviderID, data.ProviderID)
	fields = appendOptionalPayloadString(fields, PayloadKeyOperation, data.Operation)
	fields = appendOptionalPayloadString(fields, PayloadKeyReason, data.Reason)
	return workorcheventpayload.New(fields...)
}

func (data LLMUsagePayloadData) EventPayload() workorcheventpayload.Payload {
	fields := []workorcheventpayload.Field{
		workorcheventpayload.StringField(PayloadKeyWorkspaceID, data.WorkspaceID),
		workorcheventpayload.StringField(PayloadKeyRunID, data.RunID),
		workorcheventpayload.StringField(PayloadKeyStatus, data.Status),
		workorcheventpayload.BoolField(PayloadKeyEstimated, data.Estimated),
		workorcheventpayload.BoolField(PayloadKeyStreaming, data.Streaming),
		workorcheventpayload.BoolField("providerSubmitted", data.ProviderSubmitted),
		workorcheventpayload.IntField(PayloadKeyInputTokens, int64(data.InputTokens)),
		workorcheventpayload.IntField(PayloadKeyOutputTokens, int64(data.OutputTokens)),
		workorcheventpayload.IntField(PayloadKeyTotalTokens, int64(data.TotalTokens)),
		workorcheventpayload.IntField(PayloadKeyRequestBytes, int64(data.RequestBytes)),
		workorcheventpayload.IntField(PayloadKeyResponseBytes, int64(data.ResponseBytes)),
		workorcheventpayload.IntField(PayloadKeyDurationMs, int64(data.DurationMs)),
	}
	fields = appendOptionalPayloadString(fields, PayloadKeyRequestID, data.RequestID)
	fields = appendOptionalPayloadString(fields, PayloadKeyStepID, data.StepID)
	fields = appendOptionalPayloadString(fields, PayloadKeyProviderID, data.ProviderID)
	fields = appendOptionalPayloadString(fields, PayloadKeyProvider, data.Provider)
	fields = appendOptionalPayloadString(fields, PayloadKeyAPISurface, data.APISurface)
	fields = appendOptionalPayloadString(fields, PayloadKeyModel, data.Model)
	fields = appendOptionalPayloadString(fields, PayloadKeyOperation, data.Operation)
	fields = appendOptionalPayloadString(fields, "capabilityRevision", data.CapabilityRevision)
	fields = appendOptionalPayloadString(fields, "configurationRevision", data.ConfigurationRevision)
	fields = appendOptionalPayloadString(fields, "stablePrefixHash", data.StablePrefixHash)
	fields = appendOptionalPayloadString(fields, PayloadKeyKind, data.Kind)
	fields = appendOptionalPayloadString(fields, PayloadKeyUsageSource, data.UsageSource)
	fields = appendOptionalPayloadString(fields, PayloadKeyStartedAt, data.StartedAt)
	fields = appendOptionalPayloadString(fields, PayloadKeyCompletedAt, data.CompletedAt)
	fields = appendOptionalPayloadString(fields, PayloadKeyFailureClass, data.FailureClass)
	if !data.ContextBudget.Empty() {
		fields = append(fields, workorcheventpayload.ObjectField(PayloadKeyContextBudget, data.ContextBudget.Payload()))
	}
	if !data.CacheUsage.Empty() {
		fields = append(fields, workorcheventpayload.ObjectField(PayloadKeyCacheUsage, data.CacheUsage.Payload()))
	}
	return workorcheventpayload.New(fields...)
}

func (data AutoGovernancePayloadData) EventPayload() workorcheventpayload.Payload {
	fields := []workorcheventpayload.Field{
		workorcheventpayload.StringField(PayloadKeyTraceID, string(data.TraceID)),
		workorcheventpayload.StringField(PayloadKeyItemType, data.ItemType),
		workorcheventpayload.StringField(PayloadKeyItemID, data.ItemID),
		workorcheventpayload.StringField(PayloadKeyAction, data.Action),
		workorcheventpayload.StringField(PayloadKeyRisk, data.RiskLevel),
		workorcheventpayload.FloatField(PayloadKeyConfidenceScore, data.Confidence),
		workorcheventpayload.StringField(PayloadKeyPolicyMatched, data.PolicyMatched),
		workorcheventpayload.StringField(PayloadKeyPolicyBlocked, data.PolicyBlocked),
		workorcheventpayload.IntField(PayloadKeyBudgetBefore, int64(data.BudgetBefore)),
		workorcheventpayload.IntField(PayloadKeyBudgetAfter, int64(data.BudgetAfter)),
		workorcheventpayload.StringField(PayloadKeyLLMProviderID, data.LLMProviderID),
		workorcheventpayload.StringField(PayloadKeyLLMModel, data.LLMModel),
		workorcheventpayload.StringField(PayloadKeyLLMDecisionPreview, data.LLMDecisionPreview),
		workorcheventpayload.StringField(PayloadKeyPromptFingerprint, data.PromptFingerprint),
		workorcheventpayload.StringField(PayloadKeyStatus, data.Status),
	}
	fields = appendOptionalPayloadString(fields, PayloadKeyReason, data.Reason)
	fields = appendOptionalPayloadString(fields, PayloadKeyEscalationReason, data.EscalationReason)
	return workorcheventpayload.New(fields...)
}

func (data ArtifactContinuationPayloadData) EventPayload() workorcheventpayload.Payload {
	fields := []workorcheventpayload.Field{
		workorcheventpayload.StringField("id", string(data.ContinuationID)),
		workorcheventpayload.StringField(PayloadKeyRunID, string(data.RunID)),
		workorcheventpayload.StringField(PayloadKeyArtifactID, data.ArtifactID),
		workorcheventpayload.StringField(PayloadKeyTargetRef, data.TargetRef),
		workorcheventpayload.StringField(PayloadKeyState, string(data.State)),
		workorcheventpayload.IntField("expectedSequence", int64(data.ExpectedSequence)),
		workorcheventpayload.BoolField("finalReceived", data.FinalReceived),
		workorcheventpayload.IntField("bytesWritten", data.BytesWritten),
	}
	fields = appendOptionalPayloadString(fields, "rollingHash", data.RollingHash)
	fields = appendOptionalPayloadString(fields, "lastChunkHash", data.LastChunkHash)
	if len(data.ReceivedSequences) > 0 {
		values := make([]workorcheventpayload.Value, 0, len(data.ReceivedSequences))
		for _, value := range data.ReceivedSequences {
			values = append(values, workorcheventpayload.Value{Kind: workorcheventpayload.ValueInt, Int: int64(value)})
		}
		fields = append(fields, workorcheventpayload.Field{Name: "receivedSequences", Value: workorcheventpayload.Value{Kind: workorcheventpayload.ValueArray, Array: values}})
	}
	if len(data.CoverageTargets) > 0 {
		fields = append(fields, workorcheventpayload.StringArrayField("coverageTargets", data.CoverageTargets))
	}
	if len(data.EvidenceRefs) > 0 {
		fields = append(fields, workorcheventpayload.StringArrayField(PayloadKeyEvidenceRefs, data.EvidenceRefs))
	}
	fields = appendOptionalPayloadString(fields, PayloadKeyFailureReason, data.FailureReason)
	return workorcheventpayload.New(fields...)
}

func appendOptionalPayloadString(fields []workorcheventpayload.Field, name string, value string) []workorcheventpayload.Field {
	if value != "" {
		fields = append(fields, workorcheventpayload.StringField(name, value))
	}
	return fields
}

func memoryIDsToStrings(input []common.MemoryID) []string {
	ret := make([]string, 0, len(input))
	for _, item := range input {
		ret = append(ret, string(item))
	}
	return ret
}
