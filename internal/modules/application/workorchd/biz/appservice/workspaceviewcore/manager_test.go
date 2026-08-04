package workspaceviewcore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"

	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"

	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"

	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"

	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"

	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"

	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"

	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

type managerEventRecordFixture struct {
	ID           string
	Version      string
	Topic        string
	OccurredAt   string
	SourceModule string
	SubjectType  string
	SubjectID    string
	WorkspaceID  string
	RunID        string
	StepID       string
	Revision     int64
	Payload      map[string]any
}

func managerTestEventPorts(hub magicevent.Hub) EventPorts {
	return EventPorts{
		ReadHighWatermarks: func(ctx context.Context, workspaceIDs []common.WorkspaceID) ([]eventstreamevents.WorkspaceHighWatermark, error) {
			event := magicevent.NewEventWithContext(eventstreamevents.TopicEventStreamCommandHighWatermarks, "workspace-view-test", eventstreamevents.CommandDestination, magicevent.NewHeader(), ctx, eventstreamevents.HighWatermarksCommand{WorkspaceIDs: workspaceIDs})
			response := hub.Send(event)
			if response == nil {
				return nil, common.ErrConflict
			}
			if err := response.Error(); err != nil {
				return nil, err
			}
			data, err := response.Get()
			if err != nil {
				return nil, err
			}
			result, ok := data.(eventstreamevents.EventStreamCommandResult)
			if !ok {
				return nil, common.ErrInvalidArgument
			}
			return result.HighWatermarks, nil
		},
		PublishChanged: func(ctx context.Context, data workorchdevents.WorkspaceProjectionChangedData) {
			hub.Post(magicevent.NewEventWithContext(workorchdevents.TopicWorkspaceProjectionChanged, "workspace-view-test", workorchdevents.NotificationDestination, magicevent.NewHeader(), ctx, data))
		},
	}
}

func publishManagerEventRecord(ctx context.Context, hub magicevent.Hub, record managerEventRecordFixture) error {
	if record.ID == "" {
		record.ID = fmt.Sprintf("workspace-view-event-%d", time.Now().UnixNano())
	}
	if record.Version == "" {
		record.Version = recordpayload.WorkorchEventVersion
	}
	if record.Revision == 0 {
		record.Revision = 1
	}
	data, err := eventstreamevents.EventRecordDataFromModel(&eventstreammodel.EventRecord{
		ID:           common.EventRecordID(record.ID),
		Version:      record.Version,
		Topic:        record.Topic,
		OccurredAt:   record.OccurredAt,
		SourceModule: record.SourceModule,
		SubjectType:  record.SubjectType,
		SubjectID:    record.SubjectID,
		WorkspaceID:  common.WorkspaceID(record.WorkspaceID),
		RunID:        common.RunID(record.RunID),
		StepID:       common.StepID(record.StepID),
		Revision:     record.Revision,
		Payload:      mustManagerEventPayload(record.Payload),
	})
	if err != nil {
		return err
	}
	event := magicevent.NewEventWithContext(
		eventstreamevents.TopicEventStreamRecordAppended,
		"blocks/eventstream",
		eventstreamevents.NotificationDestination,
		magicevent.NewHeader(),
		ctx,
		eventstreamevents.EventRecordAppendedData{Record: data},
	)
	result := hub.Send(event)
	if result == nil {
		return fmt.Errorf("record appended event returned no result")
	}
	if resultErr := result.Error(); resultErr != nil {
		return resultErr
	}
	return nil
}

func mustManagerEventPayload(input map[string]any) workorcheventpayload.Payload {
	payload, err := workorcheventpayload.FromMap(input)
	if err != nil {
		panic(err)
	}
	return payload
}

func TestStoreReplaceAllListSnapshotAndDelete(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{
		{ID: "ws-b", Title: "B", State: "active"},
		{ID: "ws-a", Title: "A", State: "active"},
	}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-a": {
			Workspace: appmodelcore.Workspace{
				ID:               "ws-a",
				Title:            "A+",
				State:            string(common.WorkspaceStateActive),
				ContextHints:     map[string]any{"large": "profile"},
				EvidencePolicies: []map[string]any{{"large": "policy"}},
			},
			Runs: []appmodelcore.Run{{
				ID:          "run-a",
				WorkspaceID: "ws-a",
				State:       string(common.RunStateActive),
			}},
			LLMUsageTraces: []projectioncontract.LLMUsageTrace{{
				ID:    "usage-a",
				RunID: "run-a",
				ContextBudget: mustManagerEventPayload(map[string]any{
					"failureSignature": strings.Repeat("x", 2048),
				}),
			}},
		},
	}, nil)

	items, status, err := store.List()
	if err != nil {
		t.Fatalf("list workspaces: %v", err)
	}
	if status.State != StateReady || status.GlobalRevision <= 0 || status.WorkspaceTotal != 2 {
		t.Fatalf("status = %+v, want ready positive global revision total 2", status)
	}
	firstGlobal := status.GlobalRevision
	if len(items) != 2 || items[0].ID != "ws-a" || items[0].Title != "A+" || items[1].ID != "ws-b" {
		t.Fatalf("items = %+v, want sorted snapshot-backed workspaces", items)
	}
	if items[0].ViewModel.WorkspaceID != "ws-a" ||
		items[0].ViewModel.PromptMode != "workspace" ||
		items[0].ViewModel.GlobalRevision != status.GlobalRevision ||
		items[0].ViewModel.WorkspaceRevision == 0 ||
		items[0].ViewModel.Revision != items[0].ViewModel.WorkspaceRevision {
		t.Fatalf("list viewModel = %+v, status = %+v, want projection-owned workspace prompt", items[0].ViewModel, status)
	}
	if item, ok := items[0].ViewModel.Sections["promptMode"]; !ok || item.Revision != items[0].ViewModel.Revision {
		t.Fatalf("list promptMode section = %+v, want section revision from viewModel %+v", item, items[0].ViewModel)
	}
	if items[0].ContextHints != nil || items[0].EvidencePolicies != nil {
		t.Fatalf("list workspace = %+v, want compact overview without large policy/context fields", items[0])
	}

	snapshot, snapshotStatus, err := store.GetSnapshot("ws-a")
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	if snapshotStatus.GlobalRevision != status.GlobalRevision || snapshot.Workspace.State != string(common.WorkspaceStateActive) {
		t.Fatalf("snapshot/status = %+v/%+v, want active snapshot at same revision", snapshot, snapshotStatus)
	}
	if snapshot.ViewModel.WorkspaceID != "ws-a" ||
		snapshot.ViewModel.PromptMode != "workspace" ||
		snapshot.ViewModel.GlobalRevision != snapshotStatus.GlobalRevision ||
		snapshot.ViewModel.WorkspaceRevision != snapshotStatus.WorkspaceRevision ||
		snapshot.ViewModel.Revision != snapshotStatus.WorkspaceRevision {
		t.Fatalf("snapshot viewModel = %+v, status = %+v, want projection-owned revisions", snapshot.ViewModel, snapshotStatus)
	}
	for _, section := range []string{"list", "activeWorkspace", "runState", "promptMode", "commands", "coverage", "currentFocus", "latestDiagnostic"} {
		item, ok := snapshot.ViewModel.Sections[section]
		if !ok {
			t.Fatalf("snapshot viewModel sections = %+v, missing %s", snapshot.ViewModel.Sections, section)
		}
		if item.Revision != snapshot.ViewModel.Revision ||
			item.WorkspaceRevision != snapshot.ViewModel.WorkspaceRevision ||
			item.SourceEventRevision != snapshot.ViewModel.SourceEventRevision ||
			item.ProjectedSourceRevision != snapshot.ViewModel.ProjectedSourceRevision {
			t.Fatalf("viewModel section %s = %+v, want same revision as %+v", section, item, snapshot.ViewModel)
		}
	}
	if snapshot.Workspace.ContextHints != nil || snapshot.Workspace.EvidencePolicies != nil {
		t.Fatalf("snapshot workspace = %+v, want detail projection stored", snapshot.Workspace)
	}
	if len(snapshot.Runs) != 1 {
		t.Fatalf("snapshot runs = %+v, want compact detail run stored", snapshot.Runs)
	}
	if signature, ok := snapshot.LLMUsageTraces[0].ContextBudget.String("failureSignature"); !ok || len(signature) > 1024+len("...[truncated]") {
		t.Fatalf("stored LLM trace signature length = %d, want bounded detail projection", len(signature))
	}

	store.DeleteWorkspace("ws-a")
	items, status, err = store.List()
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if status.GlobalRevision <= firstGlobal || status.WorkspaceTotal != 1 {
		t.Fatalf("status after delete = %+v, want global revision > %d total 1", status, firstGlobal)
	}
	if len(items) != 1 || items[0].ID != "ws-b" {
		t.Fatalf("items after delete = %+v, want only ws-b", items)
	}
}

func TestStoreDegradedAndDetailNotReady(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{{ID: "ws-a", State: "active"}}, nil, errors.New("snapshot unavailable"))

	items, status, err := store.List()
	if err != nil {
		t.Fatalf("list degraded workspaces: %v", err)
	}
	if status.State != StateDegraded || status.Error == "" || len(items) != 1 {
		t.Fatalf("status/items = %+v/%+v, want degraded with one overview", status, items)
	}
	if !status.Degraded {
		t.Fatalf("status = %+v, want degraded flag", status)
	}
	if status.StreamGeneration == "" {
		t.Fatal("degraded status missing streamGeneration")
	}
	if len(status.FailedWorkspaceIds) != 1 || status.FailedWorkspaceIds[0] != "ws-a" {
		t.Fatalf("failedWorkspaceIds = %v, want [ws-a]", status.FailedWorkspaceIds)
	}
	if status.DegradedSince == "" {
		t.Fatalf("status = %+v, want degradedSince", status)
	}
	if items[0].ViewModel.StreamGeneration != status.StreamGeneration {
		t.Fatalf("viewModel streamGeneration = %q, want %q", items[0].ViewModel.StreamGeneration, status.StreamGeneration)
	}
	if len(items[0].ViewModel.FailedWorkspaceIds) != 1 || items[0].ViewModel.FailedWorkspaceIds[0] != "ws-a" {
		t.Fatalf("viewModel failedWorkspaceIds = %v", items[0].ViewModel.FailedWorkspaceIds)
	}
	if _, detailStatus, err := store.GetSnapshot("ws-a"); err == nil {
		t.Fatal("get missing detail succeeded, want error")
	} else if detailStatus.State != StateDegraded {
		t.Fatalf("detail status = %+v, want degraded", detailStatus)
	}
}

func TestStoreListUsesCompactWorkspaceOverviewWithoutSnapshot(t *testing.T) {
	store := NewStore()
	store.ReplaceWorkspaceList([]appmodelcore.Workspace{{
		ID:               "ws-list-compact",
		Title:            "List Compact",
		State:            "active",
		SoulBindings:     map[string]string{"planning": "soul-a"},
		ContextHints:     map[string]any{"large": strings.Repeat("x", 4096)},
		EvidencePolicies: []map[string]any{{"policy": strings.Repeat("x", 4096)}},
	}}, nil)

	items, status, err := store.List()
	if err != nil {
		t.Fatalf("list workspaces: %v", err)
	}
	if status.State != StateReady || len(items) != 1 {
		t.Fatalf("status/items = %+v/%+v, want one ready workspace", status, items)
	}
	item := items[0]
	if item.ContextHints != nil || item.EvidencePolicies != nil {
		t.Fatalf("workspace overview = %+v, want compact list fields", item)
	}
	if item.SoulBindings["planning"] != "soul-a" {
		t.Fatalf("workspace overview soul bindings = %+v, want retained lightweight bindings", item.SoulBindings)
	}
	if item.ViewModel.WorkspaceID != "ws-list-compact" || item.ViewModel.Revision == 0 {
		t.Fatalf("workspace viewModel = %+v, want projected revision", item.ViewModel)
	}
}

func TestStoreListSnapshotOmitsDetailHeavyPayloads(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{{
		ID:    "ws-compact-detail",
		Title: "Compact Detail",
		State: "running",
	}}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-compact-detail": {
			Workspace: appmodelcore.Workspace{ID: "ws-compact-detail", State: "running", ActiveRunID: "run-compact-detail"},
			Run:       &appmodelcore.Run{ID: "run-compact-detail", WorkspaceID: "ws-compact-detail", State: "running"},
			RunHeartbeat: projectioncontract.RunHeartbeat{
				WorkspaceID: "ws-compact-detail",
				RunID:       "run-compact-detail",
				State:       "running",
			},
			RunCommand: &readmodelcore.RunCommandStatus{
				WorkspaceID: "ws-compact-detail",
				RunID:       "run-compact-detail",
				State:       "accepted",
				Intent:      "resume",
				Revision:    9,
			},
			TaskProgress: readmodelcore.TaskProgress{
				WorkspaceID: "ws-compact-detail",
				RunID:       "run-compact-detail",
				State:       "running",
			},
			CoverageSummary: map[string]any{"open": 1},
			ContextBundle: &readmodelcore.ContextBundle{
				WorkspaceID: "ws-compact-detail",
				RunID:       "run-compact-detail",
				Mode:        "run",
				Sections: []readmodelcore.ContextSection{{
					ID:      "detail-section",
					Kind:    "detail",
					Title:   "Detail",
					Summary: strings.Repeat("x", 4096),
				}},
			},
			Runs:           []appmodelcore.Run{{ID: "run-compact-detail", WorkspaceID: "ws-compact-detail", State: "running"}},
			Results:        []readmodelcore.Result{{ID: "result-detail", WorkspaceID: "ws-compact-detail", RunID: "run-compact-detail", State: "accepted", Summary: strings.Repeat("x", 4096)}},
			Artifacts:      []projectioncontract.Artifact{{ID: "artifact-detail", WorkspaceID: "ws-compact-detail", RunID: "run-compact-detail", ArtifactType: "trace", State: "active", Metadata: map[string]any{"payload": strings.Repeat("x", 4096)}}},
			LLMUsageTraces: []projectioncontract.LLMUsageTrace{{ID: "usage-detail", WorkspaceID: "ws-compact-detail", RunID: "run-compact-detail", ContextBudget: mustManagerEventPayload(map[string]any{"payload": strings.Repeat("x", 4096)})}},
			EventRecords:   []projectioncontract.EventRecord{{ID: "event-detail", WorkspaceID: "ws-compact-detail", RunID: "run-compact-detail"}},
			Decisions:      []appmodelcore.Decision{{ID: "ticket-detail", WorkspaceID: "ws-compact-detail", RunID: "run-compact-detail", State: string(common.DecisionTicketStateOpen)}},
			Approvals:      []appmodelcore.Approval{{ID: "approval-detail", WorkspaceID: "ws-compact-detail", RunID: "run-compact-detail", State: string(common.ApprovalStatePending)}},
		},
	}, nil)

	store.mu.RLock()
	entry := store.workspaces["ws-compact-detail"]
	listSnapshot := normalizedEntryListSnapshot(entry)
	detailSnapshot := normalizedEntrySnapshot(entry)
	store.mu.RUnlock()

	if listSnapshot.Run == nil || listSnapshot.Run.ID != "run-compact-detail" {
		t.Fatalf("list snapshot run = %+v, want compact active run", listSnapshot.Run)
	}
	if listSnapshot.RunHeartbeat.RunID != "run-compact-detail" || listSnapshot.TaskProgress.RunID != "run-compact-detail" {
		t.Fatalf("list heartbeat/progress = %+v/%+v, want compact run status", listSnapshot.RunHeartbeat, listSnapshot.TaskProgress)
	}
	if len(listSnapshot.Decisions) != 1 || len(listSnapshot.Approvals) != 1 {
		t.Fatalf("list governance = decisions:%d approvals:%d, want compact blockers retained", len(listSnapshot.Decisions), len(listSnapshot.Approvals))
	}
	if listSnapshot.ContextBundle != nil ||
		len(listSnapshot.Runs) != 0 ||
		len(listSnapshot.Results) != 0 ||
		len(listSnapshot.Artifacts) != 0 ||
		len(listSnapshot.LLMUsageTraces) != 0 ||
		len(listSnapshot.EventRecords) != 0 {
		t.Fatalf("list snapshot retained detail payloads: context=%v runs=%d results=%d artifacts=%d usage=%d events=%d",
			listSnapshot.ContextBundle != nil,
			len(listSnapshot.Runs),
			len(listSnapshot.Results),
			len(listSnapshot.Artifacts),
			len(listSnapshot.LLMUsageTraces),
			len(listSnapshot.EventRecords))
	}
	if detailSnapshot.ContextBundle == nil ||
		len(detailSnapshot.Runs) == 0 ||
		len(detailSnapshot.Results) == 0 ||
		len(detailSnapshot.Artifacts) == 0 ||
		len(detailSnapshot.LLMUsageTraces) == 0 ||
		len(detailSnapshot.EventRecords) == 0 {
		t.Fatalf("detail snapshot lost explicit detail payloads: context=%v runs=%d results=%d artifacts=%d usage=%d events=%d",
			detailSnapshot.ContextBundle != nil,
			len(detailSnapshot.Runs),
			len(detailSnapshot.Results),
			len(detailSnapshot.Artifacts),
			len(detailSnapshot.LLMUsageTraces),
			len(detailSnapshot.EventRecords))
	}

	summarySnapshot, _, err := store.GetSummarySnapshot("ws-compact-detail")
	if err != nil {
		t.Fatalf("get summary snapshot: %v", err)
	}
	if summarySnapshot.Run != nil ||
		summarySnapshot.ContextBundle != nil ||
		len(summarySnapshot.Runs) != 0 ||
		len(summarySnapshot.Results) != 0 ||
		len(summarySnapshot.Artifacts) != 0 ||
		len(summarySnapshot.LLMUsageTraces) != 0 ||
		len(summarySnapshot.EventRecords) != 0 {
		t.Fatalf("summary snapshot retained detail payloads: run=%v context=%v runs=%d results=%d artifacts=%d usage=%d events=%d",
			summarySnapshot.Run != nil,
			summarySnapshot.ContextBundle != nil,
			len(summarySnapshot.Runs),
			len(summarySnapshot.Results),
			len(summarySnapshot.Artifacts),
			len(summarySnapshot.LLMUsageTraces),
			len(summarySnapshot.EventRecords))
	}
	if summarySnapshot.RunCommand == nil || summarySnapshot.RunCommand.Revision != 9 {
		t.Fatalf("summary run command = %+v, want lightweight command projection", summarySnapshot.RunCommand)
	}
	if summarySnapshot.CoverageSummary["open"] != 1 {
		t.Fatalf("summary coverage = %+v, want lightweight coverage summary", summarySnapshot.CoverageSummary)
	}
}

func TestStoreUpsertWorkspaceSynchronizesDetailWorkspaceState(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{{ID: "ws-a", State: "waiting_decision", ActiveRunID: "run-a", BlockingTicketID: "ticket-a"}}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-a": {
			Workspace: appmodelcore.Workspace{ID: "ws-a", State: "waiting_decision", ActiveRunID: "run-a", BlockingTicketID: "ticket-a"},
			RunCommand: &readmodelcore.RunCommandStatus{
				WorkspaceID: "ws-a",
				RunID:       "run-a",
				Intent:      "replan",
				RequestID:   "request-a",
				Revision:    7,
			},
			GapBaseline: &projectioncontract.GapBaseline{
				WorkspaceID:      "ws-a",
				RunID:            "run-a",
				BaselineRevision: "gap-a",
			},
		},
	}, nil)

	store.UpsertWorkspace(appmodelcore.Workspace{ID: "ws-a", State: string(common.WorkspaceStateActive), ActiveRunID: "run-a"})

	items, _, err := store.List()
	if err != nil {
		t.Fatalf("list workspaces: %v", err)
	}
	if len(items) != 1 || items[0].State != string(common.WorkspaceStateActive) || items[0].BlockingTicketID != "" {
		t.Fatalf("list workspace = %+v, want active without blocker", items)
	}
	snapshot, _, err := store.GetSnapshot("ws-a")
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	if snapshot.Workspace.State != string(common.WorkspaceStateActive) || snapshot.Workspace.BlockingTicketID != "" {
		t.Fatalf("snapshot workspace = %+v, want synchronized active state", snapshot.Workspace)
	}
	if snapshot.RunCommand == nil || snapshot.RunCommand.RequestID != "request-a" {
		t.Fatalf("RunCommand = %+v, want retained command projection", snapshot.RunCommand)
	}
	if snapshot.GapBaseline == nil || snapshot.GapBaseline.BaselineRevision != "gap-a" {
		t.Fatalf("GapBaseline = %+v, want retained baseline projection", snapshot.GapBaseline)
	}
}

func TestStoreReplaceWorkspaceListSynchronizesDetailWorkspaceState(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{{ID: "ws-a", State: "waiting_decision", ActiveRunID: "run-a", BlockingTicketID: "ticket-a"}}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-a": {
			Workspace:  appmodelcore.Workspace{ID: "ws-a", State: "waiting_decision", ActiveRunID: "run-a", BlockingTicketID: "ticket-a"},
			RunCommand: &readmodelcore.RunCommandStatus{WorkspaceID: "ws-a", RunID: "run-a", Intent: "resume", RequestID: "request-a", Revision: 9},
			GapBaseline: &projectioncontract.GapBaseline{
				WorkspaceID:      "ws-a",
				RunID:            "run-a",
				BaselineRevision: "gap-a",
			},
		},
	}, nil)

	store.ReplaceWorkspaceList([]appmodelcore.Workspace{{ID: "ws-a", State: "active"}}, nil)

	snapshot, _, err := store.GetSnapshot("ws-a")
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	if snapshot.Workspace.State != "active" || snapshot.Workspace.ActiveRunID != "" || snapshot.Workspace.BlockingTicketID != "" {
		t.Fatalf("snapshot workspace = %+v, want synchronized active state", snapshot.Workspace)
	}
	if snapshot.RunCommand == nil || snapshot.RunCommand.RequestID != "request-a" {
		t.Fatalf("RunCommand = %+v, want retained command projection", snapshot.RunCommand)
	}
	if snapshot.GapBaseline == nil || snapshot.GapBaseline.BaselineRevision != "gap-a" {
		t.Fatalf("GapBaseline = %+v, want retained baseline projection", snapshot.GapBaseline)
	}
}

func TestStoreReplaceWorkspaceListPreservesSnapshotGovernanceAuthority(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{{ID: "ws-a", State: "waiting_decision", ActiveRunID: "run-a", BlockingTicketID: "ticket-a"}}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-a": {
			Workspace: appmodelcore.Workspace{ID: "ws-a", State: "waiting_decision", ActiveRunID: "run-a", BlockingTicketID: "ticket-a"},
			Run:       &appmodelcore.Run{ID: "run-a", WorkspaceID: "ws-a", State: string(common.RunStateBlockedOnDecision), BlockingTicketID: "ticket-a"},
			Decisions: []appmodelcore.Decision{{
				ID:          "ticket-a",
				WorkspaceID: "ws-a",
				RunID:       "run-a",
				State:       string(common.DecisionTicketStateOpen),
			}},
		},
	}, nil)

	store.ReplaceWorkspaceList([]appmodelcore.Workspace{{ID: "ws-a", State: "active", ActiveRunID: "run-a"}}, nil)

	items, _, err := store.List()
	if err != nil {
		t.Fatalf("list workspaces: %v", err)
	}
	if len(items) != 1 ||
		items[0].State != string(common.WorkspaceStateActive) ||
		items[0].BlockingTicketID != "ticket-a" ||
		items[0].ViewModel.PromptMode != "decision" {
		t.Fatalf("list workspace = %+v, want decision projection from snapshot governance", items)
	}
	snapshot, _, err := store.GetSnapshot("ws-a")
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	if snapshot.Workspace.State != string(common.WorkspaceStateActive) ||
		snapshot.Workspace.BlockingTicketID != "ticket-a" ||
		snapshot.ViewModel.PromptMode != "decision" {
		t.Fatalf("snapshot workspace = %+v viewModel = %+v, want decision projection", snapshot.Workspace, snapshot.ViewModel)
	}
}

func TestStoreProjectionDerivesDecisionPromptFromBlockedRun(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{{ID: "ws-a", Title: "Workspace A", State: "active", ActiveRunID: "run-a"}}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-a": {
			Workspace: appmodelcore.Workspace{ID: "ws-a", Title: "Workspace A", State: "active", ActiveRunID: "run-a"},
			Run:       &appmodelcore.Run{ID: "run-a", WorkspaceID: "ws-a", State: string(common.RunStateBlockedOnDecision), BlockingTicketID: "ticket-a"},
			Decisions: []appmodelcore.Decision{{
				ID:          "ticket-a",
				WorkspaceID: "ws-a",
				RunID:       "run-a",
				State:       string(common.DecisionTicketStateOpen),
			}},
		},
	}, nil)

	items, _, err := store.List()
	if err != nil {
		t.Fatalf("list workspaces: %v", err)
	}
	if len(items) != 1 ||
		items[0].State != string(common.WorkspaceStateActive) ||
		items[0].BlockingTicketID != "ticket-a" ||
		items[0].ViewModel.PromptMode != "decision" ||
		items[0].Title != "Workspace A" {
		t.Fatalf("list workspace = %+v, want decision prompt from blocked run with preserved workspace fields", items)
	}

	detail, _, err := store.GetSnapshot("ws-a")
	if err != nil {
		t.Fatalf("get detail snapshot: %v", err)
	}
	if detail.Workspace.State != string(common.WorkspaceStateActive) ||
		detail.Workspace.BlockingTicketID != "ticket-a" ||
		detail.ViewModel.PromptMode != "decision" {
		t.Fatalf("detail snapshot workspace = %+v viewModel = %+v, want decision prompt", detail.Workspace, detail.ViewModel)
	}

	summary, _, err := store.GetSummarySnapshot("ws-a")
	if err != nil {
		t.Fatalf("get summary snapshot: %v", err)
	}
	if summary.Workspace.State != string(common.WorkspaceStateActive) ||
		summary.Workspace.BlockingTicketID != "ticket-a" ||
		summary.ViewModel.PromptMode != "decision" {
		t.Fatalf("summary snapshot workspace = %+v viewModel = %+v, want decision prompt", summary.Workspace, summary.ViewModel)
	}
}

func TestStoreProjectionRequiresSameScopeOpenDecisionForPrompt(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{{ID: "ws-a", State: string(common.WorkspaceStateActive), ActiveRunID: "run-a", BlockingTicketID: "ticket-a"}}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-a": {
			Workspace: appmodelcore.Workspace{ID: "ws-a", State: string(common.WorkspaceStateActive), ActiveRunID: "run-a", BlockingTicketID: "ticket-a"},
			Run:       &appmodelcore.Run{ID: "run-a", WorkspaceID: "ws-a", State: string(common.RunStateBlockedOnDecision), BlockingTicketID: "ticket-a"},
			Decisions: []appmodelcore.Decision{{
				ID:          "ticket-a",
				WorkspaceID: "ws-other",
				RunID:       "run-a",
				State:       string(common.DecisionTicketStateOpen),
			}},
		},
	}, nil)

	items, _, err := store.List()
	if err != nil {
		t.Fatalf("list workspaces: %v", err)
	}
	if len(items) != 1 ||
		items[0].State != string(common.WorkspaceStateActive) ||
		items[0].BlockingTicketID != "" ||
		items[0].ViewModel.PromptMode == "decision" {
		t.Fatalf("list workspace = %+v, want no decision prompt without same-scope open ticket", items)
	}
	snapshot, _, err := store.GetSnapshot("ws-a")
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	if snapshot.Workspace.State != string(common.WorkspaceStateActive) ||
		snapshot.Workspace.BlockingTicketID != "" ||
		snapshot.ViewModel.PromptMode == "decision" {
		t.Fatalf("snapshot workspace = %+v viewModel = %+v, want no decision prompt without same-scope open ticket", snapshot.Workspace, snapshot.ViewModel)
	}
}

func TestStoreMarksStaleWithSourceEventRevision(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{{ID: "ws-a", State: "active"}}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-a": {Workspace: appmodelcore.Workspace{ID: "ws-a", State: "active"}},
	}, nil)

	store.MarkStale("ws-a", 42)
	_, status, err := store.List()
	if err != nil {
		t.Fatalf("list stale workspaces: %v", err)
	}
	if status.State != StateStale || status.SourceEventRevision != 42 {
		t.Fatalf("status = %+v, want stale source revision 42", status)
	}
	_, detailStatus, err := store.GetSnapshot("ws-a")
	if err != nil {
		t.Fatalf("get stale snapshot: %v", err)
	}
	if detailStatus.State != StateStale || detailStatus.SourceEventRevision != 42 {
		t.Fatalf("detail status = %+v, want stale source revision 42", detailStatus)
	}
	workspaceStatus := store.StatusForWorkspace("ws-a")
	if workspaceStatus.State != StateStale || workspaceStatus.SourceEventRevision != 42 {
		t.Fatalf("workspace status = %+v, want stale source revision 42", workspaceStatus)
	}

	store.UpsertSnapshot(readmodelcore.WorkspaceSnapshot{Workspace: appmodelcore.Workspace{ID: "ws-a", State: "running"}})
	_, status, err = store.List()
	if err != nil {
		t.Fatalf("list rebuilt workspaces: %v", err)
	}
	if status.State != StateReady || status.SourceEventRevision != 42 {
		t.Fatalf("rebuilt status = %+v, want ready retaining source revision 42", status)
	}
}

func TestStoreStaleSnapshotKeepsLastReadyViewModel(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{{ID: "ws-a", State: "waiting_decision", ActiveRunID: "run-a", BlockingTicketID: "ticket-a"}}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-a": {
			Workspace: appmodelcore.Workspace{
				ID:               "ws-a",
				State:            "waiting_decision",
				ActiveRunID:      "run-a",
				BlockingTicketID: "ticket-a",
			},
			Decisions: []appmodelcore.Decision{{
				ID:          "ticket-a",
				WorkspaceID: "ws-a",
				RunID:       "run-a",
				State:       "open",
			}},
		},
	}, nil)

	readySnapshot, readyStatus, err := store.GetSnapshot("ws-a")
	if err != nil {
		t.Fatalf("get ready snapshot: %v", err)
	}
	if readySnapshot.ViewModel.PromptMode != "decision" {
		t.Fatalf("ready viewModel = %+v, want decision prompt", readySnapshot.ViewModel)
	}

	store.MarkStale("ws-a", 42)

	staleSnapshot, staleStatus, err := store.GetSnapshot("ws-a")
	if err != nil {
		t.Fatalf("get stale snapshot: %v", err)
	}
	if staleStatus.State != StateStale || staleStatus.StaleReason != "source_event" {
		t.Fatalf("stale status = %+v, want source_event stale state", staleStatus)
	}
	if staleSnapshot.Workspace.State != readySnapshot.Workspace.State ||
		staleSnapshot.Workspace.BlockingTicketID != readySnapshot.Workspace.BlockingTicketID {
		t.Fatalf("stale workspace = %+v, ready = %+v, want last ready workspace state", staleSnapshot.Workspace, readySnapshot.Workspace)
	}
	if staleSnapshot.ViewModel.PromptMode != "decision" {
		t.Fatalf("stale viewModel = %+v, want last ready decision prompt", staleSnapshot.ViewModel)
	}
	if staleSnapshot.ViewModel.State != StateStale ||
		staleSnapshot.ViewModel.StaleReason != "source_event" ||
		staleSnapshot.ViewModel.SourceEventRevision != 42 {
		t.Fatalf("stale viewModel = %+v, want stale source revision metadata", staleSnapshot.ViewModel)
	}
	if staleSnapshot.ViewModel.ProjectedSourceRevision != readyStatus.ProjectedSourceRevision {
		t.Fatalf("stale viewModel projected revision = %d, want last ready projected revision %d", staleSnapshot.ViewModel.ProjectedSourceRevision, readyStatus.ProjectedSourceRevision)
	}
}

func TestStoreDoesNotUseResolvedDecisionForPromptMode(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{{
		ID:               "ws-resolved",
		State:            "waiting_decision",
		ActiveRunID:      "run-resolved",
		BlockingTicketID: "ticket-resolved",
	}}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-resolved": {
			Workspace: appmodelcore.Workspace{
				ID:               "ws-resolved",
				State:            "waiting_decision",
				ActiveRunID:      "run-resolved",
				BlockingTicketID: "ticket-resolved",
			},
			Decisions: []appmodelcore.Decision{{
				ID:          "ticket-resolved",
				WorkspaceID: "ws-resolved",
				RunID:       "run-resolved",
				State:       "resolved",
			}},
		},
	}, nil)

	items, _, err := store.List()
	if err != nil {
		t.Fatalf("list workspaces: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("workspaces = %d, want 1", len(items))
	}
	if items[0].ViewModel.PromptMode == "decision" || items[0].BlockingTicketID != "" {
		t.Fatalf("list workspace = %+v, want no decision prompt for resolved ticket", items[0])
	}
	snapshot, _, err := store.GetSnapshot("ws-resolved")
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	if snapshot.ViewModel.PromptMode == "decision" || snapshot.Workspace.BlockingTicketID != "" {
		t.Fatalf("snapshot workspace = %+v viewModel = %+v, want no decision prompt for resolved ticket", snapshot.Workspace, snapshot.ViewModel)
	}
}

func TestStoreWorkspaceStatusUsesGlobalRevisionForQuietSnapshot(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{
		{ID: "ws-quiet", State: "blocked"},
		{ID: "ws-active", State: "running"},
	}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-quiet":  {Workspace: appmodelcore.Workspace{ID: "ws-quiet", State: "blocked"}},
		"ws-active": {Workspace: appmodelcore.Workspace{ID: "ws-active", State: "running"}},
	}, nil)

	_, initialStatus, err := store.GetSnapshot("ws-quiet")
	if err != nil {
		t.Fatalf("get initial quiet snapshot: %v", err)
	}
	initialProjectionBytes, err := json.Marshal(initialStatus)
	if err != nil {
		t.Fatalf("marshal initial status: %v", err)
	}
	var initialProjection map[string]any
	if err := json.Unmarshal(initialProjectionBytes, &initialProjection); err != nil {
		t.Fatalf("unmarshal initial status: %v", err)
	}
	if _, ok := initialProjection["sourceEventRevision"]; !ok {
		t.Fatalf("initial projection json = %s, want sourceEventRevision field even when zero", initialProjectionBytes)
	}
	if _, ok := initialProjection["projectedSourceRevision"]; !ok {
		t.Fatalf("initial projection json = %s, want projectedSourceRevision field even when zero", initialProjectionBytes)
	}

	store.MarkStale("ws-active", 99)

	_, quietStatus, err := store.GetSnapshot("ws-quiet")
	if err != nil {
		t.Fatalf("get quiet snapshot: %v", err)
	}
	if quietStatus.State != StateReady {
		t.Fatalf("quiet status = %+v, want ready quiet workspace status", quietStatus)
	}
	if quietStatus.SourceEventRevision != 99 || quietStatus.ProjectedSourceRevision != 99 {
		t.Fatalf("quiet status revisions = %+v, want source/projected revision 99", quietStatus)
	}
	if quietStatus.WorkspaceRevision == 0 || quietStatus.UpdatedAt == "" || quietStatus.RebuiltAt == "" {
		t.Fatalf("quiet status display metadata = %+v, want workspace revision and timestamps", quietStatus)
	}
}

func TestStoreKeepsExistingProjectionReadableWhileRebuilding(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{{ID: "ws-a", State: "active"}}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-a": {Workspace: appmodelcore.Workspace{ID: "ws-a", State: string(common.WorkspaceStateActive)}},
	}, nil)

	store.setBuilding()

	items, status, err := store.List()
	if err != nil {
		t.Fatalf("list stale workspaces during rebuild: %v", err)
	}
	if status.State != StateStale || len(items) != 1 || items[0].ID != "ws-a" {
		t.Fatalf("status/items = %+v/%+v, want stale existing projection", status, items)
	}
	snapshot, detailStatus, err := store.GetSnapshot("ws-a")
	if err != nil {
		t.Fatalf("get stale snapshot during rebuild: %v", err)
	}
	if detailStatus.State != StateStale || snapshot.Workspace.State != string(common.WorkspaceStateActive) {
		t.Fatalf("detail/status = %+v/%+v, want stale active snapshot", snapshot, detailStatus)
	}
}

func TestStoreRunHeartbeatMetricsCountsActiveProblemSignals(t *testing.T) {
	store := NewStore()
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	store.ReplaceAll([]appmodelcore.Workspace{
		{ID: "ws-active", State: "running"},
		{ID: "ws-decision", State: "waiting_decision"},
		{ID: "ws-terminal", State: "completed"},
	}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-active": {
			Workspace: appmodelcore.Workspace{ID: "ws-active", State: "running"},
			RunHeartbeat: projectioncontract.RunHeartbeat{
				WorkspaceID:               "ws-active",
				RunID:                     "run-active",
				State:                     string(common.RunStateActive),
				ActivityState:             "stalled",
				LatestEffectiveProgressAt: now.Add(-12 * time.Minute).Format(time.RFC3339Nano),
				Stalled:                   true,
				IneffectiveRetry:          true,
			},
			Acceptance: projectioncontract.AcceptanceProjection{
				Summary: projectioncontract.AcceptanceSummary{ActiveSlices: 1},
				CurrentFocus: &projectioncontract.CoverageFocus{
					ID:      "focus-active",
					State:   "active",
					Attempt: 2,
				},
				Budget: &projectioncontract.ConvergenceBudget{
					Decision:              "decision_required",
					RepairAttempts:        3,
					NoCoverageDeltaRounds: 1,
				},
			},
		},
		"ws-decision": {
			Workspace: appmodelcore.Workspace{ID: "ws-decision", State: "waiting_decision", ActiveRunID: "run-decision", BlockingTicketID: "ticket-decision"},
			RunHeartbeat: projectioncontract.RunHeartbeat{
				WorkspaceID:               "ws-decision",
				RunID:                     "run-decision",
				State:                     string(common.RunStateBlockedOnDecision),
				ActivityState:             "stalled",
				LatestEffectiveProgressAt: now.Add(-20 * time.Minute).Format(time.RFC3339Nano),
				Stalled:                   true,
				IneffectiveRetry:          true,
			},
		},
		"ws-terminal": {
			Workspace: appmodelcore.Workspace{ID: "ws-terminal", State: "completed"},
			RunHeartbeat: projectioncontract.RunHeartbeat{
				WorkspaceID:               "ws-terminal",
				RunID:                     "run-terminal",
				State:                     string(common.RunStateCompleted),
				ActivityState:             "stalled",
				LatestEffectiveProgressAt: now.Add(-30 * time.Minute).Format(time.RFC3339Nano),
				Stalled:                   true,
				IneffectiveRetry:          true,
			},
		},
	}, nil)

	metrics := store.RunHeartbeatMetrics(now)
	if metrics.ActiveRunHeartbeatTotal != 1 {
		t.Fatalf("active heartbeat total = %d, want 1", metrics.ActiveRunHeartbeatTotal)
	}
	if metrics.ActiveRunHeartbeatAgeSecondsMax != int64((12 * time.Minute).Seconds()) {
		t.Fatalf("active heartbeat age max = %d, want 720", metrics.ActiveRunHeartbeatAgeSecondsMax)
	}
	if metrics.StalledRunTotal != 1 || metrics.IneffectiveRetryRunTotal != 1 {
		t.Fatalf("problem signal totals = stalled %d ineffective %d, want 1/1", metrics.StalledRunTotal, metrics.IneffectiveRetryRunTotal)
	}
	if metrics.ConvergenceActiveSlices != 1 ||
		metrics.ConvergenceOpenFocuses != 1 ||
		metrics.ConvergenceRepairAttemptsTotal != 3 ||
		metrics.ConvergenceNoDeltaRoundsTotal != 1 ||
		metrics.ConvergenceBudgetEscalations != 1 {
		t.Fatalf("convergence metrics = %+v, want active/open/repair/no-delta/escalation 1/1/3/1/1", metrics)
	}
}

func TestManagerPublishesProjectionChangedAndIgnoresOwnEvent(t *testing.T) {
	ctx := context.Background()
	commandHub := magicevent.NewHub(32)
	t.Cleanup(func() { commandHub.Terminate(ctx) })
	projectionEvents := subscribeProjectionNotifications(t, commandHub)
	scheduled := map[string]int{}
	scheduledFuncs := map[string][]func(context.Context){}
	manager := NewManager(ManagerOptions{
		Loader: fakeProjectionLoader{
			workspaces: []appmodelcore.Workspace{{ID: "ws-a", State: "active"}},
			snapshots: map[string]readmodelcore.WorkspaceSnapshot{
				"ws-a": {Workspace: appmodelcore.Workspace{ID: "ws-a", State: "running"}},
			},
		},
		EventPorts: managerTestEventPorts(commandHub),
		Schedule: func(name string, fn func(context.Context)) bool {
			scheduled[name]++
			scheduledFuncs[name] = append(scheduledFuncs[name], fn)
			return true
		},
	})
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("start manager: %v", err)
	}
	if scheduled["workspace_projection_rebuild_all"] != 1 {
		t.Fatalf("startup full rebuild scheduled %d times, want 1", scheduled["workspace_projection_rebuild_all"])
	}
	scheduledFuncs["workspace_projection_rebuild_all"][0](ctx)
	baselineWorkspaceRebuilds := scheduled["workspace_projection_rebuild"]
	if baselineWorkspaceRebuilds != 0 {
		t.Fatalf("baseline workspace rebuilds = %d, want no detail fan-out", baselineWorkspaceRebuilds)
	}
	if err := manager.RebuildWorkspace(ctx, "ws-a"); err != nil {
		t.Fatalf("rebuild workspace: %v", err)
	}
	if scheduled["workspace_projection_rebuild"] != baselineWorkspaceRebuilds {
		t.Fatalf("scheduled own projection event rebuild count = %d, want unchanged %d", scheduled["workspace_projection_rebuild"], baselineWorkspaceRebuilds)
	}
	found := waitProjectionNotification(t, projectionEvents, func(data workorchdevents.WorkspaceProjectionChangedData) bool {
		return data.WorkspaceID == "ws-a" && data.ChangeKind == "detail"
	})
	if found.ProjectionState != StateReady {
		t.Fatalf("projection data = %+v, want detail ready", found)
	}

	// The application Biz owns the EventHub subscription and passes typed
	// records to this pure projection manager.
	_ = manager.HandleEventRecord(ctx, eventstreamevents.EventRecordData{Topic: runevents.TopicRunStateChanged, WorkspaceID: "ws-a", Revision: 1})
	if scheduled["workspace_projection_rebuild"] != baselineWorkspaceRebuilds+1 {
		t.Fatalf("scheduled rebuild count after run event = %d, want %d", scheduled["workspace_projection_rebuild"], baselineWorkspaceRebuilds+1)
	}
	status := manager.Status()
	if status.State != StateStale || status.SourceEventRevision == 0 {
		t.Fatalf("manager status after run event = %+v, want stale with source revision", status)
	}
}

func TestManagerProjectionRebuildNeverIssuesRuntimeAuthorityCommands(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	unsafeCalls := 0
	orchestratorObserver := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	for _, topic := range []string{
		orchestratorevents.TopicOrchestratorDriveRuntime,
		orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceTicket,
		fileevents.TopicExecuteFileEditOperation,
	} {
		orchestratorObserver.Subscribe(topic, func(magicevent.Event, magicevent.Result) { unsafeCalls++ })
	}
	resultObserver := magicevent.NewSimpleObserver(resultevents.CommandDestination, hub)
	resultObserver.Subscribe(resultevents.TopicResultCommandCreate, func(magicevent.Event, magicevent.Result) { unsafeCalls++ })
	t.Cleanup(func() {
		resultObserver.Unsubscribe(resultevents.TopicResultCommandCreate)
		for _, topic := range []string{
			orchestratorevents.TopicOrchestratorDriveRuntime,
			orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceTicket,
			fileevents.TopicExecuteFileEditOperation,
		} {
			orchestratorObserver.Unsubscribe(topic)
		}
	})
	published := 0
	manager := NewManager(ManagerOptions{
		Loader: fakeProjectionLoader{
			workspaces: []appmodelcore.Workspace{{ID: "ws-read-only", State: "active"}},
			snapshots: map[string]readmodelcore.WorkspaceSnapshot{
				"ws-read-only": {Workspace: appmodelcore.Workspace{ID: "ws-read-only", State: "active"}},
			},
		},
		EventPorts: EventPorts{PublishChanged: func(context.Context, workorchdevents.WorkspaceProjectionChangedData) { published++ }},
		Schedule:   func(string, func(context.Context)) bool { return true },
	})
	if err := manager.RebuildAll(ctx); err != nil {
		t.Fatalf("RebuildAll: %v", err)
	}
	if err := manager.RebuildWorkspace(ctx, "ws-read-only"); err != nil {
		t.Fatalf("RebuildWorkspace: %v", err)
	}
	if unsafeCalls != 0 {
		t.Fatalf("projection rebuild issued %d authority commands", unsafeCalls)
	}
	if published != 2 {
		t.Fatalf("projection notifications = %d, want baseline and detail only", published)
	}
}

func TestManagerPublishesProjectionChangedThroughAsyncHub(t *testing.T) {
	ctx := context.Background()
	commandHub := magicevent.NewHub(16)
	t.Cleanup(func() { commandHub.Terminate(ctx) })
	projectionEvents := subscribeProjectionNotifications(t, commandHub)
	manager := NewManager(ManagerOptions{EventPorts: managerTestEventPorts(commandHub)})
	manager.store.UpsertWorkspace(appmodelcore.Workspace{ID: "ws-a", State: "active"})

	manager.publishProjectionChanged(ctx, "ws-a", "detail")

	data := waitProjectionNotification(t, projectionEvents, func(data workorchdevents.WorkspaceProjectionChangedData) bool {
		return data.WorkspaceID == "ws-a" && data.ChangeKind == "detail"
	})
	status := manager.store.StatusForWorkspace("ws-a")
	if data.WorkspaceRevision != status.WorkspaceRevision ||
		data.GlobalRevision != status.GlobalRevision ||
		data.SourceEventRevision != status.SourceEventRevision ||
		data.ProjectedSourceRevision != status.ProjectedSourceRevision ||
		data.ProjectionState != status.State {
		t.Fatalf("projection data = %+v, want owner projection status %+v", data, status)
	}
}

func TestManagerReconcilesMissedEventFromOwnerHighWatermark(t *testing.T) {
	ctx := context.Background()
	commandHub := magicevent.NewHub(16)
	t.Cleanup(func() { commandHub.Terminate(ctx) })
	observer := magicevent.NewSimpleObserver(eventstreamevents.CommandDestination, commandHub)
	observer.Subscribe(eventstreamevents.TopicEventStreamCommandHighWatermarks, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(eventstreamevents.HighWatermarksCommand)
		if len(command.WorkspaceIDs) != 1 || command.WorkspaceIDs[0] != "ws-a" {
			t.Fatalf("high-watermark command = %+v", command)
		}
		result.Set(eventstreamevents.EventStreamCommandResult{HighWatermarks: []eventstreamevents.WorkspaceHighWatermark{{WorkspaceID: "ws-a", Revision: 42}}}, nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(eventstreamevents.TopicEventStreamCommandHighWatermarks) })
	scheduled := map[string]int{}
	manager := NewManager(ManagerOptions{
		Loader:     fakeProjectionLoader{workspaces: []appmodelcore.Workspace{{ID: "ws-a", State: "active"}}},
		EventPorts: managerTestEventPorts(commandHub),
		Schedule: func(name string, _ func(context.Context)) bool {
			scheduled[name]++
			return true
		},
	})
	manager.store.ReplaceAll([]appmodelcore.Workspace{{ID: "ws-a", State: "active"}}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-a": {Workspace: appmodelcore.Workspace{ID: "ws-a", State: "active"}},
	}, nil)
	if err := manager.ReconcileSourceHighWatermarks(ctx); err != nil {
		t.Fatal(err)
	}
	status := manager.Status()
	if status.State != StateStale || status.SourceEventRevision != 42 || status.ProjectedSourceRevision >= 42 {
		t.Fatalf("reconciled projection status = %+v", status)
	}
	if scheduled["workspace_projection_rebuild"] != 1 {
		t.Fatalf("workspace rebuild schedules = %d, want 1", scheduled["workspace_projection_rebuild"])
	}
}

func TestStoreRunCommandAcceptedProjectsCommandRevision(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{{ID: "ws-a", State: "active"}}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-a": {
			Workspace: appmodelcore.Workspace{ID: "ws-a", State: "active"},
		},
	}, nil)

	if !store.ApplyRunCommandAccepted(RunCommandAcceptedUpdate{
		WorkspaceID:        "ws-a",
		RunID:              "run-a",
		Intent:             "resume",
		Reason:             "user_requested",
		RequestID:          "request-a",
		IdempotencyKey:     "resume/ws-a/run-a",
		Source:             "tui",
		DefinitionRevision: "rev-a",
		DecisionID:         "ticket-a",
		Revision:           42,
		AcceptedAt:         "2026-06-21T00:00:00Z",
	}) {
		t.Fatalf("ApplyRunCommandAccepted returned false")
	}
	snapshot, status, err := store.GetSnapshot("ws-a")
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	if snapshot.RunCommand == nil ||
		snapshot.RunCommand.WorkspaceID != "ws-a" ||
		snapshot.RunCommand.RunID != "run-a" ||
		snapshot.RunCommand.State != "accepted" ||
		snapshot.RunCommand.Intent != "resume" ||
		snapshot.RunCommand.RequestID != "request-a" ||
		snapshot.RunCommand.IdempotencyKey != "resume/ws-a/run-a" ||
		snapshot.RunCommand.DefinitionRevision != "rev-a" ||
		snapshot.RunCommand.DecisionID != "ticket-a" ||
		snapshot.RunCommand.Revision != 42 {
		t.Fatalf("RunCommand = %+v, want accepted command projection", snapshot.RunCommand)
	}
	if status.SourceEventRevision != 42 || status.ProjectedSourceRevision != 42 {
		t.Fatalf("status revisions = %+v, want accepted command revision", status)
	}
}

func TestStoreRunCommandQueuedTransitionsToAccepted(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{{ID: "ws-a", State: "active"}}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-a": {Workspace: appmodelcore.Workspace{ID: "ws-a", State: "active"}},
	}, nil)

	if !store.ApplyRunCommandQueued(RunCommandQueuedUpdate{
		WorkspaceID: "ws-a", Intent: "resume", RequestID: "request-queued",
		IdempotencyKey: "resume/ws-a", Source: "web", QueuedAt: "2026-07-20T00:00:00Z",
	}) {
		t.Fatal("ApplyRunCommandQueued returned false")
	}
	snapshot, _, err := store.GetSnapshot("ws-a")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.RunCommand == nil || snapshot.RunCommand.State != "queued" || snapshot.RunCommand.RequestID != "request-queued" || snapshot.RunCommand.QueuedAt == "" {
		t.Fatalf("queued RunCommand = %+v", snapshot.RunCommand)
	}

	if !store.ApplyRunCommandAccepted(RunCommandAcceptedUpdate{
		WorkspaceID: "ws-a", Intent: "resume", RequestID: "request-queued",
		IdempotencyKey: "resume/ws-a", Source: "web", AcceptedAt: "2026-07-20T00:00:01Z",
	}) {
		t.Fatal("ApplyRunCommandAccepted returned false")
	}
	snapshot, _, err = store.GetSnapshot("ws-a")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.RunCommand == nil || snapshot.RunCommand.State != "accepted" || snapshot.RunCommand.QueuedAt != "2026-07-20T00:00:00Z" || snapshot.RunCommand.AcceptedAt == "" {
		t.Fatalf("accepted RunCommand = %+v, want queued timestamp retained", snapshot.RunCommand)
	}
}

func TestStoreRunCommandAcceptedCreatesDetailPlaceholderForKnownWorkspace(t *testing.T) {
	store := NewStore()
	store.ReplaceWorkspaceList([]appmodelcore.Workspace{{ID: "ws-a", State: "active"}}, nil)

	if !store.ApplyRunCommandAccepted(RunCommandAcceptedUpdate{
		WorkspaceID: "ws-a",
		RunID:       "run-a",
		Intent:      "replan",
		RequestID:   "request-a",
		Revision:    42,
	}) {
		t.Fatalf("ApplyRunCommandAccepted returned false")
	}
	snapshot, status, err := store.GetSnapshot("ws-a")
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	if snapshot.Workspace.ID != "ws-a" || snapshot.Workspace.State != "active" {
		t.Fatalf("Workspace = %+v, want placeholder workspace detail", snapshot.Workspace)
	}
	if snapshot.RunCommand == nil ||
		snapshot.RunCommand.WorkspaceID != "ws-a" ||
		snapshot.RunCommand.RunID != "run-a" ||
		snapshot.RunCommand.State != "accepted" ||
		snapshot.RunCommand.Intent != "replan" ||
		snapshot.RunCommand.RequestID != "request-a" ||
		snapshot.RunCommand.Revision != 42 {
		t.Fatalf("RunCommand = %+v, want accepted command projection", snapshot.RunCommand)
	}
	if status.SourceEventRevision != 42 || status.ProjectedSourceRevision != 42 {
		t.Fatalf("status revisions = %+v, want accepted command revision", status)
	}
}

func TestStoreUpsertSnapshotPreservesAcceptedRunCommand(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{{ID: "ws-a", State: "active"}}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-a": {Workspace: appmodelcore.Workspace{ID: "ws-a", State: "active"}},
	}, nil)
	if !store.ApplyRunCommandAccepted(RunCommandAcceptedUpdate{
		WorkspaceID: "ws-a",
		Intent:      "resume",
		RequestID:   "request-a",
		Revision:    42,
	}) {
		t.Fatalf("ApplyRunCommandAccepted returned false")
	}

	store.UpsertSnapshot(readmodelcore.WorkspaceSnapshot{
		Workspace: appmodelcore.Workspace{ID: "ws-a", State: "running"},
	})

	snapshot, status, err := store.GetSnapshot("ws-a")
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	if snapshot.RunCommand == nil ||
		snapshot.RunCommand.Intent != "resume" ||
		snapshot.RunCommand.RequestID != "request-a" ||
		snapshot.RunCommand.Revision != 42 {
		t.Fatalf("RunCommand = %+v, want preserved accepted command", snapshot.RunCommand)
	}
	if status.SourceEventRevision != 42 {
		t.Fatalf("status = %+v, want source revision retained", status)
	}
}

func TestStoreRunCommandAcceptedRejectsUnknownWorkspace(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{{ID: "ws-a", State: "active"}}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-a": {Workspace: appmodelcore.Workspace{ID: "ws-a", State: "active"}},
	}, nil)
	if store.ApplyRunCommandAccepted(RunCommandAcceptedUpdate{WorkspaceID: "ws-b", Intent: "resume", Revision: 2}) {
		t.Fatal("ApplyRunCommandAccepted returned true for unknown workspace")
	}
	snapshot, status, err := store.GetSnapshot("ws-a")
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	if snapshot.RunCommand != nil {
		t.Fatalf("RunCommand = %+v, want no cross-workspace command projection", snapshot.RunCommand)
	}
	if status.SourceEventRevision != 0 || status.ProjectedSourceRevision != 0 {
		t.Fatalf("status after rejected command = %+v, want unchanged revisions", status)
	}
}

func TestStoreRunCommandRejectedProjectsCommandRevision(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{{ID: "ws-a", State: "active"}}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-a": {
			Workspace: appmodelcore.Workspace{ID: "ws-a", State: "active"},
		},
	}, nil)

	if !store.ApplyRunCommandRejected(RunCommandRejectedUpdate{
		WorkspaceID:        "ws-a",
		RunID:              "run-a",
		Intent:             "resume",
		Reason:             "user_requested",
		RequestID:          "request-a",
		IdempotencyKey:     "resume/ws-a/run-a",
		Source:             "tui",
		DefinitionRevision: "rev-a",
		DecisionID:         "ticket-a",
		Revision:           43,
		RejectedAt:         "2026-06-21T00:01:00Z",
		Error:              "scheduler command was not queued",
	}) {
		t.Fatalf("ApplyRunCommandRejected returned false")
	}
	snapshot, status, err := store.GetSnapshot("ws-a")
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	if snapshot.RunCommand == nil ||
		snapshot.RunCommand.WorkspaceID != "ws-a" ||
		snapshot.RunCommand.RunID != "run-a" ||
		snapshot.RunCommand.State != "rejected" ||
		snapshot.RunCommand.Intent != "resume" ||
		snapshot.RunCommand.RequestID != "request-a" ||
		snapshot.RunCommand.IdempotencyKey != "resume/ws-a/run-a" ||
		snapshot.RunCommand.DefinitionRevision != "rev-a" ||
		snapshot.RunCommand.DecisionID != "ticket-a" ||
		snapshot.RunCommand.RejectedAt != "2026-06-21T00:01:00Z" ||
		snapshot.RunCommand.Error != "scheduler command was not queued" ||
		snapshot.RunCommand.Revision != 43 {
		t.Fatalf("RunCommand = %+v, want rejected command projection", snapshot.RunCommand)
	}
	if status.SourceEventRevision != 43 || status.ProjectedSourceRevision != 43 {
		t.Fatalf("status revisions = %+v, want rejected command revision", status)
	}
}

func TestStoreRunCommandRejectedRejectsUnknownWorkspace(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{{ID: "ws-a", State: "active"}}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-a": {Workspace: appmodelcore.Workspace{ID: "ws-a", State: "active"}},
	}, nil)
	if store.ApplyRunCommandRejected(RunCommandRejectedUpdate{WorkspaceID: "ws-b", Intent: "resume", Revision: 2}) {
		t.Fatal("ApplyRunCommandRejected returned true for unknown workspace")
	}
	snapshot, status, err := store.GetSnapshot("ws-a")
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	if snapshot.RunCommand != nil {
		t.Fatalf("RunCommand = %+v, want no cross-workspace command projection", snapshot.RunCommand)
	}
	if status.SourceEventRevision != 0 || status.ProjectedSourceRevision != 0 {
		t.Fatalf("status after rejected command = %+v, want unchanged revisions", status)
	}
}

func TestManagerStartRequiresScheduler(t *testing.T) {
	manager := NewManager(ManagerOptions{
		Loader: fakeProjectionLoader{},
	})
	if err := manager.Start(context.Background()); err == nil {
		t.Fatal("Start returned nil error without scheduler")
	}
}

func TestManagerReschedulesWorkspaceRebuildWhenEventArrivesInFlight(t *testing.T) {
	ctx := context.Background()
	loader := &countingProjectionLoader{}
	var scheduled []func(context.Context)
	manager := NewManager(ManagerOptions{
		Loader: loader,
		Schedule: func(name string, fn func(context.Context)) bool {
			if name != "workspace_projection_rebuild" {
				t.Fatalf("scheduled task = %q, want workspace_projection_rebuild", name)
			}
			scheduled = append(scheduled, fn)
			return true
		},
	})

	manager.ScheduleRebuildWorkspace("ws-a")
	manager.ScheduleRebuildWorkspace("ws-a")
	if len(scheduled) != 1 {
		t.Fatalf("scheduled rebuilds = %d, want one in-flight rebuild before pending rerun", len(scheduled))
	}
	if metrics := manager.Metrics(); metrics.WorkspaceRebuildDebouncedTotal != 1 || metrics.PendingWorkspaceRebuilds != 1 {
		t.Fatalf("debounce metrics = %+v, want one debounced pending workspace rebuild", metrics)
	}

	scheduled[0](ctx)
	if len(scheduled) != 2 {
		t.Fatalf("scheduled rebuilds after first run = %d, want pending rerun", len(scheduled))
	}
	scheduled[1](ctx)

	snapshot, _, err := manager.ReadWorkspaceSnapshot("ws-a")
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if snapshot.Workspace.Title != "snapshot-2" || loader.calls != 2 {
		t.Fatalf("snapshot title/calls = %q/%d, want second rebuild to win", snapshot.Workspace.Title, loader.calls)
	}
}

func TestManagerCoalescesWorkspaceRebuildsBeforeScheduledTaskRuns(t *testing.T) {
	ctx := context.Background()
	loader := &countingProjectionLoader{}
	var scheduled []func(context.Context)
	manager := NewManager(ManagerOptions{
		Loader: loader,
		Schedule: func(name string, fn func(context.Context)) bool {
			if name != "workspace_projection_rebuild" {
				t.Fatalf("scheduled task = %q, want workspace_projection_rebuild", name)
			}
			scheduled = append(scheduled, fn)
			return true
		},
		RebuildCoalesceDelay: time.Nanosecond,
	})

	manager.ScheduleRebuildWorkspace("ws-a")
	manager.ScheduleRebuildWorkspace("ws-a")
	manager.ScheduleRebuildWorkspace("ws-a")
	if len(scheduled) != 1 {
		t.Fatalf("scheduled rebuilds = %d, want one coalesced task", len(scheduled))
	}
	if metrics := manager.Metrics(); metrics.WorkspaceRebuildDebouncedTotal != 2 || metrics.PendingWorkspaceRebuilds != 1 {
		t.Fatalf("debounce metrics = %+v, want two coalesced events and one pending scheduled workspace", metrics)
	}

	scheduled[0](ctx)
	if loader.calls != 1 {
		t.Fatalf("loader calls = %d, want one rebuild after coalescing", loader.calls)
	}
	if metrics := manager.Metrics(); metrics.PendingWorkspaceRebuilds != 0 {
		t.Fatalf("pending workspace rebuilds = %d, want drained", metrics.PendingWorkspaceRebuilds)
	}
}

func TestManagerFullRebuildDoesNotLoadWorkspaceDetails(t *testing.T) {
	ctx := context.Background()
	loader := &countingProjectionLoader{}
	manager := NewManager(ManagerOptions{
		Loader: loader,
		Schedule: func(string, func(context.Context)) bool {
			return false
		},
	})

	if err := manager.RebuildAll(ctx); err != nil {
		t.Fatalf("rebuild all: %v", err)
	}
	if loader.calls != 0 {
		t.Fatalf("detail snapshot calls after full rebuild = %d, want 0", loader.calls)
	}
	items, status, err := manager.ListWorkspaces()
	if err != nil {
		t.Fatalf("list workspaces: %v", err)
	}
	if status.State != StateReady || len(items) != 1 || items[0].ID != "ws-a" {
		t.Fatalf("items/status = %+v/%+v, want ready overview projection", items, status)
	}
	if _, _, err := manager.ReadWorkspaceSnapshot("ws-a"); err == nil {
		t.Fatal("memory snapshot read succeeded before detail load, want detail not ready")
	}
	snapshot, _, err := manager.ReadWorkspaceSnapshotWithContext(ctx, "ws-a")
	if err != nil {
		t.Fatalf("on-demand snapshot read: %v", err)
	}
	if loader.calls != 1 || snapshot.Workspace.Title != "snapshot-1" {
		t.Fatalf("snapshot title/calls = %q/%d, want single on-demand detail load", snapshot.Workspace.Title, loader.calls)
	}
}

func TestManagerSchedulesIndependentWorkspaceRebuildsWithPerWorkspaceDebounce(t *testing.T) {
	ctx := context.Background()
	loader := &workspaceCountingProjectionLoader{}
	var scheduled []func(context.Context)
	manager := NewManager(ManagerOptions{
		Loader: loader,
		Schedule: func(name string, fn func(context.Context)) bool {
			if name != "workspace_projection_rebuild" {
				t.Fatalf("scheduled task = %q, want workspace_projection_rebuild", name)
			}
			scheduled = append(scheduled, fn)
			return true
		},
	})

	manager.ScheduleRebuildWorkspace("ws-a")
	manager.ScheduleRebuildWorkspace("ws-b")
	manager.ScheduleRebuildWorkspace("ws-a")
	if len(scheduled) != 2 {
		t.Fatalf("scheduled rebuilds = %d, want one task per distinct workspace before pending rerun", len(scheduled))
	}
	if metrics := manager.Metrics(); metrics.WorkspaceRebuildDebouncedTotal != 1 || metrics.PendingWorkspaceRebuilds != 1 {
		t.Fatalf("debounce metrics = %+v, want only ws-a pending rerun", metrics)
	}

	scheduled[1](ctx)
	if got := loader.callsFor("ws-b"); got != 1 {
		t.Fatalf("ws-b rebuild calls = %d, want independent rebuild while ws-a is pending", got)
	}
	if len(scheduled) != 2 {
		t.Fatalf("scheduled rebuilds after ws-b run = %d, want no cross-workspace rerun", len(scheduled))
	}

	scheduled[0](ctx)
	if len(scheduled) != 3 {
		t.Fatalf("scheduled rebuilds after first ws-a run = %d, want pending ws-a rerun", len(scheduled))
	}
	scheduled[2](ctx)
	if got := loader.callsFor("ws-a"); got != 2 {
		t.Fatalf("ws-a rebuild calls = %d, want pending rerun", got)
	}
	if metrics := manager.Metrics(); metrics.PendingWorkspaceRebuilds != 0 {
		t.Fatalf("pending workspace rebuilds = %d, want drained", metrics.PendingWorkspaceRebuilds)
	}
	snapshotA, _, err := manager.ReadWorkspaceSnapshot("ws-a")
	if err != nil {
		t.Fatalf("read ws-a snapshot: %v", err)
	}
	snapshotB, _, err := manager.ReadWorkspaceSnapshot("ws-b")
	if err != nil {
		t.Fatalf("read ws-b snapshot: %v", err)
	}
	if snapshotA.Workspace.Title != "ws-a-snapshot-2" || snapshotB.Workspace.Title != "ws-b-snapshot-1" {
		t.Fatalf("snapshots = %q/%q, want per-workspace rebuild results", snapshotA.Workspace.Title, snapshotB.Workspace.Title)
	}
}

func TestManagerSchedulesWorkspaceRebuildAfterWorkspaceEventDuringFullRebuild(t *testing.T) {
	ctx := context.Background()
	loader := &blockingProjectionLoader{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	var (
		scheduledMu sync.Mutex
		scheduled   []func(context.Context)
	)
	manager := NewManager(ManagerOptions{
		Loader: loader,
		Schedule: func(name string, fn func(context.Context)) bool {
			scheduledMu.Lock()
			scheduled = append(scheduled, fn)
			scheduledMu.Unlock()
			return true
		},
	})

	done := make(chan error, 1)
	go func() {
		done <- manager.RebuildAll(ctx)
	}()
	<-loader.started

	if err := manager.HandleEventRecord(ctx, eventstreamevents.EventRecordData{
		Topic:       runevents.TopicRunStateChanged,
		WorkspaceID: common.WorkspaceID("ws-a"),
		SubjectType: "run",
		SubjectID:   "run-a",
		Revision:    7,
	}); err != nil {
		t.Fatalf("handle workspace event: %v", err)
	}
	close(loader.release)
	if err := <-done; err != nil {
		t.Fatalf("full rebuild: %v", err)
	}

	scheduledMu.Lock()
	if len(scheduled) != 1 {
		t.Fatalf("scheduled rebuilds = %d, want one workspace rebuild", len(scheduled))
	}
	workspaceRebuild := scheduled[0]
	scheduledMu.Unlock()
	workspaceRebuild(ctx)
	if loader.calls != 1 {
		t.Fatalf("loader list calls = %d, want only initial full rebuild", loader.calls)
	}
	snapshot, _, err := manager.ReadWorkspaceSnapshot("ws-a")
	if err != nil {
		t.Fatalf("read workspace snapshot: %v", err)
	}
	if snapshot.Workspace.Title != "snapshot-1" {
		t.Fatalf("snapshot title = %q, want snapshot-1", snapshot.Workspace.Title)
	}
}

func TestManagerSchedulesFullRebuildAfterGlobalWorkspaceEventDuringFullRebuild(t *testing.T) {
	ctx := context.Background()
	loader := &blockingProjectionLoader{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	var (
		scheduledMu sync.Mutex
		scheduled   []scheduledProjectionTask
	)
	manager := NewManager(ManagerOptions{
		Loader: loader,
		Schedule: func(name string, fn func(context.Context)) bool {
			scheduledMu.Lock()
			scheduled = append(scheduled, scheduledProjectionTask{name: name, fn: fn})
			scheduledMu.Unlock()
			return true
		},
	})

	done := make(chan error, 1)
	go func() {
		done <- manager.RebuildAll(ctx)
	}()
	<-loader.started

	if err := manager.HandleEventRecord(ctx, eventstreamevents.EventRecordData{
		Topic:       runevents.TopicRunStateChanged,
		SubjectType: "workspace",
		SubjectID:   "ws-a",
		Revision:    8,
	}); err != nil {
		t.Fatalf("handle global workspace event: %v", err)
	}
	close(loader.release)
	if err := <-done; err != nil {
		t.Fatalf("full rebuild: %v", err)
	}

	scheduledMu.Lock()
	if len(scheduled) != 1 {
		t.Fatalf("scheduled rebuilds = %d, want follow-up full rebuild only", len(scheduled))
	}
	if scheduled[0].name != "workspace_projection_rebuild_all" {
		t.Fatalf("scheduled task name = %q, want workspace_projection_rebuild_all", scheduled[0].name)
	}
	followup := scheduled[0].fn
	scheduledMu.Unlock()
	followup(ctx)
	if loader.calls != 2 {
		t.Fatalf("loader calls = %d, want initial and follow-up full rebuild", loader.calls)
	}
}

func TestManagerMetricsTracksProjectionRebuilds(t *testing.T) {
	ctx := context.Background()
	loader := &countingProjectionLoader{}
	manager := NewManager(ManagerOptions{
		Loader: loader,
		Schedule: func(string, func(context.Context)) bool {
			return false
		},
	})

	if err := manager.RebuildAll(ctx); err != nil {
		t.Fatalf("rebuild all: %v", err)
	}
	if err := manager.RebuildWorkspace(ctx, "ws-a"); err != nil {
		t.Fatalf("rebuild workspace: %v", err)
	}
	metrics := manager.Metrics()
	if metrics.FullRebuildTotal != 1 || metrics.FullRebuildFailedTotal != 0 {
		t.Fatalf("full rebuild metrics = %+v, want one successful full rebuild", metrics)
	}
	if len(metrics.WorkspaceRebuilds) != 1 || metrics.WorkspaceRebuilds[0].WorkspaceID != "ws-a" || metrics.WorkspaceRebuilds[0].RebuildTotal != 1 || metrics.WorkspaceRebuilds[0].FailedTotal != 0 {
		t.Fatalf("workspace rebuild metrics = %+v, want one successful ws-a rebuild", metrics.WorkspaceRebuilds)
	}

	droppedBefore := metrics.ScheduleDroppedTotal
	manager.ScheduleRebuildWorkspace("ws-a")
	metrics = manager.Metrics()
	if metrics.ScheduleDroppedTotal != droppedBefore+1 {
		t.Fatalf("schedule dropped total = %d, want one additional failed scheduler counted from %d", metrics.ScheduleDroppedTotal, droppedBefore)
	}
	if metrics.WorkspaceRebuildDebouncedTotal != 0 || metrics.PendingWorkspaceRebuilds != 0 {
		t.Fatalf("debounce/pending metrics = %d/%d, want no pending after schedule rejection", metrics.WorkspaceRebuildDebouncedTotal, metrics.PendingWorkspaceRebuilds)
	}
}

func TestManagerMetricsTracksPendingWorkspaceRebuilds(t *testing.T) {
	ctx := context.Background()
	loader := &blockingProjectionLoader{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	manager := NewManager(ManagerOptions{
		Loader: loader,
		Schedule: func(string, func(context.Context)) bool {
			return true
		},
	})

	done := make(chan error, 1)
	go func() {
		done <- manager.RebuildAll(ctx)
	}()
	<-loader.started
	if err := manager.HandleEventRecord(ctx, eventstreamevents.EventRecordData{
		Topic:       runevents.TopicRunStateChanged,
		WorkspaceID: common.WorkspaceID("ws-a"),
		SubjectType: "run",
		SubjectID:   "run-a",
		Revision:    9,
	}); err != nil {
		t.Fatalf("handle workspace event: %v", err)
	}
	if metrics := manager.Metrics(); metrics.PendingWorkspaceRebuilds != 1 {
		t.Fatalf("pending workspace rebuilds = %d, want 1", metrics.PendingWorkspaceRebuilds)
	}
	close(loader.release)
	if err := <-done; err != nil {
		t.Fatalf("full rebuild: %v", err)
	}
}

type scheduledProjectionTask struct {
	name string
	fn   func(context.Context)
}

func subscribeProjectionNotifications(t *testing.T, hub magicevent.Hub) <-chan workorchdevents.WorkspaceProjectionChangedData {
	t.Helper()
	const observerID = "/workorchd/consumers/application/workorchd/workspace-view-test"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, hub)
	ret := make(chan workorchdevents.WorkspaceProjectionChangedData, 16)
	observer.Subscribe(workorchdevents.TopicWorkspaceProjectionChanged, func(event magicevent.Event, result magicevent.Result) {
		if data, ok := event.Data().(workorchdevents.WorkspaceProjectionChangedData); ok {
			ret <- data
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	t.Cleanup(func() { observer.Unsubscribe(workorchdevents.TopicWorkspaceProjectionChanged) })
	return ret
}

func waitProjectionNotification(t *testing.T, items <-chan workorchdevents.WorkspaceProjectionChangedData, match func(workorchdevents.WorkspaceProjectionChangedData) bool) workorchdevents.WorkspaceProjectionChangedData {
	t.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for {
		select {
		case item := <-items:
			if match(item) {
				return item
			}
		case <-timer.C:
			t.Fatal("typed workspace projection notification not received")
		}
	}
}

func drainProjectionNotifications(items <-chan workorchdevents.WorkspaceProjectionChangedData) {
	for {
		select {
		case <-items:
		default:
			return
		}
	}
}

type fakeProjectionLoader struct {
	workspaces []appmodelcore.Workspace
	snapshots  map[string]readmodelcore.WorkspaceSnapshot
}

func (f fakeProjectionLoader) ListWorkspaces(context.Context) ([]appmodelcore.Workspace, error) {
	return append([]appmodelcore.Workspace(nil), f.workspaces...), nil
}

func (f fakeProjectionLoader) ReadWorkspace(_ context.Context, workspaceID string) (appmodelcore.Workspace, error) {
	for _, item := range f.workspaces {
		if item.ID == workspaceID {
			return item, nil
		}
	}
	return appmodelcore.Workspace{}, errors.New("workspace not found")
}

func (f fakeProjectionLoader) ReadWorkspaceSnapshot(_ context.Context, workspaceID string) (readmodelcore.WorkspaceSnapshot, error) {
	if item, ok := f.snapshots[workspaceID]; ok {
		return item, nil
	}
	return readmodelcore.WorkspaceSnapshot{}, errors.New("snapshot not found")
}

type deletedProjectionLoader struct{}

func (deletedProjectionLoader) ListWorkspaces(context.Context) ([]appmodelcore.Workspace, error) {
	return nil, nil
}

func (deletedProjectionLoader) ReadWorkspace(context.Context, string) (appmodelcore.Workspace, error) {
	return appmodelcore.Workspace{}, fmt.Errorf("workspace deleted: %w", common.ErrNotFound)
}

func (deletedProjectionLoader) ReadWorkspaceSnapshot(context.Context, string) (readmodelcore.WorkspaceSnapshot, error) {
	return readmodelcore.WorkspaceSnapshot{}, fmt.Errorf("workspace snapshot deleted: %w", common.ErrNotFound)
}

type countingProjectionLoader struct {
	calls int
}

func (f *countingProjectionLoader) ListWorkspaces(context.Context) ([]appmodelcore.Workspace, error) {
	return []appmodelcore.Workspace{{ID: "ws-a", Title: "workspace"}}, nil
}

func (f *countingProjectionLoader) ReadWorkspace(context.Context, string) (appmodelcore.Workspace, error) {
	return appmodelcore.Workspace{ID: "ws-a", Title: "workspace"}, nil
}

func (f *countingProjectionLoader) ReadWorkspaceSnapshot(context.Context, string) (readmodelcore.WorkspaceSnapshot, error) {
	f.calls++
	return readmodelcore.WorkspaceSnapshot{
		Workspace: appmodelcore.Workspace{ID: "ws-a", Title: fmt.Sprintf("snapshot-%d", f.calls)},
	}, nil
}

type workspaceCountingProjectionLoader struct {
	mu    sync.Mutex
	calls map[string]int
}

func (f *workspaceCountingProjectionLoader) ListWorkspaces(context.Context) ([]appmodelcore.Workspace, error) {
	return []appmodelcore.Workspace{
		{ID: "ws-a", Title: "workspace-a"},
		{ID: "ws-b", Title: "workspace-b"},
	}, nil
}

func (f *workspaceCountingProjectionLoader) ReadWorkspace(_ context.Context, workspaceID string) (appmodelcore.Workspace, error) {
	return appmodelcore.Workspace{ID: workspaceID, Title: workspaceID + "-workspace"}, nil
}

func (f *workspaceCountingProjectionLoader) ReadWorkspaceSnapshot(_ context.Context, workspaceID string) (readmodelcore.WorkspaceSnapshot, error) {
	f.mu.Lock()
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[workspaceID]++
	call := f.calls[workspaceID]
	f.mu.Unlock()
	return readmodelcore.WorkspaceSnapshot{
		Workspace: appmodelcore.Workspace{ID: workspaceID, Title: fmt.Sprintf("%s-snapshot-%d", workspaceID, call)},
	}, nil
}

func (f *workspaceCountingProjectionLoader) callsFor(workspaceID string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[workspaceID]
}

type blockingProjectionLoader struct {
	mu      sync.Mutex
	calls   int
	started chan struct{}
	release chan struct{}
}

func (f *blockingProjectionLoader) ListWorkspaces(context.Context) ([]appmodelcore.Workspace, error) {
	f.mu.Lock()
	f.calls++
	call := f.calls
	f.mu.Unlock()
	if call == 1 {
		close(f.started)
		<-f.release
	}
	return []appmodelcore.Workspace{{ID: "ws-a", Title: fmt.Sprintf("workspace-%d", call)}}, nil
}

func (f *blockingProjectionLoader) ReadWorkspace(context.Context, string) (appmodelcore.Workspace, error) {
	return appmodelcore.Workspace{ID: "ws-a", Title: "workspace"}, nil
}

func (f *blockingProjectionLoader) ReadWorkspaceSnapshot(context.Context, string) (readmodelcore.WorkspaceSnapshot, error) {
	f.mu.Lock()
	call := f.calls
	f.mu.Unlock()
	return readmodelcore.WorkspaceSnapshot{
		Workspace: appmodelcore.Workspace{ID: "ws-a", Title: fmt.Sprintf("snapshot-%d", call)},
	}, nil
}

func TestStoreUpsertSnapshotPersistsActivityRevisionTransactionally(t *testing.T) {
	store := NewStore()
	snapshot := readmodelcore.WorkspaceSnapshot{
		Workspace: appmodelcore.Workspace{ID: "ws-activity-store", Title: "activity store", State: "active", ActiveRunID: "run-1"},
		TaskProgress: readmodelcore.TaskProgress{
			WorkspaceID: "ws-activity-store",
			RunID:       "run-1",
			State:       "running",
			Summary:     "revision 1 | focus f1",
			Runtime: &projectioncontract.RuntimeProgress{
				StateRevision:    1,
				EvidenceSequence: 1,
				CurrentFocusID:   "focus-1",
				ConvergenceState: "need_state_change",
			},
		},
		ViewModel: appmodelcore.WorkspaceViewModel{WorkspaceID: "ws-activity-store", WorkspaceRevision: 1},
	}
	store.UpsertSnapshot(snapshot)
	feed1, ok := store.GetActivity("ws-activity-store")
	if !ok {
		t.Fatal("expected store-owned activity after first upsert")
	}
	if feed1.ActivityRevision == 0 {
		t.Fatalf("activity revision = 0, want transaction revision")
	}
	firstRevision := feed1.ActivityRevision
	if len(feed1.Items) == 0 {
		t.Fatal("expected activity items from runtime progress")
	}

	// Same content: ActivityRevision must stay.
	store.UpsertSnapshot(snapshot)
	feed2, ok := store.GetActivity("ws-activity-store")
	if !ok {
		t.Fatal("expected activity after second upsert")
	}
	if feed2.ActivityRevision != firstRevision {
		t.Fatalf("unchanged activity revision = %d, want keep %d", feed2.ActivityRevision, firstRevision)
	}

	// Content change: ActivityRevision advances to new entry revision.
	snapshot.TaskProgress.Runtime.StateRevision = 2
	snapshot.TaskProgress.Runtime.CurrentFocusID = "focus-2"
	snapshot.TaskProgress.Summary = "revision 2 | focus f2"
	store.UpsertSnapshot(snapshot)
	feed3, ok := store.GetActivity("ws-activity-store")
	if !ok {
		t.Fatal("expected activity after content change")
	}
	if feed3.ActivityRevision <= firstRevision {
		t.Fatalf("changed activity revision = %d, want > %d", feed3.ActivityRevision, firstRevision)
	}
	// Read path must only page, not regenerate revision.
	paged := readmodelprojection.PageWorkspaceActivityFeed(feed3, 1, "")
	if paged.ActivityRevision != feed3.ActivityRevision {
		t.Fatalf("paged revision = %d, want store revision %d", paged.ActivityRevision, feed3.ActivityRevision)
	}
	if len(paged.Items) != 1 {
		t.Fatalf("paged items = %d, want 1", len(paged.Items))
	}
}

// CR01: server/store restart must not produce Workspace/Activity revisions that a
// long-lived TUI with a larger pre-restart AppliedRevision would permanently reject.
func TestStoreRestartRevisionEpochAcceptableByPriorAppliedRevision(t *testing.T) {
	oldStore := NewStore()
	// Simulate a pre-epoch plain counter process that advanced far.
	oldStore.revisionEpoch = 0
	snapshot := readmodelcore.WorkspaceSnapshot{
		Workspace: appmodelcore.Workspace{ID: "ws-restart", Title: "restart", State: "active", ActiveRunID: "run-1"},
		TaskProgress: readmodelcore.TaskProgress{
			WorkspaceID: "ws-restart",
			RunID:       "run-1",
			State:       "running",
			Summary:     "revision 1 | focus f1",
			Runtime: &projectioncontract.RuntimeProgress{
				StateRevision:    1,
				EvidenceSequence: 1,
				CurrentFocusID:   "focus-1",
				ConvergenceState: "need_state_change",
			},
		},
		ViewModel: appmodelcore.WorkspaceViewModel{WorkspaceID: "ws-restart", WorkspaceRevision: 1},
	}
	// Force many advances so AppliedRevision looks large to a remote TUI.
	for i := 0; i < 50; i++ {
		snapshot.TaskProgress.Runtime.StateRevision = uint64(i + 1)
		snapshot.TaskProgress.Runtime.CurrentFocusID = fmt.Sprintf("focus-%d", i+1)
		snapshot.TaskProgress.Summary = fmt.Sprintf("revision %d", i+1)
		oldStore.UpsertSnapshot(snapshot)
	}
	oldFeed, ok := oldStore.GetActivity("ws-restart")
	if !ok {
		t.Fatal("expected activity from pre-restart store")
	}
	oldStatus := oldStore.StatusForWorkspace("ws-restart")
	if oldStatus.WorkspaceRevision == 0 || oldFeed.ActivityRevision == 0 {
		t.Fatalf("pre-restart revisions zero: workspace=%d activity=%d", oldStatus.WorkspaceRevision, oldFeed.ActivityRevision)
	}
	priorAppliedWorkspace := oldStatus.WorkspaceRevision
	priorAppliedActivity := oldFeed.ActivityRevision

	// New process: NewStore assigns a wall-clock epoch floor.
	newStore := NewStore()
	if newStore.revisionEpoch <= priorAppliedWorkspace || newStore.revisionEpoch <= priorAppliedActivity {
		// Extremely unlikely with UnixNano, but assert the contract explicitly.
		t.Fatalf("new epoch %d not above prior applied workspace=%d activity=%d", newStore.revisionEpoch, priorAppliedWorkspace, priorAppliedActivity)
	}
	// Rebuild same durable content after restart.
	rebuild := snapshot
	rebuild.TaskProgress.Runtime.StateRevision = 50
	rebuild.TaskProgress.Runtime.CurrentFocusID = "focus-50"
	rebuild.TaskProgress.Summary = "revision 50"
	newStore.UpsertSnapshot(rebuild)
	newFeed, ok := newStore.GetActivity("ws-restart")
	if !ok {
		t.Fatal("expected activity after restart rebuild")
	}
	newStatus := newStore.StatusForWorkspace("ws-restart")
	if newStatus.WorkspaceRevision <= priorAppliedWorkspace {
		t.Fatalf("post-restart workspace revision %d <= prior applied %d", newStatus.WorkspaceRevision, priorAppliedWorkspace)
	}
	if newFeed.ActivityRevision <= priorAppliedActivity {
		t.Fatalf("post-restart activity revision %d <= prior applied %d", newFeed.ActivityRevision, priorAppliedActivity)
	}
	// Simulate TUI runtimeViewRevisionAcceptable gate: non-zero applied rejects lower.
	if newStatus.WorkspaceRevision < priorAppliedWorkspace || newFeed.ActivityRevision < priorAppliedActivity {
		t.Fatalf("restart revisions not acceptable: workspace %d/%d activity %d/%d", newStatus.WorkspaceRevision, priorAppliedWorkspace, newFeed.ActivityRevision, priorAppliedActivity)
	}
	// Activity content recovered (not lost).
	if len(newFeed.Items) == 0 {
		t.Fatal("expected activity items after restart rebuild")
	}
}

// FR03/CR05: store cap must persist Truncated=true with empty NextCursor (dropped
// history is not pageable). Read paging of retained items still terminates, and
// Truncated/NextCursor participate in content equality so cap transitions advance
// ActivityRevision.
func TestStoreActivityCapPreservesTruncationFacts(t *testing.T) {
	store := NewStore()
	const overCap = 250
	records := make([]projectioncontract.EventRecord, 0, overCap)
	for i := 0; i < overCap; i++ {
		records = append(records, projectioncontract.EventRecord{
			ID:          fmt.Sprintf("evt-%04d", i),
			Topic:       "run.state_changed",
			SubjectType: "run",
			WorkspaceID: "ws-cap",
			RunID:       "run-1",
			Revision:    int64(i + 1),
			OccurredAt:  fmt.Sprintf("2026-07-15T00:00:%02dZ", i%60),
		})
	}
	snapshot := readmodelcore.WorkspaceSnapshot{
		Workspace:    appmodelcore.Workspace{ID: "ws-cap", Title: "cap", State: "active", ActiveRunID: "run-1"},
		EventRecords: records,
		ViewModel:    appmodelcore.WorkspaceViewModel{WorkspaceID: "ws-cap", WorkspaceRevision: 1},
	}
	store.UpsertSnapshot(snapshot)
	feed, ok := store.GetActivity("ws-cap")
	if !ok {
		t.Fatal("expected store-owned activity")
	}
	if !feed.Truncated {
		t.Fatal("expected store-level Truncated=true when items exceed cap")
	}
	if strings.TrimSpace(feed.NextCursor) != "" {
		t.Fatalf("store-cap NextCursor = %q, want empty (not pageable)", feed.NextCursor)
	}
	if len(feed.Items) > 200 {
		t.Fatalf("stored items = %d, want <= 200", len(feed.Items))
	}
	firstRev := feed.ActivityRevision

	// Final page of retained items still reports Truncated; NextCursor empty terminates.
	fullPage := readmodelprojection.PageWorkspaceActivityFeed(feed, 200, "")
	if !fullPage.Truncated {
		t.Fatal("final page of store-truncated feed must keep Truncated=true")
	}
	if strings.TrimSpace(fullPage.NextCursor) != "" {
		t.Fatalf("final page NextCursor = %q, want empty termination", fullPage.NextCursor)
	}

	// Read-limit paging of retained items advances then terminates.
	page1 := readmodelprojection.PageWorkspaceActivityFeed(feed, 10, "")
	if !page1.Truncated || page1.NextCursor == "" || len(page1.Items) != 10 {
		t.Fatalf("page1 truncated=%v cursor=%q items=%d", page1.Truncated, page1.NextCursor, len(page1.Items))
	}
	cursor := page1.NextCursor
	seenEmpty := false
	for hops := 0; hops < 40; hops++ {
		last := readmodelprojection.PageWorkspaceActivityFeed(feed, 10, cursor)
		if last.NextCursor == "" {
			// Terminal page: either empty (unknown/stale cursor) or last retained page.
			if last.Truncated != true && len(last.Items) > 0 {
				// retained last page under store-cap must keep Truncated
				t.Fatalf("terminal retained page Truncated=false items=%d", len(last.Items))
			}
			seenEmpty = true
			break
		}
		cursor = last.NextCursor
	}
	if !seenEmpty {
		t.Fatal("paging did not terminate with empty NextCursor")
	}
	// Unknown cursor terminates with empty page (no full-feed replay).
	unknown := readmodelprojection.PageWorkspaceActivityFeed(feed, 10, "not-a-real-cursor")
	if len(unknown.Items) != 0 {
		t.Fatalf("unknown cursor items = %d, want 0", len(unknown.Items))
	}

	// Cap transition 200→201 with identical retained window still advances revision
	// because Truncated/NextCursor participate in content equality.
	under := snapshot
	under.EventRecords = records[:200]
	store2 := NewStore()
	store2.UpsertSnapshot(under)
	feedUnder, _ := store2.GetActivity("ws-cap")
	if feedUnder.Truncated {
		t.Fatal("200 items must not set store Truncated")
	}
	revUnder := feedUnder.ActivityRevision
	store2.UpsertSnapshot(snapshot) // 250 items → cap
	feedOver, _ := store2.GetActivity("ws-cap")
	if !feedOver.Truncated {
		t.Fatal("expected Truncated after cap transition")
	}
	if feedOver.ActivityRevision <= revUnder {
		t.Fatalf("cap transition activity revision %d, want > %d", feedOver.ActivityRevision, revUnder)
	}
	_ = firstRev
}

// FR07: NewStoreWithRevisionFloor and highWaterMark keep revisions monotonic
// under clock skew / future epoch / in-process reassignment.
func TestStoreRevisionFloorAndHighWaterMarkMonotonic(t *testing.T) {
	// Simulate old process with high wall-clock epoch.
	old := NewStoreWithRevisionFloor(1_000_000)
	snap := readmodelcore.WorkspaceSnapshot{
		Workspace: appmodelcore.Workspace{ID: "ws-floor", Title: "floor", State: "active", ActiveRunID: "run-1"},
		TaskProgress: readmodelcore.TaskProgress{
			WorkspaceID: "ws-floor", RunID: "run-1", State: "running", Summary: "r1",
			Runtime: &projectioncontract.RuntimeProgress{StateRevision: 1, CurrentFocusID: "f1", ConvergenceState: "need_state_change"},
		},
		ViewModel: appmodelcore.WorkspaceViewModel{WorkspaceID: "ws-floor", WorkspaceRevision: 1},
	}
	old.UpsertSnapshot(snap)
	oldFeed, _ := old.GetActivity("ws-floor")
	oldStatus := old.StatusForWorkspace("ws-floor")
	if oldFeed.ActivityRevision < 1_000_000 || oldStatus.WorkspaceRevision < 1_000_000 {
		t.Fatalf("old revisions too small: activity=%d workspace=%d", oldFeed.ActivityRevision, oldStatus.WorkspaceRevision)
	}

	// Clock skew: new process epoch lower than old high-water. Seed floor from
	// durable observation (previous applied activity/workspace revision).
	newStore := NewStoreWithRevisionFloor(500) // skewed clock
	newStore.SeedRevisionFloor(oldStatus.WorkspaceRevision + 1)
	snap.TaskProgress.Runtime.StateRevision = 2
	snap.TaskProgress.Runtime.CurrentFocusID = "f2"
	snap.TaskProgress.Summary = "r2"
	newStore.UpsertSnapshot(snap)
	newFeed, _ := newStore.GetActivity("ws-floor")
	newStatus := newStore.StatusForWorkspace("ws-floor")
	if newStatus.WorkspaceRevision <= oldStatus.WorkspaceRevision {
		t.Fatalf("seeded floor workspace revision %d <= old %d", newStatus.WorkspaceRevision, oldStatus.WorkspaceRevision)
	}
	if newFeed.ActivityRevision <= oldFeed.ActivityRevision {
		t.Fatalf("seeded floor activity revision %d <= old %d", newFeed.ActivityRevision, oldFeed.ActivityRevision)
	}

	// In-process highWaterMark: successive assigns never decrease.
	prev := newStatus.WorkspaceRevision
	for i := 0; i < 5; i++ {
		snap.TaskProgress.Runtime.StateRevision = uint64(3 + i)
		snap.TaskProgress.Runtime.CurrentFocusID = fmt.Sprintf("f-%d", i)
		snap.TaskProgress.Summary = fmt.Sprintf("r-%d", i)
		newStore.UpsertSnapshot(snap)
		st := newStore.StatusForWorkspace("ws-floor")
		if st.WorkspaceRevision <= prev {
			t.Fatalf("highWaterMark violated: %d <= %d", st.WorkspaceRevision, prev)
		}
		prev = st.WorkspaceRevision
	}
}

// FR05: GlobalRevision/stream sequence is seeded from process floor so SSE
// Last-Event-ID values do not restart at small counters after process restart.
func TestStoreGlobalRevisionSharesProcessFloor(t *testing.T) {
	old := NewStoreWithRevisionFloor(5_000_000)
	snap := readmodelcore.WorkspaceSnapshot{
		Workspace: appmodelcore.Workspace{ID: "ws-stream", Title: "stream", State: "active"},
		ViewModel: appmodelcore.WorkspaceViewModel{WorkspaceID: "ws-stream"},
	}
	old.UpsertSnapshot(snap)
	oldStatus := old.Status()
	if oldStatus.GlobalRevision < 5_000_000 {
		t.Fatalf("old global revision %d, want >= floor", oldStatus.GlobalRevision)
	}
	// New process with lower wall clock, then seed floor from observed stream.
	neu := NewStoreWithRevisionFloor(100)
	neu.SeedRevisionFloor(oldStatus.GlobalRevision + 1)
	neu.UpsertSnapshot(snap)
	newStatus := neu.Status()
	if newStatus.GlobalRevision <= oldStatus.GlobalRevision {
		t.Fatalf("post-restart global revision %d <= old %d", newStatus.GlobalRevision, oldStatus.GlobalRevision)
	}
}

// FR05: generations, unlike sequence counters, are explicitly incomparable
// across store processes. Every Store must publish a non-empty fresh token so
// clients can safely accept a post-restart RuntimeView after clock rollback.
func TestStoreRestartCreatesDistinctNonEmptyStreamGeneration(t *testing.T) {
	old := NewStoreWithRevisionFloor(10_000)
	neu := NewStoreWithRevisionFloor(1) // intentionally models a rolled-back clock
	oldGeneration := old.Status().StreamGeneration
	newGeneration := neu.Status().StreamGeneration
	if oldGeneration == "" || newGeneration == "" {
		t.Fatalf("stream generation must be non-empty: old=%q new=%q", oldGeneration, newGeneration)
	}
	if oldGeneration == newGeneration {
		t.Fatalf("store restart reused stream generation %q", oldGeneration)
	}
}

// RVC-13: current-generation failure set drives health; successful rebuild recovers ready.
func TestStoreProjectionGenerationRecovery(t *testing.T) {
	store := NewStore()
	generation := store.Status().StreamGeneration
	if generation == "" {
		t.Fatal("empty stream generation")
	}

	// Seed a healthy workspace, then mark degraded via rebuild failure.
	store.ReplaceAll([]appmodelcore.Workspace{{ID: "ws-a", State: "active"}}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-a": {
			Workspace:    appmodelcore.Workspace{ID: "ws-a", State: "active", ActiveRunID: "run-a"},
			TaskProgress: readmodelcore.TaskProgress{RunID: "run-a", State: "running"},
		},
	}, nil)
	ready := store.Status()
	if ready.State != StateReady || ready.Degraded || len(ready.FailedWorkspaceIds) != 0 {
		t.Fatalf("ready status = %+v", ready)
	}

	store.MarkWorkspaceDegraded("ws-a", errors.New("snapshot rebuild failed"))
	degraded := store.Status()
	if degraded.State != StateDegraded || !degraded.Degraded {
		t.Fatalf("after MarkWorkspaceDegraded status = %+v, want degraded", degraded)
	}
	if degraded.StreamGeneration != generation {
		t.Fatalf("streamGeneration changed unexpectedly: %q -> %q", generation, degraded.StreamGeneration)
	}
	if len(degraded.FailedWorkspaceIds) != 1 || degraded.FailedWorkspaceIds[0] != "ws-a" {
		t.Fatalf("failedWorkspaceIds = %v", degraded.FailedWorkspaceIds)
	}
	if degraded.Error == "" || degraded.DegradedSince == "" {
		t.Fatalf("degraded status missing error/since: %+v", degraded)
	}
	// Metrics failure counters (if any) must not be required for degraded.
	// Successful rebuild of the same workspace recovers ready for current generation.
	store.UpsertSnapshot(readmodelcore.WorkspaceSnapshot{
		Workspace:    appmodelcore.Workspace{ID: "ws-a", State: "active", ActiveRunID: "run-a"},
		TaskProgress: readmodelcore.TaskProgress{RunID: "run-a", State: "running"},
	})
	recovered := store.Status()
	if recovered.State != StateReady || recovered.Degraded {
		t.Fatalf("after successful rebuild status = %+v, want ready", recovered)
	}
	if len(recovered.FailedWorkspaceIds) != 0 || recovered.Error != "" || recovered.DegradedSince != "" {
		t.Fatalf("recovered status still reports failure: %+v", recovered)
	}
	if recovered.StreamGeneration != generation {
		t.Fatalf("streamGeneration drifted: %q -> %q", generation, recovered.StreamGeneration)
	}
}

// RVC-13: deleted/retired workspace failures must not pollute current generation health.
func TestStoreDeletedWorkspaceClearsGenerationFailure(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{
		{ID: "ws-a", State: "active"},
		{ID: "ws-b", State: "active"},
	}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-a": {Workspace: appmodelcore.Workspace{ID: "ws-a", State: "active"}},
		"ws-b": {Workspace: appmodelcore.Workspace{ID: "ws-b", State: "active"}},
	}, nil)

	store.MarkWorkspaceDegraded("ws-b", errors.New("ws-b rebuild failed"))
	degraded := store.Status()
	if degraded.State != StateDegraded || len(degraded.FailedWorkspaceIds) != 1 || degraded.FailedWorkspaceIds[0] != "ws-b" {
		t.Fatalf("status = %+v, want degraded only on ws-b", degraded)
	}

	store.DeleteWorkspace("ws-b")
	afterDelete := store.Status()
	if afterDelete.State != StateReady || afterDelete.Degraded {
		t.Fatalf("after delete status = %+v, want ready", afterDelete)
	}
	if len(afterDelete.FailedWorkspaceIds) != 0 {
		t.Fatalf("failedWorkspaceIds after delete = %v", afterDelete.FailedWorkspaceIds)
	}

	// Full ReplaceAll success clears any residual failure pollution.
	store.MarkWorkspaceDegraded("ws-a", errors.New("ws-a rebuild failed"))
	if store.Status().State != StateDegraded {
		t.Fatalf("expected degraded after MarkWorkspaceDegraded, got %+v", store.Status())
	}
	store.ReplaceAll([]appmodelcore.Workspace{
		{ID: "ws-a", State: "active"},
	}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-a": {Workspace: appmodelcore.Workspace{ID: "ws-a", State: "active"}},
	}, nil)
	cleared := store.Status()
	if cleared.State != StateReady || cleared.Degraded || len(cleared.FailedWorkspaceIds) != 0 {
		t.Fatalf("after ReplaceAll success status = %+v, want ready", cleared)
	}
}

func TestStoreDeleteWorkspaceDropsRetiredSourceWatermark(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{{ID: "ws-live", State: "active"}, {ID: "ws-retired", State: "active"}}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-live":    {Workspace: appmodelcore.Workspace{ID: "ws-live", State: "active"}},
		"ws-retired": {Workspace: appmodelcore.Workspace{ID: "ws-retired", State: "active"}},
	}, nil)
	store.ObserveSourceHighWatermark("ws-live", 60)
	store.UpsertSnapshot(readmodelcore.WorkspaceSnapshot{Workspace: appmodelcore.Workspace{ID: "ws-live", State: "active"}})
	store.ObserveSourceHighWatermark("ws-retired", 76)
	before := store.Status()
	if before.SourceEventRevision != 76 || before.ProjectedSourceRevision != 60 {
		t.Fatalf("before delete status = %+v, want retained workspace projected at 60 and retired source at 76", before)
	}

	store.DeleteWorkspace("ws-retired")
	after := store.Status()
	if after.SourceEventRevision != 60 || after.ProjectedSourceRevision != 60 {
		t.Fatalf("after delete status = %+v, want active workspace watermarks to converge at 60", after)
	}
}

func TestManagerRebuildDeletedWorkspaceConvergesWithoutDegradingReadiness(t *testing.T) {
	manager := NewManager(ManagerOptions{Loader: deletedProjectionLoader{}})
	manager.store.UpsertWorkspace(appmodelcore.Workspace{ID: "ws-deleted", State: "active"})

	if err := manager.RebuildWorkspace(context.Background(), "ws-deleted"); err != nil {
		t.Fatalf("RebuildWorkspace deleted workspace error = %v, want nil", err)
	}
	items, status, err := manager.ListWorkspaces()
	if err != nil {
		t.Fatalf("ListWorkspaces after deleted rebuild error = %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("projected workspaces = %+v, want deleted workspace removed", items)
	}
	if status.State != StateReady || status.Degraded || len(status.FailedWorkspaceIds) != 0 {
		t.Fatalf("status after deleted rebuild = %+v, want ready without failed workspace", status)
	}
	for _, metric := range manager.Metrics().WorkspaceRebuilds {
		if metric.WorkspaceID == "ws-deleted" && metric.FailedTotal != 0 {
			t.Fatalf("deleted workspace rebuild failure count = %d, want 0", metric.FailedTotal)
		}
	}
}

// RVC-13: process restart creates a new generation; old failures are not copied.
func TestStoreRestartDoesNotCarryGenerationFailures(t *testing.T) {
	old := NewStore()
	old.ReplaceAll([]appmodelcore.Workspace{{ID: "ws-a", State: "active"}}, nil, errors.New("old generation failure"))
	oldStatus := old.Status()
	if oldStatus.State != StateDegraded || len(oldStatus.FailedWorkspaceIds) == 0 {
		t.Fatalf("old status = %+v, want degraded with failures", oldStatus)
	}

	// New process: fresh Store has empty failure set and distinct generation.
	neu := NewStore()
	newStatus := neu.Status()
	if newStatus.StreamGeneration == "" || newStatus.StreamGeneration == oldStatus.StreamGeneration {
		t.Fatalf("new generation = %q, old = %q", newStatus.StreamGeneration, oldStatus.StreamGeneration)
	}
	if newStatus.State == StateDegraded || newStatus.Degraded || len(newStatus.FailedWorkspaceIds) != 0 {
		t.Fatalf("new store inherited failures: %+v", newStatus)
	}
}

// RVC-13: metrics failure totals live on Manager and never decide Store ready/degraded.
func TestManagerMetricsDoNotDriveProjectionHealth(t *testing.T) {
	manager := NewManager(ManagerOptions{})
	store := manager.store
	store.ReplaceAll([]appmodelcore.Workspace{{ID: "ws-a", State: "active"}}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-a": {Workspace: appmodelcore.Workspace{ID: "ws-a", State: "active"}},
	}, nil)

	// Accumulate historical failure metrics without a current generation failure.
	manager.mu.Lock()
	manager.metrics.fullRebuildFailedTotal = 99
	if manager.metrics.workspaceRebuilds == nil {
		manager.metrics.workspaceRebuilds = map[string]*workspaceRebuildMetrics{}
	}
	manager.metrics.workspaceRebuilds["ws-a"] = &workspaceRebuildMetrics{failedTotal: 12, rebuildTotal: 12}
	manager.mu.Unlock()

	status := manager.Status()
	if status.State != StateReady || status.Degraded || len(status.FailedWorkspaceIds) != 0 {
		t.Fatalf("metrics alone drove degraded: %+v", status)
	}
	metrics := manager.Metrics()
	if metrics.FullRebuildFailedTotal != 99 {
		t.Fatalf("metrics not preserved: %+v", metrics)
	}
	// Current-generation failure still drives degraded independently of metrics.
	store.MarkWorkspaceDegraded("ws-a", errors.New("current rebuild failed"))
	degraded := manager.Status()
	if degraded.State != StateDegraded || !degraded.Degraded {
		t.Fatalf("current failure did not degrade: %+v", degraded)
	}
	// Successful rebuild recovers even while historical metrics remain high.
	store.UpsertSnapshot(readmodelcore.WorkspaceSnapshot{
		Workspace: appmodelcore.Workspace{ID: "ws-a", State: "active"},
	})
	recovered := manager.Status()
	if recovered.State != StateReady || recovered.Degraded {
		t.Fatalf("recovery failed while metrics high: %+v", recovered)
	}
	if manager.Metrics().FullRebuildFailedTotal != 99 {
		t.Fatalf("metrics cleared unexpectedly: %+v", manager.Metrics())
	}
}
