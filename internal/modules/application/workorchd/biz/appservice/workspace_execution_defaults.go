package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/configcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/localizationcore"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type workspaceExecutionDefaults struct {
	systemDefaultSoulID            common.SoulID
	defaultDialogLanguage          string
	defaultEmbeddingProviderLoader activationports.DefaultEmbeddingProviderLoader
	skillHubCatalog                contextregistrycore.SkillHubSkillCatalog
}

func workspaceExecutionDefaultsFromContext(ctx context.Context, hub magicevent.Hub) (workspaceExecutionDefaults, error) {
	cfg, err := configcore.LoadFromContext(ctx)
	if err != nil {
		return workspaceExecutionDefaults{}, err
	}
	return workspaceExecutionDefaults{
		systemDefaultSoulID:   common.SoulID(cfg.Runtime.DefaultSoulID),
		defaultDialogLanguage: localizationcore.DefaultDialogLanguageFromContext(ctx),
		defaultEmbeddingProviderLoader: func(context.Context) (runtimecontract.ProviderModelTarget, error) {
			// defaultEmbedding is optional. Empty means semantic recall is disabled and
			// memory selection must fall back to workspace list baseline.
			if cfg.LLM.DefaultEmbedding == nil {
				return runtimecontract.ProviderModelTarget{}, nil
			}
			return cfg.LLM.DefaultEmbedding.ToRuntimeTarget(), nil
		},
		skillHubCatalog: skillHubCatalogForEventHub(hub),
	}, nil
}
