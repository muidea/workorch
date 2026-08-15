package readmodelprojection

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

const (
	defaultRuntimeViewActivityLimit = 50
	maxRuntimeViewActivityLimit     = 200
)

// buildWorkspaceActivityFeedContent builds the full bounded activity item set for a
// snapshot without assigning ActivityRevision. Projection store transactions must
// decide revision: advance to the transaction revision when content changes,
// otherwise keep the previous revision.
//
// When the candidate item set exceeds maxRuntimeViewActivityLimit, older items are
// dropped and store-level Truncated=true is saved with NextCursor empty (FR03).
// Dropped history is not pageable; empty NextCursor terminates client paging.
// RuntimeView read only paginates retained items and must preserve Truncated.
func buildWorkspaceActivityFeedContent(snapshot readmodelcore.WorkspaceSnapshotState) projectioncontract.WorkspaceActivityFeed {
	workspaceID := strings.TrimSpace(snapshot.Workspace.ID)
	displayRunID := selectDisplayRunID(snapshot)
	items := collectWorkspaceActivityItems(snapshot, displayRunID)
	sortWorkspaceActivityItems(items)
	items = dedupeWorkspaceActivityItems(items)
	truncated := false
	nextCursor := ""
	// Bound stored feed to max limit; RuntimeView read only paginates further.
	// Store-level truncation is distinct from read-limit truncation: when the store
	// drops older items, Truncated remains true even on the final page.
	if len(items) > maxRuntimeViewActivityLimit {
		start := len(items) - maxRuntimeViewActivityLimit
		items = append([]projectioncontract.WorkspaceActivityItem(nil), items[start:]...)
		// FR03: store-cap drops older history permanently. Truncated=true signals
		// that earlier items existed, but NextCursor stays empty so clients do not
		// enter an empty-page loop by reusing a retained item ID as beforeCursor.
		truncated = true
		nextCursor = ""
	}
	return projectioncontract.WorkspaceActivityFeed{
		WorkspaceID:  workspaceID,
		DisplayRunID: displayRunID,
		Items:        items,
		Truncated:    truncated,
		NextCursor:   nextCursor,
	}
}

// commitActivityFeedTransaction assigns ActivityRevision for a projection store
// transaction. Content equality is typed field comparison, not hash/fingerprint.
func commitActivityFeedTransaction(previous projectioncontract.WorkspaceActivityFeed, candidateContent projectioncontract.WorkspaceActivityFeed, transactionRevision int64) projectioncontract.WorkspaceActivityFeed {
	ret := cloneActivityFeed(candidateContent)
	// Runtime progress is a stream of successive snapshots. Keep the prior
	// items for the same workspace/display run and merge the newly projected
	// items by stable ID so clients can append them without losing intermediate
	// focus/retry/evidence states. A new display run starts a new stream.
	if strings.TrimSpace(previous.WorkspaceID) != "" &&
		strings.TrimSpace(previous.WorkspaceID) == strings.TrimSpace(ret.WorkspaceID) &&
		strings.TrimSpace(previous.DisplayRunID) == strings.TrimSpace(ret.DisplayRunID) {
		merged := make([]projectioncontract.WorkspaceActivityItem, 0, len(previous.Items)+len(ret.Items))
		merged = append(merged, previous.Items...)
		merged = append(merged, ret.Items...)
		sortWorkspaceActivityItems(merged)
		merged = dedupeWorkspaceActivityItems(merged)
		ret.Truncated = previous.Truncated || ret.Truncated
		if len(merged) > maxRuntimeViewActivityLimit {
			merged = append([]projectioncontract.WorkspaceActivityItem(nil), merged[len(merged)-maxRuntimeViewActivityLimit:]...)
			ret.Truncated = true
			ret.NextCursor = ""
		}
		ret.Items = merged
	}
	if transactionRevision < 0 {
		transactionRevision = 0
	}
	if activityFeedContentEqual(previous, ret) {
		ret.ActivityRevision = previous.ActivityRevision
		return ret
	}
	// Content changed: ActivityRevision advances to this projection transaction revision.
	// Never fall back to source/runtime revisions or content fingerprints.
	if transactionRevision == 0 {
		// First projection / empty transaction revision: still move off previous when content differs.
		if previous.ActivityRevision > 0 {
			ret.ActivityRevision = previous.ActivityRevision + 1
		} else {
			ret.ActivityRevision = 1
		}
		return ret
	}
	if previous.ActivityRevision > 0 && transactionRevision <= previous.ActivityRevision {
		// Content changed but transaction floor did not advance (clock skew /
		// epoch regression). Still move ActivityRevision so dirty gates observe
		// the change without decreasing (FR07).
		ret.ActivityRevision = previous.ActivityRevision + 1
		return ret
	}
	ret.ActivityRevision = transactionRevision
	return ret
}

// pageWorkspaceActivityFeed applies limit/beforeCursor to a stored feed without
// regenerating ActivityRevision.
//
// Store-level Truncated (set when buildWorkspaceActivityFeedContent drops items
// beyond maxRuntimeViewActivityLimit) is preserved even when the current page is
// the last page of the retained feed. Read-limit truncation and store-cap
// truncation are therefore distinguishable: a final page with Truncated=true means
// earlier history was dropped from the store, not merely paged out.
func pageWorkspaceActivityFeed(feed projectioncontract.WorkspaceActivityFeed, limit int, beforeCursor string) projectioncontract.WorkspaceActivityFeed {
	ret := cloneActivityFeed(feed)
	storeTruncated := feed.Truncated
	if limit <= 0 {
		limit = defaultRuntimeViewActivityLimit
	}
	if limit > maxRuntimeViewActivityLimit {
		limit = maxRuntimeViewActivityLimit
	}
	items := ret.Items
	if beforeCursor != "" {
		items = activityItemsBeforeCursor(items, beforeCursor)
	}
	pageTruncated := false
	nextCursor := ""
	if len(items) > limit {
		pageTruncated = true
		start := len(items) - limit
		nextCursor = items[start].ID
		items = append([]projectioncontract.WorkspaceActivityItem(nil), items[start:]...)
	} else {
		items = append([]projectioncontract.WorkspaceActivityItem(nil), items...)
		// Last page of retained feed: if the store itself dropped older items,
		// keep Truncated=true. NextCursor must be empty so paging terminates
		// (FR03) — dropped history is not loadable via beforeCursor.
		if storeTruncated {
			pageTruncated = true
			nextCursor = ""
		}
	}
	ret.Items = items
	ret.Truncated = pageTruncated
	ret.NextCursor = nextCursor
	return ret
}

// activityFeedContentEqual compares activity content without revision.
// Truncated/NextCursor are store-owned boundary facts and must participate so
// cap transitions advance ActivityRevision and ChangedSections (FR03).
func activityFeedContentEqual(a, b projectioncontract.WorkspaceActivityFeed) bool {
	if strings.TrimSpace(a.WorkspaceID) != strings.TrimSpace(b.WorkspaceID) {
		return false
	}
	if strings.TrimSpace(a.DisplayRunID) != strings.TrimSpace(b.DisplayRunID) {
		return false
	}
	if a.Truncated != b.Truncated {
		return false
	}
	if strings.TrimSpace(a.NextCursor) != strings.TrimSpace(b.NextCursor) {
		return false
	}
	return activityItemsEqual(a.Items, b.Items)
}

// activityItemsEqual compares ordered activity items by typed fields.
func activityItemsEqual(a, b []projectioncontract.WorkspaceActivityItem) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !activityItemEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func activityItemEqual(a, b projectioncontract.WorkspaceActivityItem) bool {
	return strings.TrimSpace(a.ID) == strings.TrimSpace(b.ID) &&
		strings.TrimSpace(a.WorkspaceID) == strings.TrimSpace(b.WorkspaceID) &&
		strings.TrimSpace(a.RunID) == strings.TrimSpace(b.RunID) &&
		strings.TrimSpace(a.StepID) == strings.TrimSpace(b.StepID) &&
		a.Kind == b.Kind &&
		strings.TrimSpace(a.State) == strings.TrimSpace(b.State) &&
		strings.TrimSpace(a.Title) == strings.TrimSpace(b.Title) &&
		strings.TrimSpace(a.Summary) == strings.TrimSpace(b.Summary) &&
		strings.TrimSpace(a.OccurredAt) == strings.TrimSpace(b.OccurredAt) &&
		a.SourceEventRevision == b.SourceEventRevision &&
		a.RuntimeStateRevision == b.RuntimeStateRevision &&
		a.EvidenceSequence == b.EvidenceSequence
}

func cloneActivityFeed(feed projectioncontract.WorkspaceActivityFeed) projectioncontract.WorkspaceActivityFeed {
	ret := feed
	if feed.Items != nil {
		ret.Items = append([]projectioncontract.WorkspaceActivityItem(nil), feed.Items...)
	}
	return ret
}

func collectWorkspaceActivityItems(snapshot readmodelcore.WorkspaceSnapshotState, displayRunID string) []projectioncontract.WorkspaceActivityItem {
	workspaceID := strings.TrimSpace(snapshot.Workspace.ID)
	items := make([]projectioncontract.WorkspaceActivityItem, 0, 32)

	if runtime := snapshot.TaskProgress.Runtime; runtime != nil {
		items = append(items, runtimeProgressActivityItems(workspaceID, displayRunID, *runtime, snapshot)...)
	}
	items = append(items, governanceActivityItems(snapshot, displayRunID)...)
	items = append(items, resultActivityItems(snapshot, displayRunID)...)
	items = append(items, artifactActivityItems(snapshot, displayRunID)...)
	items = append(items, patchActivityItems(snapshot, displayRunID)...)
	items = append(items, memoryActivityItems(snapshot, displayRunID)...)
	items = append(items, eventRecordActivityItems(snapshot, displayRunID)...)
	if outcome := snapshot.Outcome; strings.TrimSpace(outcome.State) != "" || strings.TrimSpace(outcome.Summary) != "" {
		items = append(items, outcomeActivityItem(workspaceID, displayRunID, outcome))
	}
	return items
}

func runtimeProgressActivityItems(workspaceID, displayRunID string, runtime projectioncontract.RuntimeProgress, snapshot readmodelcore.WorkspaceSnapshotState) []projectioncontract.WorkspaceActivityItem {
	occurredAt := firstNonEmptyTimelineString(snapshot.RunHeartbeat.LatestEventAt, snapshot.ViewModel.UpdatedAt, snapshot.ViewModel.RebuiltAt)
	items := make([]projectioncontract.WorkspaceActivityItem, 0, 4)
	if focusID := strings.TrimSpace(runtime.CurrentFocusID); focusID != "" {
		items = append(items, projectioncontract.WorkspaceActivityItem{
			ID:                   activityID("runtime", displayRunID, "focus", focusID, strconv.FormatUint(runtime.StateRevision, 10)),
			WorkspaceID:          workspaceID,
			RunID:                displayRunID,
			Kind:                 projectioncontract.WorkspaceActivityRuntimeTransition,
			State:                firstNonEmptyTimelineString(snapshot.TaskProgress.State, snapshot.Outcome.State),
			Title:                "runtime focus",
			Summary:              fmt.Sprintf("runtime focus %s activated", focusID),
			OccurredAt:           occurredAt,
			RuntimeStateRevision: runtime.StateRevision,
			EvidenceSequence:     runtime.EvidenceSequence,
		})
	}
	if state := strings.TrimSpace(runtime.ConvergenceState); state != "" {
		items = append(items, projectioncontract.WorkspaceActivityItem{
			ID:                   activityID("runtime", displayRunID, "convergence", state, strconv.FormatUint(runtime.StateRevision, 10)),
			WorkspaceID:          workspaceID,
			RunID:                displayRunID,
			Kind:                 projectioncontract.WorkspaceActivityRuntimeTransition,
			State:                state,
			Title:                "runtime convergence",
			Summary:              fmt.Sprintf("runtime convergence %s", state),
			OccurredAt:           occurredAt,
			RuntimeStateRevision: runtime.StateRevision,
			EvidenceSequence:     runtime.EvidenceSequence,
		})
	}
	if runtime.CompletedDispatchCount > 0 || strings.TrimSpace(runtime.ActiveDispatchID) != "" {
		dispatchID := firstNonEmptyTimelineString(runtime.ActiveDispatchID, strconv.Itoa(runtime.CompletedDispatchCount))
		items = append(items, projectioncontract.WorkspaceActivityItem{
			ID:                   activityID("dispatch", displayRunID, dispatchID, strconv.FormatUint(runtime.StateRevision, 10)),
			WorkspaceID:          workspaceID,
			RunID:                displayRunID,
			Kind:                 projectioncontract.WorkspaceActivityDispatch,
			State:                firstNonEmptyTimelineString(runtime.ConvergenceState, "dispatch"),
			Title:                "dispatch",
			Summary:              fmt.Sprintf("dispatch %s | completed %d | attempts %d", firstNonEmptyTimelineString(runtime.ActiveDispatchID, "none"), runtime.CompletedDispatchCount, runtime.CandidateAttemptCount),
			OccurredAt:           occurredAt,
			RuntimeStateRevision: runtime.StateRevision,
			EvidenceSequence:     runtime.EvidenceSequence,
		})
	}
	if state := strings.TrimSpace(runtime.PrerequisiteRefreshState); state != "" {
		summary := fmt.Sprintf("prerequisite refresh %s | %d/%d", state, runtime.PrerequisiteRefreshUsed, runtime.PrerequisiteRefreshLimit)
		if len(runtime.PrerequisiteInvalidations) > 0 {
			invalidation := runtime.PrerequisiteInvalidations[0]
			resource := strings.TrimSpace(invalidation.ResourceDomain + ":" + invalidation.SubjectRef)
			if resource != ":" {
				summary += " | " + resource
			}
		}
		items = append(items, projectioncontract.WorkspaceActivityItem{
			ID:                   activityID("runtime", displayRunID, "prerequisite-refresh", state, strconv.FormatUint(runtime.StateRevision, 10)),
			WorkspaceID:          workspaceID,
			RunID:                displayRunID,
			Kind:                 projectioncontract.WorkspaceActivityRuntimeTransition,
			State:                state,
			Title:                "prerequisite refresh",
			Summary:              summary,
			OccurredAt:           occurredAt,
			RuntimeStateRevision: runtime.StateRevision,
			EvidenceSequence:     runtime.EvidenceSequence,
		})
	}
	if runtime.AcceptedEvidenceCount > 0 || runtime.RejectedEvidenceCount > 0 || len(runtime.MissingEvidenceKinds) > 0 {
		missing := "none"
		if len(runtime.MissingEvidenceKinds) > 0 {
			missing = strings.Join(runtime.MissingEvidenceKinds, ",")
		}
		items = append(items, projectioncontract.WorkspaceActivityItem{
			ID:                   activityID("evidence", displayRunID, strconv.FormatUint(runtime.EvidenceSequence, 10), strconv.Itoa(runtime.AcceptedEvidenceCount), strconv.Itoa(runtime.RejectedEvidenceCount)),
			WorkspaceID:          workspaceID,
			RunID:                displayRunID,
			Kind:                 projectioncontract.WorkspaceActivityEvidence,
			State:                firstNonEmptyTimelineString(runtime.FailureClass, "evidence"),
			Title:                "evidence",
			Summary:              fmt.Sprintf("evidence accepted %d rejected %d | missing %s", runtime.AcceptedEvidenceCount, runtime.RejectedEvidenceCount, missing),
			OccurredAt:           occurredAt,
			RuntimeStateRevision: runtime.StateRevision,
			EvidenceSequence:     runtime.EvidenceSequence,
		})
	}
	if msg := strings.TrimSpace(runtime.FailureMessage); msg != "" || strings.TrimSpace(runtime.FailureClass) != "" {
		items = append(items, projectioncontract.WorkspaceActivityItem{
			ID:                   activityID("failure", displayRunID, runtime.FailureClass, runtime.FailureCode, strconv.FormatUint(runtime.StateRevision, 10)),
			WorkspaceID:          workspaceID,
			RunID:                displayRunID,
			Kind:                 projectioncontract.WorkspaceActivityFailure,
			State:                firstNonEmptyTimelineString(runtime.FailureClass, "failed"),
			Title:                "runtime failure",
			Summary:              firstNonEmptyTimelineString(msg, runtime.FailureClass, runtime.FailureCode, "runtime failure"),
			OccurredAt:           occurredAt,
			RuntimeStateRevision: runtime.StateRevision,
			EvidenceSequence:     runtime.EvidenceSequence,
		})
	}
	return items
}

func governanceActivityItems(snapshot readmodelcore.WorkspaceSnapshotState, displayRunID string) []projectioncontract.WorkspaceActivityItem {
	workspaceID := strings.TrimSpace(snapshot.Workspace.ID)
	items := make([]projectioncontract.WorkspaceActivityItem, 0, len(snapshot.Decisions)+len(snapshot.Approvals))
	for _, decision := range snapshot.Decisions {
		if displayRunID != "" && strings.TrimSpace(decision.RunID) != "" && strings.TrimSpace(decision.RunID) != displayRunID {
			continue
		}
		items = append(items, projectioncontract.WorkspaceActivityItem{
			ID:          activityID("governance", "decision", decision.ID, decision.State),
			WorkspaceID: workspaceID,
			RunID:       firstNonEmptyTimelineString(decision.RunID, displayRunID),
			StepID:      decision.StepID,
			Kind:        projectioncontract.WorkspaceActivityGovernance,
			State:       decision.State,
			Title:       "decision",
			Summary:     firstNonEmptyTimelineString(decision.Question, decision.Reason, "decision "+decision.ID),
			OccurredAt:  firstNonEmptyTimelineString(decision.UpdatedAt, decision.ResolvedAt, decision.CreatedAt),
		})
	}
	for _, approval := range snapshot.Approvals {
		if displayRunID != "" && strings.TrimSpace(approval.RunID) != "" && strings.TrimSpace(approval.RunID) != displayRunID {
			continue
		}
		items = append(items, projectioncontract.WorkspaceActivityItem{
			ID:          activityID("governance", "approval", approval.ID, approval.State),
			WorkspaceID: workspaceID,
			RunID:       firstNonEmptyTimelineString(approval.RunID, displayRunID),
			StepID:      approval.StepID,
			Kind:        projectioncontract.WorkspaceActivityGovernance,
			State:       approval.State,
			Title:       "approval",
			Summary:     firstNonEmptyTimelineString(approval.Reason, "approval "+approval.ID),
			OccurredAt:  firstNonEmptyTimelineString(approval.UpdatedAt, approval.ResolvedAt, approval.CreatedAt),
		})
	}
	return items
}

func resultActivityItems(snapshot readmodelcore.WorkspaceSnapshotState, displayRunID string) []projectioncontract.WorkspaceActivityItem {
	workspaceID := strings.TrimSpace(snapshot.Workspace.ID)
	items := make([]projectioncontract.WorkspaceActivityItem, 0, len(snapshot.Results))
	for _, result := range snapshot.Results {
		if displayRunID != "" && strings.TrimSpace(result.RunID) != "" && strings.TrimSpace(result.RunID) != displayRunID {
			continue
		}
		items = append(items, projectioncontract.WorkspaceActivityItem{
			ID:          activityID("result", result.ID, result.State),
			WorkspaceID: workspaceID,
			RunID:       firstNonEmptyTimelineString(result.RunID, displayRunID),
			Kind:        projectioncontract.WorkspaceActivityResult,
			State:       result.State,
			Title:       "result",
			Summary:     firstNonEmptyTimelineString(result.Summary, result.State, "result "+result.ID),
			OccurredAt:  firstNonEmptyTimelineString(result.UpdatedAt, result.CreatedAt),
		})
	}
	return items
}

func artifactActivityItems(snapshot readmodelcore.WorkspaceSnapshotState, displayRunID string) []projectioncontract.WorkspaceActivityItem {
	workspaceID := strings.TrimSpace(snapshot.Workspace.ID)
	items := make([]projectioncontract.WorkspaceActivityItem, 0, len(snapshot.Artifacts))
	for _, artifact := range snapshot.Artifacts {
		if displayRunID != "" && strings.TrimSpace(artifact.RunID) != "" && strings.TrimSpace(artifact.RunID) != displayRunID {
			continue
		}
		items = append(items, projectioncontract.WorkspaceActivityItem{
			ID:          activityID("artifact", artifact.ID, artifact.State),
			WorkspaceID: workspaceID,
			RunID:       firstNonEmptyTimelineString(artifact.RunID, displayRunID),
			Kind:        projectioncontract.WorkspaceActivityArtifact,
			State:       artifact.State,
			Title:       "artifact",
			Summary:     firstNonEmptyTimelineString(artifact.Summary, artifact.Name, artifact.URI, "artifact "+artifact.ID),
			OccurredAt:  firstNonEmptyTimelineString(artifact.UpdatedAt, artifact.CreatedAt),
		})
	}
	return items
}

func patchActivityItems(snapshot readmodelcore.WorkspaceSnapshotState, displayRunID string) []projectioncontract.WorkspaceActivityItem {
	workspaceID := strings.TrimSpace(snapshot.Workspace.ID)
	items := make([]projectioncontract.WorkspaceActivityItem, 0, len(snapshot.Patches))
	for _, patch := range snapshot.Patches {
		if displayRunID != "" && strings.TrimSpace(patch.RunID) != "" && strings.TrimSpace(patch.RunID) != displayRunID {
			continue
		}
		items = append(items, projectioncontract.WorkspaceActivityItem{
			ID:          activityID("patch", patch.ID, patch.State),
			WorkspaceID: workspaceID,
			RunID:       firstNonEmptyTimelineString(patch.RunID, displayRunID),
			Kind:        projectioncontract.WorkspaceActivityPatch,
			State:       patch.State,
			Title:       "patch",
			Summary:     firstNonEmptyTimelineString(patch.TargetRef, patch.Summary, "patch "+patch.ID),
			OccurredAt:  firstNonEmptyTimelineString(patch.AppliedAt, patch.CreatedAt),
		})
	}
	return items
}

func memoryActivityItems(snapshot readmodelcore.WorkspaceSnapshotState, displayRunID string) []projectioncontract.WorkspaceActivityItem {
	workspaceID := strings.TrimSpace(snapshot.Workspace.ID)
	items := make([]projectioncontract.WorkspaceActivityItem, 0, len(snapshot.MemoryRecallTraces))
	for _, trace := range snapshot.MemoryRecallTraces {
		if displayRunID != "" && strings.TrimSpace(trace.RunID) != "" && strings.TrimSpace(trace.RunID) != displayRunID {
			continue
		}
		items = append(items, projectioncontract.WorkspaceActivityItem{
			ID:          activityID("memory", firstNonEmptyTimelineString(trace.ID, trace.RunID), trace.Status),
			WorkspaceID: workspaceID,
			RunID:       firstNonEmptyTimelineString(trace.RunID, displayRunID),
			Kind:        projectioncontract.WorkspaceActivityMemory,
			State:       trace.Status,
			Title:       "memory recall",
			Summary:     memoryRecallTimelineSummary(trace),
			OccurredAt:  trace.OccurredAt,
		})
	}
	return items
}

func eventRecordActivityItems(snapshot readmodelcore.WorkspaceSnapshotState, displayRunID string) []projectioncontract.WorkspaceActivityItem {
	workspaceID := strings.TrimSpace(snapshot.Workspace.ID)
	items := make([]projectioncontract.WorkspaceActivityItem, 0, len(snapshot.EventRecords))
	for _, record := range snapshot.EventRecords {
		if displayRunID != "" && strings.TrimSpace(record.RunID) != "" && strings.TrimSpace(record.RunID) != displayRunID {
			continue
		}
		kind, title, summary := classifyEventRecordActivity(record)
		if kind == "" {
			continue
		}
		items = append(items, projectioncontract.WorkspaceActivityItem{
			ID:                  activityID("event", record.ID, record.Topic, record.SubjectType),
			WorkspaceID:         firstNonEmptyTimelineString(record.WorkspaceID, workspaceID),
			RunID:               firstNonEmptyTimelineString(record.RunID, displayRunID),
			StepID:              record.StepID,
			Kind:                kind,
			State:               eventRecordState(record),
			Title:               title,
			Summary:             summary,
			OccurredAt:          record.OccurredAt,
			SourceEventRevision: record.Revision,
		})
	}
	return items
}

func outcomeActivityItem(workspaceID, displayRunID string, outcome projectioncontract.WorkspaceOutcome) projectioncontract.WorkspaceActivityItem {
	kind := projectioncontract.WorkspaceActivityResult
	if !outcome.Accepted && outcome.Terminal {
		kind = projectioncontract.WorkspaceActivityFailure
	}
	return projectioncontract.WorkspaceActivityItem{
		ID:          activityID("outcome", firstNonEmptyTimelineString(outcome.RunID, displayRunID), outcome.State, outcome.ResultID),
		WorkspaceID: firstNonEmptyTimelineString(outcome.WorkspaceID, workspaceID),
		RunID:       firstNonEmptyTimelineString(outcome.RunID, displayRunID),
		Kind:        kind,
		State:       outcome.State,
		Title:       "outcome",
		Summary:     firstNonEmptyTimelineString(outcome.Summary, outcome.Reason, outcome.State),
	}
}

func classifyEventRecordActivity(record projectioncontract.EventRecord) (projectioncontract.WorkspaceActivityKind, string, string) {
	topic := strings.ToLower(strings.TrimSpace(record.Topic))
	subject := strings.ToLower(strings.TrimSpace(record.SubjectType))
	summary := firstNonEmptyTimelineString(eventRecordSummary(record), record.Topic, record.SubjectType, record.ID)
	switch {
	case strings.Contains(topic, "decision") || strings.Contains(topic, "approval") || strings.Contains(topic, "governance") ||
		strings.Contains(subject, "decision") || strings.Contains(subject, "approval"):
		return projectioncontract.WorkspaceActivityGovernance, "governance", summary
	case strings.Contains(topic, "patch") || subject == "patch":
		return projectioncontract.WorkspaceActivityPatch, "patch", summary
	case strings.Contains(topic, "artifact") || subject == "artifact":
		return projectioncontract.WorkspaceActivityArtifact, "artifact", summary
	case strings.Contains(topic, "memory") || subject == "memory":
		return projectioncontract.WorkspaceActivityMemory, "memory", summary
	case strings.Contains(topic, "result") || subject == "result":
		return projectioncontract.WorkspaceActivityResult, "result", summary
	case strings.Contains(topic, "evidence") || subject == "evidence":
		return projectioncontract.WorkspaceActivityEvidence, "evidence", summary
	case strings.Contains(topic, "dispatch") || subject == "dispatch":
		return projectioncontract.WorkspaceActivityDispatch, "dispatch", summary
	case strings.Contains(topic, "run") || strings.Contains(topic, "runtime") || subject == "run" || subject == "runtime":
		return projectioncontract.WorkspaceActivityRuntimeTransition, "runtime", summary
	case strings.Contains(topic, "fail") || strings.Contains(summary, "fail"):
		return projectioncontract.WorkspaceActivityFailure, "failure", summary
	default:
		// Unknown topics are omitted rather than rendered as raw event dumps.
		return "", "", ""
	}
}

func eventRecordSummary(record projectioncontract.EventRecord) string {
	if summary, ok := record.Payload.String(recordpayload.PayloadKeySummary); ok {
		if value := strings.TrimSpace(summary); value != "" {
			return value
		}
	}
	if reason, ok := record.Payload.String(recordpayload.PayloadKeyReason); ok {
		if value := strings.TrimSpace(reason); value != "" {
			return value
		}
	}
	if state, ok := record.Payload.String(recordpayload.PayloadKeyState); ok {
		if value := strings.TrimSpace(state); value != "" {
			return value
		}
	}
	return ""
}

func eventRecordState(record projectioncontract.EventRecord) string {
	if state, ok := record.Payload.String(recordpayload.PayloadKeyState); ok {
		return strings.TrimSpace(state)
	}
	return strings.TrimSpace(record.SubjectType)
}

func sortWorkspaceActivityItems(items []projectioncontract.WorkspaceActivityItem) {
	sort.SliceStable(items, func(i, j int) bool {
		left := items[i]
		right := items[j]
		if left.OccurredAt != right.OccurredAt {
			return left.OccurredAt < right.OccurredAt
		}
		if left.SourceEventRevision != right.SourceEventRevision {
			return left.SourceEventRevision < right.SourceEventRevision
		}
		if left.RuntimeStateRevision != right.RuntimeStateRevision {
			return left.RuntimeStateRevision < right.RuntimeStateRevision
		}
		return left.ID < right.ID
	})
}

func dedupeWorkspaceActivityItems(items []projectioncontract.WorkspaceActivityItem) []projectioncontract.WorkspaceActivityItem {
	if len(items) == 0 {
		return items
	}
	seen := make(map[string]struct{}, len(items))
	ret := make([]projectioncontract.WorkspaceActivityItem, 0, len(items))
	for _, item := range items {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ret = append(ret, item)
	}
	return ret
}

func activityItemsBeforeCursor(items []projectioncontract.WorkspaceActivityItem, beforeCursor string) []projectioncontract.WorkspaceActivityItem {
	beforeCursor = strings.TrimSpace(beforeCursor)
	if beforeCursor == "" {
		return items
	}
	for idx, item := range items {
		if item.ID == beforeCursor {
			return append([]projectioncontract.WorkspaceActivityItem(nil), items[:idx]...)
		}
	}
	// Unknown/stale cursor (including store-cap sentinel or dropped IDs): terminate
	// paging with an empty page instead of replaying the full retained feed (FR03).
	return nil
}

func activityID(parts ...string) string {
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		cleaned = append(cleaned, part)
	}
	if len(cleaned) == 0 {
		return "activity"
	}
	return strings.Join(cleaned, ":")
}
