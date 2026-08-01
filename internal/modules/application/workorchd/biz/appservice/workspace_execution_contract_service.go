package appservice

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceports"
	"github.com/muidea/workorch/internal/pkg/common"
)

// UpdateWorkspaceExecutionContractWithHub persists a declaration owned by one
// workspace. It does not activate, resume, restart, or otherwise mutate a
// Run; Runtime V2 can observe the new declaration only in a later explicit
// restart bootstrap.
func UpdateWorkspaceExecutionContractWithHub(ctx context.Context, hub magicevent.Hub, workspaceID string, declaration appmodelcore.WorkspaceExecutionContractDeclaration) (Workspace, error) {
	id := common.WorkspaceID(strings.TrimSpace(workspaceID))
	if hub == nil || id == "" {
		return Workspace{}, common.ErrInvalidArgument
	}
	item, err := workspaceports.UpdateWorkspaceExecutionContract(ctx, hub, id, declaration.ContextHints, declaration.EvidencePolicies)
	if err != nil {
		return Workspace{}, err
	}
	return workspacecore.MapWorkspace(item), nil
}

func UpdateWorkspaceExecutionContractWithSession(ctx context.Context, session *RuntimeSession, workspaceID string, declaration appmodelcore.WorkspaceExecutionContractDeclaration) (Workspace, error) {
	if session == nil {
		return Workspace{}, common.ErrInvalidArgument
	}
	hub, err := session.RequiredCommandHub()
	if err != nil {
		return Workspace{}, err
	}
	return UpdateWorkspaceExecutionContractWithHub(ctx, hub, workspaceID, declaration)
}

func UpdateWorkspaceExecutionContractWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string, declaration appmodelcore.WorkspaceExecutionContractDeclaration) (Workspace, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (Workspace, error) {
		return UpdateWorkspaceExecutionContractWithSession(ctx, session, workspaceID, declaration)
	})
}
