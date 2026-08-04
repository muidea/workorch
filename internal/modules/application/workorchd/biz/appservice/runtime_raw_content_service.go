package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contentcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contentports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
)

func ReadRuntimeRawContentWithSession(ctx context.Context, session *runtimecore.Session, ref string) (ret contentcore.RuntimeRawContent, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (contentcore.RuntimeRawContent, error) {
		reader := contentports.NewReader(hub, session.RepositoryBackend(), session.RepositoryDataDir())
		return reader.ReadRuntimeRaw(ctx, ref)
	})
}

func ReadRuntimeRawContentWithBackend(ctx context.Context, backend string, dataRoot string, ref string) (contentcore.RuntimeRawContent, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (contentcore.RuntimeRawContent, error) {
		return ReadRuntimeRawContentWithSession(ctx, session, ref)
	})
}
