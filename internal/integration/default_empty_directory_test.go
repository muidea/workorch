package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	application "github.com/muidea/workorch/internal/modules/application/workorchd"
	approvalmodule "github.com/muidea/workorch/internal/modules/blocks/approval"
	approvalbiz "github.com/muidea/workorch/internal/modules/blocks/approval/biz"
	decisionmodule "github.com/muidea/workorch/internal/modules/blocks/decision"
	decisionbiz "github.com/muidea/workorch/internal/modules/blocks/decision/biz"
	eventstreammodule "github.com/muidea/workorch/internal/modules/blocks/eventstream"
	eventstreambiz "github.com/muidea/workorch/internal/modules/blocks/eventstream/biz"
	filemodule "github.com/muidea/workorch/internal/modules/blocks/file"
	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	gitmodule "github.com/muidea/workorch/internal/modules/blocks/git"
	gitbiz "github.com/muidea/workorch/internal/modules/blocks/git/biz"
	llmmodule "github.com/muidea/workorch/internal/modules/blocks/llm"
	llmbiz "github.com/muidea/workorch/internal/modules/blocks/llm/biz"
	mcpblockmodule "github.com/muidea/workorch/internal/modules/blocks/mcp"
	mcpblockbiz "github.com/muidea/workorch/internal/modules/blocks/mcp/biz"
	mcpmodule "github.com/muidea/workorch/internal/modules/blocks/mcpregistry"
	mcpbiz "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/biz"
	memorymodule "github.com/muidea/workorch/internal/modules/blocks/memory"
	memorybiz "github.com/muidea/workorch/internal/modules/blocks/memory/biz"
	resultmodule "github.com/muidea/workorch/internal/modules/blocks/result"
	resultbiz "github.com/muidea/workorch/internal/modules/blocks/result/biz"
	runmodule "github.com/muidea/workorch/internal/modules/blocks/run"
	runbiz "github.com/muidea/workorch/internal/modules/blocks/run/biz"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	shellmodule "github.com/muidea/workorch/internal/modules/blocks/shell"
	shellbiz "github.com/muidea/workorch/internal/modules/blocks/shell/biz"
	skillmodule "github.com/muidea/workorch/internal/modules/blocks/skill"
	skillbiz "github.com/muidea/workorch/internal/modules/blocks/skill/biz"
	soulmodule "github.com/muidea/workorch/internal/modules/blocks/soul"
	soulbiz "github.com/muidea/workorch/internal/modules/blocks/soul/biz"
	sshmodule "github.com/muidea/workorch/internal/modules/blocks/ssh"
	sshbiz "github.com/muidea/workorch/internal/modules/blocks/ssh/biz"
	webmodule "github.com/muidea/workorch/internal/modules/blocks/web"
	webbiz "github.com/muidea/workorch/internal/modules/blocks/web/biz"
	workspacemodule "github.com/muidea/workorch/internal/modules/blocks/workspace"
	workspacebiz "github.com/muidea/workorch/internal/modules/blocks/workspace/biz"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	governancemodule "github.com/muidea/workorch/internal/modules/kernel/governance"
	governancebiz "github.com/muidea/workorch/internal/modules/kernel/governance/biz"
	orchestratormodule "github.com/muidea/workorch/internal/modules/kernel/orchestrator"
	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	systemmodule "github.com/muidea/workorch/internal/modules/kernel/system"
	systembiz "github.com/muidea/workorch/internal/modules/kernel/system/biz"
	"github.com/muidea/workorch/internal/pkg/common"
	fileRepo "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	management "github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 此测试仅证明 production bootstrap 装配与真实 prerequisite facts；不是 Runtime 首失败或工程交付正例。
// 所有配置和业务存储均由测试合成，仅 provider 是 localhost scripted HTTP。
// 不写交付源码，不手工创建 bootstrap/Run，不移除 availability gate。
func TestDefaultSoftwareDevEmptyDirectoryProductionBootstrapOnly(t *testing.T) {
	ctx := context.Background()
	storage, directory, configDir := t.TempDir(), t.TempDir(), t.TempDir()
	repos := workorchrepo.NewWorkspaceScopedFileRepositorySet(storage)
	provider := &emptyDirectoryProvider{repos: repos}
	proxy := httptest.NewServer(http.HandlerFunc(provider.serveHTTP))
	t.Cleanup(proxy.Close)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	config := fmt.Sprintf(`endpointName = "workorch-integration"
[server]
addr = %q
shutdownTimeout = "5s"
[repository]
backend = "file"
dataDir = %q
[llm.aiProxy]
baseURL = %q
timeout = "10s"
catalogRequestTimeout = "2s"
catalogTTL = "5m"
catalogMaxStaleAge = "24h"
maxCatalogResponseBytes = 1048576
maxCatalogModels = 10
[llm.defaultChat]
providerID = "ai-proxy"
modelID = "fixture-model"
[llm.budget]
contextUtilizationRatio = 0.75
estimatedTokensPerByte = 0.25
[llm.aiProxy.modelOverrides.fixture-model]
contextWindowTokens = 128000
maxOutputTokens = 16384
operations = ["responses"]
`, address, storage, proxy.URL)
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	// 先关闭未使用的 manager，再指定全新 synthetic 目录；不读取旧配置。
	_ = workorchconfig.Close()
	workorchconfig.ConfigureDirectory(configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })
	cfg, err := workorchconfig.Current()
	if err != nil {
		t.Fatal(err)
	}

	hub := magicevent.NewHub(1024)
	background := task.NewBackgroundRoutine(1024)
	startProductionOwners(t, ctx, hub, background, repos, storage, cfg)
	app := application.New()
	if err := app.Setup(ctx, hub, background); err != nil {
		t.Fatal(err)
	}
	if err := app.Run(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		app.Teardown(shutdown)
	})
	baseURL := "http://" + address
	client := &http.Client{Timeout: 10 * time.Second}
	waitForHTTP(t, client, baseURL)

	template, err := repos.WorkspaceTemplates.Get(ctx, "software-dev")
	if err != nil {
		t.Fatalf("System 未物化正式默认模板: %v", err)
	}
	if len(template.DoDItems) == 0 {
		t.Fatal("正式默认模板没有 DoD")
	}
	assertEmptyDeliverable(t, directory)
	created := struct {
		Authoring management.DefinitionAuthoringView `json:"authoring"`
	}{}
	productionJSON(t, client, http.MethodPost, baseURL+"/api/v1/workorch/workspace-authoring", management.DefinitionAuthoringCreate{
		RequestID: "empty-directory-production", Title: "空目录工程组合", TemplateID: template.ID, DeliverableDirectory: directory,
		Input: management.DefinitionAuthoringInput{GoalText: "创建一个小型通用多文件 Go 工程，先实现行为，再通过至少两轮独立实现修订完成验收，交付完整构建与测试证据。", AcceptanceRequirements: []string{"至少两轮合法 durable 实现变更，完整 build/test/race 与 Git 交付"}},
	}, &created)
	if created.Authoring.WorkspaceID == "" {
		t.Fatal("production 创建未返回 Workspace 身份")
	}
	workspaceID := common.WorkspaceID(created.Authoring.WorkspaceID)
	assertNoRun(t, repos)
	assertEmptyDeliverable(t, directory)
	path := baseURL + "/api/v1/workorch/workspaces/" + created.Authoring.WorkspaceID
	productionJSON(t, client, http.MethodPost, path+"/authoring/compile", nil, nil)
	var reviewed management.DefinitionAuthoringView
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		var envelope struct {
			Authoring management.DefinitionAuthoringView `json:"authoring"`
		}
		productionJSON(t, client, http.MethodGet, path+"/authoring", nil, &envelope)
		reviewed = envelope.Authoring
		if reviewed.State == "ready_to_confirm" {
			break
		}
		if reviewed.State == "failed" || reviewed.State == "preparation_failed" {
			t.Fatalf("创建编译失败: %+v provider=%v", reviewed, provider.snapshot())
		}
		time.Sleep(150 * time.Millisecond)
	}
	if reviewed.Proposal == nil || len(reviewed.Proposal.Reviews) != 6 {
		t.Fatalf("完整六审查不可达: %+v provider=%v", reviewed, provider.snapshot())
	}
	for _, review := range reviewed.Proposal.Reviews {
		if review.Status != workspace.GoalReviewPass {
			t.Fatalf("审查未通过: %+v", review)
		}
	}
	assertNoRun(t, repos)
	confirmation := management.DefinitionAuthoringConfirm{InputHash: reviewed.InputHash, DerivationPermitID: reviewed.Proposal.DerivationPermitID, ProposalHash: reviewed.Proposal.ProposalHash, SpecificationHash: reviewed.Proposal.SpecificationHash, ExpectedWorkspaceRevision: reviewed.WorkspaceRevision}
	productionJSON(t, client, http.MethodPost, path+"/authoring/confirm", confirmation, nil)
	current, err := repos.Workspaces.Get(ctx, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if current.StartupInput == nil || current.ActivationAdmission == nil || current.DefinitionAuthoringConfirmation == nil || current.DefinitionAuthoringConfirmation.InputHash != confirmation.InputHash || current.DefinitionAuthoringConfirmation.SpecificationHash != confirmation.SpecificationHash {
		t.Fatalf("正式确认未封存 exact 身份/准入: workspace=%s startupPresent=%t activationPresent=%t confirmationPresent=%t", current.ID, current.StartupInput != nil, current.ActivationAdmission != nil, current.DefinitionAuthoringConfirmation != nil)
	}
	assertNoRun(t, repos)
	assertEmptyDeliverable(t, directory)
	productionJSON(t, client, http.MethodPost, path+"/execution-activations", management.WorkspaceExecutionCommandRequest{Intent: "restart", RequestID: "explicit-first-execution"}, nil)

	var first *runmodel.Run
	deadline = time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		runs, err := repos.Runs.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(runs) > 1 {
			t.Fatalf("首次执行创建重复 Run: %d", len(runs))
		}
		if len(runs) == 1 {
			first = runs[0]
			if first.RuntimeState != nil && productionPrerequisitesObserved(first.RuntimeState) {
				break
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	if first == nil || first.RuntimeBootstrap == nil || first.RuntimeState == nil {
		t.Fatalf("真实 Runtime 未启动: runPresent=%t provider=%v", first != nil, provider.snapshot())
	}
	if first.StartupInputHash != current.StartupInput.StartupInputHash || first.RuntimeBootstrap.Definition.StartupInputHash != first.StartupInputHash {
		t.Fatal("production bootstrap 身份偏离确认 startup")
	}
	state := first.RuntimeState
	t.Logf("bootstrap_only 观察: Run=%s state=%s revision=%d RuntimeRevision=%d semanticPhase=%s receipts=%d provider=%v", first.ID, first.State, first.Revision, state.StateRevision, state.SemanticPlanning.Phase, len(state.OperationReceipts), provider.snapshot())
	if !productionPrerequisitesObserved(state) {
		t.Fatalf("bootstrap_only 所需真实 owner prerequisite 尚未完成: semanticPhase=%s decisionState=%s catalogs=%d capabilityStates=%d", state.SemanticPlanning.Phase, state.Decision.State, len(state.Facts.Resources.OperationAdmissibilities), len(state.Facts.Resources.CapabilityStates))
	}
	if err := state.Validate(); err != nil {
		t.Fatalf("持久 Runtime 聚合不满足合同: %v", err)
	}
	calls := provider.snapshot()
	if len(calls) < 8 || calls[0] != "derive" || calls[1] != "proposal" {
		t.Fatalf("真实 authoring HTTP 调用序列不完整: %v", calls)
	}
	for _, kind := range []string{"derive", "proposal", "review/" + workspace.GoalReviewDimensionCoverage, "review/" + workspace.GoalReviewNegativePath, "review/" + workspace.GoalReviewRegression, "review/" + workspace.GoalReviewResourceEnvironment, "review/" + workspace.GoalReviewTraceability, "review/" + workspace.GoalReviewVerifierFeasibility} {
		count := 0
		for _, call := range calls {
			if call == kind {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("authoring HTTP 调用不是 exact-once: kind=%s count=%d", kind, count)
		}
	}
	assertEmptyDeliverable(t, directory)
	// pre-semantic target_grounding_unresolved 是暂态投影，不能当 Runtime 终结/owner-effect 失败。
	t.Log("证据范围=bootstrap_only；首个真实 capability effect/稳定 Runtime 失败=not-run，完整多轮工程正例=blocked/未完成。")
}

func productionPrerequisitesObserved(state *runtimecontract.RuntimeStateRecord) bool {
	if state == nil || state.Facts.Resources.Manifest == nil || state.Facts.Resources.TargetSelection == nil || state.Facts.Resources.ProviderCeiling == nil {
		return false
	}
	if len(state.Facts.Resources.Manifest.TargetRefs) != 0 {
		return false
	}
	file, git, shell := false, false, false
	for _, catalog := range state.Facts.Resources.OperationAdmissibilities {
		switch catalog.OwnerModuleID {
		case runtimecontract.RuntimeModuleFile:
			file = true
		case runtimecontract.RuntimeModuleGit:
			git = true
		case runtimecontract.RuntimeModuleShell:
			shell = true
		}
	}
	gitState := false
	for _, fact := range state.Facts.Resources.CapabilityStates {
		if fact.OwnerModuleID == runtimecontract.RuntimeModuleGit && fact.CapabilityStateKind == "repository_state" && fact.QueryReceiptRef != "" {
			gitState = true
		}
	}
	return file && git && shell && gitState
}

type ownerLifecycle interface {
	Start() error
	Stop(context.Context) error
}

func startProductionOwners(t *testing.T, ctx context.Context, hub magicevent.Hub, background task.BackgroundRoutine, repos workorchrepo.WorkspaceScopedFileRepositorySet, storage string, cfg workorchconfig.Config) {
	t.Helper()
	var started []ownerLifecycle
	t.Cleanup(func() {
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for n := len(started) - 1; n >= 0; n-- {
			if err := started[n].Stop(shutdown); err != nil {
				t.Errorf("停止 owner: %v", err)
			}
		}
		hub.Terminate(shutdown)
		if !background.Shutdown(shutdown) {
			t.Error("background 未完成有界排空")
		}
	})
	start := func(owner ownerLifecycle, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		if err := owner.Start(); err != nil {
			t.Fatal(err)
		}
		started = append(started, owner)
	}
	file := filemodule.New()
	start(file, file.Configure(filebiz.Dependencies{EventHub: hub}))
	git := gitmodule.New()
	receipts, err := gitbiz.NewCapabilityStateReceiptStore(filepath.Join(storage, "runtime", "git-receipts"))
	if err != nil {
		t.Fatal(err)
	}
	start(git, git.Configure(gitbiz.Dependencies{EventHub: hub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(), CapabilityStateReceipts: receipts}))
	shell := shellmodule.New()
	start(shell, shell.Configure(shellbiz.Dependencies{EventHub: hub}))
	ssh := sshmodule.New()
	start(ssh, ssh.Configure(sshbiz.Dependencies{EventHub: hub}))
	web := webmodule.New()
	start(web, web.Configure(webbiz.Dependencies{EventHub: hub}))
	llm := llmmodule.New()
	start(llm, llm.Configure(llmbiz.Dependencies{EventHub: hub, LoadLLMConfig: func() (workorchconfig.LLMConfig, error) { return cfg.LLM, nil }}))
	mcpBlock := mcpblockmodule.New()
	start(mcpBlock, mcpBlock.Configure(mcpblockbiz.Dependencies{EventHub: hub}))
	soul := soulmodule.New()
	start(soul, soul.Configure(soulbiz.Dependencies{EventHub: hub, Souls: fileRepo.NewSoulRepository(filepath.Join(storage, "system", "souls"))}))
	memory := memorymodule.New()
	start(memory, memory.Configure(memorybiz.Dependencies{EventHub: hub, OwnerRepositories: repos.MemoryOwner}))
	skill := skillmodule.New()
	start(skill, skill.Configure(skillbiz.Dependencies{EventHub: hub, Skills: fileRepo.NewSkillRepository(filepath.Join(storage, "system", "skills.json"))}))
	mcp := mcpmodule.New()
	start(mcp, mcp.Configure(mcpbiz.Dependencies{EventHub: hub, Registry: fileRepo.NewMCPRegistryRepository(filepath.Join(storage, "system", "mcp.json"))}))
	events := eventstreammodule.New()
	start(events, events.Configure(eventstreambiz.Dependencies{EventHub: hub, OwnerRepositories: repos.EventStreamOwner}))
	workspaceOwner := workspacemodule.New()
	start(workspaceOwner, workspaceOwner.Configure(workspacebiz.Dependencies{EventHub: hub, OwnerRepositories: repos.WorkspaceOwner}))
	decision := decisionmodule.New()
	start(decision, decision.Configure(decisionbiz.Dependencies{EventHub: hub, OwnerRepositories: repos.DecisionOwner}))
	approval := approvalmodule.New()
	start(approval, approval.Configure(approvalbiz.Dependencies{EventHub: hub, OwnerRepositories: repos.ApprovalOwner}))
	result := resultmodule.New()
	start(result, result.Configure(resultbiz.Dependencies{EventHub: hub, OwnerRepositories: repos.ResultOwner}))
	run := runmodule.New()
	start(run, run.Configure(runbiz.Dependencies{EventHub: hub, OwnerRepositories: repos.RunOwner}))
	orchestrator := orchestratormodule.New()
	start(orchestrator, orchestrator.Configure(orchestratorbiz.Dependencies{CommandHub: hub, BackgroundRoutine: background, Evidence: repos.OrchestratorEvidence}))
	governance := governancemodule.New()
	start(governance, governance.Configure(governancebiz.EventDependencies{EventHub: hub, BackgroundRoutine: background, AutoGovernanceTraces: fileRepo.NewAutoGovernanceTraceRepository(filepath.Join(storage, "system", "governance.json")), LoadConfig: func() (workorchconfig.Config, error) { return cfg, nil }}))
	system := systemmodule.New()
	if err := system.Configure(systembiz.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := system.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = system.Stop(ctx) })
}

type emptyDirectoryProvider struct {
	mu    sync.Mutex
	calls []string
	repos workorchrepo.WorkspaceScopedFileRepositorySet
}

func (p *emptyDirectoryProvider) snapshot() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.calls...)
}
func (p *emptyDirectoryProvider) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/v1/models" {
		_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": []any{map[string]any{"id": "fixture-model", "supported_endpoints": []string{"/v1/responses"}, "contextWindowTokens": 128000, "maxOutputTokens": 16384}}})
		return
	}
	if r.URL.Path != "/v1/responses" {
		http.NotFound(w, r)
		return
	}
	wire, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	var request struct {
		Input string `json:"input"`
	}
	if err := json.Unmarshal(wire, &request); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	all, err := p.repos.Workspaces.List(r.Context())
	if err != nil || len(all) != 1 || all[0].DefinitionAuthoring == nil {
		http.Error(w, "missing persisted authoring input", 500)
		return
	}
	record := all[0].DefinitionAuthoring
	var payload []byte
	kind := ""
	if len(record.SpecificationRequests) == 0 && len(record.DerivationRequests) == 1 && record.DerivationRequests[0].Proposal == nil {
		kind = "derive"
		payload, err = json.Marshal(workspace.DefinitionDerivedAcceptance{AuthoringInputHash: record.InputHash, TemplateSnapshotHash: record.TemplateSnapshotHash, AdditionalItems: []string{"完整工程运行时验收"}})
	} else {
		for _, pending := range record.SpecificationRequests {
			if pending.Proposal != nil || pending.Review != nil {
				continue
			}
			if pending.ReviewKind == "" {
				kind = "proposal"
				proposal, buildErr := productionProposal(record)
				if buildErr != nil {
					err = buildErr
					break
				}
				payload, err = json.Marshal(struct {
					Specification workspace.GoalSpecification `json:"specification"`
				}{proposal})
			} else {
				kind = "review/" + pending.ReviewKind
				proposal := record.SpecificationRequests[0].Proposal
				if proposal == nil {
					err = fmt.Errorf("missing persisted proposal")
					break
				}
				payload, err = json.Marshal(struct {
					Review workspace.GoalSpecificationCompletenessReview `json:"review"`
				}{workspace.GoalSpecificationCompletenessReview{Kind: pending.ReviewKind, Status: workspace.GoalReviewPass, SpecificationHash: proposal.ContentHash(), Reason: "独立 scripted 审查", EvidenceRefs: []string{workspace.DefinitionAuthoringGoalItemRef}, RequirementIDs: []string{"goal"}}})
			}
			break
		}
	}
	if kind == "" {
		payload, kind, err = emptyDirectoryRuntimeDocument(request.Input)

	}
	p.mu.Lock()
	p.calls = append(p.calls, kind)
	p.mu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	event, _ := json.Marshal(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed", "output_text": string(payload), "usage": map[string]any{"input_tokens": 128, "output_tokens": 128, "total_tokens": 256}}})
	_, _ = fmt.Fprintf(w, "data: %s\n\n", event)
}

// 只从真实 provider prompt 取声明和当前 batch；不读取 repository 重建 Runtime 权威。
func emptyDirectoryRuntimeDocument(input string) ([]byte, string, error) {
	for _, line := range strings.Split(input, "\n") {
		var authority struct {
			PlanStep         runtimecontract.RuntimeSemanticPlanStep `json:"planStep"`
			BatchID          string                                  `json:"batchId"`
			BatchSequence    int                                     `json:"batchSequence"`
			BatchCoverageIDs []common.CoverageWorkItemID             `json:"batchCoverageWorkItemIds"`
			Manifest         struct {
				Hash               string                                `json:"hash"`
				DefinitionSeedHash string                                `json:"definitionSeedHash"`
				CoverageItems      []runtimecontract.RuntimeCoverageItem `json:"coverageItems"`
				ProofObligations   []struct {
					CoverageID     common.CoverageWorkItemID `json:"coverageWorkItemId"`
					DeclarationRef string                    `json:"declarationRef"`
					EvidenceKinds  []string                  `json:"currentRequiredEvidenceKinds"`
					Constraints    []string                  `json:"declarationProspectiveTargetAllOfConstraintRefs"`
					Acquisition    *struct {
						Owner       string   `json:"acquisitionOwnerModuleId"`
						Kind        string   `json:"acquisitionOperationKind"`
						Variant     string   `json:"acquisitionOperationVariantId"`
						Policy      string   `json:"acquisitionTargetPolicy"`
						Selector    string   `json:"acquisitionTargetSelectorRef"`
						Constraints []string `json:"acquisitionConstraintRefs"`
					} `json:"ownerFactAcquisition"`
				} `json:"proofObligations"`
				OwnerCatalogs []struct {
					Owner       string   `json:"ownerModuleId"`
					Kinds       []string `json:"operationKinds"`
					Selectors   []string `json:"selectorRefs"`
					Constraints []string `json:"constraintRefs"`
				} `json:"ownerCatalogs"`
			} `json:"manifest"`
		}
		if json.Unmarshal([]byte(line), &authority) != nil || authority.Manifest.Hash == "" || authority.PlanStep == "" {
			continue
		}
		wire := map[string]any{"documentKind": runtimecontract.RuntimeProviderDocumentSemanticPlan, "schemaVersion": runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent, "manifestHash": authority.Manifest.Hash, "definitionSeedHash": authority.Manifest.DefinitionSeedHash, "planStep": authority.PlanStep, "proposals": []any{}}
		if authority.PlanStep == runtimecontract.RuntimeSemanticPlanStepCoverageOrder {
			order := make([]common.CoverageWorkItemID, 0, len(authority.Manifest.CoverageItems))
			// 交付变更先于其验证及交付证明；不减少 CWI/DoD 集合。
			for _, item := range authority.Manifest.CoverageItems {
				if item.SourceScope == "deliverable" && item.SourceID == "source_change" {
					order = append(order, item.ID)
				}
			}
			for _, item := range authority.Manifest.CoverageItems {
				if item.SourceScope != "deliverable" || item.SourceID != "source_change" {
					order = append(order, item.ID)
				}
			}
			wire["coverageOrder"] = order
		} else if authority.PlanStep == runtimecontract.RuntimeSemanticPlanStepProofBatch {
			wire["batchId"], wire["batchSequence"], wire["batchCoverageWorkItemIds"] = authority.BatchID, authority.BatchSequence, authority.BatchCoverageIDs
			proposals := []any{}
			for _, obligation := range authority.Manifest.ProofObligations {
				proposal := map[string]any{"coverageWorkItemId": obligation.CoverageID, "declarationRef": obligation.DeclarationRef, "requiredEvidenceKinds": obligation.EvidenceKinds}
				if acquisition := obligation.Acquisition; acquisition != nil {
					proposal["ownerModuleId"], proposal["operationKind"] = acquisition.Owner, acquisition.Kind
					if acquisition.Variant != "" {
						proposal["operationVariantId"] = acquisition.Variant
					}
					target := map[string]any{"policy": acquisition.Policy, "ownerSelectorRefs": []string{acquisition.Selector}}
					if acquisition.Policy == string(runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget) {
						target["prospectiveConstraintRefs"], target["prospectiveTargetRefs"] = acquisition.Constraints, []string{"workorch_validation/v3/project_test.go"}
					}
					proposal["target"] = target
				} else {
					proposal["ownerModuleId"], proposal["operationKind"] = runtimecontract.RuntimeModuleFile, runtimecontract.StepKindFileWrite
					selector := ""
					for _, catalog := range authority.Manifest.OwnerCatalogs {
						if catalog.Owner == runtimecontract.RuntimeModuleFile {
							for _, candidate := range catalog.Selectors {
								if candidate == "workspace-relative-new-file-target" {
									selector = candidate
								}
							}
						}
					}
					if selector == "" {
						return nil, "runtime/proof_batch", fmt.Errorf("真实 File catalog 无 prospective selector")
					}
					proposal["target"] = map[string]any{"policy": runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget, "ownerSelectorRefs": []string{selector}, "prospectiveConstraintRefs": obligation.Constraints, "prospectiveTargetRefs": []string{"go.mod", "main.go", "logic.go"}}
				}
				proposals = append(proposals, proposal)
			}
			wire["proposals"] = proposals
		} else {
			return nil, "runtime", fmt.Errorf("未知 semantic planStep %s", authority.PlanStep)
		}
		payload, err := json.Marshal(wire)
		return payload, "runtime/" + string(authority.PlanStep), err
	}
	return nil, "runtime", fmt.Errorf("当前 scripted provider 未识别真实 Runtime prompt 文档")
}

func productionProposal(record *workspace.DefinitionAuthoringRecord) (workspace.GoalSpecification, error) {
	derivation := record.DerivationRequests[0]
	items, err := derivation.Proposal.CandidateItems(record.Input, &record.TemplateSnapshot)
	if err != nil {
		return workspace.GoalSpecification{}, err
	}
	proposal := workspace.GoalSpecification{
		Version: 1, GoalItemRef: workspace.DefinitionAuthoringGoalItemRef,
		GoalTextHash: workspace.StableDefinitionTextHash(record.Input.GoalText),
		Dimensions:   []workspace.GoalSpecificationDimension{{ID: "all", Required: true, Activated: true, ResourceIDs: []string{}}},
		Requirements: []workspace.GoalSpecificationRequirement{{
			ID: "goal", DimensionID: "all", Required: true, SourceKind: workspace.GoalRequirementUserExplicit,
			SourceItemRefs: []string{workspace.DefinitionAuthoringGoalItemRef}, ResourceIDs: []string{}, Text: record.Input.GoalText,
		}},
		Reviews: []workspace.GoalSpecificationCompletenessReview{},
	}
	for n, item := range items {
		kind := workspace.GoalRequirementDerived
		if strings.HasPrefix(item.CandidateItemRef, "derived-dod:user:") {
			kind = workspace.GoalRequirementUserExplicit
		} else if strings.HasPrefix(item.CandidateItemRef, "derived-dod:template:") {
			kind = workspace.GoalRequirementTemplateRequired
		}
		id := fmt.Sprintf("item-%03d", n+1)
		proposal.Requirements = append(proposal.Requirements, workspace.GoalSpecificationRequirement{ID: id, DimensionID: "all", Required: true, SourceKind: kind, SourceItemRefs: []string{item.CandidateItemRef}, ResourceIDs: []string{}, Text: item.Text})
		proposal.Obligations = append(proposal.Obligations, workspace.GoalSpecificationObligation{ID: "obl-" + id, RequirementID: id, DoDItemRefs: []string{item.CandidateItemRef}, Text: item.Text})
	}
	proposal.Obligations = append(proposal.Obligations, workspace.GoalSpecificationObligation{ID: "obl-goal", RequirementID: "goal", DoDItemRefs: []string{items[0].CandidateItemRef}, Text: record.Input.GoalText})
	for _, obligation := range proposal.Obligations {
		proposal.Verifications = append(proposal.Verifications, workspace.GoalSpecificationVerification{ID: "ver-" + obligation.ID, ObligationID: obligation.ID, Declaration: "完整保留默认模板验证约束", Status: workspace.GoalVerificationDeclared, ProfileID: "software-dev", EvidenceRequirementID: "*", Scope: "dodItem"})
	}
	proposal.Resources, err = workspace.AuthoringSpecificationResources(record.Input, derivation.Resources)
	return proposal, err
}

func productionJSON(t *testing.T, client *http.Client, method, url string, body, output any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = strings.NewReader(string(data))
	}
	request, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		t.Fatalf("%s %s: status=%d body=%s", method, url, response.StatusCode, data)
	}
	if output != nil {
		if err := json.Unmarshal(data, output); err != nil {
			t.Fatalf("decode %s: %v body=%s", url, err, data)
		}
	}
}
func waitForHTTP(t *testing.T, client *http.Client, base string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		response, err := client.Get(base + "/healthz")
		if err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("Application healthz 未返回 HTTP 200")
}
func assertEmptyDeliverable(t *testing.T, directory string) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 0 {
		t.Fatalf("交付目录不是空目录: %v entries=%v", err, entries)
	}
}
func assertNoRun(t *testing.T, repos workorchrepo.WorkspaceScopedFileRepositorySet) {
	t.Helper()
	runs, err := repos.Runs.List(context.Background())
	if err != nil || len(runs) != 0 {
		t.Fatalf("创建/编译/确认自动启动 Run: %d %v", len(runs), err)
	}
}
