package workspace

import (
	workspacebiz "github.com/muidea/workorch/internal/modules/kernel/workspace/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/common"
	workspaceservice "github.com/muidea/workorch/internal/modules/kernel/workspace/service"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type GoalDoDManager = workspacebiz.GoalDoDManager
type CreateWorkspaceCommand = workspaceservice.CreateWorkspaceCommand
type Service = workspaceservice.Service

func NewGoalDoDManager(workspaces repository.WorkspaceRepository) *workspacebiz.GoalDoDManager {
	return workspacebiz.NewGoalDoDManager(workspaces)
}
