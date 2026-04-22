package observed

import (
	"testing"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestBuildPhase1SnapshotIncludesExpectedTopics(t *testing.T) {
	ws := &workspacemodel.Workspace{
		ID:          common.WorkspaceID("ws-observed"),
		State:       common.WorkspaceStateActive,
		ActiveRunID: common.RunID("run-observed"),
	}
	run := &runmodel.Run{
		ID:          common.RunID("run-observed"),
		WorkspaceID: ws.ID,
		State:       common.RunStateCompleted,
	}
	step := &runmodel.Step{
		ID:    common.StepID("run-observed/step-1"),
		RunID: run.ID,
		Kind:  "legacy/minimal",
		State: common.StepStateCompleted,
	}
	ticket := &decisionmodel.Ticket{
		ID:          common.TicketID("ticket-observed"),
		WorkspaceID: ws.ID,
		RunID:       run.ID,
		StepID:      common.StepID("run-observed/manual-gate"),
		Reason:      "decision gate",
		State:       common.DecisionTicketStateResolved,
	}
	approval := &approvalmodel.Approval{
		ID:          common.ApprovalID("approval-observed"),
		WorkspaceID: ws.ID,
		RunID:       run.ID,
		StepID:      common.StepID("run-observed/manual-approval"),
		Reason:      "approval gate",
		State:       common.ApprovalStateApproved,
	}

	items := BuildPhase1Snapshot(ws, run, step, ticket, approval, nil)
	if len(items) != 5 {
		t.Fatalf("len(items) = %d, want 5", len(items))
	}

	assertTopic(t, items, "/workspace/snapshot")
	assertTopic(t, items, "/run/snapshot")
	assertTopic(t, items, "/step/snapshot")
	assertTopic(t, items, "/decision/ticket/snapshot")
	assertTopic(t, items, "/approval/snapshot")
}

func TestBuildPhase1SnapshotOmitsOptionalTopicsWhenObjectsMissing(t *testing.T) {
	ws := &workspacemodel.Workspace{
		ID:    common.WorkspaceID("ws-observed-min"),
		State: common.WorkspaceStateActive,
	}
	run := &runmodel.Run{
		ID:          common.RunID("run-observed-min"),
		WorkspaceID: ws.ID,
		State:       common.RunStateFailed,
	}

	items := BuildPhase1Snapshot(ws, run, nil, nil, nil, nil)
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}

	assertTopic(t, items, "/workspace/snapshot")
	assertTopic(t, items, "/run/snapshot")
	assertTopicAbsent(t, items, "/step/snapshot")
	assertTopicAbsent(t, items, "/decision/ticket/snapshot")
	assertTopicAbsent(t, items, "/approval/snapshot")
}

func TestBuildPhase1SnapshotPrefersPublishedRunAndStepEvents(t *testing.T) {
	ws := &workspacemodel.Workspace{
		ID:    common.WorkspaceID("ws-published"),
		State: common.WorkspaceStateActive,
	}
	run := &runmodel.Run{
		ID:                 common.RunID("run-published"),
		WorkspaceID:        ws.ID,
		State:              common.RunStateCompleted,
		BlockingTicketID:   common.TicketID("ticket-stale"),
		BlockingApprovalID: common.ApprovalID("approval-stale"),
	}
	step := &runmodel.Step{
		ID:    common.StepID("run-published/step-1"),
		RunID: run.ID,
		Kind:  "legacy/minimal",
		State: common.StepStateCompleted,
	}

	items := BuildPhase1Snapshot(ws, run, step, nil, nil, []events.Phase1Event{
		mustPhase1Event(t, events.Envelope{
			Version:      events.Phase1EventVersion,
			Topic:        events.TopicRunStateChanged,
			OccurredAt:   "2026-04-21T09:00:00Z",
			SourceModule: "kernel/orchestrator",
			SubjectType:  "run",
			SubjectID:    string(run.ID),
			WorkspaceID:  string(ws.ID),
			RunID:        string(run.ID),
			Payload: map[string]any{
				"state":              string(common.RunStateCompleted),
				"blockingTicketId":   "",
				"blockingApprovalId": "",
			},
		}),
		mustPhase1Event(t, events.Envelope{
			Version:      events.Phase1EventVersion,
			Topic:        events.TopicStepStateChanged,
			OccurredAt:   "2026-04-21T09:00:01Z",
			SourceModule: "kernel/orchestrator",
			SubjectType:  "step",
			SubjectID:    string(step.ID),
			WorkspaceID:  string(ws.ID),
			RunID:        string(run.ID),
			StepID:       string(step.ID),
			Payload: map[string]any{
				"kind":   step.Kind,
				"state":  string(common.StepStateCompleted),
				"output": map[string]any{"status": "completed"},
			},
		}),
	})

	runItem := topicItem(t, items, "/run/snapshot")
	runPayload, ok := runItem.Payload.(RunPayload)
	if !ok {
		t.Fatalf("run payload type = %T, want RunPayload", runItem.Payload)
	}
	if runPayload.BlockingTicketID != "" || runPayload.BlockingApprovalID != "" {
		t.Fatalf("run payload should prefer published payload, got %+v", runPayload)
	}
	if runItem.OccurredAt != "2026-04-21T09:00:00Z" {
		t.Fatalf("run item occurredAt = %s, want published timestamp", runItem.OccurredAt)
	}

	stepItem := topicItem(t, items, "/step/snapshot")
	stepPayload, ok := stepItem.Payload.(StepPayload)
	if !ok {
		t.Fatalf("step payload type = %T, want StepPayload", stepItem.Payload)
	}
	if stepPayload.State != string(common.StepStateCompleted) {
		t.Fatalf("step payload state = %v, want completed", stepPayload.State)
	}
	if stepItem.OccurredAt != "2026-04-21T09:00:01Z" {
		t.Fatalf("step item occurredAt = %s, want published timestamp", stepItem.OccurredAt)
	}
}

func TestBuildPhase1SnapshotPrefersPublishedWorkspaceTicketAndApprovalEvents(t *testing.T) {
	ws := &workspacemodel.Workspace{
		ID:          common.WorkspaceID("ws-published-aux"),
		State:       common.WorkspaceStateActive,
		ActiveRunID: common.RunID("run-stale"),
	}
	run := &runmodel.Run{
		ID:          common.RunID("run-published-aux"),
		WorkspaceID: ws.ID,
		State:       common.RunStateCompleted,
	}
	ticket := &decisionmodel.Ticket{
		ID:          common.TicketID("ticket-published-aux"),
		WorkspaceID: ws.ID,
		RunID:       run.ID,
		StepID:      common.StepID("run-published-aux/manual-gate"),
		Reason:      "stale ticket reason",
		State:       common.DecisionTicketStateOpen,
	}
	approval := &approvalmodel.Approval{
		ID:          common.ApprovalID("approval-published-aux"),
		WorkspaceID: ws.ID,
		RunID:       run.ID,
		StepID:      common.StepID("run-published-aux/manual-approval"),
		Reason:      "stale approval reason",
		State:       common.ApprovalStatePending,
	}

	items := BuildPhase1Snapshot(ws, run, nil, ticket, approval, []events.Phase1Event{
		mustPhase1Event(t, events.Envelope{
			Version:      events.Phase1EventVersion,
			Topic:        events.TopicWorkspaceCreated,
			OccurredAt:   "2026-04-21T09:10:00Z",
			SourceModule: "kernel/workspace",
			SubjectType:  "workspace",
			SubjectID:    string(ws.ID),
			WorkspaceID:  string(ws.ID),
			Payload: map[string]any{
				"state":       string(common.WorkspaceStateActive),
				"activeRunId": "",
			},
		}),
		mustPhase1Event(t, events.Envelope{
			Version:      events.Phase1EventVersion,
			Topic:        events.TopicTicketResolved,
			OccurredAt:   "2026-04-21T09:10:01Z",
			SourceModule: "kernel/decision",
			SubjectType:  "decision_ticket",
			SubjectID:    string(ticket.ID),
			WorkspaceID:  string(ws.ID),
			RunID:        string(run.ID),
			StepID:       string(ticket.StepID),
			Payload: map[string]any{
				events.PayloadKeyReason:       ticket.Reason,
				events.PayloadKeyState:        string(common.DecisionTicketStateResolved),
				events.PayloadKeyResolutionID: "resolution-published-aux",
				events.PayloadKeyType:         "approve-plan",
				events.PayloadKeyPayload:      map[string]any{"operator": "phase1-demo"},
			},
		}),
		mustPhase1Event(t, events.Envelope{
			Version:      events.Phase1EventVersion,
			Topic:        events.TopicApprovalResolved,
			OccurredAt:   "2026-04-21T09:10:02Z",
			SourceModule: "blocks/approval",
			SubjectType:  "approval",
			SubjectID:    string(approval.ID),
			WorkspaceID:  string(ws.ID),
			RunID:        string(run.ID),
			StepID:       string(approval.StepID),
			Payload: map[string]any{
				"reason": approval.Reason,
				"state":  string(common.ApprovalStateApproved),
			},
		}),
	})

	workspaceItem := topicItem(t, items, "/workspace/snapshot")
	workspacePayload, ok := workspaceItem.Payload.(WorkspacePayload)
	if !ok {
		t.Fatalf("workspace payload type = %T, want WorkspacePayload", workspaceItem.Payload)
	}
	if workspaceItem.OccurredAt != "2026-04-21T09:10:00Z" {
		t.Fatalf("workspace item occurredAt = %s, want published timestamp", workspaceItem.OccurredAt)
	}
	if workspacePayload.ActiveRunID != "" {
		t.Fatalf("workspace activeRunId = %v, want empty from published payload", workspacePayload.ActiveRunID)
	}

	ticketItem := topicItem(t, items, "/decision/ticket/snapshot")
	ticketPayload, ok := ticketItem.Payload.(DecisionTicketPayload)
	if !ok {
		t.Fatalf("ticket payload type = %T, want DecisionTicketPayload", ticketItem.Payload)
	}
	if ticketPayload.State != string(common.DecisionTicketStateResolved) {
		t.Fatalf("ticket state = %v, want resolved", ticketPayload.State)
	}
	if ticketPayload.ResolutionID != "resolution-published-aux" {
		t.Fatalf("ticket resolutionId = %s, want resolution-published-aux", ticketPayload.ResolutionID)
	}
	if ticketItem.OccurredAt != "2026-04-21T09:10:01Z" {
		t.Fatalf("ticket occurredAt = %s, want published timestamp", ticketItem.OccurredAt)
	}

	approvalItem := topicItem(t, items, "/approval/snapshot")
	approvalPayload, ok := approvalItem.Payload.(ApprovalPayload)
	if !ok {
		t.Fatalf("approval payload type = %T, want ApprovalPayload", approvalItem.Payload)
	}
	if approvalPayload.State != string(common.ApprovalStateApproved) {
		t.Fatalf("approval state = %v, want approved", approvalPayload.State)
	}
	if approvalItem.OccurredAt != "2026-04-21T09:10:02Z" {
		t.Fatalf("approval occurredAt = %s, want published timestamp", approvalItem.OccurredAt)
	}
}

func TestBuildPhase1SnapshotPrefersLatestWorkspaceStateChangedEvent(t *testing.T) {
	ws := &workspacemodel.Workspace{
		ID:          common.WorkspaceID("ws-published-state"),
		State:       common.WorkspaceStateActive,
		ActiveRunID: common.RunID("run-stale"),
	}
	run := &runmodel.Run{
		ID:          common.RunID("run-published-state"),
		WorkspaceID: ws.ID,
		State:       common.RunStateCompleted,
	}

	items := BuildPhase1Snapshot(ws, run, nil, nil, nil, []events.Phase1Event{
		mustPhase1Event(t, events.Envelope{
			Version:      events.Phase1EventVersion,
			Topic:        events.TopicWorkspaceCreated,
			OccurredAt:   "2026-04-21T09:20:00Z",
			SourceModule: "kernel/workspace",
			SubjectType:  "workspace",
			SubjectID:    string(ws.ID),
			WorkspaceID:  string(ws.ID),
			Payload: map[string]any{
				"state":       string(common.WorkspaceStateActive),
				"activeRunId": "run-old",
			},
		}),
		mustPhase1Event(t, events.Envelope{
			Version:      events.Phase1EventVersion,
			Topic:        events.TopicWorkspaceStateChanged,
			OccurredAt:   "2026-04-21T09:20:01Z",
			SourceModule: "kernel/orchestrator",
			SubjectType:  "workspace",
			SubjectID:    string(ws.ID),
			WorkspaceID:  string(ws.ID),
			RunID:        string(run.ID),
			Payload: map[string]any{
				"state":       string(common.WorkspaceStateActive),
				"activeRunId": "",
			},
		}),
	})

	workspaceItem := topicItem(t, items, "/workspace/snapshot")
	workspacePayload, ok := workspaceItem.Payload.(WorkspacePayload)
	if !ok {
		t.Fatalf("workspace payload type = %T, want WorkspacePayload", workspaceItem.Payload)
	}
	if workspaceItem.OccurredAt != "2026-04-21T09:20:01Z" {
		t.Fatalf("workspace occurredAt = %s, want latest state-changed timestamp", workspaceItem.OccurredAt)
	}
	if workspacePayload.ActiveRunID != "" {
		t.Fatalf("workspace activeRunId = %v, want cleared by latest event", workspacePayload.ActiveRunID)
	}
	if workspaceItem.RunID != string(run.ID) {
		t.Fatalf("workspace runID = %s, want %s", workspaceItem.RunID, run.ID)
	}
}

func TestBuildPhase1SnapshotFallsBackWhenPublishedEventsDoNotMatchCurrentObjects(t *testing.T) {
	ws := &workspacemodel.Workspace{
		ID:          common.WorkspaceID("ws-fallback-published"),
		State:       common.WorkspaceStateActive,
		ActiveRunID: common.RunID("run-fallback-published"),
	}
	run := &runmodel.Run{
		ID:                 common.RunID("run-fallback-published"),
		WorkspaceID:        ws.ID,
		State:              common.RunStateCompleted,
		BlockingTicketID:   common.TicketID("ticket-fallback"),
		BlockingApprovalID: common.ApprovalID("approval-fallback"),
	}
	step := &runmodel.Step{
		ID:     common.StepID("run-fallback-published/step-1"),
		RunID:  run.ID,
		Kind:   "legacy/minimal",
		State:  common.StepStateCompleted,
		Output: map[string]any{"status": "fallback"},
	}

	items := BuildPhase1Snapshot(ws, run, step, nil, nil, []events.Phase1Event{
		mustPhase1Event(t, events.Envelope{
			Version:      events.Phase1EventVersion,
			Topic:        events.TopicRunStateChanged,
			OccurredAt:   "2026-04-21T09:30:00Z",
			SourceModule: "kernel/orchestrator",
			SubjectType:  "run",
			SubjectID:    "run-other",
			WorkspaceID:  string(ws.ID),
			RunID:        "run-other",
			Payload: map[string]any{
				events.PayloadKeyState: string(common.RunStateRunning),
			},
		}),
		mustPhase1Event(t, events.Envelope{
			Version:      events.Phase1EventVersion,
			Topic:        events.TopicStepStateChanged,
			OccurredAt:   "2026-04-21T09:30:01Z",
			SourceModule: "kernel/orchestrator",
			SubjectType:  "step",
			SubjectID:    "run-other/step-1",
			WorkspaceID:  string(ws.ID),
			RunID:        "run-other",
			StepID:       "run-other/step-1",
			Payload: map[string]any{
				events.PayloadKeyKind:   step.Kind,
				events.PayloadKeyState:  string(common.StepStateCompleted),
				events.PayloadKeyOutput: map[string]any{"status": "other"},
			},
		}),
	})

	runItem := topicItem(t, items, "/run/snapshot")
	runPayload, ok := runItem.Payload.(RunPayload)
	if !ok {
		t.Fatalf("run payload type = %T, want RunPayload", runItem.Payload)
	}
	if runItem.OccurredAt == "2026-04-21T09:30:00Z" {
		t.Fatalf("run item should fall back instead of using unrelated published event")
	}
	if runPayload.BlockingTicketID != string(run.BlockingTicketID) {
		t.Fatalf("run blockingTicketId = %v, want %s", runPayload.BlockingTicketID, run.BlockingTicketID)
	}
	if runPayload.BlockingApprovalID != string(run.BlockingApprovalID) {
		t.Fatalf("run blockingApprovalId = %v, want %s", runPayload.BlockingApprovalID, run.BlockingApprovalID)
	}

	stepItem := topicItem(t, items, "/step/snapshot")
	stepPayload, ok := stepItem.Payload.(StepPayload)
	if !ok {
		t.Fatalf("step payload type = %T, want StepPayload", stepItem.Payload)
	}
	if stepItem.OccurredAt == "2026-04-21T09:30:01Z" {
		t.Fatalf("step item should fall back instead of using unrelated published event")
	}
	if stepPayload.Output["status"] != "fallback" {
		t.Fatalf("step output status = %v, want fallback", stepPayload.Output["status"])
	}
}

func assertTopic(t *testing.T, items []SnapshotItem, topic string) {
	t.Helper()

	for _, item := range items {
		if item.Topic == topic {
			return
		}
	}
	t.Fatalf("missing topic %s", topic)
}

func assertTopicAbsent(t *testing.T, items []SnapshotItem, topic string) {
	t.Helper()

	for _, item := range items {
		if item.Topic == topic {
			t.Fatalf("unexpected topic %s", topic)
		}
	}
}

func topicItem(t *testing.T, items []SnapshotItem, topic string) SnapshotItem {
	t.Helper()

	for _, item := range items {
		if item.Topic == topic {
			return item
		}
	}
	t.Fatalf("missing topic %s", topic)
	return SnapshotItem{}
}

func mustPhase1Event(t *testing.T, envelope events.Envelope) events.Phase1Event {
	t.Helper()

	event, ok := events.DecodePhase1Event(envelope)
	if !ok {
		t.Fatalf("DecodePhase1Event returned !ok for topic %s", envelope.Topic)
	}
	return event
}
