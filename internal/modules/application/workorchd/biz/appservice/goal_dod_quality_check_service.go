package appservice

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type GoalDoDQualityCheckCommand struct {
	WorkspaceID string   `json:"workspaceId"`
	GoalID      string   `json:"goalId,omitempty"`
	DoDID       string   `json:"doDId,omitempty"`
	GoalText    string   `json:"goalText,omitempty"`
	DoDItems    []string `json:"doDItems,omitempty"`
}

type GoalDoDQualityIssue struct {
	Code       string `json:"code"`
	Severity   string `json:"severity"`
	Target     string `json:"target"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

type GoalDoDQualityCheck struct {
	WorkspaceID        string                `json:"workspaceId"`
	GoalID             string                `json:"goalId,omitempty"`
	DoDID              string                `json:"doDId,omitempty"`
	GoalText           string                `json:"goalText,omitempty"`
	DoDItems           []string              `json:"doDItems,omitempty"`
	Executable         bool                  `json:"executable"`
	Verifiable         bool                  `json:"verifiable"`
	ExecutionScore     int                   `json:"executionScore"`
	VerificationScore  int                   `json:"verificationScore"`
	Status             string                `json:"status"`
	MissingConstraints []string              `json:"missingConstraints,omitempty"`
	Suggestions        []string              `json:"suggestions,omitempty"`
	Issues             []GoalDoDQualityIssue `json:"issues,omitempty"`
}

func CheckGoalDoDQualityWithSession(ctx context.Context, session *RuntimeSession, cmd GoalDoDQualityCheckCommand) (ret GoalDoDQualityCheck, err error) {
	exports, err := session.Exports()
	if err != nil {
		return GoalDoDQualityCheck{}, err
	}
	if exports.Repositories.Goals == nil || exports.Repositories.DoDs == nil {
		return GoalDoDQualityCheck{}, common.ErrInvalidArgument
	}
	defer session.CloseOnReturn(ctx, &err)

	workspaceID := common.WorkspaceID(strings.TrimSpace(cmd.WorkspaceID))
	if workspaceID == "" {
		return GoalDoDQualityCheck{}, common.ErrInvalidArgument
	}

	goalID := common.GoalID(strings.TrimSpace(cmd.GoalID))
	dodID := common.DoDID(strings.TrimSpace(cmd.DoDID))
	goalText, resolvedGoalID, err := resolveGoalForQualityCheck(ctx, exports.Repositories.Goals, workspaceID, goalID, cmd.GoalText)
	if err != nil {
		return GoalDoDQualityCheck{}, err
	}
	dodItems, resolvedDoDID, err := resolveDoDForQualityCheck(ctx, exports.Repositories.DoDs, workspaceID, dodID, cmd.DoDItems)
	if err != nil {
		return GoalDoDQualityCheck{}, err
	}

	ret = evaluateGoalDoDQuality(workspaceID, resolvedGoalID, resolvedDoDID, goalText, dodItems)
	return ret, nil
}

func CheckGoalDoDQualityWithBackend(ctx context.Context, backend string, dataRoot string, cmd GoalDoDQualityCheckCommand) (GoalDoDQualityCheck, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return GoalDoDQualityCheck{}, err
	}
	return CheckGoalDoDQualityWithSession(ctx, session, cmd)
}

func CheckGoalDoDQualityFromEnv(ctx context.Context, cmd GoalDoDQualityCheckCommand) (GoalDoDQualityCheck, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return GoalDoDQualityCheck{}, err
	}
	return CheckGoalDoDQualityWithSession(ctx, session, cmd)
}

func resolveGoalForQualityCheck(
	ctx context.Context,
	repo interface {
		Get(context.Context, common.GoalID) (*workspacemodel.Goal, error)
		ListByWorkspaceID(context.Context, common.WorkspaceID) ([]*workspacemodel.Goal, error)
	},
	workspaceID common.WorkspaceID,
	requestedID common.GoalID,
	text string,
) (string, common.GoalID, error) {
	text = strings.TrimSpace(text)
	if text != "" {
		return text, requestedID, nil
	}
	if requestedID != "" {
		item, err := repo.Get(ctx, requestedID)
		if err != nil {
			return "", "", err
		}
		if item == nil {
			return "", "", common.ErrNotFound
		}
		return strings.TrimSpace(item.Text), item.ID, nil
	}
	item, err := selectWorkspaceExecutionGoal(ctx, repo.ListByWorkspaceID, nil, workspaceID, "")
	if err != nil {
		if errors.Is(err, common.ErrInvalidArgument) || errors.Is(err, common.ErrConflict) {
			return "", "", nil
		}
		return "", "", err
	}
	if item == nil {
		return "", "", nil
	}
	return strings.TrimSpace(item.Text), item.ID, nil
}

func resolveDoDForQualityCheck(
	ctx context.Context,
	repo interface {
		Get(context.Context, common.DoDID) (*workspacemodel.DoD, error)
		ListByWorkspaceID(context.Context, common.WorkspaceID) ([]*workspacemodel.DoD, error)
	},
	workspaceID common.WorkspaceID,
	requestedID common.DoDID,
	items []string,
) ([]string, common.DoDID, error) {
	items = normalizeDoDItems(items)
	if len(items) > 0 {
		return items, requestedID, nil
	}
	if requestedID != "" {
		item, err := repo.Get(ctx, requestedID)
		if err != nil {
			return nil, "", err
		}
		if item == nil {
			return nil, "", common.ErrNotFound
		}
		return normalizeDoDItems(item.Items), item.ID, nil
	}
	item, err := selectWorkspaceExecutionDoD(ctx, repo.ListByWorkspaceID, nil, workspaceID, "")
	if err != nil {
		if errors.Is(err, common.ErrInvalidArgument) || errors.Is(err, common.ErrConflict) {
			return nil, "", nil
		}
		return nil, "", err
	}
	if item == nil {
		return nil, "", nil
	}
	return normalizeDoDItems(item.Items), item.ID, nil
}

func evaluateGoalDoDQuality(
	workspaceID common.WorkspaceID,
	goalID common.GoalID,
	dodID common.DoDID,
	goalText string,
	dodItems []string,
) GoalDoDQualityCheck {
	ret := GoalDoDQualityCheck{
		WorkspaceID:       string(workspaceID),
		GoalID:            string(goalID),
		DoDID:             string(dodID),
		GoalText:          goalText,
		DoDItems:          append([]string(nil), dodItems...),
		ExecutionScore:    100,
		VerificationScore: 100,
	}

	goalTextNorm := normalizeQualityText(goalText)
	dodCombinedNorm := normalizeQualityText(strings.Join(dodItems, " "))

	var measurableCount int
	for _, item := range dodItems {
		itemNorm := normalizeQualityText(item)
		if containsAny(itemNorm, measurableDoDKeywords) {
			measurableCount++
		}
	}

	if strings.TrimSpace(goalText) == "" {
		ret.ExecutionScore -= 70
		ret.addIssue("goal_missing", "error", "goal", "缺少 Goal，无法判断执行目标。", "补充一个明确、单一的目标描述。")
	}
	if runeCount(goalText) > 0 && runeCount(goalText) < 8 {
		ret.ExecutionScore -= 15
		ret.addIssue("goal_too_short", "warning", "goal", "Goal 过短，可能缺少上下文。", "补充目标对象、范围和期望结果。")
	}
	if looksQuestionOnly(goalTextNorm) && !containsAny(goalTextNorm, actionKeywords) {
		ret.ExecutionScore -= 10
		ret.addIssue("goal_ambiguous", "warning", "goal", "Goal 更像问题陈述，执行动作不够明确。", "在 Goal 中补充要执行的动作和期望交付。")
	}

	if len(dodItems) == 0 {
		ret.ExecutionScore -= 30
		ret.VerificationScore -= 80
		ret.addIssue("dod_missing", "error", "dod", "缺少 DoD，无法验证完成标准。", "补充可验证的 DoD 条目。")
	}
	if len(dodItems) > 0 && len(dodItems) < 2 {
		ret.VerificationScore -= 15
		ret.addIssue("dod_too_few_items", "warning", "dod", "DoD 条目较少，可能无法覆盖验收范围。", "增加至少一条验证性条目（例如测试、输出或证据要求）。")
	}
	for idx, item := range dodItems {
		if runeCount(item) < 6 {
			ret.VerificationScore -= 5
			ret.addIssue("dod_item_too_short", "warning", "dod_item", "DoD 条目过短，可验证性不足。", "补充可观察结果或验证方式。")
			_ = idx
		}
	}

	if len(dodItems) > 0 && measurableCount == 0 {
		ret.VerificationScore -= 50
		ret.addIssue("dod_not_measurable", "error", "dod", "DoD 缺少可验证信号（命令、输出、证据或度量）。", "补充 build/test/lint、交付物或 evidenceRefs 要求。")
		ret.addMissingConstraint("可验证证据")
	}
	if len(dodItems) > 0 && measurableCount > 0 && measurableCount*2 < len(dodItems) {
		ret.VerificationScore -= 20
		ret.addIssue("dod_measurability_low", "warning", "dod", "DoD 中可验证条目占比偏低。", "将更多条目改为可执行、可验证的形式。")
	}

	if !containsAny(goalTextNorm+" "+dodCombinedNorm, scopeBoundaryKeywords) {
		ret.ExecutionScore -= 10
		ret.addMissingConstraint("范围边界")
		ret.addSuggestion("补充范围边界，说明包含与不包含内容。")
	}
	if !containsAny(goalTextNorm+" "+dodCombinedNorm, riskConstraintKeywords) {
		ret.VerificationScore -= 10
		ret.addMissingConstraint("风险与限制")
		ret.addSuggestion("补充风险、限制或已知依赖假设，便于验收判定。")
	}

	ret.ExecutionScore = clampScore(ret.ExecutionScore)
	ret.VerificationScore = clampScore(ret.VerificationScore)
	ret.Executable = ret.ExecutionScore >= 60 && !ret.hasError("goal_missing")
	ret.Verifiable = ret.VerificationScore >= 60 && len(dodItems) > 0 && measurableCount > 0 && !ret.hasError("dod_missing") && !ret.hasError("dod_not_measurable")
	if ret.Executable && ret.Verifiable {
		ret.Status = "ready"
	} else {
		ret.Status = "needs_refinement"
	}
	if len(ret.Suggestions) == 0 && ret.Status != "ready" {
		ret.addSuggestion("补充 Goal / DoD 的执行边界和验收证据，再重新发起预检。")
	}
	return ret
}

func (c *GoalDoDQualityCheck) hasError(code string) bool {
	for _, issue := range c.Issues {
		if issue.Code == code && issue.Severity == "error" {
			return true
		}
	}
	return false
}

func (c *GoalDoDQualityCheck) addIssue(code string, severity string, target string, message string, suggestion string) {
	c.Issues = append(c.Issues, GoalDoDQualityIssue{
		Code:       code,
		Severity:   severity,
		Target:     target,
		Message:    message,
		Suggestion: suggestion,
	})
	if suggestion != "" {
		c.addSuggestion(suggestion)
	}
}

func (c *GoalDoDQualityCheck) addMissingConstraint(value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	for _, existing := range c.MissingConstraints {
		if existing == value {
			return
		}
	}
	c.MissingConstraints = append(c.MissingConstraints, value)
}

func (c *GoalDoDQualityCheck) addSuggestion(value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	for _, existing := range c.Suggestions {
		if existing == value {
			return
		}
	}
	c.Suggestions = append(c.Suggestions, value)
}

func normalizeQualityText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer("-", " ", "_", " ", "/", " ", ".", " ", ",", " ", "，", " ", ":", " ", "：", " ", ";", " ", "；", " ", "\n", " ", "\t", " ")
	return strings.Join(strings.Fields(replacer.Replace(value)), " ")
}

func containsAny(text string, keywords []string) bool {
	for _, keyword := range keywords {
		if keyword != "" && strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func looksQuestionOnly(text string) bool {
	return strings.Contains(text, "?") || strings.Contains(text, "？")
}

func runeCount(value string) int {
	return utf8.RuneCountInString(strings.TrimSpace(value))
}

func clampScore(value int) int {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

var actionKeywords = []string{
	"implement", "add", "fix", "update", "refactor", "generate", "migrate", "optimize", "review", "document",
	"实现", "新增", "修复", "更新", "重构", "生成", "迁移", "优化", "审查", "编写",
}

var measurableDoDKeywords = []string{
	"test", "build", "lint", "diff", "artifact", "report", "evidence", "metric", "assert", "verify", "command", "output",
	"测试", "构建", "编译", "静态检查", "校验", "验证", "证据", "报告", "交付物", "输出", "命令",
}

var scopeBoundaryKeywords = []string{
	"scope", "boundary", "exclude", "only", "not include", "out of scope",
	"范围", "边界", "仅", "只", "不包含", "不修改", "排除",
}

var riskConstraintKeywords = []string{
	"risk", "limitation", "dependency", "assumption", "constraint",
	"风险", "限制", "依赖", "假设", "约束",
}
