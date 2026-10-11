package biz

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/testkit"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	workspacetestkit "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/testkit"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/definitionconfiguration"
	management "github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

type definitionChangeReviewProviderHub struct {
	*fullSizeAuthoringProviderHub
	loseCompletion   bool
	loseReserve      bool
	loseRegistration bool
	lostCompletions  int
}

func (h *definitionChangeReviewProviderHub) Send(ev magicevent.Event) magicevent.Result {
	response := h.fullSizeAuthoringProviderHub.Send(ev)
	if h.loseCompletion && (ev.ID() == workspaceevents.TopicWorkspaceDerivationComplete || ev.ID() == workspaceevents.TopicWorkspaceAuthoringSpecificationComplete) || h.loseReserve && ev.ID() == workspaceevents.TopicWorkspaceDerivationReserve || h.loseRegistration && ev.ID() == decisionevents.TopicRegisterContractChangeProposal {
		h.lostCompletions++
		lost := magicevent.NewResult(ev.ID(), ev.Source(), ev.Destination())
		lost.Set(nil, &cd.Error{Code: cd.Unexpected, Message: "实际 completion 持久写后响应丢失"})
		return lost
	}
	return response
}

func (h *definitionChangeReviewProviderHub) Drain(ctx context.Context) *cd.Error {
	return h.Hub.(interface {
		Drain(context.Context) *cd.Error
	}).Drain(ctx)
}

func (h *definitionChangeReviewProviderHub) TerminateChecked(ctx context.Context) *cd.Error {
	return h.Hub.(magicevent.DrainingHub).TerminateChecked(ctx)
}

// 仅 provider document 为 mock；Decision、Workspace、File 及原确认准入均使用实际装配。
func TestDefinitionChangeActualCandidateDerivationAndSixReviewsKeepOriginalSeal(t *testing.T) {
	for _, scenario := range []string{"complete_lost", "reserve_lost", "missing_review", "proposal_drift"} {
		t.Run(scenario, func(t *testing.T) { testDefinitionChangeCandidateReview(t, scenario) })
	}
}

func testDefinitionChangeCandidateReview(t *testing.T, scenario string) {
	t.Helper()
	ctx := context.Background()
	root, directory := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "requirements.txt"), []byte("原资源\r\n完整保留"), 0o600); err != nil {
		t.Fatal(err)
	}
	hub := &definitionChangeReviewProviderHub{fullSizeAuthoringProviderHub: &fullSizeAuthoringProviderHub{Hub: magicevent.NewHub(256)}}
	runtime := testkit.NewRuntime("file", root, hub)
	t.Cleanup(func() {
		if err := runtime.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	repos := testkit.RuntimeExports(runtime).Repositories
	app := &Biz{Base: basebiz.New(ModuleID, hub, nil), authoringTasks: newAuthoringTaskLifecycle()}
	calls := 0
	hub.complete = func(command llmevents.CompleteProviderDocumentCommand) llmevents.CompleteProviderDocumentResult {
		all, err := repos.Workspaces.List(ctx)
		if err != nil || len(all) != 1 {
			t.Fatalf("provider scope=%v", err)
		}
		w := all[0]
		record := w.DefinitionAuthoring
		if w.DefinitionChangeCandidate != nil {
			record = &w.DefinitionChangeCandidate.Record
		}
		var derivation *workspace.DefinitionDerivationRequest
		for n := range record.DerivationRequests {
			if record.DerivationRequests[n].Input.InputHash() == record.InputHash {
				derivation = &record.DerivationRequests[n]
			}
		}
		if derivation == nil {
			t.Fatal("provider 未绑定持久 permit")
		}
		var payload any
		switch command.Request.Kind {
		case "workspace/specification/derive":
			if derivation.Proposal != nil || derivation.FailureCode != "" {
				t.Fatal("重发已结算派生")
			}
			if len(derivation.Resources) != 1 || derivation.Resources[0].Content != "原资源\r\n完整保留" {
				t.Fatal("未取得实际 File 原文")
			}
			payload = workspace.DefinitionDerivedAcceptance{AuthoringInputHash: record.InputHash, TemplateSnapshotHash: record.TemplateSnapshotHash, AdditionalItems: []string{}}
		case "workspace/specification/proposal":
			bindings, err := definitionconfiguration.Compile(w, &record.TemplateSnapshot)
			if err != nil {
				t.Fatal(err)
			}
			verification := workspace.DefinitionAuthoringVerificationMapping{Status: workspace.GoalVerificationDeclared, Declaration: "已封存模板的逐义务证明"}
			for _, profile := range bindings.ProfileContracts {
				for _, requirement := range profile.EvidenceRequirements {
					if requirement.Scope == "dodItem" {
						verification.ProfileID, verification.EvidenceRequirementID, verification.Scope = profile.ID, requirement.ID, requirement.Scope
					}
				}
			}
			mapping, _, _, err := workspacetestkit.Proposal(*derivation, verification)
			if err != nil {
				t.Fatal(err)
			}
			payload = struct {
				Mapping workspace.DefinitionAuthoringMapping `json:"mapping"`
			}{mapping}
		default:
			kind := strings.TrimPrefix(command.Request.Kind, "workspace/specification/review/")
			var proposal *workspace.GoalSpecification
			for _, request := range record.SpecificationRequests {
				if request.DerivationPermitID == derivation.PermitID && request.Proposal != nil {
					proposal = request.Proposal
				}
			}
			if proposal == nil {
				t.Fatal("critic 无持久 proposal")
			}
			for _, section := range command.Request.PromptContext.Sections {
				if strings.Contains(section.Content, `"Reviews":[{`) {
					t.Fatal("critic 继承前序结论")
				}
			}
			ids, err := proposal.RequiredReviewRequirementIDs(kind)
			if err != nil {
				t.Fatal(err)
			}
			if w.DefinitionChangeCandidate != nil && scenario == "missing_review" && len(ids) > 1 {
				ids = ids[:len(ids)-1]
			}
			payload = struct {
				Review workspace.GoalSpecificationCompletenessReview `json:"review"`
			}{workspace.GoalSpecificationCompletenessReview{Kind: kind, Status: workspace.GoalReviewPass, SpecificationHash: proposal.ContentHash(), Reason: "独立完整覆盖", EvidenceRefs: []string{workspace.DefinitionAuthoringGoalItemRef}, RequirementIDs: ids}}
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		calls++
		return authoringTestDocumentResult(t, command, encoded)
	}
	created, err := app.CreateDefinitionAuthoringView(ctx, "file", root, management.DefinitionAuthoringCreate{RequestID: "create", Title: "独立候选审查", TemplateID: "software-dev", DeliverableDirectory: directory, Input: management.DefinitionAuthoringInput{GoalText: "原合同目标", ResourceRefs: []string{"requirements.txt"}, AcceptanceRequirements: []string{"原用户验收"}}})
	if err != nil {
		t.Fatal(err)
	}
	id := common.WorkspaceID(created.WorkspaceID)
	if _, err := app.DeriveDefinitionAcceptance(ctx, id); err != nil {
		t.Fatal(err)
	}
	ready, err := app.CompileDefinitionAuthoringView(ctx, id)
	if err != nil || ready.Proposal == nil || calls != 8 {
		t.Fatalf("原定义审查=%v calls=%d", err, calls)
	}
	sealed, err := app.ConfirmDefinitionAuthoringView(ctx, id, management.DefinitionAuthoringConfirm{InputHash: ready.InputHash, DerivationPermitID: ready.DerivationPermitID, ProposalHash: ready.Proposal.ProposalHash, SpecificationHash: ready.Proposal.SpecificationHash, ExpectedWorkspaceRevision: ready.WorkspaceRevision})
	if err != nil || sealed.ActivationState != "eligible" {
		t.Fatalf("原定义封存=%v", err)
	}
	before, err := repos.Workspaces.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	proposal := decisionmodel.ContractChangeProposal{SchemaVersion: 1, WorkspaceID: id, BaseInputRevision: before.DefinitionAuthoring.Input.Revision, BaseInputHash: before.DefinitionAuthoring.InputHash, BaseDefinitionRevision: before.StartupInput.DefinitionRevision, BaseStartupInputHash: before.StartupInput.StartupInputHash, ProposedBy: "test-user", TriggerFacts: "事实", Discovery: "发现", RootCauseOrUnknown: "未知", WhyChange: "完整原因", EvidenceGaps: "候选须重审", Differences: []decisionmodel.ContractChangeDifference{{ItemID: workspace.DefinitionAuthoringGoalItemRef, Kind: "goal", Action: "modify", OriginalText: before.DefinitionAuthoring.Input.GoalText, ProposedText: "新候选目标", SemanticChange: "待批准"}}, Alternatives: "原合同", NoChangeConsequences: "原合同继续", UnaffectedContinuationConditions: "原范围", Impacts: []decisionmodel.ContractChangeImpact{{SubjectID: "goal", Kind: "requirement", ProposedHandling: "recheck", Reason: "重审"}}, EffectsAndReceipts: "保留", OutstandingCosts: "保留", ReservedBudget: "保留", AdditionalCostDurationRisk: "未知", RequiredAuthorization: "未批准", DecisionScope: "合同", ApplicationConditions: "exact approve", RecoveryRestrictions: "不能回滚"}
	registration, err := app.RegisterContractChangeProposal(ctx, decisionevents.RegisterContractChangeProposalCommand{TicketID: "change", RequestID: "register", Proposal: proposal})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := app.PrepareDefinitionChange(ctx, workspaceevents.PrepareDefinitionChangeCommand{WorkspaceID: id, RequestID: "prepare", ExpectedWorkspaceRevision: before.Revision, TicketID: registration.TicketID, ProposalRevision: registration.Revision, ProposalHash: registration.Hash})
	if err != nil {
		t.Fatal(err)
	}
	hash := prepared.Candidate.Record.InputHash
	if _, err := app.RegisterReviewedDefinitionChange(ctx, id, "premature-final"); err == nil || calls != 8 {
		t.Fatal("未审候选登记最终身份")
	}
	hub.loseCompletion = scenario == "complete_lost"
	if scenario == "reserve_lost" || scenario == "proposal_drift" {
		if scenario == "reserve_lost" {
			hub.loseReserve = true
		} else {
			revision := decisionevents.RegisterContractChangeProposalCommand{TicketID: registration.TicketID, RequestID: "revise", ExpectedRevision: registration.Revision, ExpectedHash: registration.Hash, Proposal: proposal.Clone()}
			revision.Proposal.WhyChange = "持久提案漂移"
			if _, err := app.RegisterContractChangeProposal(ctx, revision); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := app.DeriveDefinitionChangeAcceptance(ctx, id, hash); err == nil || calls != 8 {
			t.Fatalf("拒绝反例仍调用 provider=%v calls=%d", err, calls)
		}
		hub.loseReserve = false
		fresh := &Biz{Base: basebiz.New(ModuleID, hub, nil), authoringTasks: newAuthoringTaskLifecycle()}
		if _, err := fresh.DeriveDefinitionChangeAcceptance(ctx, id, hash); err == nil || calls != 8 {
			t.Fatalf("fresh 重发未决/漂移调用=%v", err)
		}
		stored, err := repos.Workspaces.Get(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if scenario == "reserve_lost" {
			requests := stored.DefinitionChangeCandidate.Record.DerivationRequests
			last := requests[len(requests)-1]
			if len(requests) != len(before.DefinitionAuthoring.DerivationRequests)+1 || last.Proposal != nil || last.FailureCode != "" {
				t.Fatal("持久 reserve 未保留 unknown")
			}
		}
		check := *stored
		check.Revision, check.DefinitionChangeCandidate = before.Revision, nil
		if !reflect.DeepEqual(check, *before) {
			t.Fatal("拒绝改写原定义")
		}
		return
	}
	if _, err := app.DeriveDefinitionChangeAcceptance(ctx, id, "self-declared"); !errors.Is(err, common.ErrConflict) || calls != 8 {
		t.Fatalf("自证候选=%v calls=%d", err, calls)
	}
	if _, err := app.DeriveDefinitionChangeAcceptance(ctx, id, hash); err != nil {
		t.Fatal(err)
	}
	candidateSpec, err := app.CompileDefinitionChangeSpecification(ctx, id, hash)
	if scenario == "missing_review" {
		if err == nil || calls != 11 {
			t.Fatalf("漏 required review 被接受=%v calls=%d", err, calls)
		}
		if _, err := app.CompileDefinitionChangeSpecification(ctx, id, hash); err == nil || calls != 11 {
			t.Fatal("漏审结果重发 provider")
		}
		stored, err := repos.Workspaces.Get(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		record := stored.DefinitionChangeCandidate.Record
		last := record.SpecificationRequests[len(record.SpecificationRequests)-1]
		if last.FailureCode != "invalid_review" || last.Diagnostic == nil {
			t.Fatal("漏审原诊断未保存")
		}
		if _, err := app.QueryAuthoringDiagnostic(ctx, workspaceevents.QueryAuthoringDiagnosticCommand{WorkspaceID: id, PermitID: last.PermitID}); err != nil {
			t.Fatal(err)
		}
		check := *stored
		check.Revision, check.DefinitionChangeCandidate = before.Revision, nil
		if !reflect.DeepEqual(check, *before) {
			t.Fatal("漏审改写原定义")
		}
		return
	}
	if err != nil || len(candidateSpec.Reviews) != 6 || calls != 16 {
		t.Fatalf("候选六审=%v calls=%d", err, calls)
	}
	if hub.lostCompletions != 8 {
		t.Fatalf("实际 completion 响应丢失覆盖=%d", hub.lostCompletions)
	}
	freshApp := &Biz{Base: basebiz.New(ModuleID, hub, nil), authoringTasks: newAuthoringTaskLifecycle()}
	if replay, err := freshApp.CompileDefinitionChangeSpecification(ctx, id, hash); err != nil || !reflect.DeepEqual(replay, candidateSpec) || calls != 16 {
		t.Fatalf("候选重放=%v calls=%d", err, calls)
	}
	after, err := repos.Workspaces.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	record := after.DefinitionChangeCandidate.Record
	if len(record.DerivationRequests) != len(before.DefinitionAuthoring.DerivationRequests)+1 || len(record.SpecificationRequests) != len(before.DefinitionAuthoring.SpecificationRequests)+7 {
		t.Fatal("候选重置原预算")
	}
	candidatePermit := record.DerivationRequests[len(record.DerivationRequests)-1].PermitID
	if _, _, err := record.ConfirmableSpecification(candidatePermit, candidateSpec.ContentHash()); err != nil {
		t.Fatal(err)
	}
	check := *after
	check.Revision, check.DefinitionChangeCandidate = before.Revision, nil
	if !reflect.DeepEqual(check, *before) {
		t.Fatal("候选六审修改原 seal/journal")
	}
	original, err := app.QueryDefinitionAuthoring(ctx, workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: id})
	if err != nil || original.Record.InputHash != before.DefinitionAuthoring.InputHash || original.ActivationAdmission == nil {
		t.Fatalf("默认查询改为候选=%v", err)
	}
	if runs, err := repos.Runs.List(common.ContextWithRuntimeWorkspaceID(ctx, id)); err != nil || len(runs) != 0 {
		t.Fatalf("候选审查启动 Run=%v", err)
	}
	for _, request := range record.SpecificationRequests {
		if request.DerivationPermitID == candidatePermit && request.Proposal != nil {
			audit, err := app.QueryAuthoringSuccessAudit(ctx, workspaceevents.QueryAuthoringSuccessAuditCommand{WorkspaceID: id, PermitID: request.PermitID})
			if err != nil || audit.Audit.Document.SHA256 != request.SuccessAudit.Document.SHA256 {
				t.Fatalf("候选原文审计=%v", err)
			}
		}
	}
	if scenario == "complete_lost" {
		view, err := app.QueryDefinitionChange(ctx, workspaceevents.QueryDefinitionChangeCommand{WorkspaceID: id})
		if err != nil || view.Reviewed == nil {
			t.Fatalf("最终候选 identity=%v", err)
		}
		identity := *view.Reviewed
		view.Reviewed.Hash = "污染副本"
		if view.ValidateFor(workspaceevents.QueryDefinitionChangeCommand{WorkspaceID: id}) == nil {
			t.Fatal("detached reviewed hash 未反证")
		}
		for name, mutate := range map[string]func(*decisionmodel.ContractChangeProposal){
			"self_hash":          func(p *decisionmodel.ContractChangeProposal) { p.FinalCandidate.Hash = "self-declared" },
			"wrong_id":           func(p *decisionmodel.ContractChangeProposal) { p.FinalCandidate.ID = "other-candidate" },
			"wrong_revision":     func(p *decisionmodel.ContractChangeProposal) { p.FinalCandidate.Revision++ },
			"changed_reason":     func(p *decisionmodel.ContractChangeProposal) { p.WhyChange = "未重新审查的新原因" },
			"changed_difference": func(p *decisionmodel.ContractChangeProposal) { p.Differences[0].ProposedText = "未审新目标" },
		} {
			forged := proposal.Clone()
			forged.FinalCandidate = &decisionmodel.ContractChangeCandidate{ID: identity.ID, Revision: identity.Revision, Hash: identity.Hash}
			mutate(&forged)
			if _, err := app.RegisterContractChangeProposal(ctx, decisionevents.RegisterContractChangeProposalCommand{TicketID: registration.TicketID, RequestID: "forged-final-" + name, ExpectedRevision: registration.Revision, ExpectedHash: registration.Hash, Proposal: forged}); err == nil {
				t.Fatalf("错误最终材料登记=%s", name)
			}
		}
		unchanged, err := repos.Tickets.Get(common.ContextWithRuntimeWorkspaceID(ctx, id), registration.TicketID)
		if err != nil || len(unchanged.ContractChangeRevisions) != 1 {
			t.Fatalf("最终登记拒绝仍追加版本=%v", err)
		}
		hub.loseRegistration = true
		if _, err := app.RegisterReviewedDefinitionChange(ctx, id, "final-register"); err == nil {
			t.Fatal("登记实际写后丢响应未模拟")
		}
		hub.loseRegistration = false
		final, err := app.RegisterReviewedDefinitionChange(ctx, id, "final-register")
		if err != nil || final.Disposition != "duplicate" || final.Revision != registration.Revision+1 {
			t.Fatalf("最终登记=%v", err)
		}
		duplicate, err := freshApp.RegisterReviewedDefinitionChange(ctx, id, "final-register")
		if err != nil || duplicate.Disposition != "duplicate" || duplicate.Revision != final.Revision || duplicate.Hash != final.Hash {
			t.Fatalf("最终登记重放=%v", err)
		}
		current, err := app.QueryDefinitionChange(ctx, workspaceevents.QueryDefinitionChangeCommand{WorkspaceID: id})
		if err != nil || current.Reviewed == nil || *current.Reviewed != identity || calls != 16 {
			t.Fatalf("登记后候选 identity 漂移=%v", err)
		}
		ticket, err := repos.Tickets.Get(common.ContextWithRuntimeWorkspaceID(ctx, id), registration.TicketID)
		if err != nil || len(ticket.ContractChangeRevisions) != 2 || ticket.ContractChangeRevisions[1].Proposal.FinalCandidate.Hash != identity.Hash {
			t.Fatalf("原版本 append=%v", err)
		}
		if resolution, err := repos.Resolutions.GetByTicketID(common.ContextWithRuntimeWorkspaceID(ctx, id), registration.TicketID); resolution != nil || !errors.Is(err, common.ErrNotFound) {
			t.Fatalf("登记偷生批准=%v", err)
		}
		persisted, err := repos.Workspaces.Get(ctx, id)
		if err != nil || !reflect.DeepEqual(after, persisted) {
			t.Fatalf("最终登记改写 Workspace=%v", err)
		}
	}
}
