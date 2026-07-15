package common

import (
	"context"
	"strings"
)

type runtimeWorkspaceIDContextKey struct{}

func ContextWithRuntimeWorkspaceID(ctx context.Context, workspaceID WorkspaceID) context.Context {
	workspaceID = WorkspaceID(strings.TrimSpace(string(workspaceID)))
	if workspaceID == "" {
		return ctx
	}
	return context.WithValue(ctx, runtimeWorkspaceIDContextKey{}, workspaceID)
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
