package tui

import (
	"fmt"
	"strings"

	backend "github.com/muidea/workorch/internal/pkg/workorchport"
)

func (m AppModel) renderInspectStatus() string {
	var b strings.Builder
	b.WriteString("Status\n")
	b.WriteString("- workspace: ")
	b.WriteString(nonEmpty(m.activeWorkspaceID, "-"))
	b.WriteString("\n")
	b.WriteString("- state: ")
	b.WriteString(string(m.progress))
	if m.progressText != "" {
		b.WriteString(" - ")
		b.WriteString(m.progressText)
	}
	b.WriteString("\n")
	if m.activeRunID != "" {
		b.WriteString("- run: ")
		b.WriteString(m.activeRunID)
		b.WriteString("\n")
	}
	if runtimeState := m.workspaceRuntimeState(m.activeWorkspaceID); runtimeState != nil && (runtimeState.LastRevision > 0 || runtimeState.RevisionGapCount > 0) {
		b.WriteString("- stream revision: ")
		b.WriteString(fmt.Sprintf("last %d, gaps %d", runtimeState.LastRevision, runtimeState.RevisionGapCount))
		if runtimeState.LastRevisionGap > 0 {
			b.WriteString(fmt.Sprintf(", latest gap %d", runtimeState.LastRevisionGap))
		}
		b.WriteString("\n")
	}
	if m.runHeartbeat.ActivityState != "" {
		b.WriteString("- activity: ")
		b.WriteString(m.runHeartbeat.ActivityState)
		if m.runHeartbeat.Summary != "" {
			b.WriteString(" - ")
			b.WriteString(m.runHeartbeat.Summary)
		}
		b.WriteString("\n")
		if m.runHeartbeat.LatestEffectiveProgressAt != "" {
			b.WriteString("- latest progress: ")
			b.WriteString(m.runHeartbeat.LatestEffectiveProgressAt)
			b.WriteString("\n")
		}
	}
	if status := runtimeExecutionStatusText(m.runtimeExecution); status != "" {
		b.WriteString("- runtime execution: ")
		b.WriteString(status)
		b.WriteString("\n")
		if m.runtimeExecution.LastProgressSummary != "" {
			b.WriteString("- runtime progress: ")
			b.WriteString(m.runtimeExecution.LastProgressSummary)
			b.WriteString("\n")
		}
	}
	if scheduler := schedulerDiagnosticsStatusText(m.schedulerDiagnostics); scheduler != "" {
		b.WriteString(scheduler)
		b.WriteString("\n")
	}
	if m.currentTask != "" {
		b.WriteString("- goal: ")
		b.WriteString(m.currentTask)
		b.WriteString("\n")
	}
	if m.repairLoop != "" {
		b.WriteString("- repair: ")
		b.WriteString(m.repairLoop)
		b.WriteString("\n")
	}
	if decision := m.activeBlockingDecision(); decision != nil {
		b.WriteString("- decision: ")
		b.WriteString(blockingDecisionLabel(decision))
		b.WriteString("\n")
	}
	if approval := m.activeBlockingApproval(); approval != nil {
		b.WriteString("- approval: ")
		b.WriteString(blockingApprovalLabel(approval))
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func schedulerDiagnosticsStatusText(item *backend.SchedulerDiagnosticsInfo) string {
	if item == nil {
		return ""
	}
	var b strings.Builder
	if item.LatestContinuation != nil {
		latest := item.LatestContinuation
		b.WriteString("- scheduler: ")
		b.WriteString(strings.TrimSpace(strings.Join(compactStrings([]string{latest.Kind, latest.State, latest.Reason}), " ")))
		if latest.Detail != "" {
			b.WriteString(" - ")
			b.WriteString(latest.Detail)
		}
		b.WriteString("\n")
		if latest.Recoverability != "" || len(latest.RecommendedActions) > 0 {
			b.WriteString("- scheduler recovery: ")
			b.WriteString(strings.TrimSpace(strings.Join(compactStrings([]string{
				latest.Recoverability,
				strings.Join(latest.RecommendedActions, ","),
			}), " ")))
			b.WriteString("\n")
		}
	}
	if item.Watchdog.LatestTickAt != "" {
		b.WriteString("- watchdog: ")
		b.WriteString(item.Watchdog.LatestTickAt)
		if item.Watchdog.LatestDurationMs > 0 {
			b.WriteString(fmt.Sprintf(" %dms", item.Watchdog.LatestDurationMs))
		}
		if item.Watchdog.LatestError != "" {
			b.WriteString(" - ")
			b.WriteString(item.Watchdog.LatestError)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m AppModel) renderInspectCoverage() string {
	if len(m.dodItems) == 0 && m.repairLoop == "" {
		return "No DoD coverage data is available for the current workspace."
	}
	var b strings.Builder
	b.WriteString("Coverage\n")
	for _, item := range m.dodItems {
		b.WriteString("- ")
		b.WriteString(item)
		b.WriteString("\n")
	}
	if m.repairLoop != "" {
		if len(m.dodItems) > 0 {
			b.WriteString("\n")
		}
		b.WriteString("Repair loop: ")
		b.WriteString(m.repairLoop)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m AppModel) renderInspectSteps() string {
	if len(m.taskDecomp) == 0 {
		return "No task decomposition or step summary is available for the current workspace."
	}
	var b strings.Builder
	b.WriteString("Recent steps\n")
	count := 0
	for _, decomp := range m.taskDecomp {
		for _, node := range decomp.Nodes {
			if count >= 20 {
				b.WriteString(fmt.Sprintf("- +%d more nodes\n", taskDecompositionNodeCount(m.taskDecomp)-count))
				return strings.TrimRight(b.String(), "\n")
			}
			label := strings.TrimSpace(strings.Join(compactStrings([]string{
				node.StepState,
				node.Kind,
				node.Title,
				string(node.StepID),
			}), " "))
			if label == "" {
				label = node.ID
			}
			b.WriteString("- ")
			b.WriteString(label)
			b.WriteString("\n")
			count++
		}
	}
	if count == 0 {
		return "No task decomposition nodes are available for the current workspace."
	}
	return strings.TrimRight(b.String(), "\n")
}

func taskDecompositionNodeCount(items []backend.TaskDecompositionInfo) int {
	total := 0
	for _, item := range items {
		total += len(item.Nodes)
	}
	return total
}

func renderInspectTimeline(items []string) string {
	if len(items) == 0 {
		return "No runtime timeline events recorded for the current workspace."
	}
	var b strings.Builder
	b.WriteString("Timeline\n")
	start := len(items) - 30
	if start < 0 {
		start = 0
	}
	for _, item := range items[start:] {
		b.WriteString("- ")
		b.WriteString(item)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderInspectPatches(items []backend.PatchInfo) string {
	if len(items) == 0 {
		return "No patches recorded for the current workspace."
	}
	var b strings.Builder
	b.WriteString("Patches\n")
	for _, item := range items {
		b.WriteString("- ")
		b.WriteString(patchDisplayLabel(item))
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m AppModel) renderInspectArtifacts() string {
	artifacts := m.visibleArtifacts()
	continuations := m.visibleArtifactContinuations()
	if len(artifacts) == 0 && len(continuations) == 0 {
		return "No artifacts recorded for the current workspace."
	}
	var b strings.Builder
	b.WriteString("Artifacts\n")
	for _, item := range artifacts {
		b.WriteString("- ")
		b.WriteString(item)
		b.WriteString("\n")
	}
	if len(continuations) > 0 {
		if len(artifacts) > 0 {
			b.WriteString("\n")
		}
		b.WriteString("Artifact continuations\n")
		for _, item := range continuations {
			b.WriteString("- ")
			b.WriteString(artifactContinuationDisplayLabel(item))
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
