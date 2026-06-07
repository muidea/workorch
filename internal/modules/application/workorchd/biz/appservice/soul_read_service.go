package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistryports"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ReadSoulWithSession(ctx context.Context, session *RuntimeSession, soulID string) (ret Soul, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) (Soul, error) {
		return contextregistryports.ReadSoul(ctx, hub, soulID)
	})
}

func ReadSoulWithBackend(ctx context.Context, backend string, dataRoot string, soulID string) (Soul, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (Soul, error) {
		return ReadSoulWithSession(ctx, session, soulID)
	})
}

func ListSoulsWithSession(ctx context.Context, session *RuntimeSession) (ret []Soul, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) ([]Soul, error) {
		return contextregistryports.ListSouls(ctx, hub)
	})
}

func ListSoulsWithBackend(ctx context.Context, backend string, dataRoot string) ([]Soul, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) ([]Soul, error) {
		return ListSoulsWithSession(ctx, session)
	})
}
