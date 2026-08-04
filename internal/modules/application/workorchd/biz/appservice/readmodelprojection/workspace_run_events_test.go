package readmodelprojection

import (
	"testing"

	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func mustWorkspaceRunEventPayload(t *testing.T, input map[string]any) workorcheventpayload.Payload {
	t.Helper()
	payload, err := workorcheventpayload.FromMap(input)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestSelectWorkspaceProjectionRunUsesLatestTerminalRunWhenWorkspaceHasNoActiveRun(t *testing.T) {
	workspaceID := common.WorkspaceID("ws-terminal-successor")
	oldRunID := common.RunID("run-001")
	latestRunID := common.RunID("run-002")
	workspace := &workspacemodel.Workspace{
		ID:          workspaceID,
		State:       common.WorkspaceStateActive,
		ActiveRunID: "",
	}
	runs := []*runmodel.Run{
		{ID: oldRunID, WorkspaceID: workspaceID, State: common.RunStateSuperseded},
		{ID: latestRunID, WorkspaceID: workspaceID, State: common.RunStateFailed},
	}
	eventRecords := []*eventstreammodel.EventRecord{{
		ID:          "evt-latest-failed",
		Topic:       runevents.TopicRunStateChanged,
		OccurredAt:  "2026-07-11T07:36:31Z",
		WorkspaceID: workspaceID,
		RunID:       latestRunID,
		Payload:     mustWorkspaceRunEventPayload(t, map[string]any{recordpayload.PayloadKeyState: string(common.RunStateFailed)}),
	}}

	selected := SelectWorkspaceProjectionRun(workspace, runs, nil, eventRecords)
	if selected == nil || selected.ID != latestRunID {
		t.Fatalf("selected run = %+v, want latest failed successor %s", selected, latestRunID)
	}
}

func TestSelectLatestRunEventPrefersReplacementRunOverNewerSupersededMarker(t *testing.T) {
	workspaceID := common.WorkspaceID("ws-replan")
	replacementRunID := common.RunID("run-001-replan-001")
	supersededRunID := common.RunID("run-001")
	runs := map[common.RunID]*runmodel.Run{
		supersededRunID: {
			ID:          supersededRunID,
			WorkspaceID: workspaceID,
			State:       common.RunStateSuperseded,
		},
		replacementRunID: {
			ID:          replacementRunID,
			WorkspaceID: workspaceID,
			State:       common.RunStateFailed,
		},
	}
	records := []*eventstreammodel.EventRecord{
		{
			ID:          "evt-replacement-failed",
			Topic:       runevents.TopicRunStateChanged,
			OccurredAt:  "2026-06-09T14:50:05Z",
			WorkspaceID: workspaceID,
			RunID:       replacementRunID,
			Payload:     mustWorkspaceRunEventPayload(t, map[string]any{recordpayload.PayloadKeyState: string(common.RunStateFailed)}),
		},
		{
			ID:          "evt-old-superseded",
			Topic:       runevents.TopicRunStateChanged,
			OccurredAt:  "2026-06-09T14:50:06Z",
			WorkspaceID: workspaceID,
			RunID:       supersededRunID,
			Payload:     mustWorkspaceRunEventPayload(t, map[string]any{recordpayload.PayloadKeyState: string(common.RunStateFailed)}),
		},
	}

	selected := SelectLatestRunEvent(workspaceID, runs, records)
	if selected == nil || selected.RunID != replacementRunID {
		t.Fatalf("selected event = %+v, want replacement failed run", selected)
	}
}

func TestSelectLatestRunEventReturnsNilWhenOnlySupersededRunEventsAreAvailableAndReplacementExists(t *testing.T) {
	workspaceID := common.WorkspaceID("ws-replan-stale-events")
	replacementRunID := common.RunID("run-001-replan-001")
	supersededRunID := common.RunID("run-001")
	runs := map[common.RunID]*runmodel.Run{
		supersededRunID: {
			ID:          supersededRunID,
			WorkspaceID: workspaceID,
			State:       common.RunStateSuperseded,
		},
		replacementRunID: {
			ID:          replacementRunID,
			WorkspaceID: workspaceID,
			State:       common.RunStateFailed,
		},
	}
	records := []*eventstreammodel.EventRecord{
		{
			ID:          "evt-old-failed-before-superseded",
			Topic:       runevents.TopicRunStateChanged,
			OccurredAt:  "2026-06-09T14:50:06Z",
			WorkspaceID: workspaceID,
			RunID:       supersededRunID,
			Payload:     mustWorkspaceRunEventPayload(t, map[string]any{recordpayload.PayloadKeyState: string(common.RunStateFailed)}),
		},
	}

	if selected := SelectLatestRunEvent(workspaceID, runs, records); selected != nil {
		t.Fatalf("selected event = %+v, want nil so caller can fall back to replacement run", selected)
	}
}
