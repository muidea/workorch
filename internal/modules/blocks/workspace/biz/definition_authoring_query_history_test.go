package workspace

import (
	"context"
	"testing"

	appbiz "github.com/muidea/workorch/internal/modules/application/workorchd/biz"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	workspaceeventmapping "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/eventmapping"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

func TestAuthoringQueryRejectsCorruptArchivedSource(t *testing.T) {
	ctx := context.Background()
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	command := seedReviewedAuthoringConfirmation(t, owners, "ws-query-history")
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	application := &appbiz.Biz{Base: basebiz.New(appbiz.ModuleID, owners.hub, nil)}
	if _, err := application.ConfirmAndAdmitDefinitionAuthoring(ctx, command); err != nil {
		t.Fatal(err)
	}
	reviewedAuthoringSuccessor(t, owners, command.WorkspaceID, false)
	query := workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: command.WorkspaceID}
	view, err := owners.workspace.queryDefinitionAuthoring(ctx, query)
	if err != nil || len(view.History) != 1 || view.Confirmation != nil {
		t.Fatalf("后继草稿缺少原确认历史: %v", err)
	}
	stored, err := owners.repositories.Workspaces.Get(ctx, command.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*workspace.DefinitionAuthoringConfirmedRevision){
		"ledger": func(r *workspace.DefinitionAuthoringConfirmedRevision) {
			r.Record.SpecificationRequests[1].PermitID = "forged"
		},
		"review": func(r *workspace.DefinitionAuthoringConfirmedRevision) {
			r.Record.SpecificationRequests[1].Review.Reason += " changed"
		},
		"formal_image": func(r *workspace.DefinitionAuthoringConfirmedRevision) { r.Confirmation.DoD.Items[0] += " changed" },
		"phase": func(r *workspace.DefinitionAuthoringConfirmedRevision) {
			r.Confirmation.Phase = workspace.DefinitionAuthoringConfirmationPrepared
		},
		"sequence": func(r *workspace.DefinitionAuthoringConfirmedRevision) { r.Confirmation.FormalSequence = 2 },
	} {
		t.Run(name, func(t *testing.T) {
			archived, err := view.History[0].Model()
			if err != nil {
				t.Fatal(err)
			}
			change(&archived)
			observation := view
			history := []workspace.DefinitionAuthoringConfirmedRevision{archived}
			observation.History = workspaceeventmapping.DefinitionAuthoringHistoryDTOs(history)
			if observation.ValidateFor(query) == nil {
				t.Fatal("查询只验证当前草稿而忽略坏历史")
			}
			aggregate := *stored
			aggregate.DefinitionAuthoringHistory = history
			if aggregate.ValidateDefinitionAuthoringHistory() == nil {
				t.Fatal("历史校验复用旧成功结果")
			}
			if _, _, err := aggregate.ValidateActivationAdmissionPersistence(); err == nil {
				t.Fatal("准入校验忽略坏历史")
			}
		})
	}
}
