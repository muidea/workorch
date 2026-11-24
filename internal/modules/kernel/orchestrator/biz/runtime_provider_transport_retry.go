package orchestrator

import (
	"fmt"
	"strings"
	"time"

	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// runtimeProviderTransportRetryPendingError is private SM-4 scheduling state.
// The persisted CandidateRequestSession remains the only retry/due-time
// authority; this error merely asks SM-4 exit settlement to schedule a
// RuntimeDriveLeaseReasonProviderRetry timer at session.TransportRetryDueAt.
type runtimeProviderTransportRetryPendingError struct {
	LogicalRequestID  string
	OutcomeSignature  string
	DueAt             string
	TransportOutcomes int
	Revision          uint64
}

func (e *runtimeProviderTransportRetryPendingError) Error() string {
	if e == nil {
		return "Runtime provider transport retry is pending"
	}
	return fmt.Sprintf("Runtime provider transport retry %s attempt %d is pending until %s", e.LogicalRequestID, e.TransportOutcomes, e.DueAt)
}

func (e *runtimeProviderTransportRetryPendingError) leaseSignature() string {
	if e == nil {
		return "provider-transport/invalid"
	}
	signature := strings.TrimSpace(e.OutcomeSignature)
	if signature == "" {
		signature = "provider-transient"
	}
	return fmt.Sprintf("provider-transport/%s/%s/%d", strings.TrimSpace(e.LogicalRequestID), signature, e.TransportOutcomes)
}

func (m *Biz) runtimeNowUTC() time.Time {
	if m != nil && m.nowFn != nil {
		return m.nowFn().UTC()
	}
	return time.Now().UTC()
}

func runtimeProviderTransportRetryFromSession(aggregate runtime.RuntimeAggregate, session runtimecontract.RuntimeCandidateRequestSession) (runtimeProviderTransportRetryPendingError, bool) {
	if session.ProviderSubmitted || session.State != runtimecontract.RuntimeCandidateRequestSessionTransportPending {
		return runtimeProviderTransportRetryPendingError{}, false
	}
	return runtimeProviderTransportRetryPendingError{
		LogicalRequestID:  session.LogicalRequestID,
		OutcomeSignature:  session.LastOutcome.Signature,
		DueAt:             strings.TrimSpace(session.TransportRetryDueAt),
		TransportOutcomes: session.TransportOutcomes,
		Revision:          aggregate.StateRevision,
	}, true
}

func runtimePendingProviderTransportRetry(aggregate runtime.RuntimeAggregate) (runtimeProviderTransportRetryPendingError, bool) {
	if aggregate.DefinitionClarification != nil {
		return runtimeProviderTransportRetryPendingError{}, false
	}
	if aggregate.Decision.State != runtimecontract.ConvergenceStateNeedGovernance && runtimeSemanticAuthorityPending(aggregate) {
		if logicalID := strings.TrimSpace(aggregate.SemanticPlanning.LogicalRequestID); logicalID != "" {
			if session, found := runtimeCandidateRequestSession(aggregate, logicalID); found {
				return runtimeProviderTransportRetryFromSession(aggregate, session)
			}
		}
		return runtimeProviderTransportRetryPendingError{}, false
	}
	if aggregate.ActiveDispatch != nil || runtimeConvergenceTerminal(aggregate.Decision.State) {
		return runtimeProviderTransportRetryPendingError{}, false
	}
	transaction := aggregate.Decision.AuthorityTransaction()
	for _, session := range aggregate.CandidateRequestSessions {
		if session.DocumentKind != runtimecontract.RuntimeProviderDocumentOperations || session.Transaction != transaction {
			continue
		}
		if pending, ok := runtimeProviderTransportRetryFromSession(aggregate, session); ok {
			return pending, true
		}
	}
	return runtimeProviderTransportRetryPendingError{}, false
}

func (m *Biz) runtimeProviderTransportRetryNotDue(aggregate runtime.RuntimeAggregate, session runtimecontract.RuntimeCandidateRequestSession) (runtimeProviderTransportRetryPendingError, bool) {
	pending, ok := runtimeProviderTransportRetryFromSession(aggregate, session)
	if !ok {
		return runtimeProviderTransportRetryPendingError{}, false
	}
	dueAt, err := time.Parse(time.RFC3339Nano, pending.DueAt)
	if err != nil {
		return runtimeProviderTransportRetryPendingError{}, false
	}
	if m.runtimeNowUTC().Before(dueAt) {
		return pending, true
	}
	return runtimeProviderTransportRetryPendingError{}, false
}

func (m *Biz) runtimeProviderTransportRecoveryLeaseRequest(pending runtimeProviderTransportRetryPendingError, source string) *runtimeRecoveryLeaseRequest {
	delay := time.Second
	dueText := strings.TrimSpace(pending.DueAt)
	if dueAt, err := time.Parse(time.RFC3339Nano, dueText); err == nil {
		delay = dueAt.Sub(m.runtimeNowUTC())
		if delay <= 0 {
			delay = time.Millisecond
		}
	}
	return &runtimeRecoveryLeaseRequest{
		Reason:           runtimecontract.RuntimeDriveLeaseReasonProviderRetry,
		FailureSignature: pending.leaseSignature(),
		DueAt:            dueText,
		Delay:            delay,
		Source:           source,
	}
}
