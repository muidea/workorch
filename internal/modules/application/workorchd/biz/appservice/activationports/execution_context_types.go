package activationports

import (
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type WorkspaceRuntimeActivationRequest struct {
	Hub                            magicevent.Hub
	Workspace                      *workspacemodel.Workspace
	Goal                           *workspacemodel.Goal
	DoD                            *workspacemodel.DoD
	Command                        appmodelcore.ActivateWorkspaceExecutionCommand
	SystemDefaultSoulID            common.SoulID
	DefaultDialogLanguage          string
	DefaultEmbeddingProviderLoader DefaultEmbeddingProviderLoader
	SkillHubCatalog                contextregistrycore.SkillHubSkillCatalog
}

type WorkspaceRuntimeActivationByIDRequest struct {
	Hub                            magicevent.Hub
	WorkspaceID                    common.WorkspaceID
	GoalID                         common.GoalID
	DoDID                          common.DoDID
	Command                        appmodelcore.ActivateWorkspaceExecutionCommand
	SystemDefaultSoulID            common.SoulID
	DefaultDialogLanguage          string
	DefaultEmbeddingProviderLoader DefaultEmbeddingProviderLoader
	SkillHubCatalog                contextregistrycore.SkillHubSkillCatalog
}
