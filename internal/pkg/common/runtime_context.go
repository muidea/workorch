package common

import (
	"context"
	"strings"
)

type runtimeLLMProviderIDContextKey struct{}

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
