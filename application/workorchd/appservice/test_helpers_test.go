package appservice

import (
	"context"
	"testing"
)

func newTestRuntimeSession(t *testing.T, backend string, dataRoot string) (*RuntimeSession, context.Context) {
	t.Helper()

	ctx := context.Background()
	session, err := NewRuntimeSessionWithBackend(backend, dataRoot)
	if err != nil {
		t.Fatalf("new runtime session: %v", err)
	}
	t.Cleanup(func() {
		if err := session.Close(ctx); err != nil {
			t.Errorf("close runtime session: %v", err)
		}
	})
	return session, ctx
}

func newWorkorchTestService(t *testing.T) (*WorkorchService, Runtime) {
	t.Helper()

	session, _ := newTestRuntimeSession(t, "memory", "")
	return NewWorkorchService(session.Runtime), session.Runtime
}
