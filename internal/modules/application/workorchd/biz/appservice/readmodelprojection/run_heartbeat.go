package readmodelprojection

import (
	"strings"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const runHeartbeatStalledThreshold = 10 * time.Minute

func mapRunHeartbeat(snapshot readmodelcore.WorkspaceSnapshotState) projectioncontract.RunHeartbeat {
	return mapRunHeartbeatAt(snapshot, time.Now().UTC())
}

func mapRunHeartbeatAt(snapshot readmodelcore.WorkspaceSnapshotState, now time.Time) projectioncontract.RunHeartbeat {
	run := heartbeatRun(snapshot)
	if run == nil {
		return projectioncontract.RunHeartbeat{
			WorkspaceID:   snapshot.Workspace.ID,
			ActivityState: "idle",
			Summary:       "no active run",
		}
	}
	runID := run.ID
	if snapshot.TaskProgress.RunID != runID {
		snapshot.TaskProgress = projectioncontract.TaskProgress{}
	}
	if audit := snapshot.Acceptance.Audit; audit != nil && audit.RunID != runID {
		snapshot.Acceptance = projectioncontract.AcceptanceProjection{}
	}
	ret := projectioncontract.RunHeartbeat{
		WorkspaceID:     firstNonEmptyString(run.WorkspaceID, snapshot.Workspace.ID),
		RunID:           runID,
		State:           run.State,
		LatestStepID:    snapshot.TaskProgress.CurrentStepID,
		LatestStepKind:  snapshot.TaskProgress.CurrentStepKind,
		LatestStepState: snapshot.TaskProgress.CurrentStepState,
	}
	ret.LatestEvidenceRef = latestEvidenceRef(snapshot, runID)
	ret.LatestEventTopic, ret.LatestEventRevision, ret.LatestEventAt = latestEventProgress(snapshot.EventRecords, runID)
	ret.LatestLLMUsageAt = latestLLMUsageProgress(snapshot.LLMUsageTraces, runID)
	ret.LatestResultAt = latestResultProgress(snapshot.Results, runID)
	ret.LatestArtifactAt = latestArtifactProgress(snapshot.Artifacts, snapshot.ArtifactContinuations, runID)
	ret.LatestPatchAt = latestPatchProgress(snapshot.Patches, runID)
	ret.LatestActivityAt = latestStringTime(
		ret.LatestEventAt,
		ret.LatestLLMUsageAt,
		ret.LatestResultAt,
		ret.LatestArtifactAt,
		ret.LatestPatchAt,
		latestMemoryRecallProgress(snapshot.MemoryRecallTraces, runID),
	)
	if summary := snapshot.TaskProgress.Runtime; summary != nil {
		ret.LatestEffectiveProgressAt = summary.LatestAcceptedEvidenceAt
	}
	for _, result := range snapshot.Results {
		if result.RunID == runID && result.State == string(common.ResultStateAccepted) {
			ret.LatestEffectiveProgressAt = latestStringTime(ret.LatestEffectiveProgressAt, result.CreatedAt)
		}
	}
	ret.LatestActivityAt = latestStringTime(ret.LatestActivityAt, ret.LatestEffectiveProgressAt)
	ret.IneffectiveRetry = heartbeatIneffectiveRetry(snapshot)
	ret.ActivityState = heartbeatActivityState(*run, snapshot.TaskProgress, ret)
	if runStateIsLiveForHeartbeat(run.State) {
		ret.Stalled = ret.ActivityState == "no_activity"
		if ret.ActivityState == "running" && heartbeatProgressStalledAt(ret.LatestActivityAt, now) {
			ret.ActivityState = "stalled"
			ret.Stalled = true
			ret.Summary = heartbeatStalledSummary(ret.LatestActivityAt, now)
		}
	}
	ret.Summary = heartbeatSummary(ret)
	return ret
}

func heartbeatIneffectiveRetry(snapshot readmodelcore.WorkspaceSnapshotState) bool {
	if summary := snapshot.TaskProgress.Runtime; summary != nil {
		// Current owner-derived facts replace earlier display flags, including
		// the prior snapshot's ConvergenceStatus. Do not latch a projection bit.
		return runtimeRetryExhausted(summary) || strings.TrimSpace(snapshot.TaskProgress.RepairLoop.RepeatedFailureReason) != ""
	}
	if strings.TrimSpace(snapshot.TaskProgress.RepairLoop.RepeatedFailureReason) != "" || snapshot.TaskProgress.RepairLoop.Escalated {
		return true
	}
	if budget := snapshot.Acceptance.Budget; budget != nil && heartbeatConvergenceBudgetEscalated(budget.Decision) {
		return true
	}
	return false
}

func runtimeRetryExhausted(summary *projectioncontract.RuntimeProgress) bool {
	if summary == nil {
		return false
	}
	// The terminal code is the authoritative SM-3 outcome. Check the zero-retry
	// authority exhaustion first so records persisted before the failure-class
	// split are not displayed as ineffective retries.
	if strings.TrimSpace(summary.TerminalCode) == "candidate_authority_exhausted" || strings.TrimSpace(summary.TerminalCode) == "validation_failure_repair_route_unavailable" {
		return false
	}
	switch strings.TrimSpace(summary.FailureClass) {
	case string(runtimecontract.FailureClassNoProgressExhausted), string(runtimecontract.FailureClassCandidateExhausted):
		return true
	}
	switch strings.TrimSpace(summary.TerminalCode) {
	case "no_progress_exhausted", "candidate_retry_exhausted":
		return true
	default:
		return false
	}
}

func heartbeatConvergenceBudgetEscalated(decision string) bool {
	switch strings.TrimSpace(decision) {
	case "fail", "retry_exhaustion", "no_progress_exhausted", "candidate_retry_exhausted", "budget_exhausted":
		return true
	default:
		return false
	}
}

func heartbeatRun(snapshot readmodelcore.WorkspaceSnapshotState) *appmodelcore.Run {
	if snapshot.Run != nil {
		return snapshot.Run
	}
	if snapshot.Workspace.ActiveRunID != "" {
		for idx := range snapshot.Runs {
			if snapshot.Runs[idx].ID == snapshot.Workspace.ActiveRunID {
				return &snapshot.Runs[idx]
			}
		}
	}
	for idx := len(snapshot.Runs) - 1; idx >= 0; idx-- {
		if runStateIsLiveForHeartbeat(snapshot.Runs[idx].State) {
			return &snapshot.Runs[idx]
		}
	}
	if len(snapshot.Runs) == 0 {
		return nil
	}
	return &snapshot.Runs[len(snapshot.Runs)-1]
}

func latestEvidenceRef(snapshot readmodelcore.WorkspaceSnapshotState, runID string) string {
	if len(snapshot.TaskProgress.EvidenceRefs) > 0 {
		return snapshot.TaskProgress.EvidenceRefs[len(snapshot.TaskProgress.EvidenceRefs)-1]
	}
	for idx := len(snapshot.Results) - 1; idx >= 0; idx-- {
		if snapshot.Results[idx].RunID == runID && len(snapshot.Results[idx].EvidenceRefs) > 0 {
			refs := snapshot.Results[idx].EvidenceRefs
			return refs[len(refs)-1]
		}
	}
	for idx := len(snapshot.Artifacts) - 1; idx >= 0; idx-- {
		if snapshot.Artifacts[idx].RunID == runID && len(snapshot.Artifacts[idx].EvidenceRefs) > 0 {
			refs := snapshot.Artifacts[idx].EvidenceRefs
			return refs[len(refs)-1]
		}
	}
	return ""
}

func latestEventProgress(items []projectioncontract.EventRecord, runID string) (string, int64, string) {
	var topic string
	var revision int64
	var occurredAt string
	for _, item := range items {
		if item.RunID != runID {
			continue
		}
		if latestStringTime(occurredAt, item.OccurredAt) == item.OccurredAt {
			topic = item.Topic
			revision = item.Revision
			occurredAt = item.OccurredAt
		}
	}
	return topic, revision, occurredAt
}

func latestLLMUsageProgress(items []projectioncontract.LLMUsageTrace, runID string) string {
	var latest string
	for _, item := range items {
		if item.RunID != runID {
			continue
		}
		latest = latestStringTime(latest, item.CompletedAt, item.OccurredAt, item.StartedAt)
	}
	return latest
}

func latestResultProgress(items []projectioncontract.Result, runID string) string {
	var latest string
	for _, item := range items {
		if item.RunID != runID {
			continue
		}
		latest = latestStringTime(latest, item.UpdatedAt, item.CreatedAt)
	}
	return latest
}

func latestArtifactProgress(artifacts []projectioncontract.Artifact, continuations []projectioncontract.ArtifactContinuation, runID string) string {
	var latest string
	for _, item := range artifacts {
		if item.RunID != runID {
			continue
		}
		latest = latestStringTime(latest, item.UpdatedAt, item.CreatedAt)
	}
	for _, item := range continuations {
		if item.RunID != runID {
			continue
		}
		latest = latestStringTime(latest, item.UpdatedAt, item.CreatedAt)
	}
	return latest
}

func latestPatchProgress(items []projectioncontract.Patch, runID string) string {
	var latest string
	for _, item := range items {
		if item.RunID != runID {
			continue
		}
		latest = latestStringTime(latest, item.AppliedAt, item.RevertedAt, item.CreatedAt)
	}
	return latest
}

func latestMemoryRecallProgress(items []projectioncontract.MemoryRecallTrace, runID string) string {
	var latest string
	for _, item := range items {
		if item.RunID != runID {
			continue
		}
		latest = latestStringTime(latest, item.OccurredAt)
	}
	return latest
}

func heartbeatActivityState(run appmodelcore.Run, progress projectioncontract.TaskProgress, heartbeat projectioncontract.RunHeartbeat) string {
	state := strings.TrimSpace(run.State)
	switch common.RunState(state) {
	case common.RunStateCompleted, common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		return state
	case common.RunStateBlockedOnDecision:
		return "waiting_decision"
	case common.RunStateBlockedOnApproval:
		return "waiting_approval"
	}
	if strings.TrimSpace(progress.BlockingTicketID) != "" {
		return "waiting_decision"
	}
	if strings.TrimSpace(progress.BlockingApprovalID) != "" {
		return "waiting_approval"
	}
	if heartbeat.IneffectiveRetry {
		return "ineffective_retry"
	}
	if strings.TrimSpace(heartbeat.LatestActivityAt) == "" {
		return "no_activity"
	}
	return "running"
}

func heartbeatSummary(item projectioncontract.RunHeartbeat) string {
	if strings.TrimSpace(item.Summary) != "" {
		return item.Summary
	}
	switch item.ActivityState {
	case "waiting_decision":
		return "waiting on decision"
	case "waiting_approval":
		return "waiting on approval"
	case "ineffective_retry":
		return "ineffective retry detected"
	case "no_activity":
		return "no run activity observed yet"
	case "completed", "failed", "cancelled", "superseded":
		return "run " + item.ActivityState
	default:
		if item.LatestStepID != "" {
			return "latest step " + item.LatestStepID
		}
		return "run activity tracked"
	}
}

func heartbeatProgressStalledAt(latestProgressAt string, now time.Time) bool {
	if now.IsZero() {
		return false
	}
	latestProgressAt = strings.TrimSpace(latestProgressAt)
	if latestProgressAt == "" {
		return false
	}
	latest, err := time.Parse(time.RFC3339Nano, latestProgressAt)
	if err != nil {
		return false
	}
	if now.Before(latest) {
		return false
	}
	return now.Sub(latest) >= runHeartbeatStalledThreshold
}

func heartbeatStalledSummary(latestProgressAt string, now time.Time) string {
	latest, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(latestProgressAt))
	if err != nil || now.Before(latest) {
		return "run stalled"
	}
	return "no run activity for " + now.Sub(latest).Truncate(time.Second).String() + " since " + latestProgressAt
}

func runStateIsLiveForHeartbeat(state string) bool {
	switch common.RunState(state) {
	case common.RunStateCompleted, common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		return false
	default:
		return strings.TrimSpace(state) != ""
	}
}

func latestStringTime(values ...string) string {
	latest := ""
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if latest == "" || stringTimeAfter(value, latest) {
			latest = value
		}
	}
	return latest
}

func stringTimeAfter(candidate string, current string) bool {
	candidateTime, candidateErr := time.Parse(time.RFC3339Nano, candidate)
	currentTime, currentErr := time.Parse(time.RFC3339Nano, current)
	if candidateErr == nil && currentErr == nil {
		return candidateTime.After(currentTime)
	}
	return candidate > current
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
