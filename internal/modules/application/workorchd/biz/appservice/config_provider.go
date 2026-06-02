package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

type configLoaderContextKey struct{}

func configLoaderFromExports(exports bootstrap.Exports) func() (workorchconfig.Config, error) {
	if exports.Infrastructure.LoadConfig != nil {
		return exports.Infrastructure.LoadConfig
	}
	return func() (workorchconfig.Config, error) {
		return workorchconfig.Config{}, nil
	}
}

func contextWithConfigLoader(ctx context.Context, loader func() (workorchconfig.Config, error)) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if loader == nil {
		return ctx
	}
	return context.WithValue(ctx, configLoaderContextKey{}, loader)
}

func configLoaderFromContext(ctx context.Context) func() (workorchconfig.Config, error) {
	if ctx == nil {
		return nil
	}
	loader, _ := ctx.Value(configLoaderContextKey{}).(func() (workorchconfig.Config, error))
	return loader
}

func loadConfigFromContext(ctx context.Context) (workorchconfig.Config, error) {
	loader := configLoaderFromContext(ctx)
	if loader == nil {
		return workorchconfig.Config{}, nil
	}
	return loader()
}

func loadConfigFromExports(exports bootstrap.Exports) (workorchconfig.Config, error) {
	return configLoaderFromExports(exports)()
}
