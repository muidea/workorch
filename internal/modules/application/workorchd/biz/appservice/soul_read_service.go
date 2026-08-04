package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistryports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func ReadSoulWithSession(ctx context.Context, session *runtimecore.Session, soulID string) (ret workorchmanagementcontract.Soul, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (workorchmanagementcontract.Soul, error) {
		return contextregistryports.ReadSoul(ctx, hub, soulID)
	})
}

func ReadSoulWithBackend(ctx context.Context, backend string, dataRoot string, soulID string) (workorchmanagementcontract.Soul, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (workorchmanagementcontract.Soul, error) {
		return ReadSoulWithSession(ctx, session, soulID)
	})
}

func ListSoulsWithSession(ctx context.Context, session *runtimecore.Session) (ret []workorchmanagementcontract.Soul, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) ([]workorchmanagementcontract.Soul, error) {
		return contextregistryports.ListSouls(ctx, hub)
	})
}

func ListSoulsWithBackend(ctx context.Context, backend string, dataRoot string) ([]workorchmanagementcontract.Soul, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) ([]workorchmanagementcontract.Soul, error) {
		return ListSoulsWithSession(ctx, session)
	})
}
