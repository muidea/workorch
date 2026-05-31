package common

import (
	"context"
	"strings"
)

type runtimeLLMProviderIDContextKey struct{}
type runtimeWorkspaceIDContextKey struct{}

func ContextWithRuntimeLLMProviderID(ctx context.Context, providerID string) context.Context {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return ctx
	}
	return context.WithValue(ctx, runtimeLLMProviderIDContextKey{}, providerID)
}

func RuntimeLLMProviderIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	providerID, _ := ctx.Value(runtimeLLMProviderIDContextKey{}).(string)
	return strings.TrimSpace(providerID)
}

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
