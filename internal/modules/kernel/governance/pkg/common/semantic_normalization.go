package common

import (
	"strings"
	"unicode"

	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
)

const (
	GovernanceIntentProvideMissingInfo = "provide_missing_info"
	GovernanceIntentModifyGoal         = "modify_goal"
	GovernanceIntentModifyDoD          = "modify_dod"
	GovernanceIntentApproveExecution   = "approve_execution"
	GovernanceIntentRejectExecution    = "reject_execution"
	GovernanceIntentRequestReplan      = "request_replan"
	GovernanceIntentRequestTerminate   = "request_terminate"
	GovernanceIntentUnknown            = "unknown"
)

type DecisionSemanticNormalization struct {
	ResolutionType string
	Intent         string
	Confidence     float64
	RawInput       string
	Source         string
	Inferred       bool
}

type ApprovalSemanticNormalization struct {
	Action     string
	Intent     string
	Confidence float64
	RawInput   string
	Inferred   bool
}

func NormalizeDecisionResolution(resolutionType string, payload map[string]any) DecisionSemanticNormalization {
	rawInput := firstNonEmptyPayloadString(payload, "answer", "value", "input", "note", "text", "message")
	source := firstNonEmptyPayloadString(payload, "source", "operator", "entry", "client", "channel")
	if strings.TrimSpace(source) == "" {
		source = "unknown"
	}
	explicitType := strings.TrimSpace(resolutionType)
	if decisioncommon.IsSupportedResolutionType(explicitType) {
		intent, confidence := intentFromDecisionTypeAndInput(explicitType, rawInput)
		return DecisionSemanticNormalization{
			ResolutionType: explicitType,
			Intent:         intent,
			Confidence:     confidence,
			RawInput:       strings.TrimSpace(rawInput),
			Source:         source,
			Inferred:       false,
		}
	}
	if explicitType != "" && !strings.EqualFold(explicitType, "auto") {
		return DecisionSemanticNormalization{
			ResolutionType: "",
			Intent:         GovernanceIntentUnknown,
			Confidence:     0,
			RawInput:       strings.TrimSpace(rawInput),
			Source:         source,
			Inferred:       false,
		}
	}
	inferredType, intent, confidence := inferDecisionResolutionFromText(rawInput)
	return DecisionSemanticNormalization{
		ResolutionType: inferredType,
		Intent:         intent,
		Confidence:     confidence,
		RawInput:       strings.TrimSpace(rawInput),
		Source:         source,
		Inferred:       true,
	}
}

func NormalizeApprovalAction(action string, rawInput string, reason string) ApprovalSemanticNormalization {
	action = strings.TrimSpace(strings.ToLower(action))
	if action == "approve" {
		return ApprovalSemanticNormalization{
			Action:     "approve",
			Intent:     GovernanceIntentApproveExecution,
			Confidence: 1,
			RawInput:   strings.TrimSpace(rawInput),
			Inferred:   false,
		}
	}
	if action == "reject" {
		return ApprovalSemanticNormalization{
			Action:     "reject",
			Intent:     GovernanceIntentRejectExecution,
			Confidence: 1,
			RawInput:   strings.TrimSpace(rawInput),
			Inferred:   false,
		}
	}
	candidate := strings.TrimSpace(rawInput)
	if candidate == "" {
		candidate = strings.TrimSpace(action)
	}
	normalized := semanticText(candidate)
	if normalized == "" {
		return ApprovalSemanticNormalization{}
	}
	if containsAnySemanticCue(normalized, approvalRejectCues) {
		return ApprovalSemanticNormalization{
			Action:     "reject",
			Intent:     GovernanceIntentRejectExecution,
			Confidence: 0.93,
			RawInput:   strings.TrimSpace(candidate),
			Inferred:   true,
		}
	}
	if containsAnySemanticCue(normalized, approvalApproveCues) {
		return ApprovalSemanticNormalization{
			Action:     "approve",
			Intent:     GovernanceIntentApproveExecution,
			Confidence: 0.9,
			RawInput:   strings.TrimSpace(candidate),
			Inferred:   true,
		}
	}
	if approvalReasonAsksForConfirmation(reason) && looksLikeContextualApproval(candidate, reason) {
		return ApprovalSemanticNormalization{
			Action:     "approve",
			Intent:     GovernanceIntentProvideMissingInfo,
			Confidence: 0.72,
			RawInput:   strings.TrimSpace(candidate),
			Inferred:   true,
		}
	}
	return ApprovalSemanticNormalization{}
}

func intentFromDecisionTypeAndInput(resolutionType string, rawInput string) (string, float64) {
	_, inferredIntent, inferredConfidence := inferDecisionResolutionFromText(rawInput)
	if inferredIntent != GovernanceIntentProvideMissingInfo {
		switch resolutionType {
		case decisioncommon.ResolutionTypeProvideMissingInfo:
			return inferredIntent, inferredConfidence
		case decisioncommon.ResolutionTypeReplan:
			if inferredIntent == GovernanceIntentRequestReplan {
				return inferredIntent, inferredConfidence
			}
		case decisioncommon.ResolutionTypeRejectCurrentPath:
			if inferredIntent == GovernanceIntentRejectExecution || inferredIntent == GovernanceIntentRequestTerminate {
				return inferredIntent, inferredConfidence
			}
		}
	}
	switch resolutionType {
	case decisioncommon.ResolutionTypeReplan:
		return GovernanceIntentRequestReplan, 0.86
	case decisioncommon.ResolutionTypeRejectCurrentPath:
		return GovernanceIntentRejectExecution, 0.86
	default:
		if strings.TrimSpace(rawInput) == "" {
			return GovernanceIntentProvideMissingInfo, 0.6
		}
		return GovernanceIntentProvideMissingInfo, 0.76
	}
}

func inferDecisionResolutionFromText(rawInput string) (string, string, float64) {
	normalized := semanticText(rawInput)
	switch {
	case normalized == "":
		return decisioncommon.ResolutionTypeProvideMissingInfo, GovernanceIntentProvideMissingInfo, 0.6
	case containsAnySemanticCue(normalized, decisionReplanCues):
		return decisioncommon.ResolutionTypeReplan, GovernanceIntentRequestReplan, 0.93
	case containsAnySemanticCue(normalized, decisionTerminateCues):
		return decisioncommon.ResolutionTypeRejectCurrentPath, GovernanceIntentRequestTerminate, 0.95
	case containsAnySemanticCue(normalized, decisionRejectCues):
		return decisioncommon.ResolutionTypeRejectCurrentPath, GovernanceIntentRejectExecution, 0.9
	case containsAnySemanticCue(normalized, decisionModifyDoDCues):
		return decisioncommon.ResolutionTypeProvideMissingInfo, GovernanceIntentModifyDoD, 0.88
	case containsAnySemanticCue(normalized, decisionModifyGoalCues):
		return decisioncommon.ResolutionTypeProvideMissingInfo, GovernanceIntentModifyGoal, 0.88
	case containsAnySemanticCue(normalized, decisionApproveCues):
		return decisioncommon.ResolutionTypeProvideMissingInfo, GovernanceIntentApproveExecution, 0.78
	default:
		return decisioncommon.ResolutionTypeProvideMissingInfo, GovernanceIntentProvideMissingInfo, 0.72
	}
}

func approvalReasonAsksForConfirmation(reason string) bool {
	normalized := semanticText(reason)
	if normalized == "" {
		return false
	}
	return containsAnySemanticCue(normalized, approvalReasonConfirmationCues)
}

func looksLikeContextualApproval(value string, reason string) bool {
	normalized := semanticText(value)
	if normalized == "" {
		return false
	}
	if containsAnySemanticCue(normalized, approvalRejectCues) {
		return false
	}
	if containsAnySemanticCue(normalized, approvalContextualCues) {
		return true
	}
	if containsAnySemanticCue(semanticText(reason), approvalReasonContextCues) {
		return hasLettersOrDigits(value) && len([]rune(strings.TrimSpace(value))) >= 2
	}
	return false
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

func firstNonEmptyPayloadString(payload map[string]any, keys ...string) string {
	if payload == nil {
		return ""
	}
	for _, key := range keys {
		raw, ok := payload[key]
		if !ok {
			continue
		}
		if value, ok := raw.(string); ok {
			value = strings.TrimSpace(value)
			if value != "" {
				return value
			}
		}
	}
	return ""
}

var decisionReplanCues = []string{
	"replan", "replanning", "re-plan", "重新规划", "重新计划", "重做计划", "换方案", "重构计划", "重新构建", "rebuild plan",
}

var decisionTerminateCues = []string{
	"terminate", "stop execution", "abort run", "cancel run", "exit workflow", "终止执行", "停止执行", "取消运行", "结束流程",
}

var decisionRejectCues = []string{
	"reject", "reject current path", "decline", "refuse", "deny", "否决", "拒绝", "驳回", "不继续", "不执行", "不要继续", "not proceed",
}

var decisionModifyGoalCues = []string{
	"modifygoal", "updategoal", "changegoal", "adjustgoal", "rewritegoal", "目标调整", "修改目标", "更新目标", "变更目标",
}

var decisionModifyDoDCues = []string{
	"modifydod", "updatedod", "changedod", "adjustdod", "acceptancecriteria", "definitionofdone", "修改dod", "更新dod", "调整dod", "验收项调整", "修改验收",
}

var decisionApproveCues = []string{
	"approve", "agree", "confirm", "continue", "proceed", "goahead", "yes", "同意", "确认", "继续", "可以执行", "请继续",
}

var approvalRejectCues = []string{
	"不同意", "不批准", "不确认", "不允许", "不要", "拒绝", "取消", "停止", "终止", "驳回", "否决", "notapprove", "notagree", "reject", "deny", "disapprove", "cancel", "stop", "abort", "decline", "refuse",
}

var approvalApproveCues = []string{
	"同意", "批准", "确认", "允许", "通过", "继续", "执行", "没问题", "没意见", "好的", "赞成", "yes", "approve", "allow", "ok", "okay", "confirm", "accept", "agree", "proceed", "continue", "goahead",
}

var approvalReasonConfirmationCues = []string{
	"确认", "核对", "检查", "参数", "目标", "信息", "上下文", "调用", "工具", "接口", "confirm", "check", "parameter", "target", "context", "info", "call", "tool", "api",
}

var approvalContextualCues = []string{
	"查询", "调用", "使用", "执行", "继续", "参数", "目标", "信息", "上下文", "接口", "工具", "api", "mcp", "query", "call", "use", "execute", "run", "continue", "parameter", "target", "context", "info", "tool",
}

var approvalReasonContextCues = []string{
	"参数", "目标", "信息", "上下文", "parameter", "target", "context", "info",
}
