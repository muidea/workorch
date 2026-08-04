package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistryports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func CreateSoulWithSession(ctx context.Context, session *runtimecore.Session, cmd workorchmanagementcontract.Soul) (ret workorchmanagementcontract.Soul, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (workorchmanagementcontract.Soul, error) {
		return contextregistryports.CreateSoul(ctx, hub, cmd)
	})
}

func CreateSoulWithBackend(ctx context.Context, backend string, dataRoot string, cmd workorchmanagementcontract.Soul) (workorchmanagementcontract.Soul, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (workorchmanagementcontract.Soul, error) {
		return CreateSoulWithSession(ctx, session, cmd)
	})
}

func UpdateSoulWithSession(ctx context.Context, session *runtimecore.Session, soulID string, cmd workorchmanagementcontract.Soul) (ret workorchmanagementcontract.Soul, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (workorchmanagementcontract.Soul, error) {
		return contextregistryports.UpdateSoulByID(ctx, hub, soulID, cmd)
	})
}

func UpdateSoulWithBackend(ctx context.Context, backend string, dataRoot string, soulID string, cmd workorchmanagementcontract.Soul) (workorchmanagementcontract.Soul, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (workorchmanagementcontract.Soul, error) {
		return UpdateSoulWithSession(ctx, session, soulID, cmd)
	})
}

func DeleteSoulWithSession(ctx context.Context, session *runtimecore.Session, soulID string) (err error) {
	_, err = withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (struct{}, error) {
		return struct{}{}, contextregistryports.DeleteSoul(ctx, hub, soulID)
	})
	return err
}

func DeleteSoulWithBackend(ctx context.Context, backend string, dataRoot string, soulID string) error {
	_, err := withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (struct{}, error) {
		return struct{}{}, DeleteSoulWithSession(ctx, session, soulID)
	})
	return err
}
