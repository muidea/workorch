package common

import (
	"context"
	"fmt"
	"strings"
)

type runtimeWorkspaceIDContextKey struct{}

func ContextWithRuntimeWorkspaceID(ctx context.Context, workspaceID WorkspaceID) context.Context {
	if ctx == nil {
		return nil
	}
	workspaceID = WorkspaceID(strings.TrimSpace(string(workspaceID)))
	if workspaceID == "" {
		return ctx
	}
	// This convenience constructor may establish scope for an unscoped root context,
	// but must not silently replace the scope of an EventHub execution lane.
	// Workspace-scoped command boundaries use BindRuntimeWorkspaceContext so a
	// mismatch is reported as ErrConflict.
	if RuntimeWorkspaceIDFromContext(ctx) != "" {
		return ctx
	}
	return context.WithValue(ctx, runtimeWorkspaceIDContextKey{}, workspaceID)
}

// BindRuntimeWorkspaceContext establishes the workspace scope of one inbound
// command or event. A context may be completed when it has no scope yet, but
// it must never be silently rebound to a different workspace. This keeps a
// shared component instance safe when EventHub lanes for multiple workspaces
// are active concurrently.
//
// WorkspaceID remains part of typed commands as their durable, auditable
// identity. Context is the execution scope propagated to repositories,
// downstream EventHub calls and background work.
func BindRuntimeWorkspaceContext(ctx context.Context, workspaceID WorkspaceID) (context.Context, error) {
	if ctx == nil {
		return nil, fmt.Errorf("%w: runtime workspace context is required", ErrInvalidArgument)
	}
	workspaceID = WorkspaceID(strings.TrimSpace(string(workspaceID)))
	if workspaceID == "" {
		return nil, fmt.Errorf("%w: runtime workspace scope is required", ErrInvalidArgument)
	}
	if current := RuntimeWorkspaceIDFromContext(ctx); current != "" && current != workspaceID {
		return nil, fmt.Errorf("%w: runtime workspace context %q does not match command scope %q", ErrConflict, current, workspaceID)
	}
	return ContextWithRuntimeWorkspaceID(ctx, workspaceID), nil
}

func RuntimeWorkspaceIDFromContext(ctx context.Context) WorkspaceID {
	if ctx == nil {
		return ""
	}
	workspaceID, _ := ctx.Value(runtimeWorkspaceIDContextKey{}).(WorkspaceID)
	if workspaceID != "" {
		return WorkspaceID(strings.TrimSpace(string(workspaceID)))
	}
	workspaceIDString, _ := ctx.Value(runtimeWorkspaceIDContextKey{}).(string)
	return WorkspaceID(strings.TrimSpace(workspaceIDString))
}
