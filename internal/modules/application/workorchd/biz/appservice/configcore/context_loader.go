package configcore

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

type loaderContextKey struct{}

func ContextWithLoader(ctx context.Context, loader func() (workorchconfig.Config, error)) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if loader == nil {
		return ctx
	}
	return context.WithValue(ctx, loaderContextKey{}, loader)
}

func LoaderFromContext(ctx context.Context) func() (workorchconfig.Config, error) {
	if ctx == nil {
		return nil
	}
	loader, _ := ctx.Value(loaderContextKey{}).(func() (workorchconfig.Config, error))
	return loader
}

func LoadFromContext(ctx context.Context) (workorchconfig.Config, error) {
	loader := LoaderFromContext(ctx)
	if loader == nil {
		return workorchconfig.Config{}, nil
	}
	return loader()
}
