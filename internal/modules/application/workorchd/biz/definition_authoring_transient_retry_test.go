package biz

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	fileowner "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	llmowner "github.com/muidea/workorch/internal/modules/blocks/llm/biz"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	workspaceowner "github.com/muidea/workorch/internal/modules/kernel/workspace/biz"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/testkit"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
)

type authoringTransientHarness struct {
	ctx          context.Context
	storage      string
	directory    string
	template     workspace.WorkspaceTemplate
	input        workspace.DefinitionAuthoringInput
	repositories workorchrepo.WorkspaceScopedFileRepositorySet
	hub          magicevent.Hub
	workspace    *workspaceowner.Biz
	llm          *llmowner.Biz
	file         *fileowner.Biz
	biz          *Biz
	server       *httptest.Server
	calls        *atomic.Int32
}

func openAuthoringTransientHarness(t *testing.T, withResource bool, handler http.HandlerFunc) *authoringTransientHarness {
	t.Helper()
	ctx := context.Background()
	storage, directory := t.TempDir(), t.TempDir()
	repositories := workorchrepo.NewWorkspaceScopedFileRepositorySet(storage)
	hints, err := workorcheventpayload.FromMap(map[string]any{
		"profileContracts": map[string]any{
			"generic": map[string]any{
				"evidenceRequirements": []any{map[string]any{
					"id": "*", "scope": "dodItem", "requiredEvidenceKinds": []any{"validation"}, "acceptedStepKinds": []any{"validation/managed_session"},
				}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	template := workspace.WorkspaceTemplate{ID: "t", DoDItems: []string{"模板验收"}, ContextHints: hints}
	if err := repositories.WorkspaceTemplates.Create(ctx, &template); err != nil {
		t.Fatal(err)
	}
	input := workspace.DefinitionAuthoringInput{
		Version: 1, WorkspaceID: "ws-transient-retry", Revision: 1,
		GoalText: "交付目标", TemplateID: template.ID, AcceptanceRequirements: []string{"用户验收"},
	}
	if withResource {
		if err := os.WriteFile(filepath.Join(directory, "requirements.txt"), []byte("资源验收项\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		input.ResourceRefs = []string{"requirements.txt"}
	}
	if err := repositories.Workspaces.Create(ctx, &workspace.Workspace{ID: input.WorkspaceID, TemplateID: template.ID, DeliverableDirectory: directory}); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": []any{map[string]any{"id": "chat-model", "supported_endpoints": []string{"/v1/responses"}, "contextWindowTokens": 131072, "maxOutputTokens": 16384}}})
			return
		}
		if r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}
		calls.Add(1)
		handler(w, r)
	}))
	h := &authoringTransientHarness{
		ctx:          ctx,
		storage:      storage,
		directory:    directory,
		template:     template,
		input:        input,
		repositories: repositories,
		server:       server,
		calls:        &calls,
	}
	h.openOwners()
	row, err := repositories.Workspaces.Get(ctx, input.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.biz.SaveDefinitionAuthoring(ctx, workspaceevents.SaveDefinitionAuthoringCommand{
		RequestID: "save-initial", Input: input, ExpectedWorkspaceRevision: row.Revision,
	}); err != nil {
		t.Fatal(err)
	}
	return h
}

func (h *authoringTransientHarness) openOwners() {
	h.repositories = workorchrepo.NewWorkspaceScopedFileRepositorySet(h.storage)
	h.hub = magicevent.NewHub(32)
	wsOwner, err := workspaceowner.New(h.hub, nil, workspaceowner.Dependencies{OwnerRepositories: h.repositories.WorkspaceOwner, EventHub: h.hub})
	if err != nil {
		panic(err)
	}
	if err := wsOwner.Start(); err != nil {
		panic(err)
	}
	fileOwner, err := fileowner.New(h.hub, nil)
	if err != nil {
		panic(err)
	}
	if err := fileOwner.Start(); err != nil {
		panic(err)
	}
	configured := workorchconfig.DefaultLLMConfig()
	configured.AIProxy.BaseURL, configured.AIProxy.APIKey = h.server.URL, "synthetic-key"
	configured.DefaultChat = workorchconfig.ProviderModelTargetConfig{ProviderID: "ai-proxy", ModelID: "chat-model"}
	llmOwner, err := llmowner.New(h.hub, nil, llmowner.Dependencies{LoadLLMConfig: func() (workorchconfig.LLMConfig, error) { return configured, nil }})
	if err != nil {
		panic(err)
	}
	if err := llmOwner.Start(); err != nil {
		panic(err)
	}
	h.workspace = wsOwner
	h.file = fileOwner
	h.llm = llmOwner
	h.biz = &Biz{
		Base:                basebiz.New(ModuleID, h.hub, nil),
		authoringTasks:      newAuthoringTaskLifecycle(),
		authoringRetryDelay: func(int) time.Duration { return 0 },
	}
}

func (h *authoringTransientHarness) closeOwners() {
	if h.biz != nil && h.biz.authoringTasks != nil {
		h.biz.authoringTasks.stop()
	}
	if h.llm != nil {
		h.llm.Teardown(h.ctx)
	}
	if h.file != nil {
		h.file.Teardown(h.ctx)
	}
	if h.workspace != nil {
		h.workspace.Teardown(h.ctx)
	}
	if h.hub != nil {
		h.hub.Terminate(h.ctx)
	}
}

func (h *authoringTransientHarness) closeAll() {
	h.closeOwners()
	if h.server != nil {
		h.server.Close()
	}
}

func pendingAuthoringStage(record *workspace.DefinitionAuthoringRecord) (stage, kind string, slotAttempts int, completedDerivation *workspace.DefinitionDerivationRequest, activeProposal *workspace.GoalSpecification, pendingSpec *workspace.DefinitionAuthoringSpecificationRequest) {
	for i := range record.DerivationRequests {
		d := &record.DerivationRequests[i]
		if d.Input.InputHash() != record.InputHash {
			continue
		}
		if d.Proposal != nil {
			completedDerivation = d
		}
		if d.Proposal == nil && d.FailureCode == "" {
			return "derivation", "", len(record.CurrentDerivationRequests()), nil, nil, nil
		}
	}
	if completedDerivation == nil {
		return "", "", 0, nil, nil, nil
	}
	for i := range record.SpecificationRequests {
		req := &record.SpecificationRequests[i]
		if req.DerivationPermitID != completedDerivation.PermitID {
			continue
		}
		if req.Proposal != nil {
			p := req.Proposal.Clone()
			activeProposal = &p
		}
		if req.Proposal == nil && req.Review == nil && req.FailureCode == "" {
			slot := record.SpecificationSlotRequests(req.DerivationPermitID, req.Phase, req.ReviewKind, req.ProposalHash, req.PreviousProposalHash, req.FeedbackHash)
			if req.Phase == workspace.DefinitionSpecificationProposal {
				return "proposal", "", len(slot), completedDerivation, activeProposal, req
			}
			return "critic", req.ReviewKind, len(slot), completedDerivation, activeProposal, req
		}
	}
	return "", "", 0, completedDerivation, activeProposal, nil
}

func writeAuthoringFixtureSuccess(t *testing.T, w http.ResponseWriter, record *workspace.DefinitionAuthoringRecord, stage, kind string, derivation *workspace.DefinitionDerivationRequest, activeProposal *workspace.GoalSpecification, pendingSpec *workspace.DefinitionAuthoringSpecificationRequest, reviseOnLastGen0Critic bool) {
	t.Helper()
	switch stage {
	case "derivation":
		payload, _ := json.Marshal(workspace.DefinitionDerivedAcceptance{
			AuthoringInputHash:   record.InputHash,
			TemplateSnapshotHash: record.TemplateSnapshotHash,
			AdditionalItems:      []string{"补充验收"},
		})
		writeSSECompletedJSON(w, testkit.OwnerBoundFixtureDocument(payload))
	case "proposal":
		mapping, err := testkit.Mapping(*derivation, workspace.DefinitionAuthoringVerificationMapping{
			Declaration: "按模板验证", Status: workspace.GoalVerificationDeclared,
			ProfileID: "generic", EvidenceRequirementID: "*", Scope: "dodItem",
		})
		if err != nil {
			t.Error(err)
			return
		}
		if pendingSpec != nil && pendingSpec.PreviousProposalHash != "" {
			feedback, fbErr := record.SpecificationFeedback(derivation.PermitID, pendingSpec.PreviousProposalHash)
			if fbErr != nil {
				t.Error(fbErr)
				return
			}
			mapping.DesignDecisions = []workspace.GoalSpecificationDesignDecision{{
				CandidateItemRef:  mapping.Obligations[0].CandidateItemRef,
				Decision:          "Concrete design rule",
				VerificationBasis: "Compare expected test output",
			}}
			for _, ref := range feedback.IssueRefs() {
				mapping.IssueResolutions = append(mapping.IssueResolutions, workspace.DefinitionAuthoringIssueResolution{
					IssueRef: ref, Change: "Added concrete design decision",
				})
			}
		}
		payload, _ := json.Marshal(struct {
			Mapping workspace.DefinitionAuthoringMapping `json:"mapping"`
		}{mapping})
		writeSSECompletedJSON(w, testkit.OwnerBoundFixtureDocument(payload))
	case "critic":
		reqIDs, err := activeProposal.RequiredReviewRequirementIDs(kind)
		if err != nil {
			t.Error(err)
			return
		}
		ids := make([]string, 0, len(reqIDs))
		for _, id := range reqIDs {
			ids = append(ids, id)
		}
		review := workspace.GoalSpecificationCompletenessReview{
			Kind: kind, Status: workspace.GoalReviewPass,
			SpecificationHash: activeProposal.ContentHash(), Reason: "覆盖",
			EvidenceRefs: []string{workspace.DefinitionAuthoringGoalItemRef}, RequirementIDs: ids,
		}
		if reviseOnLastGen0Critic && len(activeProposal.DesignDecisions) == 0 && kind == workspace.GoalReviewVerifierFeasibility {
			review.Status = workspace.GoalReviewUnknown
			review.Issues = []workspace.GoalSpecificationReviewIssue{{
				Class:          workspace.AuthoringIssueDesign,
				Detail:         "Need concrete design decision",
				RequirementIDs: ids,
				EvidenceRefs:   []string{workspace.DefinitionAuthoringGoalItemRef},
			}}
		}
		payload, _ := json.Marshal(struct {
			Review workspace.GoalSpecificationCompletenessReview `json:"review"`
		}{review})
		writeSSECompletedJSON(w, testkit.OwnerBoundFixtureDocument(payload))
	default:
		t.Errorf("unexpected pending authoring stage=%q", stage)
	}
}

func writeTransientFailureResponse(t *testing.T, w http.ResponseWriter, fault string, partialBytes []byte) []byte {
	t.Helper()
	switch fault {
	case "unexpected_eof":
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("response writer does not support http.Hijacker")
		}
		conn, rw, err := hj.Hijack()
		if err != nil {
			t.Fatalf("hijack failed: %v", err)
		}
		_, _ = rw.WriteString("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nTransfer-Encoding: chunked\r\n\r\n")
		delta, _ := json.Marshal(map[string]any{"type": "response.output_text.delta", "delta": string(partialBytes)})
		chunk := append(append([]byte("data: "), delta...), '\n', '\n')
		_, _ = rw.WriteString(strconv.FormatInt(int64(len(chunk)), 16) + "\r\n")
		_, _ = rw.Write(chunk)
		_, _ = rw.WriteString("\r\n")
		_ = rw.Flush()
		_ = conn.Close()
		return chunk
	case "stream_ended_before_completion":
		w.Header().Set("Content-Type", "text/event-stream")
		delta, _ := json.Marshal(map[string]any{"type": "response.output_text.delta", "delta": string(partialBytes)})
		wire := append(append([]byte("data: "), delta...), '\n', '\n')
		_, _ = w.Write(wire)
		return wire
	case "http_503":
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		wire := []byte("upstream 503 unavailable\r\n")
		_, _ = w.Write(wire)
		return wire
	default:
		t.Fatalf("unknown transient fault %q", fault)
		return nil
	}
}

func writeSSECompletedJSON(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", "text/event-stream")
	terminal, _ := json.Marshal(map[string]any{
		"type":     "response.completed",
		"response": map[string]any{"status": "completed", "output_text": string(payload)},
	})
	_, _ = w.Write(append(append([]byte("data: "), terminal...), '\n', '\n'))
}

func TestAuthoringBoundedTransientRetryMatrixAcrossReopen(t *testing.T) {
	for _, phase := range []string{"derivation", "proposal", "critic"} {
		for _, fault := range []string{"unexpected_eof", "stream_ended_before_completion", "http_503"} {
			t.Run(phase+"/"+fault, func(t *testing.T) {
				var h *authoringTransientHarness
				h = openAuthoringTransientHarness(t, phase == "derivation", func(w http.ResponseWriter, _ *http.Request) {
					row, err := h.repositories.Workspaces.Get(h.ctx, h.input.WorkspaceID)
					if err != nil || row.DefinitionAuthoring == nil {
						t.Errorf("workspace unavailable during provider call: %v", err)
						return
					}
					stage, kind, attempt, derivation, activeProposal, pendingSpec := pendingAuthoringStage(row.DefinitionAuthoring)
					targetMatch := (stage == phase) && (phase != "critic" || kind == workspace.GoalReviewVerifierFeasibility)
					if targetMatch && attempt <= 2 {
						writeTransientFailureResponse(t, w, fault, []byte(`{"partial":`))
						return
					}
					writeAuthoringFixtureSuccess(t, w, row.DefinitionAuthoring, stage, kind, derivation, activeProposal, pendingSpec, false)
				})
				defer h.closeAll()

				if _, err := h.biz.DeriveDefinitionAcceptance(h.ctx, h.input.WorkspaceID); err != nil {
					t.Fatalf("derivation failed with transient retries: %v", err)
				}
				if _, err := h.biz.CompileAuthoringSpecification(h.ctx, h.input.WorkspaceID); err != nil {
					t.Fatalf("specification failed with transient retries: %v", err)
				}
				if gotCalls := h.calls.Load(); gotCalls != 10 {
					t.Fatalf("total physical calls=%d want=10 (2 transient failures + 8 normal steps)", gotCalls)
				}
				view, err := h.biz.ReadDefinitionAuthoringView(h.ctx, h.input.WorkspaceID)
				if err != nil || view.State != "ready_to_confirm" {
					t.Fatalf("state=%q want=ready_to_confirm err=%v", view.State, err)
				}

				failedPermits := []string{}
				for _, req := range view.Requests {
					if req.State == "failed" {
						failedPermits = append(failedPermits, req.PermitID)
						if req.Diagnostic == nil || req.Diagnostic.ProviderFailureClass != "provider_transient" {
							t.Fatalf("failed retry request lost transient diagnostic metadata: %+v", req)
						}
					}
				}
				if len(failedPermits) != 2 {
					t.Fatalf("failed permits=%d want=2 (requests=%+v)", len(failedPermits), view.Requests)
				}

				// Reopen file-backed owners and verify original diagnostics + idempotent replay.
				h.closeOwners()
				h.openOwners()
				reopenedView, err := h.biz.ReadDefinitionAuthoringView(h.ctx, h.input.WorkspaceID)
				if err != nil || reopenedView.State != "ready_to_confirm" {
					t.Fatalf("reopened state=%q err=%v", reopenedView.State, err)
				}
				for _, permitID := range failedPermits {
					raw, err := h.biz.ReadDefinitionAuthoringDiagnosticView(h.ctx, h.input.WorkspaceID, permitID)
					if err != nil {
						t.Fatalf("ReadDefinitionAuthoringDiagnosticView(%s): %v", permitID, err)
					}
					if raw.FailureCode != "provider_failed" || raw.ProviderFailureClass != "provider_transient" {
						t.Fatalf("reopened diagnostic lost failure class: %+v", raw)
					}
					switch fault {
					case "unexpected_eof":
						if raw.ProviderCode != "request_failed" || raw.ProviderCause == nil || !raw.ProviderCause.Complete || raw.ProviderDocument == nil || raw.ProviderDocument.Complete {
							t.Fatalf("unexpected_eof diagnostic incomplete: %+v", raw)
						}
					case "stream_ended_before_completion":
						if raw.ProviderCode != "stream_ended_before_completion" || raw.ProviderDocument == nil || raw.ProviderDocument.Complete {
							t.Fatalf("stream_ended_before_completion diagnostic unexpected: %+v", raw)
						}
					case "http_503":
						if raw.ProviderCode != "http_503" || raw.ProviderWire == nil || !raw.ProviderWire.Complete {
							t.Fatalf("http_503 diagnostic unexpected: %+v", raw)
						}
					}
				}
				if _, err := h.biz.DeriveDefinitionAcceptance(h.ctx, h.input.WorkspaceID); err != nil {
					t.Fatal(err)
				}
				if _, err := h.biz.CompileAuthoringSpecification(h.ctx, h.input.WorkspaceID); err != nil {
					t.Fatal(err)
				}
				if h.calls.Load() != 10 {
					t.Fatalf("replay after ready_to_confirm made extra calls: %d", h.calls.Load())
				}
			})
		}
	}
}

func TestAuthoringTransientRetryExhaustionAndReplayStop(t *testing.T) {
	for _, phase := range []string{"derivation", "critic"} {
		t.Run(phase, func(t *testing.T) {
			var h *authoringTransientHarness
			h = openAuthoringTransientHarness(t, false, func(w http.ResponseWriter, _ *http.Request) {
				row, err := h.repositories.Workspaces.Get(h.ctx, h.input.WorkspaceID)
				if err != nil || row.DefinitionAuthoring == nil {
					t.Error(err)
					return
				}
				stage, kind, _, derivation, activeProposal, pendingSpec := pendingAuthoringStage(row.DefinitionAuthoring)
				if (stage == "derivation" && phase == "derivation") || (stage == "critic" && phase == "critic" && kind == workspace.GoalReviewVerifierFeasibility) {
					writeTransientFailureResponse(t, w, "unexpected_eof", []byte(`{"review":`))
					return
				}
				writeAuthoringFixtureSuccess(t, w, row.DefinitionAuthoring, stage, kind, derivation, activeProposal, pendingSpec, false)
			})
			defer h.closeAll()

			wantCalls := int32(4)
			if phase == "derivation" {
				if _, err := h.biz.DeriveDefinitionAcceptance(h.ctx, h.input.WorkspaceID); err == nil {
					t.Fatal("exhausted derivation unexpectedly succeeded")
				}
			} else {
				wantCalls = 1 + 1 + 5 + 4
				if _, err := h.biz.DeriveDefinitionAcceptance(h.ctx, h.input.WorkspaceID); err != nil {
					t.Fatal(err)
				}
				if _, err := h.biz.CompileAuthoringSpecification(h.ctx, h.input.WorkspaceID); err == nil {
					t.Fatal("exhausted critic unexpectedly succeeded")
				}
			}
			if h.calls.Load() != wantCalls {
				t.Fatalf("exhausted calls=%d want=%d", h.calls.Load(), wantCalls)
			}
			view, err := h.biz.ReadDefinitionAuthoringView(h.ctx, h.input.WorkspaceID)
			if err != nil || view.State != "failed" {
				t.Fatalf("exhausted state=%q want=failed err=%v", view.State, err)
			}
			failedCount := 0
			for _, req := range view.Requests {
				if req.State == "failed" {
					failedCount++
					raw, err := h.biz.ReadDefinitionAuthoringDiagnosticView(h.ctx, h.input.WorkspaceID, req.PermitID)
					if err != nil || raw.ProviderCode != "request_failed" || raw.ProviderFailureClass != "provider_transient" {
						t.Fatalf("missing intact exhausted diagnostic for %s: %+v %v", req.PermitID, raw, err)
					}
				}
			}
			if failedCount != 4 {
				t.Fatalf("failed diagnostics count=%d want=4", failedCount)
			}

			// Reopen & replay must issue 0 calls and reject background compile start.
			h.closeOwners()
			h.openOwners()
			if phase == "derivation" {
				if _, err := h.biz.DeriveDefinitionAcceptance(h.ctx, h.input.WorkspaceID); err == nil {
					t.Fatal("reopened exhausted derivation succeeded")
				}
			} else {
				if _, err := h.biz.CompileAuthoringSpecification(h.ctx, h.input.WorkspaceID); err == nil {
					t.Fatal("reopened exhausted critic succeeded")
				}
			}
			if started, startErr := h.biz.StartDefinitionAuthoringCompilation(h.ctx, h.input.WorkspaceID, workorchmanagementcontract.DefinitionAuthoringCompile{
				ExpectedInputHash: view.InputHash, ExpectedTemplateSnapshotHash: view.TemplateSnapshotHash,
			}); startErr != nil || started.State != "failed" || started.CompilationAccepted {
				t.Fatalf("StartDefinitionAuthoringCompilation after exhaustion=%+v err=%v", started, startErr)
			}
			if h.calls.Load() != wantCalls {
				t.Fatalf("exhausted replay made extra calls: got=%d want=%d", h.calls.Load(), wantCalls)
			}
		})
	}
}

func TestAuthoringTransientRetryCrashRecoveryMidBackoff(t *testing.T) {
	for _, phase := range []string{"derivation", "critic"} {
		t.Run(phase, func(t *testing.T) {
			var h *authoringTransientHarness
			h = openAuthoringTransientHarness(t, false, func(w http.ResponseWriter, _ *http.Request) {
				row, _ := h.repositories.Workspaces.Get(h.ctx, h.input.WorkspaceID)
				stage, kind, attempt, derivation, activeProposal, pendingSpec := pendingAuthoringStage(row.DefinitionAuthoring)
				if ((stage == "derivation" && phase == "derivation") || (stage == "critic" && phase == "critic" && kind == workspace.GoalReviewVerifierFeasibility)) && attempt == 1 {
					writeTransientFailureResponse(t, w, "unexpected_eof", []byte(`{"partial":`))
					return
				}
				writeAuthoringFixtureSuccess(t, w, row.DefinitionAuthoring, stage, kind, derivation, activeProposal, pendingSpec, false)
			})
			defer h.closeAll()

			cancelCtx, cancel := context.WithCancel(h.ctx)
			defer cancel()
			if phase == "derivation" {
				h.biz.authoringRetryDelay = func(int) time.Duration {
					cancel()
					return 50 * time.Millisecond
				}
				if _, err := h.biz.DeriveDefinitionAcceptance(cancelCtx, h.input.WorkspaceID); !errors.Is(err, context.Canceled) {
					t.Fatalf("expected cancellation mid-backoff, got %v", err)
				}
				view, err := h.biz.ReadDefinitionAuthoringView(h.ctx, h.input.WorkspaceID)
				if err != nil || view.State != "deriving" {
					t.Fatalf("mid-backoff derivation state=%q want=deriving err=%v", view.State, err)
				}
			} else {
				if _, err := h.biz.DeriveDefinitionAcceptance(h.ctx, h.input.WorkspaceID); err != nil {
					t.Fatal(err)
				}
				h.biz.authoringRetryDelay = func(int) time.Duration {
					cancel()
					return 50 * time.Millisecond
				}
				if _, err := h.biz.CompileAuthoringSpecification(cancelCtx, h.input.WorkspaceID); !errors.Is(err, context.Canceled) {
					t.Fatalf("expected critic cancellation mid-backoff, got %v", err)
				}
				view, err := h.biz.ReadDefinitionAuthoringView(h.ctx, h.input.WorkspaceID)
				if err != nil || view.State != "reviewing" {
					t.Fatalf("mid-backoff critic state=%q want=reviewing err=%v", view.State, err)
				}
			}

			// Reopen fresh owners and resume cleanly without repeating already completed stages.
			callsBeforeRestart := h.calls.Load()
			h.closeOwners()
			h.openOwners()
			if _, err := h.biz.DeriveDefinitionAcceptance(h.ctx, h.input.WorkspaceID); err != nil {
				t.Fatal(err)
			}
			if _, err := h.biz.CompileAuthoringSpecification(h.ctx, h.input.WorkspaceID); err != nil {
				t.Fatal(err)
			}
			view, err := h.biz.ReadDefinitionAuthoringView(h.ctx, h.input.WorkspaceID)
			if err != nil || view.State != "ready_to_confirm" {
				t.Fatalf("resumed state=%q want=ready_to_confirm err=%v", view.State, err)
			}
			if gotCalls := h.calls.Load(); gotCalls != 9 {
				t.Fatalf("resumed total physical calls=%d want=9 (beforeRestart=%d)", gotCalls, callsBeforeRestart)
			}
		})
	}
}

func TestAuthoringGen1RevisionTransientRetryReachesReadyToConfirm(t *testing.T) {
	var h *authoringTransientHarness
	h = openAuthoringTransientHarness(t, false, func(w http.ResponseWriter, _ *http.Request) {
		row, _ := h.repositories.Workspaces.Get(h.ctx, h.input.WorkspaceID)
		stage, kind, attempt, derivation, activeProposal, pendingSpec := pendingAuthoringStage(row.DefinitionAuthoring)
		if stage == "proposal" && pendingSpec != nil && pendingSpec.PreviousProposalHash != "" && attempt == 1 {
			writeTransientFailureResponse(t, w, "unexpected_eof", []byte(`{"mapping":`))
			return
		}
		writeAuthoringFixtureSuccess(t, w, row.DefinitionAuthoring, stage, kind, derivation, activeProposal, pendingSpec, true)
	})
	defer h.closeAll()

	if _, err := h.biz.DeriveDefinitionAcceptance(h.ctx, h.input.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.biz.CompileAuthoringSpecification(h.ctx, h.input.WorkspaceID); err != nil {
		t.Fatalf("Gen1 revision transient retry failed: %v", err)
	}
	view, err := h.biz.ReadDefinitionAuthoringView(h.ctx, h.input.WorkspaceID)
	if err != nil || view.State != "ready_to_confirm" {
		t.Fatalf("Gen1 retried state=%q want=ready_to_confirm err=%v", view.State, err)
	}
	// 1 derive + 1 gen0 proposal + 6 gen0 critics + 2 gen1 proposals + 6 gen1 critics = 16 calls.
	if got := h.calls.Load(); got != 16 {
		t.Fatalf("Gen1 retried calls=%d want=16", got)
	}
}

type authoringUnresolvedDeliveryHub struct {
	magicevent.Hub
}

func (h authoringUnresolvedDeliveryHub) Send(ev magicevent.Event) magicevent.Result {
	result := h.Hub.Send(ev)
	if ev.ID() == llmevents.TopicLLMCommandCompleteProviderDocument {
		res := magicevent.NewResult(ev.ID(), ev.Source(), ev.Destination())
		res.Set(nil, def.NewError(def.Unexpected, "synthetic lost provider document reply"))
		return res
	}
	return result
}

func TestAuthoringNonTransientAndUnresolvedNeverRetry(t *testing.T) {
	for _, fault := range []string{"http_401", "http_400", "output_limit", "invalid_proposal", "invalid_review", "outcome_unresolved"} {
		t.Run(fault, func(t *testing.T) {
			var h *authoringTransientHarness
			h = openAuthoringTransientHarness(t, false, func(w http.ResponseWriter, _ *http.Request) {
				row, _ := h.repositories.Workspaces.Get(h.ctx, h.input.WorkspaceID)
				stage, kind, _, derivation, activeProposal, pendingSpec := pendingAuthoringStage(row.DefinitionAuthoring)
				if fault == "http_401" && stage == "derivation" {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusUnauthorized)
					_, _ = w.Write([]byte("unauthorized"))
					return
				}
				if fault == "http_400" && stage == "critic" && kind == workspace.GoalReviewVerifierFeasibility {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte("bad request"))
					return
				}
				if fault == "invalid_proposal" && stage == "proposal" {
					writeSSECompletedJSON(w, []byte(`{"invalid":true}`))
					return
				}
				if fault == "output_limit" && stage == "critic" && kind == workspace.GoalReviewVerifierFeasibility {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"{\"}\n\ndata: {\"type\":\"response.incomplete\",\"response\":{\"status\":\"incomplete\",\"incomplete_details\":{\"reason\":\"max_output_tokens\"}}}\n\n"))
					return
				}
				if fault == "invalid_review" && stage == "critic" && kind == workspace.GoalReviewVerifierFeasibility {
					writeSSECompletedJSON(w, []byte(`{"notReview":true}`))
					return
				}
				writeAuthoringFixtureSuccess(t, w, row.DefinitionAuthoring, stage, kind, derivation, activeProposal, pendingSpec, false)
			})
			defer h.closeAll()

			if fault == "outcome_unresolved" {
				h.biz.Base = basebiz.New(ModuleID, authoringUnresolvedDeliveryHub{Hub: h.hub}, nil)
				if _, err := h.biz.DeriveDefinitionAcceptance(h.ctx, h.input.WorkspaceID); err == nil {
					t.Fatal("unresolved delivery succeeded")
				}
				h.biz.Base = basebiz.New(ModuleID, h.hub, nil)
				view, err := h.biz.ReadDefinitionAuthoringView(h.ctx, h.input.WorkspaceID)
				if err != nil || view.State != "outcome_unresolved" || h.calls.Load() != 1 {
					t.Fatalf("outcome_unresolved state=%q calls=%d err=%v", view.State, h.calls.Load(), err)
				}
				if _, err := h.biz.DeriveDefinitionAcceptance(h.ctx, h.input.WorkspaceID); !errors.Is(err, common.ErrConflict) || h.calls.Load() != 1 {
					t.Fatalf("outcome_unresolved retried: calls=%d err=%v", h.calls.Load(), err)
				}
				return
			}

			wantCalls := map[string]int32{
				"http_401":         1,
				"invalid_proposal": 2,
				"http_400":         8,
				"output_limit":     8,
				"invalid_review":   8,
			}[fault]
			if fault == "http_401" {
				if _, err := h.biz.DeriveDefinitionAcceptance(h.ctx, h.input.WorkspaceID); err == nil {
					t.Fatal("http_401 succeeded")
				}
			} else {
				if _, err := h.biz.DeriveDefinitionAcceptance(h.ctx, h.input.WorkspaceID); err != nil {
					t.Fatal(err)
				}
				if _, err := h.biz.CompileAuthoringSpecification(h.ctx, h.input.WorkspaceID); err == nil {
					t.Fatalf("%s unexpectedly compiled", fault)
				}
			}
			view, err := h.biz.ReadDefinitionAuthoringView(h.ctx, h.input.WorkspaceID)
			if err != nil || view.State != "failed" || h.calls.Load() != wantCalls {
				t.Fatalf("%s state=%q calls=%d wantCalls=%d err=%v", fault, view.State, h.calls.Load(), wantCalls, err)
			}
		})
	}
}
