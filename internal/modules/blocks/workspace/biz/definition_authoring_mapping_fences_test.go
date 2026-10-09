package workspace

import (
	"context"
	"reflect"
	"testing"

	appbiz "github.com/muidea/workorch/internal/modules/application/workorchd/biz"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

// 同步伪造 raw、mapping、canonical 和审查身份，排除仅由结构/hash 漂移拒绝的假阳性。
func TestAuthoringReceivingOwnerFencesRecheckCurrentAndHistoryBeforeMutation(t *testing.T) {
	for _, scope := range []string{"current", "history"} {
		t.Run(scope, func(t *testing.T) {
			ctx, root := context.Background(), t.TempDir()
			owners := openFileDefinitionAdmissionOwners(t, root)
			defer func() { owners.close() }()
			confirmation := seedReviewedAuthoringConfirmationWithAdmissionBlocker(t, owners, "ws-mapping-fences")
			if scope == "history" {
				if err := owners.workspace.Start(); err != nil {
					t.Fatal(err)
				}
				application := &appbiz.Biz{Base: basebiz.New(appbiz.ModuleID, owners.hub, nil)}
				if _, err := application.ConfirmAndAdmitDefinitionAuthoring(ctx, confirmation); err != nil {
					t.Fatal(err)
				}
				reviewedAuthoringSuccessor(t, owners, confirmation.WorkspaceID, false)
			}
			current, err := owners.repositories.Workspaces.Get(ctx, confirmation.WorkspaceID)
			if err != nil {
				t.Fatal(err)
			}
			input := current.DefinitionAuthoring.Input.Clone()
			input.Revision++
			input.GoalText = "下一份真实修订目标"
			save := workspaceevents.SaveDefinitionAuthoringCommand{RequestID: "save-fence", Input: input, ExpectedInputHash: current.DefinitionAuthoring.InputHash, ExpectedWorkspaceRevision: current.Revision}
			reserve := workspaceevents.ReserveDefinitionDerivationCommand{WorkspaceID: current.ID, InputHash: current.DefinitionAuthoring.InputHash, RequestID: "derive-fence", ProviderBindingHash: "provider", ResourceSnapshotHash: workspace.DefinitionAuthoringResourcesHash(nil)}
			observation := definitionSpecificationObservation{workspace: current, template: &current.DefinitionAuthoring.TemplateSnapshot}
			for _, event := range []definitionSpecificationInternalEvent{{WorkspaceID: current.ID, Authoring: &save}, {WorkspaceID: current.ID, DeriveReserve: &reserve}} {
				transition, err := reduceDefinitionSpecificationInternalEvent(observation, event)
				if err != nil || transition.Next == nil {
					t.Fatalf("合法前像必须能够生成有效 CAS 后像：%v", err)
				}
			}
			record := current.DefinitionAuthoring
			if scope == "history" {
				if len(current.DefinitionAuthoringHistory) != 1 {
					t.Fatal("必须使用真实确认后继归档")
				}
				record = &current.DefinitionAuthoringHistory[0].Record
			}
			proposal := record.SpecificationRequests[0].Proposal.Clone()
			proposal.Verifications[0].ProfileID = "invented"
			resealAuthoringProposalClaimForTest(t, record, proposal)
			if err := record.Validate(); err != nil {
				t.Fatalf("反例必须到达独立封存 Profile 门禁：%v", err)
			}
			if err := owners.repositories.Workspaces.CompareAndSwap(ctx, current, current.Revision); err != nil {
				t.Fatal(err)
			}
			owners.close()
			owners = openFileDefinitionAdmissionOwners(t, root)
			before, err := owners.repositories.Workspaces.Get(ctx, current.ID)
			if err != nil {
				t.Fatal(err)
			}
			save.ExpectedWorkspaceRevision = before.Revision
			diagnostic := workspace.NewDefinitionAuthoringPreparationDiagnostic(workspace.DefinitionAuthoringPreparationDiagnostic{WorkspaceID: before.ID, InputHash: before.DefinitionAuthoring.InputHash, TemplateSnapshotHash: before.DefinitionAuthoring.TemplateSnapshotHash, ObservedWorkspaceRevision: before.Revision, Stage: workspace.DefinitionAuthoringPreparationDerivation, TargetRequestID: "derive-preparation-fence", PhaseCode: "render", ProviderBindingHash: "provider"}, []byte("保留原始准备失败\r\n"))
			prepare := workspaceevents.RecordPreparationDiagnosticCommand{Diagnostic: diagnostic}
			if save.Validate() != nil || reserve.Validate() != nil || prepare.Validate() != nil {
				t.Fatal("命令自身必须合法")
			}
			failing := &failPreparationDiagnosticCASRepository{WorkspaceRepository: owners.workspace.workspaces}
			owners.workspace.workspaces = failing
			observation = definitionSpecificationObservation{workspace: before, template: &before.DefinitionAuthoring.TemplateSnapshot}
			for _, event := range []definitionSpecificationInternalEvent{{WorkspaceID: before.ID, Authoring: &save}, {WorkspaceID: before.ID, DeriveReserve: &reserve}, {WorkspaceID: before.ID, PreparationDiagnostic: &prepare}} {
				if _, err := reduceDefinitionSpecificationInternalEvent(observation, event); err == nil {
					t.Fatal("私有 reducer 接受了协调伪造的持久来源")
				}
				if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, event); err == nil {
					t.Fatal("dispatcher 接受了协调伪造的持久来源")
				}
			}
			if _, _, err := replayDefinitionAuthoringSave(before, save); err == nil {
				t.Fatal("save 快速重放绕过来源反证")
			}
			if _, _, err := replayPreparationDiagnostic(before, prepare); err == nil {
				t.Fatal("preparation 快速重放绕过来源反证")
			}
			if _, err := reduceDefinitionAuthoring(observation, save); err == nil {
				t.Fatal("直接 save/archive reducer 绕过来源反证")
			}
			if _, err := reduceDefinitionDerivation(observation, definitionSpecificationInternalEvent{WorkspaceID: before.ID, DeriveReserve: &reserve}); err == nil {
				t.Fatal("直接 derivation reducer 绕过来源反证")
			}
			if _, err := owners.workspace.observeDefinitionAuthoring(ctx, before.ID); err == nil {
				t.Fatal("owner load 接受了伪造来源")
			}
			if _, err := owners.workspace.queryDefinitionAuthoring(ctx, workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: before.ID}); err == nil {
				t.Fatal("普通查询接受了伪造来源")
			}
			after, err := owners.repositories.Workspaces.Get(ctx, before.ID)
			if err != nil || failing.calls != 0 || !reflect.DeepEqual(before, after) {
				t.Fatalf("来源拒绝产生 CAS 或改变预算/原文：calls=%d %v", failing.calls, err)
			}
		})
	}
}
