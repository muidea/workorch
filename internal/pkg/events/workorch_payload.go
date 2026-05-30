package events

import (
	"encoding/json"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
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
	PayloadKeyInput                  = "input"
	PayloadKeyOutput                 = "output"
	PayloadKeyReason                 = "reason"
	PayloadKeyAction                 = "action"
	PayloadKeyContext                = "context"
	PayloadKeyResolutionID           = "resolutionId"
	PayloadKeyType                   = "type"
	PayloadKeyPayload                = "payload"
	PayloadKeySummary                = "summary"
	PayloadKeySourceType             = "sourceType"
	PayloadKeySourceRef              = "sourceRef"
	PayloadKeyEvidenceRefs           = "evidenceRefs"
	PayloadKeyArtifactRefs           = "artifactRefs"
	PayloadKeyResultID               = "resultId"
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
	PayloadKeyToolCallID             = "toolCallId"
	PayloadKeyPatchType              = "patchType"
	PayloadKeyTargetRef              = "targetRef"
	PayloadKeyAddedLines             = "addedLines"
	PayloadKeyDeletedLines           = "deletedLines"
	PayloadKeyContentHash            = "contentHash"
	PayloadKeyDiffHash               = "diffHash"
	PayloadKeyApprovalID             = "approvalId"
	PayloadKeyRisk                   = "risk"
	PayloadKeyError                  = "error"
	PayloadKeyWorkspaceID            = "workspaceId"
	PayloadKeyRunID                  = "runId"
	PayloadKeyModel                  = "model"
	PayloadKeyChunkCount             = "chunkCount"
	PayloadKeyReceivedBytes          = "receivedBytes"
	PayloadKeyElapsedMs              = "elapsedMs"
	PayloadKeyProvider               = "provider"
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
	PayloadKeyStartedAt              = "startedAt"
	PayloadKeyCompletedAt            = "completedAt"
	PayloadKeyDurationMs             = "durationMs"
	PayloadKeyFailureClass           = "failureClass"
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
	GoalID             common.GoalID
	DoDID              common.DoDID
	State              common.RunState
	BlockingTicketID   common.TicketID
	BlockingApprovalID common.ApprovalID
	FailureReason      string
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

func GoalPayload(text string, definitionRevision string, previousDefinitionHash string, previousGoalID common.GoalID, lineageState string) map[string]any {
	ret := map[string]any{
		PayloadKeyText: text,
	}
	if definitionRevision != "" {
		ret[PayloadKeyDefinitionRevision] = definitionRevision
	}
	if previousDefinitionHash != "" {
		ret[PayloadKeyPreviousDefinitionHash] = previousDefinitionHash
	}
	if previousGoalID != "" {
		ret[PayloadKeyPreviousGoalID] = string(previousGoalID)
	}
	if lineageState != "" {
		ret[PayloadKeyLineageState] = lineageState
	}
	return ret
}

func DoDPayload(items []string, state common.DoDState, definitionRevision string, previousDefinitionHash string, previousDoDID common.DoDID, lineageState string) map[string]any {
	ret := map[string]any{
		PayloadKeyItems: append([]string(nil), items...),
		PayloadKeyState: string(state),
	}
	if definitionRevision != "" {
		ret[PayloadKeyDefinitionRevision] = definitionRevision
	}
	if previousDefinitionHash != "" {
		ret[PayloadKeyPreviousDefinitionHash] = previousDefinitionHash
	}
	if previousDoDID != "" {
		ret[PayloadKeyPreviousDoDID] = string(previousDoDID)
	}
	if lineageState != "" {
		ret[PayloadKeyLineageState] = lineageState
	}
	return ret
}

type StepPayloadData struct {
	Kind   string
	State  common.StepState
	Input  map[string]any
	Output map[string]any
}

type DecisionTicketPayloadData struct {
	Reason string
	State  common.DecisionTicketState
}

type DecisionResolutionPayloadData struct {
	DecisionTicketPayloadData
	ResolutionID common.ResolutionID
	Type         string
	Payload      map[string]any
}

type ApprovalPayloadData struct {
	Reason  string
	Action  string
	Context map[string]any
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

type LLMStreamProgressPayloadData struct {
	WorkspaceID   common.WorkspaceID
	RunID         common.RunID
	StepID        common.StepID
	ProviderID    string
	Model         string
	ChunkCount    int
	ReceivedBytes int
	ElapsedMs     int
	Status        string
	Summary       string
}

type LLMUsagePayloadData struct {
	WorkspaceID   common.WorkspaceID
	RunID         common.RunID
	StepID        common.StepID
	ProviderID    string
	Provider      string
	Model         string
	Operation     string
	InputTokens   int
	OutputTokens  int
	TotalTokens   int
	Estimated     bool
	UsageSource   string
	RequestBytes  int
	ResponseBytes int
	Streaming     bool
	ContextBudget map[string]any
	CacheUsage    map[string]any
	StartedAt     string
	CompletedAt   string
	DurationMs    int
	Status        string
	FailureClass  string
}

func WorkspacePayload(title string, description string, deliverableDirectory string, internalDirectory string, state common.WorkspaceState, activeRunID common.RunID) map[string]any {
	payload := map[string]any{
		PayloadKeyState:       string(state),
		PayloadKeyActiveRunID: string(activeRunID),
	}
	if title != "" {
		payload[PayloadKeyTitle] = title
	}
	if description != "" {
		payload[PayloadKeyDescription] = description
	}
	if deliverableDirectory != "" {
		payload[PayloadKeyDeliverableDirectory] = deliverableDirectory
	}
	if internalDirectory != "" {
		payload[PayloadKeyInternalDirectory] = internalDirectory
	}
	return payload
}

func RunPayload(
	goalID common.GoalID,
	dodID common.DoDID,
	state common.RunState,
	blockingTicketID common.TicketID,
	blockingApprovalID common.ApprovalID,
	failureReason string,
) map[string]any {
	payload := map[string]any{
		PayloadKeyState:              string(state),
		PayloadKeyBlockingTicketID:   string(blockingTicketID),
		PayloadKeyBlockingApprovalID: string(blockingApprovalID),
	}
	if goalID != "" {
		payload[PayloadKeyGoalID] = string(goalID)
	}
	if dodID != "" {
		payload[PayloadKeyDoDID] = string(dodID)
	}
	if failureReason != "" {
		payload[PayloadKeyFailureReason] = failureReason
	}
	return payload
}

func StepPayload(kind string, state common.StepState, input, output map[string]any) map[string]any {
	payload := map[string]any{
		PayloadKeyKind:  llmcommon.CanonicalRuntimeBlockKind(kind),
		PayloadKeyState: string(state),
	}
	if input != nil {
		payload[PayloadKeyInput] = clonePayloadMap(input)
	}
	if output != nil {
		payload[PayloadKeyOutput] = clonePayloadMap(output)
	}
	return payload
}

func DecisionTicketPayload(reason string, state common.DecisionTicketState) map[string]any {
	return map[string]any{
		PayloadKeyReason: reason,
		PayloadKeyState:  string(state),
	}
}

func DecisionResolutionPayload(
	reason string,
	state common.DecisionTicketState,
	resolutionID common.ResolutionID,
	resolutionType string,
	payload map[string]any,
) map[string]any {
	ret := DecisionTicketPayload(reason, state)
	ret[PayloadKeyResolutionID] = string(resolutionID)
	ret[PayloadKeyType] = resolutionType
	ret[PayloadKeyPayload] = clonePayloadMap(payload)
	return ret
}

func ApprovalPayload(reason string, action string, context map[string]any, state common.ApprovalState) map[string]any {
	ret := map[string]any{
		PayloadKeyReason: reason,
		PayloadKeyState:  string(state),
	}
	if action != "" {
		ret[PayloadKeyAction] = action
	}
	if len(context) > 0 {
		ret[PayloadKeyContext] = clonePayloadMap(context)
	}
	return ret
}

func ResultPayload(
	goalID common.GoalID,
	dodID common.DoDID,
	state common.ResultState,
	title string,
	summary string,
	sourceType string,
	sourceRef string,
	evidenceRefs []string,
	artifactRefs []string,
) map[string]any {
	payload := map[string]any{
		PayloadKeyState:        string(state),
		PayloadKeyEvidenceRefs: append([]string(nil), evidenceRefs...),
		PayloadKeyArtifactRefs: append([]string(nil), artifactRefs...),
	}
	if goalID != "" {
		payload[PayloadKeyGoalID] = string(goalID)
	}
	if dodID != "" {
		payload[PayloadKeyDoDID] = string(dodID)
	}
	if title != "" {
		payload[PayloadKeyTitle] = title
	}
	if summary != "" {
		payload[PayloadKeySummary] = summary
	}
	if sourceType != "" {
		payload[PayloadKeySourceType] = sourceType
	}
	if sourceRef != "" {
		payload[PayloadKeySourceRef] = sourceRef
	}
	return payload
}

func ArtifactPayload(
	resultID common.ResultID,
	stepID common.StepID,
	toolCallID common.ToolCallID,
	patchID common.PatchID,
	validationResultID common.ValidationResultID,
	artifactType string,
	name string,
	summary string,
	uri string,
	contentType string,
	state common.ArtifactState,
	evidenceRefs []string,
) map[string]any {
	payload := map[string]any{
		PayloadKeyArtifactType: artifactType,
		PayloadKeyState:        string(state),
		PayloadKeyEvidenceRefs: append([]string(nil), evidenceRefs...),
	}
	if resultID != "" {
		payload[PayloadKeyResultID] = string(resultID)
	}
	if stepID != "" {
		payload["stepId"] = string(stepID)
	}
	if toolCallID != "" {
		payload["toolCallId"] = string(toolCallID)
	}
	if patchID != "" {
		payload["patchId"] = string(patchID)
	}
	if validationResultID != "" {
		payload["validationResultId"] = string(validationResultID)
	}
	if name != "" {
		payload[PayloadKeyName] = name
	}
	if summary != "" {
		payload[PayloadKeySummary] = summary
	}
	if uri != "" {
		payload[PayloadKeyURI] = uri
	}
	if contentType != "" {
		payload[PayloadKeyContentType] = contentType
	}
	return payload
}

func PatchPayload(
	patchID common.PatchID,
	stepID common.StepID,
	toolCallID common.ToolCallID,
	patchType string,
	targetRef string,
	summary string,
	state common.PatchState,
	addedLines int,
	deletedLines int,
	contentHash string,
	diffHash string,
	approvalID common.ApprovalID,
	risk string,
	errText string,
) map[string]any {
	payload := map[string]any{
		PayloadKeyState:        string(state),
		PayloadKeyPatchType:    patchType,
		PayloadKeyTargetRef:    targetRef,
		PayloadKeyAddedLines:   addedLines,
		PayloadKeyDeletedLines: deletedLines,
	}
	if patchID != "" {
		payload[PayloadKeyPatchID] = string(patchID)
	}
	if stepID != "" {
		payload[PayloadKeyStepID] = string(stepID)
	}
	if toolCallID != "" {
		payload[PayloadKeyToolCallID] = string(toolCallID)
	}
	if summary != "" {
		payload[PayloadKeySummary] = summary
	}
	if contentHash != "" {
		payload[PayloadKeyContentHash] = contentHash
	}
	if diffHash != "" {
		payload[PayloadKeyDiffHash] = diffHash
	}
	if approvalID != "" {
		payload[PayloadKeyApprovalID] = string(approvalID)
	}
	if risk != "" {
		payload[PayloadKeyRisk] = risk
	}
	if errText != "" {
		payload[PayloadKeyError] = errText
	}
	return payload
}

func MemoryPayload(memoryType string, summary string, content string, sourceType string, sourceRef string, tags []string, importance int, confidence int, visibility string) map[string]any {
	payload := map[string]any{
		PayloadKeyMemoryType: memoryType,
		PayloadKeySummary:    summary,
		PayloadKeySourceType: sourceType,
		PayloadKeyTags:       append([]string(nil), tags...),
		PayloadKeyImportance: importance,
		PayloadKeyConfidence: confidence,
		PayloadKeyVisibility: visibility,
	}
	if content != "" {
		payload[PayloadKeyContent] = content
	}
	if sourceRef != "" {
		payload[PayloadKeySourceRef] = sourceRef
	}
	return payload
}

func MemoryRecallPayload(traceID common.MemoryRecallTraceID, stage string, source string, policy string, status string, providerID string, operation string, retrievedMemoryIDs []common.MemoryID, reason string) map[string]any {
	payload := map[string]any{
		PayloadKeyTraceID:            string(traceID),
		PayloadKeyStage:              stage,
		PayloadKeySourceType:         source,
		PayloadKeyPolicy:             policy,
		PayloadKeyStatus:             status,
		PayloadKeyRetrievedMemoryIDs: memoryIDsToStrings(retrievedMemoryIDs),
	}
	if providerID != "" {
		payload[PayloadKeyProviderID] = providerID
	}
	if operation != "" {
		payload[PayloadKeyOperation] = operation
	}
	if reason != "" {
		payload[PayloadKeyReason] = reason
	}
	return payload
}

func AutoGovernancePayload(
	traceID common.AutoGovernanceTraceID,
	itemType string,
	itemID string,
	action string,
	riskLevel string,
	confidence float64,
	policyMatched string,
	policyBlocked string,
	budgetBefore int,
	budgetAfter int,
	llmProviderID string,
	llmModel string,
	llmDecisionPreview string,
	promptFingerprint string,
	status string,
	reason string,
	escalationReason string,
) map[string]any {
	payload := map[string]any{
		PayloadKeyTraceID:            string(traceID),
		PayloadKeyItemType:           itemType,
		PayloadKeyItemID:             itemID,
		PayloadKeyAction:             action,
		PayloadKeyRisk:               riskLevel,
		PayloadKeyConfidenceScore:    confidence,
		PayloadKeyPolicyMatched:      policyMatched,
		PayloadKeyPolicyBlocked:      policyBlocked,
		PayloadKeyBudgetBefore:       budgetBefore,
		PayloadKeyBudgetAfter:        budgetAfter,
		PayloadKeyLLMProviderID:      llmProviderID,
		PayloadKeyLLMModel:           llmModel,
		PayloadKeyLLMDecisionPreview: llmDecisionPreview,
		PayloadKeyPromptFingerprint:  promptFingerprint,
		PayloadKeyStatus:             status,
	}
	if reason != "" {
		payload[PayloadKeyReason] = reason
	}
	if escalationReason != "" {
		payload[PayloadKeyEscalationReason] = escalationReason
	}
	return payload
}

func LLMStreamProgressPayload(
	workspaceID common.WorkspaceID,
	runID common.RunID,
	stepID common.StepID,
	providerID string,
	model string,
	chunkCount int,
	receivedBytes int,
	elapsedMs int,
	status string,
	summary string,
) map[string]any {
	payload := map[string]any{
		PayloadKeyWorkspaceID:   string(workspaceID),
		PayloadKeyRunID:         string(runID),
		PayloadKeyStepID:        string(stepID),
		PayloadKeyProviderID:    providerID,
		PayloadKeyModel:         model,
		PayloadKeyChunkCount:    chunkCount,
		PayloadKeyReceivedBytes: receivedBytes,
		PayloadKeyElapsedMs:     elapsedMs,
		PayloadKeyStatus:        status,
	}
	if summary != "" {
		payload[PayloadKeySummary] = summary
	}
	return payload
}

func clonePayloadMap(payload map[string]any) map[string]any {
	if payload == nil {
		return nil
	}
	ret := make(map[string]any, len(payload))
	for k, v := range payload {
		ret[k] = v
	}
	return ret
}

func DecodeWorkspacePayload(payload map[string]any) (WorkspacePayloadData, bool) {
	state, ok := stringField(payload, PayloadKeyState)
	if !ok {
		return WorkspacePayloadData{}, false
	}
	title, _ := optionalStringField(payload, PayloadKeyTitle)
	description, _ := optionalStringField(payload, PayloadKeyDescription)
	deliverableDirectory, _ := optionalStringField(payload, PayloadKeyDeliverableDirectory)
	internalDirectory, _ := optionalStringField(payload, PayloadKeyInternalDirectory)
	activeRunID, _ := optionalStringField(payload, PayloadKeyActiveRunID)
	return WorkspacePayloadData{
		Title:                title,
		Description:          description,
		DeliverableDirectory: deliverableDirectory,
		InternalDirectory:    internalDirectory,
		State:                common.WorkspaceState(state),
		ActiveRunID:          common.RunID(activeRunID),
	}, true
}

func DecodeRunPayload(payload map[string]any) (RunPayloadData, bool) {
	state, ok := stringField(payload, PayloadKeyState)
	if !ok {
		return RunPayloadData{}, false
	}
	goalID, ok := optionalStringField(payload, PayloadKeyGoalID)
	if !ok {
		return RunPayloadData{}, false
	}
	dodID, ok := optionalStringField(payload, PayloadKeyDoDID)
	if !ok {
		return RunPayloadData{}, false
	}
	blockingTicketID, _ := optionalStringField(payload, PayloadKeyBlockingTicketID)
	blockingApprovalID, _ := optionalStringField(payload, PayloadKeyBlockingApprovalID)
	failureReason, ok := optionalStringField(payload, PayloadKeyFailureReason)
	if !ok {
		return RunPayloadData{}, false
	}
	return RunPayloadData{
		GoalID:             common.GoalID(goalID),
		DoDID:              common.DoDID(dodID),
		State:              common.RunState(state),
		BlockingTicketID:   common.TicketID(blockingTicketID),
		BlockingApprovalID: common.ApprovalID(blockingApprovalID),
		FailureReason:      failureReason,
	}, true
}

func DecodeGoalPayload(payload map[string]any) (GoalPayloadData, bool) {
	text, ok := stringField(payload, PayloadKeyText)
	if !ok {
		return GoalPayloadData{}, false
	}
	definitionRevision, _ := optionalStringField(payload, PayloadKeyDefinitionRevision)
	previousDefinitionHash, _ := optionalStringField(payload, PayloadKeyPreviousDefinitionHash)
	previousGoalID, _ := optionalStringField(payload, PayloadKeyPreviousGoalID)
	lineageState, _ := optionalStringField(payload, PayloadKeyLineageState)
	return GoalPayloadData{
		Text:                   text,
		DefinitionRevision:     definitionRevision,
		PreviousDefinitionHash: previousDefinitionHash,
		PreviousGoalID:         common.GoalID(previousGoalID),
		LineageState:           lineageState,
	}, true
}

func DecodeDoDPayload(payload map[string]any) (DoDPayloadData, bool) {
	items, ok := optionalStringSliceField(payload, PayloadKeyItems)
	if !ok || len(items) == 0 {
		return DoDPayloadData{}, false
	}
	state, ok := stringField(payload, PayloadKeyState)
	if !ok {
		return DoDPayloadData{}, false
	}
	definitionRevision, _ := optionalStringField(payload, PayloadKeyDefinitionRevision)
	previousDefinitionHash, _ := optionalStringField(payload, PayloadKeyPreviousDefinitionHash)
	previousDoDID, _ := optionalStringField(payload, PayloadKeyPreviousDoDID)
	lineageState, _ := optionalStringField(payload, PayloadKeyLineageState)
	return DoDPayloadData{
		Items:                  items,
		State:                  common.DoDState(state),
		DefinitionRevision:     definitionRevision,
		PreviousDefinitionHash: previousDefinitionHash,
		PreviousDoDID:          common.DoDID(previousDoDID),
		LineageState:           lineageState,
	}, true
}

func DecodeStepPayload(payload map[string]any) (StepPayloadData, bool) {
	kind, ok := stringField(payload, PayloadKeyKind)
	if !ok {
		return StepPayloadData{}, false
	}
	state, ok := stringField(payload, PayloadKeyState)
	if !ok {
		return StepPayloadData{}, false
	}
	input, ok := optionalMapField(payload, PayloadKeyInput)
	if !ok {
		return StepPayloadData{}, false
	}
	output, ok := optionalMapField(payload, PayloadKeyOutput)
	if !ok {
		return StepPayloadData{}, false
	}
	return StepPayloadData{
		Kind:   llmcommon.CanonicalRuntimeBlockKind(kind),
		State:  common.StepState(state),
		Input:  input,
		Output: output,
	}, true
}

func DecodeDecisionTicketPayload(payload map[string]any) (DecisionTicketPayloadData, bool) {
	reason, ok := stringField(payload, PayloadKeyReason)
	if !ok {
		return DecisionTicketPayloadData{}, false
	}
	state, ok := stringField(payload, PayloadKeyState)
	if !ok {
		return DecisionTicketPayloadData{}, false
	}
	return DecisionTicketPayloadData{
		Reason: reason,
		State:  common.DecisionTicketState(state),
	}, true
}

func DecodeDecisionResolutionPayload(payload map[string]any) (DecisionResolutionPayloadData, bool) {
	base, ok := DecodeDecisionTicketPayload(payload)
	if !ok {
		return DecisionResolutionPayloadData{}, false
	}
	resolutionID, ok := stringField(payload, PayloadKeyResolutionID)
	if !ok {
		return DecisionResolutionPayloadData{}, false
	}
	resolutionType, ok := stringField(payload, PayloadKeyType)
	if !ok {
		return DecisionResolutionPayloadData{}, false
	}
	resolutionPayload, ok := optionalMapField(payload, PayloadKeyPayload)
	if !ok {
		return DecisionResolutionPayloadData{}, false
	}
	return DecisionResolutionPayloadData{
		DecisionTicketPayloadData: base,
		ResolutionID:              common.ResolutionID(resolutionID),
		Type:                      resolutionType,
		Payload:                   resolutionPayload,
	}, true
}

func DecodeApprovalPayload(payload map[string]any) (ApprovalPayloadData, bool) {
	reason, ok := stringField(payload, PayloadKeyReason)
	if !ok {
		return ApprovalPayloadData{}, false
	}
	state, ok := stringField(payload, PayloadKeyState)
	if !ok {
		return ApprovalPayloadData{}, false
	}
	action, ok := optionalStringField(payload, PayloadKeyAction)
	if !ok {
		return ApprovalPayloadData{}, false
	}
	context, ok := optionalMapField(payload, PayloadKeyContext)
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

func DecodeResultPayload(payload map[string]any) (ResultPayloadData, bool) {
	state, ok := stringField(payload, PayloadKeyState)
	if !ok {
		return ResultPayloadData{}, false
	}
	goalID, ok := optionalStringField(payload, PayloadKeyGoalID)
	if !ok {
		return ResultPayloadData{}, false
	}
	dodID, ok := optionalStringField(payload, PayloadKeyDoDID)
	if !ok {
		return ResultPayloadData{}, false
	}
	title, ok := optionalStringField(payload, PayloadKeyTitle)
	if !ok {
		return ResultPayloadData{}, false
	}
	summary, ok := optionalStringField(payload, PayloadKeySummary)
	if !ok {
		return ResultPayloadData{}, false
	}
	sourceType, ok := optionalStringField(payload, PayloadKeySourceType)
	if !ok {
		return ResultPayloadData{}, false
	}
	sourceRef, ok := optionalStringField(payload, PayloadKeySourceRef)
	if !ok {
		return ResultPayloadData{}, false
	}
	evidenceRefs, ok := optionalStringSliceField(payload, PayloadKeyEvidenceRefs)
	if !ok {
		return ResultPayloadData{}, false
	}
	artifactRefs, ok := optionalStringSliceField(payload, PayloadKeyArtifactRefs)
	if !ok {
		return ResultPayloadData{}, false
	}
	return ResultPayloadData{
		GoalID:       common.GoalID(goalID),
		DoDID:        common.DoDID(dodID),
		State:        common.ResultState(state),
		Title:        title,
		Summary:      summary,
		SourceType:   sourceType,
		SourceRef:    sourceRef,
		EvidenceRefs: evidenceRefs,
		ArtifactRefs: artifactRefs,
	}, true
}

func DecodeArtifactPayload(payload map[string]any) (ArtifactPayloadData, bool) {
	artifactType, ok := stringField(payload, PayloadKeyArtifactType)
	if !ok {
		return ArtifactPayloadData{}, false
	}
	state, ok := stringField(payload, PayloadKeyState)
	if !ok {
		return ArtifactPayloadData{}, false
	}
	resultID, ok := optionalStringField(payload, PayloadKeyResultID)
	if !ok {
		return ArtifactPayloadData{}, false
	}
	stepID, ok := optionalStringField(payload, "stepId")
	if !ok {
		return ArtifactPayloadData{}, false
	}
	toolCallID, ok := optionalStringField(payload, "toolCallId")
	if !ok {
		return ArtifactPayloadData{}, false
	}
	patchID, ok := optionalStringField(payload, "patchId")
	if !ok {
		return ArtifactPayloadData{}, false
	}
	validationResultID, ok := optionalStringField(payload, "validationResultId")
	if !ok {
		return ArtifactPayloadData{}, false
	}
	name, ok := optionalStringField(payload, PayloadKeyName)
	if !ok {
		return ArtifactPayloadData{}, false
	}
	summary, ok := optionalStringField(payload, PayloadKeySummary)
	if !ok {
		return ArtifactPayloadData{}, false
	}
	uri, ok := optionalStringField(payload, PayloadKeyURI)
	if !ok {
		return ArtifactPayloadData{}, false
	}
	contentType, ok := optionalStringField(payload, PayloadKeyContentType)
	if !ok {
		return ArtifactPayloadData{}, false
	}
	evidenceRefs, ok := optionalStringSliceField(payload, PayloadKeyEvidenceRefs)
	if !ok {
		return ArtifactPayloadData{}, false
	}
	return ArtifactPayloadData{
		ResultID:           common.ResultID(resultID),
		StepID:             common.StepID(stepID),
		ToolCallID:         common.ToolCallID(toolCallID),
		PatchID:            common.PatchID(patchID),
		ValidationResultID: common.ValidationResultID(validationResultID),
		ArtifactType:       artifactType,
		Name:               name,
		Summary:            summary,
		URI:                uri,
		ContentType:        contentType,
		State:              common.ArtifactState(state),
		EvidenceRefs:       evidenceRefs,
	}, true
}

func DecodePatchPayload(payload map[string]any) (PatchPayloadData, bool) {
	state, ok := stringField(payload, PayloadKeyState)
	if !ok {
		return PatchPayloadData{}, false
	}
	patchType, ok := optionalStringField(payload, PayloadKeyPatchType)
	if !ok {
		return PatchPayloadData{}, false
	}
	targetRef, ok := optionalStringField(payload, PayloadKeyTargetRef)
	if !ok {
		return PatchPayloadData{}, false
	}
	patchID, ok := optionalStringField(payload, PayloadKeyPatchID)
	if !ok {
		return PatchPayloadData{}, false
	}
	stepID, ok := optionalStringField(payload, PayloadKeyStepID)
	if !ok {
		return PatchPayloadData{}, false
	}
	toolCallID, ok := optionalStringField(payload, PayloadKeyToolCallID)
	if !ok {
		return PatchPayloadData{}, false
	}
	summary, ok := optionalStringField(payload, PayloadKeySummary)
	if !ok {
		return PatchPayloadData{}, false
	}
	addedLines, ok := optionalIntField(payload, PayloadKeyAddedLines)
	if !ok {
		return PatchPayloadData{}, false
	}
	deletedLines, ok := optionalIntField(payload, PayloadKeyDeletedLines)
	if !ok {
		return PatchPayloadData{}, false
	}
	contentHash, ok := optionalStringField(payload, PayloadKeyContentHash)
	if !ok {
		return PatchPayloadData{}, false
	}
	diffHash, ok := optionalStringField(payload, PayloadKeyDiffHash)
	if !ok {
		return PatchPayloadData{}, false
	}
	approvalID, ok := optionalStringField(payload, PayloadKeyApprovalID)
	if !ok {
		return PatchPayloadData{}, false
	}
	risk, ok := optionalStringField(payload, PayloadKeyRisk)
	if !ok {
		return PatchPayloadData{}, false
	}
	errText, ok := optionalStringField(payload, PayloadKeyError)
	if !ok {
		return PatchPayloadData{}, false
	}
	return PatchPayloadData{
		PatchID:      common.PatchID(patchID),
		StepID:       common.StepID(stepID),
		ToolCallID:   common.ToolCallID(toolCallID),
		PatchType:    patchType,
		TargetRef:    targetRef,
		Summary:      summary,
		State:        common.PatchState(state),
		AddedLines:   addedLines,
		DeletedLines: deletedLines,
		ContentHash:  contentHash,
		DiffHash:     diffHash,
		ApprovalID:   common.ApprovalID(approvalID),
		Risk:         risk,
		Error:        errText,
	}, true
}

func DecodeMemoryPayload(payload map[string]any) (MemoryPayloadData, bool) {
	memoryType, ok := stringField(payload, PayloadKeyMemoryType)
	if !ok {
		return MemoryPayloadData{}, false
	}
	summary, ok := stringField(payload, PayloadKeySummary)
	if !ok {
		return MemoryPayloadData{}, false
	}
	sourceType, ok := stringField(payload, PayloadKeySourceType)
	if !ok {
		return MemoryPayloadData{}, false
	}
	content, ok := optionalStringField(payload, PayloadKeyContent)
	if !ok {
		return MemoryPayloadData{}, false
	}
	sourceRef, ok := optionalStringField(payload, PayloadKeySourceRef)
	if !ok {
		return MemoryPayloadData{}, false
	}
	tags, ok := optionalStringSliceField(payload, PayloadKeyTags)
	if !ok {
		return MemoryPayloadData{}, false
	}
	importance, ok := optionalIntField(payload, PayloadKeyImportance)
	if !ok {
		return MemoryPayloadData{}, false
	}
	confidence, ok := optionalIntField(payload, PayloadKeyConfidence)
	if !ok {
		return MemoryPayloadData{}, false
	}
	visibility, ok := optionalStringField(payload, PayloadKeyVisibility)
	if !ok {
		return MemoryPayloadData{}, false
	}
	return MemoryPayloadData{
		MemoryType: memoryType,
		Summary:    summary,
		Content:    content,
		SourceType: sourceType,
		SourceRef:  sourceRef,
		Tags:       tags,
		Importance: importance,
		Confidence: confidence,
		Visibility: visibility,
	}, true
}

func DecodeMemoryRecallPayload(payload map[string]any) (MemoryRecallPayloadData, bool) {
	traceID, ok := stringField(payload, PayloadKeyTraceID)
	if !ok {
		return MemoryRecallPayloadData{}, false
	}
	stage, ok := stringField(payload, PayloadKeyStage)
	if !ok {
		return MemoryRecallPayloadData{}, false
	}
	source, ok := stringField(payload, PayloadKeySourceType)
	if !ok {
		return MemoryRecallPayloadData{}, false
	}
	policy, ok := stringField(payload, PayloadKeyPolicy)
	if !ok {
		return MemoryRecallPayloadData{}, false
	}
	status, ok := stringField(payload, PayloadKeyStatus)
	if !ok {
		return MemoryRecallPayloadData{}, false
	}
	providerID, ok := optionalStringField(payload, PayloadKeyProviderID)
	if !ok {
		return MemoryRecallPayloadData{}, false
	}
	operation, ok := optionalStringField(payload, PayloadKeyOperation)
	if !ok {
		return MemoryRecallPayloadData{}, false
	}
	retrievedMemoryIDs, ok := optionalStringSliceField(payload, PayloadKeyRetrievedMemoryIDs)
	if !ok {
		return MemoryRecallPayloadData{}, false
	}
	reason, ok := optionalStringField(payload, PayloadKeyReason)
	if !ok {
		return MemoryRecallPayloadData{}, false
	}
	return MemoryRecallPayloadData{
		TraceID:            common.MemoryRecallTraceID(traceID),
		Stage:              stage,
		Source:             source,
		Policy:             policy,
		Status:             status,
		ProviderID:         providerID,
		Operation:          operation,
		RetrievedMemoryIDs: stringSliceToMemoryIDs(retrievedMemoryIDs),
		Reason:             reason,
	}, true
}

func DecodeAutoGovernancePayload(payload map[string]any) (AutoGovernancePayloadData, bool) {
	traceID, ok := stringField(payload, PayloadKeyTraceID)
	if !ok {
		return AutoGovernancePayloadData{}, false
	}
	itemType, ok := stringField(payload, PayloadKeyItemType)
	if !ok {
		return AutoGovernancePayloadData{}, false
	}
	itemID, ok := stringField(payload, PayloadKeyItemID)
	if !ok {
		return AutoGovernancePayloadData{}, false
	}
	action, ok := stringField(payload, PayloadKeyAction)
	if !ok {
		return AutoGovernancePayloadData{}, false
	}
	riskLevel, ok := stringField(payload, PayloadKeyRisk)
	if !ok {
		return AutoGovernancePayloadData{}, false
	}
	confidence, ok := optionalFloatField(payload, PayloadKeyConfidenceScore)
	if !ok {
		return AutoGovernancePayloadData{}, false
	}
	policyMatched, ok := optionalStringField(payload, PayloadKeyPolicyMatched)
	if !ok {
		return AutoGovernancePayloadData{}, false
	}
	policyBlocked, ok := optionalStringField(payload, PayloadKeyPolicyBlocked)
	if !ok {
		return AutoGovernancePayloadData{}, false
	}
	budgetBefore, ok := optionalIntField(payload, PayloadKeyBudgetBefore)
	if !ok {
		return AutoGovernancePayloadData{}, false
	}
	budgetAfter, ok := optionalIntField(payload, PayloadKeyBudgetAfter)
	if !ok {
		return AutoGovernancePayloadData{}, false
	}
	llmProviderID, ok := optionalStringField(payload, PayloadKeyLLMProviderID)
	if !ok {
		return AutoGovernancePayloadData{}, false
	}
	llmModel, ok := optionalStringField(payload, PayloadKeyLLMModel)
	if !ok {
		return AutoGovernancePayloadData{}, false
	}
	llmDecisionPreview, ok := optionalStringField(payload, PayloadKeyLLMDecisionPreview)
	if !ok {
		return AutoGovernancePayloadData{}, false
	}
	promptFingerprint, ok := optionalStringField(payload, PayloadKeyPromptFingerprint)
	if !ok {
		return AutoGovernancePayloadData{}, false
	}
	status, ok := stringField(payload, PayloadKeyStatus)
	if !ok {
		return AutoGovernancePayloadData{}, false
	}
	reason, ok := optionalStringField(payload, PayloadKeyReason)
	if !ok {
		return AutoGovernancePayloadData{}, false
	}
	escalationReason, ok := optionalStringField(payload, PayloadKeyEscalationReason)
	if !ok {
		return AutoGovernancePayloadData{}, false
	}
	return AutoGovernancePayloadData{
		TraceID:            common.AutoGovernanceTraceID(traceID),
		ItemType:           itemType,
		ItemID:             itemID,
		Action:             action,
		RiskLevel:          riskLevel,
		Confidence:         confidence,
		PolicyMatched:      policyMatched,
		PolicyBlocked:      policyBlocked,
		BudgetBefore:       budgetBefore,
		BudgetAfter:        budgetAfter,
		PromptFingerprint:  promptFingerprint,
		LLMProviderID:      llmProviderID,
		LLMModel:           llmModel,
		LLMDecisionPreview: llmDecisionPreview,
		Status:             status,
		Reason:             reason,
		EscalationReason:   escalationReason,
	}, true
}

func DecodeLLMStreamProgressPayload(payload map[string]any) (LLMStreamProgressPayloadData, bool) {
	workspaceID, ok := stringField(payload, PayloadKeyWorkspaceID)
	if !ok {
		return LLMStreamProgressPayloadData{}, false
	}
	runID, ok := stringField(payload, PayloadKeyRunID)
	if !ok {
		return LLMStreamProgressPayloadData{}, false
	}
	stepID, ok := stringField(payload, PayloadKeyStepID)
	if !ok {
		return LLMStreamProgressPayloadData{}, false
	}
	providerID, ok := optionalStringField(payload, PayloadKeyProviderID)
	if !ok {
		return LLMStreamProgressPayloadData{}, false
	}
	model, ok := optionalStringField(payload, PayloadKeyModel)
	if !ok {
		return LLMStreamProgressPayloadData{}, false
	}
	chunkCount, ok := optionalIntField(payload, PayloadKeyChunkCount)
	if !ok {
		return LLMStreamProgressPayloadData{}, false
	}
	receivedBytes, ok := optionalIntField(payload, PayloadKeyReceivedBytes)
	if !ok {
		return LLMStreamProgressPayloadData{}, false
	}
	elapsedMs, ok := optionalIntField(payload, PayloadKeyElapsedMs)
	if !ok {
		return LLMStreamProgressPayloadData{}, false
	}
	status, ok := stringField(payload, PayloadKeyStatus)
	if !ok {
		return LLMStreamProgressPayloadData{}, false
	}
	summary, ok := optionalStringField(payload, PayloadKeySummary)
	if !ok {
		return LLMStreamProgressPayloadData{}, false
	}
	return LLMStreamProgressPayloadData{
		WorkspaceID:   common.WorkspaceID(workspaceID),
		RunID:         common.RunID(runID),
		StepID:        common.StepID(stepID),
		ProviderID:    providerID,
		Model:         model,
		ChunkCount:    chunkCount,
		ReceivedBytes: receivedBytes,
		ElapsedMs:     elapsedMs,
		Status:        status,
		Summary:       summary,
	}, true
}

func LLMUsagePayload(data LLMUsagePayloadData) map[string]any {
	payload := map[string]any{
		PayloadKeyWorkspaceID:   string(data.WorkspaceID),
		PayloadKeyRunID:         string(data.RunID),
		PayloadKeyStepID:        string(data.StepID),
		PayloadKeyProviderID:    data.ProviderID,
		PayloadKeyProvider:      data.Provider,
		PayloadKeyModel:         data.Model,
		PayloadKeyOperation:     data.Operation,
		PayloadKeyInputTokens:   data.InputTokens,
		PayloadKeyOutputTokens:  data.OutputTokens,
		PayloadKeyTotalTokens:   data.TotalTokens,
		PayloadKeyEstimated:     data.Estimated,
		PayloadKeyUsageSource:   data.UsageSource,
		PayloadKeyRequestBytes:  data.RequestBytes,
		PayloadKeyResponseBytes: data.ResponseBytes,
		PayloadKeyStreaming:     data.Streaming,
		PayloadKeyStartedAt:     data.StartedAt,
		PayloadKeyCompletedAt:   data.CompletedAt,
		PayloadKeyDurationMs:    data.DurationMs,
		PayloadKeyStatus:        data.Status,
	}
	if data.FailureClass != "" {
		payload[PayloadKeyFailureClass] = data.FailureClass
	}
	if len(data.ContextBudget) > 0 {
		payload[PayloadKeyContextBudget] = clonePayloadMap(data.ContextBudget)
	}
	if len(data.CacheUsage) > 0 {
		payload[PayloadKeyCacheUsage] = clonePayloadMap(data.CacheUsage)
	}
	return payload
}

func DecodeLLMUsagePayload(payload map[string]any) (LLMUsagePayloadData, bool) {
	workspaceID, ok := stringField(payload, PayloadKeyWorkspaceID)
	if !ok {
		return LLMUsagePayloadData{}, false
	}
	runID, ok := stringField(payload, PayloadKeyRunID)
	if !ok {
		return LLMUsagePayloadData{}, false
	}
	stepID, ok := stringField(payload, PayloadKeyStepID)
	if !ok {
		return LLMUsagePayloadData{}, false
	}
	providerID, ok := optionalStringField(payload, PayloadKeyProviderID)
	if !ok {
		return LLMUsagePayloadData{}, false
	}
	provider, ok := optionalStringField(payload, PayloadKeyProvider)
	if !ok {
		return LLMUsagePayloadData{}, false
	}
	model, ok := optionalStringField(payload, PayloadKeyModel)
	if !ok {
		return LLMUsagePayloadData{}, false
	}
	operation, ok := optionalStringField(payload, PayloadKeyOperation)
	if !ok {
		return LLMUsagePayloadData{}, false
	}
	inputTokens, ok := optionalIntField(payload, PayloadKeyInputTokens)
	if !ok {
		return LLMUsagePayloadData{}, false
	}
	outputTokens, ok := optionalIntField(payload, PayloadKeyOutputTokens)
	if !ok {
		return LLMUsagePayloadData{}, false
	}
	totalTokens, ok := optionalIntField(payload, PayloadKeyTotalTokens)
	if !ok {
		return LLMUsagePayloadData{}, false
	}
	estimated, ok := optionalBoolField(payload, PayloadKeyEstimated)
	if !ok {
		return LLMUsagePayloadData{}, false
	}
	usageSource, ok := optionalStringField(payload, PayloadKeyUsageSource)
	if !ok {
		return LLMUsagePayloadData{}, false
	}
	requestBytes, ok := optionalIntField(payload, PayloadKeyRequestBytes)
	if !ok {
		return LLMUsagePayloadData{}, false
	}
	responseBytes, ok := optionalIntField(payload, PayloadKeyResponseBytes)
	if !ok {
		return LLMUsagePayloadData{}, false
	}
	streaming, ok := optionalBoolField(payload, PayloadKeyStreaming)
	if !ok {
		return LLMUsagePayloadData{}, false
	}
	contextBudget, ok := optionalMapField(payload, PayloadKeyContextBudget)
	if !ok {
		return LLMUsagePayloadData{}, false
	}
	cacheUsage, ok := optionalMapField(payload, PayloadKeyCacheUsage)
	if !ok {
		return LLMUsagePayloadData{}, false
	}
	startedAt, ok := optionalStringField(payload, PayloadKeyStartedAt)
	if !ok {
		return LLMUsagePayloadData{}, false
	}
	completedAt, ok := optionalStringField(payload, PayloadKeyCompletedAt)
	if !ok {
		return LLMUsagePayloadData{}, false
	}
	durationMs, ok := optionalIntField(payload, PayloadKeyDurationMs)
	if !ok {
		return LLMUsagePayloadData{}, false
	}
	status, ok := stringField(payload, PayloadKeyStatus)
	if !ok {
		return LLMUsagePayloadData{}, false
	}
	failureClass, ok := optionalStringField(payload, PayloadKeyFailureClass)
	if !ok {
		return LLMUsagePayloadData{}, false
	}
	return LLMUsagePayloadData{
		WorkspaceID:   common.WorkspaceID(workspaceID),
		RunID:         common.RunID(runID),
		StepID:        common.StepID(stepID),
		ProviderID:    providerID,
		Provider:      provider,
		Model:         model,
		Operation:     operation,
		InputTokens:   inputTokens,
		OutputTokens:  outputTokens,
		TotalTokens:   totalTokens,
		Estimated:     estimated,
		UsageSource:   usageSource,
		RequestBytes:  requestBytes,
		ResponseBytes: responseBytes,
		Streaming:     streaming,
		ContextBudget: contextBudget,
		CacheUsage:    cacheUsage,
		StartedAt:     startedAt,
		CompletedAt:   completedAt,
		DurationMs:    durationMs,
		Status:        status,
		FailureClass:  failureClass,
	}, true
}

func memoryIDsToStrings(input []common.MemoryID) []string {
	ret := make([]string, 0, len(input))
	for _, item := range input {
		ret = append(ret, string(item))
	}
	return ret
}

func stringSliceToMemoryIDs(input []string) []common.MemoryID {
	ret := make([]common.MemoryID, 0, len(input))
	for _, item := range input {
		ret = append(ret, common.MemoryID(item))
	}
	return ret
}

func stringField(payload map[string]any, key string) (string, bool) {
	if payload == nil {
		return "", false
	}
	value, ok := payload[key]
	if !ok {
		return "", false
	}
	text, ok := value.(string)
	if !ok {
		return "", false
	}
	return text, true
}

func optionalStringField(payload map[string]any, key string) (string, bool) {
	if payload == nil {
		return "", true
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return "", true
	}
	text, ok := value.(string)
	if !ok {
		return "", false
	}
	return text, true
}

func optionalMapField(payload map[string]any, key string) (map[string]any, bool) {
	if payload == nil {
		return nil, true
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return nil, true
	}
	ret, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	return clonePayloadMap(ret), true
}

func optionalBoolField(payload map[string]any, key string) (bool, bool) {
	if payload == nil {
		return false, true
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return false, true
	}
	typed, ok := value.(bool)
	if !ok {
		return false, false
	}
	return typed, true
}

func optionalFloatField(payload map[string]any, key string) (float64, bool) {
	if payload == nil {
		return 0, true
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return 0, true
	}
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

func optionalStringSliceField(payload map[string]any, key string) ([]string, bool) {
	if payload == nil {
		return nil, true
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return nil, true
	}
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...), true
	case []any:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, false
			}
			ret = append(ret, text)
		}
		return ret, true
	default:
		return nil, false
	}
}

func optionalIntField(payload map[string]any, key string) (int, bool) {
	if payload == nil {
		return 0, true
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return 0, true
	}
	switch typed := value.(type) {
	case int:
		return typed, true
	case int8:
		return int(typed), true
	case int16:
		return int(typed), true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	default:
		return 0, false
	}
}
