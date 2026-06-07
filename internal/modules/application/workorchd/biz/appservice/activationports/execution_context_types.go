package activationports

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type WorkspaceExecutionContextRequest struct {
	Hub                            events.Hub
	Workspace                      *workspacemodel.Workspace
	Goal                           *workspacemodel.Goal
	DoD                            *workspacemodel.DoD
	Command                        appmodelcore.ActivateWorkspaceExecutionCommand
	SystemDefaultSoulID            common.SoulID
	DefaultDialogLanguage          string
	DefaultEmbeddingProviderLoader DefaultEmbeddingProviderLoader
	SkillHubCatalog                contextregistrycore.SkillHubSkillCatalog
}

type WorkspaceExecutionContextByIDRequest struct {
	Hub                            events.Hub
	WorkspaceID                    common.WorkspaceID
	GoalID                         common.GoalID
	DoDID                          common.DoDID
	Command                        appmodelcore.ActivateWorkspaceExecutionCommand
	SystemDefaultSoulID            common.SoulID
	DefaultDialogLanguage          string
	DefaultEmbeddingProviderLoader DefaultEmbeddingProviderLoader
	SkillHubCatalog                contextregistrycore.SkillHubSkillCatalog
}
