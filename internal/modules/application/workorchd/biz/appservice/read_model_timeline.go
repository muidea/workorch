package appservice

import (
	"sort"
	"strconv"

	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

const (
	maxRawStreamTimelineItems  = 32
	streamTimelineHeadItemKeep = 12
	streamTimelineTailItemKeep = 12
)

func runtimeEvidenceSummary(snapshot RunSnapshot, eventTotal int) RuntimeEvidenceSummary {
	ret := RuntimeEvidenceSummary{
		ToolCallTotal:          len(snapshot.ToolCalls),
		PatchTotal:             len(snapshot.Patches),
		ValidationTotal:        len(snapshot.ValidationResults),
		RuntimeCheckpointTotal: len(snapshot.RuntimeCheckpoints),
		ResultTotal:            len(snapshot.Results),
		ArtifactTotal:          len(snapshot.Artifacts),
		EventTotal:             eventTotal,
	}
	for _, item := range snapshot.ToolCalls {
		switch item.State {
		case string(common.ToolCallStateRunning):
			ret.ToolCallRunning++
		case string(common.ToolCallStateCompleted):
			ret.ToolCallCompleted++
		case string(common.ToolCallStateFailed):
			ret.ToolCallFailed++
		}
	}
	for _, item := range snapshot.Patches {
		switch item.State {
		case string(common.PatchStateApplied):
			ret.PatchApplied++
		case string(common.PatchStateFailed):
			ret.PatchFailed++
		}
	}
	for _, item := range snapshot.ValidationResults {
		switch item.State {
		case string(common.ValidationResultStatePassed):
			ret.ValidationPassed++
		case string(common.ValidationResultStateFailed):
			ret.ValidationFailed++
		case string(common.ValidationResultStateWarning):
			ret.ValidationWarning++
		}
	}
	return ret
}

func latestRuntimeCheckpoint(items []RuntimeCheckpoint) *RuntimeCheckpoint {
	if len(items) == 0 {
		return nil
	}
	latest := items[0]
	for _, item := range items[1:] {
		if item.OccurredAt > latest.OccurredAt ||
			(item.OccurredAt == latest.OccurredAt && item.ID > latest.ID) {
			latest = item
		}
	}
	return &latest
}

func runtimeTimeline(snapshot RunSnapshot) []RuntimeTimelineItem {
	items := make([]RuntimeTimelineItem, 0, len(snapshot.Steps)+len(snapshot.RuntimeCheckpoints)+len(snapshot.Observed))
	stepOccurrences := timelineStepOccurrences(snapshot)
	for _, trace := range snapshot.MemoryRecallTraces {
		items = append(items, RuntimeTimelineItem{
			Source:     "memory_recall",
			Type:       trace.Source,
			ID:         trace.ID,
			RunState:   snapshot.Run.State,
			StepID:     trace.StepID,
			Summary:    memoryRecallTimelineSummary(trace),
			OccurredAt: trace.OccurredAt,
			Data: map[string]any{
				"stage":              trace.Stage,
				"source":             trace.Source,
				"policy":             trace.Policy,
				"status":             trace.Status,
				"retrievedMemoryIds": append([]string(nil), trace.RetrievedMemoryIDs...),
				"reason":             trace.Reason,
			},
		})
	}
	for _, step := range snapshot.Steps {
		occurredAt := stepOccurrences[step.ID]
		stepDisplayState := firstNonEmptyReadModel(step.State, snapshot.Run.State)
		items = append(items, RuntimeTimelineItem{
			Source:     "step",
			Type:       step.Kind,
			ID:         step.ID,
			RunState:   stepDisplayState,
			StepID:     step.ID,
			StepState:  step.State,
			Summary:    timelineStepSummary(step.Output),
			OccurredAt: occurredAt,
			Output:     cloneReadModelMap(step.Output),
		})
		items = append(items, streamTimelineItems(step, stepDisplayState, occurredAt)...)
	}
	for _, checkpoint := range snapshot.RuntimeCheckpoints {
		items = append(items, RuntimeTimelineItem{
			Source:     "checkpoint",
			Type:       checkpoint.CheckpointType,
			ID:         checkpoint.ID,
			RunState:   checkpoint.RunStateSnapshot,
			StepID:     checkpoint.CurrentStepID,
			OccurredAt: checkpoint.OccurredAt,
			Data:       cloneReadModelMap(checkpoint.Data),
		})
	}
	for _, item := range snapshot.EventRecords {
		if item.RunID != "" && item.RunID != snapshot.Run.ID {
			continue
		}
		if streamItem, ok := streamProgressTimelineItemFromPersistedEvent(item, snapshot.Run.State); ok {
			items = append(items, streamItem)
			continue
		}
		items = append(items, RuntimeTimelineItem{
			Source:     "persisted_event",
			Type:       item.SubjectType,
			ID:         item.ID,
			Topic:      item.Topic,
			RunState:   readModelStringValue(item.Payload[events.PayloadKeyState]),
			StepID:     item.StepID,
			OccurredAt: item.OccurredAt,
			Data:       cloneReadModelMap(item.Payload),
		})
	}
	if len(snapshot.EventRecords) == 0 {
		for _, item := range snapshot.Observed {
			if item.RunID != "" && item.RunID != snapshot.Run.ID {
				continue
			}
			if streamItem, ok := streamProgressTimelineItemFromObserved(item, snapshot.Run.State); ok {
				items = append(items, streamItem)
				continue
			}
			items = append(items, RuntimeTimelineItem{
				Source:     "event",
				Type:       item.SubjectType,
				ID:         item.SubjectID,
				Topic:      item.Topic,
				StepID:     item.StepID,
				OccurredAt: item.OccurredAt,
				Data:       cloneReadModelMap(item.Payload),
			})
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].OccurredAt == items[j].OccurredAt {
			return items[i].Source < items[j].Source
		}
		return items[i].OccurredAt < items[j].OccurredAt
	})
	return items
}

func timelineStepOccurrences(snapshot RunSnapshot) map[string]string {
	ret := map[string]string{}
	remember := func(stepID string, occurredAt string) {
		if stepID == "" || occurredAt == "" {
			return
		}
		if ret[stepID] == "" || occurredAt > ret[stepID] {
			ret[stepID] = occurredAt
		}
	}
	for _, item := range snapshot.EventRecords {
		stepID, ok := timelineStepIDFromEventRecord(item)
		if !ok {
			continue
		}
		remember(stepID, item.OccurredAt)
	}
	for _, item := range snapshot.Observed {
		stepID, ok := timelineStepIDFromObservedEvent(item)
		if !ok {
			continue
		}
		remember(stepID, item.OccurredAt)
	}
	for _, checkpoint := range snapshot.RuntimeCheckpoints {
		if !isStepRuntimeCheckpoint(checkpoint) {
			continue
		}
		remember(checkpoint.CurrentStepID, checkpoint.OccurredAt)
	}
	return ret
}

func isStepRuntimeCheckpoint(checkpoint RuntimeCheckpoint) bool {
	if checkpoint.CurrentStepID == "" || len(checkpoint.Data) == 0 {
		return false
	}
	if readModelStringValue(checkpoint.Data["stepState"]) != "" {
		return true
	}
	eventName := readModelStringValue(checkpoint.Data["event"])
	return eventName == "step_dispatched" || eventName == "step_completed" || eventName == "step_failed"
}

func timelineStepIDFromEventRecord(item EventRecord) (string, bool) {
	if item.Topic != events.TopicStepDispatched && item.Topic != events.TopicStepStateChanged && item.SubjectType != "step" {
		return "", false
	}
	stepID := item.StepID
	if stepID == "" && item.SubjectType == "step" {
		stepID = item.SubjectID
	}
	if stepID == "" {
		return "", false
	}
	return stepID, true
}

func timelineStepIDFromObservedEvent(item presenter.ObservedView) (string, bool) {
	if item.Topic != events.TopicStepDispatched && item.Topic != events.TopicStepStateChanged && item.SubjectType != "step" {
		return "", false
	}
	stepID := item.StepID
	if stepID == "" && item.SubjectType == "step" {
		stepID = item.SubjectID
	}
	if stepID == "" {
		return "", false
	}
	return stepID, true
}

func streamTimelineItems(step presenter.StepView, runState string, stepOccurredAt string) []RuntimeTimelineItem {
	if len(step.Output) == 0 {
		return nil
	}
	raw, ok := step.Output["streamEvents"]
	if !ok {
		return streamDiagnosticsTimelineItems(step, runState, stepOccurredAt)
	}
	events := readModelMapSliceValue(raw)
	if len(events) == 0 {
		return streamDiagnosticsTimelineItems(step, runState, stepOccurredAt)
	}
	ret := make([]RuntimeTimelineItem, 0, streamTimelineItemCapacity(len(events)))
	omittedInserted := false
	for idx, event := range events {
		if streamTimelineEventOmitted(idx, len(events)) {
			if !omittedInserted {
				omitted := len(events) - streamTimelineHeadItemKeep - streamTimelineTailItemKeep
				ret = append(ret, RuntimeTimelineItem{
					Source:    "step_stream",
					Type:      "omitted",
					ID:        step.ID + "/stream/omitted",
					RunState:  runState,
					StepID:    step.ID,
					StepState: step.State,
					Summary:   "omitted " + strconv.Itoa(omitted) + " stream chunks",
					Data: map[string]any{
						"omitted": omitted,
						"total":   len(events),
					},
				})
				omittedInserted = true
			}
			continue
		}
		eventType, _ := event["type"].(string)
		if eventType == "" {
			eventType = "delta"
		}
		occurredAt := streamEventOccurredAt(event)
		if occurredAt == "" {
			occurredAt = stepOccurredAt
		}
		ret = append(ret, RuntimeTimelineItem{
			Source:     "step_stream",
			Type:       eventType,
			ID:         step.ID + "/stream/" + strconv.Itoa(idx+1),
			RunState:   runState,
			StepID:     step.ID,
			StepState:  step.State,
			Summary:    readModelStringValue(event["delta"]),
			OccurredAt: occurredAt,
			Data:       cloneReadModelMap(event),
		})
	}
	return ret
}

func streamDiagnosticsTimelineItems(step presenter.StepView, runState string, stepOccurredAt string) []RuntimeTimelineItem {
	diagnostics := readModelMapValue(step.Output["streamDiagnostics"])
	if len(diagnostics) == 0 {
		return nil
	}
	summary := "stream diagnostics"
	if receivedBytes := readModelIntValue(diagnostics["receivedBytes"]); receivedBytes > 0 {
		summary = "received " + strconv.Itoa(receivedBytes) + " stream bytes"
	}
	return []RuntimeTimelineItem{{
		Source:     "step_stream_summary",
		Type:       "summary",
		ID:         step.ID + "/stream/summary",
		RunState:   runState,
		StepID:     step.ID,
		StepState:  step.State,
		Summary:    summary,
		OccurredAt: stepOccurredAt,
		Data:       cloneReadModelMap(diagnostics),
	}}
}

func streamTimelineItemCapacity(total int) int {
	if total <= maxRawStreamTimelineItems {
		return total
	}
	return streamTimelineHeadItemKeep + streamTimelineTailItemKeep + 1
}

func streamTimelineEventOmitted(idx int, total int) bool {
	if total <= maxRawStreamTimelineItems {
		return false
	}
	return idx >= streamTimelineHeadItemKeep && idx < total-streamTimelineTailItemKeep
}

func streamEventOccurredAt(event map[string]any) string {
	for _, key := range []string{"occurredAt", "timestamp", "time"} {
		if value := readModelStringValue(event[key]); value != "" {
			return value
		}
	}
	return ""
}

func streamProgressTimelineItemFromPersistedEvent(item EventRecord, runState string) (RuntimeTimelineItem, bool) {
	if item.Topic != events.TopicRunLLMStreamProgress {
		return RuntimeTimelineItem{}, false
	}
	payload := cloneReadModelMap(item.Payload)
	stepID := firstNonEmptyReadModel(item.StepID, readModelStringValue(payload[events.PayloadKeyStepID]))
	itemType := firstNonEmptyReadModel(readModelStringValue(payload[events.PayloadKeyStatus]), "progress")
	summary := readModelStringValue(payload[events.PayloadKeySummary])
	if summary == "" {
		summary = streamProgressSummaryFromPayload(payload)
	}
	return RuntimeTimelineItem{
		Source:     "llm_stream_progress_event",
		Type:       itemType,
		ID:         firstNonEmptyReadModel(item.ID, stepID+"/llm-stream-progress"),
		Topic:      item.Topic,
		RunState:   firstNonEmptyReadModel(readModelStringValue(payload[events.PayloadKeyStatus]), runState),
		StepID:     stepID,
		StepState:  itemType,
		Summary:    summary,
		OccurredAt: item.OccurredAt,
		Data:       payload,
	}, true
}

func streamProgressTimelineItemFromObserved(item presenter.ObservedView, runState string) (RuntimeTimelineItem, bool) {
	if item.Topic != events.TopicRunLLMStreamProgress {
		return RuntimeTimelineItem{}, false
	}
	payload := cloneReadModelMap(item.Payload)
	stepID := firstNonEmptyReadModel(item.StepID, readModelStringValue(payload[events.PayloadKeyStepID]))
	itemType := firstNonEmptyReadModel(readModelStringValue(payload[events.PayloadKeyStatus]), "progress")
	summary := readModelStringValue(payload[events.PayloadKeySummary])
	if summary == "" {
		summary = streamProgressSummaryFromPayload(payload)
	}
	return RuntimeTimelineItem{
		Source:     "llm_stream_progress_event",
		Type:       itemType,
		ID:         firstNonEmptyReadModel(item.SubjectID, stepID+"/llm-stream-progress"),
		Topic:      item.Topic,
		RunState:   firstNonEmptyReadModel(readModelStringValue(payload[events.PayloadKeyStatus]), runState),
		StepID:     stepID,
		StepState:  itemType,
		Summary:    summary,
		OccurredAt: item.OccurredAt,
		Data:       payload,
	}, true
}

func streamProgressSummaryFromPayload(payload map[string]any) string {
	chunkCount := readModelIntValue(payload[events.PayloadKeyChunkCount])
	receivedBytes := readModelIntValue(payload[events.PayloadKeyReceivedBytes])
	switch {
	case chunkCount > 0 && receivedBytes > 0:
		return "received " + strconv.Itoa(chunkCount) + " chunks (" + strconv.Itoa(receivedBytes) + " bytes)"
	case chunkCount > 0:
		return "received " + strconv.Itoa(chunkCount) + " chunks"
	case receivedBytes > 0:
		return "received " + strconv.Itoa(receivedBytes) + " bytes"
	default:
		return ""
	}
}

func timelineStepSummary(output map[string]any) string {
	if len(output) == 0 {
		return ""
	}
	if stage := readModelStringValue(output["stage"]); stage != "" {
		return stage
	}
	if status := readModelStringValue(output["status"]); status != "" {
		return status
	}
	return ""
}

func memoryRecallTimelineSummary(trace MemoryRecallTrace) string {
	count := len(trace.RetrievedMemoryIDs)
	if trace.Status == "degraded" && trace.Reason != "" {
		return "memory recall degraded: " + trace.Reason
	}
	if count == 0 {
		return "memory recall " + trace.Status
	}
	return "memory recall " + trace.Status + ": " + strconv.Itoa(count) + " memories"
}

func timelineEvents(items []RuntimeTimelineItem) []RuntimeTimelineItem {
	ret := make([]RuntimeTimelineItem, 0, len(items))
	for _, item := range items {
		if item.Source == "event" || item.Source == "persisted_event" {
			ret = append(ret, item)
		}
	}
	return ret
}
