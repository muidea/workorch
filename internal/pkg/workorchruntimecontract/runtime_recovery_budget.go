package workorchruntimecontract

import (
	"fmt"
	"strconv"
	"strings"
)

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
	RuntimeRecoveryBudgetLimit      = 100
)

type RuntimeRecoveryBudgetKind string

const (
	RuntimeRecoveryBudgetProviderTransport   RuntimeRecoveryBudgetKind = "provider_transport"
	RuntimeRecoveryBudgetOperationRecovery   RuntimeRecoveryBudgetKind = "operation_recovery"
	RuntimeRecoveryBudgetCASReload           RuntimeRecoveryBudgetKind = "cas_reload"
	RuntimeRecoveryBudgetTerminalDelivery    RuntimeRecoveryBudgetKind = "terminal_delivery"
	RuntimeRecoveryBudgetDriveSchedule       RuntimeRecoveryBudgetKind = "drive_schedule"
	RuntimeRecoveryBudgetPrerequisiteRefresh RuntimeRecoveryBudgetKind = "prerequisite_refresh"
)

type RuntimeRecoveryPolicy struct {
	MaxProviderTransportAttempts int
	MaxOperationRecoveryAttempts int
	MaxCASReloadAttempts         int
	MaxTerminalDeliveryAttempts  int
	MaxDriveScheduleAttempts     int
	// MaxPrerequisiteRefreshes is the stale/no-progress recovery budget.
	MaxPrerequisiteRefreshes int
}

type RuntimeRecoveryLedger struct {
	ProviderTransportAttempts int
	OperationRecoveryAttempts int
	CASReloadAttempts         int
	// NonTerminalRecovery is the latest owner-private atomic budget/successor
	// record. FailedLeaseGeneration makes older callbacks stale without an
	// unbounded history ledger.
	NonTerminalRecovery      RuntimeNonTerminalRecoveryRecord
	TerminalDeliveryAttempts int
	// TerminalDeliveryRequest is the most recent durable idempotency fence for
	// terminal-delivery budget consumption. FailedLeaseGeneration is monotonic,
	// so an older callback remains recognizable after later attempts without a
	// second unbounded history ledger.
	TerminalDeliveryRequest RuntimeTerminalDeliveryRecoveryRequest
	// TerminalDeliveryIntervention is the exact failed delivery for which the
	// bounded automatic budget was exhausted. It is a durable, observable
	// manual-intervention disposition; no local gate/pump may keep auto-retrying
	// after this fact is sealed.
	TerminalDeliveryIntervention RuntimeTerminalDeliveryRecoveryRequest
	// RetiredTerminalDeliveryLease preserves the exact running authority that an
	// administrative termination retired in its CAS. Only one matching,
	// budget-authorized terminal successor may consume this handoff marker.
	RetiredTerminalDeliveryLease                  RuntimeDriveLease
	RetiredTerminalDeliveryLeaseRecoveryRequestID string
	DriveScheduleAttempts                         int
	// PrerequisiteRefreshes tracks stale/no-progress recovery only.
	PrerequisiteRefreshes int
}

type RuntimeTerminalDeliveryRecoveryRequest struct {
	RequestID             string
	TerminalDeliveryID    string
	FailedLeaseGeneration uint64
}

func (r RuntimeTerminalDeliveryRecoveryRequest) Normalized() RuntimeTerminalDeliveryRecoveryRequest {
	r.RequestID = strings.TrimSpace(r.RequestID)
	r.TerminalDeliveryID = strings.TrimSpace(r.TerminalDeliveryID)
	return r
}

func (r RuntimeTerminalDeliveryRecoveryRequest) Validate() error {
	r = r.Normalized()
	if r.RequestID == "" || r.TerminalDeliveryID == "" ||
		r.RequestID != StableRuntimeTerminalDeliveryRecoveryRequestID(r.TerminalDeliveryID, r.FailedLeaseGeneration) {
		return fmt.Errorf("terminal delivery recovery request requires exact stable identity")
	}
	return nil
}

func StableRuntimeTerminalDeliveryRecoveryRequestID(terminalDeliveryID string, failedLeaseGeneration uint64) string {
	fence := "initial"
	if failedLeaseGeneration != 0 {
		fence = "lease-" + strconv.FormatUint(failedLeaseGeneration, 10)
	}
	return "terminal-delivery-recovery:" + strings.TrimSpace(terminalDeliveryID) + "/" + fence
}

func DefaultRuntimeRecoveryPolicy() RuntimeRecoveryPolicy {
	return RuntimeRecoveryPolicy{
		MaxProviderTransportAttempts: DefaultMaxProviderTransportAttempts,
		MaxOperationRecoveryAttempts: DefaultMaxOperationRecoveryAttempts,
		MaxCASReloadAttempts:         DefaultMaxCASReloadAttempts,
		MaxTerminalDeliveryAttempts:  DefaultMaxTerminalDeliveryAttempts,
		MaxDriveScheduleAttempts:     DefaultMaxDriveScheduleAttempts,
		MaxPrerequisiteRefreshes:     DefaultMaxPrerequisiteRefreshes,
	}
}

func (p RuntimeRecoveryPolicy) Normalized() RuntimeRecoveryPolicy {
	return p
}

func (p RuntimeRecoveryPolicy) Validate() error {
	p = p.Normalized()
	for name, value := range map[string]int{
		"MaxProviderTransportAttempts": p.MaxProviderTransportAttempts,
		"MaxOperationRecoveryAttempts": p.MaxOperationRecoveryAttempts,
		"MaxCASReloadAttempts":         p.MaxCASReloadAttempts,
		"MaxTerminalDeliveryAttempts":  p.MaxTerminalDeliveryAttempts,
		"MaxDriveScheduleAttempts":     p.MaxDriveScheduleAttempts,
		"MaxPrerequisiteRefreshes":     p.MaxPrerequisiteRefreshes,
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
		"ProviderTransportAttempts": l.ProviderTransportAttempts,
		"OperationRecoveryAttempts": l.OperationRecoveryAttempts,
		"CASReloadAttempts":         l.CASReloadAttempts,
		"TerminalDeliveryAttempts":  l.TerminalDeliveryAttempts,
		"DriveScheduleAttempts":     l.DriveScheduleAttempts,
		"PrerequisiteRefreshes":     l.PrerequisiteRefreshes,
	} {
		if value < 0 {
			return fmt.Errorf("%s cannot be negative", name)
		}
	}
	nonterminal := l.NonTerminalRecovery.Normalized()
	if RuntimeNonTerminalRecoveryRecordPresent(nonterminal) {
		if err := nonterminal.Validate(policy); err != nil {
			return fmt.Errorf("runtime nonterminal recovery: %w", err)
		}
		used := l.CASReloadAttempts
		if used != nonterminal.Attempt {
			return fmt.Errorf("runtime nonterminal recovery record does not match consumed counter")
		}
		if nonterminal.Disposition == RuntimeNonTerminalRecoveryDispositionBudgetExhausted && used != policy.MaxCASReloadAttempts {
			return fmt.Errorf("runtime nonterminal recovery exhaustion does not match consumed limit")
		}
	} else if l.CASReloadAttempts != 0 {
		return fmt.Errorf("CAS reload attempts require an atomic nonterminal recovery record")
	}
	request := l.TerminalDeliveryRequest.Normalized()
	if request.RequestID != "" || request.TerminalDeliveryID != "" || request.FailedLeaseGeneration != 0 {
		if err := request.Validate(); err != nil {
			return err
		}
		if l.TerminalDeliveryAttempts == 0 {
			return fmt.Errorf("terminal delivery recovery request requires a consumed attempt")
		}
	}
	intervention := l.TerminalDeliveryIntervention.Normalized()
	if intervention.RequestID != "" || intervention.TerminalDeliveryID != "" || intervention.FailedLeaseGeneration != 0 {
		if err := intervention.Validate(); err != nil {
			return fmt.Errorf("terminal delivery intervention: %w", err)
		}
		if l.TerminalDeliveryAttempts < policy.MaxTerminalDeliveryAttempts {
			return fmt.Errorf("terminal delivery intervention requires exhausted automatic budget")
		}
		if request.RequestID != "" && (request.TerminalDeliveryID != intervention.TerminalDeliveryID || request.FailedLeaseGeneration > intervention.FailedLeaseGeneration) {
			return fmt.Errorf("terminal delivery intervention precedes latest consumed request")
		}
	}
	retired := l.RetiredTerminalDeliveryLease.Normalized()
	if RuntimeDriveLeasePresent(retired) {
		if err := retired.Validate(); err != nil {
			return fmt.Errorf("retired terminal delivery lease: %w", err)
		}
		if retired.State != RuntimeDriveLeaseRunning {
			return fmt.Errorf("retired terminal delivery lease must be running")
		}
		requestBoundNonterminal := RuntimeDriveLeaseBindsNonTerminalRecovery(retired) &&
			ValidateRuntimeNonTerminalRecoveryLeaseBinding(l, retired) == nil
		if (retired.RecoveryRequestID != "" && !requestBoundNonterminal) ||
			(retired.Reason == RuntimeDriveLeaseReasonDeliveryRetry && retired.FailureSignature == "terminal-delivery") {
			return fmt.Errorf("retired terminal delivery lease must preserve pre-terminal running authority")
		}
	} else if l.RetiredTerminalDeliveryLeaseRecoveryRequestID != "" {
		return fmt.Errorf("retired terminal delivery lease consumed marker requires exact lease")
	}
	retiredRequestID := strings.TrimSpace(l.RetiredTerminalDeliveryLeaseRecoveryRequestID)
	if retiredRequestID != "" {
		expected := StableRuntimeTerminalDeliveryRecoveryRequestID(request.TerminalDeliveryID, retired.RecoveryGeneration)
		if request.Validate() != nil || retiredRequestID != expected {
			return fmt.Errorf("retired terminal delivery lease handoff requires exact recovery request")
		}
	}
	return nil
}

func (l RuntimeRecoveryLedger) Normalized() RuntimeRecoveryLedger {
	l.NonTerminalRecovery = l.NonTerminalRecovery.Normalized()
	l.TerminalDeliveryRequest = l.TerminalDeliveryRequest.Normalized()
	l.TerminalDeliveryIntervention = l.TerminalDeliveryIntervention.Normalized()
	l.RetiredTerminalDeliveryLease = l.RetiredTerminalDeliveryLease.Normalized()
	l.RetiredTerminalDeliveryLeaseRecoveryRequestID = strings.TrimSpace(l.RetiredTerminalDeliveryLeaseRecoveryRequestID)
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
	default:
		return 0
	}
}
