package appservice

import (
	"context"
	"testing"
)

func newTestRuntimeSession(t *testing.T, backend string, dataRoot string) (*RuntimeSession, context.Context) {
	t.Helper()

	ctx := context.Background()
	session := NewRuntimeSessionWithBackend(backend, dataRoot)
	t.Cleanup(func() {
		if err := session.Close(ctx); err != nil {
			t.Errorf("close runtime session: %v", err)
		}
	})
	return session, ctx
}

func newPhase1TestService(t *testing.T) (*Phase1Service, Runtime) {
	t.Helper()

	session, _ := newTestRuntimeSession(t, "memory", "")
	return NewPhase1Service(session.Runtime), session.Runtime
}
