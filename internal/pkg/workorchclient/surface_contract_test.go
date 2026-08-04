package workorchclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func TestClientRunRequestsUseWorkspaceScopedPaths(t *testing.T) {
	const workspaceID = "ws-client-scope"
	const runID = "run-client-scope"

	tests := []struct {
		name      string
		method    string
		path      string
		query     string
		bodyKey   string
		bodyValue string
		call      func(context.Context, *Client) error
	}{
		{
			name:   "get run",
			method: http.MethodGet,
			path:   "/api/v1/workorch/workspaces/ws-client-scope/runs/run-client-scope",
			call: func(ctx context.Context, client *Client) error {
				_, err := client.GetRun(ctx, workspaceID, runID)
				return err
			},
		},
		{
			name:   "status wait query",
			method: http.MethodGet,
			path:   "/api/v1/workorch/workspaces/ws-client-scope/runs/run-client-scope/status",
			query:  "waitState=running&waitTimeoutMillis=2500",
			call: func(ctx context.Context, client *Client) error {
				_, err := client.GetRunStatus(ctx, workspaceID, runID, "running", 2500)
				return err
			},
		},
		{
			name:   "snapshot",
			method: http.MethodGet,
			path:   "/api/v1/workorch/workspaces/ws-client-scope/runs/run-client-scope/snapshot",
			call: func(ctx context.Context, client *Client) error {
				_, err := client.GetRunSnapshot(ctx, workspaceID, runID)
				return err
			},
		},
		{
			name:   "observability",
			method: http.MethodGet,
			path:   "/api/v1/workorch/workspaces/ws-client-scope/runs/run-client-scope/observability",
			call: func(ctx context.Context, client *Client) error {
				_, err := client.GetRunObservability(ctx, workspaceID, runID)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != tt.method {
					t.Fatalf("method = %s, want %s", r.Method, tt.method)
				}
				if r.URL.Path != tt.path {
					t.Fatalf("path = %s, want %s", r.URL.Path, tt.path)
				}
				if r.URL.RawQuery != tt.query {
					t.Fatalf("query = %q, want %q", r.URL.RawQuery, tt.query)
				}
				if tt.bodyKey != "" {
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Fatalf("decode request body: %v", err)
					}
					if body[tt.bodyKey] != tt.bodyValue {
						t.Fatalf("request body[%q] = %v, want %q", tt.bodyKey, body[tt.bodyKey], tt.bodyValue)
					}
				}
				w.Header().Set("Content-Type", "application/json")
				switch {
				case tt.name == "status wait query":
					_, _ = w.Write([]byte(`{"status":{"run":{"id":"run-client-scope","workspaceId":"ws-client-scope","state":"running"}}}`))
				case tt.name == "snapshot":
					_, _ = w.Write([]byte(`{"snapshot":{"run":{"id":"run-client-scope","workspaceId":"ws-client-scope","state":"running"}}}`))
				case tt.name == "observability":
					_, _ = w.Write([]byte(`{"observability":{"run":{"id":"run-client-scope","workspaceId":"ws-client-scope","state":"running"}}}`))
				default:
					_, _ = w.Write([]byte(`{"run":{"id":"run-client-scope","workspaceId":"ws-client-scope","state":"running"}}`))
				}
			}))
			defer server.Close()

			if err := tt.call(context.Background(), New(server.URL)); err != nil {
				t.Fatalf("client call: %v", err)
			}
		})
	}
}

func TestClientRunMutationRequiresWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	client := New("http://127.0.0.1")
	if _, err := client.GetRun(ctx, "", "run-1"); err == nil {
		t.Fatal("GetRun without workspaceID err = nil, want error")
	}
	if _, err := client.GetRunStatus(ctx, "", "run-1", "", 0); err == nil {
		t.Fatal("GetRunStatus without workspaceID err = nil, want error")
	}
	if _, err := client.GetRunSnapshot(ctx, "", "run-1"); err == nil {
		t.Fatal("GetRunSnapshot without workspaceID err = nil, want error")
	}
	if _, err := client.GetRunObservability(ctx, "", "run-1"); err == nil {
		t.Fatal("GetRunObservability without workspaceID err = nil, want error")
	}
}

func TestClientGovernanceObjectOperationsRequireWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	client := New("http://127.0.0.1")
	for _, tc := range []struct {
		name string
		call func() error
	}{
		{name: "get decision", call: func() error { _, err := client.GetDecision(ctx, "", "ticket-1"); return err }},
		{name: "resolve decision", call: func() error {
			_, err := client.ResolveDecision(ctx, "", "ticket-1", "accept", workorcheventpayload.Payload{})
			return err
		}},
		{name: "batch resolve decisions", call: func() error {
			_, err := client.BatchResolveDecisions(ctx, "", []string{"ticket-1"}, "accept", workorcheventpayload.Payload{})
			return err
		}},
		{name: "get approval", call: func() error { _, err := client.GetApproval(ctx, "", "approval-1"); return err }},
		{name: "approve approval", call: func() error { _, err := client.Approve(ctx, "", "approval-1"); return err }},
		{name: "reject approval", call: func() error { _, err := client.Reject(ctx, "", "approval-1"); return err }},
		{name: "batch resolve approvals", call: func() error {
			_, err := client.BatchResolveApprovals(ctx, "", []string{"approval-1"}, "approve")
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); err == nil {
				t.Fatal("err = nil, want workspace scope validation error")
			}
		})
	}
}

func TestClientListEventsUsesExplicitPayloadQuery(t *testing.T) {
	tests := []struct {
		name               string
		query              workorchquerycontract.EventRecordQuery
		wantIncludePayload string
	}{
		{
			name: "metadata only default",
			query: workorchquerycontract.EventRecordQuery{
				WorkspaceID:   "ws-events",
				RunID:         "run-events",
				CursorAfterID: "event-001",
				Limit:         50,
			},
		},
		{
			name: "payload detail explicit",
			query: workorchquerycontract.EventRecordQuery{
				WorkspaceID:    "ws-events",
				RunID:          "run-events",
				Topic:          "/runtime/event",
				ExcludeTopics:  []string{"/run/llm_stream/progress"},
				SubjectType:    "run",
				SubjectID:      "run-events",
				CursorAfterID:  "event-001",
				OccurredAfter:  "2026-08-04T00:00:00Z",
				OccurredBefore: "2026-08-05T00:00:00Z",
				Limit:          50,
				IncludePayload: true,
			},
			wantIncludePayload: "true",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Fatalf("method = %s, want GET", r.Method)
				}
				if r.URL.Path != "/api/v1/workorch/events" {
					t.Fatalf("path = %s, want events endpoint", r.URL.Path)
				}
				values := r.URL.Query()
				if values.Get("workspaceId") != string(tt.query.WorkspaceID) ||
					values.Get("runId") != string(tt.query.RunID) ||
					values.Get("cursorAfterId") != string(tt.query.CursorAfterID) ||
					values.Get("limit") != "50" {
					t.Fatalf("query = %s, want scoped paged event query", r.URL.RawQuery)
				}
				if values.Get("includePayload") != tt.wantIncludePayload {
					t.Fatalf("includePayload = %q, want %q", values.Get("includePayload"), tt.wantIncludePayload)
				}
				if tt.query.Topic != "" && (values.Get("topic") != tt.query.Topic ||
					values.Get("subjectType") != tt.query.SubjectType ||
					values.Get("subjectId") != tt.query.SubjectID ||
					values.Get("excludeTopic") != tt.query.ExcludeTopics[0] ||
					values.Get("occurredAfter") != tt.query.OccurredAfter ||
					values.Get("occurredBefore") != tt.query.OccurredBefore) {
					t.Fatalf("detail filters = %s, want topic/subject filters", r.URL.RawQuery)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"eventRecords":[{"id":"event-001","payloadSummary":{"state":"running"},"payloadRef":"payloads/event-001.json"}]}`))
			}))
			defer server.Close()

			items, err := New(server.URL).ListEvents(context.Background(), tt.query)
			if err != nil {
				t.Fatalf("list events: %v", err)
			}
			if len(items) != 1 {
				t.Fatalf("items = %+v, want one event", items)
			}
			state, _ := items[0].PayloadSummary.String("state")
			if state != "running" || items[0].PayloadRef != "payloads/event-001.json" {
				t.Fatalf("items = %+v, want event summary/ref", items)
			}
		})
	}
}
