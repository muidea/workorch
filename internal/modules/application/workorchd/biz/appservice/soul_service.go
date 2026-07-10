package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistryports"
)

func CreateSoulWithSession(ctx context.Context, session *RuntimeSession, cmd CreateSoulCommand) (ret Soul, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (Soul, error) {
		return contextregistryports.CreateSoul(ctx, hub, cmd)
	})
}

func CreateSoulWithBackend(ctx context.Context, backend string, dataRoot string, cmd CreateSoulCommand) (Soul, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (Soul, error) {
		return CreateSoulWithSession(ctx, session, cmd)
	})
}

func UpdateSoulWithSession(ctx context.Context, session *RuntimeSession, soulID string, cmd UpdateSoulCommand) (ret Soul, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (Soul, error) {
		return contextregistryports.UpdateSoulByID(ctx, hub, soulID, cmd)
	})
}

func UpdateSoulWithBackend(ctx context.Context, backend string, dataRoot string, soulID string, cmd UpdateSoulCommand) (Soul, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (Soul, error) {
		return UpdateSoulWithSession(ctx, session, soulID, cmd)
	})
}

func DeleteSoulWithSession(ctx context.Context, session *RuntimeSession, soulID string) (err error) {
	_, err = withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (struct{}, error) {
		return struct{}{}, contextregistryports.DeleteSoul(ctx, hub, soulID)
	})
	return err
}

func DeleteSoulWithBackend(ctx context.Context, backend string, dataRoot string, soulID string) error {
	_, err := withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (struct{}, error) {
		return struct{}{}, DeleteSoulWithSession(ctx, session, soulID)
	})
	return err
}
