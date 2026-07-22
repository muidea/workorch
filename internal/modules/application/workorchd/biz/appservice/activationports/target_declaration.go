package activationports

import (
	"context"
	"fmt"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceports"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// ErrTargetDeclarationRequired distinguishes activation input that has no
// owner-grounded implementation surface from a Runtime V2 terminal. Callers
// can present this before creating a Run; it is never a decision ticket.
var ErrTargetDeclarationRequired = activationcore.ErrTargetDeclarationRequired

func materializeWorkspaceTargetDeclaration(ctx context.Context, request ActivateWorkspaceExecutionViewRequest, workspace *workspacemodel.Workspace) error {
	if request.Hub == nil || workspace == nil {
		return common.ErrInvalidArgument
	}
	if request.Command.TargetDeclaration == nil {
		return nil
	}
	value := request.Command.TargetDeclaration.Normalized()
	if err := value.Validate(); err != nil {
		return fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
	}
	if runtimecontract.SameRuntimeTargetDeclaration(workspace.TargetDeclaration, &value) {
		return nil
	}
	updated, err := workspaceports.UpdateWorkspaceTargetDeclaration(ctx, request.Hub, workspace.ID, &value)
	if err != nil {
		return err
	}
	workspace.TargetDeclaration = updated.TargetDeclaration
	return nil
}

func rejectActiveRunTargetDeclarationOverride(workspace *workspacemodel.Workspace, declaration *runtimecontract.RuntimeTargetDeclaration) error {
	if declaration == nil {
		return nil
	}
	if workspace == nil || !runtimecontract.SameRuntimeTargetDeclaration(workspace.TargetDeclaration, declaration) {
		return fmt.Errorf("%w: active Run target declaration is sealed; create a replacement Run after it reaches terminal", common.ErrConflict)
	}
	return nil
}
