package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/content"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
)

func ReadRuntimeRawContentWithSession(ctx context.Context, session *runtimecore.Session, ref string) (ret content.RuntimeRawContent, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (content.RuntimeRawContent, error) {
		reader := content.NewReader(hub)
		return reader.ReadRuntimeRaw(ctx, ref)
	})
}

func ReadRuntimeRawContentWithBackend(ctx context.Context, backend string, dataRoot string, ref string) (content.RuntimeRawContent, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (content.RuntimeRawContent, error) {
		return ReadRuntimeRawContentWithSession(ctx, session, ref)
	})
}
