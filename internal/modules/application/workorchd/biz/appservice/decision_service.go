package appservice

import (
	"context"
	"errors"
	"fmt"
	"strings"

	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	governancecommon "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

const (
	decisionContextKeyScope                 = "scope"
	decisionContextScopeWorkorchRuntime     = "workorch_runtime"
	decisionContextScopeTargetWorkspace     = "target_workspace"
	decisionContextScopeExternalDependency  = "external_dependency"
	decisionContextKeyDependency            = "dependency"
	decisionContextDependencyLLMProvider    = "llm_provider"
	decisionContextKeyRecoveryPolicy        = "recoveryPolicy"
	decisionContextRecoveryPolicyConfigOnly = "system_config_required"
	decisionContextRecoveryPolicyRepairPlan = "runtime_repair_exhausted_requires_strategy"
)

type Decision struct {
	ID                string                 `json:"id"`
	WorkspaceID       string                 `json:"workspaceId"`
	RunID             string                 `json:"runId"`
	StepID            string                 `json:"stepId,omitempty"`
	Reason            string                 `json:"reason"`
	DecisionKind      string                 `json:"decisionKind,omitempty"`
	Question          string                 `json:"question,omitempty"`
	Context           map[string]any         `json:"context,omitempty"`
	State             string                 `json:"state"`
	ResolutionID      string                 `json:"resolutionId,omitempty"`
	ResolutionType    string                 `json:"resolutionType,omitempty"`
	ResolutionPayload map[string]any         `json:"resolutionPayload,omitempty"`
	AutoGovernance    *AutoGovernanceSummary `json:"autoGovernance,omitempty"`
	CreatedAt         string                 `json:"createdAt,omitempty"`
	UpdatedAt         string                 `json:"updatedAt,omitempty"`
}

type DecisionQuery struct {
	WorkspaceID   string
	RunID         string
	CursorAfterID string
	Limit         int
}

type DecisionPage struct {
	WorkspaceID       string     `json:"workspaceId,omitempty"`
	RunID             string     `json:"runId,omitempty"`
	Decisions         []Decision `json:"decisions,omitempty"`
	Limit             int        `json:"limit"`
	CursorAfterID     string     `json:"cursorAfterId,omitempty"`
	NextCursorAfterID string     `json:"nextCursorAfterId,omitempty"`
	HasMore           bool       `json:"hasMore"`
}

type ResolveDecisionCommand struct {
	ResolutionType    string         `json:"resolutionType"`
	ResolutionPayload map[string]any `json:"resolutionPayload,omitempty"`
}

type BatchResolveDecisionCommand struct {
	TicketIDs         []string       `json:"ticketIds"`
	ResolutionType    string         `json:"resolutionType"`
	ResolutionPayload map[string]any `json:"resolutionPayload,omitempty"`
}

func ReadDecisionWithSession(ctx context.Context, session *RuntimeSession, ticketID string) (ret Decision, err error) {
	hub, err := decisionDependenciesFromSession(session)
	if err != nil {
		return Decision{}, err
	}
	traces := autoGovernanceTraceReaderFromSession(session)
	defer session.CloseOnReturn(ctx, &err)

	item, err := readDecisionTicketByOwnerCommand(ctx, hub, common.TicketID(ticketID))
	if err != nil {
		return Decision{}, err
	}
	if err := ensureDecisionTicketInRuntimeWorkspace(ctx, item); err != nil {
		return Decision{}, err
	}
	resolution, err := resolutionForTicketByOwnerCommand(ctx, hub, item)
	if err != nil {
		return Decision{}, err
	}
	autoGovernance, err := latestAutoGovernanceSummary(ctx, traces, item.WorkspaceID, item.RunID, "ticket", string(item.ID))
	if err != nil {
		return Decision{}, err
	}
	return mapDecision(item, resolution, autoGovernance), nil
}

func ReadDecisionWithBackend(ctx context.Context, backend string, dataRoot string, ticketID string) (Decision, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return Decision{}, err
	}
	return ReadDecisionWithSession(ctx, session, ticketID)
}

func ReadDecisionFromEnv(ctx context.Context, ticketID string) (Decision, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return Decision{}, err
	}
	return ReadDecisionWithSession(ctx, session, ticketID)
}

func ListDecisionsWithSession(ctx context.Context, session *RuntimeSession, workspaceID string, runID string) (ret []Decision, err error) {
	hub, err := decisionDependenciesFromSession(session)
	if err != nil {
		return nil, err
	}
	traces := autoGovernanceTraceReaderFromSession(session)
	defer session.CloseOnReturn(ctx, &err)

	candidates, err := decisionTicketsForQuery(ctx, hub, workspaceID, runID)
	if err != nil {
		return nil, err
	}
	ret = make([]Decision, 0, len(candidates))
	for _, item := range candidates {
		resolution, resolutionErr := resolutionForTicketByOwnerCommand(ctx, hub, item)
		if resolutionErr != nil {
			return nil, resolutionErr
		}
		autoGovernance, autoErr := latestAutoGovernanceSummary(ctx, traces, item.WorkspaceID, item.RunID, "ticket", string(item.ID))
		if autoErr != nil {
			return nil, autoErr
		}
		ret = append(ret, mapDecision(item, resolution, autoGovernance))
	}
	return ret, nil
}

func ListDecisionsWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string, runID string) ([]Decision, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return nil, err
	}
	return ListDecisionsWithSession(ctx, session, workspaceID, runID)
}

func ListDecisionPageWithSession(ctx context.Context, session *RuntimeSession, query DecisionQuery) (ret DecisionPage, err error) {
	hub, err := decisionDependenciesFromSession(session)
	if err != nil {
		return DecisionPage{}, err
	}
	traces := autoGovernanceTraceReaderFromSession(session)
	defer session.CloseOnReturn(ctx, &err)

	filtered, err := decisionTicketsForQuery(ctx, hub, query.WorkspaceID, query.RunID)
	if err != nil {
		return DecisionPage{}, err
	}
	pageTickets, limit, cursorAfterID, nextCursor, hasMore := pageDecisionTickets(filtered, query)
	resolutionMap, err := resolutionsForTicketsByOwnerCommand(ctx, hub, pageTickets)
	if err != nil {
		return DecisionPage{}, err
	}
	decisions := make([]Decision, 0, len(pageTickets))
	for _, item := range pageTickets {
		summary, err := latestAutoGovernanceSummary(ctx, traces, item.WorkspaceID, item.RunID, "ticket", string(item.ID))
		if err != nil {
			return DecisionPage{}, err
		}
		decisions = append(decisions, projectDecisionView(mapDecision(item, resolutionMap[item.ID], summary)))
	}
	return DecisionPage{
		WorkspaceID:       strings.TrimSpace(query.WorkspaceID),
		RunID:             strings.TrimSpace(query.RunID),
		Decisions:         decisions,
		Limit:             limit,
		CursorAfterID:     cursorAfterID,
		NextCursorAfterID: nextCursor,
		HasMore:           hasMore,
	}, nil
}

func ListDecisionPageWithBackend(ctx context.Context, backend string, dataRoot string, query DecisionQuery) (DecisionPage, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return DecisionPage{}, err
	}
	return ListDecisionPageWithSession(ctx, session, query)
}

func ListDecisionsFromEnv(ctx context.Context, workspaceID string, runID string) ([]Decision, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return nil, err
	}
	return ListDecisionsWithSession(ctx, session, workspaceID, runID)
}

func ResolveDecisionWithSession(ctx context.Context, session *RuntimeSession, ticketID string, cmd ResolveDecisionCommand) (ret Decision, err error) {
	hub, err := decisionDependenciesFromSession(session)
	if err != nil {
		return Decision{}, err
	}
	traces := autoGovernanceTraceReaderFromSession(session)
	defer session.CloseOnReturn(ctx, &err)

	semantic := governancecommon.NormalizeDecisionResolution(cmd.ResolutionType, cmd.ResolutionPayload)
	if !decisioncommon.IsSupportedResolutionType(semantic.ResolutionType) {
		return Decision{}, common.ErrInvalidArgument
	}
	resolutionType := semantic.ResolutionType
	payload := withDecisionSemanticPayload(cmd.ResolutionPayload, semantic)
	ticket, err := readDecisionTicketByOwnerCommand(ctx, hub, common.TicketID(ticketID))
	if err != nil {
		return Decision{}, err
	}
	if err := ensureDecisionTicketInRuntimeWorkspace(ctx, ticket); err != nil {
		return Decision{}, err
	}
	if ticket.State != common.DecisionTicketStateOpen {
		return Decision{}, common.ErrConflict
	}
	payload, resolutionType = normalizeDecisionResolutionPayloadForTicket(ticket, payload, resolutionType)
	if err := validateDecisionResolutionPayloadForTicket(ticket, payload); err != nil {
		return Decision{}, err
	}
	payload = common.RedactSensitiveMap(payload)

	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicDecisionCommandEnsureResolved, "decision_ticket", ticketID, ticket.WorkspaceID, ticket.RunID, map[string]any{
		events.PayloadKeyResolutionID:     "resolution-" + ticketID + "-" + resolutionType,
		events.PayloadKeyBlockingTicketID: ticketID,
		events.PayloadKeyType:             resolutionType,
		events.PayloadKeyPayload:          payload,
	})
	if err != nil {
		return Decision{}, err
	}
	item, resolution, err := decisionResponseItems(response)
	if err != nil {
		return Decision{}, err
	}

	autoGovernance, err := latestAutoGovernanceSummary(ctx, traces, item.WorkspaceID, item.RunID, "ticket", string(item.ID))
	if err != nil {
		return Decision{}, err
	}
	return mapDecision(item, resolution, autoGovernance), nil
}

func BatchResolveDecisionsWithSession(ctx context.Context, session *RuntimeSession, cmd BatchResolveDecisionCommand) (ret []Decision, err error) {
	hub, err := decisionDependenciesFromSession(session)
	if err != nil {
		return nil, err
	}
	traces := autoGovernanceTraceReaderFromSession(session)
	defer session.CloseOnReturn(ctx, &err)

	semantic := governancecommon.NormalizeDecisionResolution(cmd.ResolutionType, cmd.ResolutionPayload)
	resolutionType := semantic.ResolutionType
	if !decisioncommon.IsSupportedResolutionType(resolutionType) || len(cmd.TicketIDs) == 0 {
		return nil, common.ErrInvalidArgument
	}
	payload := withDecisionSemanticPayload(cmd.ResolutionPayload, semantic)
	seen := make(map[string]struct{}, len(cmd.TicketIDs))
	for _, ticketID := range cmd.TicketIDs {
		if ticketID == "" {
			return nil, common.ErrInvalidArgument
		}
		if _, ok := seen[ticketID]; ok {
			return nil, common.ErrInvalidArgument
		}
		seen[ticketID] = struct{}{}
	}
	type ticketRead struct {
		item *decisionmodel.Ticket
		err  error
	}
	reads := make([]ticketRead, len(cmd.TicketIDs))
	for i, ticketID := range cmd.TicketIDs {
		reads[i].item, reads[i].err = readDecisionTicketByOwnerCommand(ctx, hub, common.TicketID(ticketID))
	}
	for _, r := range reads {
		if r.err != nil {
			return nil, r.err
		}
		if err := ensureDecisionTicketInRuntimeWorkspace(ctx, r.item); err != nil {
			return nil, err
		}
		if r.item.State != common.DecisionTicketStateOpen {
			return nil, common.ErrConflict
		}
		payload, resolutionType = normalizeDecisionResolutionPayloadForTicket(r.item, payload, resolutionType)
		if err := validateDecisionResolutionPayloadForTicket(r.item, payload); err != nil {
			return nil, err
		}
	}
	payload = common.RedactSensitiveMap(payload)

	ret = make([]Decision, 0, len(cmd.TicketIDs))
	for _, r := range reads {
		ticketID := string(r.item.ID)
		response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicDecisionCommandEnsureResolved, "decision_ticket", ticketID, r.item.WorkspaceID, r.item.RunID, map[string]any{
			events.PayloadKeyResolutionID:     "resolution-" + ticketID + "-" + resolutionType,
			events.PayloadKeyBlockingTicketID: ticketID,
			events.PayloadKeyType:             resolutionType,
			events.PayloadKeyPayload:          cloneDecisionPayload(payload),
		})
		if err != nil {
			return nil, err
		}
		item, resolution, err := decisionResponseItems(response)
		if err != nil {
			return nil, err
		}
		autoGovernance, autoErr := latestAutoGovernanceSummary(ctx, traces, item.WorkspaceID, item.RunID, "ticket", string(item.ID))
		if autoErr != nil {
			return nil, autoErr
		}
		ret = append(ret, mapDecision(item, resolution, autoGovernance))
	}
	return ret, nil
}

func ensureDecisionTicketInRuntimeWorkspace(ctx context.Context, item *decisionmodel.Ticket) error {
	if item == nil {
		return common.ErrNotFound
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if workspaceID == "" || item.WorkspaceID == workspaceID {
		return nil
	}
	return common.ErrNotFound
}

func withDecisionSemanticPayload(payload map[string]any, semantic governancecommon.DecisionSemanticNormalization) map[string]any {
	ret := cloneDecisionPayload(payload)
	if ret == nil {
		ret = map[string]any{}
	}
	ret["governanceSemantic"] = map[string]any{
		"resolutionType": semantic.ResolutionType,
		"intent":         semantic.Intent,
		"confidence":     semantic.Confidence,
		"rawInput":       semantic.RawInput,
		"source":         semantic.Source,
		"inferred":       semantic.Inferred,
		"version":        "v1",
	}
	return ret
}

func validateDecisionResolutionPayloadForTicket(ticket *decisionmodel.Ticket, payload map[string]any) error {
	if ticket == nil {
		return nil
	}
	if isWorkorchRuntimeLLMProviderRecoveryTicket(ticket) && containsInlineLLMProviderConfig(payload) {
		return fmt.Errorf("%w: WorkOrch runtime LLM provider configuration must be changed in WorkOrch system configuration, not pasted into a decision answer", common.ErrInvalidArgument)
	}
	if isRuntimeRepairExhaustionTicket(ticket) && isAmbiguousRuntimeRepairExhaustionResolution(payload) {
		return fmt.Errorf("%w: runtime repair exhaustion requires an explicit recovery action: retry_same_step, replan, provide_information, or terminate", common.ErrInvalidArgument)
	}
	return nil
}

func normalizeDecisionResolutionPayloadForTicket(ticket *decisionmodel.Ticket, payload map[string]any, resolutionType string) (map[string]any, string) {
	payload = normalizeCandidateSelectionResolutionPayloadForTicket(ticket, payload)
	if isWorkorchRuntimeLLMProviderRecoveryTicket(ticket) {
		action := inferRuntimeRepairRecoveryAction(payload)
		if action == "retry_same_step" {
			ret := cloneDecisionPayload(payload)
			if ret == nil {
				ret = map[string]any{}
			}
			ret["recoveryAction"] = action
			return ret, decisioncommon.ResolutionTypeProvideMissingInfo
		}
	}
	if !isRuntimeRepairExhaustionTicket(ticket) {
		return payload, resolutionType
	}
	action := inferRuntimeRepairRecoveryAction(payload)
	if action != "" {
		action = normalizeRuntimeRepairRecoveryActionByTicketPolicy(ticket, action)
	}
	if action == "" {
		return payload, resolutionType
	}
	ret := cloneDecisionPayload(payload)
	if ret == nil {
		ret = map[string]any{}
	}
	ret["recoveryAction"] = action
	switch action {
	case "replan":
		return ret, decisioncommon.ResolutionTypeReplan
	case "terminate":
		return ret, decisioncommon.ResolutionTypeRejectCurrentPath
	default:
		return ret, decisioncommon.ResolutionTypeProvideMissingInfo
	}
}

func normalizeRuntimeRepairRecoveryActionByTicketPolicy(ticket *decisionmodel.Ticket, action string) string {
	if ticket == nil || ticket.Context == nil {
		return action
	}
	allowed := runtimeRepairAllowedRecoveryActions(ticket.Context)
	if len(allowed) == 0 || containsRuntimeRepairRecoveryAction(allowed, action) {
		return action
	}
	for _, candidate := range []string{"replan", "provide_information", "retry_same_step", "terminate"} {
		if containsRuntimeRepairRecoveryAction(allowed, candidate) {
			return candidate
		}
	}
	return ""
}

func normalizeCandidateSelectionResolutionPayloadForTicket(ticket *decisionmodel.Ticket, payload map[string]any) map[string]any {
	if ticket == nil || len(ticket.Context) == 0 || len(payload) == 0 {
		return payload
	}
	semantic := decisionPayloadMap(payload["governanceSemantic"])
	if strings.TrimSpace(stringFromDecisionPayload(semantic, "intent")) != governancecommon.GovernanceIntentApproveExecution {
		return payload
	}
	candidates := decisionPayloadMap(ticket.Context["candidateValues"])
	if len(candidates) == 0 {
		return payload
	}
	ret := cloneDecisionPayload(payload)
	if ret == nil {
		ret = map[string]any{}
	}
	selected := make([]string, 0, len(candidates))
	for field, raw := range candidates {
		field = strings.TrimSpace(field)
		if field == "" || strings.TrimSpace(stringFromDecisionPayload(ret, field)) != "" {
			continue
		}
		if candidate := preferredContinuationCandidate(raw); candidate != "" {
			ret[field] = candidate
			selected = append(selected, field)
		}
	}
	if len(selected) == 0 {
		return payload
	}
	ret["selectedCandidateFields"] = selected
	ret["candidateSelectionMode"] = "semantic_continue"
	return ret
}

func preferredContinuationCandidate(value any) string {
	switch typed := value.(type) {
	case []string:
		for _, item := range typed {
			if isContinuationCandidate(item) {
				return strings.TrimSpace(item)
			}
		}
	case []any:
		for _, item := range typed {
			if candidate := preferredContinuationCandidate(item); candidate != "" {
				return candidate
			}
		}
	case map[string]any:
		for _, key := range []string{"value", "id", "mode", "target", "name"} {
			if candidate := preferredContinuationCandidate(typed[key]); candidate != "" {
				return candidate
			}
		}
	case string:
		if isContinuationCandidate(typed) {
			return strings.TrimSpace(typed)
		}
	}
	return ""
}

func isContinuationCandidate(value string) bool {
	normalized := normalizeRepairResolutionText(value)
	if normalized == "" {
		return false
	}
	for _, marker := range []string{
		"init", "initialize", "bootstrap", "fromscratch", "newscratch", "newproject",
		"currentworkspace", "currentdirectory",
		"初始化", "从零", "新项目", "当前工作区", "当前目录",
	} {
		if strings.Contains(normalized, normalizeRepairResolutionText(marker)) {
			return true
		}
	}
	return false
}

func isWorkorchRuntimeLLMProviderRecoveryTicket(ticket *decisionmodel.Ticket) bool {
	if ticket == nil {
		return false
	}
	scope := strings.TrimSpace(stringFromDecisionPayload(ticket.Context, decisionContextKeyScope))
	if scope == decisionContextScopeTargetWorkspace || scope == decisionContextScopeExternalDependency {
		return false
	}
	if scope == decisionContextScopeWorkorchRuntime {
		return isLLMProviderRecoveryTicket(ticket)
	}
	// Older tickets did not carry a scope. Preserve provider recovery protection
	// for those records while keeping scoped target-workspace tickets separate.
	return isLLMProviderRecoveryTicket(ticket)
}

func isLLMProviderRecoveryTicket(ticket *decisionmodel.Ticket) bool {
	if ticket == nil {
		return false
	}
	if dependency := strings.TrimSpace(stringFromDecisionPayload(ticket.Context, decisionContextKeyDependency)); dependency != "" {
		return dependency == decisionContextDependencyLLMProvider
	}
	if policy := strings.TrimSpace(stringFromDecisionPayload(ticket.Context, decisionContextKeyRecoveryPolicy)); policy != "" {
		return policy == decisionContextRecoveryPolicyConfigOnly
	}
	class := strings.TrimSpace(stringFromDecisionPayload(ticket.Context, "failureClass"))
	if strings.HasPrefix(class, "provider_") ||
		class == "context_too_large" ||
		class == "rate_limited" ||
		class == "auth_or_config_error" {
		return true
	}
	text := strings.ToLower(ticket.Question + "\n" + ticket.Reason)
	return strings.Contains(text, "llm provider") || strings.Contains(text, "provider retry")
}

func isRuntimeRepairExhaustionTicket(ticket *decisionmodel.Ticket) bool {
	if ticket == nil {
		return false
	}
	return strings.TrimSpace(stringFromDecisionPayload(ticket.Context, decisionContextKeyRecoveryPolicy)) == decisionContextRecoveryPolicyRepairPlan
}

func isAmbiguousRuntimeRepairExhaustionResolution(payload map[string]any) bool {
	if len(payload) == 0 {
		return true
	}
	if action := inferRuntimeRepairRecoveryAction(payload); action != "" {
		switch action {
		case "retry_same_step", "replan", "provide_information", "terminate":
			return false
		default:
			return true
		}
	}
	return true
}

func inferRuntimeRepairRecoveryAction(payload map[string]any) string {
	if len(payload) == 0 {
		return ""
	}
	if rawAction := strings.TrimSpace(stringFromDecisionPayload(payload, "recoveryAction")); rawAction != "" {
		return normalizeRuntimeRepairRecoveryAction(rawAction)
	}
	semantic := decisionPayloadMap(payload["governanceSemantic"])
	intent := strings.TrimSpace(stringFromDecisionPayload(semantic, "intent"))
	switch intent {
	case governancecommon.GovernanceIntentRequestReplan:
		return "replan"
	case governancecommon.GovernanceIntentRejectExecution, governancecommon.GovernanceIntentRequestTerminate:
		return "terminate"
	case governancecommon.GovernanceIntentApproveExecution:
		return "retry_same_step"
	}
	text := semanticTextForRepairResolution(payload)
	if action := normalizeRuntimeRepairRecoveryAction(text); action != "" {
		return action
	}
	if intent == governancecommon.GovernanceIntentProvideMissingInfo && strings.TrimSpace(text) != "" {
		return "provide_information"
	}
	if strings.TrimSpace(text) != "" {
		return "provide_information"
	}
	return ""
}

func normalizeRuntimeRepairRecoveryAction(value string) string {
	text := normalizeRepairResolutionText(value)
	switch {
	case text == "":
		return ""
	case containsAnyRepairResolutionCue(text, []string{
		"replan", "replanning", "重新规划", "重新计划", "重做计划", "换方案", "重构计划",
	}):
		return "replan"
	case containsAnyRepairResolutionCue(text, []string{
		"terminate", "stop", "abort", "cancel", "reject", "decline", "refuse", "deny",
		"终止", "停止", "取消", "拒绝", "驳回", "否决", "不继续", "不执行",
	}):
		return "terminate"
	case containsAnyRepairResolutionCue(text, []string{
		"retry_same_step", "retrysamestep", "retry", "rerun", "resume", "continue", "proceed", "goahead", "again",
		"重试", "重新尝试", "再次尝试", "继续", "继续执行", "重新执行", "恢复执行", "再试", "同意", "确认", "可以", "好的",
	}):
		return "retry_same_step"
	case containsAnyRepairResolutionCue(text, []string{
		"provide_information", "provideinfo", "information", "guidance", "instruction", "context",
		"补充信息", "提供信息", "实现指导", "修复指导", "上下文", "说明",
	}):
		return "provide_information"
	default:
		return ""
	}
}

func normalizeRepairResolutionText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "", "-", "", "_", "", "/", "", "\\", "", ".", "")
	return replacer.Replace(value)
}

func containsAnyRepairResolutionCue(value string, cues []string) bool {
	for _, cue := range cues {
		if normalized := normalizeRepairResolutionText(cue); normalized != "" && strings.Contains(value, normalized) {
			return true
		}
	}
	return false
}

func semanticTextForRepairResolution(payload map[string]any) string {
	semantic := decisionPayloadMap(payload["governanceSemantic"])
	if raw := strings.TrimSpace(stringFromDecisionPayload(semantic, "rawInput")); raw != "" {
		return strings.ToLower(strings.ReplaceAll(raw, " ", ""))
	}
	for _, key := range []string{"answer", "value", "input", "resumeInstruction"} {
		if raw := strings.TrimSpace(stringFromDecisionPayload(payload, key)); raw != "" {
			return strings.ToLower(strings.ReplaceAll(raw, " ", ""))
		}
	}
	return ""
}

func runtimeRepairAllowedRecoveryActions(input map[string]any) []string {
	if input == nil {
		return nil
	}
	value, ok := input["allowedRecoveryActions"]
	if !ok {
		return nil
	}
	switch typed := value.(type) {
	case []string:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			action := normalizeRuntimeRepairRecoveryAction(item)
			if action != "" {
				ret = append(ret, action)
			}
		}
		return ret
	case []any:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			raw, _ := item.(string)
			action := normalizeRuntimeRepairRecoveryAction(raw)
			if action != "" {
				ret = append(ret, action)
			}
		}
		return ret
	default:
		return nil
	}
}

func containsRuntimeRepairRecoveryAction(values []string, want string) bool {
	want = normalizeRuntimeRepairRecoveryAction(want)
	if want == "" {
		return false
	}
	for _, value := range values {
		if normalizeRuntimeRepairRecoveryAction(value) == want {
			return true
		}
	}
	return false
}

func containsInlineLLMProviderConfig(payload map[string]any) bool {
	if len(payload) == 0 {
		return false
	}
	hits := map[string]bool{}
	visitProviderConfigValue(payload, hits)
	if hits["apiKey"] || hits["secret"] {
		return true
	}
	configHits := 0
	for _, key := range []string{"provider", "baseURL", "model"} {
		if hits[key] {
			configHits++
		}
	}
	return configHits >= 2
}

func visitProviderConfigValue(value any, hits map[string]bool) {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			recordProviderConfigKey(key, hits)
			visitProviderConfigValue(nested, hits)
		}
	case []any:
		for _, nested := range typed {
			visitProviderConfigValue(nested, hits)
		}
	case []map[string]any:
		for _, nested := range typed {
			visitProviderConfigValue(nested, hits)
		}
	case string:
		recordProviderConfigText(typed, hits)
	}
}

func recordProviderConfigKey(key string, hits map[string]bool) {
	normalized := normalizeProviderConfigText(key)
	switch normalized {
	case "provider":
		hits["provider"] = true
	case "baseurl", "baseuri", "endpoint", "apiurl":
		hits["baseURL"] = true
	case "model", "modelid":
		hits["model"] = true
	case "apikey", "token", "secret":
		hits["apiKey"] = true
	}
}

func recordProviderConfigText(value string, hits map[string]bool) {
	lower := strings.ToLower(value)
	if strings.Contains(lower, "sk-") || strings.Contains(lower, "api_key") || strings.Contains(lower, "apikey") || strings.Contains(lower, "api key") {
		hits["apiKey"] = true
	}
	for _, marker := range []string{"provider", "baseurl", "base_url", "base url", "endpoint", "model"} {
		if strings.Contains(lower, marker) {
			recordProviderConfigKey(marker, hits)
		}
	}
}

func normalizeProviderConfigText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer("_", "", "-", "", " ", "", ".", "")
	return replacer.Replace(value)
}

func decisionPayloadMap(value any) map[string]any {
	typed, _ := value.(map[string]any)
	return typed
}

func stringFromDecisionPayload(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	value, _ := payload[key].(string)
	return strings.TrimSpace(value)
}

func ResolveDecisionWithBackend(ctx context.Context, backend string, dataRoot string, ticketID string, cmd ResolveDecisionCommand) (Decision, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return Decision{}, err
	}
	return ResolveDecisionWithSession(ctx, session, ticketID, cmd)
}

func BatchResolveDecisionsWithBackend(ctx context.Context, backend string, dataRoot string, cmd BatchResolveDecisionCommand) ([]Decision, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return nil, err
	}
	return BatchResolveDecisionsWithSession(ctx, session, cmd)
}

func ResolveDecisionFromEnv(ctx context.Context, ticketID string, cmd ResolveDecisionCommand) (Decision, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return Decision{}, err
	}
	return ResolveDecisionWithSession(ctx, session, ticketID, cmd)
}

func BatchResolveDecisionsFromEnv(ctx context.Context, cmd BatchResolveDecisionCommand) ([]Decision, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return nil, err
	}
	return BatchResolveDecisionsWithSession(ctx, session, cmd)
}

func decisionDependenciesFromSession(session *RuntimeSession) (events.Hub, error) {
	exports, err := session.Exports()
	if err != nil {
		return nil, err
	}
	if exports.Infrastructure.EventHub == nil {
		return nil, common.ErrInvalidArgument
	}
	return exports.Infrastructure.EventHub, nil
}

func decisionTicketsForQuery(ctx context.Context, hub events.Hub, workspaceID string, runID string) ([]*decisionmodel.Ticket, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	runID = strings.TrimSpace(runID)
	var (
		items []*decisionmodel.Ticket
		err   error
	)
	switch {
	case runID != "":
		items, err = listDecisionTicketsByRunOwnerCommand(ctx, hub, common.RunID(runID))
	case workspaceID != "":
		items, err = listDecisionTicketsByWorkspaceOwnerCommand(ctx, hub, common.WorkspaceID(workspaceID))
	default:
		items, err = listDecisionTicketsByOwnerCommand(ctx, hub)
	}
	if err != nil {
		return nil, err
	}
	filtered := make([]*decisionmodel.Ticket, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		if workspaceID != "" && string(item.WorkspaceID) != workspaceID {
			continue
		}
		if runID != "" && string(item.RunID) != runID {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered, nil
}

func resolutionForTicketByOwnerCommand(ctx context.Context, hub events.Hub, item *decisionmodel.Ticket) (*decisionmodel.Resolution, error) {
	if item == nil || hub == nil || item.State != common.DecisionTicketStateResolved {
		return nil, nil
	}
	resolution, err := readDecisionResolutionByTicketOwnerCommand(ctx, hub, item.ID)
	if errors.Is(err, common.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return resolution, nil
}

func resolutionsForTicketsByOwnerCommand(ctx context.Context, hub events.Hub, items []*decisionmodel.Ticket) (map[common.TicketID]*decisionmodel.Resolution, error) {
	ret := make(map[common.TicketID]*decisionmodel.Resolution, len(items))
	for _, item := range items {
		if item == nil || item.State != common.DecisionTicketStateResolved {
			continue
		}
		resolution, err := resolutionForTicketByOwnerCommand(ctx, hub, item)
		if err != nil {
			return nil, err
		}
		if resolution != nil {
			ret[item.ID] = resolution
		}
	}
	return ret, nil
}

func decisionResponseItems(response events.Envelope) (*decisionmodel.Ticket, *decisionmodel.Resolution, error) {
	ticket, ok := response.Payload[events.PayloadKeyTicket].(*decisionmodel.Ticket)
	if !ok || ticket == nil {
		return nil, nil, common.ErrInvalidArgument
	}
	resolution, ok := response.Payload[events.PayloadKeyResolution].(*decisionmodel.Resolution)
	if !ok || resolution == nil {
		return nil, nil, common.ErrInvalidArgument
	}
	return ticket, resolution, nil
}

func mapDecision(item *decisionmodel.Ticket, resolution *decisionmodel.Resolution, autoGovernance *AutoGovernanceSummary) Decision {
	if item == nil {
		return Decision{}
	}
	ret := Decision{
		ID:             string(item.ID),
		WorkspaceID:    string(item.WorkspaceID),
		RunID:          string(item.RunID),
		StepID:         string(item.StepID),
		Reason:         item.Reason,
		DecisionKind:   item.DecisionKind,
		Question:       item.Question,
		Context:        cloneDecisionPayload(item.Context),
		State:          string(item.State),
		AutoGovernance: autoGovernance,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	}
	if resolution != nil {
		ret.ResolutionID = string(resolution.ID)
		ret.ResolutionType = resolution.Type
		ret.ResolutionPayload = cloneDecisionPayload(resolution.Payload)
	}
	return ret
}

func cloneDecisionPayload(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for k, v := range input {
		ret[k] = v
	}
	return ret
}
