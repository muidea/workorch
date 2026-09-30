package biz

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/service/server"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	decisionowner "github.com/muidea/workorch/internal/modules/blocks/decision/biz"
	fileowner "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	workspaceowner "github.com/muidea/workorch/internal/modules/blocks/workspace/biz"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	management "github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 使用正式 HTTP ports 和真实持久 owners；仅 provider 与任务排队由测试控制。
func TestAuthoringHTTPFileBackedCompilationConfirmationAndRecovery(t *testing.T) {
	for _, scenario := range []string{"pass", "review_fail", "review_unknown", "resource_drift"} {
		t.Run(scenario, func(t *testing.T) { testAuthoringHTTPFileBackedScenario(t, scenario) })
	}
}

func testAuthoringHTTPFileBackedScenario(t *testing.T, scenario string) {
	ctx := context.Background()
	storage, directory := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "requirements.txt"), []byte("可验证的资源\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	repositories := workorchrepo.NewWorkspaceScopedFileRepositorySet(storage)
	hints, err := workorcheventpayload.FromMap(map[string]any{"profileContracts": map[string]any{"generic": map[string]any{"evidenceRequirements": []any{map[string]any{"id": "proof", "scope": "dodItem", "requiredEvidenceKinds": []any{"validation"}, "acceptedStepKinds": []any{"validation/managed_session"}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := repositories.WorkspaceTemplates.Create(ctx, &workspace.WorkspaceTemplate{ID: "http-template", Name: "模板", DoDItems: []string{"模板验收"}, ContextHints: hints}); err != nil {
		t.Fatal(err)
	}
	var physicalCalls atomic.Int32
	open := func() (string, *authoringTestRoutine, func()) {
		t.Helper()
		hub := magicevent.NewHub(32)
		owners := workorchrepo.NewWorkspaceScopedFileRepositorySet(storage)
		owner, err := workspaceowner.New(hub, nil, workspaceowner.Dependencies{OwnerRepositories: owners.WorkspaceOwner, EventHub: hub})
		if err != nil {
			t.Fatal(err)
		}
		if err := owner.Start(); err != nil {
			t.Fatal(err)
		}
		decision, err := decisionowner.New(hub, nil, decisionowner.Dependencies{OwnerRepositories: owners.DecisionOwner, EventHub: hub})
		if err != nil {
			t.Fatal(err)
		}
		if err := decision.Start(); err != nil {
			t.Fatal(err)
		}
		file, err := fileowner.New(hub, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := file.Start(); err != nil {
			t.Fatal(err)
		}
		provider := magicevent.NewSimpleObserver(llmevents.CommandDestination, hub)
		target := runtimecontract.ProviderModelTarget{ProviderID: "mock", ModelID: "mock"}
		if err := provider.Subscribe(llmevents.TopicLLMCommandGetActiveConfiguration, func(_ magicevent.Event, result magicevent.Result) {
			result.Set(llmevents.GetActiveConfigurationResult{ConfigurationRevision: "http-provider", DefaultChat: target}, nil)
		}); err != nil {
			t.Fatal(err)
		}
		if err := provider.Subscribe(llmevents.TopicLLMCommandCompleteProvider, func(ev magicevent.Event, result magicevent.Result) {
			command := ev.Data().(llmevents.CompleteProviderCommand)
			all, err := owners.Workspaces.List(ctx)
			if err != nil || len(all) != 1 {
				t.Errorf("provider workspace: %v", err)
				return
			}
			record := all[0].DefinitionAuthoring
			if record == nil {
				t.Error("provider called before persisted input")
				return
			}
			var payload []byte
			switch command.Request.Kind {
			case "workspace/specification/derive":
				if len(record.DerivationRequests) != 1 || len(record.DerivationRequests[0].Resources) != 1 {
					t.Error("provider called before File snapshot and permit")
					return
				}
				payload, err = json.Marshal(workspace.DefinitionDerivedAcceptance{AuthoringInputHash: record.InputHash, TemplateSnapshotHash: record.TemplateSnapshotHash, AdditionalItems: []string{"派生验收"}})
			case "workspace/specification/proposal":
				if len(record.SpecificationRequests) != 1 {
					t.Error("proposal permit not durable")
					return
				}
				proposal := authoringHTTPProposal(t, record)
				payload, err = json.Marshal(struct {
					Specification workspace.GoalSpecification `json:"specification"`
				}{proposal})
			default:
				kind := strings.TrimPrefix(command.Request.Kind, "workspace/specification/review/")
				proposal := record.SpecificationRequests[0].Proposal
				if proposal == nil || len(record.SpecificationRequests) != int(physicalCalls.Load()) {
					t.Error("critic permit not durable")
					return
				}
				if strings.Contains(command.Request.PromptContext.Sections[1].Content, `"Reviews":[{`) {
					t.Error("critic received earlier reviews")
				}
				review := workspace.GoalSpecificationCompletenessReview{Kind: kind, Status: workspace.GoalReviewPass, SpecificationHash: proposal.ContentHash(), Reason: "独立覆盖", EvidenceRefs: []string{workspace.DefinitionAuthoringGoalItemRef}, RequirementIDs: []string{"goal"}}
				if kind == workspace.GoalReviewVerifierFeasibility {
					switch scenario {
					case "review_fail":
						review.Status = workspace.GoalReviewFail
					case "review_unknown":
						review.Status = workspace.GoalReviewUnknown
					}
				}
				payload, err = json.Marshal(struct {
					Review workspace.GoalSpecificationCompletenessReview `json:"review"`
				}{review})
			}
			if err != nil {
				t.Error(err)
				return
			}
			physicalCalls.Add(1)
			result.Set(llmevents.CompleteProviderResult{Target: target, Completion: llmevents.ProviderCompletionData{Content: string(payload)}}, nil)
		}); err != nil {
			t.Fatal(err)
		}
		routine := &authoringTestRoutine{}
		application := &Biz{Base: basebiz.New(ModuleID, hub, routine), authoringTasks: newAuthoringTaskLifecycle()}
		component, err := newRuntimeComponentFromCommandHub(hub, routine)
		if err != nil {
			t.Fatal(err)
		}
		component.session.SetRepositoryConfig("file", storage)
		ports := component.applicationPorts()
		application.configureAuthoringBackgroundPorts(&ports)
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		address := listener.Addr().String()
		if err := listener.Close(); err != nil {
			t.Fatal(err)
		}
		transport := server.NewHTTPServerWithApplicationPorts(server.Config{Addr: address, Backend: "file", DataRoot: storage}, ports, routine)
		finished := make(chan error, 1)
		go func() { finished <- transport.Start() }()
		baseURL := "http://" + address
		client := &http.Client{Timeout: time.Second}
		deadline := time.Now().Add(5 * time.Second)
		for {
			response, err := client.Get(baseURL + "/healthz")
			if err == nil {
				response.Body.Close()
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("HTTP listener did not start")
			}
			time.Sleep(10 * time.Millisecond)
		}
		return baseURL, routine, func() {
			shutdown, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			if err := transport.Shutdown(shutdown); err != nil {
				t.Error(err)
			}
			application.authoringTasks.stop()
			if err := <-finished; err != nil {
				t.Error(err)
			}
			if err := component.close(shutdown); err != nil {
				t.Error(err)
			}
			provider.Unsubscribe(llmevents.TopicLLMCommandCompleteProvider)
			provider.Unsubscribe(llmevents.TopicLLMCommandGetActiveConfiguration)
			file.Teardown(ctx)
			decision.Teardown(ctx)
			owner.Teardown(ctx)
			hub.Terminate(ctx)
		}
	}
	baseURL, routine, closeOwners := open()
	defer func() { closeOwners() }()
	request := management.DefinitionAuthoringCreate{RequestID: "http-file-recovery", Title: "HTTP 创作", TemplateID: "http-template", DeliverableDirectory: directory, Input: management.DefinitionAuthoringInput{GoalText: "交付目标", AcceptanceRequirements: []string{"用户验收"}, ResourceRefs: []string{"requirements.txt"}}}
	assertNoRuns := func() {
		t.Helper()
		runs, err := repositories.Runs.List(ctx)
		if err != nil || len(runs) != 0 {
			t.Fatalf("创作链路自动创建 Run: %v %v", runs, err)
		}
	}
	created := authoringHTTPRequest(t, baseURL, http.MethodPost, "/api/v1/workorch/workspace-authoring", request, http.StatusCreated)
	assertNoRuns()
	path := "/api/v1/workorch/workspaces/" + created.WorkspaceID + "/authoring"
	replay := authoringHTTPRequest(t, baseURL, http.MethodPost, "/api/v1/workorch/workspace-authoring", request, http.StatusCreated)
	if replay.InputHash != created.InputHash || replay.WorkspaceRevision != created.WorkspaceRevision {
		t.Fatal("create replay changed identity")
	}
	accepted := authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/compile", nil, http.StatusAccepted)
	if !accepted.CompilationAccepted || accepted.State != "draft" || physicalCalls.Load() != 0 {
		t.Fatal("acceptance fabricated owner progress")
	}
	routine.queued.Run()
	ready := authoringHTTPRequest(t, baseURL, http.MethodGet, path, nil, http.StatusOK)
	wantState := "ready_to_confirm"
	if scenario == "review_fail" || scenario == "review_unknown" {
		wantState = "review_blocked"
	}
	if ready.State != wantState || ready.Proposal == nil || len(ready.Proposal.Reviews) != 6 || len(ready.Proposal.Items) != 3 || physicalCalls.Load() != 8 {
		t.Fatalf("compile: %+v calls=%d", ready, physicalCalls.Load())
	}
	for _, review := range ready.Proposal.Reviews {
		if review.Status != "PASS" && wantState != "review_blocked" {
			t.Fatal("non-PASS review")
		}
	}
	assertNoRuns()
	confirmation := management.DefinitionAuthoringConfirm{InputHash: ready.InputHash, DerivationPermitID: ready.DerivationPermitID, ProposalHash: ready.Proposal.ProposalHash, SpecificationHash: ready.Proposal.SpecificationHash, ExpectedWorkspaceRevision: ready.WorkspaceRevision}
	if wantState == "review_blocked" {
		authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/confirm", confirmation, http.StatusConflict)
		assertNoRuns()
		closeOwners()
		baseURL, routine, closeOwners = open()
		recovered := authoringHTTPRequest(t, baseURL, http.MethodGet, path, nil, http.StatusOK)
		if recovered.State != "review_blocked" || recovered.WorkspaceRevision != ready.WorkspaceRevision {
			t.Fatalf("阻塞审查重启后丢失: %+v", recovered)
		}
		authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/compile", nil, http.StatusOK)
		stored, err := repositories.Workspaces.Get(ctx, common.WorkspaceID(created.WorkspaceID))
		if err != nil || stored.StartupInput != nil || stored.ActivationAdmission != nil || stored.DefinitionAuthoringConfirmation != nil || physicalCalls.Load() != 8 {
			t.Fatalf("阻塞审查获得准入或重发 provider: %+v %v", stored, err)
		}
		assertNoRuns()
		return
	}
	if scenario == "resource_drift" {
		if err := os.WriteFile(filepath.Join(directory, "requirements.txt"), []byte("漂移资源\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/confirm", confirmation, http.StatusConflict)
		stored, err := repositories.Workspaces.Get(ctx, common.WorkspaceID(created.WorkspaceID))
		if err != nil || stored.StartupInput != nil || stored.ActivationAdmission != nil || stored.DefinitionAuthoringConfirmation != nil {
			t.Fatalf("资源漂移绕过准入: %+v %v", stored, err)
		}
		assertNoRuns()
		if err := os.WriteFile(filepath.Join(directory, "requirements.txt"), []byte("可验证的资源\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	stale := confirmation
	stale.InputHash = "stale"
	authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/confirm", stale, http.StatusConflict)
	admitted := authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/confirm", confirmation, http.StatusOK)
	if admitted.State != "admitted" || admitted.ActivationState != "eligible" || admitted.StartupInputHash == "" {
		t.Fatalf("admission: %+v", admitted)
	}
	assertNoRuns()
	closeOwners()
	baseURL, routine, closeOwners = open()
	recovered := authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/confirm", confirmation, http.StatusOK)
	if recovered.WorkspaceRevision != admitted.WorkspaceRevision || recovered.StartupInputHash != admitted.StartupInputHash {
		t.Fatal("exact confirmation replay changed persisted admission")
	}
	authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/compile", nil, http.StatusOK)
	if physicalCalls.Load() != 8 {
		t.Fatal("restart repeated provider calls")
	}
	stored, err := repositories.Workspaces.Get(ctx, common.WorkspaceID(created.WorkspaceID))
	if err != nil || stored.ActiveRunID != "" || stored.DefinitionAuthoringConfirmation == nil {
		t.Fatalf("persisted workspace: %+v %v", stored, err)
	}
	runs, err := repositories.Runs.List(ctx)
	if err != nil || len(runs) != 0 {
		t.Fatalf("authoring created Run: %v %v", runs, err)
	}
}

func authoringHTTPRequest(t *testing.T, baseURL, method, path string, body any, status int) management.DefinitionAuthoringView {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(method, baseURL+path, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 180 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != status {
		var detail json.RawMessage
		_ = json.NewDecoder(response.Body).Decode(&detail)
		t.Fatalf("%s %s: status=%d want=%d body=%s", method, path, response.StatusCode, status, detail)
	}
	if status != http.StatusOK && status != http.StatusCreated && status != http.StatusAccepted {
		return management.DefinitionAuthoringView{}
	}
	var envelope struct {
		Authoring management.DefinitionAuthoringView `json:"authoring"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	return envelope.Authoring
}

func authoringHTTPProposal(t *testing.T, record *workspace.DefinitionAuthoringRecord) workspace.GoalSpecification {
	t.Helper()
	derivation := record.DerivationRequests[0]
	items, err := derivation.Proposal.CandidateItems(record.Input, &record.TemplateSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	proposal := workspace.GoalSpecification{Version: 1, GoalItemRef: workspace.DefinitionAuthoringGoalItemRef, GoalTextHash: workspace.StableDefinitionTextHash(record.Input.GoalText), Dimensions: []workspace.GoalSpecificationDimension{{ID: "dimension", Required: true, Activated: true}}, Requirements: []workspace.GoalSpecificationRequirement{{ID: "goal", DimensionID: "dimension", Required: true, SourceKind: workspace.GoalRequirementUserExplicit, SourceItemRefs: []string{workspace.DefinitionAuthoringGoalItemRef}, Text: record.Input.GoalText}}}
	for index, item := range items {
		kind := workspace.GoalRequirementDerived
		if index == 0 {
			kind = workspace.GoalRequirementUserExplicit
		} else if index == 1 {
			kind = workspace.GoalRequirementTemplateRequired
		}
		id := fmt.Sprintf("item-%d", index)
		proposal.Requirements = append(proposal.Requirements, workspace.GoalSpecificationRequirement{ID: id, DimensionID: "dimension", Required: true, SourceKind: kind, SourceItemRefs: []string{item.CandidateItemRef}, Text: item.Text})
		proposal.Obligations = append(proposal.Obligations, workspace.GoalSpecificationObligation{ID: "obligation-" + id, RequirementID: id, DoDItemRefs: []string{item.CandidateItemRef}, Text: item.Text})
	}
	proposal.Obligations = append(proposal.Obligations, workspace.GoalSpecificationObligation{ID: "goal-obligation", RequirementID: "goal", DoDItemRefs: []string{items[0].CandidateItemRef}, Text: "目标验收"})
	for _, obligation := range proposal.Obligations {
		proposal.Verifications = append(proposal.Verifications, workspace.GoalSpecificationVerification{ID: "verify-" + obligation.ID, ObligationID: obligation.ID, Declaration: "按模板验证", Status: workspace.GoalVerificationDeclared, ProfileID: "generic", EvidenceRequirementID: "proof", Scope: "dodItem"})
	}
	proposal.Resources, err = workspace.AuthoringSpecificationResources(record.Input, derivation.Resources)
	if err != nil {
		t.Fatal(err)
	}
	return proposal
}
