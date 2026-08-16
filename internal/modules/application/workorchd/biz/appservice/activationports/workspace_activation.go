package activationports

import (
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/pkg/common"
)

type ActivateWorkspaceExecutionViewRequest struct {
	Hub                            magicevent.Hub
	WorkspaceID                    string
	SystemDefaultSoulID            common.SoulID
	DefaultDialogLanguage          string
	DefaultEmbeddingProviderLoader DefaultEmbeddingProviderLoader
	SkillHubCatalog                contextregistrycore.SkillHubSkillCatalog
}
