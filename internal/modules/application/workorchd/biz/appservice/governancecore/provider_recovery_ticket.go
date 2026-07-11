package governancecore

import (
	"strings"

	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
)

func IsWorkorchRuntimeLLMProviderRecoveryTicket(ticket *decisionmodel.Ticket) bool {
	if ticket == nil {
		return false
	}
	scope := strings.TrimSpace(StringFromDecisionPayload(ticket.Context, decisionContextKeyScope))
	if scope == decisionContextScopeTargetWorkspace || scope == decisionContextScopeExternalDependency {
		return false
	}
	if scope == decisionContextScopeWorkorchRuntime {
		return IsLLMProviderRecoveryTicket(ticket)
	}
	// Older tickets did not carry a scope. Preserve provider recovery protection
	// for those records while keeping scoped target-workspace tickets separate.
	return IsLLMProviderRecoveryTicket(ticket)
}

func IsLLMProviderRecoveryTicket(ticket *decisionmodel.Ticket) bool {
	if ticket == nil {
		return false
	}
	if dependency := strings.TrimSpace(StringFromDecisionPayload(ticket.Context, decisionContextKeyDependency)); dependency != "" {
		return dependency == decisionContextDependencyLLMProvider
	}
	if policy := strings.TrimSpace(StringFromDecisionPayload(ticket.Context, decisionContextKeyRecoveryPolicy)); policy != "" {
		return policy == decisionContextRecoveryPolicyConfigOnly
	}
	class := strings.TrimSpace(StringFromDecisionPayload(ticket.Context, "failureClass"))
	if strings.HasPrefix(class, "provider_") ||
		class == "context_too_large" ||
		class == "rate_limited" ||
		class == "auth_or_config_error" {
		return true
	}
	text := strings.ToLower(ticket.Question + "\n" + ticket.Reason)
	return strings.Contains(text, "llm provider") || strings.Contains(text, "provider retry")
}
