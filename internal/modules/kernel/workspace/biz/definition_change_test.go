package workspace

import (
	"context"
	"errors"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	runbiz "github.com/muidea/workorch/internal/modules/blocks/run/biz"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
)

func definitionChangeProposalForTest(w *workspace.Workspace) decisionmodel.ContractChangeProposal {
	proposal := decisionmodel.ContractChangeProposal{
		SchemaVersion: 1, WorkspaceID: w.ID, BaseInputRevision: w.DefinitionAuthoring.Input.Revision, BaseInputHash: w.DefinitionAuthoring.InputHash,
		ProposedBy: "test-user", TriggerFacts: "原观测\r\n", Discovery: "发现过程", RootCauseOrUnknown: "未知", WhyChange: "完整原因", EvidenceGaps: "尚缺审查证据",
		Differences:  []decisionmodel.ContractChangeDifference{{ItemID: workspace.DefinitionAuthoringGoalItemRef, Kind: "goal", Action: "modify", OriginalText: w.DefinitionAuthoring.Input.GoalText, ProposedText: "新的待审目标", SemanticChange: "改变承诺待精确批准"}},
		Alternatives: "原合同继续", NoChangeConsequences: "保持原约束", UnaffectedContinuationConditions: "原授权范围",
		Impacts:            []decisionmodel.ContractChangeImpact{{SubjectID: "goal", Kind: "requirement", ProposedHandling: "recheck", Reason: "变更需要重审"}},
		EffectsAndReceipts: "未知效果保留", OutstandingCosts: "成本保留", ReservedBudget: "预留不清零", AdditionalCostDurationRisk: "未知", RequiredAuthorization: "未批准", DecisionScope: "合同持有人", ApplicationConditions: "六审与approve未完成", RecoveryRestrictions: "不能回滚现实效果",
	}
	if w.StartupInput != nil {
		proposal.BaseDefinitionRevision, proposal.BaseStartupInputHash = w.StartupInput.DefinitionRevision, w.StartupInput.StartupInputHash
	}
	return proposal
}

var errDefinitionChangeResponseLost = errors.New("candidate response lost after actual persistence")

type definitionChangeLostResponseRepository struct {
	repository.WorkspaceRepository
}

func (r *definitionChangeLostResponseRepository) CompareAndSwap(ctx context.Context, w *workspace.Workspace, revision uint64) error {
	if err := r.WorkspaceRepository.CompareAndSwap(ctx, w, revision); err != nil {
		return err
	}
	return errDefinitionChangeResponseLost
}

func TestDefinitionChangePreparationKeepsSealedDefinitionAndOriginalBudget(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	defer owners.close()
	confirmation := seedReviewedAuthoringConfirmation(t, owners, "change-sealed")
	if _, err := owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: confirmation}); err != nil {
		t.Fatal(err)
	}
	current, err := owners.repositories.Workspaces.Get(ctx, confirmation.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	admission := workspaceevents.AdmitDefinitionAuthoringCommand{Candidate: authoringAdmissionCandidateFromCurrentConfiguration(t, current), ExpectedWorkspaceRevision: current.Revision, ExpectedConfigurationHash: workspace.WorkspaceConfigurationHash(current)}
	if _, err := owners.workspace.submitDefinitionAuthoringAdmission(ctx, admission); err != nil {
		t.Fatal(err)
	}
	current, err = owners.repositories.Workspaces.Get(ctx, confirmation.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	runOwner := runbiz.New()
	if err := runOwner.Configure(runbiz.Dependencies{OwnerRepositories: owners.repositories.RunOwner, EventHub: owners.hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := runOwner.Stop(context.Background()); err != nil {
			t.Error(err)
		}
	}()
	proposal := definitionChangeProposalForTest(current)
	register := decisionevents.RegisterContractChangeProposalCommand{TicketID: "change-ticket", RequestID: "register", Proposal: proposal}
	response := owners.hub.Send(magicevent.NewEventWithContext(decisionevents.TopicRegisterContractChangeProposal, definitionSpecificationCommandSource, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, register))
	if response == nil || response.Error() != nil {
		t.Fatalf("登记失败=%v", response)
	}
	data, resultErr := response.Get()
	registration, ok := data.(decisionevents.RegisterContractChangeProposalResult)
	if resultErr != nil || !ok || registration.ValidateFor(register) != nil {
		t.Fatalf("登记回执=%v", resultErr)
	}
	cmd := workspaceevents.PrepareDefinitionChangeCommand{WorkspaceID: current.ID, RequestID: "prepare", ExpectedWorkspaceRevision: current.Revision, TicketID: registration.TicketID, ProposalRevision: registration.Revision, ProposalHash: registration.Hash}
	originalRepository := owners.workspace.workspaces
	owners.workspace.workspaces = &definitionChangeLostResponseRepository{WorkspaceRepository: originalRepository}
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: current.ID, DefinitionChange: &cmd}); !errors.Is(err, errDefinitionChangeResponseLost) {
		t.Fatalf("真实持久写后丢响应=%v", err)
	}
	owners.workspace.workspaces = originalRepository
	first, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: current.ID, DefinitionChange: &cmd})
	if err != nil || first.DefinitionChange == nil || first.DefinitionChange.ValidateFor(cmd) != nil {
		t.Fatalf("准备=%+v %v", first, err)
	}
	stored, err := owners.repositories.Workspaces.Get(ctx, current.ID)
	if err != nil || stored.DefinitionChangeCandidate == nil || stored.DefinitionChangeCandidate.Validate(stored) != nil {
		t.Fatalf("候选持久化=%v", err)
	}
	before := *current
	after := *stored
	after.Revision, after.DefinitionChangeCandidate = before.Revision, nil
	if !reflect.DeepEqual(before, after) {
		t.Fatal("准备候选改写原定义、seal、journal或配置")
	}
	candidate := stored.DefinitionChangeCandidate
	if !reflect.DeepEqual(candidate.Record.DerivationRequests, current.DefinitionAuthoring.DerivationRequests) || !reflect.DeepEqual(candidate.Record.SpecificationRequests, current.DefinitionAuthoring.SpecificationRequests) {
		t.Fatal("候选准备重置预算历史")
	}
	if _, _, err := candidate.Record.ConfirmableSpecification(confirmation.DerivationPermitID, confirmation.ProposalHash); err == nil {
		t.Fatal("旧六审用于新候选确认")
	}
	if _, present, err := stored.ValidateActivationAdmissionPersistence(); err != nil || !present {
		t.Fatalf("原封存合同被破坏=%v", err)
	}
	admission.ExpectedWorkspaceRevision = stored.Revision
	if _, err := owners.workspace.submitDefinitionAuthoringAdmission(ctx, admission); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("普通准入消费候选或改变基线=%v", err)
	}
	afterAdmission, err := owners.repositories.Workspaces.Get(ctx, current.ID)
	if err != nil || !reflect.DeepEqual(stored, afterAdmission) {
		t.Fatalf("拒绝普通准入仍修改 seal=%v", err)
	}
	reopened := workorchrepo.NewWorkspaceScopedFileRepositorySet(root)
	loaded, err := reopened.Workspaces.Get(ctx, current.ID)
	if err != nil || !reflect.DeepEqual(stored, loaded) {
		t.Fatalf("候选reopen漂移=%v", err)
	}
	fresh, err := New(owners.hub, nil, Dependencies{OwnerRepositories: reopened.WorkspaceOwner, EventHub: owners.hub})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := fresh.Teardown(context.Background()); err != nil {
			t.Error(err)
		}
	}()
	replayed, err := fresh.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: current.ID, DefinitionChange: &cmd})
	if err != nil || !reflect.DeepEqual(first.DefinitionChange, replayed.DefinitionChange) {
		t.Fatalf("重复准备=%v", err)
	}
	bad := cmd
	bad.RequestID = "another-request"
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: current.ID, DefinitionChange: &bad}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("覆盖候选=%v", err)
	}
	runs, err := reopened.Runs.List(common.ContextWithRuntimeWorkspaceID(ctx, current.ID))
	if err != nil || len(runs) != 0 {
		t.Fatalf("准备创建Run=%v", err)
	}
}

func TestDefinitionChangePreparationBlocksOrdinaryConfirmationOfOriginalDraft(t *testing.T) {
	ctx := context.Background()
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	confirmation := seedReviewedAuthoringConfirmation(t, owners, "change-reviewed-draft")
	current, err := owners.repositories.Workspaces.Get(ctx, confirmation.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	proposal := definitionChangeProposalForTest(current)
	cmd := workspaceevents.PrepareDefinitionChangeCommand{WorkspaceID: current.ID, RequestID: "prepare", ExpectedWorkspaceRevision: current.Revision, TicketID: "ticket", ProposalRevision: 1, ProposalHash: proposal.Hash()}
	transition, err := reduceDefinitionChangePreparation(definitionSpecificationObservation{workspace: current, template: &current.DefinitionAuthoring.TemplateSnapshot}, cmd, proposal)
	if err != nil || transition.Next == nil {
		t.Fatalf("已审原草稿准备=%v", err)
	}
	if err := owners.repositories.Workspaces.CompareAndSwap(ctx, transition.Next, current.Revision); err != nil {
		t.Fatal(err)
	}
	stored, err := owners.repositories.Workspaces.Get(ctx, current.ID)
	if err != nil {
		t.Fatal(err)
	}
	confirmation.ExpectedWorkspaceRevision = stored.Revision
	if _, err := owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: confirmation}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("普通确认改变待审基线=%v", err)
	}
	after, err := owners.repositories.Workspaces.Get(ctx, current.ID)
	if err != nil || !reflect.DeepEqual(stored, after) {
		t.Fatalf("拒绝普通确认仍写 journal=%v", err)
	}
	goals, err := owners.repositories.Goals.ListByWorkspaceID(ctx, current.ID)
	if err != nil || len(goals) != 0 {
		t.Fatalf("拒绝普通确认仍物化 Goal=%v", err)
	}
	dods, err := owners.repositories.DoDs.ListByWorkspaceID(ctx, current.ID)
	if err != nil || len(dods) != 0 {
		t.Fatalf("拒绝普通确认仍物化 DoD=%v", err)
	}
}

func TestDefinitionChangePreparationRejectsUnsettledOriginalPermit(t *testing.T) {
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	w, _ := seedPreparationWorkspace(t, owners, "change-unresolved")
	record, request, fresh, err := w.DefinitionAuthoring.ReserveDerivation("pending-original", "provider", nil)
	if err != nil || !fresh {
		t.Fatalf("原 permit=%v", err)
	}
	w.DefinitionAuthoring = &record
	proposal := definitionChangeProposalForTest(w)
	cmd := workspaceevents.PrepareDefinitionChangeCommand{WorkspaceID: w.ID, RequestID: "prepare", ExpectedWorkspaceRevision: w.Revision, TicketID: "ticket", ProposalRevision: 1, ProposalHash: proposal.Hash()}
	observation := definitionSpecificationObservation{workspace: w, template: &record.TemplateSnapshot}
	if _, err := reduceDefinitionChangePreparation(observation, cmd, proposal); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("原 unknown 被冻结=%v", err)
	}
	if !reflect.DeepEqual(w.DefinitionAuthoring, &record) {
		t.Fatal("拒绝准备改写原 unknown 或预算")
	}
	settled, err := record.CompleteDerivation(request.PermitID, nil, "provider_failed", nil)
	if err != nil {
		t.Fatal(err)
	}
	w.DefinitionAuthoring = &settled
	transition, err := reduceDefinitionChangePreparation(observation, cmd, proposal)
	if err != nil || transition.Next == nil || !reflect.DeepEqual(transition.Next.DefinitionChangeCandidate.Record.DerivationRequests, settled.DerivationRequests) {
		t.Fatalf("已结算原预算未保留=%v", err)
	}
}

func TestDefinitionChangeInputRejectsUnmappedOrNonexactDifferences(t *testing.T) {
	base := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "目标", TemplateID: "template", AcceptanceRequirements: []string{"验收甲", "验收乙"}}
	for _, d := range []decisionmodel.ContractChangeDifference{
		{ItemID: "goal", Kind: "goal", Action: "modify", OriginalText: "目标", ProposedText: "新目标"},
		{ItemID: workspace.DefinitionAuthoringGoalItemRef, Kind: "goal", Action: "modify", OriginalText: "错误原文", ProposedText: "新目标"},
		{ItemID: "derived-dod:template:000001", Kind: "criterion", Action: "modify", OriginalText: "模板要求", ProposedText: "弱化"},
		{ItemID: "constraint", Kind: "constraint", Action: "remove", OriginalText: "权限限制"},
	} {
		if _, err := definitionChangeInput(base, []decisionmodel.ContractChangeDifference{d}); !errors.Is(err, common.ErrConflict) {
			t.Fatalf("未映射差异%+v=%v", d, err)
		}
	}
	input, err := definitionChangeInput(base, []decisionmodel.ContractChangeDifference{
		{ItemID: "derived-dod:user:000002", Kind: "criterion", Action: "modify", OriginalText: "验收乙", ProposedText: "新验收乙\r\n"},
		{ItemID: "derived-dod:user:000001", Kind: "criterion", Action: "remove", OriginalText: "验收甲"},
		{ItemID: "derived-dod:user:000003", Kind: "criterion", Action: "add", ProposedText: "新增验收"},
	})
	if err != nil || input.Revision != 2 || !reflect.DeepEqual(input.AcceptanceRequirements, []string{"新验收乙\r\n", "新增验收"}) || !reflect.DeepEqual(base.AcceptanceRequirements, []string{"验收甲", "验收乙"}) {
		t.Fatalf("精确输入映射=%+v %v", input, err)
	}
}
