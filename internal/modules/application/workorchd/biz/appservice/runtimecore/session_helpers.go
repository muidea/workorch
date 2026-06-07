package runtimecore

import (
	"context"
	"strings"

	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
)

type RepositoryConfig struct {
	Backend string
	DataDir string
}

func ResolveRepositoryConfig(backend string, dataRoot string) RepositoryConfig {
	ret := RepositoryConfig{
		Backend: strings.TrimSpace(backend),
	}
	if ret.Backend == "" {
		ret.Backend = "memory"
	}
	ret.DataDir = workorchrepo.ResolveRepositoryDataRoot(ret.Backend, strings.TrimSpace(dataRoot))
	return ret
}

type eventHubContextKey struct{}

func ContextWithEventHub(ctx context.Context, eventHub events.Hub) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if eventHub == nil {
		return ctx
	}
	return context.WithValue(ctx, eventHubContextKey{}, eventHub)
}

func EventHubFromContext(ctx context.Context) events.Hub {
	if ctx == nil {
		return nil
	}
	eventHub, _ := ctx.Value(eventHubContextKey{}).(events.Hub)
	return eventHub
}
