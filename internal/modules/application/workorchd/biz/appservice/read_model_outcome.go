package appservice

import (
	"strings"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func mapRunOutcome(snapshot RunSnapshot) RunOutcome {
	ret := RunOutcome{
		RunID:    snapshot.Run.ID,
		State:    snapshot.Run.State,
		RawState: snapshot.Run.State,
		Terminal: isTerminalRunState(snapshot.Run.State),
	}
	if common.RunState(snapshot.Run.State) == common.RunStateSuperseded {
		ret.State = string(common.RunStateSuperseded)
		ret.Terminal = true
		ret.Reason = "run superseded"
		ret.Summary = ret.Reason
		return ret
	}
	if result := latestTerminalWorkspaceResult(runScopedResults(snapshot)); result != nil {
		if result.State == string(common.ResultStateAccepted) {
			ret.State = "accepted"
			ret.Accepted = true
		} else {
			ret.State = "failed"
			ret.Reason = result.Summary
		}
		ret.Terminal = true
		ret.ResultID = result.ID
		ret.Summary = result.Summary
		ret.EvidenceRefs = compactUniqueStrings(result.EvidenceRefs)
		ret.ArtifactRefs = compactUniqueStrings(result.ArtifactRefs)
		return ret
	}
	switch common.RunState(snapshot.Run.State) {
	case common.RunStateCompleted:
		ret.State = "failed"
		ret.Terminal = true
		ret.Reason = "run completed without accepted result"
	case common.RunStateFailed:
		ret.State = "failed"
		ret.Terminal = true
		ret.Reason = snapshot.Run.FailureReason
	case common.RunStateCancelled:
		ret.State = string(common.RunStateCancelled)
		ret.Terminal = true
		ret.Reason = "run cancelled"
	case common.RunStateSuperseded:
		ret.State = string(common.RunStateSuperseded)
		ret.Terminal = true
		ret.Reason = "run superseded"
	}
	if ret.Summary == "" {
		ret.Summary = ret.Reason
	}
	return ret
}

func mapWorkspaceOutcome(snapshot WorkspaceSnapshot) WorkspaceOutcome {
	ret := WorkspaceOutcome{
		WorkspaceID:       snapshot.Workspace.ID,
		State:             snapshot.Workspace.State,
		RawWorkspaceState: snapshot.Workspace.State,
	}
	if common.WorkspaceState(snapshot.Workspace.State) == common.WorkspaceStateActive &&
		strings.TrimSpace(snapshot.Workspace.ActiveRunID) == "" {
		ret.State = string(common.WorkspaceStateActive)
		ret.Terminal = false
		ret.Accepted = false
		ret.Reason = "workspace definitions changed; execution must be rerun"
		ret.Summary = ret.Reason
		return ret
	}
	var selectedRun *Run
	if isTerminalProjectionState(snapshot.Workspace.State) {
		selectedRun = latestWorkspaceRunForOutcome(snapshot)
	}
	if selectedRun == nil && snapshot.TaskProgress.RunID != "" {
		for idx := range snapshot.Runs {
			if snapshot.Runs[idx].ID == snapshot.TaskProgress.RunID {
				selectedRun = &snapshot.Runs[idx]
				break
			}
		}
	}
	if selectedRun == nil && snapshot.Workspace.ActiveRunID != "" {
		for idx := range snapshot.Runs {
			if snapshot.Runs[idx].ID == snapshot.Workspace.ActiveRunID {
				selectedRun = &snapshot.Runs[idx]
				break
			}
		}
	}
	if selectedRun == nil && len(snapshot.Runs) > 0 {
		selectedRun = &snapshot.Runs[len(snapshot.Runs)-1]
	}
	if selectedRun == nil {
		return ret
	}
	runSnapshot := RunSnapshot{
		Workspace: snapshot.Workspace,
		Run:       *selectedRun,
		Results:   snapshot.Results,
		Artifacts: snapshot.Artifacts,
	}
	outcome := mapRunOutcome(runSnapshot)
	ret.State = outcome.State
	ret.RunID = outcome.RunID
	ret.ResultID = outcome.ResultID
	ret.Accepted = outcome.Accepted
	ret.Terminal = outcome.Terminal
	ret.Summary = outcome.Summary
	ret.Reason = outcome.Reason
	ret.EvidenceRefs = outcome.EvidenceRefs
	ret.ArtifactRefs = outcome.ArtifactRefs
	return ret
}

func isTerminalProjectionState(state string) bool {
	switch common.WorkspaceState(state) {
	case common.WorkspaceStateCompleted, common.WorkspaceStateFailed, common.WorkspaceStateArchived:
		return true
	default:
		return false
	}
}

func latestWorkspaceRunForOutcome(snapshot WorkspaceSnapshot) *Run {
	if len(snapshot.Runs) == 0 {
		return nil
	}
	runsByID := make(map[string]*Run, len(snapshot.Runs))
	for idx := range snapshot.Runs {
		runsByID[snapshot.Runs[idx].ID] = &snapshot.Runs[idx]
	}
	var selectedEvent *EventRecord
	for idx := range snapshot.EventRecords {
		item := &snapshot.EventRecords[idx]
		if item.WorkspaceID != snapshot.Workspace.ID || item.RunID == "" {
			continue
		}
		if _, ok := runsByID[item.RunID]; !ok {
			continue
		}
		switch item.Topic {
		case events.TopicRunCreated, events.TopicRunStateChanged:
		default:
			continue
		}
		if selectedEvent == nil || item.OccurredAt >= selectedEvent.OccurredAt {
			selectedEvent = item
		}
	}
	if selectedEvent != nil {
		if result := latestTerminalWorkspaceResult(snapshot.Results); result != nil && resultNotBeforeEvent(*result, *selectedEvent) {
			if run := runsByID[result.RunID]; run != nil {
				return run
			}
		}
		return runsByID[selectedEvent.RunID]
	}
	if result := latestTerminalWorkspaceResult(snapshot.Results); result != nil {
		if run := runsByID[result.RunID]; run != nil {
			return run
		}
	}
	if result := primaryAcceptedResult(snapshot.Results); result != nil {
		if run := runsByID[result.RunID]; run != nil {
			return run
		}
	}
	return &snapshot.Runs[len(snapshot.Runs)-1]
}

func latestTerminalWorkspaceResult(items []Result) *Result {
	var selected *Result
	for idx := range items {
		switch common.ResultState(items[idx].State) {
		case common.ResultStateAccepted, common.ResultStateRejected:
		default:
			continue
		}
		if selected == nil || resultTimeValue(items[idx]) >= resultTimeValue(*selected) {
			selected = &items[idx]
		}
	}
	return selected
}

func resultNotBeforeEvent(result Result, event EventRecord) bool {
	resultAt := resultTimeValue(result)
	eventAt := strings.TrimSpace(event.OccurredAt)
	if resultAt == "" || eventAt == "" {
		return resultAt >= eventAt
	}
	resultTime, resultErr := time.Parse(time.RFC3339Nano, resultAt)
	eventTime, eventErr := time.Parse(time.RFC3339Nano, eventAt)
	if resultErr == nil && eventErr == nil {
		return !resultTime.Before(eventTime)
	}
	return resultAt >= eventAt
}

func primaryAcceptedResult(items []Result) *Result {
	var selected *Result
	for idx := range items {
		if items[idx].State != string(common.ResultStateAccepted) {
			continue
		}
		if selected == nil || resultTimeValue(items[idx]) >= resultTimeValue(*selected) {
			selected = &items[idx]
		}
	}
	return selected
}

func resultTimeValue(item Result) string {
	if item.UpdatedAt != "" {
		return item.UpdatedAt
	}
	if item.CreatedAt != "" {
		return item.CreatedAt
	}
	return item.ID
}
