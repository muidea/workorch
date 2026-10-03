package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceviewcore"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

// 使用真实 owner、loader、projection Store 和 HTTP；在 EventRecord 边界注入通知，
// 不通过直接 RebuildWorkspace 掩盖事件失效与后台重建链路。
func TestAuthoringSummaryEventRecordRefreshesHTTPListAndDetail(t *testing.T) {
	server := newTestHTTPServer(t, Config{Backend: "file", DataRoot: t.TempDir()})
	handler := server.http.Handler
	created := createWorkspaceForHTTP(t, handler, "摘要刷新", "workspaces/authoring-summary")
	ctx := context.Background()
	hub := testStateCommandHub(server.state)
	send := func(topic string, command any) any {
		t.Helper()
		result := hub.Send(magicevent.NewEventWithContext(topic, "application/workorchd", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command))
		if result == nil {
			t.Fatal("owner command returned no result")
		}
		value, resultErr := result.Get()
		if resultErr != nil {
			t.Fatal(resultErr)
		}
		return value
	}
	query := workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: common.WorkspaceID(created.ID)}
	view := send(workspaceevents.TopicWorkspaceDefinitionAuthoringQuery, query).(workspaceevents.QueryDefinitionAuthoringResult)
	if err := view.ValidateFor(query); err != nil {
		t.Fatal(err)
	}
	if view.Record == nil {
		t.Fatal("missing persisted authoring record")
	}
	readDetail := func() workspaceResponse {
		t.Helper()
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/"+created.ID, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("detail status %d: %s", recorder.Code, recorder.Body.String())
		}
		var response workspaceResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		return response
	}
	initial := readDetail().Workspace.AuthoringSummary
	if initial == nil || initial.State != "draft" {
		t.Fatalf("initial summary = %+v", initial)
	}
	reserve := workspaceevents.ReserveDefinitionDerivationCommand{
		WorkspaceID: query.WorkspaceID, WorkspaceRevision: view.WorkspaceRevision,
		InputHash: view.Record.InputHash, RequestID: "derive/" + view.Record.InputHash,
		ResourceSnapshotHash: workspace.DefinitionAuthoringResourcesHash(nil), ProviderBindingHash: "fixture-provider",
	}
	reserved := send(workspaceevents.TopicWorkspaceDerivationReserve, reserve).(workspaceevents.DefinitionDerivationRequestResult)
	if err := reserved.ValidateReserve(reserve); err != nil {
		t.Fatal(err)
	}
	complete := workspaceevents.CompleteDefinitionDerivationCommand{WorkspaceID: query.WorkspaceID, PermitID: reserved.Request.PermitID, FailureCode: "invalid_proposal"}
	completed := send(workspaceevents.TopicWorkspaceDerivationComplete, complete).(workspaceevents.DefinitionDerivationRequestResult)
	if err := completed.ValidateCompletion(complete); err != nil {
		t.Fatal(err)
	}
	manager := server.state.workspaceProjection.(*workspaceviewcore.Manager)
	if err := manager.HandleEventRecord(ctx, eventstreamevents.EventRecordData{
		Topic: workspaceevents.TopicWorkspaceStateChanged, SubjectType: "workspace", SubjectID: created.ID, WorkspaceID: query.WorkspaceID,
	}); err != nil {
		t.Fatal(err)
	}
	waitForCondition(t, 3*time.Second, func() bool {
		items, _, err := server.state.listProjectedWorkspaces()
		if err != nil || len(items) != 1 {
			return false
		}
		summary := items[0].AuthoringSummary
		return summary != nil && summary.State == "failed" && summary.FailurePhase == "derivation" && summary.FailureCode == "invalid_proposal"
	})
	listRecorder := httptest.NewRecorder()
	handler.ServeHTTP(listRecorder, httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces", nil))
	if listRecorder.Code != http.StatusOK {
		t.Fatalf("list status %d: %s", listRecorder.Code, listRecorder.Body.String())
	}
	var listed workspacesResponse
	if err := json.Unmarshal(listRecorder.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Workspaces) != 1 || listed.Workspaces[0].AuthoringSummary == nil {
		t.Fatalf("list lost summary: %+v", listed.Workspaces)
	}
	if *listed.Workspaces[0].AuthoringSummary != *readDetail().Workspace.AuthoringSummary {
		t.Fatal("list and detail summaries diverged after event-driven rebuild")
	}
}
