package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	application "github.com/muidea/workorch/internal/modules/application/workorchd"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceports"
	approvalmodule "github.com/muidea/workorch/internal/modules/blocks/approval"
	approvalbiz "github.com/muidea/workorch/internal/modules/blocks/approval/biz"
	decisionmodule "github.com/muidea/workorch/internal/modules/blocks/decision"
	decisionbiz "github.com/muidea/workorch/internal/modules/blocks/decision/biz"
	eventstreammodule "github.com/muidea/workorch/internal/modules/blocks/eventstream"
	eventstreambiz "github.com/muidea/workorch/internal/modules/blocks/eventstream/biz"
	filemodule "github.com/muidea/workorch/internal/modules/blocks/file"
	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	gitmodule "github.com/muidea/workorch/internal/modules/blocks/git"
	gitbiz "github.com/muidea/workorch/internal/modules/blocks/git/biz"
	gitevents "github.com/muidea/workorch/internal/modules/blocks/git/pkg/events"
	llmmodule "github.com/muidea/workorch/internal/modules/blocks/llm"
	llmbiz "github.com/muidea/workorch/internal/modules/blocks/llm/biz"
	mcpblockmodule "github.com/muidea/workorch/internal/modules/blocks/mcp"
	mcpblockbiz "github.com/muidea/workorch/internal/modules/blocks/mcp/biz"
	mcpblockevents "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/events"
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
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	skillmodule "github.com/muidea/workorch/internal/modules/blocks/skill"
	skillbiz "github.com/muidea/workorch/internal/modules/blocks/skill/biz"
	soulmodule "github.com/muidea/workorch/internal/modules/blocks/soul"
	soulbiz "github.com/muidea/workorch/internal/modules/blocks/soul/biz"
	sshmodule "github.com/muidea/workorch/internal/modules/blocks/ssh"
	sshbiz "github.com/muidea/workorch/internal/modules/blocks/ssh/biz"
	sshevents "github.com/muidea/workorch/internal/modules/blocks/ssh/pkg/events"
	webmodule "github.com/muidea/workorch/internal/modules/blocks/web"
	webbiz "github.com/muidea/workorch/internal/modules/blocks/web/biz"
	webevents "github.com/muidea/workorch/internal/modules/blocks/web/pkg/events"
	governancemodule "github.com/muidea/workorch/internal/modules/kernel/governance"
	governancebiz "github.com/muidea/workorch/internal/modules/kernel/governance/biz"
	orchestratormodule "github.com/muidea/workorch/internal/modules/kernel/orchestrator"
	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	systemmodule "github.com/muidea/workorch/internal/modules/kernel/system"
	systembiz "github.com/muidea/workorch/internal/modules/kernel/system/biz"
	workspacemodule "github.com/muidea/workorch/internal/modules/kernel/workspace"
	workspacebiz "github.com/muidea/workorch/internal/modules/kernel/workspace/biz"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/definitionadmission"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/testkit"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/definitionconfiguration"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	fileRepo "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	management "github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 此测试仅证明 production bootstrap 装配与真实 prerequisite facts；不是 Runtime 首失败或工程交付正例。
// 所有配置和业务存储均由测试合成，仅 provider 是 localhost scripted HTTP。
// 不写交付源码，不手工创建 bootstrap/Run，不移除 availability gate。
func TestDefaultSoftwareDevEmptyDirectoryProductionBootstrapOnly(t *testing.T) {
	testEmptyDirectoryProduction(t, false)
}

// 公开创作与 exact 确认合法，显式工作策略仍必须在 owner 准入阻塞。
func TestSameCWIImplementationWorkProductionAvailabilityBlocksBeforeRun(t *testing.T) {
	testEmptyDirectoryProduction(t, true)
}

func TestRequiredProfilePurposePublicAuthoringHTTP(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(fmt.Sprintf("missing_%t", missing), func(t *testing.T) { testEmptyDirectoryProduction(t, false, true, missing) })
	}
}

func testEmptyDirectoryProduction(t *testing.T, requireImplementationWork bool, authoringOnly ...bool) {
	ctx := context.Background()
	storage, directory, configDir := t.TempDir(), t.TempDir(), t.TempDir()
	repos := workorchrepo.NewWorkspaceScopedFileRepositorySet(storage)
	provider := &emptyDirectoryProvider{repos: repos}
	if len(authoringOnly) > 1 {
		provider.omitPurposeSelections = authoringOnly[1]
	}
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
	var executeCalls atomic.Int64
	if requireImplementationWork {
		observer := magicevent.NewSimpleObserver("*", hub)
		var topics []string
		contracts := append(fileevents.Contracts(), gitevents.Contracts()...)
		contracts = append(contracts, shellevents.Contracts()...)
		contracts = append(contracts, mcpblockevents.Contracts()...)
		contracts = append(contracts, webevents.Contracts()...)
		contracts = append(contracts, sshevents.Contracts()...)
		t.Cleanup(func() {
			for _, topic := range topics {
				if err := observer.Unsubscribe(topic); err != nil {
					t.Errorf("execute observer 清理失败：%v", err)
				}
			}
		})
		for _, contract := range contracts {
			if contract.CommandClass != eventcontract.CommandClassExecute {
				continue
			}
			if err := observer.Subscribe(contract.Topic, func(_ magicevent.Event, _ magicevent.Result) { executeCalls.Add(1) }); err != nil {
				t.Fatal(err)
			}
			topics = append(topics, contract.Topic)
		}
		if len(topics) == 0 {
			t.Fatal("capability execute 观察未订阅任何合同")
		}
	}

	template, err := workspaceports.ReadWorkspaceTemplateRecord(ctx, hub, "software-dev")
	if err != nil {
		t.Fatalf("Workspace owner 未提供当前程序默认模板: %v", err)
	}
	if _, err := repos.WorkspaceTemplates.Get(ctx, "software-dev"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("内置模板被持久化: %v", err)
	}
	if !template.Builtin {
		t.Fatal("默认模板没有只读来源标记")
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
	if requireImplementationWork {
		current, err := repos.Workspaces.Get(ctx, workspaceID)
		if err != nil {
			t.Fatal(err)
		}
		// 完整复制模板声明，仅增加显式工作策略，不丢失原 proof 或默认约束。
		hints := template.ContextHints.Map()
		profiles, ok := hints["profileContracts"].(map[string]any)
		if !ok {
			t.Fatal("默认模板缺 profileContracts")
		}
		profile, ok := profiles["software-dev"].(map[string]any)
		if !ok {
			t.Fatal("默认模板缺 software-dev Profile")
		}
		requirements, ok := profile["evidenceRequirements"].([]any)
		if !ok {
			t.Fatal("默认模板缺 evidenceRequirements")
		}
		selected := 0
		for _, value := range requirements {
			requirement, ok := value.(map[string]any)
			if !ok || requirement["id"] != "*" || requirement["scope"] != "dodItem" {
				continue
			}
			requirement["implementationWorkPolicy"] = map[string]any{
				"protocolRef": runtimecontract.RuntimeImplementationWorkProtocolV1, "ownerModuleId": runtimecontract.RuntimeModuleFile,
				"allowedOperations":     []any{map[string]any{"kind": runtimecontract.StepKindFileWrite}},
				"allowedTargetPolicies": []string{string(runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget), string(runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope)},
				"targetSelectorRef":     "file/profile_declared_candidates", "targetSelectorOwnerRef": "profile/software-dev", "targetSelectorSubjectRef": "implementation",
				"targetSelectorCandidateRefs": []string{"resource-a"}, "prospectiveTargetAllOfConstraintRefs": []string{},
				"maxRoundsPerCWI": 2, "maxTargetsPerRound": 1, "requireFreshObservation": true, "requireFreshValidation": true, "protectValidationInputs": true,
			}
			selected++
		}
		if selected != 1 {
			t.Fatal("工作策略必须绑定唯一原 wildcard 声明")
		}
		workspaceHints := current.ContextHints.Map()
		workspaceHints["profileContracts"] = profiles
		current.ContextHints, err = workorcheventpayload.FromMap(workspaceHints)
		if err != nil {
			t.Fatal(err)
		}
		bindings, err := definitionconfiguration.Compile(current, template)
		if err != nil {
			t.Fatal(err)
		}
		if err := runtimecontract.ValidateRuntimeProfileContracts(bindings.ProfileContracts); err != nil {
			t.Fatalf("反例须为合法声明而非结构错误：%v", err)
		}
		if runtimecontract.ExplainRuntimeImplementationWorkAvailability(bindings.ProfileContracts).Available {
			t.Fatal("显式工作策略未进入完整有效 Profile")
		}
		if err := workspaceports.UpdateWorkspaceRecord(ctx, hub, current); err != nil {
			t.Fatalf("typed Workspace 策略更新：%v", err)
		}
	}
	assertNoRun(t, repos)
	assertEmptyDeliverable(t, directory)
	path := baseURL + "/api/v1/workorch/workspaces/" + created.Authoring.WorkspaceID
	productionJSON(t, client, http.MethodPost, path+"/authoring/compile", management.DefinitionAuthoringCompile{ExpectedInputHash: created.Authoring.InputHash, ExpectedTemplateSnapshotHash: created.Authoring.TemplateSnapshotHash}, nil)
	var reviewed management.DefinitionAuthoringView
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		var envelope struct {
			Authoring management.DefinitionAuthoringView `json:"authoring"`
		}
		productionJSON(t, client, http.MethodGet, path+"/authoring", nil, &envelope)
		reviewed = envelope.Authoring
		if reviewed.State == "ready_to_confirm" {
			break
		}
		if reviewed.State == "failed" && provider.omitPurposeSelections {
			current, err := repos.Workspaces.Get(ctx, workspaceID)
			if err != nil {
				t.Fatal(err)
			}
			if len(current.DefinitionAdmissionCandidates) != 0 || current.StartupInput != nil || current.DefinitionAuthoringConfirmation != nil {
				t.Fatal("missing relationship reached admission/confirmation")
			}
			if calls := provider.snapshot(); len(calls) != 2 || calls[1] != "proposal" {
				t.Fatalf("missing relationship reached reviews or retried: %v", calls)
			}
			if reviewed.Proposal == nil || len(reviewed.Proposal.RequiredEvidencePurposes) != 1 || reviewed.Proposal.RequiredEvidencePurposes[0].AcceptanceItemRef != "" {
				t.Fatal("missing source not projected")
			}
			assertNoRun(t, repos)
			assertEmptyDeliverable(t, directory)
			return
		}
		if reviewed.State == "failed" || reviewed.State == "preparation_failed" {
			t.Fatalf("创建编译失败: %+v provider=%v", reviewed, provider.snapshot())
		}
		// 每次 GET 都经 receiving-owner 完整校验；沿用 Web 观察间隔，避免测试读压挤占六审写入。
		time.Sleep(1500 * time.Millisecond)
	}
	if reviewed.State != "ready_to_confirm" || reviewed.Proposal == nil || len(reviewed.Proposal.Reviews) != 6 {
		t.Fatalf("完整可确认六审查不可达: %+v provider=%v", reviewed, provider.snapshot())
	}
	for _, review := range reviewed.Proposal.Reviews {
		if review.Status != workspace.GoalReviewPass {
			t.Fatalf("审查未通过: %+v", review)
		}
	}
	assertNoRun(t, repos)
	confirmation := management.DefinitionAuthoringConfirm{InputHash: reviewed.InputHash, DerivationPermitID: reviewed.Proposal.DerivationPermitID, ProposalHash: reviewed.Proposal.ProposalHash, SpecificationHash: reviewed.Proposal.SpecificationHash, ExpectedWorkspaceRevision: reviewed.WorkspaceRevision}
	if requireImplementationWork {
		// 只提交一次公开确认；独立 ticket 交付可能超时，持久 owner 事实仍须可查。
		wire, err := json.Marshal(confirmation)
		if err != nil {
			t.Fatal(err)
		}
		response, requestErr := client.Post(path+"/authoring/confirm", "application/json", strings.NewReader(string(wire)))
		if response != nil {
			_, readErr := io.ReadAll(response.Body)
			response.Body.Close()
			if readErr != nil {
				t.Fatal(readErr)
			}
		}
		var blocked management.DefinitionAuthoringView
		deadline = time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			var envelope struct {
				Authoring management.DefinitionAuthoringView `json:"authoring"`
			}
			productionJSON(t, client, http.MethodGet, path+"/authoring", nil, &envelope)
			blocked = envelope.Authoring
			if blocked.State == "clarification_required" && blocked.AdmissionViolationCode == workspace.DefinitionAdmissionProfileContractInvalid {
				break
			}
			time.Sleep(150 * time.Millisecond)
		}
		if blocked.State != "clarification_required" || blocked.AdmissionViolationCode != workspace.DefinitionAdmissionProfileContractInvalid {
			t.Fatalf("工作策略未到达持久准入阻塞：%+v requestErr=%v", blocked, requestErr)
		}
		current, err := repos.Workspaces.Get(ctx, workspaceID)
		if err != nil {
			t.Fatal(err)
		}
		if current.DefinitionAuthoringConfirmation == nil || current.DefinitionAuthoringConfirmation.Phase != workspace.DefinitionAuthoringConfirmationMaterialized ||
			current.DefinitionAuthoringConfirmation.InputHash != confirmation.InputHash || current.DefinitionAuthoringConfirmation.SpecificationHash != confirmation.SpecificationHash || len(current.DefinitionAdmissionCandidates) != 1 {
			t.Fatal("负例未保留 exact 物化确认与唯一持久 candidate")
		}
		candidate := current.DefinitionAdmissionCandidates[0]
		admission, err := definitionadmission.Compile(candidate)
		if err != nil || admission.Disposition != workspace.DefinitionAdmissionClarificationRequired || admission.ViolationCode != workspace.DefinitionAdmissionProfileContractInvalid || !strings.Contains(admission.CanonicalViolation, "implementation_work_protocol_unavailable") {
			t.Fatalf("持久 candidate 的阻塞并非工作协议不可用：%+v %v", admission, err)
		}
		if current.StartupInput != nil || current.ActivationAdmission != nil || current.ActiveRunID != "" {
			t.Fatal("阻塞准入生成执行身份")
		}
		if _, present, err := current.ValidateActivationAdmissionPersistence(); err != nil || present {
			t.Fatalf("阻塞前像不合法或被视为准入：present=%t %v", present, err)
		}
		reopened := workorchrepo.NewWorkspaceScopedFileRepositorySet(storage)
		restored, err := reopened.Workspaces.Get(ctx, workspaceID)
		if err != nil || !reflect.DeepEqual(current, restored) {
			t.Fatalf("file 重开改变原准入候选：%v", err)
		}
		replayed, err := definitionadmission.Compile(restored.DefinitionAdmissionCandidates[0])
		if err != nil || !reflect.DeepEqual(admission, replayed) {
			t.Fatalf("file 重开改变阻塞判定：%v", err)
		}
		originalTemplate, err := workspaceports.ReadWorkspaceTemplateRecord(ctx, hub, template.ID)
		if err != nil || !reflect.DeepEqual(template, originalTemplate) {
			t.Fatalf("负例改写默认模板：%v", err)
		}
		assertNoRun(t, reopened)
		assertEmptyDeliverable(t, directory)
		calls := provider.snapshot()
		if len(calls) != 8 || calls[0] != "derive" || calls[1] != "proposal" {
			t.Fatalf("准入负例必须只有八次创作调用：%v", calls)
		}
		for _, kind := range []string{workspace.GoalReviewDimensionCoverage, workspace.GoalReviewNegativePath, workspace.GoalReviewRegression, workspace.GoalReviewResourceEnvironment, workspace.GoalReviewTraceability, workspace.GoalReviewVerifierFeasibility} {
			count := 0
			for _, call := range calls {
				if call == "review/"+kind {
					count++
				}
			}
			if count != 1 {
				t.Fatalf("六审查未 exact-once：%s count=%d", kind, count)
			}
		}
		if executeCalls.Load() != 0 {
			t.Fatal("阻塞准入发出 capability execute")
		}
		t.Log("availability_negative：公开创作与确认后持久阻塞，零 Run/Runtime provider/capability execute；不是两轮工程交付正例。")
		return
	}
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
	if len(authoringOnly) > 0 && authoringOnly[0] {
		if len(reviewed.Proposal.RequiredEvidencePurposes) != 1 || len(current.DefinitionAdmissionCandidates) != 1 || len(current.DefinitionAdmissionCandidates[0].Specification.EvidencePurposeMappings) != 1 {
			t.Fatal("required relation not confirmed")
		}
		if d := current.DefinitionAuthoring.DerivationRequests[0]; d.WireContractRef != workspace.DefinitionAuthoringOwnerBound {
			t.Fatal("new producer did not use v3")
		}
		request := current.DefinitionAuthoring.SpecificationRequests[0]
		if request.Mapping == nil || len(request.Mapping.ProfileAcceptanceSelections) != 1 || len(request.Mapping.EvidencePurposeMappings) != 0 || bytes.Contains(request.SuccessAudit.Document.Content, []byte("HistoricalProfileHash")) {
			t.Fatal("provider supplied machine mapping metadata")
		}
		reopened := workorchrepo.NewWorkspaceScopedFileRepositorySet(storage)
		restored, err := reopened.Workspaces.Get(ctx, workspaceID)
		if err != nil || restored.DefinitionAuthoring == nil || restored.DefinitionAuthoring.Validate() != nil {
			t.Fatal("confirmed source invalid after reopen", err)
		}
		assertNoRun(t, reopened)
		return
	}
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
	mu                    sync.Mutex
	calls                 []string
	repos                 workorchrepo.WorkspaceScopedFileRepositorySet
	omitPurposeSelections bool
	mappingProposal       func(*workspace.DefinitionAuthoringRecord) (workspace.DefinitionAuthoringMapping, error)
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
		Input fixtureResponsesInput `json:"input"`
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
				mapping, buildErr := productionMapping(record)
				if record.DerivationRequests[0].WireContractRef == workspace.DefinitionAuthoringOwnerBound && p.mappingProposal == nil && buildErr == nil {
					// Consume the real production prompt catalog, returning only a scenario's
					// explicit semantic original DoD choice; no full mapping injection hook.
					mapping.ProfileAcceptanceSelections = nil
					for _, line := range strings.Split(string(request.Input), "\n") {
						var catalog struct {
							Required []struct {
								Source   struct{ HistoricalItemRef string } `json:"source"`
								Eligible []string                           `json:"eligibleAcceptanceItemRefs"`
							} `json:"requiredEvidencePurposes"`
						}
						if json.Unmarshal([]byte(line), &catalog) != nil || len(catalog.Required) == 0 {
							continue
						}
						for _, source := range catalog.Required {
							if !slices.Contains(source.Eligible, "derived-dod:template:000001") {
								buildErr = fmt.Errorf("fixture acceptance source unavailable")
								break
							}
							mapping.ProfileAcceptanceSelections = append(mapping.ProfileAcceptanceSelections, workspace.DefinitionProfileAcceptanceSelection{SourceRef: source.Source.HistoricalItemRef, AcceptanceItemRef: "derived-dod:template:000001"})
						}
					}
					if len(mapping.ProfileAcceptanceSelections) != 1 {
						buildErr = fmt.Errorf("production request lacks required source catalog")
					}
					if p.omitPurposeSelections {
						mapping.ProfileAcceptanceSelections = nil
					}
				}
				if p.mappingProposal != nil {
					mapping, buildErr = p.mappingProposal(record)
				}
				if buildErr != nil {
					err = buildErr
					break
				}
				payload, err = json.Marshal(struct {
					Mapping workspace.DefinitionAuthoringMapping `json:"mapping"`
				}{mapping})
			} else {
				kind = "review/" + pending.ReviewKind
				proposal := record.SpecificationRequests[0].Proposal
				if proposal == nil {
					err = fmt.Errorf("missing persisted proposal")
					break
				}
				goalRequirementIDs, requirementsErr := proposal.RequiredReviewRequirementIDs(pending.ReviewKind)
				if requirementsErr != nil {
					err = requirementsErr
					break
				}
				if len(goalRequirementIDs) == 0 {
					err = fmt.Errorf("missing owner-compiled required review requirements")
					break
				}
				payload, err = json.Marshal(struct {
					Review workspace.GoalSpecificationCompletenessReview `json:"review"`
				}{workspace.GoalSpecificationCompletenessReview{Kind: pending.ReviewKind, Status: workspace.GoalReviewPass, SpecificationHash: proposal.ContentHash(), Reason: "独立 scripted 审查", EvidenceRefs: []string{workspace.DefinitionAuthoringGoalItemRef}, RequirementIDs: goalRequirementIDs}})
			}
			break
		}
	}
	if kind == "" {
		payload, kind, err = emptyDirectoryRuntimeDocument(string(request.Input))

	}
	p.mu.Lock()
	p.calls = append(p.calls, kind)
	p.mu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if (kind == "derive" || kind == "proposal" || strings.HasPrefix(kind, "review/")) && len(record.DerivationRequests) == 1 && workspace.OwnerBoundAuthoringWire(record.DerivationRequests[0].WireContractRef) {
		payload = testkit.OwnerBoundFixtureDocument(payload)
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
						target["prospectiveConstraintRefs"], target["prospectiveTargetRefs"] = acquisition.Constraints, []string{"workorch_validation/entrypoint/project_test.go"}
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

func productionMapping(record *workspace.DefinitionAuthoringRecord) (workspace.DefinitionAuthoringMapping, error) {
	m, err := testkit.Mapping(record.DerivationRequests[0], workspace.DefinitionAuthoringVerificationMapping{
		Declaration: "完整保留默认模板验证约束", Status: workspace.GoalVerificationDeclared,
		ProfileID: "software-dev", EvidenceRequirementID: "*", Scope: "dodItem",
	})
	if err != nil {
		return m, err
	}
	return testkit.SelectProfileAcceptance(record.DerivationRequests[0], m, "derived-dod:template:000001")
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
