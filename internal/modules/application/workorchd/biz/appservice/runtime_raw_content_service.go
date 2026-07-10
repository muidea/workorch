package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contentcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contentports"
)

type RuntimeRawContent = contentcore.RuntimeRawContent

func ReadRuntimeRawContentWithSession(ctx context.Context, session *RuntimeSession, ref string) (ret RuntimeRawContent, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (RuntimeRawContent, error) {
		reader := contentports.NewReader(hub, session.RepositoryBackend(), session.RepositoryDataDir())
		return reader.ReadRuntimeRaw(ctx, ref)
	})
}

func ReadRuntimeRawContentWithBackend(ctx context.Context, backend string, dataRoot string, ref string) (RuntimeRawContent, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (RuntimeRawContent, error) {
		return ReadRuntimeRawContentWithSession(ctx, session, ref)
	})
}
