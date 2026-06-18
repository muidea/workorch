package appservice

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/muidea/workorch/internal/pkg/events"
)

var sharedBackendSessions sync.Map

func withSharedBackendSession[T any](ctx context.Context, backend string, dataRoot string, read func(context.Context, *RuntimeSession) (T, error)) (ret T, err error) {
	if !shouldUseSharedBackendSession(backend, dataRoot) {
		return withBackendSession(ctx, backend, dataRoot, read)
	}
	owner, err := sharedBackendSession(ctx, backend, dataRoot)
	if err != nil {
		return ret, err
	}
	borrowed := owner.Borrow()
	defer borrowed.CloseOnReturn(ctx, &err)
	return read(ctx, borrowed)
}

func sharedBackendSession(ctx context.Context, backend string, dataRoot string) (*RuntimeSession, error) {
	key := sharedBackendSessionKey(ctx, backend, dataRoot)
	if value, ok := sharedBackendSessions.Load(key); ok {
		if session, ok := value.(*RuntimeSession); ok && session != nil {
			return session, nil
		}
	}
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return nil, err
	}
	actual, loaded := sharedBackendSessions.LoadOrStore(key, session)
	if !loaded {
		return session, nil
	}
	_ = session.Close(context.Background())
	typed, _ := actual.(*RuntimeSession)
	if typed == nil {
		return nil, fmt.Errorf("shared backend session %q is invalid", key)
	}
	return typed, nil
}

func closeSharedBackendSession(ctx context.Context, backend string, dataRoot string) error {
	if !shouldUseSharedBackendSession(backend, dataRoot) {
		return nil
	}
	key := sharedBackendSessionKey(ctx, backend, dataRoot)
	value, ok := sharedBackendSessions.LoadAndDelete(key)
	if !ok {
		return nil
	}
	session, _ := value.(*RuntimeSession)
	if session == nil {
		return nil
	}
	return session.Close(ctx)
}

func CloseSharedBackendSession(ctx context.Context, backend string, dataRoot string) error {
	return closeSharedBackendSession(ctx, backend, dataRoot)
}

func shouldUseSharedBackendSession(backend string, dataRoot string) bool {
	return strings.EqualFold(strings.TrimSpace(backend), "file") && strings.TrimSpace(dataRoot) != ""
}

func sharedBackendSessionKey(ctx context.Context, backend string, dataRoot string) string {
	configDir := os.Getenv("CONFIG_PATH")
	if strings.TrimSpace(configDir) == "" {
		configDir = os.Getenv("WORKORCH_CONFIG_DIR")
	}
	return strings.Join([]string{
		strings.ToLower(strings.TrimSpace(backend)),
		strings.TrimSpace(dataRoot),
		strings.TrimSpace(configDir),
		eventHubIdentity(EventHubFromContext(ctx)),
	}, "|")
}

func eventHubIdentity(hub events.Hub) string {
	if hub == nil {
		return ""
	}
	return fmt.Sprintf("%T/%p", hub, hub)
}
