package activationports

import (
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type workspaceRuntimeActivationRequest struct {
	Hub                            magicevent.Hub
	Workspace                      *workspacemodel.Workspace
	Template                       *workspacemodel.WorkspaceTemplate
	Goal                           *workspacemodel.Goal
	DoD                            *workspacemodel.DoD
	SystemDefaultSoulID            common.SoulID
	DefaultDialogLanguage          string
	DefaultEmbeddingProviderLoader DefaultEmbeddingProviderLoader
	SkillHubCatalog                contextregistrycore.SkillHubSkillCatalog
}
