package workorchruntimecontract

import "fmt"

const (
	DefaultMaxProviderTransportAttempts = 3
	DefaultMaxOperationRecoveryAttempts = 3
	DefaultMaxCASReloadAttempts         = 5
	DefaultMaxTerminalDeliveryAttempts  = 5
	DefaultMaxCapabilityStateRefreshes  = 3
	RuntimeRecoveryBudgetLimit          = 100
)

type RuntimeRecoveryBudgetKind string

const (
	RuntimeRecoveryBudgetProviderTransport      RuntimeRecoveryBudgetKind = "provider_transport"
	RuntimeRecoveryBudgetOperationRecovery      RuntimeRecoveryBudgetKind = "operation_recovery"
	RuntimeRecoveryBudgetCASReload              RuntimeRecoveryBudgetKind = "cas_reload"
	RuntimeRecoveryBudgetTerminalDelivery       RuntimeRecoveryBudgetKind = "terminal_delivery"
	RuntimeRecoveryBudgetCapabilityStateRefresh RuntimeRecoveryBudgetKind = "capability_state_refresh"
)

type RuntimeRecoveryPolicy struct {
	MaxProviderTransportAttempts int
	MaxOperationRecoveryAttempts int
	MaxCASReloadAttempts         int
	MaxTerminalDeliveryAttempts  int
	MaxCapabilityStateRefreshes  int
}

type RuntimeRecoveryLedger struct {
	ProviderTransportAttempts int
	OperationRecoveryAttempts int
	CASReloadAttempts         int
	TerminalDeliveryAttempts  int
	CapabilityStateRefreshes  int
}

func DefaultRuntimeRecoveryPolicy() RuntimeRecoveryPolicy {
	return RuntimeRecoveryPolicy{
		MaxProviderTransportAttempts: DefaultMaxProviderTransportAttempts,
		MaxOperationRecoveryAttempts: DefaultMaxOperationRecoveryAttempts,
		MaxCASReloadAttempts:         DefaultMaxCASReloadAttempts,
		MaxTerminalDeliveryAttempts:  DefaultMaxTerminalDeliveryAttempts,
		MaxCapabilityStateRefreshes:  DefaultMaxCapabilityStateRefreshes,
	}
}

func (p RuntimeRecoveryPolicy) Normalized() RuntimeRecoveryPolicy {
	defaults := DefaultRuntimeRecoveryPolicy()
	if p.MaxProviderTransportAttempts <= 0 {
		p.MaxProviderTransportAttempts = defaults.MaxProviderTransportAttempts
	}
	if p.MaxOperationRecoveryAttempts <= 0 {
		p.MaxOperationRecoveryAttempts = defaults.MaxOperationRecoveryAttempts
	}
	if p.MaxCASReloadAttempts <= 0 {
		p.MaxCASReloadAttempts = defaults.MaxCASReloadAttempts
	}
	if p.MaxTerminalDeliveryAttempts <= 0 {
		p.MaxTerminalDeliveryAttempts = defaults.MaxTerminalDeliveryAttempts
	}
	if p.MaxCapabilityStateRefreshes <= 0 {
		p.MaxCapabilityStateRefreshes = defaults.MaxCapabilityStateRefreshes
	}
	return p
}

func (p RuntimeRecoveryPolicy) Validate() error {
	p = p.Normalized()
	for name, value := range map[string]int{
		"MaxProviderTransportAttempts": p.MaxProviderTransportAttempts,
		"MaxOperationRecoveryAttempts": p.MaxOperationRecoveryAttempts,
		"MaxCASReloadAttempts":         p.MaxCASReloadAttempts,
		"MaxTerminalDeliveryAttempts":  p.MaxTerminalDeliveryAttempts,
		"MaxCapabilityStateRefreshes":  p.MaxCapabilityStateRefreshes,
	} {
		if value < 1 || value > RuntimeRecoveryBudgetLimit {
			return fmt.Errorf("%s must be in 1..%d", name, RuntimeRecoveryBudgetLimit)
		}
	}
	return nil
}

func (l RuntimeRecoveryLedger) Validate(policy RuntimeRecoveryPolicy) error {
	policy = policy.Normalized()
	if err := policy.Validate(); err != nil {
		return err
	}
	for name, value := range map[string]int{
		"ProviderTransportAttempts": l.ProviderTransportAttempts,
		"OperationRecoveryAttempts": l.OperationRecoveryAttempts,
		"CASReloadAttempts":         l.CASReloadAttempts,
		"TerminalDeliveryAttempts":  l.TerminalDeliveryAttempts,
		"CapabilityStateRefreshes":  l.CapabilityStateRefreshes,
	} {
		if value < 0 {
			return fmt.Errorf("%s cannot be negative", name)
		}
	}
	return nil
}

func (l RuntimeRecoveryLedger) Used(kind RuntimeRecoveryBudgetKind) int {
	switch kind {
	case RuntimeRecoveryBudgetProviderTransport:
		return l.ProviderTransportAttempts
	case RuntimeRecoveryBudgetOperationRecovery:
		return l.OperationRecoveryAttempts
	case RuntimeRecoveryBudgetCASReload:
		return l.CASReloadAttempts
	case RuntimeRecoveryBudgetTerminalDelivery:
		return l.TerminalDeliveryAttempts
	case RuntimeRecoveryBudgetCapabilityStateRefresh:
		return l.CapabilityStateRefreshes
	default:
		return 0
	}
}

func (p RuntimeRecoveryPolicy) Limit(kind RuntimeRecoveryBudgetKind) int {
	p = p.Normalized()
	switch kind {
	case RuntimeRecoveryBudgetProviderTransport:
		return p.MaxProviderTransportAttempts
	case RuntimeRecoveryBudgetOperationRecovery:
		return p.MaxOperationRecoveryAttempts
	case RuntimeRecoveryBudgetCASReload:
		return p.MaxCASReloadAttempts
	case RuntimeRecoveryBudgetTerminalDelivery:
		return p.MaxTerminalDeliveryAttempts
	case RuntimeRecoveryBudgetCapabilityStateRefresh:
		return p.MaxCapabilityStateRefreshes
	default:
		return 0
	}
}

func (l RuntimeRecoveryLedger) Consume(kind RuntimeRecoveryBudgetKind, policy RuntimeRecoveryPolicy) (RuntimeRecoveryLedger, bool, error) {
	if policy.Limit(kind) == 0 {
		return l, false, fmt.Errorf("unknown runtime recovery budget %q", kind)
	}
	if l.Used(kind) >= policy.Limit(kind) {
		return l, false, nil
	}
	switch kind {
	case RuntimeRecoveryBudgetProviderTransport:
		l.ProviderTransportAttempts++
	case RuntimeRecoveryBudgetOperationRecovery:
		l.OperationRecoveryAttempts++
	case RuntimeRecoveryBudgetCASReload:
		l.CASReloadAttempts++
	case RuntimeRecoveryBudgetTerminalDelivery:
		l.TerminalDeliveryAttempts++
	case RuntimeRecoveryBudgetCapabilityStateRefresh:
		l.CapabilityStateRefreshes++
	}
	return l, true, nil
}
