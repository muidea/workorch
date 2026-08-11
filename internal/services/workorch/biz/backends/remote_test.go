package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	port "github.com/muidea/workorch/internal/pkg/workorchport"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRemoteEventsBackendSubscribesToSSEStream(t *testing.T) {
	envelope := backendEventData{
		Topic:       workspaceevents.TopicWorkspaceStateChanged,
		WorkspaceID: "ws-remote",
		Payload:     recordpayload.WorkspacePayloadData{Title: "remote workspace", DeliverableDirectory: "/tmp/remote", State: common.WorkspaceStateActive, ActiveRunID: "run-remote"}.EventPayload().Map(),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/workorch/events/stream" {
			http.NotFound(w, r)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		data, err := json.Marshal(envelope)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, ": connected\n\n")
		_, _ = fmt.Fprintf(w, "event: workorch\n")
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}))
	defer server.Close()

	received := make(chan port.Event, 1)
	be := NewRemote(server.URL)
	unsubscribe, err := be.Events().Subscribe(context.Background(), func(_ context.Context, evt port.Event) {
		received <- evt
	})
	if err != nil {
		t.Fatalf("subscribe remote events: %v", err)
	}
	defer unsubscribe()

	deadline := time.After(2 * time.Second)
	for {
		select {
		case evt := <-received:
			if evt.Type == port.EventWorkspaceListChanged {
				continue
			}
			if evt.Type != port.EventWorkspaceStateChanged || evt.WorkspaceID != "ws-remote" {
				t.Fatalf("event = %+v, want workspace state event for ws-remote", evt)
			}
			return
		case <-deadline:
			t.Fatal("timed out waiting for remote SSE event")
		}
	}
}

func TestWorkspaceSnapshotFromClientPreservesCompleteDetachedProjection(t *testing.T) {
	input := projectioncontract.WorkspaceSnapshot{
		Workspace: projectioncontract.Workspace{ID: "ws-1"},
		Run: &projectioncontract.Run{
			ID:                 "run-1",
			SealedLLMSelection: &runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "provider-1", ModelID: "model-1"}},
		},
		Step:                 &projectioncontract.StepView{ID: "step-1"},
		Result:               &projectioncontract.Result{ID: "result-1", EvidenceRefs: []string{"evidence-1"}},
		AuthorityDiagnostics: []projectioncontract.GapDiagnostic{{Code: "gap-1"}},
		ExecutionSections: []projectioncontract.ExecutionSection{{
			ID:              "section-1",
			RelatedDoDItems: []string{"dod-1"},
		}},
		DoDs: []workorchmanagementcontract.DoD{{ID: "dod-1", Items: []string{"item-1"}}},
	}

	got := workspaceSnapshotFromClient(input)
	if got.Run == nil || got.Run.ID != "run-1" || got.Step == nil || got.Step.ID != "step-1" || got.Result == nil || got.Result.ID != "result-1" {
		t.Fatalf("snapshot identity fields lost: %+v", got)
	}
	if len(got.AuthorityDiagnostics) != 1 || len(got.ExecutionSections) != 1 || len(got.DoDs) != 1 {
		t.Fatalf("snapshot collections lost: %+v", got)
	}

	got.Run.SealedLLMSelection.Target.ModelID = "changed"
	got.Result.EvidenceRefs[0] = "changed"
	got.ExecutionSections[0].RelatedDoDItems[0] = "changed"
	got.DoDs[0].Items[0] = "changed"
	if input.Run.SealedLLMSelection.Target.ModelID != "model-1" || input.Result.EvidenceRefs[0] != "evidence-1" || input.ExecutionSections[0].RelatedDoDItems[0] != "dod-1" || input.DoDs[0].Items[0] != "item-1" {
		t.Fatal("remote snapshot conversion retained mutable input aliases")
	}
}

func TestCanonicalSchedulerDiagnosticsPreservesContinuationReadback(t *testing.T) {
	got := projectioncontract.CloneSchedulerDiagnostics(&projectioncontract.SchedulerDiagnostics{
		LatestContinuation: &projectioncontract.ContinuationDiagnostic{
			WorkspaceID: "ws-remote-readback",
			RunID:       "run-remote-readback",
			Kind:        "run_continuation",
			State:       "failed",
			Reason:      "owner_readback_not_advanced",
			Readback: &projectioncontract.ContinuationReadbackDiagnostic{
				WorkspaceID:             "ws-remote-readback",
				RunID:                   "run-remote-readback",
				Action:                  "resume",
				Found:                   true,
				OldRunState:             "blocked_on_decision",
				NewRunState:             "blocked_on_decision",
				OldBlockingTicketID:     "ticket-old",
				NewBlockingTicketID:     "ticket-old",
				Reason:                  "blocked_on_resolved_decision",
				ProjectionRevision:      11,
				SourceEventRevision:     12,
				ProjectedSourceRevision: 12,
			},
		},
	})
	if got == nil || got.LatestContinuation == nil || got.LatestContinuation.Readback == nil {
		t.Fatalf("diagnostics = %+v, want readback", got)
	}
	readback := got.LatestContinuation.Readback
	if readback.Action != "resume" ||
		readback.NewBlockingTicketID != "ticket-old" ||
		readback.Reason != "blocked_on_resolved_decision" ||
		readback.ProjectionRevision != 11 {
		t.Fatalf("readback = %+v, want preserved remote readback", readback)
	}
}

func TestRuntimeProgressFromClientPreservesPrerequisiteRefreshAudit(t *testing.T) {
	got := projectioncontract.CloneRuntimeProgress(&projectioncontract.RuntimeProgress{
		PrerequisiteRefreshState: "prerequisite_refreshing", PrerequisiteRefreshUsed: 1, PrerequisiteRefreshLimit: 3,
		AcceptedValidationEvidenceCount: 2,
		PrerequisiteInvalidations: []projectioncontract.RuntimePrerequisiteRefreshInvalidation{{
			Signature: "stale-1", Source: "execution_receipt_stale", OwnerModuleID: runtimecontract.RuntimeModuleGit,
			ResourceDomain: string(runtimecontract.RuntimeResourceRepositoryState), SubjectRef: "workspace_deliverable",
			ExpectedRevision: "repo-v1", ObservedRevision: "repo-v2", Reason: "repository changed",
		}},
	})
	if got == nil || got.PrerequisiteRefreshState != "prerequisite_refreshing" || got.PrerequisiteRefreshUsed != 1 ||
		got.PrerequisiteRefreshLimit != 3 || got.AcceptedValidationEvidenceCount != 2 || len(got.PrerequisiteInvalidations) != 1 {
		t.Fatalf("remote refresh progress = %+v", got)
	}
	invalidation := got.PrerequisiteInvalidations[0]
	if invalidation.Source != "execution_receipt_stale" || invalidation.OwnerModuleID != runtimecontract.RuntimeModuleGit ||
		invalidation.ResourceDomain != string(runtimecontract.RuntimeResourceRepositoryState) || invalidation.ObservedRevision != "repo-v2" {
		t.Fatalf("remote refresh invalidation = %+v", invalidation)
	}
}

func TestRuntimeProgressFromClientKeepsProviderFailureSeparateFromFreshness(t *testing.T) {
	got := projectioncontract.CloneRuntimeProgress(&projectioncontract.RuntimeProgress{
		FailureClass:              "provider_transport",
		FailureCode:               "deadline_exceeded",
		FailureMessage:            "POST /v1/responses: context deadline exceeded",
		PrerequisiteRefreshState:  "prerequisite_refreshing",
		PrerequisiteRefreshUsed:   1,
		PrerequisiteRefreshLimit:  3,
		PrerequisiteInvalidations: []projectioncontract.RuntimePrerequisiteRefreshInvalidation{{Source: "execution_receipt_stale", OwnerModuleID: "llm", ResourceDomain: "provider_config", SubjectRef: "ai-proxy/model-a"}},
	})
	if got == nil {
		t.Fatal("progress = nil")
	}
	if got.FailureClass != "provider_transport" || got.FailureCode != "deadline_exceeded" || !strings.Contains(got.FailureMessage, "deadline exceeded") {
		t.Fatalf("provider failure was not preserved: %+v", got)
	}
	if got.PrerequisiteRefreshState != "prerequisite_refreshing" || got.PrerequisiteRefreshUsed != 1 || len(got.PrerequisiteInvalidations) != 1 {
		t.Fatalf("freshness fact was conflated with provider failure: %+v", got)
	}
}

func TestRemoteProjectionEventsBackendRequestsProjectionStreamView(t *testing.T) {
	seenView := make(chan string, 1)
	envelope := backendEventData{
		Topic:       workorchdevents.TopicWorkspaceProjectionChanged,
		WorkspaceID: "ws-projection",
		Payload: map[string]any{
			"type":              "workspace_projection_changed",
			"workspaceId":       "ws-projection",
			"workspaceRevision": 3,
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/workorch/events/stream" {
			http.NotFound(w, r)
			return
		}
		seenView <- r.URL.Query().Get("view")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		data, err := json.Marshal(envelope)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, ": connected\n\n")
		_, _ = fmt.Fprintf(w, "event: workorch\n")
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}))
	defer server.Close()

	received := make(chan port.Event, 2)
	be := NewRemoteProjectionEvents(server.URL)
	unsubscribe, err := be.Events().Subscribe(context.Background(), func(_ context.Context, evt port.Event) {
		received <- evt
	})
	if err != nil {
		t.Fatalf("subscribe projection remote events: %v", err)
	}
	defer unsubscribe()

	select {
	case view := <-seenView:
		if view != "projection" {
			t.Fatalf("stream view = %q, want projection", view)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for projection stream request")
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case evt := <-received:
			if evt.Type == port.EventWorkspaceListChanged {
				continue
			}
			if evt.Type != port.EventWorkspaceProjectionChanged || evt.WorkspaceID != "ws-projection" {
				t.Fatalf("event = %+v, want workspace projection event for ws-projection", evt)
			}
			return
		case <-deadline:
			t.Fatal("timed out waiting for projection remote SSE event")
		}
	}
}

func TestRemoteWorkspaceListPreservesWorkspaceProjection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/workorch/workspaces" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{
			"service":"workorch",
			"projection":{"state":"ready","globalRevision":99,"sourceEventRevision":99,"projectedSourceRevision":99},
			"workspaces":[{
				"id":"ws-remote",
				"title":"Remote",
				"state":"completed",
				"projection":{"state":"ready","globalRevision":21,"workspaceRevision":7,"sourceEventRevision":12,"projectedSourceRevision":12}
			}]
		}`)
	}))
	defer server.Close()

	be := NewRemote(server.URL)
	got, err := be.Workspace().List(context.Background())
	if err != nil {
		t.Fatalf("remote workspace list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("workspaces = %+v, want one workspace", got)
	}
	projection := got[0].Projection
	if projection.GlobalRevision != 21 || projection.WorkspaceRevision != 7 || projection.SourceEventRevision != 12 || projection.ProjectedSourceRevision != 12 {
		t.Fatalf("workspace projection = %+v, want item projection without envelope global overwrite", projection)
	}
}

func TestRemoteEventsBackendReconnectsAndSignalsWorkspaceListRefresh(t *testing.T) {
	var requests atomic.Int32
	envelope := backendEventData{
		Topic:       workspaceevents.TopicWorkspaceCreated,
		WorkspaceID: "ws-created-after-reconnect",
		Payload:     recordpayload.WorkspacePayloadData{Title: "created after reconnect", DeliverableDirectory: "/tmp/reconnect", State: common.WorkspaceStateActive}.EventPayload().Map(),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/workorch/events/stream" {
			http.NotFound(w, r)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, ": connected\n\n")
		flusher.Flush()
		if requests.Add(1) == 1 {
			return
		}
		data, err := json.Marshal(envelope)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = fmt.Fprintf(w, "event: workorch\n")
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}))
	defer server.Close()

	received := make(chan port.Event, 8)
	be := NewRemote(server.URL)
	unsubscribe, err := be.Events().Subscribe(context.Background(), func(_ context.Context, evt port.Event) {
		received <- evt
	})
	if err != nil {
		t.Fatalf("subscribe remote events: %v", err)
	}
	defer unsubscribe()

	sawRefresh := false
	deadline := time.After(3 * time.Second)
	for {
		select {
		case evt := <-received:
			if evt.Type == port.EventWorkspaceListChanged {
				sawRefresh = true
				continue
			}
			if evt.Type != port.EventWorkspaceCreated || evt.WorkspaceID != "ws-created-after-reconnect" {
				t.Fatalf("event = %+v, want workspace created after reconnect", evt)
			}
			if !sawRefresh {
				t.Fatal("expected workspace list refresh signal before replayed live event")
			}
			if requests.Load() < 2 {
				t.Fatalf("requests = %d, want reconnect", requests.Load())
			}
			return
		case <-deadline:
			t.Fatal("timed out waiting for reconnected remote SSE event")
		}
	}
}

func TestRemoteEventsBackendReconnectsWithLastEventID(t *testing.T) {
	var requests atomic.Int32
	seenLastEventID := make(chan string, 2)
	first := backendEventData{
		ID:          "evt-first",
		Topic:       workspaceevents.TopicWorkspaceStateChanged,
		WorkspaceID: "ws-replay",
		Payload:     recordpayload.WorkspacePayloadData{Title: "replay", DeliverableDirectory: "/tmp/replay", State: common.WorkspaceStateActive, ActiveRunID: "run-replay"}.EventPayload().Map(),
	}
	second := backendEventData{
		ID:          "evt-second",
		Topic:       workspaceevents.TopicWorkspaceStateChanged,
		WorkspaceID: "ws-replay",
		Payload:     recordpayload.WorkspacePayloadData{Title: "replay", DeliverableDirectory: "/tmp/replay", State: common.WorkspaceStateActive, ActiveRunID: "run-replay"}.EventPayload().Map(),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/workorch/events/stream" {
			http.NotFound(w, r)
			return
		}
		lastID := r.Header.Get("Last-port.Event-ID")
		if lastID == "" {
			lastID = r.URL.Query().Get("lastEventId")
		}
		seenLastEventID <- lastID
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		envelope := first
		if requests.Add(1) > 1 {
			envelope = second
		}
		data, err := json.Marshal(envelope)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = fmt.Fprintf(w, "id: %s\n", envelope.ID)
		_, _ = fmt.Fprintf(w, "event: workorch\n")
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}))
	defer server.Close()

	received := make(chan port.Event, 8)
	be := NewRemote(server.URL)
	unsubscribe, err := be.Events().Subscribe(context.Background(), func(_ context.Context, evt port.Event) {
		received <- evt
	})
	if err != nil {
		t.Fatalf("subscribe remote events: %v", err)
	}
	defer unsubscribe()

	sawSecond := false
	deadline := time.After(3 * time.Second)
	for !sawSecond {
		select {
		case evt := <-received:
			if evt.Type == port.EventWorkspaceListChanged {
				continue
			}
			if evt.ID == "evt-second" {
				sawSecond = true
			}
		case <-deadline:
			t.Fatal("timed out waiting for replayed remote SSE event")
		}
	}
	if requests.Load() < 2 {
		t.Fatalf("requests = %d, want reconnect", requests.Load())
	}
	firstLastID := <-seenLastEventID
	secondLastID := <-seenLastEventID
	if firstLastID != "" {
		t.Fatalf("first stream lastEventId = %q, want empty", firstLastID)
	}
	if secondLastID != "evt-first" {
		t.Fatalf("reconnect lastEventId = %q, want evt-first", secondLastID)
	}
}

func TestRemoteRunStatusMapsCurrentStepFields(t *testing.T) {
	be := NewRemote("http://127.0.0.1:1")
	status, err := be.Run().Status(context.Background(), "", common.RunID("run-remote"))
	if err == nil || !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("remote run status err = %v, want invalid argument", err)
	}
	if status != nil {
		t.Fatalf("status = %+v, want nil on missing workspace scope", status)
	}
}

func TestCanonicalAcceptanceProjectionSeparatesDirectiveFromAssessmentObservation(t *testing.T) {
	got := projectioncontract.CloneAcceptanceProjection(projectioncontract.AcceptanceProjection{
		Budget: &projectioncontract.ConvergenceBudget{
			FocusID: "focus-remote", SliceID: "slice-remote", LastNoDeltaReason: "no durable effect",
			MaxProviderTransportAttempts: 3, ProviderTransportAttempts: 1,
			MaxPrerequisiteRefreshes: 2, PrerequisiteRefreshes: 1,
		},
		RepairDirective: &projectioncontract.RuntimeRepairDirective{ID: "repair-remote", FocusID: "focus-remote", RequiredAction: "need_validation", Closure: &projectioncontract.RuntimeRepairClosure{ID: "closure-remote"}},
		SemanticPlanning: &projectioncontract.RuntimeSemanticPlanning{
			WorkspaceID: "ws-remote", RunID: "run-remote", Phase: "feasibility_pending",
			Items: []projectioncontract.RuntimeSemanticPlanningItem{{CoverageWorkItemID: "cwi-remote", Status: "pending"}},
		},
		Audit: &projectioncontract.RuntimeAudit{Failure: &projectioncontract.RuntimeAuditFailure{
			Code: "definition_mismatch", OriginalDiagnostic: &runtimecontract.RuntimeOriginalDiagnostic{OwnerModuleID: "blocks/run", ObservedRevision: "4"},
		}, Assessments: []projectioncontract.RuntimeAuditAssessment{{
			ID: "assessment-remote", ClaimedDisposition: "terminal_rejection", CanonicalDisposition: "repairable_gap", Summary: "producer summary", Gap: &projectioncontract.RuntimeAuditAssessmentGap{MissingEvidenceKinds: []string{"state_change"}, Reason: "producer reason"},
		}}, CoverageBindings: []projectioncontract.RuntimeAuditCoverageBinding{{
			ID: "binding/cwi-remote", CoverageWorkItemID: "cwi-remote", TargetRefs: []string{"remote.go"},
			Proofs: []projectioncontract.RuntimeAuditCoverageProof{{EvidenceKind: "validation", ValidationRecipeID: "recipe-remote", RequiredCheckRefs: []string{"check-remote"}}},
		}}, Dispatches: []projectioncontract.RuntimeAuditDispatch{{
			ID: "dispatch-remote", AuthoritySnapshotID: "authority-remote", State: "committed",
			Operations: []projectioncontract.RuntimeAuditSealedOperation{{ID: "validate-remote", Kind: "validation/managed_session", BindingRefs: []string{"binding/cwi-remote"}, TargetRefs: []string{"remote.go"}}},
		}}, AcceptedEvidence: []projectioncontract.RuntimeAuditEvidence{{
			ID: "evidence-remote", OperationID: "validate-remote", BindingRefs: []string{"binding/cwi-remote"}, CoverageWorkItemIDs: []string{"cwi-remote"}, PassedChecks: []string{"check-remote"},
		}}, OperationReceipts: []projectioncontract.RuntimeAuditOperationReceipt{{
			ExecutionKey: "exec/ws/run/dispatch/validate-remote", OperationID: "validate-remote", Outcome: "accepted", ObservedEffectScope: "none", StableCategory: "accepted",
			OriginalDiagnostic: &runtimecontract.RuntimeOriginalDiagnostic{
				OwnerModuleID: "blocks/shell", OriginalMessage: "safe owner fact", DetailsBytes: 16385, OmittedReason: "raw_owner_payload", Redacted: true,
				FieldViolations: []runtimecontract.RuntimeFieldViolation{{Path: "parameters.summary", Forbidden: []string{"summary", "summary"}}},
			}, PassedChecks: []string{"check-remote"},
		}}},
	})
	if got.RepairDirective == nil || got.RepairDirective.RequiredAction != "need_validation" || got.RepairDirective.Closure == nil ||
		got.RepairDirective.Closure.ID != "closure-remote" || got.Audit == nil || len(got.Audit.Assessments) != 1 ||
		got.Audit.Assessments[0].ClaimedDisposition != "terminal_rejection" || got.Audit.Assessments[0].CanonicalDisposition != "repairable_gap" ||
		got.Audit.Assessments[0].Gap == nil || got.Audit.Assessments[0].Gap.Reason != "producer reason" {
		t.Fatalf("remote acceptance mapping = %+v", got)
	}
	if got.Budget == nil || got.Budget.FocusID != "focus-remote" || got.Budget.LastNoDeltaReason != "no durable effect" ||
		got.Budget.ProviderTransportAttempts != 1 || got.Budget.PrerequisiteRefreshes != 1 ||
		got.SemanticPlanning == nil || got.SemanticPlanning.WorkspaceID != "ws-remote" || len(got.SemanticPlanning.Items) != 1 {
		t.Fatalf("remote acceptance canonical projections = %+v", got)
	}
	if got.Audit.Failure == nil || got.Audit.Failure.Code != "definition_mismatch" || got.Audit.Failure.OriginalDiagnostic == nil || got.Audit.Failure.OriginalDiagnostic.ObservedRevision != "4" {
		t.Fatalf("remote failure diagnostic mapping = %+v", got.Audit.Failure)
	}
	if len(got.Audit.CoverageBindings) != 1 || got.Audit.CoverageBindings[0].ID != "binding/cwi-remote" ||
		len(got.Audit.Dispatches) != 1 || got.Audit.Dispatches[0].ID != "dispatch-remote" || len(got.Audit.Dispatches[0].Operations) != 1 ||
		got.Audit.Dispatches[0].Operations[0].BindingRefs[0] != "binding/cwi-remote" ||
		len(got.Audit.AcceptedEvidence) != 1 || got.Audit.AcceptedEvidence[0].CoverageWorkItemIDs[0] != "cwi-remote" ||
		len(got.Audit.OperationReceipts) != 1 || got.Audit.OperationReceipts[0].ExecutionKey != "exec/ws/run/dispatch/validate-remote" ||
		got.Audit.OperationReceipts[0].OriginalDiagnostic == nil || got.Audit.OperationReceipts[0].OriginalDiagnostic.OwnerModuleID != "blocks/shell" ||
		got.Audit.OperationReceipts[0].OriginalDiagnostic.OriginalMessage != "safe owner fact" || got.Audit.OperationReceipts[0].OriginalDiagnostic.DetailsBytes != 16385 ||
		len(got.Audit.OperationReceipts[0].OriginalDiagnostic.FieldViolations) != 1 || len(got.Audit.OperationReceipts[0].OriginalDiagnostic.FieldViolations[0].Forbidden) != 2 ||
		got.Audit.OperationReceipts[0].OriginalDiagnostic.OmittedReason != "raw_owner_payload" || !got.Audit.OperationReceipts[0].OriginalDiagnostic.Redacted {
		t.Fatalf("remote binding audit chain = %+v", got.Audit)
	}
}

func TestRemoteRunCommandFirstResumeSendsWorkspaceCommandMetadata(t *testing.T) {
	requestBody := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/workorch/workspaces/ws-remote/execution-activations" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		requestBody <- body
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = fmt.Fprint(w, `{"accepted":true}`)
	}))
	defer server.Close()

	be := NewRemote(server.URL)
	run, err := be.Run().Command(context.Background(), port.WorkspaceRunCommand{
		WorkspaceID: "ws-remote",
		Intent:      port.RunCommandIntentResume,
		RequestID:   "request-first-resume",
		Reason:      "user_requested",
		Source:      "test",
	})
	if err != nil {
		t.Fatalf("remote first resume command: %v", err)
	}
	if run != nil {
		t.Fatalf("run = %+v, want accepted-only nil run", run)
	}
	select {
	case body := <-requestBody:
		if body["intent"] != string(port.RunCommandIntentResume) ||
			body["requestId"] != "request-first-resume" ||
			body["reason"] != "user_requested" ||
			body["source"] != "test" {
			t.Fatalf("request body = %+v, want resume command metadata", body)
		}
	case <-time.After(time.Second):
		t.Fatal("remote server did not receive first resume request")
	}
}

func TestRemoteRunCommandResumeSendsRequestID(t *testing.T) {
	requestBody := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/workorch/workspaces/ws-remote/execution-activations" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		requestBody <- body
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = fmt.Fprint(w, `{"accepted":true}`)
	}))
	defer server.Close()

	be := NewRemote(server.URL)
	run, err := be.Run().Command(context.Background(), port.WorkspaceRunCommand{
		WorkspaceID: "ws-remote",
		Intent:      port.RunCommandIntentResume,
		RequestID:   "request-remote-resume",
		Reason:      "test_remote_resume",
		Source:      "test",
	})
	if err != nil {
		t.Fatalf("remote run command: %v", err)
	}
	if run != nil {
		t.Fatalf("run = %+v, want accepted-only nil run", run)
	}
	select {
	case body := <-requestBody:
		if body["intent"] != string(port.RunCommandIntentResume) ||
			body["requestId"] != "request-remote-resume" ||
			body["reason"] != "test_remote_resume" ||
			body["source"] != "test" {
			t.Fatalf("request body = %+v, want workspace-scoped request metadata", body)
		}
		for _, field := range []string{"llmTarget", "goalId", "doDId", "soulId", "skillIds", "mcpServerIds", "capabilityIds", "memoryIds"} {
			if _, ok := body[field]; ok {
				t.Fatalf("resume request body unexpectedly carries activation override %s: %+v", field, body)
			}
		}
	case <-time.After(time.Second):
		t.Fatal("remote server did not receive resume request")
	}
}

func TestRemoteRunCommandRejectsReplanBeforeRequest(t *testing.T) {
	for _, tc := range []struct {
		name string
		req  port.WorkspaceRunCommand
	}{
		{
			name: "replan",
			req:  port.WorkspaceRunCommand{WorkspaceID: "ws-remote", Intent: port.RunCommandIntent("replan")},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := atomic.Int64{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				http.Error(w, "unexpected request", http.StatusInternalServerError)
			}))
			defer server.Close()

			_, err := NewRemote(server.URL).Run().Command(context.Background(), tc.req)
			if !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("err = %v, want invalid argument", err)
			}
			if requests.Load() != 0 {
				t.Fatalf("requests = %d, want no remote request", requests.Load())
			}
		})
	}
}

func TestRemoteRunCommandRestartPostsWorkspaceScopedFreshActivation(t *testing.T) {
	requests := atomic.Int64{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/workorch/workspaces/ws-remote-restart/execution-activations" {
			t.Fatalf("request=%s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["intent"] != string(port.RunCommandIntentRestart) || body["runId"] != nil {
			t.Fatalf("restart body=%+v, want workspace scoped restart", body)
		}
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{"accepted": true, "intent": string(port.RunCommandIntentRestart)})
	}))
	defer server.Close()

	run, err := NewRemote(server.URL).Run().Command(context.Background(), port.WorkspaceRunCommand{
		WorkspaceID: "ws-remote-restart", Intent: port.RunCommandIntentRestart,
		RequestID: "request-remote-restart", Reason: "fresh_activation_required", Source: "tui",
	})
	if err != nil || run != nil || requests.Load() != 1 {
		t.Fatalf("run=%+v requests=%d err=%v", run, requests.Load(), err)
	}
}

func TestRemoteRunCommandRejectsUnsupportedIntent(t *testing.T) {
	requests := atomic.Int64{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	defer server.Close()

	be := NewRemote(server.URL)
	_, err := be.Run().Command(context.Background(), port.WorkspaceRunCommand{
		WorkspaceID: "ws-remote",
		Intent:      port.RunCommandIntent("unsupported"),
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("requests = %d, want no remote request for unsupported intent", requests.Load())
	}
}

func TestRemoteRunCommandRejectsHistoricalIntentWithAnyScopeBeforeRequest(t *testing.T) {
	for _, tc := range []struct {
		name string
		req  port.WorkspaceRunCommand
	}{
		{
			name: "replan without run scope",
			req: port.WorkspaceRunCommand{
				WorkspaceID: "ws-remote",
				Intent:      port.RunCommandIntent("replan"),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := atomic.Int64{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				http.Error(w, "unexpected request", http.StatusInternalServerError)
			}))
			defer server.Close()

			be := NewRemote(server.URL)
			_, err := be.Run().Command(context.Background(), tc.req)
			if !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("err = %v, want invalid argument", err)
			}
			if requests.Load() != 0 {
				t.Fatalf("requests = %d, want no remote request for historical intent", requests.Load())
			}
		})
	}
}

func assertRemoteBodyStringSlice(t *testing.T, body map[string]any, key string, want string) {
	t.Helper()
	values, ok := body[key].([]any)
	if !ok {
		t.Fatalf("request body %s = %+v, want string slice containing %q", key, body[key], want)
	}
	for _, value := range values {
		if value == want {
			return
		}
	}
	t.Fatalf("request body %s = %+v, want %q", key, body[key], want)
}

func assertRemoteBodyStringMapValue(t *testing.T, body map[string]any, key string, mapKey string, want string) {
	t.Helper()
	values, ok := body[key].(map[string]any)
	if !ok {
		t.Fatalf("request body %s = %+v, want object with %s=%q", key, body[key], mapKey, want)
	}
	if values[mapKey] != want {
		t.Fatalf("request body %s = %+v, want %s=%q", key, values, mapKey, want)
	}
}

func TestRemoteRunGetMapsDefinitionRevision(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/workorch/workspaces/ws-remote/runs/run-001" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"service":"workorch","run":{"id":"run-001","workspaceId":"ws-remote","goalId":"goal-001","doDId":"dod-001","definitionRevision":"goal:goal-001|dod:dod-001","state":"running","sealedLLMSelection":{"target":{"providerId":"sealed-provider","modelId":"sealed-model"},"reasoningEffort":"high"}}}`)
	}))
	defer server.Close()

	be := NewRemote(server.URL)
	got, err := be.Run().Get(context.Background(), "ws-remote", "run-001")
	if err != nil {
		t.Fatalf("remote run get: %v", err)
	}
	if got.DefinitionRevision != "goal:goal-001|dod:dod-001" {
		t.Fatalf("definitionRevision = %q, want mapped owner revision", got.DefinitionRevision)
	}
	if got.SealedLLMSelection == nil || got.SealedLLMSelection.Target.ProviderID != "sealed-provider" || got.SealedLLMSelection.Target.ModelID != "sealed-model" {
		t.Fatalf("sealed target = %+v, want server-sealed target", got.SealedLLMSelection)
	}
}

func TestRemoteRunListMapsDefinitionRevision(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/workorch/runs" || r.URL.Query().Get("workspaceId") != "ws-remote" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"service":"workorch","runs":[{"id":"run-001","workspaceId":"ws-remote","definitionRevision":"rev-list-001","state":"running"}]}`)
	}))
	defer server.Close()

	be := NewRemote(server.URL)
	got, err := be.Run().List(context.Background(), "ws-remote")
	if err != nil {
		t.Fatalf("remote run list: %v", err)
	}
	if len(got) != 1 || got[0].DefinitionRevision != "rev-list-001" {
		t.Fatalf("runs = %+v, want mapped definition revision", got)
	}
}

func TestRemoteRunStatusUsesWorkspaceScopedPathWhenContextCarriesWorkspace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/workorch/workspaces/ws-remote/runs/run-001/status" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"status":{"id":"run-001","workspaceId":"ws-remote","state":"running"}}`)
	}))
	defer server.Close()

	be := NewRemote(server.URL)
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-remote")
	status, err := be.Run().Status(ctx, "ws-remote", common.RunID("run-001"))
	if err != nil {
		t.Fatalf("remote run status: %v", err)
	}
	if status.WorkspaceID != "ws-remote" || status.ID != "run-001" {
		t.Fatalf("status = %+v, want scoped workspace/run", status)
	}
}

func llmTargetMatches(raw any, providerID string, modelID string) bool {
	target, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	gotProvider, _ := target["providerId"].(string)
	gotModel, _ := target["modelId"].(string)
	return gotProvider == providerID && gotModel == modelID
}

// TC10: typed projection SSE publishes connecting → live only after HTTP 2xx,
// then reconnecting on disconnect, and restores live with Last-port.Event-ID.
func TestRemoteProjectionEventsHealthAndReconnectWithLastEventID(t *testing.T) {
	var requests atomic.Int32
	seenLastEventID := make(chan string, 4)
	firstDTO := map[string]any{
		"id":             "proj-evt-1",
		"streamRevision": int64(11),
		"workspaceId":    "ws-proj-reconnect",
		"runId":          "run-1",
		"revision": map[string]any{
			"streamRevision":       int64(11),
			"workspaceRevision":    int64(5),
			"runtimeStateRevision": uint64(42),
			"evidenceSequence":     uint64(7),
			"activityRevision":     int64(9),
		},
		"changedSections": []string{"runtime_progress", "activity"},
		"projectionState": "ready",
		"updatedAt":       "2026-07-15T14:30:00Z",
	}
	secondDTO := map[string]any{
		"id":             "proj-evt-2",
		"streamRevision": int64(12),
		"workspaceId":    "ws-proj-reconnect",
		"runId":          "run-1",
		"revision": map[string]any{
			"streamRevision":       int64(12),
			"workspaceRevision":    int64(6),
			"runtimeStateRevision": uint64(43),
			"evidenceSequence":     uint64(8),
			"activityRevision":     int64(10),
		},
		"changedSections": []string{"activity"},
		"projectionState": "ready",
		"updatedAt":       "2026-07-15T14:31:00Z",
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/workorch/events/stream" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("view") != "projection" {
			http.Error(w, "want projection view", http.StatusBadRequest)
			return
		}
		lastID := r.Header.Get("Last-port.Event-ID")
		if lastID == "" {
			lastID = r.URL.Query().Get("lastEventId")
		}
		seenLastEventID <- lastID
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		dto := firstDTO
		reqN := requests.Add(1)
		if reqN > 1 {
			dto = secondDTO
		}
		data, err := json.Marshal(dto)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = fmt.Fprint(w, ": connected\n\n")
		_, _ = fmt.Fprintf(w, "id: %s\n", dto["id"])
		_, _ = fmt.Fprintf(w, "event: workorch\n")
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
		// First connection ends immediately to force reconnect.
	}))
	defer server.Close()

	statuses := make(chan port.SubscriptionStatus, 16)
	changed := make(chan projectioncontract.WorkspaceProjectionChanged, 8)
	be := NewRemoteProjectionEvents(server.URL)
	unsub, err := be.ProjectionEvents().SubscribeWorkspaceProjection(
		context.Background(),
		func(_ context.Context, info projectioncontract.WorkspaceProjectionChanged) {
			changed <- info
		},
		func(_ context.Context, status port.SubscriptionStatus) {
			statuses <- status
		},
	)
	if err != nil {
		t.Fatalf("subscribe projection events: %v", err)
	}
	defer unsub()

	// Subscribe returns before live; first status must not skip connecting.
	sawConnecting := false
	sawLive := false
	sawReconnecting := false
	sawSecondLive := false
	var firstInfo, secondInfo projectioncontract.WorkspaceProjectionChanged
	deadline := time.After(5 * time.Second)
	for !(sawSecondLive && secondInfo.ID == "proj-evt-2") {
		select {
		case status := <-statuses:
			switch status.State {
			case port.SubscriptionConnecting:
				sawConnecting = true
			case port.SubscriptionLive:
				if !sawLive {
					sawLive = true
				} else if sawReconnecting {
					sawSecondLive = true
				}
			case port.SubscriptionReconnecting:
				sawReconnecting = true
			}
		case info := <-changed:
			if info.ID == "proj-evt-1" {
				firstInfo = info
			}
			if info.ID == "proj-evt-2" {
				secondInfo = info
			}
		case <-deadline:
			t.Fatalf("timed out: connecting=%v live=%v reconnecting=%v secondLive=%v first=%+v second=%+v requests=%d",
				sawConnecting, sawLive, sawReconnecting, sawSecondLive, firstInfo, secondInfo, requests.Load())
		}
	}
	if !sawConnecting {
		t.Fatal("expected connecting status before live")
	}
	if !sawLive {
		t.Fatal("expected live after SSE connected")
	}
	if !sawReconnecting {
		t.Fatal("expected reconnecting after first stream ends")
	}
	if firstInfo.Revision.RuntimeStateRevision != 42 || firstInfo.Revision.ActivityRevision != 9 {
		t.Fatalf("first typed notification = %+v, want runtime/activity revisions", firstInfo)
	}
	if len(firstInfo.ChangedSections) != 2 {
		t.Fatalf("first ChangedSections = %v, want runtime_progress+activity", firstInfo.ChangedSections)
	}
	if secondInfo.Revision.RuntimeStateRevision != 43 || secondInfo.Revision.ActivityRevision != 10 {
		t.Fatalf("second typed notification = %+v, want advanced revisions", secondInfo)
	}
	if requests.Load() < 2 {
		t.Fatalf("requests = %d, want reconnect", requests.Load())
	}
	firstLastID := <-seenLastEventID
	secondLastID := <-seenLastEventID
	if firstLastID != "" {
		t.Fatalf("first stream Last-port.Event-ID = %q, want empty", firstLastID)
	}
	if secondLastID != "proj-evt-1" {
		t.Fatalf("reconnect Last-port.Event-ID = %q, want proj-evt-1", secondLastID)
	}
}

// TC10: degraded status on connection failure; Subscribe itself does not report live.
func TestRemoteProjectionEventsDegradedOnConnectFailure(t *testing.T) {
	// Closed listener forces dial error.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	addr := server.URL
	server.Close()

	statuses := make(chan port.SubscriptionStatus, 8)
	be := NewRemoteProjectionEvents(addr)
	unsub, err := be.ProjectionEvents().SubscribeWorkspaceProjection(
		context.Background(),
		func(context.Context, projectioncontract.WorkspaceProjectionChanged) {},
		func(_ context.Context, status port.SubscriptionStatus) {
			statuses <- status
		},
	)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer unsub()

	deadline := time.After(2 * time.Second)
	sawConnecting := false
	for {
		select {
		case status := <-statuses:
			if status.State == port.SubscriptionConnecting {
				sawConnecting = true
				continue
			}
			if status.State == port.SubscriptionLive {
				t.Fatal("must not report live when SSE cannot connect")
			}
			if status.State == port.SubscriptionDegraded || status.State == port.SubscriptionReconnecting {
				if !sawConnecting {
					t.Fatal("expected connecting before degraded/reconnecting")
				}
				if strings.TrimSpace(status.LastError) == "" {
					t.Fatalf("status = %+v, want LastError", status)
				}
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for degraded/reconnecting status")
		}
	}
}
