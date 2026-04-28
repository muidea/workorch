package events

import "github.com/muidea/workorch/internal/pkg/common"

const (
	PayloadKeyTitle              = "title"
	PayloadKeyDescription        = "description"
	PayloadKeyWorkingDirectory   = "workingDirectory"
	PayloadKeyGoalID             = "goalId"
	PayloadKeyDoDID              = "doDId"
	PayloadKeyText               = "text"
	PayloadKeyItems              = "items"
	PayloadKeyState              = "state"
	PayloadKeyActiveRunID        = "activeRunId"
	PayloadKeyBlockingTicketID   = "blockingTicketId"
	PayloadKeyBlockingApprovalID = "blockingApprovalId"
	PayloadKeyFailureReason      = "failureReason"
	PayloadKeyKind               = "kind"
	PayloadKeyInput              = "input"
	PayloadKeyOutput             = "output"
	PayloadKeyReason             = "reason"
	PayloadKeyResolutionID       = "resolutionId"
	PayloadKeyType               = "type"
	PayloadKeyPayload            = "payload"
	PayloadKeySummary            = "summary"
	PayloadKeySourceType         = "sourceType"
	PayloadKeySourceRef          = "sourceRef"
	PayloadKeyEvidenceRefs       = "evidenceRefs"
	PayloadKeyArtifactRefs       = "artifactRefs"
	PayloadKeyResultID           = "resultId"
	PayloadKeyArtifactType       = "artifactType"
	PayloadKeyName               = "name"
	PayloadKeyURI                = "uri"
	PayloadKeyContentType        = "contentType"
)

type WorkspacePayloadData struct {
	Title            string
	Description      string
	WorkingDirectory string
	State            common.WorkspaceState
	ActiveRunID      common.RunID
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
	Text string
}

type DoDPayloadData struct {
	Items []string
	State common.DoDState
}

func GoalPayload(text string) map[string]any {
	return map[string]any{
		PayloadKeyText: text,
	}
}

func DoDPayload(items []string, state common.DoDState) map[string]any {
	return map[string]any{
		PayloadKeyItems: append([]string(nil), items...),
		PayloadKeyState: string(state),
	}
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
	Reason string
	State  common.ApprovalState
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

func WorkspacePayload(title string, description string, workingDirectory string, state common.WorkspaceState, activeRunID common.RunID) map[string]any {
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
	if workingDirectory != "" {
		payload[PayloadKeyWorkingDirectory] = workingDirectory
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
		PayloadKeyKind:  kind,
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

func ApprovalPayload(reason string, state common.ApprovalState) map[string]any {
	return map[string]any{
		PayloadKeyReason: reason,
		PayloadKeyState:  string(state),
	}
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
	workingDirectory, _ := optionalStringField(payload, PayloadKeyWorkingDirectory)
	activeRunID, _ := optionalStringField(payload, PayloadKeyActiveRunID)
	return WorkspacePayloadData{
		Title:            title,
		Description:      description,
		WorkingDirectory: workingDirectory,
		State:            common.WorkspaceState(state),
		ActiveRunID:      common.RunID(activeRunID),
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
	return GoalPayloadData{Text: text}, true
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
	return DoDPayloadData{Items: items, State: common.DoDState(state)}, true
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
		Kind:   kind,
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
	return ApprovalPayloadData{
		Reason: reason,
		State:  common.ApprovalState(state),
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
