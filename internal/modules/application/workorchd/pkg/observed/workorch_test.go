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

func TestBuildWorkorchSnapshotIncludesExpectedPublishedTopics(t *testing.T) {
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
		Kind:  "llm/workspace_execution",
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

	items := BuildWorkorchSnapshot(ws, run, []*runmodel.Step{step}, ticket, nil, approval, []events.WorkorchEvent{
		mustWorkorchEvent(t, events.Envelope{
			Version:      events.WorkorchEventVersion,
			Topic:        events.TopicWorkspaceStateChanged,
			OccurredAt:   "2026-04-21T08:00:00Z",
			SourceModule: "kernel/orchestrator",
			SubjectType:  "workspace",
			SubjectID:    string(ws.ID),
			WorkspaceID:  string(ws.ID),
			RunID:        string(run.ID),
			Payload: map[string]any{
				events.PayloadKeyState:       string(ws.State),
				events.PayloadKeyActiveRunID: string(ws.ActiveRunID),
			},
		}),
		mustWorkorchEvent(t, events.Envelope{
			Version:      events.WorkorchEventVersion,
			Topic:        events.TopicRunStateChanged,
			OccurredAt:   "2026-04-21T08:00:01Z",
			SourceModule: "kernel/orchestrator",
			SubjectType:  "run",
			SubjectID:    string(run.ID),
			WorkspaceID:  string(ws.ID),
			RunID:        string(run.ID),
			Payload: map[string]any{
				events.PayloadKeyState: string(run.State),
			},
		}),
		mustWorkorchEvent(t, events.Envelope{
			Version:      events.WorkorchEventVersion,
			Topic:        events.TopicStepStateChanged,
			OccurredAt:   "2026-04-21T08:00:02Z",
			SourceModule: "kernel/orchestrator",
			SubjectType:  "step",
			SubjectID:    string(step.ID),
			WorkspaceID:  string(ws.ID),
			RunID:        string(run.ID),
			StepID:       string(step.ID),
			Payload: map[string]any{
				events.PayloadKeyKind:  step.Kind,
				events.PayloadKeyState: string(step.State),
			},
		}),
		mustWorkorchEvent(t, events.Envelope{
			Version:      events.WorkorchEventVersion,
			Topic:        events.TopicTicketCreated,
			OccurredAt:   "2026-04-21T08:00:03Z",
			SourceModule: "kernel/decision",
			SubjectType:  "decision_ticket",
			SubjectID:    string(ticket.ID),
			WorkspaceID:  string(ws.ID),
			RunID:        string(run.ID),
			StepID:       string(ticket.StepID),
			Payload: map[string]any{
				events.PayloadKeyReason: ticket.Reason,
				events.PayloadKeyState:  string(ticket.State),
			},
		}),
		mustWorkorchEvent(t, events.Envelope{
			Version:      events.WorkorchEventVersion,
			Topic:        events.TopicApprovalResolved,
			OccurredAt:   "2026-04-21T08:00:04Z",
			SourceModule: "blocks/approval",
			SubjectType:  "approval",
			SubjectID:    string(approval.ID),
			WorkspaceID:  string(ws.ID),
			RunID:        string(run.ID),
			StepID:       string(approval.StepID),
			Payload: map[string]any{
				events.PayloadKeyReason: approval.Reason,
				events.PayloadKeyState:  string(approval.State),
			},
		}),
	})
	if len(items) != 5 {
		t.Fatalf("len(items) = %d, want 5", len(items))
	}

	assertTopic(t, items, "/workspace/snapshot")
	assertTopic(t, items, "/run/snapshot")
	assertTopic(t, items, "/step/snapshot")
	assertTopic(t, items, "/decision/ticket/snapshot")
	assertTopic(t, items, "/approval/snapshot")
}

func TestBuildWorkorchSnapshotRequiresPublishedEvents(t *testing.T) {
	ws := &workspacemodel.Workspace{
		ID:    common.WorkspaceID("ws-observed-min"),
		State: common.WorkspaceStateActive,
	}
	run := &runmodel.Run{
		ID:          common.RunID("run-observed-min"),
		WorkspaceID: ws.ID,
		State:       common.RunStateFailed,
	}

	items := BuildWorkorchSnapshot(ws, run, nil, nil, nil, nil, nil)
	if len(items) != 0 {
		t.Fatalf("len(items) = %d, want 0", len(items))
	}

	assertTopicAbsent(t, items, "/workspace/snapshot")
	assertTopicAbsent(t, items, "/run/snapshot")
	assertTopicAbsent(t, items, "/step/snapshot")
	assertTopicAbsent(t, items, "/decision/ticket/snapshot")
	assertTopicAbsent(t, items, "/approval/snapshot")
}

func TestBuildWorkorchSnapshotPublishedTicketIncludesResolvedTicketResolution(t *testing.T) {
	ws := &workspacemodel.Workspace{
		ID:    common.WorkspaceID("ws-observed-resolution"),
		State: common.WorkspaceStateActive,
	}
	run := &runmodel.Run{
		ID:          common.RunID("run-observed-resolution"),
		WorkspaceID: ws.ID,
		State:       common.RunStateCompleted,
	}
	ticket := &decisionmodel.Ticket{
		ID:          common.TicketID("ticket-observed-resolution"),
		WorkspaceID: ws.ID,
		RunID:       run.ID,
		StepID:      common.StepID("run-observed-resolution/manual-gate"),
		Reason:      "decision gate",
		State:       common.DecisionTicketStateResolved,
	}
	resolution := &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-observed"),
		TicketID: ticket.ID,
		Type:     "provide_missing_info",
		Payload:  map[string]any{"operator": "workorch"},
	}

	items := BuildWorkorchSnapshot(ws, run, nil, ticket, resolution, nil, []events.WorkorchEvent{
		mustWorkorchEvent(t, events.Envelope{
			Version:      events.WorkorchEventVersion,
			Topic:        events.TopicTicketResolved,
			OccurredAt:   "2026-04-21T08:05:00Z",
			SourceModule: "kernel/decision",
			SubjectType:  "decision_ticket",
			SubjectID:    string(ticket.ID),
			WorkspaceID:  string(ws.ID),
			RunID:        string(run.ID),
			StepID:       string(ticket.StepID),
			Payload: map[string]any{
				events.PayloadKeyReason:       ticket.Reason,
				events.PayloadKeyState:        string(ticket.State),
				events.PayloadKeyResolutionID: string(resolution.ID),
				events.PayloadKeyType:         resolution.Type,
				events.PayloadKeyPayload:      resolution.Payload,
			},
		}),
	})

	ticketItem := topicItem(t, items, "/decision/ticket/snapshot")
	ticketPayload, ok := ticketItem.Payload.(DecisionTicketPayload)
	if !ok {
		t.Fatalf("ticket payload type = %T, want DecisionTicketPayload", ticketItem.Payload)
	}
	if ticketPayload.ResolutionID != string(resolution.ID) {
		t.Fatalf("ticket resolutionId = %s, want %s", ticketPayload.ResolutionID, resolution.ID)
	}
	if ticketPayload.ResolutionType != resolution.Type {
		t.Fatalf("ticket resolutionType = %s, want %s", ticketPayload.ResolutionType, resolution.Type)
	}
	if ticketPayload.ResolutionPayload["operator"] != "workorch" {
		t.Fatalf("ticket resolution payload operator = %v, want workorch", ticketPayload.ResolutionPayload["operator"])
	}
}

func TestBuildWorkorchSnapshotPrefersPublishedRunAndStepEvents(t *testing.T) {
	ws := &workspacemodel.Workspace{
		ID:    common.WorkspaceID("ws-published"),
		State: common.WorkspaceStateActive,
	}
	run := &runmodel.Run{
		ID:                 common.RunID("run-published"),
		WorkspaceID:        ws.ID,
		GoalID:             common.GoalID("goal-published"),
		DoDID:              common.DoDID("dod-published"),
		State:              common.RunStateCompleted,
		BlockingTicketID:   common.TicketID("ticket-stale"),
		BlockingApprovalID: common.ApprovalID("approval-stale"),
	}
	step := &runmodel.Step{
		ID:    common.StepID("run-published/step-1"),
		RunID: run.ID,
		Kind:  "llm/workspace_execution",
		State: common.StepStateCompleted,
	}

	items := BuildWorkorchSnapshot(ws, run, []*runmodel.Step{step}, nil, nil, nil, []events.WorkorchEvent{
		mustWorkorchEvent(t, events.Envelope{
			Version:      events.WorkorchEventVersion,
			Topic:        events.TopicRunStateChanged,
			OccurredAt:   "2026-04-21T09:00:00Z",
			SourceModule: "kernel/orchestrator",
			SubjectType:  "run",
			SubjectID:    string(run.ID),
			WorkspaceID:  string(ws.ID),
			RunID:        string(run.ID),
			Payload: map[string]any{
				events.PayloadKeyGoalID: string(run.GoalID),
				events.PayloadKeyDoDID:  string(run.DoDID),
				"state":                 string(common.RunStateCompleted),
				"blockingTicketId":      "",
				"blockingApprovalId":    "",
			},
		}),
		mustWorkorchEvent(t, events.Envelope{
			Version:      events.WorkorchEventVersion,
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
	if runPayload.GoalID != "goal-published" || runPayload.DoDID != "dod-published" {
		t.Fatalf("run payload goal/dod = %+v, want goal-published/dod-published", runPayload)
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

func TestBuildWorkorchSnapshotPrefersPublishedRunCreatedAndFailureReason(t *testing.T) {
	ws := &workspacemodel.Workspace{
		ID:    common.WorkspaceID("ws-published-created"),
		State: common.WorkspaceStateActive,
	}
	run := &runmodel.Run{
		ID:            common.RunID("run-published-created"),
		WorkspaceID:   ws.ID,
		GoalID:        common.GoalID("goal-created"),
		DoDID:         common.DoDID("dod-created"),
		State:         common.RunStateFailed,
		FailureReason: "manual fail",
	}

	items := BuildWorkorchSnapshot(ws, run, nil, nil, nil, nil, []events.WorkorchEvent{
		mustWorkorchEvent(t, events.Envelope{
			Version:      events.WorkorchEventVersion,
			Topic:        events.TopicRunCreated,
			OccurredAt:   "2026-04-21T09:05:00Z",
			SourceModule: "kernel/run",
			SubjectType:  "run",
			SubjectID:    string(run.ID),
			WorkspaceID:  string(ws.ID),
			RunID:        string(run.ID),
			Payload: map[string]any{
				events.PayloadKeyGoalID:             string(run.GoalID),
				events.PayloadKeyDoDID:              string(run.DoDID),
				events.PayloadKeyState:              string(common.RunStateCreated),
				events.PayloadKeyBlockingTicketID:   "",
				events.PayloadKeyBlockingApprovalID: "",
			},
		}),
		mustWorkorchEvent(t, events.Envelope{
			Version:      events.WorkorchEventVersion,
			Topic:        events.TopicRunStateChanged,
			OccurredAt:   "2026-04-21T09:05:01Z",
			SourceModule: "kernel/orchestrator",
			SubjectType:  "run",
			SubjectID:    string(run.ID),
			WorkspaceID:  string(ws.ID),
			RunID:        string(run.ID),
			Payload: map[string]any{
				events.PayloadKeyGoalID:             string(run.GoalID),
				events.PayloadKeyDoDID:              string(run.DoDID),
				events.PayloadKeyState:              string(common.RunStateFailed),
				events.PayloadKeyBlockingTicketID:   "",
				events.PayloadKeyBlockingApprovalID: "",
				events.PayloadKeyFailureReason:      run.FailureReason,
			},
		}),
	})

	runItem := topicItem(t, items, "/run/snapshot")
	runPayload, ok := runItem.Payload.(RunPayload)
	if !ok {
		t.Fatalf("run payload type = %T, want RunPayload", runItem.Payload)
	}
	if runPayload.GoalID != "goal-created" || runPayload.DoDID != "dod-created" {
		t.Fatalf("run payload goal/dod = %+v, want goal-created/dod-created", runPayload)
	}
	if runPayload.FailureReason != "manual fail" || runPayload.State != string(common.RunStateFailed) {
		t.Fatalf("run payload = %+v, want failed/manual fail", runPayload)
	}
	if runItem.OccurredAt != "2026-04-21T09:05:01Z" {
		t.Fatalf("run item occurredAt = %s, want latest run event timestamp", runItem.OccurredAt)
	}
}

func TestBuildWorkorchSnapshotPrefersPublishedWorkspaceTicketAndApprovalEvents(t *testing.T) {
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

	items := BuildWorkorchSnapshot(ws, run, nil, ticket, nil, approval, []events.WorkorchEvent{
		mustWorkorchEvent(t, events.Envelope{
			Version:      events.WorkorchEventVersion,
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
		mustWorkorchEvent(t, events.Envelope{
			Version:      events.WorkorchEventVersion,
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
				events.PayloadKeyPayload:      map[string]any{"operator": "workorch"},
			},
		}),
		mustWorkorchEvent(t, events.Envelope{
			Version:      events.WorkorchEventVersion,
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

func TestBuildWorkorchSnapshotPrefersLatestWorkspaceStateChangedEvent(t *testing.T) {
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

	items := BuildWorkorchSnapshot(ws, run, nil, nil, nil, nil, []events.WorkorchEvent{
		mustWorkorchEvent(t, events.Envelope{
			Version:      events.WorkorchEventVersion,
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
		mustWorkorchEvent(t, events.Envelope{
			Version:      events.WorkorchEventVersion,
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

func TestBuildWorkorchSnapshotIgnoresPublishedEventsThatDoNotMatchCurrentObjects(t *testing.T) {
	ws := &workspacemodel.Workspace{
		ID:          common.WorkspaceID("ws-unmatched-published"),
		State:       common.WorkspaceStateActive,
		ActiveRunID: common.RunID("run-unmatched-published"),
	}
	run := &runmodel.Run{
		ID:                 common.RunID("run-unmatched-published"),
		WorkspaceID:        ws.ID,
		State:              common.RunStateCompleted,
		BlockingTicketID:   common.TicketID("ticket-unmatched"),
		BlockingApprovalID: common.ApprovalID("approval-unmatched"),
	}
	step := &runmodel.Step{
		ID:     common.StepID("run-unmatched-published/step-1"),
		RunID:  run.ID,
		Kind:   "llm/workspace_execution",
		State:  common.StepStateCompleted,
		Output: map[string]any{"status": "unmatched"},
	}

	items := BuildWorkorchSnapshot(ws, run, []*runmodel.Step{step}, nil, nil, nil, []events.WorkorchEvent{
		mustWorkorchEvent(t, events.Envelope{
			Version:      events.WorkorchEventVersion,
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
		mustWorkorchEvent(t, events.Envelope{
			Version:      events.WorkorchEventVersion,
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

	if len(items) != 0 {
		t.Fatalf("len(items) = %d, want 0", len(items))
	}
	assertTopicAbsent(t, items, "/run/snapshot")
	assertTopicAbsent(t, items, "/step/snapshot")
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

func mustWorkorchEvent(t *testing.T, envelope events.Envelope) events.WorkorchEvent {
	t.Helper()

	event, ok := events.DecodeWorkorchEvent(envelope)
	if !ok {
		t.Fatalf("DecodeWorkorchEvent returned !ok for topic %s", envelope.Topic)
	}
	return event
}
