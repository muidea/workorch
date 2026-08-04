package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/configcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// CompileWorkspaceExecutionRestartBootstrapWithSession is the intent-only
// restart compiler. It compiles an immutable bootstrap and deliberately
// performs no appmodelcore.Workspace/appmodelcore.Run lifecycle transition.
func CompileWorkspaceExecutionRestartBootstrapWithSession(ctx context.Context, session *runtimecore.Session, workspaceID string, cmd appmodelcore.ActivateWorkspaceExecutionCommand) (ret *runtimecontract.RuntimeBootstrap, err error) {
	if session == nil {
		return nil, common.ErrInvalidArgument
	}
	ctx = configcore.ContextWithLoader(ctx, session.ConfigLoader())
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (*runtimecontract.RuntimeBootstrap, error) {
		defaults, err := workspaceExecutionDefaultsFromContext(ctx, hub)
		if err != nil {
			return nil, err
		}
		return activationports.CompileWorkspaceExecutionRestartBootstrap(ctx, activationports.ActivateWorkspaceExecutionViewRequest{
			Hub:                            hub,
			WorkspaceID:                    workspaceID,
			Command:                        cmd,
			SystemDefaultSoulID:            defaults.systemDefaultSoulID,
			DefaultDialogLanguage:          defaults.defaultDialogLanguage,
			DefaultEmbeddingProviderLoader: defaults.defaultEmbeddingProviderLoader,
			SkillHubCatalog:                defaults.skillHubCatalog,
		})
	})
}
