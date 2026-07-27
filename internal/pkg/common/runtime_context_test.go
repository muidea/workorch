package common

import (
	"context"
	"errors"
	"testing"
)

func TestBindRuntimeWorkspaceContext(t *testing.T) {
	t.Run("binds an unscoped context", func(t *testing.T) {
		ctx, err := BindRuntimeWorkspaceContext(context.Background(), "ws-a")
		if err != nil {
			t.Fatalf("BindRuntimeWorkspaceContext: %v", err)
		}
		if got := RuntimeWorkspaceIDFromContext(ctx); got != "ws-a" {
			t.Fatalf("workspace = %q", got)
		}
	})

	t.Run("preserves matching scope", func(t *testing.T) {
		ctx, err := BindRuntimeWorkspaceContext(ContextWithRuntimeWorkspaceID(context.Background(), "ws-a"), "ws-a")
		if err != nil || RuntimeWorkspaceIDFromContext(ctx) != "ws-a" {
			t.Fatalf("ctx=%v err=%v", RuntimeWorkspaceIDFromContext(ctx), err)
		}
	})

	t.Run("legacy constructor does not rebind an existing scope", func(t *testing.T) {
		ctx := ContextWithRuntimeWorkspaceID(context.Background(), "ws-a")
		ctx = ContextWithRuntimeWorkspaceID(ctx, "ws-b")
		if got := RuntimeWorkspaceIDFromContext(ctx); got != "ws-a" {
			t.Fatalf("workspace = %q, want ws-a", got)
		}
	})

	t.Run("rejects a conflicting scope", func(t *testing.T) {
		_, err := BindRuntimeWorkspaceContext(ContextWithRuntimeWorkspaceID(context.Background(), "ws-a"), "ws-b")
		if !errors.Is(err, ErrConflict) {
			t.Fatalf("err = %v, want conflict", err)
		}
	})
}
