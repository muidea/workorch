package orchestrator

import (
	"fmt"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func resultAssessmentWithRuntimeFacts(run *runModel.Run, assessment map[string]any, summary string, patchDetails []resultPatchDetail) (map[string]any, string) {
	if assessment == nil {
		return nil, strings.TrimSpace(summary)
	}
	ret := cloneStepPayload(assessment)
	fact, ok := appliedWorkspaceChangeFact(run, patchDetails)
	if !ok {
		return ret, strings.TrimSpace(summary)
	}
	summary = runtimeFactCorrectedText(summary, fact)
	ret["summary"] = runtimeFactCorrectedText(stringFromMap(ret, "summary"), fact)
	for _, key := range []string{"goalOutcome", "implementationSummary"} {
		value := stringFromMap(ret, key)
		if runtimeTextConflictsWithAppliedWorkspaceChange(value) || strings.TrimSpace(value) == "" {
			ret[key] = fact
		}
	}
	return ret, summary
}

func runtimeFactCorrectedText(text string, fact string) string {
	text = strings.TrimSpace(text)
	fact = strings.TrimSpace(fact)
	if fact == "" {
		return text
	}
	if text == "" || runtimeTextConflictsWithAppliedWorkspaceChange(text) {
		return fact
	}
	return text
}

func runtimeTextConflictsWithAppliedWorkspaceChange(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	if normalized == "" {
		return false
	}
	for _, pattern := range []string{
		"无新增可执行",
		"未新增可执行",
		"没有新增可执行",
		"无新的可执行",
		"未产生新的可执行",
		"无新增状态",
		"未新增状态",
		"没有新增状态",
		"不包含新的执行性修复",
		"不包含新的可执行",
		"仅执行 assessment-only",
		"仅进行 assessment-only",
		"仅做 assessment-only",
		"只执行 assessment-only",
		"本次仅执行 assessment-only",
		"本步为评估收敛",
		"no new executable",
		"no executable work",
		"no new execution",
		"no state-changing",
		"no state changing",
		"no new state change",
		"without new executable",
		"assessment-only, no",
		"assessment-only with no",
	} {
		if strings.Contains(normalized, pattern) {
			return true
		}
	}
	return strings.Contains(normalized, "assessment-only") &&
		(strings.Contains(normalized, "only") || strings.Contains(normalized, "仅") || strings.Contains(normalized, "只"))
}

func appliedWorkspaceChangeFact(run *runModel.Run, patchDetails []resultPatchDetail) (string, bool) {
	applied := appliedWorkspaceChangeDetails(patchDetails)
	if len(applied) == 0 {
		return "", false
	}
	targets := make([]string, 0, len(applied))
	seen := map[string]bool{}
	for _, patch := range applied {
		target := strings.TrimSpace(patch.TargetRef)
		if target == "" || seen[target] {
			continue
		}
		seen[target] = true
		targets = append(targets, target)
	}
	targetText := strings.Join(targets, ", ")
	if len(targets) > 3 {
		targetText = strings.Join(targets[:3], ", ") + fmt.Sprintf(" and %d more", len(targets)-3)
	}
	if targetText == "" {
		targetText = "workspace targets"
	}
	if prefersChinese(runDialogLanguage(run)) {
		return fmt.Sprintf("验收已完成；runtime 记录到 %d 个已应用的 workspace 变更（%s），交付物索引和 workspace-change-diff artifact 已保留对应证据。", len(applied), targetText), true
	}
	return fmt.Sprintf("Acceptance completed; the runtime recorded %d applied workspace change(s) (%s), with evidence preserved in the deliverable manifest and workspace-change-diff artifact.", len(applied), targetText), true
}

func appliedWorkspaceChangeDetails(patchDetails []resultPatchDetail) []resultPatchDetail {
	if len(patchDetails) == 0 {
		return nil
	}
	ret := make([]resultPatchDetail, 0, len(patchDetails))
	for _, patch := range patchDetails {
		if patch.State != common.PatchStateApplied {
			continue
		}
		if strings.TrimSpace(patch.TargetRef) == "" &&
			strings.TrimSpace(patch.Diff) == "" &&
			strings.TrimSpace(patch.Content) == "" {
			continue
		}
		ret = append(ret, patch)
	}
	return ret
}
