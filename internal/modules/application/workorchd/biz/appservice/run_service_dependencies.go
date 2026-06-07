package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/configcore"
)

func runCompressedContextDependencies(ctx context.Context, session *RuntimeSession) (context.Context, readModelDependencies, error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return ctx, readModelDependencies{}, err
	}
	return configcore.ContextWithLoader(ctx, deps.loadConfig), deps, nil
}
