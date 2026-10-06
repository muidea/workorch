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
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	application "github.com/muidea/workorch/internal/modules/application/workorchd"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	management "github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 严格 fail-first 公共正例，不是 BootstrapOnly，也不接受 unavailable 为成功。
// 只复用主最新 default_empty_directory_test.go 的 public helpers；本文件不改
// template/availability，不直接创建 Run、bootstrap、authority、binding 或 receipt。
// 当前默认 Profile 未声明 work policy、availability 仍 closed 时，本正例应失败。
// synthetic provider 仅提议文档；所有交付文件、Git、证据与终态由 production owners 产生。
func TestImplementationWorkPublicAuthoringFullFlow(t *testing.T) {
	ctx := context.Background()
	storage, directory := t.TempDir(), t.TempDir()
	repos := workorchrepo.NewWorkspaceScopedFileRepositorySet(storage)
	provider := &implementationWorkFullflowProvider{authoring: emptyDirectoryProvider{repos: repos}, reviewedInputs: map[string]string{}}
	proxy := httptest.NewServer(http.HandlerFunc(provider.serveHTTP))
	t.Cleanup(proxy.Close)
	baseURL, client := implementationWorkFullflowStart(t, ctx, storage, proxy.URL, repos)
	template, err := repos.WorkspaceTemplates.Get(ctx, "software-dev")
	if err != nil {
		t.Fatal(err)
	}
	assertEmptyDeliverable(t, directory)
	var created struct {
		Authoring management.DefinitionAuthoringView `json:"authoring"`
	}
	productionJSON(t, client, http.MethodPost, baseURL+"/api/v1/workorch/workspace-authoring", management.DefinitionAuthoringCreate{
		RequestID: "implementation-work-public-fullflow", Title: "公开创建期到同原 CWI 两轮真实工程交付", TemplateID: template.ID, DeliverableDirectory: directory,
		Input: management.DefinitionAuthoringInput{
			GoalText:               "创建一个多文件 Go CLI 工程。真实生产实现 internal/logic.DeliveredValue 最终返回 4，cmd/delivery 输出 4。保留原全部 DoD；在同一个原 dodItem 上先完成两轮独立 durable 实现变更，再独立审查运行时生成的验证输入。真实行为失败后仅修复实现，重新完成 all_of、build/test/race，最后由 Git owner 本地提交并完成 Result/Workspace/Run 全部交付确认。",
			AcceptanceRequirements: []string{"同一个原 dodItem CWI 至少两轮一般工作，工作 receipt 不授予终验 credit", "最终 CLI 输出 4，真实失败后 bounded exact repair，验证输入不弱化，fresh build/test/race 全通过"},
		},
	}, &created)
	if created.Authoring.WorkspaceID == "" {
		t.Fatal("公共创建未返回 Workspace")
	}
	workspaceID := common.WorkspaceID(created.Authoring.WorkspaceID)
	path := baseURL + "/api/v1/workorch/workspaces/" + created.Authoring.WorkspaceID
	assertNoRun(t, repos)
	assertEmptyDeliverable(t, directory)
	productionJSON(t, client, http.MethodPost, path+"/authoring/compile", nil, nil)
	var reviewed management.DefinitionAuthoringView
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); {
		var envelope struct {
			Authoring management.DefinitionAuthoringView `json:"authoring"`
		}
		productionJSON(t, client, http.MethodGet, path+"/authoring", nil, &envelope)
		reviewed = envelope.Authoring
		assertNoRun(t, repos)
		if reviewed.State == "ready_to_confirm" {
			break
		}
		if reviewed.State == "failed" || reviewed.State == "preparation_failed" {
			t.Fatalf("公共 authoring 失败，不能降格 bootstrap: %+v", reviewed)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if reviewed.State != "ready_to_confirm" || reviewed.Proposal == nil || len(reviewed.Proposal.Reviews) != 6 {
		t.Fatalf("六 review 未完整完成: %+v", reviewed)
	}
	seen := map[string]bool{}
	for _, review := range reviewed.Proposal.Reviews {
		if review.Status != workspace.GoalReviewPass || seen[review.Kind] {
			t.Fatalf("review 未独立完整通过: %+v", review)
		}
		seen[review.Kind] = true
	}
	for _, kind := range []string{workspace.GoalReviewDimensionCoverage, workspace.GoalReviewNegativePath, workspace.GoalReviewRegression, workspace.GoalReviewResourceEnvironment, workspace.GoalReviewTraceability, workspace.GoalReviewVerifierFeasibility} {
		if !seen[kind] {
			t.Fatalf("缺少 review %s", kind)
		}
	}
	assertEmptyDeliverable(t, directory)
	confirmation := management.DefinitionAuthoringConfirm{InputHash: reviewed.InputHash, DerivationPermitID: reviewed.Proposal.DerivationPermitID, ProposalHash: reviewed.Proposal.ProposalHash, SpecificationHash: reviewed.Proposal.SpecificationHash, ExpectedWorkspaceRevision: reviewed.WorkspaceRevision}
	productionJSON(t, client, http.MethodPost, path+"/authoring/confirm", confirmation, nil)
	current, err := repos.Workspaces.Get(ctx, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if current.StartupInput == nil || current.ActivationAdmission == nil || current.ActivationAdmission.State != workspace.WorkspaceActivationEligible || current.DefinitionAuthoringConfirmation == nil || current.DefinitionAuthoringConfirmation.InputHash != confirmation.InputHash || current.DefinitionAuthoringConfirmation.SpecificationHash != confirmation.SpecificationHash || current.ActivationAdmission.StartupInputHash != current.StartupInput.StartupInputHash {
		t.Fatal("exact confirmation/eligible 身份未封存")
	}
	startupBytes, err := json.Marshal(current.StartupInput)
	if err != nil {
		t.Fatal(err)
	}
	assertNoRun(t, repos)
	assertEmptyDeliverable(t, directory)
	productionJSON(t, client, http.MethodPost, path+"/execution-activations", management.WorkspaceExecutionCommandRequest{Intent: "restart", RequestID: "implementation-work-explicit-first-execution"}, nil)

	var first *runmodel.Run
	var initialBootstrap []byte
	for deadline := time.Now().Add(3 * time.Minute); time.Now().Before(deadline); {
		runs, err := repos.Runs.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(runs) > 1 {
			t.Fatalf("公开显式 activation 创建重复 Run: %d", len(runs))
		}
		if len(runs) == 1 {
			first = runs[0]
			if first.RuntimeBootstrap != nil && len(initialBootstrap) == 0 {
				initialBootstrap, err = json.Marshal(first.RuntimeBootstrap)
				if err != nil {
					t.Fatal(err)
				}
				policyPresent := false
				for _, profile := range first.RuntimeBootstrap.Activation.ExecutionContract.ProfileContracts {
					for _, requirement := range profile.EvidenceRequirements {
						policyPresent = policyPresent || requirement.Scope == "dodItem" && requirement.ImplementationWorkPolicy != nil
					}
				}
				if !policyPresent {
					t.Fatal("严格 fullflow 前置失败：正式默认 Profile 未封存 dodItem implementationWorkPolicy；不能以普通 source_change 或 bootstrap 冒两轮工作正例")
				}
			}
			if first.State == common.RunStateFailed {
				t.Fatalf("真实 Runtime 失败，不能当正例通过: state=%+v provider=%+v", first.RuntimeState, provider.snapshot())
			}
			if first.State == common.RunStateCompleted && first.RuntimeState != nil && len(first.RuntimeState.DeliveryAcknowledgements) == 3 {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if first == nil || first.RuntimeState == nil || first.State != common.RunStateCompleted {
		t.Fatalf("完整公开 DriveRuntime 未完成: run=%+v provider=%+v", first, provider.snapshot())
	}
	if err := first.RuntimeState.Validate(); err != nil {
		t.Fatal(err)
	}
	finalBootstrap, err := json.Marshal(first.RuntimeBootstrap)
	if err != nil || !bytes.Equal(initialBootstrap, finalBootstrap) || first.StartupInputHash != current.StartupInput.StartupInputHash {
		t.Fatal("真实执行修改 sealed bootstrap/startup 身份")
	}
	finalWorkspace, err := repos.Workspaces.Get(ctx, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	finalStartup, err := json.Marshal(finalWorkspace.StartupInput)
	if err != nil || !bytes.Equal(startupBytes, finalStartup) || finalWorkspace.ActiveRunID != "" {
		t.Fatal("Workspace 未释放 Run 或修改确认 startup")
	}
	implementationWorkFullflowAssertDocuments(t, first.RuntimeState)
	commitRef := implementationWorkFullflowAssertOutcome(t, first.RuntimeState, provider, directory)
	results, err := repos.Results.ListByRun(ctx, first.ID)
	if err != nil || len(results) != 1 || results[0].State != common.ResultStateAccepted || results[0].RuntimeCloseout == nil {
		t.Fatalf("Result 未正式接受全 closeout: %+v err=%v", results, err)
	}
	closeout := results[0].RuntimeCloseout
	if !slices.Contains(closeout.AcceptedEvidenceRefs, commitRef) {
		t.Fatal("正式 Result 未保留 exact Git owner commit receipt 引用")
	}
	if closeout.StartupInputHash != first.StartupInputHash || len(closeout.UnsatisfiedCoverageWorkItemIDs) != 0 || len(closeout.SatisfiedCoverageWorkItemIDs) != len(first.RuntimeState.Facts.DefinitionSeed.CoverageItems) || len(closeout.AcceptedEvidenceRefs) == 0 {
		t.Fatal("Result 未保留完整原 CWI/all_of/evidence links")
	}
	for _, item := range first.RuntimeState.Facts.DefinitionSeed.CoverageItems {
		if !slices.Contains(closeout.SatisfiedCoverageWorkItemIDs, item.ID) {
			t.Fatalf("Result 缺原 CWI %s", item.ID)
		}
	}
	owners := map[runtimecontract.RuntimeDeliveryOwner]bool{}
	for _, ack := range first.RuntimeState.DeliveryAcknowledgements {
		if err := ack.Validate(); err != nil {
			t.Fatal(err)
		}
		if ack.WorkspaceID != workspaceID || ack.RunID != first.ID || owners[ack.Owner] || ack.AcknowledgedState == runtimecontract.RuntimeDeliveryAcknowledgedNoop {
			t.Fatal("terminal ack 重复、错 scope 或 noop 冒正式确认")
		}
		owners[ack.Owner] = true
	}
	for _, owner := range []runtimecontract.RuntimeDeliveryOwner{runtimecontract.RuntimeDeliveryOwnerResult, runtimecontract.RuntimeDeliveryOwnerWorkspace, runtimecontract.RuntimeDeliveryOwnerRun} {
		if !owners[owner] {
			t.Fatalf("缺 owner ack %s", owner)
		}
	}
	for _, kind := range []string{"derive", "proposal", "review/" + workspace.GoalReviewDimensionCoverage, "review/" + workspace.GoalReviewNegativePath, "review/" + workspace.GoalReviewRegression, "review/" + workspace.GoalReviewResourceEnvironment, "review/" + workspace.GoalReviewTraceability, "review/" + workspace.GoalReviewVerifierFeasibility} {
		count := 0
		for _, call := range provider.authoring.snapshot() {
			if call == kind {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("公共 authoring 请求不是 exact-once: %s=%d", kind, count)
		}
	}
}

// 负例保留真正不受支持的 selector，日后仅开放具体组合也不能一概 available。
func TestImplementationWorkFullflowUnsupportedCombinationStaysClosed(t *testing.T) {
	policy := runtimecontract.RuntimeImplementationWorkPolicy{
		ProtocolRef: runtimecontract.RuntimeImplementationWorkProtocolV1, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		AllowedOperations: []runtimecontract.RuntimeOperationVariantRef{{Kind: runtimecontract.StepKindFileEdit}}, AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope},
		TargetSelectorRef: "file/unimplemented-fullflow-selector", TargetSelectorOwnerRef: "software-dev", TargetSelectorSubjectRef: "dodItem", MaxRoundsPerCWI: 2, MaxTargetsPerRound: 4, RequireFreshObservation: true, RequireFreshValidation: true, ProtectValidationInputs: true,
	}
	if err := policy.Validate(); err != nil {
		t.Fatalf("负例须是语法合法、执行不支持的 policy: %v", err)
	}
	profiles := []runtimecontract.RuntimeProfileContract{{ID: "software-dev", EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{ID: "*", Scope: "dodItem", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, ImplementationWorkPolicy: &policy}}}}
	if err := runtimecontract.ValidateRuntimeImplementationWorkAvailability(profiles); !errors.Is(err, runtimecontract.ErrRuntimeImplementationWorkUnavailable) {
		t.Fatalf("不支持组合须 closed，不能用测试绕过: %v", err)
	}
}

func implementationWorkFullflowStart(t *testing.T, ctx context.Context, storage, proxyURL string, repos workorchrepo.WorkspaceScopedFileRepositorySet) (string, *http.Client) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	configDir := t.TempDir()
	config := fmt.Sprintf(`endpointName = "workorch-fullflow-integration"
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
`, address, storage, proxyURL)
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = workorchconfig.Close()
	workorchconfig.ConfigureDirectory(configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })
	cfg, err := workorchconfig.Current()
	if err != nil {
		t.Fatal(err)
	}
	hub, background := magicevent.NewHub(1024), task.NewBackgroundRoutine(1024)
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
	client, base := &http.Client{Timeout: 10 * time.Second}, "http://"+address
	waitForHTTP(t, client, base)
	return base, client
}

type implementationWorkFullflowProvider struct {
	authoring      emptyDirectoryProvider
	mu             sync.Mutex
	calls          []string
	failures       []string
	workCWI        common.CoverageWorkItemID
	workOrdinal    int
	reviewedInputs map[string]string
	criteria       map[common.CoverageWorkItemID]string
	inputCriteria  map[string]string
}

type implementationWorkFullflowProviderSnapshot struct {
	Calls, Failures []string
	WorkCWI         common.CoverageWorkItemID
	ReviewedInputs  map[string]string
}

func (p *implementationWorkFullflowProvider) snapshot() implementationWorkFullflowProviderSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	v := implementationWorkFullflowProviderSnapshot{Calls: append([]string(nil), p.calls...), Failures: append([]string(nil), p.failures...), WorkCWI: p.workCWI, ReviewedInputs: map[string]string{}}
	for target, hash := range p.reviewedInputs {
		v.ReviewedInputs[target] = hash
	}
	return v
}

func (p *implementationWorkFullflowProvider) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v1/responses" {
		p.authoring.serveHTTP(w, r)
		return
	}
	wire, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var request struct {
		Input string `json:"input"`
	}
	if err := json.Unmarshal(wire, &request); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var authority map[string]json.RawMessage
	for _, line := range strings.Split(request.Input, "\n") {
		var value map[string]json.RawMessage
		if json.Unmarshal([]byte(line), &value) == nil && (value["planStep"] != nil || value["outputContract"] != nil) {
			authority = value
			break
		}
	}
	if authority == nil {
		r.Body = io.NopCloser(bytes.NewReader(wire))
		p.authoring.serveHTTP(w, r)
		return
	}
	p.mu.Lock()
	var payload []byte
	var kind string
	if authority["planStep"] != nil {
		payload, kind, err = p.semantic(authority)
	} else {
		payload, kind, err = p.candidate(authority)
	}
	p.calls = append(p.calls, kind)
	if err != nil {
		p.failures = append(p.failures, err.Error())
	}
	p.mu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	event, err := json.Marshal(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed", "output_text": string(payload), "usage": map[string]any{"input_tokens": 128, "output_tokens": 128, "total_tokens": 256}}})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", event)
}

// 以下 DTO 只读 provider-safe C3，绝不从 Run repo 重建 Runtime authority。
type implementationWorkFullflowWork struct {
	CoverageID        common.CoverageWorkItemID                       `json:"coverageWorkItemId"`
	PurposeChoiceOnly bool                                            `json:"purposeChoiceOnly"`
	EffectAvailable   bool                                            `json:"effectAvailable"`
	Ordinal           int                                             `json:"ordinal"`
	Policy            runtimecontract.RuntimeImplementationWorkPolicy `json:"policy"`
}
type implementationWorkFullflowPreparation struct {
	NativeCheckRef     string `json:"nativeCheckRef"`
	TargetRef          string `json:"targetRef"`
	ContentContractRef string `json:"contentContractRef"`
}
type implementationWorkFullflowValidation struct {
	Recipe  string   `json:"validationRecipeId"`
	Recipes []string `json:"selectableValidationRecipeIds"`
	Checks  []string `json:"requiredCheckRefs"`
	Inputs  []struct {
		Recipe            string                                             `json:"validationRecipeId"`
		Hash              string                                             `json:"subjectHash"`
		Protocol          string                                             `json:"protocolRef"`
		ScopeContractRef  string                                             `json:"scopeContractRef"`
		ScopeRequirements string                                             `json:"scopeRequirements"`
		Files             []runtimecontract.RuntimeValidationInputReviewFile `json:"files"`
	} `json:"inputReviews"`
}
type implementationWorkFullflowObligation struct {
	CoverageID    common.CoverageWorkItemID             `json:"coverageWorkItemId"`
	Declaration   string                                `json:"declarationRef"`
	EvidenceKinds []string                              `json:"currentRequiredEvidenceKinds"`
	Constraints   []string                              `json:"declarationProspectiveTargetAllOfConstraintRefs"`
	Work          *implementationWorkFullflowWork       `json:"implementationWork"`
	Validation    *implementationWorkFullflowValidation `json:"validationProofRequirement"`
	Acquisition   *struct {
		Owner       string                                       `json:"acquisitionOwnerModuleId"`
		Kind        string                                       `json:"acquisitionOperationKind"`
		Policy      runtimecontract.RuntimeOperationTargetPolicy `json:"acquisitionTargetPolicy"`
		Selector    string                                       `json:"acquisitionTargetSelectorRef"`
		Constraints []string                                     `json:"acquisitionConstraintRefs"`
		Prospective *struct {
			Preparations []implementationWorkFullflowPreparation `json:"validationProofPreparations"`
		} `json:"prospectiveTargetConstraint"`
	} `json:"ownerFactAcquisition"`
	Repair *struct {
		Owner       string                                         `json:"repairOwnerModuleId"`
		Kinds       []string                                       `json:"repairOperationKinds"`
		Policy      runtimecontract.RuntimeOperationTargetPolicy   `json:"repairTargetPolicy"`
		Selector    string                                         `json:"repairOperationTargetSelectorRef"`
		Constraints []string                                       `json:"repairConstraintRefs"`
		Addition    runtimecontract.RuntimeValidationInputAddition `json:"inputAddition"`
	} `json:"validationFailureRepair"`
}

func (p *implementationWorkFullflowProvider) semantic(raw map[string]json.RawMessage) ([]byte, string, error) {
	var request struct {
		Step     runtimecontract.RuntimeSemanticPlanStep `json:"planStep"`
		Batch    string                                  `json:"batchId"`
		Sequence int                                     `json:"batchSequence"`
		BatchIDs []common.CoverageWorkItemID             `json:"batchCoverageWorkItemIds"`
		Manifest struct {
			Hash        string                                 `json:"hash"`
			SeedHash    string                                 `json:"definitionSeedHash"`
			Items       []runtimecontract.RuntimeCoverageItem  `json:"coverageItems"`
			Obligations []implementationWorkFullflowObligation `json:"proofObligations"`
			Work        *implementationWorkFullflowWork        `json:"implementationPurpose"`
			Catalogs    []struct {
				Owner   string   `json:"ownerModuleId"`
				Targets []string `json:"targetRefs"`
				Recipes []string `json:"validationRecipeRefs"`
			} `json:"ownerCatalogs"`
		} `json:"manifest"`
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, "runtime/semantic", err
	}
	if err := json.Unmarshal(data, &request); err != nil {
		return nil, "runtime/semantic", err
	}
	if request.Manifest.Hash == "" || request.Manifest.SeedHash == "" {
		return nil, "runtime/semantic", fmt.Errorf("缺 exact manifest/seed identity")
	}
	if p.criteria == nil {
		p.criteria, p.inputCriteria = map[common.CoverageWorkItemID]string{}, map[string]string{}
	}
	for _, item := range request.Manifest.Items {
		if previous, present := p.criteria[item.ID]; present && previous != item.TargetText {
			return nil, "runtime/semantic", fmt.Errorf("provider 观察到原 criterion 漂移: %s", item.ID)
		}
		p.criteria[item.ID] = item.TargetText
	}
	kind := "runtime/" + string(request.Step)
	document := map[string]any{"documentKind": runtimecontract.RuntimeProviderDocumentSemanticPlan, "schemaVersion": runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent, "manifestHash": request.Manifest.Hash, "definitionSeedHash": request.Manifest.SeedHash, "planStep": request.Step, "proposals": []any{}}
	if request.Step == runtimecontract.RuntimeSemanticPlanStepCoverageOrder {
		order := []common.CoverageWorkItemID{}
		for _, item := range request.Manifest.Items {
			if item.SourceScope == "deliverable" && item.SourceID == "source_change" {
				order = append(order, item.ID)
			}
		}
		for _, item := range request.Manifest.Items {
			if item.SourceScope != "deliverable" || item.SourceID != "source_change" {
				order = append(order, item.ID)
			}
		}
		document["coverageOrder"] = order
		work := request.Manifest.Work
		if work == nil && len(request.Manifest.Obligations) == 1 {
			work = request.Manifest.Obligations[0].Work
		}
		if work != nil && work.PurposeChoiceOnly {
			if p.workCWI == "" {
				p.workCWI = work.CoverageID
			}
			purpose := runtimecontract.RuntimeImplementationWorkPurposeTerminal
			if work.CoverageID == p.workCWI && work.Ordinal <= 2 {
				if !work.EffectAvailable {
					return nil, kind, fmt.Errorf("原 CWI 第%d轮 working effect 不可用", work.Ordinal)
				}
				purpose = runtimecontract.RuntimeImplementationWorkPurposeEffect
			}
			p.workOrdinal = work.Ordinal
			document["implementationPurpose"] = purpose
			kind += "/" + string(purpose)
		}
	} else if request.Step == runtimecontract.RuntimeSemanticPlanStepProofBatch {
		document["batchId"], document["batchSequence"], document["batchCoverageWorkItemIds"] = request.Batch, request.Sequence, request.BatchIDs
		proposals := []any{}
		for _, obligation := range request.Manifest.Obligations {
			proposal := map[string]any{"coverageWorkItemId": obligation.CoverageID, "declarationRef": obligation.Declaration, "requiredEvidenceKinds": obligation.EvidenceKinds}
			target := map[string]any{}
			switch {
			case obligation.Work != nil:
				work := obligation.Work
				if work.CoverageID != p.workCWI || work.Ordinal < 1 || work.Ordinal > 2 {
					return nil, kind, fmt.Errorf("working proof 非原两轮 CWI")
				}
				p.workOrdinal = work.Ordinal
				proposal["ownerModuleId"], proposal["operationKind"] = runtimecontract.RuntimeModuleFile, runtimecontract.StepKindFileEdit
				target = map[string]any{"policy": runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope, "ownerScopeTargetRefs": []string{"internal/logic/logic.go"}}
			case obligation.Repair != nil:
				repair := obligation.Repair
				if len(repair.Kinds) != 1 || repair.Kinds[0] != runtimecontract.StepKindFileEdit || repair.Addition.TargetRef != "" {
					return nil, kind, fmt.Errorf("此工程仅支持 exact behavior File/edit 修复，不借 input repair 权限")
				}
				proposal["ownerModuleId"], proposal["operationKind"] = repair.Owner, repair.Kinds[0]
				target = map[string]any{"policy": repair.Policy, "ownerSelectorRefs": []string{repair.Selector}, "ownerScopeTargetRefs": []string{"internal/logic/logic.go"}}
			case obligation.Acquisition != nil:
				acquisition := obligation.Acquisition
				if acquisition.Prospective == nil || len(acquisition.Prospective.Preparations) != 1 || acquisition.Prospective.Preparations[0].TargetRef == "" {
					return nil, kind, fmt.Errorf("owner 必须发布唯一 exact input preparation，不能猜验证目标")
				}
				proposal["ownerModuleId"], proposal["operationKind"] = acquisition.Owner, acquisition.Kind
				prepared := acquisition.Prospective.Preparations[0]
				p.inputCriteria[prepared.TargetRef] = p.criteria[obligation.CoverageID]
				target = map[string]any{"policy": acquisition.Policy, "ownerSelectorRefs": []string{acquisition.Selector}, "prospectiveConstraintRefs": acquisition.Constraints, "prospectiveTargetRefs": []string{prepared.TargetRef}}
			case obligation.Validation != nil:
				validation := obligation.Validation
				recipe := validation.Recipe
				if recipe == "" && len(validation.Recipes) == 1 {
					recipe = validation.Recipes[0]
				}
				if recipe == "" {
					return nil, kind, fmt.Errorf("validation recipe 不唯一，不猜 owner recipe")
				}
				proposal["ownerModuleId"], proposal["operationKind"], proposal["validationRecipeId"] = runtimecontract.RuntimeModuleShell, runtimecontract.RuntimeBlockKindManagedValidationSession, recipe
				target = map[string]any{"policy": runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope, "ownerScopeTargetRefs": implementationWorkFullflowCatalogTargets(request.Manifest.Catalogs, runtimecontract.RuntimeModuleFile)}
				for _, input := range validation.Inputs {
					if input.Recipe != recipe {
						continue
					}
					review := runtimecontract.RuntimeValidationInputReview{SubjectHash: input.Hash}
					criterion := ""
					for _, item := range request.Manifest.Items {
						if item.ID == obligation.CoverageID {
							criterion = item.TargetText
						}
					}
					refs := []string{}
					for _, file := range input.Files {
						if runtimecontract.RuntimeValidationInputContentHash(file.Content) != file.ContentHash {
							return nil, kind, fmt.Errorf("input 原文/hash 错配")
						}
						refs = append(refs, file.TargetRef)
						if before, exists := p.reviewedInputs[file.TargetRef]; exists && before != file.ContentHash {
							return nil, kind, fmt.Errorf("已发布验证输入发生变化 %s", file.TargetRef)
						}
						p.reviewedInputs[file.TargetRef] = file.ContentHash
					}
					conclusion, finding, reason := implementationWorkFullflowReviewInput(criterion, input.Files, input.ScopeRequirements)
					for _, check := range validation.Checks {
						item := runtimecontract.RuntimeValidationInputCheckReview{RequiredCheckRef: check, Conclusion: conclusion, Reason: reason, InputTargetRefs: refs}
						if input.Protocol == runtimecontract.RuntimeValidationInputReviewProtocolV2 {
							item.Obligations = []runtimecontract.RuntimeValidationInputObligationReview{{CriterionText: criterion, Finding: finding, Reason: item.Reason, InputTargetRefs: refs}}
						}
						review.Checks = append(review.Checks, item)
					}
					proposal["validationInputReview"] = review
					kind += "/independent_input_review"
				}
			default:
				var sourceID string
				for _, item := range request.Manifest.Items {
					if item.ID == obligation.CoverageID && item.SourceScope == "deliverable" {
						sourceID = item.SourceID
					}
				}
				if sourceID == "source_change" {
					proposal["ownerModuleId"], proposal["operationKind"] = runtimecontract.RuntimeModuleFile, runtimecontract.StepKindFileWrite
					targets := []string{}
					for ref := range implementationWorkFullflowProjectFiles() {
						targets = append(targets, ref)
					}
					slices.Sort(targets)
					target = map[string]any{"policy": runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget, "ownerSelectorRefs": []string{"workspace-relative-new-file-target"}, "prospectiveConstraintRefs": obligation.Constraints, "prospectiveTargetRefs": targets}
				} else if sourceID == "version_control_snapshot" {
					proposal["ownerModuleId"], proposal["operationKind"], proposal["operationVariantId"] = runtimecontract.RuntimeModuleGit, runtimecontract.StepKindGitWrite, "commit"
					target = map[string]any{"policy": runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope, "ownerScopeTargetRefs": implementationWorkFullflowCatalogTargets(request.Manifest.Catalogs, runtimecontract.RuntimeModuleFile)}
				} else {
					// Goal、acceptanceDimension、dodItem 与 validation_report 都必须消费
					// 当前 owner-published validation requirement，不能退化为任意 File proof。
					return nil, kind, fmt.Errorf("完整验证 consumer 无 owner-published validationProofRequirement: CWI=%s source=%s", obligation.CoverageID, sourceID)
				}
			}
			proposal["target"] = target
			proposals = append(proposals, proposal)
		}
		document["proposals"] = proposals
	} else {
		return nil, kind, fmt.Errorf("未知 semantic step %s", request.Step)
	}
	payload, err := json.Marshal(document)
	return payload, kind, err
}

func implementationWorkFullflowCatalogTargets(catalogs []struct {
	Owner   string   `json:"ownerModuleId"`
	Targets []string `json:"targetRefs"`
	Recipes []string `json:"validationRecipeRefs"`
}, owner string) []string {
	for _, catalog := range catalogs {
		if catalog.Owner == owner {
			return append([]string(nil), catalog.Targets...)
		}
	}
	return nil
}

func (p *implementationWorkFullflowProvider) candidate(raw map[string]json.RawMessage) ([]byte, string, error) {
	var request struct {
		Decision runtimecontract.RuntimeDecision `json:"decision"`
		Contract struct {
			State  runtimecontract.ConvergenceState `json:"state"`
			Layout string                           `json:"canonicalWireLayout"`
			Rules  []struct {
				Ref        string   `json:"requiredOperationRef"`
				Kind       string   `json:"compiledKind"`
				Variant    string   `json:"compiledVariantId"`
				Fields     []string `json:"providerFields"`
				Parameters []string `json:"providerParameterNames"`
			} `json:"wireRules"`
			Operations []struct {
				Kind   string `json:"kind"`
				Target struct {
					runtimecontract.RuntimeOperationTargetAuthorization
					AllowedTargetRefs []string `json:"allowedTargetRefs"`
				} `json:"target"`
				Recipes []string `json:"validationRecipeIds"`
			} `json:"operationContracts"`
		} `json:"outputContract"`
		Context struct {
			Evidence     []string                                `json:"allowedAcceptedEvidenceRefs"`
			Preparations []implementationWorkFullflowPreparation `json:"validationProofPreparations"`
			Repair       json.RawMessage                         `json:"repairDirective"`
		} `json:"context"`
	}
	wire, err := json.Marshal(raw)
	if err != nil {
		return nil, "runtime/candidate", err
	}
	if err := json.Unmarshal(wire, &request); err != nil {
		return nil, "runtime/candidate", err
	}
	kind := "runtime/" + string(request.Contract.State)
	if request.Contract.State == runtimecontract.ConvergenceStateNeedAssessment {
		if len(request.Context.Evidence) == 0 {
			return nil, kind, fmt.Errorf("assessment 不得无真实 owner evidence 提议 satisfied")
		}
		items := []any{}
		for _, id := range request.Decision.Focus.CoverageWorkItemIDs {
			items = append(items, map[string]any{"coverageWorkItemId": id, "satisfied": true, "reason": "仅引用当前 focus 允许的 receiving-owner evidence，Runtime 独立判定。", "evidenceRefs": request.Context.Evidence})
		}
		payload, err := json.Marshal(map[string]any{"focusId": request.Decision.Focus.ID, "coverageWorkItemIds": request.Decision.Focus.CoverageWorkItemIDs, "disposition": "satisfied", "items": items, "summary": "真实 owner 证据验收提议，不写终态 authority", "evidenceRefs": request.Context.Evidence})
		return payload, kind, err
	}
	if request.Contract.State != runtimecontract.ConvergenceStateNeedStateChange && request.Contract.State != runtimecontract.ConvergenceStateNeedValidation {
		return nil, kind, fmt.Errorf("未支持 candidate state %s", request.Contract.State)
	}
	var layout struct {
		Operations []map[string]any `json:"operations"`
	}
	layoutJSON := request.Contract.Layout
	if start := strings.IndexByte(layoutJSON, '{'); start >= 0 {
		layoutJSON = layoutJSON[start:]
	}
	if err := json.Unmarshal([]byte(layoutJSON), &layout); err != nil || len(layout.Operations) == 0 {
		return nil, kind, fmt.Errorf("缺 canonical provider wire layout: %v", err)
	}
	operations := []any{}
	for _, example := range layout.Operations {
		ruleIndex := -1
		for i, candidateRule := range request.Contract.Rules {
			if ref, ok := example["requiredOperationRef"].(string); ok && ref == candidateRule.Ref && ref != "" {
				ruleIndex = i
				break
			}
			if kind, ok := example["kind"].(string); ok && kind == candidateRule.Kind && candidateRule.Ref == "" {
				ruleIndex = i
				break
			}
		}
		if ruleIndex < 0 {
			return nil, kind, fmt.Errorf("canonical layout/wireRules 错配")
		}
		rule := request.Contract.Rules[ruleIndex]
		var target runtimecontract.RuntimeOperationTargetAuthorization
		var allowedTargets, recipes []string
		for _, contract := range request.Contract.Operations {
			if contract.Kind == rule.Kind {
				target, recipes = contract.Target.RuntimeOperationTargetAuthorization, contract.Recipes
				allowedTargets = contract.Target.AllowedTargetRefs
				break
			}
		}
		refs := append([]string(nil), target.ProspectiveTargetRefs...)
		if len(refs) == 0 {
			refs = append([]string(nil), target.ExactTargetRefs...)
		}
		if len(refs) == 0 {
			refs = append([]string(nil), target.OwnerScopeTargetRefs...)
		}
		if len(refs) == 0 {
			refs = append([]string(nil), allowedTargets...)
		}
		refs = slices.DeleteFunc(refs, func(ref string) bool { return slices.Contains(target.ExcludedTargetRefs, ref) })
		if len(refs) == 0 {
			return nil, kind, fmt.Errorf("owner 未给 candidate target inventory: %s", rule.Kind)
		}
		if rule.Kind == runtimecontract.StepKindFileEdit {
			if !slices.Contains(refs, "internal/logic/logic.go") {
				return nil, kind, fmt.Errorf("edit 未授权生产实现")
			}
			refs = []string{"internal/logic/logic.go"}
		}
		if rule.Kind == runtimecontract.RuntimeBlockKindManagedValidationSession || rule.Kind == runtimecontract.StepKindGitWrite {
			refs = refs[:1]
		}
		for _, ref := range refs {
			operation := map[string]any{}
			for _, field := range rule.Fields {
				if value, ok := example[field]; ok && field != "parameters" {
					operation[field] = value
				}
			}
			if slices.Contains(rule.Fields, "requiredOperationRef") {
				operation["requiredOperationRef"] = rule.Ref
			}
			if slices.Contains(rule.Fields, "kind") {
				operation["kind"] = rule.Kind
			}
			if slices.Contains(rule.Fields, "targetRefs") {
				operation["targetRefs"] = []string{ref}
			}
			parameters := []any{}
			add := func(name string, value any) {
				if slices.Contains(rule.Parameters, name) {
					entry := map[string]any{"name": name}
					switch value.(type) {
					case []string:
						entry["stringList"] = value
					default:
						entry["string"] = value
					}
					parameters = append(parameters, entry)
				}
			}
			switch rule.Kind {
			case runtimecontract.StepKindFileWrite:
				content, exists := implementationWorkFullflowProjectFiles()[ref]
				if !exists {
					for _, preparation := range request.Context.Preparations {
						if preparation.TargetRef == ref && preparation.NativeCheckRef != "" {
							content = implementationWorkFullflowInput(ref, preparation.NativeCheckRef, p.inputCriteria[ref])
							exists = true
							break
						}
					}
				}
				if !exists {
					return nil, kind, fmt.Errorf("不猜 input/nativecheck identity: %s", ref)
				}
				add("content", content)
			case runtimecontract.StepKindFileEdit:
				old, new := "return 0", "return 1"
				if p.workOrdinal == 2 {
					old, new = "return 1", "return 3"
				}
				if len(request.Context.Repair) != 0 && string(request.Context.Repair) != "null" {
					old, new = "return 3", "return 4"
					kind += "/exact_behavior_repair"
				}
				add("oldText", old)
				add("newText", new)
			case runtimecontract.RuntimeBlockKindManagedValidationSession:
				if len(recipes) != 1 {
					return nil, kind, fmt.Errorf("candidate recipe 不唯一")
				}
				add("recipeId", recipes[0])
			case runtimecontract.StepKindGitWrite:
				add("operation", rule.Variant)
				if rule.Variant == "add" {
					add("pathspecs", []string{"."})
				}
				if rule.Variant == "commit" {
					add("message", "feat: synthetic public fullflow delivery")
				}
			default:
				return nil, kind, fmt.Errorf("未支持 concrete operation %s", rule.Kind)
			}
			if slices.Contains(rule.Fields, "parameters") {
				operation["parameters"] = parameters
			}
			operations = append(operations, operation)
		}
	}
	payload, err := json.Marshal(map[string]any{"operations": operations})
	return payload, kind, err
}

func implementationWorkFullflowProjectFiles() map[string]string {
	return map[string]string{
		"go.mod":                  "module example.com/workorch-fullflow\n\ngo 1.26\n",
		"internal/logic/logic.go": "package logic\n\nfunc DeliveredValue() int { return 0 }\n",
		"cmd/delivery/main.go":    "package main\nimport (\"fmt\"; \"example.com/workorch-fullflow/internal/logic\")\nfunc main() { fmt.Println(logic.DeliveredValue()) }\n",
		"internal/logic/logic_test.go": `package logic
import ("testing"; "sync")
func TestDeliveredValueProduction(t *testing.T) {
 if got:=DeliveredValue(); got != 4 { t.Fatalf("WORKORCH_ASSERTION_V1 code=value_mismatch expected=equal observed=different: %d",got) }
}
func TestDeliveredValueConcurrent(t *testing.T) {
 want:=DeliveredValue(); var wait sync.WaitGroup
 for n:=0;n<16;n++ { wait.Add(1); go func(){ defer wait.Done(); if got:=DeliveredValue(); got!=want { t.Errorf("concurrent instability %d != %d",got,want) } }() }
 wait.Wait()
}
`,
		"README.md": `# 合成工程

## 目标与验收映射
真实生产实现 internal/logic.DeliveredValue 最终返回 4，cmd/delivery 输出 4。
主路径调用生产逻辑与最终 CLI；错误路径由错误值引发真实 WORKORCH_ASSERTION_V1。
并发路径由 production package TestDeliveredValueConcurrent 调用同一生产实现验证稳定性。
运行时逐 CWI 生成独立验证包，不能用 test-local fake/helper 或常量 PASS 替代交付实现。

## 验证与静态检查
完整工程命令：go build ./...、go test ./...、go test -race ./...、go vet ./...。
每个隔离 CWI 验证包还明确执行生产包 build/test/race/vet 并构建运行 CLI；该包不能冒整个相邻验证包集合。
实际运行命令、失败与跳过、修复和重新验证结果以真实 owner receipts 和正式 Result 为准。
此文档记录规格和命令，不声明尚未执行的命令、Git或终态事务已成功。

## 治理、交付与残余风险
同原 CWI 两轮工作、失败后 exact repair、StartupInput 不变、独立 input review 与最终 Git snapshot 均需 Runtime/receiving owner 原账本证明。
决策和审批仅在实际发生时由 Decision/Approval owner 记录；不得写虚构工单。
正式 Result 须逐项保留 DoD evidenceRefs、命令与实际结果、失败/跳过说明及 artifacts；Workspace/Run 全部 ack 后才算交付。
残余风险：isolated input scope 不含 Runtime/Git/Result 账本，也不覆盖相邻验证包；需要对应 typed owner consumer 和 fresh full-suite recipe，不能以 README 自述放行。
`,
	}
}

// 验证文件作为 provider content 提议交给真实 File owner；测试本身不预写输入。
// 仅一个 top-level Test，调用实际生产实现并构建/执行最终 CLI。
func implementationWorkFullflowInput(target, native, criterion string) string {
	depth := strings.Count(target, "/")
	root := strings.TrimSuffix(strings.Repeat("../", depth), "/")
	if root == "" {
		root = "."
	}
	return fmt.Sprintf(`package checks
import ("testing"; "os"; "os/exec"; "path/filepath"; "strings"; "example.com/workorch-fullflow/internal/logic")
func %s(t *testing.T) {
 criterion := %q
 root := %q
 if logic.DeliveredValue()!=4 { t.Fatal("WORKORCH_ASSERTION_V1 code=value_mismatch expected=equal observed=different") }
 // 执行真实工程的生产包，不使用常量或 native-check marker 代替。
 for _, args := range [][]string{{"build","-buildvcs=false","./..."},{"test","./internal/...","./cmd/..."},{"test","-race","./internal/...","./cmd/..."},{"vet","./internal/...","./cmd/..."}} {
  command:=exec.Command("go",args...); command.Dir=root
  if output,err:=command.CombinedOutput(); err!=nil { t.Fatalf("real project command %%v: %%v %%s",args,err,output) }
 }
 documentation,err:=os.ReadFile(filepath.Join(root,"README.md")); if err!=nil { t.Fatal(err) }
 for _, required:=range []string{"目标与验收映射","主路径","错误路径","并发","go build ./...","go test ./...","go test -race ./...","go vet ./...","owner receipts","残余风险"} {
  if !strings.Contains(string(documentation),required) { t.Fatalf("documentation lacks %%s for %%s",required,criterion) }
 }
 bin:=filepath.Join(t.TempDir(),"delivery")
 build:=exec.Command("go","build","-buildvcs=false","-o",bin,"./cmd/delivery"); build.Dir=root
 if output,err:=build.CombinedOutput(); err!=nil { t.Fatalf("build: %%v %%s",err,output) }
 output,err:=exec.Command(bin).Output(); if err!=nil || strings.TrimSpace(string(output))!="4" { t.Fatal("WORKORCH_ASSERTION_V1 code=value_mismatch expected=equal observed=different") }
}
`, native, criterion, root)
}

// 独立review仅分析owner给出的exact input bytes/scope；不把目标文本包含某关键字
// 当已满足，不读取Run repo，不把README自述当receipts或Git提交事实。
func implementationWorkFullflowReviewInput(criterion string, files []runtimecontract.RuntimeValidationInputReviewFile, scope string) (string, string, string) {
	input := ""
	for _, file := range files {
		if strings.HasSuffix(file.TargetRef, "_test.go") {
			input += file.Content
		}
	}
	checks := []string{"logic.DeliveredValue()!=4", `exec.Command("go",args...)`, `{"build","-buildvcs=false","./..."}`, `{"test","./internal/...","./cmd/..."}`, `{"test","-race","./internal/...","./cmd/..."}`, `{"vet","./internal/...","./cmd/..."}`, `os.ReadFile(filepath.Join(root,"README.md"))`, `exec.Command(bin).Output()`, `strings.TrimSpace(string(output))!="4"`}
	for _, expression := range checks {
		if !strings.Contains(input, expression) {
			return runtimecontract.RuntimeValidationInputConflict, runtimecontract.RuntimeValidationInputMissing, "owner原文缺少实际生产实现、构建/测试/并发/文档或最终CLI断言，不能报告完整覆盖。"
		}
	}
	// 当前隔离 subject 不含 owner Run 账本和未来终态事务；
	// 只能观察工程文件的输入不能实现这些 owner 义务。
	for _, term := range []string{"Git", "git/write", "版本控制", "owner receipt", "capability-owner receipt", "evidenceRefs", "DoD 验收评估", "决策工单", "审批工单", "两轮", "修复", "全部交付确认"} {
		if strings.Contains(criterion, term) {
			return runtimecontract.RuntimeValidationInputInconclusive, runtimecontract.RuntimeValidationInputUnknown, "原criterion包含真实Runtime/Git/终态owner账本或故障恢复义务；当前owner input subject只含工程验证包与模块，不能读取或伪造这些尚未发生的事实，需对应typed owner证明consumer。"
		}
	}
	if strings.Contains(criterion, "完整") || strings.Contains(criterion, "全") {
		if strings.Contains(scope, "not the project-wide test suite") {
			return runtimecontract.RuntimeValidationInputInconclusive, runtimecontract.RuntimeValidationInputUnknown, "该输入实际执行完整build与生产包test/race/vet，但不包含相邻validation包；原full-suite/all_of义务需独立完整owner recipe，不能由isolated package作用域推定。"
		}
	}
	return runtimecontract.RuntimeValidationInputConsistent, runtimecontract.RuntimeValidationInputCovered, "逐项核对owner原文：直接调用生产逻辑，执行build/production包test/race/vet，读取并检查交付文档，并构建/执行最终CLI且断言4；不是constant PASS，review不授予实际执行或terminal credit。"
}

func implementationWorkFullflowAssertOutcome(t *testing.T, state *runtimecontract.RuntimeStateRecord, p *implementationWorkFullflowProvider, directory string) string {
	t.Helper()
	observed := p.snapshot()
	if len(observed.Failures) != 0 {
		t.Fatalf("scripted provider 合同失败，不可算 fullflow: %v", observed.Failures)
	}
	rounds := []runtimecontract.RuntimeImplementationWorkRound{}
	for _, round := range state.ImplementationWork.Rounds {
		if round.CoverageWorkItemID == observed.WorkCWI {
			rounds = append(rounds, round)
		}
	}
	if observed.WorkCWI == "" || len(rounds) != 2 {
		t.Fatalf("未取得同原 CWI 两轮，不接受普通 source_change 替代: %+v", state.ImplementationWork)
	}
	original := false
	for _, item := range state.Facts.DefinitionSeed.CoverageItems {
		if item.ID == observed.WorkCWI && item.SourceScope == "dodItem" {
			original = true
		}
	}
	if !original {
		t.Fatal("两轮工作不属于原 dodItem")
	}
	for i, round := range rounds {
		if round.Ordinal != i+1 || round.CompletedReceiptSetHash == "" || round.WorkingBinding == nil || round.ScopeObservation == nil || round.FailedReceiptHash != "" {
			t.Fatal("工作 round 无完整原 owner transaction")
		}
		if i == 1 && (round.PrecedingReceiptSetHash != rounds[0].CompletedReceiptSetHash || round.ScopeObservation.Selection.ObservedRevision == rounds[0].ScopeObservation.Selection.ObservedRevision) {
			t.Fatal("工作 round2 无 prior receipt/fresh scope")
		}
		found := false
		for _, dispatch := range state.CompletedDispatches {
			if dispatch.ID != round.DispatchID {
				continue
			}
			found = true
			if dispatch.ImplementationWorkCommitment == nil || len(dispatch.CoverageBindings) != 0 {
				t.Fatal("工作 dispatch 冒 terminal binding")
			}
			for _, fact := range state.Facts.Evidence.Accepted {
				if fact.DispatchID == dispatch.ID && (len(fact.BindingRefs) != 0 || len(fact.CoverageWorkItemIDs) != 0 || fact.Validation != nil) {
					t.Fatal("working receipt 获得 terminal credit")
				}
			}
		}
		if !found {
			t.Fatal("work round 无 completed original dispatch")
		}
	}
	if len(observed.ReviewedInputs) == 0 {
		t.Fatal("未经过独立 owner input review")
	}
	for target, hash := range observed.ReviewedInputs {
		data, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(target)))
		if err != nil || runtimecontract.RuntimeValidationInputContentHash(string(data)) != hash {
			t.Fatalf("已发布验证输入被修改: %s err=%v", target, err)
		}
	}
	failed, repaired, fresh, gitCommit := false, false, false, false
	commitRef := ""
	var repairReceipt runtimecontract.RuntimeOperationReceipt
	repairDispatchIndex := -1
	for index, dispatch := range state.CompletedDispatches {
		commitment := dispatch.RepairCommitment
		if commitment == nil {
			continue
		}
		if err := commitment.Validate(); err != nil {
			t.Fatal(err)
		}
		if commitment.RepairOwnerModuleID != runtimecontract.RuntimeModuleFile || commitment.InputRepair != nil || dispatch.RepairCommitmentHash != runtimecontract.StableRuntimeDispatchRepairCommitmentHash(*commitment) {
			t.Fatal("repair 不是 exact File 实现修复 commitment")
		}
		sourceMatches := 0
		for _, receipt := range state.OperationReceipts {
			if receipt.ExecutionKey != commitment.SourceReceiptExecutionKey || receipt.Fence != commitment.SourceReceiptFence {
				continue
			}
			sourceMatches++
			if runtimecontract.StableRuntimeOperationReceiptHash(receipt) != commitment.SourceReceiptHash || receipt.Evidence.Validation == nil || len(receipt.Evidence.Validation.FailedChecks) == 0 {
				t.Fatal("repair source key/fence/hash 未绑定真实失败 validation receipt")
			}
		}
		evidenceMatches := 0
		for _, fact := range state.Facts.Evidence.Rejected {
			if fact.ID != commitment.SourceEvidenceFactID {
				continue
			}
			evidenceMatches++
			if fact.DispatchID != commitment.SourceDispatchID || fact.OperationID != commitment.SourceOperationID || runtimecontract.StableRuntimeEvidenceFactHash(fact) != commitment.SourceEvidenceFactHash {
				t.Fatal("repair source 未 exact join 原 rejected evidence")
			}
		}
		if sourceMatches != 1 || evidenceMatches != 1 {
			t.Fatal("repair 原 receipt/evidence source 不唯一")
		}
		failed = true
		for _, execution := range dispatch.OperationExecutions {
			for _, receipt := range state.OperationReceipts {
				if receipt.ExecutionKey != execution.ExecutionKey || receipt.Fence != execution.Fence {
					continue
				}
				if execution.State != runtimecontract.OperationExecutionEvidenceCommitted || receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || !receipt.Changed || !slices.Equal(receipt.TargetRefs, []string{"internal/logic/logic.go"}) {
					t.Fatal("exact repair 无真实实现 mutation receipt")
				}
				repairReceipt, repairDispatchIndex, repaired = receipt, index, true
			}
		}
	}
	executedRecipes := map[string]bool{}
	for dispatchIndex, dispatch := range state.CompletedDispatches {
		for _, execution := range dispatch.OperationExecutions {
			for _, receipt := range state.OperationReceipts {
				if receipt.ExecutionKey != execution.ExecutionKey || receipt.Fence != execution.Fence {
					continue
				}
				if repaired && dispatchIndex > repairDispatchIndex && receipt.Evidence.Validation != nil && len(receipt.Evidence.Validation.PassedChecks) != 0 && len(receipt.Evidence.Validation.FailedChecks) == 0 && len(receipt.Evidence.Validation.SkippedChecks) == 0 && len(receipt.Evidence.Validation.IncompleteChecks) == 0 {
					repairAt, repairErr := time.Parse(time.RFC3339Nano, repairReceipt.CompletedAt)
					validationAt, validationErr := time.Parse(time.RFC3339Nano, receipt.CompletedAt)
					if repairErr != nil || validationErr != nil || validationAt.Before(repairAt) || execution.State != runtimecontract.OperationExecutionEvidenceCommitted || receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted {
						t.Fatal("fresh validation 不在 exact repair receipt 后或无 committed accepted owner execution")
					}
					matchesCWI := false
					for _, binding := range dispatch.CoverageBindings {
						if binding.CoverageWorkItemID == observed.WorkCWI {
							matchesCWI = true
						}
					}
					if !matchesCWI {
						continue
					}
					fresh = true
					for _, recipe := range dispatch.ValidationRecipes {
						if recipe.ID != receipt.Evidence.Validation.RecipeID || filepath.Base(recipe.Command) != "go" || len(recipe.Args) == 0 {
							continue
						}
						if recipe.Args[0] == "build" {
							executedRecipes["build"] = true
						}
						if recipe.Args[0] == "test" {
							executedRecipes["test"] = true
							if slices.Contains(recipe.Args, "-race") {
								executedRecipes["race"] = true
							}
						}
					}
				}
			}
		}
		if dispatch.Candidate.StateChange != nil {
			for _, member := range dispatch.Candidate.StateChange.Operations {
				if member.Operation.Kind != runtimecontract.StepKindGitWrite {
					continue
				}
				for _, parameter := range member.Operation.Parameters {
					if parameter.Name == "operation" && parameter.String != nil && *parameter.String == "commit" {
						for _, execution := range dispatch.OperationExecutions {
							if execution.OperationID != member.Operation.ID || execution.State != runtimecontract.OperationExecutionEvidenceCommitted {
								continue
							}
							matches := 0
							for _, receipt := range state.OperationReceipts {
								if receipt.ExecutionKey != execution.ExecutionKey || receipt.Fence != execution.Fence {
									continue
								}
								matches++
								if runtimecontract.StableRuntimeOperationReceiptHash(receipt) != execution.OwnerReceiptHash {
									t.Fatal("Git commit execution 未绑定原 owner receipt hash")
								}
								ref, err := implementationWorkFullflowCommitReceiptRef(receipt, member.Operation.ID)
								if err != nil || (commitRef != "" && commitRef != ref) {
									t.Fatalf("Git commit execution 未唯一绑定 accepted durable receipt: %v", err)
								}
								commitRef, gitCommit = ref, true
							}
							if matches != 1 {
								t.Fatal("Git commit execution 原 key/fence receipt 不唯一")
							}
						}
					}
				}
			}
		}
	}
	if !failed || !repaired || !fresh || !gitCommit {
		t.Fatalf("完整 owner 故障/修复/fresh/Git链不成立: failed=%t repair=%t fresh=%t gitCommit=%t", failed, repaired, fresh, gitCommit)
	}
	for _, check := range []string{"build", "test", "race"} {
		if !executedRecipes[check] {
			t.Fatalf("未取得真实 fresh %s recipe receipt；native check 名称、输入内 build、README或外跑命令不能冒 sealed owner 命令证据: %v", check, executedRecipes)
		}
	}
	content, err := os.ReadFile(filepath.Join(directory, "internal", "logic", "logic.go"))
	if err != nil || !strings.Contains(string(content), "return 4") {
		t.Fatal("最终生产实现未实际修复")
	}
	if _, err := os.Stat(filepath.Join(directory, ".git")); err != nil {
		t.Fatal("真实 Git owner 未创建临时repo")
	}
	if reflect.DeepEqual(implementationWorkFullflowProjectFiles()["internal/logic/logic.go"], string(content)) {
		t.Fatal("工程未产生真实 effect")
	}
	finalFiles := implementationWorkFullflowProjectFiles()
	finalFiles["internal/logic/logic.go"] = strings.Replace(finalFiles["internal/logic/logic.go"], "return 0", "return 4", 1)
	if err := implementationWorkFullflowVerifyCommitTree(directory, commitRef, finalFiles, observed.ReviewedInputs); err != nil {
		t.Fatalf("Git receipt 未绑定最终完整交付 tree/bytes: %v", err)
	}
	return commitRef
}

// 对真实已产生的 manifest/document 做纯编解码合同回归；不构造owner事实、不写authority。
// 仅在公开主线实际到达此处后才形成证据，helper 存在不代表该路径通过。
func implementationWorkFullflowAssertDocuments(t *testing.T, state *runtimecontract.RuntimeStateRecord) {
	t.Helper()
	checkSemantic := func(ledger runtimecontract.RuntimeSemanticPlanningLedger) {
		if ledger.Manifest == nil {
			return
		}
		if ledger.CoverageOrderDocument != nil {
			if _, err := runtimecontract.EncodeRuntimeSemanticPlanPayloadWithManifest(*ledger.Manifest, *ledger.CoverageOrderDocument); err != nil {
				t.Fatalf("original semantic order wire: %v", err)
			}
		}
		for _, document := range ledger.BatchDocuments {
			if _, err := runtimecontract.EncodeRuntimeSemanticPlanPayloadWithManifest(*ledger.Manifest, document); err != nil {
				t.Fatalf("original semantic batch wire: %v", err)
			}
		}
	}
	checkSemantic(state.SemanticPlanning)
	for _, history := range state.SemanticPlanning.History {
		checkSemantic(history.Snapshot)
	}
	for _, dispatch := range state.CompletedDispatches {
		matches := 0
		for _, attempt := range state.CandidateAttempts {
			if attempt.DispatchID != dispatch.ID || attempt.State != runtimecontract.RuntimeCandidateAttemptStateAccepted {
				continue
			}
			matches++
			if !runtimecontract.RuntimeAcceptedCandidateAttemptMatchesDispatch(attempt, dispatch) {
				t.Fatal("dispatch无独立exact admission source")
			}
		}
		if matches != 1 {
			t.Fatal("dispatch不是唯一original accepted admission")
		}
	}
}
