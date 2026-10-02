package biz

import (
	"context"
	"reflect"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"

	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func (b *Biz) SaveDefinitionAuthoringView(ctx context.Context, id common.WorkspaceID, request workorchmanagementcontract.DefinitionAuthoringSave) (workorchmanagementcontract.DefinitionAuthoringView, error) {
	if strings.TrimSpace(request.TemplateID) == "" {
		return workorchmanagementcontract.DefinitionAuthoringView{}, common.ErrInvalidArgument
	}
	view, err := b.QueryDefinitionAuthoring(ctx, workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: id})
	if err != nil {
		return workorchmanagementcontract.DefinitionAuthoringView{}, err
	}
	if view.WorkspaceRevision != request.ExpectedWorkspaceRevision && view.Record == nil {
		return workorchmanagementcontract.DefinitionAuthoringView{}, common.ErrConflict
	}
	input := workspace.DefinitionAuthoringInput{
		Version: workspace.DefinitionAuthoringInputVersionCurrent, WorkspaceID: id,
		Revision: 1, TemplateID: strings.TrimSpace(request.TemplateID), GoalText: request.Input.GoalText,
		AcceptanceRequirements: request.Input.AcceptanceRequirements, ResourceRefs: request.Input.ResourceRefs,
	}
	var directory *string
	if request.DeliverableDirectory != nil {
		value := strings.TrimSpace(*request.DeliverableDirectory)
		if value == "" {
			return workorchmanagementcontract.DefinitionAuthoringView{}, common.ErrInvalidArgument
		}
		layout, err := workspacecore.ResolveDirectoryLayout("memory", "", string(id), value)
		if err != nil {
			return workorchmanagementcontract.DefinitionAuthoringView{}, err
		}
		value = layout.DeliverableDirectory
		directory = &value
	}
	if view.Record != nil {
		input.Revision = view.Record.Input.Revision
		if !reflect.DeepEqual(input.Normalized(), view.Record.Input.Normalized()) || (directory != nil && *directory != view.DeliverableDirectory) || !view.CurrentTemplateAvailable || view.CurrentTemplateSnapshotHash != view.Record.TemplateSnapshotHash {
			input.Revision++
		}
	} else {
		if request.ExpectedInputHash != "" {
			return workorchmanagementcontract.DefinitionAuthoringView{}, common.ErrConflict
		}

	}
	if input.Validate() != nil || input.Revision == 0 {
		return workorchmanagementcontract.DefinitionAuthoringView{}, common.ErrInvalidArgument
	}
	saved, err := b.SaveDefinitionAuthoring(ctx, workspaceevents.SaveDefinitionAuthoringCommand{RequestID: request.RequestID, ExpectedInputHash: request.ExpectedInputHash, Input: input, ExpectedWorkspaceRevision: request.ExpectedWorkspaceRevision, DeliverableDirectory: directory})
	if err != nil {
		return workorchmanagementcontract.DefinitionAuthoringView{}, err
	}
	// 原回执可供 owner 重放，但不能将被后续修订替代的保存冒充当前成功。
	if saved.Receipt.InputHash != saved.Record.InputHash || saved.Receipt.TemplateSnapshotHash != saved.Record.TemplateSnapshotHash {
		return workorchmanagementcontract.DefinitionAuthoringView{}, common.ErrConflict
	}
	current, err := b.QueryDefinitionAuthoring(ctx, workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: id})
	if err != nil {
		return workorchmanagementcontract.DefinitionAuthoringView{}, err
	}
	if current.Record == nil || current.Record.InputHash != saved.Receipt.InputHash || current.Record.TemplateSnapshotHash != saved.Receipt.TemplateSnapshotHash || current.WorkspaceRevision < saved.Receipt.WorkspaceRevision {
		return workorchmanagementcontract.DefinitionAuthoringView{}, common.ErrConflict
	}
	return definitionAuthoringView(current)
}

func (b *Biz) DeriveDefinitionAuthoringView(ctx context.Context, id common.WorkspaceID) (workorchmanagementcontract.DefinitionAuthoringView, error) {
	if _, err := b.DeriveDefinitionAcceptance(ctx, id); err != nil {
		return workorchmanagementcontract.DefinitionAuthoringView{}, err
	}
	return b.ReadDefinitionAuthoringView(ctx, id)
}

func (b *Biz) CompileDefinitionAuthoringView(ctx context.Context, id common.WorkspaceID) (workorchmanagementcontract.DefinitionAuthoringView, error) {
	if _, err := b.CompileAuthoringSpecification(ctx, id); err != nil {
		return workorchmanagementcontract.DefinitionAuthoringView{}, err
	}
	return b.ReadDefinitionAuthoringView(ctx, id)
}

func (b *Biz) ConfirmDefinitionAuthoringView(ctx context.Context, id common.WorkspaceID, request workorchmanagementcontract.DefinitionAuthoringConfirm) (workorchmanagementcontract.DefinitionAuthoringView, error) {
	command := workspaceevents.ConfirmDefinitionAuthoringCommand{
		WorkspaceID: id, InputHash: request.InputHash, DerivationPermitID: request.DerivationPermitID,
		ProposalHash: request.ProposalHash, SpecificationHash: request.SpecificationHash,
		ExpectedWorkspaceRevision: request.ExpectedWorkspaceRevision,
	}
	if _, err := b.ConfirmAndAdmitDefinitionAuthoring(ctx, command); err != nil {
		return workorchmanagementcontract.DefinitionAuthoringView{}, err
	}
	return b.ReadDefinitionAuthoringView(ctx, id)
}
