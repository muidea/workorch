package workorchport

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// ReadWorkspaceRuntimeViewRequest is the port-level RuntimeView read request.
type ReadWorkspaceRuntimeViewRequest struct {
	WorkspaceID   common.WorkspaceID
	ActivityLimit int
	BeforeCursor  string
}

// SubscriptionState describes EventHub/SSE subscription health.
type SubscriptionState string

const (
	SubscriptionConnecting   SubscriptionState = "connecting"
	SubscriptionLive         SubscriptionState = "live"
	SubscriptionReconnecting SubscriptionState = "reconnecting"
	SubscriptionDegraded     SubscriptionState = "degraded"
)

// SubscriptionStatus is process-local EventHub/SSE transport health. It is not
// a workspace projection and cannot drive owner state.
type SubscriptionStatus struct {
	State            SubscriptionState
	StreamGeneration string
	StreamRevision   int64
	LastEventAt      string
	LastError        string
}

// WorkspaceRuntimeViewReader is the stable RuntimeView read surface for TUI/CLI.
// Implemented by WorkspaceBackend.
type WorkspaceRuntimeViewReader interface {
	RuntimeView(ctx context.Context, req ReadWorkspaceRuntimeViewRequest) (*projectioncontract.WorkspaceRuntimeView, error)
}

// WorkspaceProjectionChangedHandler receives typed projection invalidation notices.
// TUI must not receive generic Event payloads on this path.
type WorkspaceProjectionChangedHandler func(ctx context.Context, info projectioncontract.WorkspaceProjectionChanged)

// SubscriptionStatusHandler receives typed EventHub/SSE health updates.
type SubscriptionStatusHandler func(ctx context.Context, status SubscriptionStatus)

// WorkspaceProjectionEventsBackend is the TUI projection subscription surface.
// It is intentionally separate from the generic EventsBackend so raw owner
// event streams cannot re-enter the TUI main view.
type WorkspaceProjectionEventsBackend interface {
	SubscribeWorkspaceProjection(
		ctx context.Context,
		onChanged WorkspaceProjectionChangedHandler,
		onStatus SubscriptionStatusHandler,
	) (unsubscribe func(), err error)
}
