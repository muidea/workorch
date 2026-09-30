package biz

import (
	"context"
	"reflect"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceports"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func (b *Biz) SaveDefinitionAuthoringView(ctx context.Context, id common.WorkspaceID, request workorchmanagementcontract.DefinitionAuthoringSave) (workorchmanagementcontract.DefinitionAuthoringView, error) {
	view, err := b.QueryDefinitionAuthoring(ctx, workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: id})
	if err != nil {
		return workorchmanagementcontract.DefinitionAuthoringView{}, err
	}
	if view.WorkspaceRevision != request.ExpectedWorkspaceRevision && view.Record == nil {
		return workorchmanagementcontract.DefinitionAuthoringView{}, common.ErrConflict
	}
	input := workspace.DefinitionAuthoringInput{
		Version: workspace.DefinitionAuthoringInputVersionCurrent, WorkspaceID: id,
		Revision: 1, GoalText: request.Input.GoalText,
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
		input.TemplateID = view.Record.Input.TemplateID
		if request.TemplateID != "" {
			input.TemplateID = strings.TrimSpace(request.TemplateID)
		}
		if request.ExpectedInputHash != view.Record.InputHash {
			return workorchmanagementcontract.DefinitionAuthoringView{}, common.ErrConflict
		}
		input.Revision = view.Record.Input.Revision
		if reflect.DeepEqual(input.Normalized(), view.Record.Input.Normalized()) && (directory == nil || *directory == view.DeliverableDirectory) && view.CurrentTemplateAvailable && view.CurrentTemplateSnapshotHash == view.Record.TemplateSnapshotHash {
			// Retry after a lost save response returns the exact owner record.
			return b.ReadDefinitionAuthoringView(ctx, id)
		}
		if view.WorkspaceRevision != request.ExpectedWorkspaceRevision {
			return workorchmanagementcontract.DefinitionAuthoringView{}, common.ErrConflict
		}
		input.Revision++
	} else {
		if request.ExpectedInputHash != "" {
			return workorchmanagementcontract.DefinitionAuthoringView{}, common.ErrConflict
		}
		// The owner record has no Template selection until the Workspace is
		// created with its template. A blank Template fails closed in Save.
		workspaceRecord, readErr := workspaceports.ReadWorkspaceRecord(ctx, b.EventHub(), id)
		if readErr != nil {
			return workorchmanagementcontract.DefinitionAuthoringView{}, readErr
		}
		input.TemplateID = workspaceRecord.TemplateID
		if request.TemplateID != "" {
			input.TemplateID = strings.TrimSpace(request.TemplateID)
		}
	}
	if input.Validate() != nil || input.Revision == 0 {
		return workorchmanagementcontract.DefinitionAuthoringView{}, common.ErrInvalidArgument
	}
	if _, err := b.SaveDefinitionAuthoring(ctx, workspaceevents.SaveDefinitionAuthoringCommand{Input: input, ExpectedWorkspaceRevision: request.ExpectedWorkspaceRevision, DeliverableDirectory: directory}); err != nil {
		return workorchmanagementcontract.DefinitionAuthoringView{}, err
	}
	return b.ReadDefinitionAuthoringView(ctx, id)
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
