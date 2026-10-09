package biz

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/modules/application/workorchd/service/server"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	decisionowner "github.com/muidea/workorch/internal/modules/blocks/decision/biz"
	fileowner "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	workspaceowner "github.com/muidea/workorch/internal/modules/blocks/workspace/biz"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/testkit"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	management "github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 使用正式 HTTP ports 和真实持久 owners；仅 provider 与任务排队由测试控制。
func TestAuthoringHTTPFileBackedCompilationConfirmationAndRecovery(t *testing.T) {
	for _, scenario := range []string{"pass", "review_fail", "review_unknown", "review_missing_required", "resource_drift", "invalid_derivation", "invalid_proposal", "invalid_review"} {
		t.Run(scenario, func(t *testing.T) { testAuthoringHTTPFileBackedScenario(t, scenario) })
	}
}

func TestAuthoringHTTPPreparationFailureFileRecovery(t *testing.T) {
	for _, scenario := range []string{"resource_missing", "config_failure", "proposal_config_failure", "review_preparation_failure", "review_fail_preparation_failure", "review_unknown_preparation_failure", "render_failure", "request_budget", "attempt_budget"} {
		t.Run(scenario, func(t *testing.T) { testAuthoringHTTPFileBackedScenario(t, scenario) })
	}
}

func TestAuthoringHTTPRealBackgroundRoutineFileRecovery(t *testing.T) {
	testAuthoringHTTPFileBackedScenario(t, "real_background")
}

func TestAuthoringHTTPConcurrentCompileUsesOnePhysicalDerivation(t *testing.T) {
	testAuthoringHTTPFileBackedScenario(t, "concurrent_compile")
}

type authoringConcurrentRoutine struct {
	task.BackgroundRoutine
	queued []task.Task
}

func (r *authoringConcurrentRoutine) AsyncTaskContext(_ context.Context, work task.Task) error {
	r.queued = append(r.queued, work)
	return nil
}

func TestAuthoringHTTPCompileTasksCompeteForFreshPermit(t *testing.T) {
	testAuthoringHTTPFileBackedScenario(t, "reserve_compete")
}

func testAuthoringHTTPFileBackedScenario(t *testing.T, scenario string) {
	ctx := context.Background()
	storage, directory := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "requirements.txt"), []byte("可验证的资源\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if scenario == "render_failure" {
		// 模板和资源各自合法，但组装后的请求超过 render 文档边界。
		if err := os.WriteFile(filepath.Join(directory, "requirements.txt"), []byte(strings.Repeat("r", 30*1024)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if scenario == "resource_missing" {
		if err := os.Remove(filepath.Join(directory, "requirements.txt")); err != nil {
			t.Fatal(err)
		}
	}
	repositories := workorchrepo.NewWorkspaceScopedFileRepositorySet(storage)
	hints, err := workorcheventpayload.FromMap(map[string]any{"profileContracts": map[string]any{"generic": map[string]any{"evidenceRequirements": []any{map[string]any{"id": "*", "scope": "dodItem", "requiredEvidenceKinds": []any{"validation"}, "acceptedStepKinds": []any{"validation/managed_session"}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	template := workspace.WorkspaceTemplate{ID: "http-template", Name: "模板", DoDItems: []string{"模板验收"}, ContextHints: hints}
	if scenario == "render_failure" {
		template.DoDItems = append(template.DoDItems, strings.Repeat("模板原文", 20*1024))
	}
	if err := repositories.WorkspaceTemplates.Create(ctx, &template); err != nil {
		t.Fatal(err)
	}
	var preparationRepaired atomic.Bool
	configurationCause := def.NewError(def.Unexpected, "mock 配置读取失败\r\n原文 sk_test_marker\n")
	var physicalCalls atomic.Int32
	var missingReviewPayload []byte
	var missingRequirementID string
	realWorker := scenario == "real_background" || scenario == "concurrent_compile"
	deriveStarted, releaseDerive := make(chan struct{}), make(chan struct{})
	var concurrentRoutine *authoringConcurrentRoutine
	var startedOnce, releaseOnce sync.Once
	if scenario == "concurrent_compile" {
		t.Cleanup(func() { releaseOnce.Do(func() { close(releaseDerive) }) })
	}
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
			fail := !preparationRepaired.Load() && (scenario == "config_failure" || scenario == "proposal_config_failure" && physicalCalls.Load() >= 1 || (scenario == "review_preparation_failure" || scenario == "review_fail_preparation_failure" || scenario == "review_unknown_preparation_failure") && physicalCalls.Load() >= 3)
			if fail {
				result.Set(nil, configurationCause)
				return
			}
			result.Set(llmevents.GetActiveConfigurationResult{ConfigurationRevision: "http-provider", DefaultChat: target}, nil)
		}); err != nil {
			t.Fatal(err)
		}
		if err := provider.Subscribe(llmevents.TopicLLMCommandCompleteProviderDocument, func(ev magicevent.Event, result magicevent.Result) {
			command := ev.Data().(llmevents.CompleteProviderDocumentCommand)
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
				if scenario == "concurrent_compile" {
					startedOnce.Do(func() { close(deriveStarted) })
					<-releaseDerive
				}
				if len(record.DerivationRequests) != 1 || len(record.DerivationRequests[0].Resources) != 1 {
					t.Error("provider called before File snapshot and permit")
					return
				}
				payload, err = json.Marshal(workspace.DefinitionDerivedAcceptance{AuthoringInputHash: record.InputHash, TemplateSnapshotHash: record.TemplateSnapshotHash, AdditionalItems: []string{"派生验收"}})
				if scenario == "invalid_derivation" {
					payload = []byte("  {\"providerControlled\": true}\n")
				}
			case "workspace/specification/proposal":
				if scenario == "proposal_config_failure" || scenario == "review_preparation_failure" || scenario == "review_fail_preparation_failure" || scenario == "review_unknown_preparation_failure" {
					if len(record.SpecificationRequests) < 1 {
						t.Error("proposal permit not durable")
						return
					}
				} else if len(record.SpecificationRequests) != 1 {
					t.Error("proposal permit not durable")
					return
				}
				mapping := authoringHTTPMapping(t, record)
				payload, err = json.Marshal(struct {
					Mapping workspace.DefinitionAuthoringMapping `json:"mapping"`
				}{mapping})
				if scenario == "invalid_proposal" {
					payload = []byte("  {\"providerControlled\": true}\n")
				}
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
				review := workspace.GoalSpecificationCompletenessReview{Kind: kind, Status: workspace.GoalReviewPass, SpecificationHash: proposal.ContentHash(), Reason: "独立覆盖", EvidenceRefs: []string{workspace.DefinitionAuthoringGoalItemRef}, RequirementIDs: authoringHTTPRequiredReviewRequirementIDs(t, *proposal, kind)}
				if scenario == "review_missing_required" && kind == workspace.GoalReviewVerifierFeasibility {
					goalRequirement := authoringHTTPGoalRequirementID(t, *proposal)
					missingRequirementID = goalRequirement
					review.RequirementIDs = review.RequirementIDs[:0]
					for _, id := range authoringHTTPRequiredReviewRequirementIDs(t, *proposal, kind) {
						if id != goalRequirement {
							review.RequirementIDs = append(review.RequirementIDs, id)
						}
					}
				}
				if physicalCalls.Load() == 2 {
					switch scenario {
					case "review_fail_preparation_failure":
						review.Status = workspace.GoalReviewFail
					case "review_unknown_preparation_failure":
						review.Status = workspace.GoalReviewUnknown
					}
				}
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
				if scenario == "review_missing_required" && kind == workspace.GoalReviewVerifierFeasibility {
					missingReviewPayload = append([]byte(nil), payload...)
				}
				if scenario == "invalid_review" {
					payload = []byte("  {\"未知审查字段\":\"sk_test_marker\"}\r\n")
				}
			}
			if err != nil {
				t.Error(err)
				return
			}
			physicalCalls.Add(1)
			result.Set(authoringTestDocumentResult(t, command, payload), nil)
		}); err != nil {
			t.Fatal(err)
		}
		routine := &authoringTestRoutine{}
		var background task.BackgroundRoutine = routine
		if scenario == "reserve_compete" {
			concurrentRoutine = &authoringConcurrentRoutine{}
			background = concurrentRoutine
		}
		if realWorker {
			background = task.NewBackgroundRoutine(2)
		}
		application := &Biz{Base: basebiz.New(ModuleID, hub, background), authoringTasks: newAuthoringTaskLifecycle()}
		component, err := newRuntimeComponentFromCommandHub(hub, background)
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
		transport := server.NewHTTPServerWithApplicationPorts(server.Config{Addr: address, Backend: "file", DataRoot: storage}, ports, background)
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
			if realWorker && !background.Shutdown(shutdown) {
				t.Error("真实后台编译 worker 未停止")
			}
			if err := <-finished; err != nil {
				t.Error(err)
			}
			if err := component.close(shutdown); err != nil {
				t.Error(err)
			}
			provider.Unsubscribe(llmevents.TopicLLMCommandCompleteProviderDocument)
			provider.Unsubscribe(llmevents.TopicLLMCommandGetActiveConfiguration)
			file.Teardown(ctx)
			decision.Teardown(ctx)
			owner.Teardown(ctx)
			hub.Terminate(ctx)
		}
	}
	baseURL, routine, closeOwners := open()
	defer func() {
		if scenario == "concurrent_compile" {
			releaseOnce.Do(func() { close(releaseDerive) })
		}
		closeOwners()
	}()
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
	save := management.DefinitionAuthoringSave{RequestID: "save-http", TemplateID: request.TemplateID, Input: request.Input, ExpectedWorkspaceRevision: created.WorkspaceRevision, ExpectedInputHash: created.InputHash}
	save.Input.GoalText = "修订后的交付目标"
	updated := authoringHTTPRequest(t, baseURL, http.MethodPut, path, save, http.StatusOK)
	if updated.InputRevision != 2 || updated.InputHash == created.InputHash {
		t.Fatal("修订未形成新的输入身份")
	}
	closeOwners()
	baseURL, routine, closeOwners = open()
	saveReplay := authoringHTTPRequest(t, baseURL, http.MethodPut, path, save, http.StatusOK)
	if saveReplay.InputHash != updated.InputHash || saveReplay.WorkspaceRevision != updated.WorkspaceRevision {
		t.Fatal("保存响应丢失重放再次写入 revision")
	}
	conflictingSave := save
	conflictingSave.Input.GoalText = "过期不同输入"
	authoringHTTPRequest(t, baseURL, http.MethodPut, path, conflictingSave, http.StatusConflict)
	assertNoRuns()
	if scenario == "request_budget" || scenario == "attempt_budget" {
		stored, err := repositories.Workspaces.Get(ctx, common.WorkspaceID(created.WorkspaceID))
		if err != nil {
			t.Fatal(err)
		}
		record := stored.DefinitionAuthoring.Clone()
		count := workspace.DefinitionDerivationRequestLimit
		if scenario == "attempt_budget" {
			count = workspace.DefinitionDerivationAttemptLimit
		}
		content := "可验证的资源\n"
		sum := sha256.Sum256([]byte(content))
		resources := []workspace.DefinitionAuthoringResourceSnapshot{{Ref: "requirements.txt", Content: content, ContentHash: "sha256:" + hex.EncodeToString(sum[:])}}
		configuration := llmevents.GetActiveConfigurationResult{ConfigurationRevision: "http-provider", DefaultChat: runtimecontract.ProviderModelTarget{ProviderID: "mock", ModelID: "mock"}}
		for n := range count {
			input := record.Input.Clone()
			if scenario == "request_budget" {
				input.Revision = 1
				input.GoalText = fmt.Sprintf("历史目标-%d", n)
			}
			request := workspace.DefinitionDerivationRequest{RequestID: fmt.Sprintf("historical-%d", n), Input: input, Template: record.TemplateSnapshot.Clone(), Resources: resources, ResourceSnapshotHash: workspace.DefinitionAuthoringResourcesHash(resources), ProviderBindingHash: definitionProviderBindingHash(configuration), FailureCode: "provider_failed"}
			request.PermitID = request.ExpectedPermitID()
			record.DerivationRequests = append(record.DerivationRequests, request)
		}
		if err := record.Validate(); err != nil {
			t.Fatal(err)
		}
		stored.DefinitionAuthoring = &record
		if err := repositories.Workspaces.CompareAndSwap(ctx, stored, stored.Revision); err != nil {
			t.Fatal(err)
		}
	}
	accepted := authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/compile", nil, http.StatusAccepted)
	if !accepted.CompilationAccepted || accepted.State != "draft" || (!realWorker && physicalCalls.Load() != 0) {
		t.Fatal("acceptance fabricated owner progress")
	}
	if scenario == "concurrent_compile" {
		select {
		case <-deriveStarted:
		case <-time.After(5 * time.Second):
			t.Fatal("并发编译派生调用未进入 barrier")
		}
		// 派生物理调用仍在途时，重复请求只能复用 owner permit。
		for range 4 {
			authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/compile", nil, http.StatusOK)
		}
		releaseOnce.Do(func() { close(releaseDerive) })
	}
	if scenario == "reserve_compete" {
		for range 3 {
			authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/compile", nil, http.StatusAccepted)
		}
		if len(concurrentRoutine.queued) != 4 {
			t.Fatalf("竞争夹具任务数=%d，want 4", len(concurrentRoutine.queued))
		}
		start := make(chan struct{})
		var workers sync.WaitGroup
		for _, work := range concurrentRoutine.queued {
			workers.Add(1)
			go func(work task.Task) {
				defer workers.Done()
				<-start
				work.Run()
			}(work)
		}
		close(start)
		workers.Wait()
	} else if !realWorker {
		routine.queued.Run()
	}
	if scenario == "resource_missing" || scenario == "config_failure" || scenario == "proposal_config_failure" || scenario == "review_preparation_failure" || scenario == "review_fail_preparation_failure" || scenario == "review_unknown_preparation_failure" || scenario == "render_failure" || scenario == "request_budget" || scenario == "attempt_budget" {
		failed := authoringHTTPRequest(t, baseURL, http.MethodGet, path, nil, http.StatusOK)
		stage, phase := "derivation", "config"
		wantCalls := int32(0)
		switch scenario {
		case "resource_missing":
			phase = "resources"
		case "proposal_config_failure":
			stage, wantCalls = "proposal", 1
		case "review_preparation_failure", "review_fail_preparation_failure", "review_unknown_preparation_failure":
			stage, wantCalls = "review", 3
		case "render_failure":
			phase = "render"
		case "request_budget", "attempt_budget":
			phase = scenario
		}
		if failed.State != "preparation_failed" || len(failed.PreparationDiagnostics) != 1 || physicalCalls.Load() != wantCalls {
			t.Fatalf("pre-reserve failure hidden or permit issued: %+v calls=%d", failed, physicalCalls.Load())
		}
		diagnostic := failed.PreparationDiagnostics[0]
		if diagnostic.Stage != stage || diagnostic.PhaseCode != phase || !diagnostic.Current || !diagnostic.CauseSaved || diagnostic.CauseBytes == 0 || diagnostic.CauseSHA256 == "" || (scenario == "resource_missing" && diagnostic.ResourceRef != "requirements.txt") {
			t.Fatalf("preparation metadata incomplete: %+v", diagnostic)
		}
		var envelope struct {
			Diagnostic management.DefinitionAuthoringPreparationDiagnosticView `json:"diagnostic"`
		}
		response, err := http.Get(baseURL + path + "/preparation-diagnostics/" + diagnostic.DiagnosticID)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK || response.Header.Get("Cache-Control") != "no-store" {
			response.Body.Close()
			t.Fatalf("preparation raw query status=%d headers=%v", response.StatusCode, response.Header)
		}
		if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
			response.Body.Close()
			t.Fatal(err)
		}
		response.Body.Close()
		raw, err := base64.StdEncoding.DecodeString(envelope.Diagnostic.CauseBase64)
		if err != nil || uint64(len(raw)) != diagnostic.CauseBytes || envelope.Diagnostic.CauseSHA256 != diagnostic.CauseSHA256 {
			t.Fatalf("preparation raw integrity: %+v err=%v", envelope.Diagnostic, err)
		}
		if phase == "config" && !bytes.Equal(raw, []byte(configurationCause.Error())) {
			t.Fatalf("configuration original cause rewritten: %q", raw)
		}
		stored, err := repositories.Workspaces.Get(ctx, common.WorkspaceID(created.WorkspaceID))
		if err != nil || stored.DefinitionAuthoring == nil || len(stored.DefinitionAuthoring.PreparationDiagnostics) != 1 || !bytes.Equal(stored.DefinitionAuthoring.PreparationDiagnostics[0].Cause, raw) {
			t.Fatalf("owner original cause mismatch: %+v %v", stored, err)
		}
		if len(stored.DefinitionAuthoring.SpecificationRequests) != int(max(0, wantCalls-1)) {
			t.Fatal("preparation failure fabricated specification permit")
		}
		assertNoRuns()
		// 相同显式失败幂等，但仍可接受任务；诊断身份不含
		// 已推进到当前值的 observed revision。
		again := authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/compile", nil, http.StatusAccepted)
		if !again.CompilationAccepted || again.State != "preparation_failed" {
			t.Fatal("explicit preparation retry was not accepted")
		}
		routine.queued.Run()
		duplicate := authoringHTTPRequest(t, baseURL, http.MethodGet, path, nil, http.StatusOK)
		if duplicate.WorkspaceRevision != failed.WorkspaceRevision || len(duplicate.PreparationDiagnostics) != 1 || duplicate.PreparationDiagnostics[0].DiagnosticID != diagnostic.DiagnosticID || physicalCalls.Load() != wantCalls {
			t.Fatalf("duplicate preparation cause advanced owner revision: %+v", duplicate)
		}
		closeOwners()
		baseURL, routine, closeOwners = open()
		recovered := authoringHTTPRequest(t, baseURL, http.MethodGet, path, nil, http.StatusOK)
		if recovered.State != "preparation_failed" || recovered.WorkspaceRevision != failed.WorkspaceRevision || recovered.PreparationDiagnostics[0].DiagnosticID != diagnostic.DiagnosticID {
			t.Fatal("preparation history lost across file-owner restart")
		}
		if scenario == "render_failure" || scenario == "request_budget" || scenario == "attempt_budget" {
			return
		}
		if scenario == "resource_missing" {
			if err := os.WriteFile(filepath.Join(directory, "requirements.txt"), []byte("可验证的资源\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		preparationRepaired.Store(true)
		authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/compile", nil, http.StatusAccepted)
		routine.queued.Run()
		ready := authoringHTTPRequest(t, baseURL, http.MethodGet, path, nil, http.StatusOK)
		wantState := "ready_to_confirm"
		if scenario == "review_fail_preparation_failure" || scenario == "review_unknown_preparation_failure" {
			wantState = "review_blocked"
		}
		if ready.State != wantState || ready.InputHash != failed.InputHash || ready.Proposal == nil || len(ready.Proposal.Reviews) != 6 || physicalCalls.Load() != 8 || ready.PreparationDiagnostics[0].Current {
			t.Fatalf("explicit same-input recovery failed or repeated completed slots: %+v calls=%d", ready, physicalCalls.Load())
		}
		if wantState == "review_blocked" {
			confirmation := management.DefinitionAuthoringConfirm{InputHash: ready.InputHash, DerivationPermitID: ready.DerivationPermitID, ProposalHash: ready.Proposal.ProposalHash, SpecificationHash: ready.Proposal.SpecificationHash, ExpectedWorkspaceRevision: ready.WorkspaceRevision}
			authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/confirm", confirmation, http.StatusConflict)
			authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/compile", nil, http.StatusOK)
			if physicalCalls.Load() != 8 {
				t.Fatal("完整阻塞审查重复编译重发了旧 critics")
			}
		}
		assertNoRuns()
		return
	}
	if scenario == "invalid_derivation" || scenario == "invalid_proposal" || scenario == "invalid_review" || scenario == "review_missing_required" {
		failed := authoringHTTPRequest(t, baseURL, http.MethodGet, path, nil, http.StatusOK)
		requestIndex := len(failed.Requests) - 1
		failureCode := scenario
		if scenario == "review_missing_required" {
			failureCode = "invalid_review"
		}
		if scenario == "invalid_derivation" {
			failureCode = "invalid_proposal"
			if physicalCalls.Load() != 1 || failed.DerivationPermitID == "" || len(failed.Requests) != 1 || failed.Requests[0].Phase != "derivation" || failed.Requests[0].PermitID != failed.DerivationPermitID {
				t.Fatalf("派生失败继续调用或丢失诊断身份: %+v calls=%d", failed, physicalCalls.Load())
			}
		}
		if failed.State != "failed" || requestIndex < 0 || failed.Requests[requestIndex].FailureCode != failureCode || failed.Requests[requestIndex].Diagnostic == nil || failed.Requests[requestIndex].Diagnostic.ParseError != "" || failed.Requests[requestIndex].Diagnostic.ParseErrorBytes == 0 || !failed.Requests[requestIndex].Diagnostic.ParseErrorSaved {
			t.Fatalf("invalid proposal was not durably diagnosed: %+v", failed)
		}
		var envelope struct {
			Diagnostic management.DefinitionAuthoringDiagnosticView `json:"diagnostic"`
		}
		response, err := http.Get(baseURL + path + "/diagnostics/" + failed.Requests[requestIndex].PermitID)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			t.Fatalf("diagnostic status %d: %s", response.StatusCode, body)
		}
		if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
			response.Body.Close()
			t.Fatal(err)
		}
		response.Body.Close()
		raw := []byte("  {\"providerControlled\": true}\n")
		if scenario == "invalid_review" {
			raw = []byte("  {\"未知审查字段\":\"sk_test_marker\"}\r\n")
		}
		if scenario == "review_missing_required" {
			raw = missingReviewPayload
			if len(raw) == 0 {
				t.Fatal("missing-required review payload was not captured")
			}
		}
		if envelope.Diagnostic.ParseError == "" || envelope.Diagnostic.ParseErrorSHA256 != failed.Requests[requestIndex].Diagnostic.ParseErrorSHA256 || envelope.Diagnostic.ParseErrorBytes != failed.Requests[requestIndex].Diagnostic.ParseErrorBytes || envelope.Diagnostic.PayloadBase64 != base64.StdEncoding.EncodeToString(raw) || envelope.Diagnostic.PayloadBytes != uint64(len(raw)) || !envelope.Diagnostic.PayloadSaved {
			t.Fatalf("raw diagnostic changed: %+v", envelope.Diagnostic)
		}
		before := physicalCalls.Load()
		replayed := authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/compile", nil, http.StatusOK)
		if physicalCalls.Load() != before || replayed.CompilationAccepted || replayed.WorkspaceRevision != failed.WorkspaceRevision {
			t.Fatal("known failure automatically retried provider or changed its receipt")
		}
		assertNoRuns()
		closeOwners()
		baseURL, routine, closeOwners = open()
		recovered := authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/compile", nil, http.StatusOK)
		if recovered.State != "failed" || recovered.CompilationAccepted || recovered.WorkspaceRevision != failed.WorkspaceRevision || len(recovered.Requests) != len(failed.Requests) || physicalCalls.Load() != before {
			t.Fatalf("重启后失败请求身份被替换或重新调用 provider: %+v", recovered)
		}
		request := recovered.Requests[requestIndex]
		if request.PermitID != failed.Requests[requestIndex].PermitID || request.Diagnostic == nil || !reflect.DeepEqual(request.Diagnostic, failed.Requests[requestIndex].Diagnostic) {
			t.Fatal("重启重放丢失原 permit 或原始诊断元信息")
		}
		assertNoRuns()
		if scenario == "review_missing_required" {
			if failed.Proposal == nil || failed.State != "failed" || len(failed.Proposal.Reviews) != 5 {
				t.Fatalf("漏 required 的 PASS critic 未作为失败阻塞: %+v", failed)
			}
			if !strings.Contains(envelope.Diagnostic.ParseError, workspace.GoalReviewVerifierFeasibility) || missingRequirementID == "" || !strings.Contains(envelope.Diagnostic.ParseError, missingRequirementID) {
				t.Fatalf("漏审诊断未定位维度和 required ID: %s", envelope.Diagnostic.ParseError)
			}
			for _, review := range failed.Proposal.Reviews {
				if review.Status != workspace.GoalReviewPass {
					t.Fatalf("失败结果改写已完成 critic 原 status: %+v", review)
				}
			}
			if failed.Proposal.SpecificationHash != "" {
				t.Fatal("漏审候选获得可确认的 specification hash")
			}
			// 提供非空的自报身份，确保请求进入 owner 冲突门禁而不是仅因空字段被拒绝。
			confirmation := management.DefinitionAuthoringConfirm{InputHash: failed.InputHash, DerivationPermitID: failed.Proposal.DerivationPermitID, ProposalHash: failed.Proposal.ProposalHash, SpecificationHash: failed.Proposal.ProposalHash, ExpectedWorkspaceRevision: failed.WorkspaceRevision}
			authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/confirm", confirmation, http.StatusConflict)
			stored, err := repositories.Workspaces.Get(ctx, common.WorkspaceID(created.WorkspaceID))
			if err != nil || stored.StartupInput != nil || stored.ActivationAdmission != nil || stored.DefinitionAuthoringConfirmation != nil || len(stored.DefinitionAuthoringHistory) != 0 {
				t.Fatalf("漏 required 的 PASS critic 产生正式 Goal/DoD 或确认身份: %+v %v", stored, err)
			}
			goals, goalErr := repositories.Goals.ListByWorkspaceID(ctx, common.WorkspaceID(created.WorkspaceID))
			dods, dodErr := repositories.DoDs.ListByWorkspaceID(ctx, common.WorkspaceID(created.WorkspaceID))
			if goalErr != nil || dodErr != nil || len(goals) != 0 || len(dods) != 0 {
				t.Fatalf("漏审确认留下正式定义: goals=%d dods=%d errors=%v/%v", len(goals), len(dods), goalErr, dodErr)
			}
			closeOwners()
			baseURL, routine, closeOwners = open()
			recovered := authoringHTTPRequest(t, baseURL, http.MethodGet, path, nil, http.StatusOK)
			if recovered.State != "failed" || recovered.WorkspaceRevision != failed.WorkspaceRevision || recovered.Proposal == nil || len(recovered.Proposal.Reviews) != 5 || physicalCalls.Load() != before {
				t.Fatalf("file 重开后无效 PASS 被重审、改判或移除: %+v calls=%d", recovered, physicalCalls.Load())
			}
			assertNoRuns()
			return
		}
		return
	}
	ready := authoringHTTPRequest(t, baseURL, http.MethodGet, path, nil, http.StatusOK)
	if realWorker {
		deadline := time.Now().Add(30 * time.Second)
		for ready.State != "ready_to_confirm" && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
			ready = authoringHTTPRequest(t, baseURL, http.MethodGet, path, nil, http.StatusOK)
		}
	}
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
	assertSuccessAudit := func() {
		t.Helper()
		stored, err := repositories.Workspaces.Get(ctx, common.WorkspaceID(created.WorkspaceID))
		if err != nil || stored.DefinitionAuthoring == nil {
			t.Fatalf("读取成功审计前像失败：%v", err)
		}
		request := stored.DefinitionAuthoring.SpecificationRequests[0]
		if request.SuccessAudit == nil || request.Proposal == nil {
			t.Fatal("已完成提案缺少 owner 原文审计")
		}
		before := stored
		readAudit := func(workspaceID, permit string, status int) management.DefinitionAuthoringSuccessAuditView {
			t.Helper()
			response, err := http.Get(baseURL + "/api/v1/workorch/workspaces/" + workspaceID + "/authoring/success-byte-audits/" + permit)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != status || response.Header.Get("Cache-Control") != "no-store" {
				t.Fatalf("受限审计响应：%d %v", response.StatusCode, response.Header)
			}
			if status != http.StatusOK {
				return management.DefinitionAuthoringSuccessAuditView{}
			}
			var envelope struct {
				Audit management.DefinitionAuthoringSuccessAuditView `json:"audit"`
			}
			if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
				t.Fatal(err)
			}
			return envelope.Audit
		}
		for range 2 {
			audit := readAudit(created.WorkspaceID, request.PermitID, http.StatusOK)
			if audit.WorkspaceID != created.WorkspaceID || audit.PermitID != request.PermitID || audit.CanonicalHash != request.Proposal.ContentHash() {
				t.Fatal("受限审计身份变化")
			}
			for _, pair := range []struct {
				view management.DefinitionProviderObservationView
				raw  workspace.DefinitionProviderObservation
			}{{audit.Document, request.SuccessAudit.Document}, {audit.Wire, request.SuccessAudit.Wire}} {
				content, err := base64.StdEncoding.DecodeString(pair.view.ContentBase64)
				if err != nil || !bytes.Equal(content, pair.raw.Content) || !reflect.DeepEqual(pair.view.DefinitionProviderObservationMetadata, *authoringProviderObservationMetadata(&pair.raw)) {
					t.Fatal("受限审计未保全原 bytes 与 metadata")
				}
			}
		}
		readAudit(created.WorkspaceID, "unknown-permit", http.StatusNotFound)
		readAudit("other-workspace", request.PermitID, http.StatusNotFound)
		after, err := repositories.Workspaces.Get(ctx, common.WorkspaceID(created.WorkspaceID))
		if err != nil || !reflect.DeepEqual(before, after) || physicalCalls.Load() != 8 {
			t.Fatal("受限审计读取发生写入或重复 provider 调用")
		}
		assertNoRuns()
	}
	assertSuccessAudit()
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
	assertSuccessAudit()
	observed := authoringHTTPRequest(t, baseURL, http.MethodGet, path, nil, http.StatusOK)
	if observed.ConfirmationRecovery == nil || observed.ConfirmationRecovery.InputHash != confirmation.InputHash || observed.ConfirmationRecovery.DerivationPermitID != confirmation.DerivationPermitID || observed.ConfirmationRecovery.ProposalHash != confirmation.ProposalHash || observed.ConfirmationRecovery.SpecificationHash != confirmation.SpecificationHash || observed.ConfirmationRecovery.ExpectedWorkspaceRevision != observed.WorkspaceRevision {
		t.Fatalf("owner confirmation recovery identity lost: %+v", observed)
	}
	for _, changed := range []management.DefinitionAuthoringConfirm{
		{InputHash: "changed", DerivationPermitID: confirmation.DerivationPermitID, ProposalHash: confirmation.ProposalHash, SpecificationHash: confirmation.SpecificationHash, ExpectedWorkspaceRevision: observed.WorkspaceRevision},
		{InputHash: confirmation.InputHash, DerivationPermitID: "changed", ProposalHash: confirmation.ProposalHash, SpecificationHash: confirmation.SpecificationHash, ExpectedWorkspaceRevision: observed.WorkspaceRevision},
		{InputHash: confirmation.InputHash, DerivationPermitID: confirmation.DerivationPermitID, ProposalHash: "changed", SpecificationHash: confirmation.SpecificationHash, ExpectedWorkspaceRevision: observed.WorkspaceRevision},
		{InputHash: confirmation.InputHash, DerivationPermitID: confirmation.DerivationPermitID, ProposalHash: confirmation.ProposalHash, SpecificationHash: "changed", ExpectedWorkspaceRevision: observed.WorkspaceRevision},
	} {
		authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/confirm", changed, http.StatusConflict)
	}
	recovered := authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/confirm", observed.ConfirmationRecovery, http.StatusOK)
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
	if response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("%s %s permits persistent browser cache: %v", method, path, response.Header)
	}
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

func authoringHTTPMapping(t *testing.T, record *workspace.DefinitionAuthoringRecord) workspace.DefinitionAuthoringMapping {
	t.Helper()
	mapping, err := testkit.Mapping(record.DerivationRequests[0], workspace.DefinitionAuthoringVerificationMapping{Declaration: "按模板验证", Status: workspace.GoalVerificationDeclared, ProfileID: "generic", EvidenceRequirementID: "*", Scope: "dodItem"})
	if err != nil {
		t.Fatal(err)
	}
	return mapping
}

func authoringHTTPProposal(t *testing.T, record *workspace.DefinitionAuthoringRecord) workspace.GoalSpecification {
	t.Helper()
	proposal, err := workspace.CompileDefinitionAuthoringMapping(record.DerivationRequests[0], authoringHTTPMapping(t, record))
	if err != nil {
		t.Fatal(err)
	}
	return proposal
}

func authoringHTTPRequiredReviewRequirementIDs(t *testing.T, proposal workspace.GoalSpecification, kind string) []string {
	t.Helper()
	required, err := proposal.RequiredReviewRequirementIDs(kind)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(required))
	for _, id := range required {
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		t.Fatalf("review kind %q has no required requirement IDs", kind)
	}
	return ids
}

func authoringHTTPGoalRequirementID(t *testing.T, proposal workspace.GoalSpecification) string {
	t.Helper()
	for _, requirement := range proposal.Requirements {
		if len(requirement.SourceItemRefs) == 1 && requirement.SourceItemRefs[0] == workspace.DefinitionAuthoringGoalItemRef {
			return requirement.ID
		}
	}
	t.Fatal("当前 compiler 未保留独立 Goal requirement")
	return ""
}
