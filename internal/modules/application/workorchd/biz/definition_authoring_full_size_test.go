package biz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/testkit"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/modules/application/workorchd/service/server"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/definitionconfiguration"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	management "github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 只模拟外部 provider；创作、模板、审查记账、确认与准入均使用真实 owners。
type fullSizeAuthoringProviderHub struct {
	magicevent.Hub
	complete func(llmevents.CompleteProviderDocumentCommand) llmevents.CompleteProviderDocumentResult
}

// 本测试停在 SM-1 materialization；抑制异步 drive 通知，不执行 Runtime/provider。
func (h *fullSizeAuthoringProviderHub) Post(ev magicevent.Event) {
	if ev.ID() == runevents.TopicRunStateChanged {
		return
	}
	h.Hub.Post(ev)
}

func (h *fullSizeAuthoringProviderHub) Send(ev magicevent.Event) magicevent.Result {
	result := magicevent.NewResult(ev.ID(), ev.Source(), ev.Destination())
	switch ev.ID() {
	case llmevents.TopicLLMCommandValidateInferenceSelection:
		command := ev.Data().(llmevents.ValidateProviderInferenceSelectionCommand)
		result.Set(llmevents.ValidateProviderInferenceSelectionResult{Selection: command.Selection, CapabilityRevision: "test-provider-readiness"}, nil)
	case llmevents.TopicLLMCommandCompleteProviderDocument:
		result.Set(h.complete(ev.Data().(llmevents.CompleteProviderDocumentCommand)), nil)
	default:
		return h.Hub.Send(ev)
	}
	return result
}

type authoringActivationObservationHub struct {
	magicevent.Hub
	query func(workspaceevents.QueryDefinitionAuthoringResult) workspaceevents.QueryDefinitionAuthoringResult
}

func (h authoringActivationObservationHub) Send(ev magicevent.Event) magicevent.Result {
	response := h.Hub.Send(ev)
	if ev.ID() != workspaceevents.TopicWorkspaceDefinitionAuthoringQuery {
		return response
	}
	data, err := response.Get()
	if err != nil {
		return response
	}
	result := magicevent.NewResult(ev.ID(), ev.Source(), ev.Destination())
	result.Set(h.query(data.(workspaceevents.QueryDefinitionAuthoringResult)), nil)
	return result
}

func TestSoftwareDevRestartCompilesReviewedDefaultForFullSizeDefinition(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "data")
	hub := &fullSizeAuthoringProviderHub{Hub: magicevent.NewHub(1024)}
	runtime := testkit.NewRuntime("file", root, hub)
	defer runtime.Shutdown(ctx)
	repositories := testkit.RuntimeExports(runtime).Repositories
	calls := 0
	hub.complete = func(command llmevents.CompleteProviderDocumentCommand) llmevents.CompleteProviderDocumentResult {
		all, err := repositories.Workspaces.List(ctx)
		if err != nil || len(all) != 1 || all[0].DefinitionAuthoring == nil {
			t.Fatalf("provider 未绑定持久草稿: %v", err)
		}
		record := all[0].DefinitionAuthoring
		var payload any
		switch command.Request.Kind {
		case "workspace/specification/derive":
			payload = workspace.DefinitionDerivedAcceptance{AuthoringInputHash: record.InputHash, TemplateSnapshotHash: record.TemplateSnapshotHash, AdditionalItems: []string{}}
		case "workspace/specification/proposal":
			items, err := record.DerivationRequests[0].Proposal.CandidateItems(record.Input, &record.TemplateSnapshot)
			if err != nil {
				t.Fatal(err)
			}
			bindings, err := definitionconfiguration.Compile(all[0], &record.TemplateSnapshot)
			if err != nil {
				t.Fatal(err)
			}
			profileID, evidenceID := "", ""
			for _, profile := range bindings.ProfileContracts {
				for _, requirement := range profile.EvidenceRequirements {
					if requirement.Scope == "dodItem" {
						profileID, evidenceID = profile.ID, requirement.ID
					}
				}
			}
			if profileID == "" || evidenceID == "" {
				t.Fatal("模板缺少 dodItem proof 合同")
			}
			proposal := workspace.GoalSpecification{Version: 1, GoalItemRef: workspace.DefinitionAuthoringGoalItemRef, GoalTextHash: workspace.StableDefinitionTextHash(record.Input.GoalText), Dimensions: []workspace.GoalSpecificationDimension{{ID: "delivery", Required: true, Activated: true}}, Requirements: []workspace.GoalSpecificationRequirement{{ID: "goal", DimensionID: "delivery", Required: true, SourceKind: workspace.GoalRequirementUserExplicit, SourceItemRefs: []string{workspace.DefinitionAuthoringGoalItemRef}, Text: record.Input.GoalText}}}
			for index, item := range items {
				kind := workspace.GoalRequirementTemplateRequired
				if index < len(record.Input.AcceptanceRequirements) {
					kind = workspace.GoalRequirementUserExplicit
				}
				id := fmt.Sprintf("item-%d", index)
				proposal.Requirements = append(proposal.Requirements, workspace.GoalSpecificationRequirement{ID: id, DimensionID: "delivery", Required: true, SourceKind: kind, SourceItemRefs: []string{item.CandidateItemRef}, Text: item.Text})
				proposal.Obligations = append(proposal.Obligations, workspace.GoalSpecificationObligation{ID: "obligation-" + id, RequirementID: id, DoDItemRefs: []string{item.CandidateItemRef}, Text: item.Text})
			}
			proposal.Obligations = append(proposal.Obligations, workspace.GoalSpecificationObligation{ID: "goal-obligation", RequirementID: "goal", DoDItemRefs: []string{items[0].CandidateItemRef}, Text: "目标验收"})
			for _, obligation := range proposal.Obligations {
				proposal.Verifications = append(proposal.Verifications, workspace.GoalSpecificationVerification{ID: "verify-" + obligation.ID, ObligationID: obligation.ID, Declaration: "按已封存模板验证", Status: workspace.GoalVerificationDeclared, ProfileID: profileID, EvidenceRequirementID: evidenceID, Scope: "dodItem"})
			}
			proposal.Resources, err = workspace.AuthoringSpecificationResources(record.Input, record.DerivationRequests[0].Resources)
			if err != nil {
				t.Fatal(err)
			}
			payload = struct {
				Specification workspace.GoalSpecification `json:"specification"`
			}{authoringWireSpecificationFixture(proposal)}
		default:
			kind := strings.TrimPrefix(command.Request.Kind, "workspace/specification/review/")
			proposal := record.SpecificationRequests[0].Proposal
			if proposal == nil || len(record.SpecificationRequests) != calls {
				t.Fatal("critic permit 未先持久化")
			}
			payload = struct {
				Review workspace.GoalSpecificationCompletenessReview `json:"review"`
			}{workspace.GoalSpecificationCompletenessReview{Kind: kind, Status: workspace.GoalReviewPass, SpecificationHash: proposal.ContentHash(), Reason: "独立审查", EvidenceRefs: []string{workspace.DefinitionAuthoringGoalItemRef}, RequirementIDs: []string{"goal"}}}
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		calls++
		return authoringTestDocumentResult(t, command, encoded)
	}
	component, err := newRuntimeComponentFromCommandHub(testkit.RuntimeExports(runtime).Infrastructure.CommandHub, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer component.close(ctx)
	component.session.SetRepositoryConfig("file", root)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	transport := server.NewHTTPServerWithApplicationPorts(server.Config{Addr: address, Backend: "file", DataRoot: root}, component.applicationPorts(), nil)
	finished := make(chan error, 1)
	go func() { finished <- transport.Start() }()
	defer func() {
		if err := transport.Shutdown(ctx); err != nil {
			t.Error(err)
		}
		if err := <-finished; err != nil {
			t.Error(err)
		}
	}()
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
			t.Fatal("HTTP listener 未启动")
		}
		time.Sleep(10 * time.Millisecond)
	}
	goal := "Develop the requested multilingual bridge in Go and expose its delivered behavior through MCP."
	var criteria []string
	for i := range 24 {
		criteria = append(criteria, fmt.Sprintf("Required delivered behavior %02d is demonstrated by direct executable evidence.", i+1))
	}
	created := authoringHTTPRequest(t, baseURL, http.MethodPost, "/api/v1/workorch/workspace-authoring", management.DefinitionAuthoringCreate{RequestID: "full-size-authoring", Title: "完整规格创作", TemplateID: "software-dev", DeliverableDirectory: filepath.Join(t.TempDir(), "project"), Input: management.DefinitionAuthoringInput{GoalText: goal, AcceptanceRequirements: criteria}}, http.StatusCreated)
	path := "/api/v1/workorch/workspaces/" + created.WorkspaceID + "/authoring"
	ready := authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/compile", nil, http.StatusOK)
	if ready.State != "ready_to_confirm" || ready.Proposal == nil || len(ready.Proposal.Reviews) != 6 || calls != 8 {
		t.Fatalf("完整规格未审查: %+v calls=%d", ready, calls)
	}
	application := &Biz{Base: basebiz.New(ModuleID, hub, nil), workspaceGate: &workspaceCommandGateway{hub: hub, runtime: component}}
	if _, err := application.SubmitWorkspaceRun(ctx, workorchdevents.WorkspaceRunRequestedCommand{WorkspaceID: common.WorkspaceID(created.WorkspaceID), Intent: "restart", RequestID: "unconfirmed-full-size-activation"}); err == nil {
		t.Fatal("未确认规格启动了 Run")
	}
	if runs, err := repositories.Runs.List(ctx); err != nil || len(runs) != 0 {
		t.Fatalf("确认前自动创建 Run: %v %v", runs, err)
	}
	confirmation := management.DefinitionAuthoringConfirm{InputHash: ready.InputHash, DerivationPermitID: ready.DerivationPermitID, ProposalHash: ready.Proposal.ProposalHash, SpecificationHash: ready.Proposal.SpecificationHash, ExpectedWorkspaceRevision: ready.WorkspaceRevision}
	admitted := authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/confirm", confirmation, http.StatusOK)
	if admitted.State != "admitted" || admitted.ActivationState != "eligible" || admitted.StartupInputHash == "" {
		t.Fatalf("完整规格未准入: %+v", admitted)
	}
	if _, err := application.ReadDefinitionAuthoringView(ctx, common.WorkspaceID(created.WorkspaceID)); err != nil {
		t.Fatal(err)
	}
	activation := activationports.ActivateWorkspaceExecutionViewRequest{Hub: hub, WorkspaceID: created.WorkspaceID, DefaultDialogLanguage: "en"}
	first, err := activationports.CompileWorkspaceExecutionRestartBootstrap(ctx, activation)
	if err != nil {
		t.Fatal(err)
	}
	second, err := activationports.CompileWorkspaceExecutionRestartBootstrap(ctx, activation)
	if err != nil {
		t.Fatal(err)
	}
	if first.Validate() != nil || first.Definition.StartupInputHash != admitted.StartupInputHash || first.Definition.StartupInputHash != second.Definition.StartupInputHash || first.Definition.GoalDefinition != goal {
		t.Fatalf("Restart compiler 丢失封存身份或幂等性: validation=%v admitted=%s first=%s second=%s goal=%q", first.Validate(), admitted.StartupInputHash, first.Definition.StartupInputHash, second.Definition.StartupInputHash, first.Definition.GoalDefinition)
	}
	for _, criterion := range criteria {
		if !strings.Contains(first.Definition.DoDDefinition, criterion) {
			t.Fatal("创作改变用户验收要求")
		}
	}
	found := false
	for _, recipe := range runtimecontract.RuntimeValidationRecipeDeclarationsFromEvidencePolicies(first.Activation.EvidencePolicies) {
		if recipe.AdapterRef != shellcommon.ValidationRecipeAdapterIsolatedEntrypoint || recipe.InputReviewProtocolRef != runtimecontract.RuntimeValidationInputReviewProtocolV2 {
			t.Fatalf("未使用已审查 adapter/protocol: %+v", recipe)
		}
		found = true
	}
	if !found {
		t.Fatal("缺少已审查 adapter")
	}
	requirements, err := runtimecontract.CompileRuntimeValidationProofRequirements(first.Definition.Revision, first.CoverageItems, first.DefinitionFocuses)
	if err != nil || len(requirements) != 42 || len(first.CoverageItems) != 44 {
		t.Fatalf("完整规格覆盖不完整: proof=%d CWI=%d err=%v", len(requirements), len(first.CoverageItems), err)
	}
	stored, err := repositories.Workspaces.Get(ctx, common.WorkspaceID(created.WorkspaceID))
	if err != nil || stored.TemplateBindingHash != stored.DefinitionAuthoring.TemplateSnapshotHash || len(stored.DefinitionAuthoring.TemplateSnapshot.DoDItems) != 13 || len(ready.Proposal.Items) != 37 {
		t.Fatalf("模板必需项或物化身份丢失: %+v %v", stored, err)
	}
	beforeRevision := stored.Revision
	for _, mutation := range []string{"goal", "dod", "template", "directory", "policy", "profile"} {
		t.Run("reject_"+mutation, func(t *testing.T) {
			goal := stored.DefinitionAuthoringConfirmation.Goal
			dod := stored.DefinitionAuthoringConfirmation.DoD
			dod.Items = append([]string(nil), dod.Items...)
			template := stored.DefinitionAuthoring.TemplateSnapshot.Clone()
			current := *stored
			switch mutation {
			case "goal":
				goal.Text += " changed"
			case "dod":
				dod.Items[0] += " changed"
			case "template":
				template.Description += " changed"
			case "directory":
				current.DeliverableDirectory += "-changed"
			case "policy":
				current.EvidencePolicies = nil
			case "profile":
				current.ContextHints = current.ContextHints.Clone()
				changed := false
				for index := range current.ContextHints.Fields {
					field := &current.ContextHints.Fields[index]
					if field.Name != "profileContracts" {
						continue
					}
					for profileIndex := range field.Value.Object {
						profile := &field.Value.Object[profileIndex]
						for requirementIndex := range profile.Value.Object {
							requirements := &profile.Value.Object[requirementIndex]
							if requirements.Name != "evidenceRequirements" || len(requirements.Value.Array) == 0 {
								continue
							}
							for proofIndex := range requirements.Value.Array[0].Object {
								proof := &requirements.Value.Array[0].Object[proofIndex]
								if proof.Name == "acceptedStepKinds" {
									proof.Value.Array = append(proof.Value.Array, workorcheventpayload.Value{Kind: workorcheventpayload.ValueString, String: runtimecontract.StepKindFileRead})
									changed = true
								}
							}
						}
					}
				}
				if !changed {
					t.Fatal("Profile 漂移 fixture 未修改声明")
				}
			}
			if mutation == "policy" || mutation == "profile" {
				if _, err := definitionconfiguration.Compile(&current, &template); err != nil {
					t.Fatalf("漂移 fixture 必须结构合法: %v", err)
				}
			}
			// Template owner 仍返回真实当前模板；另外检查 detached source 漂移。
			if mutation == "template" {
				original, err := repositories.WorkspaceTemplates.Get(ctx, template.ID)
				if err != nil {
					t.Fatal(err)
				}
				changed := original.Clone()
				changed.Description += " changed"
				if err := repositories.WorkspaceTemplates.Update(ctx, &changed); err != nil {
					t.Fatal(err)
				}
				defer func() {
					if err := repositories.WorkspaceTemplates.Update(ctx, original); err != nil {
						t.Error(err)
					}
				}()
			}
			if _, err := activationports.EnsureWorkspaceDefinitionAdmission(ctx, hub, &current, &goal, &dod); err == nil {
				t.Fatal("变更输入复用了原确认准入")
			}
		})
	}
	for name, mutate := range map[string]func(workspaceevents.QueryDefinitionAuthoringResult) workspaceevents.QueryDefinitionAuthoringResult{
		"wrong_scope": func(view workspaceevents.QueryDefinitionAuthoringResult) workspaceevents.QueryDefinitionAuthoringResult {
			view.WorkspaceID = "other"
			return view
		},
		"stale_revision": func(view workspaceevents.QueryDefinitionAuthoringResult) workspaceevents.QueryDefinitionAuthoringResult {
			view.WorkspaceRevision--
			return view
		},
		"missing_record": func(view workspaceevents.QueryDefinitionAuthoringResult) workspaceevents.QueryDefinitionAuthoringResult {
			view.Record = nil
			return view
		},
		"duplicate_binding": func(view workspaceevents.QueryDefinitionAuthoringResult) workspaceevents.QueryDefinitionAuthoringResult {
			record := view.Record.Clone()
			if len(record.TemplateSnapshot.SoulBindings) == 0 {
				t.Fatal("完整模板 fixture 缺少 Soul binding")
			}
			record.TemplateSnapshot.SoulBindings = append(record.TemplateSnapshot.SoulBindings, record.TemplateSnapshot.SoulBindings[0])
			view.Record = &record
			return view
		},
		"missing_confirmation": func(view workspaceevents.QueryDefinitionAuthoringResult) workspaceevents.QueryDefinitionAuthoringResult {
			view.Confirmation = nil
			return view
		},
	} {
		t.Run("reject_query_"+name, func(t *testing.T) {
			observed := authoringActivationObservationHub{Hub: hub, query: mutate}
			goal := stored.DefinitionAuthoringConfirmation.Goal
			dod := stored.DefinitionAuthoringConfirmation.DoD
			if _, err := activationports.EnsureWorkspaceDefinitionAdmission(ctx, observed, stored, &goal, &dod); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("无效 typed 查询复用了准入: %v", err)
			}
		})
	}
	current, err := repositories.Workspaces.Get(ctx, stored.ID)
	if err != nil || current.Revision != beforeRevision || current.StartupInput.StartupInputHash != admitted.StartupInputHash {
		t.Fatalf("重复编译或漂移拒绝写入 owner: %+v %v", current, err)
	}
	runs, err := repositories.Runs.List(ctx)
	if err != nil || len(runs) != 0 || calls != 8 {
		t.Fatalf("创作或只读编译产生 Run/provider 重发: runs=%v calls=%d err=%v", runs, calls, err)
	}
	application.workspaceGate = &workspaceCommandGateway{hub: hub, runtime: component}
	command := workorchdevents.WorkspaceRunRequestedCommand{WorkspaceID: common.WorkspaceID(created.WorkspaceID), Intent: "restart", RequestID: "explicit-full-size-activation"}
	activated, err := application.SubmitWorkspaceRun(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := application.SubmitWorkspaceRun(ctx, command)
	if err != nil || activated.RunID == "" || replayed.RunID != activated.RunID {
		t.Fatalf("显式 activation 未精确重放: %+v %+v %v", activated, replayed, err)
	}
	current, err = repositories.Workspaces.Get(ctx, stored.ID)
	if err != nil || current.DefaultLLMSelection == nil || calls != 8 {
		t.Fatalf("默认 selection 物化或 activation 重放重发 provider: calls=%d err=%v", calls, err)
	}
	runs, err = repositories.Runs.List(ctx)
	if err != nil || len(runs) != 1 || runs[0].ActivationRequestID != command.RequestID || runs[0].StartupInputHash != admitted.StartupInputHash || runs[0].RuntimeBootstrap == nil || runs[0].RuntimeBootstrap.Definition.StartupInputHash != admitted.StartupInputHash {
		t.Fatalf("显式启动未封存原确认输入或创建重复 Run: %v %v", runs, err)
	}
}
