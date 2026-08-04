package runtimecore

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
)

func ResolveRepositoryConfig(backend string, dataRoot string) workorchconfig.RepositoryConfig {
	ret := workorchconfig.RepositoryConfig{
		Backend: strings.TrimSpace(backend),
	}
	if ret.Backend == "" {
		ret.Backend = "memory"
	}
	ret.DataDir = workorchrepo.ResolveRepositoryDataRoot(ret.Backend, strings.TrimSpace(dataRoot))
	return ret
}

type commandHubContextKey struct{}

func ContextWithCommandHub(ctx context.Context, commandHub magicevent.Hub) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if commandHub == nil {
		return ctx
	}
	return context.WithValue(ctx, commandHubContextKey{}, commandHub)
}

func CommandHubFromContext(ctx context.Context) magicevent.Hub {
	if ctx == nil {
		return nil
	}
	commandHub, _ := ctx.Value(commandHubContextKey{}).(magicevent.Hub)
	return commandHub
}
