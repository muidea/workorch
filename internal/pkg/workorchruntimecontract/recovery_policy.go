package workorchruntimecontract

import "strings"

const (
	SideEffectPolicyNone           = "none"
	SideEffectPolicyWorkspaceRead  = "workspace_read"
	SideEffectPolicyWorkspaceWrite = "workspace_write"
	SideEffectPolicyExternalRead   = "external_read"
	SideEffectPolicyExternalWrite  = "external_write"
	SideEffectPolicyUnknown        = "unknown"

	IdempotencyPolicyIdempotent        = "idempotent"
	IdempotencyPolicyDedupeKeyRequired = "dedupe_key_required"
	IdempotencyPolicyNonIdempotent     = "non_idempotent"
	IdempotencyPolicyUnknown           = "unknown"

	WatchdogRecoveryDisabled             = "disabled"
	WatchdogRecoveryFinalizeTerminalOnly = "finalize_terminal_only"
	WatchdogRecoveryRetryExternal        = "retry_external"
	WatchdogRecoveryManualOnly           = "manual_only"
)

type RuntimeRecoveryPolicy struct {
	SideEffectPolicy  string `json:"sideEffectPolicy,omitempty"`
	IdempotencyPolicy string `json:"idempotencyPolicy,omitempty"`
	WatchdogRecovery  string `json:"watchdogRecovery,omitempty"`
	IdempotencyKey    string `json:"idempotencyKey,omitempty"`
	TimeoutBehavior   string `json:"timeoutBehavior,omitempty"`
}

func RuntimeRecoveryPolicyFromInput(input map[string]any) RuntimeRecoveryPolicy {
	if input == nil {
		return RuntimeRecoveryPolicy{}.Normalized()
	}
	if value, ok := input["runtimeRecoveryPolicy"]; ok {
		return RuntimeRecoveryPolicyFromValue(value)
	}
	return RuntimeRecoveryPolicy{}.Normalized()
}

func RuntimeRecoveryPolicyFromValue(value any) RuntimeRecoveryPolicy {
	switch typed := value.(type) {
	case RuntimeRecoveryPolicy:
		return typed.Normalized()
	case *RuntimeRecoveryPolicy:
		if typed == nil {
			return RuntimeRecoveryPolicy{}.Normalized()
		}
		return typed.Normalized()
	case map[string]any:
		return RuntimeRecoveryPolicy{
			SideEffectPolicy:  stringFromPolicyMap(typed, "sideEffectPolicy"),
			IdempotencyPolicy: stringFromPolicyMap(typed, "idempotencyPolicy"),
			WatchdogRecovery:  stringFromPolicyMap(typed, "watchdogRecovery"),
			IdempotencyKey:    stringFromPolicyMap(typed, "idempotencyKey"),
			TimeoutBehavior:   stringFromPolicyMap(typed, "timeoutBehavior"),
		}.Normalized()
	case map[string]string:
		return RuntimeRecoveryPolicy{
			SideEffectPolicy:  typed["sideEffectPolicy"],
			IdempotencyPolicy: typed["idempotencyPolicy"],
			WatchdogRecovery:  typed["watchdogRecovery"],
			IdempotencyKey:    typed["idempotencyKey"],
			TimeoutBehavior:   typed["timeoutBehavior"],
		}.Normalized()
	default:
		return RuntimeRecoveryPolicy{}.Normalized()
	}
}

func (p RuntimeRecoveryPolicy) Normalized() RuntimeRecoveryPolicy {
	p.SideEffectPolicy = normalizePolicyValue(p.SideEffectPolicy, SideEffectPolicyUnknown, map[string]struct{}{
		SideEffectPolicyNone:           {},
		SideEffectPolicyWorkspaceRead:  {},
		SideEffectPolicyWorkspaceWrite: {},
		SideEffectPolicyExternalRead:   {},
		SideEffectPolicyExternalWrite:  {},
		SideEffectPolicyUnknown:        {},
	})
	p.IdempotencyPolicy = normalizePolicyValue(p.IdempotencyPolicy, IdempotencyPolicyUnknown, map[string]struct{}{
		IdempotencyPolicyIdempotent:        {},
		IdempotencyPolicyDedupeKeyRequired: {},
		IdempotencyPolicyNonIdempotent:     {},
		IdempotencyPolicyUnknown:           {},
	})
	p.WatchdogRecovery = normalizePolicyValue(p.WatchdogRecovery, WatchdogRecoveryManualOnly, map[string]struct{}{
		WatchdogRecoveryDisabled:             {},
		WatchdogRecoveryFinalizeTerminalOnly: {},
		WatchdogRecoveryRetryExternal:        {},
		WatchdogRecoveryManualOnly:           {},
	})
	p.IdempotencyKey = strings.TrimSpace(p.IdempotencyKey)
	p.TimeoutBehavior = strings.TrimSpace(p.TimeoutBehavior)
	return p
}

func (p RuntimeRecoveryPolicy) AllowsWatchdogExternalRetry() bool {
	p = p.Normalized()
	if p.WatchdogRecovery != WatchdogRecoveryRetryExternal {
		return false
	}
	if !p.HasExternalSideEffectBoundary() {
		return false
	}
	switch p.IdempotencyPolicy {
	case IdempotencyPolicyIdempotent:
		return true
	case IdempotencyPolicyDedupeKeyRequired:
		return p.IdempotencyKey != ""
	default:
		return false
	}
}

func (p RuntimeRecoveryPolicy) HasExternalSideEffectBoundary() bool {
	p = p.Normalized()
	return p.SideEffectPolicy == SideEffectPolicyExternalRead ||
		p.SideEffectPolicy == SideEffectPolicyExternalWrite
}

func (p RuntimeRecoveryPolicy) WatchdogRecoverySafety() string {
	p = p.Normalized()
	switch {
	case p.AllowsWatchdogExternalRetry() && p.IdempotencyPolicy == IdempotencyPolicyDedupeKeyRequired:
		return "recoverable_dedupe_key"
	case p.AllowsWatchdogExternalRetry():
		return "recoverable_idempotent"
	case p.WatchdogRecovery == WatchdogRecoveryRetryExternal && !p.HasExternalSideEffectBoundary():
		return "retry_external_not_external"
	case p.WatchdogRecovery == WatchdogRecoveryRetryExternal:
		return "retry_external_not_idempotent"
	case p.WatchdogRecovery == WatchdogRecoveryFinalizeTerminalOnly:
		return "finalize_terminal_only"
	case p.WatchdogRecovery == WatchdogRecoveryDisabled:
		return "disabled"
	default:
		return "manual_only"
	}
}

func (p RuntimeRecoveryPolicy) ToMap() map[string]any {
	p = p.Normalized()
	ret := map[string]any{
		"sideEffectPolicy":  p.SideEffectPolicy,
		"idempotencyPolicy": p.IdempotencyPolicy,
		"watchdogRecovery":  p.WatchdogRecovery,
	}
	if p.IdempotencyKey != "" {
		ret["idempotencyKey"] = p.IdempotencyKey
	}
	if p.TimeoutBehavior != "" {
		ret["timeoutBehavior"] = p.TimeoutBehavior
	}
	return ret
}

func stringFromPolicyMap(input map[string]any, key string) string {
	value, _ := input[key].(string)
	return strings.TrimSpace(value)
}

func normalizePolicyValue(value string, fallback string, allowed map[string]struct{}) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	if _, ok := allowed[value]; ok {
		return value
	}
	return fallback
}
