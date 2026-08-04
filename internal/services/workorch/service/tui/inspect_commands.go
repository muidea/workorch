package tui

import (
	"fmt"
	"strings"

	backend "github.com/muidea/workorch/internal/pkg/workorchport"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
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
	if status := convergenceStatusText(m.convergenceStatus); status != "" {
		b.WriteString("- convergence: ")
		b.WriteString(status)
		b.WriteString("\n")
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
	if section := renderInspectAcceptance(m.acceptance); section != "" {
		b.WriteString(section)
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
		if latest.Readback != nil {
			b.WriteString("- scheduler readback: ")
			b.WriteString(schedulerReadbackStatusText(latest.Readback))
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
	} else if item.Watchdog.TimerRegistrations > 0 {
		b.WriteString("- watchdog: registered")
		if item.Watchdog.TimerRegistrations > 1 {
			b.WriteString(fmt.Sprintf(" x%d", item.Watchdog.TimerRegistrations))
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func schedulerReadbackStatusText(item *backend.ContinuationReadbackDiagnosticInfo) string {
	if item == nil {
		return ""
	}
	parts := compactStrings([]string{
		item.Action,
		fmt.Sprintf("advanced=%t", item.Advanced),
		item.Reason,
	})
	state := strings.TrimSpace(strings.Join(compactStrings([]string{item.OldRunState, "->", item.NewRunState}), " "))
	if state != "->" && state != "" {
		parts = append(parts, state)
	}
	if item.NewBlockingTicketID != "" {
		parts = append(parts, "ticket="+item.NewBlockingTicketID)
	}
	if item.NewBlockingApprovalID != "" {
		parts = append(parts, "approval="+item.NewBlockingApprovalID)
	}
	if item.ProjectionRevision > 0 {
		parts = append(parts, fmt.Sprintf("projection=%d", item.ProjectionRevision))
	}
	return strings.TrimSpace(strings.Join(compactStrings(parts), " "))
}

func (m AppModel) renderInspectCoverage() string {
	if len(m.dodItems) == 0 && m.repairLoop == "" && acceptanceProjectionEmpty(m.acceptance) {
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
	if section := renderInspectAcceptance(m.acceptance); section != "" {
		if len(m.dodItems) > 0 || m.repairLoop != "" {
			b.WriteString("\n")
		}
		b.WriteString(section)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderInspectAcceptance(item backend.AcceptanceProjectionInfo) string {
	if acceptanceProjectionEmpty(item) {
		return ""
	}
	var b strings.Builder
	b.WriteString("Acceptance\n")
	b.WriteString(fmt.Sprintf("- slices: accepted=%d active=%d open=%d failed=%d\n",
		item.Summary.AcceptedSlices,
		item.Summary.ActiveSlices,
		item.Summary.OpenSlices,
		item.Summary.FailedSlices,
	))
	if item.ActiveSliceID != "" {
		b.WriteString("- active slice: ")
		b.WriteString(item.ActiveSliceID)
		b.WriteString("\n")
	}
	if item.CurrentFocus != nil {
		b.WriteString("- current focus: ")
		b.WriteString(strings.TrimSpace(strings.Join(compactStrings([]string{
			item.CurrentFocus.ID,
			item.CurrentFocus.State,
			item.CurrentFocus.FailureClass,
			item.CurrentFocus.FailureSignature,
		}), " | ")))
		if item.CurrentFocus.Attempt > 0 {
			b.WriteString(fmt.Sprintf(" | attempt=%d", item.CurrentFocus.Attempt))
		}
		b.WriteString("\n")
		if len(item.CurrentFocus.CoverageWorkItemIDs) > 0 {
			b.WriteString("- focus coverage items: ")
			b.WriteString(strings.Join(item.CurrentFocus.CoverageWorkItemIDs, ", "))
			b.WriteString("\n")
		}
		if len(item.CurrentFocus.RequiredEvidenceKinds) > 0 {
			b.WriteString("- required evidence: ")
			b.WriteString(strings.Join(item.CurrentFocus.RequiredEvidenceKinds, ", "))
			b.WriteString("\n")
		}
	}
	if item.Budget != nil {
		b.WriteString("- budget: ")
		b.WriteString(strings.TrimSpace(strings.Join(compactStrings([]string{
			item.Budget.Decision,
			item.Budget.LastDecisionReason,
		}), " | ")))
		if item.Budget.RepairAttempts > 0 || item.Budget.NoCoverageDeltaRounds > 0 {
			b.WriteString(fmt.Sprintf(" | repairs=%d noDelta=%d", item.Budget.RepairAttempts, item.Budget.NoCoverageDeltaRounds))
		}
		b.WriteString("\n")
	}
	if item.RepairDirective != nil {
		directive := item.RepairDirective
		b.WriteString("- canonical repair directive: ")
		b.WriteString(strings.TrimSpace(strings.Join(compactStrings([]string{
			directive.RequiredAction,
			"focus=" + directive.FocusID,
			"trigger=" + directive.Trigger,
		}), " | ")))
		if directive.Closure != nil && directive.Closure.ID != "" {
			b.WriteString(" | closure=")
			b.WriteString(directive.Closure.ID)
		}
		b.WriteString("\n")
	}
	if item.Audit != nil && item.Audit.Failure != nil {
		failure := item.Audit.Failure
		b.WriteString("- runtime failure (diagnostic only): ")
		b.WriteString(strings.TrimSpace(strings.Join(compactStrings([]string{
			failure.Class,
			failure.Code,
			failure.Signature,
			failure.Message,
		}), " | ")))
		if diagnostic := failure.OriginalDiagnostic; diagnostic != nil {
			b.WriteString(" | producer=")
			b.WriteString(diagnostic.OwnerModuleID)
			if diagnostic.OriginalMessage != "" {
				b.WriteString(" | original=")
				b.WriteString(diagnostic.OriginalMessage)
			}
			if diagnostic.ExpectedRevision != "" || diagnostic.ObservedRevision != "" {
				b.WriteString(" | revision=")
				b.WriteString(strings.Join(compactStrings([]string{
					diagnostic.ExpectedRevision,
					diagnostic.ObservedRevision,
				}), "/"))
			}
		}
		b.WriteString("\n")
	}
	if item.Audit != nil && len(item.Audit.Assessments) > 0 {
		observation := item.Audit.Assessments[len(item.Audit.Assessments)-1]
		b.WriteString("- latest assessment observation (diagnostic only): ")
		b.WriteString(strings.TrimSpace(strings.Join(compactStrings([]string{
			"claimed=" + observation.ClaimedDisposition,
			"canonical=" + observation.CanonicalDisposition,
			observation.Summary,
		}), " | ")))
		if observation.Gap != nil {
			if len(observation.Gap.MissingEvidenceKinds) > 0 {
				b.WriteString(" | claimed missing=")
				b.WriteString(strings.Join(observation.Gap.MissingEvidenceKinds, ","))
			}
			if observation.Gap.Reason != "" {
				b.WriteString(" | reason=")
				b.WriteString(observation.Gap.Reason)
			}
		}
		b.WriteString("\n")
	}
	if item.Audit != nil && len(item.Audit.CoverageBindings) > 0 {
		b.WriteString("- coverage evidence bindings (audit only): ")
		bindings := make([]string, 0, len(item.Audit.CoverageBindings))
		for _, binding := range item.Audit.CoverageBindings {
			proofs := make([]string, 0, len(binding.Proofs))
			for _, proof := range binding.Proofs {
				proofs = append(proofs, strings.TrimSpace(strings.Join(compactStrings([]string{
					proof.EvidenceKind,
					proof.ValidationRecipeID,
					strings.Join(proof.RequiredCheckRefs, ","),
				}), "/")))
			}
			bindings = append(bindings, strings.TrimSpace(strings.Join(compactStrings([]string{
				"cwi=" + binding.CoverageWorkItemID,
				"binding=" + binding.ID,
				strings.Join(binding.TargetRefs, ","),
				strings.Join(proofs, "+"),
			}), " | ")))
		}
		b.WriteString(strings.Join(bindings, "; "))
		b.WriteString("\n")
	}
	if item.Audit != nil && len(item.Audit.Dispatches) > 0 {
		b.WriteString("- sealed dispatch operations (audit only): ")
		operations := make([]string, 0)
		for _, dispatch := range item.Audit.Dispatches {
			for _, operation := range dispatch.Operations {
				operations = append(operations, strings.TrimSpace(strings.Join(compactStrings([]string{
					"dispatch=" + dispatch.ID,
					"operation=" + operation.ID,
					"kind=" + operation.Kind,
					"bindings=" + strings.Join(operation.BindingRefs, ","),
					"targets=" + strings.Join(operation.TargetRefs, ","),
				}), " | ")))
			}
		}
		b.WriteString(strings.Join(operations, "; "))
		b.WriteString("\n")
	}
	if item.Audit != nil && (len(item.Audit.AcceptedEvidence) > 0 || len(item.Audit.RejectedEvidence) > 0) {
		b.WriteString("- evidence / derived CWI credit (audit only): ")
		evidence := append(append([]projectioncontract.RuntimeAuditEvidence(nil), item.Audit.AcceptedEvidence...), item.Audit.RejectedEvidence...)
		items := make([]string, 0, len(evidence))
		for _, fact := range evidence {
			items = append(items, strings.TrimSpace(strings.Join(compactStrings([]string{
				"evidence=" + fact.ID,
				"operation=" + fact.OperationID,
				"bindings=" + strings.Join(fact.BindingRefs, ","),
				"credit=" + strings.Join(fact.CoverageWorkItemIDs, ","),
				"checks=" + strings.Join(fact.PassedChecks, ","),
			}), " | ")))
		}
		b.WriteString(strings.Join(items, "; "))
		b.WriteString("\n")
	}
	if item.Audit != nil && len(item.Audit.OperationReceipts) > 0 {
		b.WriteString("- owner operation receipts (audit only): ")
		receipts := make([]string, 0, len(item.Audit.OperationReceipts))
		for _, receipt := range item.Audit.OperationReceipts {
			var ownerModuleID, originalCode, originalMessage string
			if diagnostic := receipt.OriginalDiagnostic; diagnostic != nil {
				ownerModuleID = diagnostic.OwnerModuleID
				originalCode = diagnostic.OriginalCode
				originalMessage = diagnostic.OriginalMessage
			}
			receipts = append(receipts, strings.TrimSpace(strings.Join(compactStrings([]string{
				"operation=" + receipt.OperationID,
				"effect=" + receipt.ObservedEffectScope,
				"recipe=" + receipt.ValidationRecipeID,
				"checks=" + strings.Join(receipt.PassedChecks, ","),
				"feedback=" + strings.Join(compactStrings([]string{ownerModuleID, receipt.StableCategory, originalCode, originalMessage}), "/"),
				"receipt=" + receipt.ExecutionKey,
			}), " | ")))
		}
		b.WriteString(strings.Join(receipts, "; "))
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

func renderInspectActivityFeed(feed backend.WorkspaceActivityFeedInfo, limit int) string {
	if len(feed.Items) == 0 {
		return "No projected activity recorded for the current workspace."
	}
	if limit <= 0 {
		limit = 30
	}
	start := len(feed.Items) - limit
	if start < 0 {
		start = 0
	}
	var b strings.Builder
	b.WriteString("Timeline\n")
	if feed.DisplayRunID != "" {
		b.WriteString("display run: ")
		b.WriteString(string(feed.DisplayRunID))
		b.WriteString("\n")
	}
	for _, item := range feed.Items[start:] {
		line := strings.TrimSpace(item.Summary)
		if line == "" {
			line = strings.TrimSpace(item.Title)
		}
		if line == "" {
			line = string(item.Kind)
		}
		if ts := strings.TrimSpace(item.OccurredAt); ts != "" {
			line = ts + " " + line
		}
		b.WriteString("- ")
		b.WriteString(line)
		b.WriteString("\n")
	}
	if feed.Truncated {
		b.WriteString("(truncated; earlier history available via activity cursor)\n")
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
