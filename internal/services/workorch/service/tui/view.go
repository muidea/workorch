package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	"github.com/muidea/workorch/internal/pkg/common"
	backend "github.com/muidea/workorch/internal/pkg/workorchport"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func (m AppModel) View() string {
	if m.mode == modeQuit {
		return ""
	}
	width, height := m.viewSize()
	if m.helpOpen {
		return m.renderHelpOverlay(width, height)
	}
	if m.modelSelectMode {
		return m.renderModelSelection(width, height)
	}

	status := m.renderRuntimeStatusBar(width)
	input := m.renderInput(width)
	help := m.renderHelp(width)
	contentHeight := height - lineCount(status) - lineCount(input) - lineCount(help) - 2
	if contentHeight < 8 {
		contentHeight = 8
	}
	content := m.renderContent(width, contentHeight)
	return padLines(content, contentHeight) + "\n" + status + "\n" + input + "\n" + help
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
	status := m.renderRuntimeStatusBar(width)
	input := m.renderInput(width)
	help := m.renderHelp(width)
	contentHeight := height - lineCount(status) - lineCount(input) - lineCount(help) - 2
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
	if section := renderAcceptanceCompact(m.acceptance, width-2); section != "" {
		b.WriteString(section)
	}
	if section := renderTaskPlanSummary(m.taskPlans, width-2, 3); section != "" {
		b.WriteString(section)
	} else if section := renderTaskDecompositionSummary(m.taskDecomp, width-2, 3); section != "" {
		b.WriteString(section)
	}
	if section := renderMemoryRecallCompact(m.memoryRecalls, width-2, 3); section != "" {
		b.WriteString(section)
	}
	if section := renderLLMUsageCompact(m.llmUsage, width-2); section != "" {
		b.WriteString(section)
	}
	if section := renderLLMContextCompact(m.llmContext, m.llmContextSummary, width-2); section != "" {
		b.WriteString(section)
	}
	if section := renderExecutionStreamStatus(m.running, m.progress, m.progressText, m.activeRunID, width-2); section != "" {
		b.WriteString(section)
	}
	if section := renderRuntimeProgressCompact(m.runtime, width-2); section != "" {
		b.WriteString(section)
	}
	// Keep a bounded but scrollable stream in the main viewport. The projection
	// feed itself is capped server-side; rendering more than the compact
	// dashboard tail lets follow-tail mode visibly show intermediate runtime
	// progress while preserving user scroll/unread semantics.
	if section := renderRecentActivityCompact(m.activityFeed, width-2, 30); section != "" {
		b.WriteString(section)
	}
	if section := renderGovernanceCompact(m.activeScopeDecisions(), m.activeScopeApprovals(), width-2, 2); section != "" {
		b.WriteString(section)
	}

	tail := m.renderMainTail(width, height, lineCount(b.String()))
	if strings.TrimSpace(tail) != "" {
		b.WriteString(Dim.Render(strings.Repeat("-", max(10, width-2))))
		b.WriteString("\n")
		b.WriteString(tail)
	}
	return b.String()
}

// renderExecutionStreamStatus keeps the main viewport informative from the
// moment a Run is accepted, even while the read projection is waiting for its
// first RuntimeView. Runtime details replace this fallback as soon as the
// typed projection arrives; the status bar remains a compact duplicate for
// quick scanning.
func renderExecutionStreamStatus(running bool, progress progressState, message, runID string, width int) string {
	if !running && progress != progressSubmitting && progress != progressRunning {
		return ""
	}
	message = strings.TrimSpace(message)
	if message == "" {
		message = "waiting for runtime projection"
	}
	var b strings.Builder
	b.WriteString(Heading.Render("Execution Stream"))
	b.WriteString("\n")
	if strings.TrimSpace(runID) != "" {
		b.WriteString(truncateDisplay("- run "+shortID(runID)+": "+message, width))
	} else {
		b.WriteString(truncateDisplay("- "+message, width))
	}
	b.WriteString("\n")
	return b.String()
}

func (m AppModel) renderMainTail(width int, height int, usedLines int) string {
	var b strings.Builder
	if decision := m.activeBlockingDecision(); decision != nil {
		b.WriteString(Warning.Render("Decision required"))
		b.WriteString("\n")
		b.WriteString(truncateDisplay(blockingDecisionLabel(decision), width-2))
		b.WriteString("\n\n")
	}
	if approval := m.activeBlockingApproval(); approval != nil {
		b.WriteString(Alert.Render("Approval required"))
		b.WriteString("\n")
		b.WriteString(truncateDisplay(blockingApprovalLabel(approval), width-2))
		b.WriteString("\n\n")
	}

	switch {
	case m.result != "":
		b.WriteString(Heading.Render("Result"))
		b.WriteString("\n")
		resultLines := m.resultLineLimit(height, usedLines+lineCount(b.String()))
		b.WriteString(renderRichText(m.result, width-2, resultLines))
		b.WriteString("\n")
	case m.failureReason != "":
		b.WriteString(Heading.Render("Result"))
		b.WriteString("\n")
		b.WriteString(Error.Render(truncateDisplay(m.failureReason, width-2)))
		b.WriteString("\n")
	case m.shouldRenderEmptyWorkspaceGuidance():
		b.WriteString(Dim.Render(m.emptyResultText()))
		b.WriteString("\n")
	}

	if artifacts := m.visibleArtifacts(); len(artifacts) > 0 {
		writeMainTailSectionBreak(&b)
		b.WriteString(Heading.Render("Artifacts"))
		b.WriteString("\n")
		for _, artifact := range artifacts {
			b.WriteString(truncateDisplay("- "+artifact, width-2))
			b.WriteString("\n")
		}
	}
	if sources := m.visibleSources(); len(sources) > 0 {
		writeMainTailSectionBreak(&b)
		b.WriteString(Heading.Render("Sources"))
		b.WriteString("\n")
		for _, source := range sources {
			b.WriteString(truncateDisplay("- "+source, width-2))
			b.WriteString("\n")
		}
	}
	if patches := m.visiblePatches(); len(patches) > 0 {
		writeMainTailSectionBreak(&b)
		b.WriteString(Heading.Render("Patches"))
		b.WriteString("\n")
		for _, patch := range patches {
			b.WriteString(truncateDisplay("- "+patchDisplayLabel(patch), width-2))
			b.WriteString("\n")
		}
	}
	if continuations := m.visibleArtifactContinuations(); len(continuations) > 0 {
		writeMainTailSectionBreak(&b)
		b.WriteString(Heading.Render("Artifact Continuations"))
		b.WriteString("\n")
		for _, item := range continuations {
			b.WriteString(truncateDisplay("- "+artifactContinuationDisplayLabel(item), width-2))
			b.WriteString("\n")
		}
	}
	if m.lastError != nil {
		writeMainTailSectionBreak(&b)
		b.WriteString(Error.Render("Error: " + m.lastError.Error()))
		b.WriteString("\n")
	}
	return b.String()
}

func writeMainTailSectionBreak(b *strings.Builder) {
	if b.Len() > 0 {
		b.WriteString("\n")
	}
}

func (m AppModel) shouldRenderEmptyWorkspaceGuidance() bool {
	return len(m.workspaces) == 0 && strings.TrimSpace(m.activeWorkspaceID) == ""
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

func (m AppModel) visiblePatches() []projectioncontract.Patch {
	if len(m.patches) <= 8 {
		return m.patches
	}
	return m.patches[len(m.patches)-8:]
}

func renderAcceptanceCompact(item projectioncontract.AcceptanceProjection, width int) string {
	if acceptanceProjectionEmpty(item) || width <= 0 {
		return ""
	}
	parts := []string{
		fmt.Sprintf("accepted=%d", item.Summary.AcceptedSlices),
		fmt.Sprintf("active=%d", item.Summary.ActiveSlices),
		fmt.Sprintf("open=%d", item.Summary.OpenSlices),
		fmt.Sprintf("failed=%d", item.Summary.FailedSlices),
	}
	if item.ActiveSliceID != "" {
		parts = append(parts, "slice="+item.ActiveSliceID)
	}
	if item.CurrentFocus != nil {
		focusParts := compactStrings([]string{item.CurrentFocus.ID, item.CurrentFocus.State, item.CurrentFocus.FailureClass})
		if len(focusParts) > 0 {
			parts = append(parts, "focus="+strings.Join(focusParts, "/"))
		}
	}
	if item.Budget != nil {
		budgetParts := compactStrings([]string{item.Budget.Decision, item.Budget.LastDecisionReason})
		if len(budgetParts) > 0 {
			parts = append(parts, "budget="+strings.Join(budgetParts, "/"))
		}
	}
	if item.RepairDirective != nil && item.RepairDirective.RequiredAction != "" {
		parts = append(parts, "directive="+item.RepairDirective.RequiredAction)
	}
	return truncateDisplay("Acceptance: "+strings.Join(parts, " | "), width) + "\n"
}

func acceptanceProgressText(item projectioncontract.AcceptanceProjection) string {
	if acceptanceProjectionEmpty(item) {
		return ""
	}
	if item.CurrentFocus != nil {
		parts := compactStrings([]string{item.CurrentFocus.ID, item.CurrentFocus.State, item.CurrentFocus.FailureClass})
		if len(parts) > 0 {
			return "focus " + strings.Join(parts, "/")
		}
	}
	if item.RepairDirective != nil && item.RepairDirective.RequiredAction != "" {
		return "directive " + item.RepairDirective.RequiredAction
	}
	if item.ActiveSliceID != "" {
		return "slice " + item.ActiveSliceID
	}
	return ""
}

func acceptanceProjectionEmpty(item projectioncontract.AcceptanceProjection) bool {
	return item.ActiveSliceID == "" &&
		len(item.Slices) == 0 &&
		item.CurrentFocus == nil &&
		item.Budget == nil &&
		item.RepairDirective == nil &&
		item.Audit == nil &&
		item.Summary.AcceptedSlices == 0 &&
		item.Summary.ActiveSlices == 0 &&
		item.Summary.OpenSlices == 0 &&
		item.Summary.FailedSlices == 0
}

func renderTaskDecompositionSummary(items []projectioncontract.TaskDecomposition, width int, limit int) string {
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

func renderTaskPlanSummary(items []projectioncontract.TaskPlan, width int, limit int) string {
	if len(items) == 0 || width <= 0 || limit <= 0 {
		return ""
	}
	if limit > len(items) {
		limit = len(items)
	}
	var b strings.Builder
	b.WriteString(Heading.Render("Task Plans"))
	b.WriteString("\n")
	for i := 0; i < limit; i++ {
		item := items[i]
		title := nonEmpty(item.Summary, item.ID)
		nodes := ""
		if item.NodeTotal > 0 {
			nodes = fmt.Sprintf(" [%d/%d/%d/%d]", item.NodeCompleted, item.NodeRunning, item.NodeFailed, item.NodeTotal)
		}
		line := strings.TrimSpace(strings.Join(compactStrings([]string{
			item.State,
			item.SourceOutputState,
			title + nodes,
		}), " "))
		b.WriteString(truncateDisplay("- "+line, width))
		b.WriteString("\n")
	}
	if len(items) > limit {
		b.WriteString(truncateDisplay(fmt.Sprintf("+%d more task plans", len(items)-limit), width))
		b.WriteString("\n")
	}
	return b.String()
}

func renderMemoryRecallCompact(items []projectioncontract.MemoryRecallTrace, width int, limit int) string {
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

func renderLLMUsageCompact(summary projectioncontract.LLMUsageSummary, width int) string {
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
	if summary.FailedEntries > 0 {
		parts = append(parts, fmt.Sprintf("failed %d", summary.FailedEntries))
	}
	b.WriteString(truncateDisplay("- "+strings.Join(parts, " | "), width))
	b.WriteString("\n")
	return b.String()
}

func renderLLMContextCompact(budget map[string]any, summary projectioncontract.LLMContextBudgetSummary, width int) string {
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
	if summary.TraceTotal > 0 {
		summaryParts := []string{
			fmt.Sprintf("traces %d", summary.TraceTotal),
			fmt.Sprintf("max shrink %d", summary.MaxShrinkLevel),
		}
		if summary.MaxContextUtilization > 0 {
			summaryParts = append(summaryParts, fmt.Sprintf("max util %.0f%%", summary.MaxContextUtilization*100))
		}
		if summary.AvgContextUtilization > 0 {
			summaryParts = append(summaryParts, fmt.Sprintf("avg util %.0f%%", summary.AvgContextUtilization*100))
		}
		if summary.OverBudgetCount > 0 {
			summaryParts = append(summaryParts, fmt.Sprintf("over budget %d", summary.OverBudgetCount))
		}
		if summary.RecentTraceTotal > 0 {
			recent := fmt.Sprintf("recent %d/%d", summary.RecentTraceTotal, summary.RecentWindowSize)
			if summary.RecentAvgUtilization > 0 {
				recent += fmt.Sprintf(" avg %.0f%%", summary.RecentAvgUtilization*100)
			}
			if summary.UtilizationTrend != "" {
				recent += " " + summary.UtilizationTrend
			}
			summaryParts = append(summaryParts, recent)
		}
		b.WriteString(truncateDisplay("- "+strings.Join(summaryParts, " | "), width))
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

func renderRuntimeProgressCompact(runtime *projectioncontract.RuntimeProgress, width int) string {
	if runtime == nil || width <= 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(Heading.Render("Runtime Progress"))
	b.WriteString("\n")
	b.WriteString(truncateDisplay(fmt.Sprintf("- revision %d | evidence %d", runtime.StateRevision, runtime.EvidenceSequence), width))
	b.WriteString("\n")
	if focus := strings.TrimSpace(runtime.CurrentFocusID); focus != "" || strings.TrimSpace(runtime.ConvergenceState) != "" {
		b.WriteString(truncateDisplay(fmt.Sprintf("- focus %s | convergence %s", nonEmpty(focus, "-"), nonEmpty(runtime.ConvergenceState, "-")), width))
		b.WriteString("\n")
	}
	b.WriteString(truncateDisplay(fmt.Sprintf("- dispatch %d completed | candidate attempts %d", runtime.CompletedDispatchCount, runtime.CandidateAttemptCount), width))
	b.WriteString("\n")
	if state := strings.TrimSpace(runtime.PrerequisiteRefreshState); state != "" {
		refresh := fmt.Sprintf("- prerequisite refresh %s | stale %d/%d", state, runtime.PrerequisiteRefreshUsed, runtime.PrerequisiteRefreshLimit)
		b.WriteString(truncateDisplay(refresh, width))
		b.WriteString("\n")
		for idx, invalidation := range runtime.PrerequisiteInvalidations {
			if idx == 3 {
				break
			}
			source := strings.TrimSpace(invalidation.Source)
			resource := strings.TrimSpace(invalidation.ResourceDomain + ":" + invalidation.SubjectRef)
			if resource == ":" {
				resource = "-"
			}
			b.WriteString(truncateDisplay(fmt.Sprintf("  - %s | %s | %s", nonEmpty(source, "invalidation"), nonEmpty(invalidation.OwnerModuleID, "-"), resource), width))
			b.WriteString("\n")
		}
	}
	missing := "none"
	if len(runtime.MissingEvidenceKinds) > 0 {
		missing = strings.Join(runtime.MissingEvidenceKinds, ",")
	}
	b.WriteString(truncateDisplay(fmt.Sprintf("- evidence accepted %d rejected %d | missing %s", runtime.AcceptedEvidenceCount, runtime.RejectedEvidenceCount, missing), width))
	b.WriteString("\n")
	if msg := strings.TrimSpace(runtime.FailureMessage); msg != "" {
		b.WriteString(truncateDisplay("- failure "+msg, width))
		b.WriteString("\n")
	}
	return b.String()
}

func renderRecentActivityCompact(feed projectioncontract.WorkspaceActivityFeed, width int, limit int) string {
	if len(feed.Items) == 0 || width <= 0 || limit <= 0 {
		return ""
	}
	start := len(feed.Items) - limit
	if start < 0 {
		start = 0
	}
	var b strings.Builder
	b.WriteString(Heading.Render("Recent Activity"))
	b.WriteString("\n")
	for _, item := range feed.Items[start:] {
		line := strings.TrimSpace(item.Summary)
		if line == "" {
			line = strings.TrimSpace(item.Title)
		}
		if line == "" {
			line = string(item.Kind)
		}
		if ts := activityTimeLabel(item.OccurredAt); ts != "" {
			line = ts + " " + line
		}
		b.WriteString(truncateDisplay("- "+line, width))
		b.WriteString("\n")
	}
	return b.String()
}

func activityTimeLabel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	// Prefer clock portion when ISO timestamps are present.
	if len(value) >= 19 && value[10] == 'T' {
		return value[11:16]
	}
	if len(value) >= 5 && value[2] == ':' {
		return value[:5]
	}
	return value
}

func convergenceStatusText(item *projectioncontract.ConvergenceStatus) string {
	if item == nil {
		return ""
	}
	parts := compactStrings([]string{
		convergenceSliceText(item),
		convergenceFocusText(item),
		convergenceNextActionText(item),
		convergenceCoverageText(item),
		convergenceDispositionText(item),
		convergenceRepairText(item),
		convergenceGapText(item),
		convergenceEvidenceText(item),
		convergenceDecisionCauseText(item),
		convergenceHeartbeatText(item),
		convergenceActivationText(item),
	})
	return strings.Join(parts, " | ")
}

func convergenceSliceText(item *projectioncontract.ConvergenceStatus) string {
	if item == nil || strings.TrimSpace(item.CurrentSliceID) == "" {
		return ""
	}
	return "slice " + strings.TrimSpace(item.CurrentSliceID)
}

func convergenceFocusText(item *projectioncontract.ConvergenceStatus) string {
	if item == nil || strings.TrimSpace(item.CurrentFocusID) == "" {
		return ""
	}
	return "focus " + strings.TrimSpace(item.CurrentFocusID)
}

func convergenceNextActionText(item *projectioncontract.ConvergenceStatus) string {
	if item == nil {
		return ""
	}
	if state := strings.TrimSpace(item.ConvergenceState); state != "" {
		return "convergence " + state
	}
	if suggestion := strings.TrimSpace(item.SuggestedNextAction); suggestion != "" {
		return "suggested " + suggestion
	}
	return ""
}

func convergenceCoverageText(item *projectioncontract.ConvergenceStatus) string {
	if item == nil {
		return ""
	}
	if item.ClosedCoverageItemCount == 0 && item.RemainingCoverageItemCount == 0 {
		return ""
	}
	return fmt.Sprintf("coverage closed %d remaining %d", item.ClosedCoverageItemCount, item.RemainingCoverageItemCount)
}

func convergenceDispositionText(item *projectioncontract.ConvergenceStatus) string {
	if item == nil || strings.TrimSpace(item.AssessmentDisposition) == "" {
		return ""
	}
	return "disposition " + strings.TrimSpace(item.AssessmentDisposition)
}

func convergenceRepairText(item *projectioncontract.ConvergenceStatus) string {
	if item == nil {
		return ""
	}
	parts := make([]string, 0, 2)
	if item.MaxRepairAttempts > 0 || item.RepairAttempts > 0 {
		if item.MaxRepairAttempts > 0 {
			parts = append(parts, fmt.Sprintf("repair %d/%d", item.RepairAttempts, item.MaxRepairAttempts))
		} else {
			parts = append(parts, fmt.Sprintf("repair %d", item.RepairAttempts))
		}
	}
	if item.RepeatedFailureSignatureCount > 0 {
		parts = append(parts, fmt.Sprintf("repeat %d", item.RepeatedFailureSignatureCount))
	}
	return strings.Join(parts, " ")
}

func convergenceGapText(item *projectioncontract.ConvergenceStatus) string {
	if item == nil {
		return ""
	}
	if summary := strings.TrimSpace(item.GapSummary); summary != "" {
		return "gap " + summary
	}
	if strings.TrimSpace(item.CurrentGap) == "" {
		return ""
	}
	return "gap " + strings.TrimSpace(item.CurrentGap)
}

func convergenceEvidenceText(item *projectioncontract.ConvergenceStatus) string {
	if item == nil {
		return ""
	}
	parts := make([]string, 0, 4)
	if len(item.MissingEvidenceKinds) > 0 {
		parts = append(parts, "missing "+strings.Join(item.MissingEvidenceKinds, ","))
	}
	if item.AcceptedEvidenceCount > 0 {
		parts = append(parts, fmt.Sprintf("accepted %d", item.AcceptedEvidenceCount))
	}
	if item.RejectedEvidenceCount > 0 {
		parts = append(parts, fmt.Sprintf("rejected %d", item.RejectedEvidenceCount))
	}
	if ref := strings.TrimSpace(item.LatestAcceptedEvidenceRef); ref != "" {
		parts = append(parts, "latest "+ref)
	}
	if len(parts) == 0 {
		return ""
	}
	return "evidence " + strings.Join(parts, " ")
}

func convergenceDecisionCauseText(item *projectioncontract.ConvergenceStatus) string {
	if item == nil || strings.TrimSpace(item.DecisionCause) == "" {
		return ""
	}
	return "decision " + strings.TrimSpace(item.DecisionCause)
}

func convergenceHeartbeatText(item *projectioncontract.ConvergenceStatus) string {
	if item == nil {
		return ""
	}
	parts := make([]string, 0, 3)
	if item.Stalled {
		parts = append(parts, "stalled")
	}
	if item.IneffectiveRetry {
		parts = append(parts, "ineffectiveRetry")
	}
	progressAt := strings.TrimSpace(item.LastProgressAt)
	if progressAt == "" {
		progressAt = strings.TrimSpace(item.LatestEffectiveProgressAt)
	}
	if strings.TrimSpace(progressAt) != "" {
		parts = append(parts, "progress "+strings.TrimSpace(progressAt))
	}
	return strings.Join(parts, " ")
}

func convergenceActivationText(item *projectioncontract.ConvergenceStatus) string {
	if item == nil || !item.NextActivationRequired {
		return ""
	}
	return "nextActivationRequired"
}

func artifactContinuationProgressText(payload map[string]any) string {
	state := firstPayloadString(payload, "state", "State", backend.PayloadKeyState)
	targetRef := firstPayloadString(payload, "targetRef", "TargetRef", backend.PayloadKeyTargetRef)
	artifactID := firstPayloadString(payload, "artifactId", "ArtifactID")
	expected := payloadIntValue(payload, "expectedSequence", "ExpectedSequence")
	received := len(payloadStringSliceValue(payload, "receivedSequences", "ReceivedSequences"))
	bytesWritten := payloadIntValue(payload, "bytesWritten", "BytesWritten")
	failure := firstPayloadString(payload, "failureReason", "FailureReason", "error", "Error")
	target := nonEmpty(targetRef, artifactID, firstPayloadString(payload, "artifactContinuationId", "ID", "id"))
	parts := []string{"Artifact"}
	if state != "" {
		parts = append(parts, state)
	}
	if target != "" {
		parts = append(parts, target)
	}
	if expected > 0 || received > 0 {
		parts = append(parts, fmt.Sprintf("seq %d/%s", received, artifactExpectedSequenceText(expected)))
	}
	if bytesWritten > 0 {
		parts = append(parts, fmt.Sprintf("%dB", bytesWritten))
	}
	if failure != "" {
		parts = append(parts, failure)
	}
	return strings.Join(parts, " ")
}

func renderGovernanceCompact(decisions []projectioncontract.Decision, approvals []projectioncontract.Approval, width int, limit int) string {
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
	ws := m.activeWorkspaceOverview()
	if ws == nil {
		return Dim.Render("Workspace: " + shortID(m.activeWorkspaceID))
	}
	label := workspaceLabel(*ws)
	if ws.DeliverableDirectory != "" {
		label += "  " + ws.DeliverableDirectory
	}
	return truncateDisplay("Workspace: "+label, width-2)
}

func (m AppModel) renderWorkspaceTemplateInfo(width int) string {
	ws := m.activeWorkspaceOverview()
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
	if focus := acceptanceProgressText(m.acceptance); focus != "" {
		label += " | " + focus
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

func (m AppModel) renderRuntimeStatusBar(width int) string {
	parts := []string{strings.TrimSpace(m.renderProgress(width))}
	if m.activeWorkspaceID != "" {
		parts = append(parts, "ws "+shortID(m.activeWorkspaceID))
	}
	if m.activeRunID != "" {
		parts = append(parts, "run "+shortID(m.activeRunID))
	}
	if active := m.activeArtifactContinuationStatus(); active != "" {
		parts = append(parts, active)
	}
	if status := convergenceStatusText(m.convergenceStatus); status != "" {
		parts = append(parts, status)
	}
	if command := runCommandStatusText(m.runCommand); command != "" {
		parts = append(parts, command)
	}
	if decision := m.activeBlockingDecision(); decision != nil && decision.ID != "" {
		parts = append(parts, "decision "+shortID(string(decision.ID)))
	}
	if approval := m.activeBlockingApproval(); approval != nil && approval.ID != "" {
		parts = append(parts, "approval "+shortID(string(approval.ID)))
	}
	if m.mainViewportUserScrolled && m.mainViewportUnreadCount > 0 {
		parts = append(parts, fmt.Sprintf("new updates: %d · PgDn to follow", m.mainViewportUnreadCount))
	} else if m.mainViewportPendingUpdate {
		parts = append(parts, "new updates")
	}
	switch m.subscriptionStatus.State {
	case backend.SubscriptionReconnecting, backend.SubscriptionDegraded, backend.SubscriptionConnecting:
		label := string(m.subscriptionStatus.State)
		if m.subscriptionStatus.StreamRevision > 0 {
			label = fmt.Sprintf("events %s · showing projection revision %d", m.subscriptionStatus.State, m.subscriptionStatus.StreamRevision)
		} else {
			label = "events " + string(m.subscriptionStatus.State)
		}
		parts = append(parts, label)
	}
	return truncateDisplay(strings.Join(parts, " | "), width-2)
}

func runCommandStatusText(command *projectioncontract.RunCommandStatus) string {
	if command == nil {
		return ""
	}
	state := strings.ToLower(strings.TrimSpace(command.State))
	if state == "" {
		switch {
		case strings.TrimSpace(command.RejectedAt) != "":
			state = "rejected"
		case strings.TrimSpace(command.AcceptedAt) != "":
			state = "accepted"
		case strings.TrimSpace(command.RequestID) != "":
			state = "pending"
		default:
			return ""
		}
	}
	parts := []string{"command", state}
	if intent := strings.TrimSpace(command.Intent); intent != "" {
		parts = append(parts, intent)
	}
	if errText := strings.TrimSpace(command.Error); errText != "" {
		parts = append(parts, errText)
	} else if reason := strings.TrimSpace(command.Reason); reason != "" && state == "rejected" {
		parts = append(parts, reason)
	}
	return truncateDisplay(strings.Join(parts, " "), 72)
}

func (m AppModel) visibleArtifactContinuations() []projectioncontract.ArtifactContinuation {
	if len(m.artifactContinuations) == 0 {
		return nil
	}
	limit := min(len(m.artifactContinuations), 5)
	return append([]projectioncontract.ArtifactContinuation(nil), m.artifactContinuations[:limit]...)
}

func (m AppModel) activeArtifactContinuationStatus() string {
	for _, item := range m.artifactContinuations {
		switch item.State {
		case string(common.ArtifactContinuationStateApplied), string(common.ArtifactContinuationStateFailed):
			continue
		default:
			target := strings.TrimSpace(item.TargetRef)
			if target == "" {
				target = strings.TrimSpace(item.ArtifactID)
			}
			if target == "" {
				target = shortID(string(item.ID))
			}
			return truncateDisplay(fmt.Sprintf("artifact %s %d/%s", target, len(item.ReceivedSequences), artifactExpectedSequenceText(item.ExpectedSequence)), 36)
		}
	}
	return ""
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

	openDecisions := len(filterOpenDecisions(m.activeScopeDecisions()))
	pendingApprovals := len(filterPendingApprovals(m.activeScopeApprovals()))
	b.WriteString("\n")
	b.WriteString(Warning.Render(fmt.Sprintf("current ws: %d decisions", openDecisions)))
	b.WriteString("\n")
	b.WriteString(Alert.Render(fmt.Sprintf("current ws: %d approvals", pendingApprovals)))
	b.WriteString("\n")
	return b.String()
}

func (m AppModel) renderInput(width int) string {
	prompt := shortID(m.activeWorkspaceID)
	if prompt == "none" {
		prompt = "workspace"
	}
	switch m.activeWorkspacePromptMode() {
	case workspacePromptModeDecision:
		prompt += " decision"
	case workspacePromptModeApproval:
		prompt += " approval"
	case workspacePromptModeRunning:
		prompt += " running"
	}
	input := m.renderInputBuffer(prompt, width)
	if suggestions := m.renderSlashCommandSuggestions(width); suggestions != "" {
		return suggestions + "\n" + input
	}
	return input
}

func (m AppModel) renderHelp(width int) string {
	help := "enter: submit | /goal | /run resume | /run restart | /dod | /model | /status | /coverage | /timeline | /memory | /config | /exit"
	if width < 120 {
		help = "enter | /goal | /dod | /run resume | /run restart | /status | /coverage | /memory | /exit"
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
		"Workflow:",
		"  /goal <text>         submit a new goal",
		"  /run resume [text]   continue workspace execution",
		"  /run restart          create a fresh workspace activation",
		"  /dod <items>         set acceptance criteria",
		"",
		"Inspect:",
		"  /status              workspace and run status",
		"  /coverage            DoD coverage progress",
		"  /steps               recent step summary",
		"  /timeline            runtime event timeline",
		"  /patches             patch evidence",
		"  /artifacts           artifacts and continuations",
		"  /memory              memory recall",
		"",
		"Governance:",
		"  /decision answer     answer current decision",
		"  /approval approve    approve request",
		"  /approval reject     reject request",
		"",
		"System:",
		"  /workspace           select workspace",
		"  /workspace create    create workspace",
		"  /model               select workspace provider/model",
		"  /model show          show workspace and system model targets",
		"  /model clear         inherit the system default for future runs",
		"  /model reasoning X   set next-run reasoning effort (or default)",
		"  /config              system configuration",
		"  /clear               clear display",
		"  /help                show this help",
		"  /exit                quit",
		"",
		"Enter: submit command; decision/approval prompts accept plain answers",
		"Plain text resumes an active run, or creates a new goal when idle",
		"Alt+Enter: insert newline",
		"Ctrl+P / Ctrl+N: input history",
		"Tab: slash command completion",
		"Ctrl+W: workspace selector",
		"Ctrl+H: help",
		"Ctrl+C: interrupt active run; use /exit to quit",
		"PgUp / PgDn: scroll result",
		"Esc: clear input or close help",
	}
	panelWidth := min(width-4, 78)
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
