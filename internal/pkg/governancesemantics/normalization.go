// Package governancesemantics provides owner-neutral, stateless normalization
// for operator decision and approval input. It deliberately contains no
// EventHub contracts, persistence, or state transitions.
package governancesemantics

import (
	"strings"
	"unicode"

	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

const (
	DecisionResolutionTypeReplan             = "replan"
	DecisionResolutionTypeRejectCurrentPath  = "reject_current_path"
	DecisionResolutionTypeProvideMissingInfo = "provide_missing_info"
)

const (
	IntentProvideMissingInfo = "provide_missing_info"
	IntentModifyGoal         = "modify_goal"
	IntentModifyDoD          = "modify_dod"
	IntentApproveExecution   = "approve_execution"
	IntentRejectExecution    = "reject_execution"
	IntentRequestReplan      = "request_replan"
	IntentRequestTerminate   = "request_terminate"
	IntentUnknown            = "unknown"
)

type DecisionNormalization struct {
	ResolutionType string
	Intent         string
	Confidence     float64
	RawInput       string
	Source         string
	Inferred       bool
}

type ApprovalNormalization struct {
	Action     string
	Intent     string
	Confidence float64
	RawInput   string
	Inferred   bool
}

func IsSupportedDecisionResolutionType(value string) bool {
	switch value {
	case DecisionResolutionTypeReplan,
		DecisionResolutionTypeRejectCurrentPath,
		DecisionResolutionTypeProvideMissingInfo:
		return true
	default:
		return false
	}
}

func NormalizeDecisionResolution(resolutionType string, payload workorcheventpayload.Payload) DecisionNormalization {
	rawInput := firstNonEmptyPayloadString(payload, "answer", "value", "input", "note", "text", "message")
	source := firstNonEmptyPayloadString(payload, "source", "operator", "entry", "client", "channel")
	if strings.TrimSpace(source) == "" {
		source = "unknown"
	}
	explicitType := strings.TrimSpace(resolutionType)
	if IsSupportedDecisionResolutionType(explicitType) {
		intent, confidence := intentFromDecisionTypeAndInput(explicitType, rawInput)
		return DecisionNormalization{
			ResolutionType: explicitType,
			Intent:         intent,
			Confidence:     confidence,
			RawInput:       strings.TrimSpace(rawInput),
			Source:         source,
			Inferred:       false,
		}
	}
	if explicitType != "" && !strings.EqualFold(explicitType, "auto") {
		return DecisionNormalization{
			Intent:     IntentUnknown,
			RawInput:   strings.TrimSpace(rawInput),
			Source:     source,
			Confidence: 0,
			Inferred:   false,
		}
	}
	inferredType, intent, confidence := inferDecisionResolutionFromText(rawInput)
	return DecisionNormalization{
		ResolutionType: inferredType,
		Intent:         intent,
		Confidence:     confidence,
		RawInput:       strings.TrimSpace(rawInput),
		Source:         source,
		Inferred:       true,
	}
}

func NormalizeApprovalAction(action string, rawInput string, reason string) ApprovalNormalization {
	action = strings.TrimSpace(strings.ToLower(action))
	if action == "approve" {
		return ApprovalNormalization{Action: "approve", Intent: IntentApproveExecution, Confidence: 1, RawInput: strings.TrimSpace(rawInput)}
	}
	if action == "reject" {
		return ApprovalNormalization{Action: "reject", Intent: IntentRejectExecution, Confidence: 1, RawInput: strings.TrimSpace(rawInput)}
	}
	candidate := strings.TrimSpace(rawInput)
	if candidate == "" {
		candidate = strings.TrimSpace(action)
	}
	normalized := semanticText(candidate)
	if normalized == "" {
		return ApprovalNormalization{}
	}
	if containsAnySemanticCue(normalized, approvalRejectCues) {
		return ApprovalNormalization{Action: "reject", Intent: IntentRejectExecution, Confidence: 0.93, RawInput: candidate, Inferred: true}
	}
	if containsAnySemanticCue(normalized, approvalApproveCues) {
		return ApprovalNormalization{Action: "approve", Intent: IntentApproveExecution, Confidence: 0.9, RawInput: candidate, Inferred: true}
	}
	if approvalReasonAsksForConfirmation(reason) && looksLikeContextualApproval(candidate, reason) {
		return ApprovalNormalization{Action: "approve", Intent: IntentProvideMissingInfo, Confidence: 0.72, RawInput: candidate, Inferred: true}
	}
	return ApprovalNormalization{}
}

func intentFromDecisionTypeAndInput(resolutionType string, rawInput string) (string, float64) {
	_, inferredIntent, inferredConfidence := inferDecisionResolutionFromText(rawInput)
	if inferredIntent != IntentProvideMissingInfo {
		switch resolutionType {
		case DecisionResolutionTypeProvideMissingInfo:
			return inferredIntent, inferredConfidence
		case DecisionResolutionTypeReplan:
			if inferredIntent == IntentRequestReplan {
				return inferredIntent, inferredConfidence
			}
		case DecisionResolutionTypeRejectCurrentPath:
			if inferredIntent == IntentRejectExecution || inferredIntent == IntentRequestTerminate {
				return inferredIntent, inferredConfidence
			}
		}
	}
	switch resolutionType {
	case DecisionResolutionTypeReplan:
		return IntentRequestReplan, 0.86
	case DecisionResolutionTypeRejectCurrentPath:
		return IntentRejectExecution, 0.86
	default:
		if strings.TrimSpace(rawInput) == "" {
			return IntentProvideMissingInfo, 0.6
		}
		return IntentProvideMissingInfo, 0.76
	}
}

func inferDecisionResolutionFromText(rawInput string) (string, string, float64) {
	normalized := semanticText(rawInput)
	switch {
	case normalized == "":
		return DecisionResolutionTypeProvideMissingInfo, IntentProvideMissingInfo, 0.6
	case containsAnySemanticCue(normalized, decisionReplanCues):
		return DecisionResolutionTypeReplan, IntentRequestReplan, 0.93
	case containsAnySemanticCue(normalized, decisionTerminateCues):
		return DecisionResolutionTypeRejectCurrentPath, IntentRequestTerminate, 0.95
	case containsAnySemanticCue(normalized, decisionRejectCues):
		return DecisionResolutionTypeRejectCurrentPath, IntentRejectExecution, 0.9
	case containsAnySemanticCue(normalized, decisionModifyDoDCues):
		return DecisionResolutionTypeProvideMissingInfo, IntentModifyDoD, 0.88
	case containsAnySemanticCue(normalized, decisionModifyGoalCues):
		return DecisionResolutionTypeProvideMissingInfo, IntentModifyGoal, 0.88
	case containsAnySemanticCue(normalized, decisionApproveCues):
		return DecisionResolutionTypeProvideMissingInfo, IntentApproveExecution, 0.78
	default:
		return DecisionResolutionTypeProvideMissingInfo, IntentProvideMissingInfo, 0.72
	}
}

func approvalReasonAsksForConfirmation(reason string) bool {
	normalized := semanticText(reason)
	return normalized != "" && containsAnySemanticCue(normalized, approvalReasonConfirmationCues)
}

func looksLikeContextualApproval(value string, reason string) bool {
	normalized := semanticText(value)
	if normalized == "" || containsAnySemanticCue(normalized, approvalRejectCues) {
		return false
	}
	if containsAnySemanticCue(normalized, approvalContextualCues) {
		return true
	}
	return containsAnySemanticCue(semanticText(reason), approvalReasonContextCues) && hasLettersOrDigits(value) && len([]rune(strings.TrimSpace(value))) >= 2
}

func semanticText(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func containsAnySemanticCue(value string, cues []string) bool {
	for _, cue := range cues {
		normalizedCue := semanticText(cue)
		if normalizedCue != "" && strings.Contains(value, normalizedCue) {
			return true
		}
	}
	return false
}

func hasLettersOrDigits(value string) bool {
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func firstNonEmptyPayloadString(payload workorcheventpayload.Payload, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload.String(key); ok {
			if value = strings.TrimSpace(value); value != "" {
				return value
			}
		}
	}
	return ""
}

var decisionReplanCues = []string{"replan", "replanning", "re-plan", "重新规划", "重新计划", "重做计划", "换方案", "重构计划", "重新构建", "rebuild plan"}
var decisionTerminateCues = []string{"terminate", "stop execution", "abort run", "cancel run", "exit workflow", "终止执行", "停止执行", "取消运行", "结束流程"}
var decisionRejectCues = []string{"reject", "reject current path", "decline", "refuse", "deny", "否决", "拒绝", "驳回", "不继续", "不执行", "不要继续", "not proceed"}
var decisionModifyGoalCues = []string{"modifygoal", "updategoal", "changegoal", "adjustgoal", "rewritegoal", "目标调整", "修改目标", "更新目标", "变更目标"}
var decisionModifyDoDCues = []string{"modifydod", "updatedod", "changedod", "adjustdod", "acceptancecriteria", "definitionofdone", "修改dod", "更新dod", "调整dod", "验收项调整", "修改验收"}
var decisionApproveCues = []string{"approve", "agree", "confirm", "continue", "proceed", "goahead", "yes", "同意", "确认", "继续", "可以执行", "请继续"}
var approvalRejectCues = []string{"不同意", "不批准", "不确认", "不允许", "不要", "拒绝", "取消", "停止", "终止", "驳回", "否决", "notapprove", "notagree", "reject", "deny", "disapprove", "cancel", "stop", "abort", "decline", "refuse"}
var approvalApproveCues = []string{"同意", "批准", "确认", "允许", "通过", "继续", "执行", "没问题", "没意见", "好的", "赞成", "yes", "approve", "allow", "ok", "okay", "confirm", "accept", "agree", "proceed", "continue", "goahead"}
var approvalReasonConfirmationCues = []string{"确认", "核对", "检查", "参数", "目标", "信息", "上下文", "调用", "工具", "接口", "confirm", "check", "parameter", "target", "context", "info", "call", "tool", "api"}
var approvalContextualCues = []string{"查询", "调用", "使用", "执行", "继续", "参数", "目标", "信息", "上下文", "接口", "工具", "api", "mcp", "query", "call", "use", "execute", "run", "continue", "parameter", "target", "context", "info", "tool"}
var approvalReasonContextCues = []string{"参数", "目标", "信息", "上下文", "parameter", "target", "context", "info"}
