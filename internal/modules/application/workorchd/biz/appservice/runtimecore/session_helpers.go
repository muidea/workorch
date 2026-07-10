package runtimecore

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
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
type commandHubContextKey struct{}

func ContextWithEventHubs(ctx context.Context, eventHub events.Hub, commandHub magicevent.Hub) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if eventHub != nil {
		ctx = context.WithValue(ctx, eventHubContextKey{}, eventHub)
	}
	if commandHub == nil {
		return ctx
	}
	return context.WithValue(ctx, commandHubContextKey{}, commandHub)
}

func EventHubFromContext(ctx context.Context) events.Hub {
	if ctx == nil {
		return nil
	}
	eventHub, _ := ctx.Value(eventHubContextKey{}).(events.Hub)
	return eventHub
}

func CommandHubFromContext(ctx context.Context) magicevent.Hub {
	if ctx == nil {
		return nil
	}
	commandHub, _ := ctx.Value(commandHubContextKey{}).(magicevent.Hub)
	return commandHub
}
