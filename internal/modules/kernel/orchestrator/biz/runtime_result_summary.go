package orchestrator

import (
	"fmt"
	"strconv"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func resultSummaryArtifactContent(run *runModel.Run, summary string, assessment map[string]any, evidenceRefs []string, artifactRefs []common.ArtifactID, evidenceDetails []resultEvidenceDetail) string {
	var builder strings.Builder
	language := runDialogLanguage(run)
	builder.WriteString("# ")
	builder.WriteString(localizedSummaryLabel(language, "title"))
	builder.WriteString("\n\n## ")
	builder.WriteString(localizedSummaryLabel(language, "goal_outcome"))
	builder.WriteString("\n\n")
	builder.WriteString(firstNonEmptyOrchestratorString(stringFromMap(assessment, "goalOutcome"), summary))
	builder.WriteString("\n\n## ")
	builder.WriteString(localizedSummaryLabel(language, "implementation_summary"))
	builder.WriteString("\n\n")
	builder.WriteString(firstNonEmptyOrchestratorString(stringFromMap(assessment, "implementationSummary"), summary))
	builder.WriteString("\n\n## ")
	builder.WriteString(localizedSummaryLabel(language, "verification_summary"))
	builder.WriteString("\n\n")
	builder.WriteString(firstNonEmptyOrchestratorString(stringFromMap(assessment, "verificationSummary"), verificationSummaryFromEvidence(language, evidenceRefs)))
	writeEvidenceDetailSection(&builder, language, evidenceDetails)
	if items, ok := doDAssessmentItemMaps(assessment["items"]); ok && len(items) > 0 {
		builder.WriteString("\n\n## ")
		builder.WriteString(localizedSummaryLabel(language, "dod_items"))
		builder.WriteString("\n\n")
		builder.WriteString(localizedSummaryLabel(language, "dod_table_header"))
		builder.WriteString("\n")
		builder.WriteString("| --- | --- | --- |\n")
		for _, item := range items {
			builder.WriteString(fmt.Sprintf(
				"| %s | %s | %s |\n",
				markdownTableCell(stringFromMap(item, "item")),
				markdownTableCell(stringFromMap(item, "state")),
				markdownTableCell(strings.Join(evidenceRefsFromValue(item["evidenceRefs"]), ", ")),
			))
		}
	}
	writeStringListSection(&builder, localizedSummaryLabel(language, "evidence_refs"), evidenceRefs)
	writeArtifactRefSection(&builder, localizedSummaryLabel(language, "artifact_refs"), artifactRefs)
	knownLimitations := stringSliceFromValue(assessment["knownLimitations"])
	if len(knownLimitations) == 0 {
		knownLimitations = localizedDefaultList(language, "known_limitations")
	}
	writeStringListSection(&builder, localizedSummaryLabel(language, "known_limitations"), knownLimitations)
	residualRisks := stringSliceFromValue(assessment["residualRisks"])
	if len(residualRisks) == 0 {
		residualRisks = localizedDefaultList(language, "residual_risks")
	}
	writeStringListSection(&builder, localizedSummaryLabel(language, "residual_risks"), residualRisks)
	nextActions := stringSliceFromValue(assessment["nextActions"])
	if len(nextActions) == 0 {
		nextActions = localizedDefaultList(language, "next_actions")
	}
	writeStringListSection(&builder, localizedSummaryLabel(language, "next_actions"), nextActions)
	return strings.TrimSpace(builder.String())
}

func writeArtifactRefSection(builder *strings.Builder, title string, values []common.ArtifactID) {
	if builder == nil || len(values) == 0 {
		return
	}
	builder.WriteString(fmt.Sprintf("\n\n## %s\n\n", title))
	for _, value := range values {
		if value == "" {
			continue
		}
		builder.WriteString("- `")
		builder.WriteString(markdownInlineCode(string(value)))
		builder.WriteString("`\n")
	}
}

func writeEvidenceDetailSection(builder *strings.Builder, language string, details []resultEvidenceDetail) {
	if builder == nil || len(details) == 0 {
		return
	}
	builder.WriteString("\n\n## ")
	builder.WriteString(localizedSummaryLabel(language, "evidence_details"))
	builder.WriteString("\n")
	for _, detail := range details {
		builder.WriteString("\n### ")
		builder.WriteString(markdownHeadingText(firstNonEmptyOrchestratorString(detail.Ref, detail.Kind)))
		builder.WriteString("\n")
		if detail.Kind != "" {
			builder.WriteString("\n- Kind: `")
			builder.WriteString(markdownInlineCode(detail.Kind))
			builder.WriteString("`\n")
		}
		if detail.ExitCode != "" {
			builder.WriteString("- Exit Code: `")
			builder.WriteString(markdownInlineCode(detail.ExitCode))
			builder.WriteString("`\n")
		}
		if detail.Command != "" {
			builder.WriteString("- Command: `")
			builder.WriteString(markdownInlineCode(detail.Command))
			builder.WriteString("`\n")
		}
		if detail.Summary != "" {
			builder.WriteString("\n")
			builder.WriteString(detail.Summary)
			builder.WriteString("\n")
		}
		if detail.Stdout != "" {
			builder.WriteString("\n")
			builder.WriteString(localizedSummaryLabel(language, "stdout"))
			builder.WriteString(":\n\n```text\n")
			builder.WriteString(detail.Stdout)
			builder.WriteString("\n```\n")
		}
		if detail.Stderr != "" {
			builder.WriteString("\n")
			builder.WriteString(localizedSummaryLabel(language, "stderr"))
			builder.WriteString(":\n\n```text\n")
			builder.WriteString(detail.Stderr)
			builder.WriteString("\n```\n")
		}
	}
}

func verificationSummaryFromEvidence(language string, evidenceRefs []string) string {
	if len(evidenceRefs) == 0 {
		if prefersChinese(language) {
			return "验收评估未报告明确的证据引用。"
		}
		return "The acceptance assessment did not report explicit evidence references."
	}
	if prefersChinese(language) {
		return fmt.Sprintf("验收结论由 %d 个证据引用支撑。", len(evidenceRefs))
	}
	return fmt.Sprintf("Acceptance is backed by %d evidence reference(s).", len(evidenceRefs))
}

func localizedStepCompletionSummary(language string) string {
	if prefersChinese(language) {
		return "runtime 步骤已完成"
	}
	return "runtime step completed"
}

func localizedDefaultList(language string, kind string) []string {
	switch kind {
	case "residual_risks":
		if prefersChinese(language) {
			return []string{"验收评估未报告剩余风险。"}
		}
		return []string{"No residual risks were reported by the acceptance assessment."}
	case "known_limitations":
		if prefersChinese(language) {
			return []string{"验收评估未报告已知限制。"}
		}
		return []string{"No known limitations were reported by the acceptance assessment."}
	case "next_actions":
		if prefersChinese(language) {
			return []string{"使用生成的交付物和证据引用进行验收确认。"}
		}
		return []string{"Use the generated Artifact and evidence refs for user acceptance review."}
	default:
		return nil
	}
}

func localizedSummaryLabel(language string, key string) string {
	if prefersChinese(language) {
		switch key {
		case "title":
			return "已验收目标成果摘要"
		case "goal_outcome":
			return "目标成果"
		case "implementation_summary":
			return "实现摘要"
		case "verification_summary":
			return "验证摘要"
		case "evidence_details":
			return "证据输出"
		case "stdout":
			return "标准输出"
		case "stderr":
			return "标准错误"
		case "dod_items":
			return "验收项确认"
		case "dod_table_header":
			return "| 验收项 | 状态 | 证据 |"
		case "evidence_refs":
			return "证据引用"
		case "artifact_refs":
			return "交付物索引"
		case "known_limitations":
			return "已知限制"
		case "residual_risks":
			return "剩余风险"
		case "next_actions":
			return "后续动作"
		}
	}
	switch key {
	case "title":
		return "Accepted Result Summary"
	case "goal_outcome":
		return "Goal Outcome"
	case "implementation_summary":
		return "Implementation Summary"
	case "verification_summary":
		return "Verification Summary"
	case "evidence_details":
		return "Evidence Output"
	case "stdout":
		return "Stdout"
	case "stderr":
		return "Stderr"
	case "dod_items":
		return "DoD Items"
	case "dod_table_header":
		return "| Item | State | Evidence |"
	case "evidence_refs":
		return "Evidence Refs"
	case "artifact_refs":
		return "Artifact Refs"
	case "known_limitations":
		return "Known Limitations"
	case "residual_risks":
		return "Residual Risks"
	case "next_actions":
		return "Next Actions"
	default:
		return key
	}
}

func runDialogLanguage(run *runModel.Run) string {
	if run == nil {
		return ""
	}
	return strings.TrimSpace(stringFromMap(run.ExecutionContext, "dialogLanguage"))
}

func prefersChinese(language string) bool {
	language = strings.ToLower(strings.TrimSpace(language))
	return strings.Contains(language, "zh") || strings.Contains(language, "chinese") || strings.Contains(language, "中文") || strings.Contains(language, "汉语")
}

func writeStringListSection(builder *strings.Builder, title string, values []string) {
	if builder == nil || len(values) == 0 {
		return
	}
	builder.WriteString(fmt.Sprintf("\n\n## %s\n\n", title))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		builder.WriteString(fmt.Sprintf("- %s\n", value))
	}
}

func markdownTableCell(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}

func markdownInlineCode(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "`", "'")
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}

func markdownHeadingText(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}

func evidenceValueString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		if typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func boundedEvidenceText(value string) string {
	value = strings.TrimSpace(value)
	const limit = 12000
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "\n... truncated ..."
}

func firstNonEmptyOrchestratorString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
