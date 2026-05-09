package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	"github.com/muidea/workorch/internal/modules/application/workorch/biz/backend"
	"github.com/muidea/workorch/internal/pkg/events"
)

func (m AppModel) View() string {
	if m.mode == modeQuit {
		return ""
	}
	width, height := m.viewSize()
	if m.helpOpen {
		return m.renderHelpOverlay(width, height)
	}

	input := m.renderInput(width)
	help := m.renderHelp(width)
	contentHeight := height - lineCount(input) - lineCount(help) - 1
	if contentHeight < 8 {
		contentHeight = 8
	}
	content := m.renderContent(width, contentHeight)
	return padLines(content, contentHeight) + "\n" + input + "\n" + help
}

func (m AppModel) viewSize() (int, int) {
	width := m.width
	if width == 0 {
		width = 100
	}
	height := m.height
	if height == 0 {
		height = 24
	}
	return width, height
}

func (m AppModel) contentDimensions() (int, int) {
	width, height := m.viewSize()
	input := m.renderInput(width)
	help := m.renderHelp(width)
	contentHeight := height - lineCount(input) - lineCount(help) - 1
	if contentHeight < 8 {
		contentHeight = 8
	}
	if width >= 96 {
		return width - 32, contentHeight
	}
	return width, contentHeight
}

func (m *AppModel) refreshMainViewport(width int, height int) {
	vp := m.ensureMainViewport()
	vp.Width = width
	vp.Height = height
	vp.MouseWheelEnabled = false
	vp.SetContent(strings.TrimRight(m.renderMain(width, 0), "\n"))
}

func (m AppModel) renderContent(width, height int) string {
	if width >= 96 {
		rightWidth := 30
		mainWidth := width - rightWidth - 2
		main := m.renderMainViewport(m.renderMain(mainWidth, 0), mainWidth, height)
		return joinColumns(main, m.renderWorkspacePanel(rightWidth, height), mainWidth+2)
	}

	content := strings.TrimRight(m.renderMainViewport(m.renderMain(width, 0), width, height), "\n")
	if len(m.workspaces) > 1 {
		remaining := height - lineCount(content) - 1
		if remaining >= 4 {
			content += "\n\n" + strings.TrimRight(m.renderWorkspacePanel(width, remaining), "\n")
		}
	}
	return content + "\n"
}

func (m AppModel) renderMainViewport(content string, width int, height int) string {
	vp := m.mainViewport
	if vp == nil {
		v := viewport.New(width, height)
		v.MouseWheelEnabled = false
		vp = &v
	}
	vp.Width = width
	vp.Height = height
	vp.MouseWheelEnabled = false
	vp.SetContent(strings.TrimRight(content, "\n"))
	return trimRightDisplaySpaceLines(vp.View())
}

func trimRightDisplaySpaceLines(value string) string {
	lines := strings.Split(value, "\n")
	for i := range lines {
		lines[i] = trimRightDisplaySpace(lines[i])
	}
	return strings.Join(lines, "\n")
}

func joinColumns(left, right string, leftWidth int) string {
	leftLines := strings.Split(strings.TrimRight(left, "\n"), "\n")
	rightLines := strings.Split(strings.TrimRight(right, "\n"), "\n")
	lineCount := len(leftLines)
	if len(rightLines) > lineCount {
		lineCount = len(rightLines)
	}
	var b strings.Builder
	for i := 0; i < lineCount; i++ {
		leftLine := ""
		if i < len(leftLines) {
			leftLine = leftLines[i]
		}
		rightLine := ""
		if i < len(rightLines) {
			rightLine = rightLines[i]
		}
		b.WriteString(padDisplay(leftLine, leftWidth))
		b.WriteString(rightLine)
		if i < lineCount-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func padDisplay(value string, width int) string {
	padding := width - lipgloss.Width(value)
	if padding <= 0 {
		return value
	}
	return value + strings.Repeat(" ", padding)
}

func (m AppModel) renderMain(width, height int) string {
	var b strings.Builder
	b.WriteString(Title.Render("WorkOrch Agent"))
	b.WriteString("\n")
	b.WriteString(m.renderActiveWorkspace(width))
	b.WriteString("\n")
	if line := m.renderWorkspaceTemplateInfo(width); line != "" {
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString(m.renderProgress(width))
	b.WriteString("\n")
	if len(m.transcript) > 0 {
		b.WriteString(renderTranscript(m.transcript, width-2, 5))
	}
	if m.currentTask != "" {
		b.WriteString(truncateDisplay("Task: "+m.currentTask, width-2))
		b.WriteString("\n")
	}
	if len(m.dodItems) > 0 {
		b.WriteString(renderDoDItems(m.dodItems, width-2, 4))
	}
	if section := renderTaskDecompositionSummary(m.taskDecomp, width-2, 3); section != "" {
		b.WriteString(section)
	}
	if section := renderMemoryRecallCompact(m.memoryRecalls, width-2, 3); section != "" {
		b.WriteString(section)
	}
	if section := renderLLMUsageCompact(m.llmUsage, width-2); section != "" {
		b.WriteString(section)
	}
	if section := renderLLMContextCompact(m.llmContext, width-2); section != "" {
		b.WriteString(section)
	}
	if section := renderGovernanceCompact(m.decisions, m.approvals, width-2, 2); section != "" {
		b.WriteString(section)
	}
	b.WriteString(Dim.Render(strings.Repeat("-", max(10, width-2))))
	b.WriteString("\n")

	if m.blockingDecision != nil {
		b.WriteString(Warning.Render("Decision required"))
		b.WriteString("\n")
		b.WriteString(truncateDisplay(blockingDecisionLabel(m.blockingDecision), width-2))
		b.WriteString("\n\n")
	}
	if m.blockingApproval != nil {
		b.WriteString(Alert.Render("Approval required"))
		b.WriteString("\n")
		b.WriteString(truncateDisplay(blockingApprovalLabel(m.blockingApproval), width-2))
		b.WriteString("\n\n")
	}

	b.WriteString(Heading.Render("Result"))
	b.WriteString("\n")
	resultLines := m.resultLineLimit(height, lineCount(b.String()))
	switch {
	case m.result != "":
		b.WriteString(renderRichText(m.result, width-2, resultLines))
		b.WriteString("\n")
	case m.failureReason != "":
		b.WriteString(Error.Render(truncateDisplay(m.failureReason, width-2)))
		b.WriteString("\n")
	default:
		b.WriteString(Dim.Render(m.emptyResultText()))
		b.WriteString("\n")
	}

	if artifacts := m.visibleArtifacts(); len(artifacts) > 0 {
		b.WriteString("\n")
		b.WriteString(Heading.Render("Artifacts"))
		b.WriteString("\n")
		for _, artifact := range artifacts {
			b.WriteString(truncateDisplay("- "+artifact, width-2))
			b.WriteString("\n")
		}
	}
	if sources := m.visibleSources(); len(sources) > 0 {
		b.WriteString("\n")
		b.WriteString(Heading.Render("Sources"))
		b.WriteString("\n")
		for _, source := range sources {
			b.WriteString(truncateDisplay("- "+source, width-2))
			b.WriteString("\n")
		}
	}
	if patches := m.visiblePatches(); len(patches) > 0 {
		b.WriteString("\n")
		b.WriteString(Heading.Render("Patches"))
		b.WriteString("\n")
		for _, patch := range patches {
			b.WriteString(truncateDisplay("- "+patchDisplayLabel(patch), width-2))
			b.WriteString("\n")
		}
	}
	if m.lastError != nil {
		b.WriteString("\n")
		b.WriteString(Error.Render("Error: " + m.lastError.Error()))
		b.WriteString("\n")
	}
	return b.String()
}

func (m AppModel) resultLineLimit(height int, usedLines int) int {
	if height <= 0 {
		return maxResultLines
	}
	reserved := 0
	if artifacts := m.visibleArtifacts(); len(artifacts) > 0 {
		reserved += 2 + len(artifacts)
	}
	if sources := m.visibleSources(); len(sources) > 0 {
		reserved += 2 + len(sources)
	}
	if patches := m.visiblePatches(); len(patches) > 0 {
		reserved += 2 + len(patches)
	}
	if m.lastError != nil {
		reserved += 2
	}
	limit := height - usedLines - reserved
	if limit < 4 {
		return 4
	}
	return limit
}

func (m AppModel) visibleArtifacts() []string {
	if len(m.artifacts) == 0 {
		return nil
	}
	result := strings.ToLower(m.result)
	if strings.Contains(result, "交付物索引") || strings.Contains(result, "artifact index") || strings.Contains(result, "artifacts") {
		return nil
	}
	return m.artifacts
}

func (m AppModel) visibleSources() []string {
	if len(m.sources) <= 6 {
		return m.sources
	}
	return m.sources[:6]
}

func (m AppModel) visiblePatches() []backend.PatchInfo {
	if len(m.patches) <= 8 {
		return m.patches
	}
	return m.patches[len(m.patches)-8:]
}

func renderTaskDecompositionSummary(items []backend.TaskDecompositionInfo, width int, limit int) string {
	if len(items) == 0 || width <= 0 || limit <= 0 {
		return ""
	}
	if limit > len(items) {
		limit = len(items)
	}
	var b strings.Builder
	b.WriteString(Heading.Render("Task Decomposition"))
	b.WriteString("\n")
	for i := 0; i < limit; i++ {
		item := items[i]
		title := nonEmpty(item.Summary, item.ID)
		nodes := ""
		if item.NodeTotal > 0 {
			nodes = fmt.Sprintf(" [%d/%d/%d/%d]", item.NodeCompleted, item.NodeRunning, item.NodeFailed, item.NodeTotal)
		}
		line := strings.TrimSpace(item.State + " " + title + nodes)
		b.WriteString(truncateDisplay("- "+line, width))
		b.WriteString("\n")
	}
	if len(items) > limit {
		b.WriteString(truncateDisplay(fmt.Sprintf("+%d more decompositions", len(items)-limit), width))
		b.WriteString("\n")
	}
	return b.String()
}

func renderMemoryRecallCompact(items []backend.MemoryRecallTraceInfo, width int, limit int) string {
	if len(items) == 0 || width <= 0 || limit <= 0 {
		return ""
	}
	if limit > len(items) {
		limit = len(items)
	}
	var b strings.Builder
	b.WriteString(Heading.Render("Memory Recall"))
	b.WriteString("\n")
	for i := 0; i < limit; i++ {
		item := items[i]
		status := nonEmpty(item.Status, "unknown")
		source := nonEmpty(item.Source, "unknown")
		stage := nonEmpty(item.Stage, "unknown")
		line := fmt.Sprintf("%s/%s: %s (%d)", source, stage, status, len(item.RetrievedMemoryIDs))
		b.WriteString(truncateDisplay("- "+line, width))
		b.WriteString("\n")
	}
	if len(items) > limit {
		b.WriteString(truncateDisplay(fmt.Sprintf("+%d more memory recalls", len(items)-limit), width))
		b.WriteString("\n")
	}
	return b.String()
}

func renderLLMUsageCompact(summary backend.LLMUsageSummaryInfo, width int) string {
	if summary.EntryTotal == 0 || width <= 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(Heading.Render("LLM Usage"))
	b.WriteString("\n")
	parts := []string{
		fmt.Sprintf("calls %d", summary.EntryTotal),
		fmt.Sprintf("tokens %d/%d/%d", summary.InputTokens, summary.OutputTokens, summary.TotalTokens),
	}
	if summary.ProviderNativeEntries > 0 || summary.EstimatedEntries > 0 {
		parts = append(parts, fmt.Sprintf("native/estimated %d/%d", summary.ProviderNativeEntries, summary.EstimatedEntries))
	}
	if cost := llmUsageCostLabel(summary); cost != "" {
		parts = append(parts, cost)
	}
	if summary.FailedEntries > 0 {
		parts = append(parts, fmt.Sprintf("failed %d", summary.FailedEntries))
	}
	b.WriteString(truncateDisplay("- "+strings.Join(parts, " | "), width))
	b.WriteString("\n")
	return b.String()
}

func renderLLMContextCompact(budget map[string]any, width int) string {
	if len(budget) == 0 || width <= 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(Heading.Render("LLM Context"))
	b.WriteString("\n")
	provider := firstPayloadString(budget, "providerId", "provider", "ProviderID")
	model := firstPayloadString(budget, "model", "Model")
	estimatedInput := payloadIntValue(budget, "estimatedInputTokens", "EstimatedInputTokens")
	maxTokens := payloadIntValue(budget, "maxTokens", "MaxTokens")
	shrinkLevel := payloadIntValue(budget, "shrinkLevel", "ShrinkLevel")
	p0Overflow := payloadBoolValue(budget, "p0Overflow", "P0Overflow")
	goalDoDCompressed := payloadBoolValue(budget, "goalDoDCompressed", "GoalDoDCompressed")
	parts := []string{}
	if provider != "" || model != "" {
		parts = append(parts, strings.Trim(strings.Join([]string{provider, model}, "/"), "/"))
	}
	if maxTokens > 0 || estimatedInput > 0 {
		parts = append(parts, fmt.Sprintf("tokens %d/%d", estimatedInput, maxTokens))
	}
	parts = append(parts, fmt.Sprintf("shrink %d", shrinkLevel))
	if p0Overflow {
		parts = append(parts, "p0 overflow")
	}
	if goalDoDCompressed {
		parts = append(parts, "goal/dod compressed")
	} else {
		parts = append(parts, "goal/dod protected")
	}
	if len(parts) > 0 {
		b.WriteString(truncateDisplay("- "+strings.Join(parts, " | "), width))
		b.WriteString("\n")
	}
	included := payloadStringSliceValue(budget, "includedSections", "IncludedSections")
	if len(included) > 0 {
		b.WriteString(truncateDisplay("- included: "+strings.Join(included, ", "), width))
		b.WriteString("\n")
	}
	normalized := payloadStringSliceValue(budget, "normalizedSections", "NormalizedSections")
	if len(normalized) > 0 {
		b.WriteString(truncateDisplay("- normalized: "+strings.Join(normalized, ", "), width))
		b.WriteString("\n")
	}
	compressed := payloadStringSliceValue(budget, "compressedSections", "CompressedSections")
	if len(compressed) > 0 {
		b.WriteString(truncateDisplay("- compressed: "+strings.Join(compressed, ", "), width))
		b.WriteString("\n")
	}
	return b.String()
}

func llmUsageProgressText(payload map[string]any) string {
	provider := firstPayloadString(payload, events.PayloadKeyProviderID, "providerId", "ProviderID")
	model := firstPayloadString(payload, events.PayloadKeyModel, "model", "Model")
	total := payloadIntValue(payload, events.PayloadKeyTotalTokens, "totalTokens", "TotalTokens")
	status := firstPayloadString(payload, events.PayloadKeyStatus, "status", "Status")
	label := strings.TrimSpace(strings.Join([]string{nonEmpty(provider, "llm"), model}, "/"))
	if total > 0 {
		return fmt.Sprintf("LLM usage recorded: %s %d tokens", label, total)
	}
	return "LLM usage recorded: " + strings.TrimSpace(label+" "+status)
}

func llmUsageCostLabel(summary backend.LLMUsageSummaryInfo) string {
	if summary.Currency != "" && summary.EstimatedCost > 0 {
		return fmt.Sprintf("cost %.6g %s", summary.EstimatedCost, summary.Currency)
	}
	if len(summary.CostBreakdown) == 0 {
		return ""
	}
	parts := make([]string, 0, len(summary.CostBreakdown))
	for _, item := range summary.CostBreakdown {
		if item.EstimatedCost <= 0 || item.Currency == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%.6g %s", item.EstimatedCost, item.Currency))
	}
	if len(parts) == 0 {
		return ""
	}
	return "cost " + strings.Join(parts, ", ")
}

func renderGovernanceCompact(decisions []backend.DecisionInfo, approvals []backend.ApprovalInfo, width int, limit int) string {
	openDecisions := filterOpenDecisions(decisions)
	pendingApprovals := filterPendingApprovals(approvals)
	if len(openDecisions) == 0 && len(pendingApprovals) == 0 {
		return ""
	}
	if limit <= 0 {
		limit = 1
	}
	var b strings.Builder
	b.WriteString(Heading.Render("Governance"))
	b.WriteString("\n")
	for i := 0; i < len(openDecisions) && i < limit; i++ {
		item := openDecisions[i]
		line := fmt.Sprintf("decision %s %s", shortID(string(item.ID)), nonEmpty(item.State, item.Question, item.Reason))
		b.WriteString(truncateDisplay("- "+line, width))
		b.WriteString("\n")
	}
	for i := 0; i < len(pendingApprovals) && i < limit; i++ {
		item := pendingApprovals[i]
		line := fmt.Sprintf("approval %s %s", shortID(string(item.ID)), nonEmpty(item.State, item.Reason, item.Action))
		b.WriteString(truncateDisplay("- "+line, width))
		b.WriteString("\n")
	}
	return b.String()
}

func (m AppModel) renderActiveWorkspace(width int) string {
	ws := findWorkspace(m.workspaces, m.activeWorkspaceID)
	if ws == nil {
		return Dim.Render("Workspace: " + shortID(m.activeWorkspaceID))
	}
	label := workspaceLabel(*ws)
	if ws.WorkingDirectory != "" {
		label += "  " + ws.WorkingDirectory
	}
	return truncateDisplay("Workspace: "+label, width-2)
}

func (m AppModel) renderWorkspaceTemplateInfo(width int) string {
	ws := findWorkspace(m.workspaces, m.activeWorkspaceID)
	if ws == nil {
		return ""
	}
	parts := make([]string, 0, 2)
	if ws.DefaultSoulID != "" {
		parts = append(parts, "default soul: "+string(ws.DefaultSoulID))
	}
	if len(ws.SoulBindings) > 0 {
		parts = append(parts, "bindings: "+workspaceBindingSummary(ws.SoulBindings))
	}
	if len(parts) == 0 {
		return ""
	}
	return Dim.Render(truncateDisplay(strings.Join(parts, " | "), width-2))
}

func (m AppModel) renderProgress(width int) string {
	label := progressLabel(m.progress)
	if m.progressText != "" && !strings.EqualFold(m.progressText, label) {
		label += " - " + m.progressText
	}
	if m.repairLoop != "" {
		label += " | " + m.repairLoop
	}
	label = truncateDisplay(label, width-2)
	switch m.progress {
	case progressCompleted:
		return Success.Render(label)
	case progressFailed:
		return Error.Render(label)
	case progressWaitingDecision:
		return Warning.Render(label)
	case progressWaitingApproval:
		return Alert.Render(label)
	default:
		return Info.Render(label)
	}
}

func (m AppModel) renderWorkspacePanel(width, height int) string {
	var b strings.Builder
	b.WriteString(Heading.Render("Workspaces"))
	b.WriteString("\n")
	if len(m.workspaces) == 0 {
		b.WriteString(Dim.Render("No workspaces"))
		b.WriteString("\n")
		return b.String()
	}

	limit := max(2, (height-5)/2)
	if limit > len(m.workspaces) {
		limit = len(m.workspaces)
	}
	counts := workspaceNameCounts(m.workspaces)
	for _, ws := range visibleWorkspaces(m.workspaces, m.cursor, limit) {
		prefix := "  "
		if m.workspaceSelectMode && string(ws.ID) == m.selectedWorkspaceID {
			prefix = "> "
		} else if !m.workspaceSelectMode && string(ws.ID) == m.activeWorkspaceID {
			prefix = "> "
		}
		b.WriteString(workspacePanelLine(prefix, ws, counts, width-1))
		b.WriteString("\n")
	}
	if len(m.workspaces) > limit {
		b.WriteString(Dim.Render(fmt.Sprintf("+%d more", len(m.workspaces)-limit)))
		b.WriteString("\n")
	}

	openDecisions := len(filterOpenDecisions(m.decisions))
	pendingApprovals := len(filterPendingApprovals(m.approvals))
	b.WriteString("\n")
	b.WriteString(Warning.Render(fmt.Sprintf("%d decisions", openDecisions)))
	b.WriteString("\n")
	b.WriteString(Alert.Render(fmt.Sprintf("%d approvals", pendingApprovals)))
	b.WriteString("\n")
	return b.String()
}

func (m AppModel) renderInput(width int) string {
	prompt := shortID(m.activeWorkspaceID)
	if prompt == "none" {
		prompt = "workspace"
	}
	if m.blockingDecision != nil {
		prompt += " decision"
	} else if m.blockingApproval != nil {
		prompt += " approval"
	}
	input := m.renderInputBuffer(prompt, width)
	if suggestions := m.renderSlashCommandSuggestions(width); suggestions != "" {
		return suggestions + "\n" + input
	}
	return input
}

func (m AppModel) renderHelp(width int) string {
	help := "enter: submit | /help: help | /workspace: select/create ws | /dod: update DoD | /memory: recall | /exit: quit | ctrl+c: interrupt/quit x2"
	if width < 120 {
		help = "enter: submit | /help | /workspace | /dod | /memory | /exit | ctrl+c x2 quit"
	}
	if m.workspaceSelectMode {
		help = "ws select: up/down/j/k move | enter confirm | ctrl+w cancel | esc cancel"
	}
	return Dim.Render(truncateDisplay(help, width-2))
}

func (m AppModel) renderHelpOverlay(width, height int) string {
	lines := []string{
		"WorkOrch Help",
		"",
		"Enter: submit current goal or response",
		"Alt+Enter: insert newline",
		"Ctrl+P / Ctrl+N: input history",
		"Tab: slash command completion",
		"Ctrl+W: workspace selector",
		"Ctrl+H: help",
		"Ctrl+C: interrupt active run; press twice outside a run to quit",
		"PgUp / PgDn: scroll result",
		"Esc: clear input or close help",
		"/help /clear /workspace /workspace create /dod /memory /result /exit",
	}
	panelWidth := min(width-4, 72)
	if panelWidth < 30 {
		panelWidth = max(20, width-2)
	}
	panelHeight := len(lines) + 4
	if panelHeight > height-2 {
		panelHeight = height - 2
	}
	b := strings.Builder{}
	padTop := max(0, (height-panelHeight)/2)
	for i := 0; i < padTop; i++ {
		b.WriteString("\n")
	}
	b.WriteString(Border.Width(panelWidth).Render(strings.Join(lines[:min(len(lines), panelHeight-4)], "\n")))
	return b.String()
}
