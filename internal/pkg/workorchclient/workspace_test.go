package workorchclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestClientDecodesWorkspaceProjectionMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/workorch/workspaces":
			_, _ = w.Write([]byte(`{
				"service":"workorch",
				"projection":{"state":"ready","globalRevision":12,"workspaceTotal":1,"sourceEventRevision":88,"projectedSourceRevision":86,"staleReason":"source_event"},
				"workspaces":[{"id":"ws-1","title":"A","state":"active"}]
			}`))
		case "/api/v1/workorch/workspaces/ws-1/snapshot":
			if r.URL.Query().Get("view") != "detail" {
				t.Fatalf("workspace snapshot query = %q, want explicit detail view", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{
				"service":"workorch",
				"projection":{"state":"ready","globalRevision":12,"workspaceRevision":7,"sourceEventRevision":91,"projectedSourceRevision":91,"degraded":false},
				"snapshot":{"workspace":{"id":"ws-1","title":"A","state":"running"},"runHeartbeat":{"workspaceId":"ws-1","runId":"run-1","activityState":"running","latestEffectiveProgressAt":"2026-06-10T00:00:00Z"}}
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := New(server.URL)
	list, err := client.ListWorkspaceProjection(context.Background())
	if err != nil {
		t.Fatalf("list workspace projection: %v", err)
	}
	if list.Projection.State != "ready" || list.Projection.GlobalRevision != 12 || list.Projection.SourceEventRevision != 88 || list.Projection.ProjectedSourceRevision != 86 || list.Projection.StaleReason != "source_event" || len(list.Workspaces) != 1 {
		t.Fatalf("list projection = %+v, want ready revision 12 with one workspace", list)
	}

	detail, err := client.GetWorkspaceSnapshotProjection(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("workspace snapshot projection: %v", err)
	}
	if detail.Projection.WorkspaceRevision != 7 || detail.Projection.SourceEventRevision != 91 || detail.Projection.ProjectedSourceRevision != 91 || detail.Snapshot.Workspace.ID != "ws-1" {
		t.Fatalf("detail projection = %+v, want workspace revision 7 for ws-1", detail)
	}
	if detail.Snapshot.RunHeartbeat.RunID != "run-1" || detail.Snapshot.RunHeartbeat.ActivityState != "running" {
		t.Fatalf("run heartbeat = %+v, want decoded running run heartbeat", detail.Snapshot.RunHeartbeat)
	}
}

func TestClientWorkspaceLLMTargetRoutesCarryProviderAndModel(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/api/v1/workorch/workspaces/ws-model/llm-target" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		switch r.Method {
		case http.MethodPut:
			var target runtimecontract.ProviderModelTarget
			if err := json.NewDecoder(r.Body).Decode(&target); err != nil {
				t.Fatalf("decode target: %v", err)
			}
			if target.ProviderID != "custom" || target.ModelID != "model-a" {
				t.Fatalf("target = %+v", target)
			}
			_, _ = w.Write([]byte(`{"workspace":{"id":"ws-model","title":"Model","state":"active","defaultLLMTarget":{"providerId":"custom","modelId":"model-a"}}}`))
		case http.MethodDelete:
			_, _ = w.Write([]byte(`{"workspace":{"id":"ws-model","title":"Model","state":"active"}}`))
		default:
			t.Fatalf("method = %q", r.Method)
		}
	}))
	defer server.Close()

	cl := New(server.URL)
	updated, err := cl.UpdateWorkspaceLLMTarget(context.Background(), "ws-model", runtimecontract.ProviderModelTarget{ProviderID: "custom", ModelID: "model-a"})
	if err != nil {
		t.Fatalf("UpdateWorkspaceLLMTarget: %v", err)
	}
	if updated.DefaultLLMTarget == nil || updated.DefaultLLMTarget.ProviderID != "custom" || updated.DefaultLLMTarget.ModelID != "model-a" {
		t.Fatalf("updated workspace = %+v", updated)
	}
	cleared, err := cl.ClearWorkspaceLLMTarget(context.Background(), "ws-model")
	if err != nil {
		t.Fatalf("ClearWorkspaceLLMTarget: %v", err)
	}
	if cleared.DefaultLLMTarget != nil || requests != 2 {
		t.Fatalf("cleared workspace = %+v requests=%d", cleared, requests)
	}
}

func TestClientLLMModelCatalogRoutesUseProviderScope(t *testing.T) {
	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.RequestURI())
		_, _ = w.Write([]byte(`{"catalog":{"providerId":"custom-provider","state":"ready","models":[{"providerId":"custom-provider","modelId":"model-a","operations":["chat_completions"],"capabilityReady":true}]}}`))
	}))
	defer server.Close()
	cl := New(server.URL)
	for _, call := range []func() (*ProviderCatalogSummary, error){
		func() (*ProviderCatalogSummary, error) {
			return cl.GetLLMModelCatalog(context.Background(), "custom-provider")
		},
		func() (*ProviderCatalogSummary, error) {
			return cl.RefreshLLMModelCatalog(context.Background(), "custom-provider")
		},
	} {
		catalog, err := call()
		if err != nil {
			t.Fatalf("catalog call: %v", err)
		}
		if catalog.ProviderID != "custom-provider" || len(catalog.Models) != 1 || catalog.Models[0].ModelID != "model-a" {
			t.Fatalf("catalog = %+v", catalog)
		}
	}
	if len(requests) != 2 || requests[0] != "GET /api/v1/workorch/configuration/llm/models?providerId=custom-provider" || requests[1] != "POST /api/v1/workorch/configuration/llm/models/refresh?providerId=custom-provider" {
		t.Fatalf("requests = %+v", requests)
	}
}

func TestClientGovernanceRequestsCarryWorkspaceScope(t *testing.T) {
	const workspaceID = "ws-governance"

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
			name:   "get decision",
			method: http.MethodGet,
			path:   "/api/v1/workorch/decisions/ticket-1",
			query:  "workspaceId=ws-governance",
			call: func(ctx context.Context, client *Client) error {
				decision, err := client.GetDecision(ctx, workspaceID, "ticket-1")
				if err != nil {
					return err
				}
				if decision.ID != "ticket-1" || decision.WorkspaceID != workspaceID {
					t.Fatalf("decision = %+v, want ticket-1 scoped to %s", decision, workspaceID)
				}
				return nil
			},
		},
		{
			name:      "resolve decision",
			method:    http.MethodPost,
			path:      "/api/v1/workorch/decisions/ticket-1/resolve",
			query:     "workspaceId=ws-governance",
			bodyKey:   "resolutionType",
			bodyValue: "accept",
			call: func(ctx context.Context, client *Client) error {
				decision, err := client.ResolveDecision(ctx, workspaceID, "ticket-1", "accept", workorcheventpayload.New(workorcheventpayload.BoolField("ok", true)))
				if err != nil {
					return err
				}
				if decision.ID != "ticket-1" || decision.WorkspaceID != workspaceID {
					t.Fatalf("decision = %+v, want ticket-1 scoped to %s", decision, workspaceID)
				}
				return nil
			},
		},
		{
			name:      "batch resolve decisions",
			method:    http.MethodPost,
			path:      "/api/v1/workorch/decisions/resolve-batch",
			query:     "workspaceId=ws-governance",
			bodyKey:   "resolutionType",
			bodyValue: "accept",
			call: func(ctx context.Context, client *Client) error {
				decisions, err := client.BatchResolveDecisions(ctx, workspaceID, []string{"ticket-1"}, "accept", workorcheventpayload.Payload{})
				if err != nil {
					return err
				}
				if len(decisions) != 1 || decisions[0].WorkspaceID != workspaceID {
					t.Fatalf("decisions = %+v, want one decision scoped to %s", decisions, workspaceID)
				}
				return nil
			},
		},
		{
			name:   "get approval",
			method: http.MethodGet,
			path:   "/api/v1/workorch/approvals/approval-1",
			query:  "workspaceId=ws-governance",
			call: func(ctx context.Context, client *Client) error {
				approval, err := client.GetApproval(ctx, workspaceID, "approval-1")
				if err != nil {
					return err
				}
				if approval.ID != "approval-1" || approval.WorkspaceID != workspaceID {
					t.Fatalf("approval = %+v, want approval-1 scoped to %s", approval, workspaceID)
				}
				return nil
			},
		},
		{
			name:   "approve approval",
			method: http.MethodPost,
			path:   "/api/v1/workorch/approvals/approval-1/approve",
			query:  "workspaceId=ws-governance",
			call: func(ctx context.Context, client *Client) error {
				approval, err := client.Approve(ctx, workspaceID, "approval-1")
				if err != nil {
					return err
				}
				if approval.ID != "approval-1" || approval.WorkspaceID != workspaceID {
					t.Fatalf("approval = %+v, want approval-1 scoped to %s", approval, workspaceID)
				}
				return nil
			},
		},
		{
			name:   "reject approval",
			method: http.MethodPost,
			path:   "/api/v1/workorch/approvals/approval-1/reject",
			query:  "workspaceId=ws-governance",
			call: func(ctx context.Context, client *Client) error {
				approval, err := client.Reject(ctx, workspaceID, "approval-1")
				if err != nil {
					return err
				}
				if approval.ID != "approval-1" || approval.WorkspaceID != workspaceID {
					t.Fatalf("approval = %+v, want approval-1 scoped to %s", approval, workspaceID)
				}
				return nil
			},
		},
		{
			name:      "batch resolve approvals",
			method:    http.MethodPost,
			path:      "/api/v1/workorch/approvals/resolve-batch",
			query:     "workspaceId=ws-governance",
			bodyKey:   "action",
			bodyValue: "approve",
			call: func(ctx context.Context, client *Client) error {
				approvals, err := client.BatchResolveApprovals(ctx, workspaceID, []string{"approval-1"}, "approve")
				if err != nil {
					return err
				}
				if len(approvals) != 1 || approvals[0].WorkspaceID != workspaceID {
					t.Fatalf("approvals = %+v, want one approval scoped to %s", approvals, workspaceID)
				}
				return nil
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
				case tt.path == "/api/v1/workorch/decisions/resolve-batch":
					_, _ = w.Write([]byte(`{"decisions":[{"id":"ticket-1","workspaceId":"ws-governance","runId":"run-1","reason":"test","state":"resolved"}]}`))
				case tt.path == "/api/v1/workorch/approvals/resolve-batch":
					_, _ = w.Write([]byte(`{"approvals":[{"id":"approval-1","workspaceId":"ws-governance","runId":"run-1","reason":"test","state":"approved"}]}`))
				case strings.HasPrefix(r.URL.Path, "/api/v1/workorch/decisions/"):
					_, _ = w.Write([]byte(`{"decision":{"id":"ticket-1","workspaceId":"ws-governance","runId":"run-1","reason":"test","state":"resolved"}}`))
				default:
					_, _ = w.Write([]byte(`{"approval":{"id":"approval-1","workspaceId":"ws-governance","runId":"run-1","reason":"test","state":"approved"}}`))
				}
			}))
			defer server.Close()

			if err := tt.call(context.Background(), New(server.URL)); err != nil {
				t.Fatalf("client call: %v", err)
			}
		})
	}
}
