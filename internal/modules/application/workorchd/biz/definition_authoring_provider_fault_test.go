package biz

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	llmowner "github.com/muidea/workorch/internal/modules/blocks/llm/biz"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	workspaceowner "github.com/muidea/workorch/internal/modules/blocks/workspace/biz"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
)

type authoringCompletionFaultHub struct {
	magicevent.Hub
	cancel         context.CancelFunc
	cancelAfter    int32
	calls          *atomic.Int32
	loseCompletion bool
}

func (h authoringCompletionFaultHub) Send(ev magicevent.Event) magicevent.Result {
	result := h.Hub.Send(ev)
	if ev.ID() == llmevents.TopicLLMCommandCompleteProviderDocument && h.cancel != nil && h.calls.Load() >= h.cancelAfter {
		h.cancel()
	}
	if h.loseCompletion && (ev.ID() == workspaceevents.TopicWorkspaceDerivationComplete || ev.ID() == workspaceevents.TopicWorkspaceAuthoringSpecificationComplete) {
		lost := magicevent.NewResult(ev.ID(), ev.Source(), ev.Destination())
		lost.Set(nil, def.NewError(def.Unexpected, "synthetic completion reply lost after owner CAS"))
		return lost
	}
	return result
}

// 真实 HTTPProvider → LLM owner → Application → file-backed Workspace completion；
// 仅外部 HTTP 响应与交付故障由 fixture 控制，不以审计重建阶段结果。
func TestAuthoringProviderDocumentFaultsPersistAcrossFileReopen(t *testing.T) {
	for _, phase := range []string{"derivation", "proposal", "critic"} {
		for _, fault := range []string{"http_failure", "output_limit", "invalid", "cancel_and_reply_loss"} {
			t.Run(phase+"/"+fault, func(t *testing.T) {
				ctx := context.Background()
				storage, directory := t.TempDir(), t.TempDir()
				repositories := workorchrepo.NewWorkspaceScopedFileRepositorySet(storage)
				hints, err := workorcheventpayload.FromMap(map[string]any{"profileContracts": map[string]any{"generic": map[string]any{"evidenceRequirements": []any{map[string]any{"id": "*", "scope": "dodItem", "requiredEvidenceKinds": []any{"validation"}, "acceptedStepKinds": []any{"validation/managed_session"}}}}}})
				if err != nil {
					t.Fatal(err)
				}
				template := workspace.WorkspaceTemplate{ID: "t", DoDItems: []string{"模板验收"}, ContextHints: hints}
				if err := repositories.WorkspaceTemplates.Create(ctx, &template); err != nil {
					t.Fatal(err)
				}
				input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws-document-fault", Revision: 1, GoalText: "交付目标", TemplateID: template.ID, AcceptanceRequirements: []string{"用户验收"}}
				if err := repositories.Workspaces.Create(ctx, &workspace.Workspace{ID: input.WorkspaceID, TemplateID: template.ID, DeliverableDirectory: directory}); err != nil {
					t.Fatal(err)
				}
				hub := magicevent.NewHub(32)
				defer hub.Terminate(ctx)
				owner, err := workspaceowner.New(hub, nil, workspaceowner.Dependencies{OwnerRepositories: repositories.WorkspaceOwner, EventHub: hub})
				if err != nil {
					t.Fatal(err)
				}
				if err := owner.Start(); err != nil {
					t.Fatal(err)
				}
				defer owner.Teardown(ctx)
				var calls atomic.Int32
				var failureWire []byte
				const invalidDocument = "  {\"providerControlled\":true}\r\n"
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/v1/models" {
						_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": []any{map[string]any{"id": "chat-model", "supported_endpoints": []string{"/v1/responses"}, "contextWindowTokens": 131072, "maxOutputTokens": 16384}}})
						return
					}
					if r.URL.Path != "/v1/responses" {
						http.NotFound(w, r)
						return
					}
					call := calls.Add(1)
					row, err := repositories.Workspaces.Get(ctx, input.WorkspaceID)
					if err != nil {
						t.Error(err)
						return
					}
					record := row.DefinitionAuthoring
					failureCall := map[string]int32{"derivation": 1, "proposal": 2, "critic": 3}[phase]
					if call == failureCall && fault == "http_failure" {
						failureWire = []byte("\xef\xbb\xbf{\"error\":\"原始原因\\r\\n\"}\r\n")
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusBadRequest)
						_, _ = w.Write(failureWire)
						return
					}
					var payload []byte
					if call == 1 {
						payload, err = json.Marshal(workspace.DefinitionDerivedAcceptance{AuthoringInputHash: record.InputHash, TemplateSnapshotHash: record.TemplateSnapshotHash, AdditionalItems: []string{"补充验收"}})
					} else if call == 2 {
						payload, err = json.Marshal(struct {
							Specification workspace.GoalSpecification `json:"specification"`
						}{authoringHTTPProposal(t, record)})
					} else if call != failureCall {
						t.Error("fault matrix unexpectedly continued beyond first critic")
						return
					}
					if err != nil {
						t.Error(err)
						return
					}
					if call == failureCall {
						payload = []byte(invalidDocument)
					}
					w.Header().Set("Content-Type", "text/event-stream")
					if call == failureCall && fault == "output_limit" {
						delta, _ := json.Marshal(map[string]any{"type": "response.output_text.delta", "delta": string(payload)})
						failureWire = append(append([]byte("data: "), delta...), '\n', '\n')
						failureWire = append(failureWire, []byte("data: {\"type\":\"response.incomplete\",\"response\":{\"status\":\"incomplete\",\"incomplete_details\":{\"reason\":\"max_output_tokens\"}}}\n\n")...)
					} else {
						terminal, _ := json.Marshal(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed", "output_text": string(payload)}})
						failureWire = append(append([]byte("data: "), terminal...), '\n', '\n')
					}
					_, _ = w.Write(failureWire)
				}))
				defer server.Close()
				configured := workorchconfig.DefaultLLMConfig()
				configured.AIProxy.BaseURL, configured.AIProxy.APIKey = server.URL, "synthetic-key"
				configured.DefaultChat = workorchconfig.ProviderModelTargetConfig{ProviderID: "ai-proxy", ModelID: "chat-model"}
				llm, err := llmowner.New(hub, nil, llmowner.Dependencies{LoadLLMConfig: func() (workorchconfig.LLMConfig, error) { return configured, nil }})
				if err != nil {
					t.Fatal(err)
				}
				if err := llm.Start(); err != nil {
					t.Fatal(err)
				}
				defer llm.Teardown(ctx)
				b := &Biz{Base: basebiz.New(ModuleID, hub, nil), authoringTasks: newAuthoringTaskLifecycle()}
				row, _ := repositories.Workspaces.Get(ctx, input.WorkspaceID)
				if _, err := b.SaveDefinitionAuthoring(ctx, workspaceevents.SaveDefinitionAuthoringCommand{RequestID: "save", Input: input, ExpectedWorkspaceRevision: row.Revision}); err != nil {
					t.Fatal(err)
				}
				callCtx, cancel := context.WithCancel(ctx)
				defer cancel()
				if fault == "cancel_and_reply_loss" {
					b.Base = basebiz.New(ModuleID, authoringCompletionFaultHub{Hub: hub, cancel: cancel, cancelAfter: map[string]int32{"derivation": 1, "proposal": 2, "critic": 3}[phase], calls: &calls, loseCompletion: true}, nil)
				}
				if phase != "derivation" {
					// 前置成功派生不注入当前 phase 的取消窗口。
					b.Base = basebiz.New(ModuleID, hub, nil)
					if _, err := b.DeriveDefinitionAcceptance(ctx, input.WorkspaceID); err != nil {
						t.Fatal(err)
					}
					if fault == "cancel_and_reply_loss" {
						b.Base = basebiz.New(ModuleID, authoringCompletionFaultHub{Hub: hub, cancel: cancel, cancelAfter: map[string]int32{"derivation": 1, "proposal": 2, "critic": 3}[phase], calls: &calls, loseCompletion: true}, nil)
					}
				}
				if phase == "derivation" {
					_, err = b.DeriveDefinitionAcceptance(callCtx, input.WorkspaceID)
				} else {
					_, err = b.CompileAuthoringSpecification(callCtx, input.WorkspaceID)
				}
				if err == nil {
					t.Fatal("known invalid/failure outcome unexpectedly succeeded")
				}
				wantCalls := map[string]int32{"derivation": 1, "proposal": 2, "critic": 3}[phase]
				if calls.Load() != wantCalls {
					t.Fatalf("physical calls=%d want=%d", calls.Load(), wantCalls)
				}
				b.Base = basebiz.New(ModuleID, hub, nil)
				view, err := b.ReadDefinitionAuthoringView(ctx, input.WorkspaceID)
				if err != nil || view.State != "failed" {
					t.Fatalf("known failure became unresolved: %s %v", view.State, err)
				}
				request := view.Requests[len(view.Requests)-1]
				raw, err := b.ReadDefinitionAuthoringDiagnosticView(ctx, input.WorkspaceID, request.PermitID)
				if err != nil {
					t.Fatal(err)
				}
				if raw.ProviderWire == nil || raw.ProviderWire.ContentBase64 != base64.StdEncoding.EncodeToString(failureWire) || !raw.ProviderWire.Complete {
					t.Fatal("provider wire was not owner-queryable intact")
				}
				if fault == "http_failure" || fault == "output_limit" {
					if fault == "output_limit" && (raw.ProviderDocument == nil || raw.ProviderDocument.Complete || raw.ProviderDocument.ContentBase64 != base64.StdEncoding.EncodeToString([]byte(invalidDocument))) {
						t.Fatal("partial document became complete or changed")
					}
				} else if raw.ProviderCode != "" || raw.ProviderDocument == nil || !raw.ProviderDocument.Complete || raw.ProviderDocument.ContentBase64 != base64.StdEncoding.EncodeToString([]byte(invalidDocument)) || raw.PayloadBase64 != base64.StdEncoding.EncodeToString([]byte(invalidDocument)) {
					t.Fatal("strict parser diagnostic changed original content or fabricated provider failure")
				}
				owner.Teardown(ctx)
				llm.Teardown(ctx)
				reopened := workorchrepo.NewWorkspaceScopedFileRepositorySet(storage)
				freshHub := magicevent.NewHub(16)
				defer freshHub.Terminate(ctx)
				freshOwner, err := workspaceowner.New(freshHub, nil, workspaceowner.Dependencies{OwnerRepositories: reopened.WorkspaceOwner, EventHub: freshHub})
				if err != nil {
					t.Fatal(err)
				}
				if err := freshOwner.Start(); err != nil {
					t.Fatal(err)
				}
				defer freshOwner.Teardown(ctx)
				fresh := &Biz{Base: basebiz.New(ModuleID, freshHub, nil), authoringTasks: newAuthoringTaskLifecycle()}
				persisted, err := fresh.ReadDefinitionAuthoringDiagnosticView(ctx, input.WorkspaceID, request.PermitID)
				before, _ := json.Marshal(raw)
				after, _ := json.Marshal(persisted)
				if err != nil || string(before) != string(after) {
					t.Fatal("file reopen changed immutable failure diagnostic")
				}
				if phase == "derivation" {
					_, err = fresh.DeriveDefinitionAcceptance(ctx, input.WorkspaceID)
				} else {
					_, err = fresh.CompileAuthoringSpecification(ctx, input.WorkspaceID)
				}
				if err == nil || calls.Load() != wantCalls {
					t.Fatal("failure replay issued another provider call")
				}
				stored, err := reopened.Workspaces.Get(ctx, input.WorkspaceID)
				if err != nil || stored.ActiveRunID != "" {
					t.Fatalf("failure created an active Run or Workspace became unreadable: %v", err)
				}
				goals, goalErr := reopened.Goals.ListByWorkspaceID(ctx, input.WorkspaceID)
				dods, dodErr := reopened.DoDs.ListByWorkspaceID(ctx, input.WorkspaceID)
				runs, runErr := reopened.Runs.ListByWorkspaceID(ctx, input.WorkspaceID)
				if goalErr != nil || dodErr != nil || runErr != nil || len(goals) != 0 || len(dods) != 0 || len(runs) != 0 {
					t.Fatalf("authoring failure materialized formal resources: goals=%d DoDs=%d Runs=%d errors=%v/%v/%v", len(goals), len(dods), len(runs), goalErr, dodErr, runErr)
				}
				if _, err := fresh.ReadDefinitionAuthoringDiagnosticView(ctx, "another-workspace", request.PermitID); err == nil {
					t.Fatal("diagnostic escaped workspace scope")
				}
			})
		}
	}
}
