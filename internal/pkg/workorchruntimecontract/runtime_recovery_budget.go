package workorchruntimecontract

import "fmt"

const (
	DefaultMaxProviderTransportAttempts = 3
	DefaultMaxOperationRecoveryAttempts = 3
	DefaultMaxCASReloadAttempts         = 5
	DefaultMaxTerminalDeliveryAttempts  = 5
	DefaultMaxDriveScheduleAttempts     = 3
	// DefaultMaxPrerequisiteRefreshes bounds stale/no-progress recovery. It is
	// deliberately separate from normal owner-reported resource changes: a
	// progressing Run may legitimately change several independent resources.
	DefaultMaxPrerequisiteRefreshes = 3
	// DefaultMaxPrerequisiteResourceChangeRefreshes is the sealed allowance for
	// normal durable owner receipts that invalidate an intersecting prerequisite.
	// It is still bounded by RuntimeRecoveryBudgetLimit and does not relax stale
	// or duplicate receipt handling.
	DefaultMaxPrerequisiteResourceChangeRefreshes = 8
	RuntimeRecoveryBudgetLimit                    = 100
)

type RuntimeRecoveryBudgetKind string

const (
	RuntimeRecoveryBudgetProviderTransport                 RuntimeRecoveryBudgetKind = "provider_transport"
	RuntimeRecoveryBudgetOperationRecovery                 RuntimeRecoveryBudgetKind = "operation_recovery"
	RuntimeRecoveryBudgetCASReload                         RuntimeRecoveryBudgetKind = "cas_reload"
	RuntimeRecoveryBudgetTerminalDelivery                  RuntimeRecoveryBudgetKind = "terminal_delivery"
	RuntimeRecoveryBudgetDriveSchedule                     RuntimeRecoveryBudgetKind = "drive_schedule"
	RuntimeRecoveryBudgetPrerequisiteRefresh               RuntimeRecoveryBudgetKind = "prerequisite_refresh"
	RuntimeRecoveryBudgetPrerequisiteResourceChangeRefresh RuntimeRecoveryBudgetKind = "prerequisite_receipt_refresh"
)

type RuntimeRecoveryPolicy struct {
	MaxProviderTransportAttempts int
	MaxOperationRecoveryAttempts int
	MaxCASReloadAttempts         int
	MaxTerminalDeliveryAttempts  int
	MaxDriveScheduleAttempts     int
	// MaxPrerequisiteRefreshes is the stale/no-progress recovery budget.
	MaxPrerequisiteRefreshes int
	// MaxPrerequisiteResourceChangeRefreshes bounds normal progressing owner
	// receipts whose resource revisions invalidate a prerequisite observation.
	// It is sealed at activation and is not a live orchestrator configuration.
	MaxPrerequisiteResourceChangeRefreshes int
}

type RuntimeRecoveryLedger struct {
	ProviderTransportAttempts int
	OperationRecoveryAttempts int
	CASReloadAttempts         int
	TerminalDeliveryAttempts  int
	DriveScheduleAttempts     int
	// PrerequisiteRefreshes tracks stale/no-progress recovery only.
	PrerequisiteRefreshes int
	// PrerequisiteResourceChangeRefreshes tracks normal progressing owner
	// receipts separately from stale/no-progress recovery.
	PrerequisiteResourceChangeRefreshes int
}

func DefaultRuntimeRecoveryPolicy() RuntimeRecoveryPolicy {
	return RuntimeRecoveryPolicy{
		MaxProviderTransportAttempts:           DefaultMaxProviderTransportAttempts,
		MaxOperationRecoveryAttempts:           DefaultMaxOperationRecoveryAttempts,
		MaxCASReloadAttempts:                   DefaultMaxCASReloadAttempts,
		MaxTerminalDeliveryAttempts:            DefaultMaxTerminalDeliveryAttempts,
		MaxDriveScheduleAttempts:               DefaultMaxDriveScheduleAttempts,
		MaxPrerequisiteRefreshes:               DefaultMaxPrerequisiteRefreshes,
		MaxPrerequisiteResourceChangeRefreshes: DefaultMaxPrerequisiteResourceChangeRefreshes,
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
	if p.MaxDriveScheduleAttempts <= 0 {
		p.MaxDriveScheduleAttempts = defaults.MaxDriveScheduleAttempts
	}
	if p.MaxPrerequisiteRefreshes <= 0 {
		p.MaxPrerequisiteRefreshes = defaults.MaxPrerequisiteRefreshes
	}
	if p.MaxPrerequisiteResourceChangeRefreshes <= 0 {
		p.MaxPrerequisiteResourceChangeRefreshes = defaults.MaxPrerequisiteResourceChangeRefreshes
	}
	return p
}

func (p RuntimeRecoveryPolicy) Validate() error {
	p = p.Normalized()
	for name, value := range map[string]int{
		"MaxProviderTransportAttempts":           p.MaxProviderTransportAttempts,
		"MaxOperationRecoveryAttempts":           p.MaxOperationRecoveryAttempts,
		"MaxCASReloadAttempts":                   p.MaxCASReloadAttempts,
		"MaxTerminalDeliveryAttempts":            p.MaxTerminalDeliveryAttempts,
		"MaxDriveScheduleAttempts":               p.MaxDriveScheduleAttempts,
		"MaxPrerequisiteRefreshes":               p.MaxPrerequisiteRefreshes,
		"MaxPrerequisiteResourceChangeRefreshes": p.MaxPrerequisiteResourceChangeRefreshes,
	} {
		if value < 1 || value > RuntimeRecoveryBudgetLimit {
			return fmt.Errorf("%s must be in 1..%d", name, RuntimeRecoveryBudgetLimit)
		}
	}
	return nil
}

func (l RuntimeRecoveryLedger) Validate(policy RuntimeRecoveryPolicy) error {
	l = l.Normalized()
	policy = policy.Normalized()
	if err := policy.Validate(); err != nil {
		return err
	}
	for name, value := range map[string]int{
		"ProviderTransportAttempts":           l.ProviderTransportAttempts,
		"OperationRecoveryAttempts":           l.OperationRecoveryAttempts,
		"CASReloadAttempts":                   l.CASReloadAttempts,
		"TerminalDeliveryAttempts":            l.TerminalDeliveryAttempts,
		"DriveScheduleAttempts":               l.DriveScheduleAttempts,
		"PrerequisiteRefreshes":               l.PrerequisiteRefreshes,
		"PrerequisiteResourceChangeRefreshes": l.PrerequisiteResourceChangeRefreshes,
	} {
		if value < 0 {
			return fmt.Errorf("%s cannot be negative", name)
		}
	}
	return nil
}

func (l RuntimeRecoveryLedger) Normalized() RuntimeRecoveryLedger {
	return l
}

func (l RuntimeRecoveryLedger) Used(kind RuntimeRecoveryBudgetKind) int {
	l = l.Normalized()
	switch kind {
	case RuntimeRecoveryBudgetProviderTransport:
		return l.ProviderTransportAttempts
	case RuntimeRecoveryBudgetOperationRecovery:
		return l.OperationRecoveryAttempts
	case RuntimeRecoveryBudgetCASReload:
		return l.CASReloadAttempts
	case RuntimeRecoveryBudgetTerminalDelivery:
		return l.TerminalDeliveryAttempts
	case RuntimeRecoveryBudgetDriveSchedule:
		return l.DriveScheduleAttempts
	case RuntimeRecoveryBudgetPrerequisiteRefresh:
		return l.PrerequisiteRefreshes
	case RuntimeRecoveryBudgetPrerequisiteResourceChangeRefresh:
		return l.PrerequisiteResourceChangeRefreshes
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
	case RuntimeRecoveryBudgetDriveSchedule:
		return p.MaxDriveScheduleAttempts
	case RuntimeRecoveryBudgetPrerequisiteRefresh:
		return p.MaxPrerequisiteRefreshes
	case RuntimeRecoveryBudgetPrerequisiteResourceChangeRefresh:
		return p.MaxPrerequisiteResourceChangeRefreshes
	default:
		return 0
	}
}
