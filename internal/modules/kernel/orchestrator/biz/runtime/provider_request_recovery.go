package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// RuntimeProviderRequestRecoveryInput is an Orchestrator-private projection
// of one LLM-owner recoverable result. Exactly one transport outcome or
// incomplete-document framing is allowed. It contains no provider body and no
// requested Runtime action.
type RuntimeProviderRequestRecoveryInput struct {
	DocumentKind       runtimecontract.RuntimeProviderDocumentKind
	TransportOutcome   *runtimecontract.RuntimeCandidateRequestOutcome
	DocumentFraming    *runtimecontract.RuntimeProviderDocumentFraming
	Message            string
	OriginalDiagnostic *runtimecontract.RuntimeOriginalDiagnostic
}

// RuntimeProviderRequestRecoveryFact carries both the observed recoverable
// result and its deterministic exhausted successor. The successor is applied
// only when the sealed guard is exhausted, but computing it before mutation
// keeps operations and semantic-plan recovery on one normalization path.
type RuntimeProviderRequestRecoveryFact struct {
	DocumentKind     runtimecontract.RuntimeProviderDocumentKind
	TransportOutcome *runtimecontract.RuntimeCandidateRequestOutcome
	DocumentFraming  *runtimecontract.RuntimeProviderDocumentFraming
	Terminal         RuntimeProviderRequestTerminalFact
}

// NormalizeRuntimeProviderRequestRecovery is the single recovery normalizer
// for operations and semantic-plan provider calls. It delegates terminal
// identity to NormalizeRuntimeProviderRequestTerminal so exact producer
// diagnostics are preserved when transport recovery is exhausted.
func NormalizeRuntimeProviderRequestRecovery(input RuntimeProviderRequestRecoveryInput) (RuntimeProviderRequestRecoveryFact, error) {
	input.DocumentKind = runtimecontract.RuntimeProviderDocumentKind(strings.TrimSpace(string(input.DocumentKind)))
	input.Message = strings.TrimSpace(input.Message)
	if input.DocumentKind != runtimecontract.RuntimeProviderDocumentOperations && input.DocumentKind != runtimecontract.RuntimeProviderDocumentSemanticPlan {
		return RuntimeProviderRequestRecoveryFact{}, common.ErrInvalidArgument
	}
	if (input.TransportOutcome == nil) == (input.DocumentFraming == nil) {
		return RuntimeProviderRequestRecoveryFact{}, common.ErrInvalidArgument
	}

	fact := RuntimeProviderRequestRecoveryFact{DocumentKind: input.DocumentKind}
	terminalInput := RuntimeProviderRequestTerminalInput{DocumentKind: input.DocumentKind}
	if input.TransportOutcome != nil {
		outcome := input.TransportOutcome.Normalized()
		if err := outcome.Validate(); err != nil || outcome.Class != runtimecontract.RuntimeCandidateRequestOutcomeProviderTransient || outcome.Code == "" || outcome.Signature == "" || input.Message == "" {
			return RuntimeProviderRequestRecoveryFact{}, common.ErrInvalidArgument
		}
		diagnostic := input.OriginalDiagnostic.CloneOriginal()
		if !providerTransientRecoveryIdentityMatches(outcome, diagnostic) || strings.TrimSpace(diagnostic.OriginalMessage) != input.Message {
			return RuntimeProviderRequestRecoveryFact{}, common.ErrInvalidArgument
		}
		fact.TransportOutcome = &outcome
		terminalInput.OutcomeClass = runtimecontract.RuntimeCandidateRequestOutcomeProviderTransient
		terminalInput.TerminalCode = "provider_transport_exhausted"
		terminalInput.TerminalSignature = "provider-transport-exhausted"
		if input.DocumentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan {
			terminalInput.TerminalCode = "semantic_provider_transport_exhausted"
			terminalInput.TerminalSignature = "semantic-provider-transport-exhausted"
		}
		terminalInput.Message = input.Message
		terminalInput.OriginalDiagnostic = diagnostic
	} else {
		framing := input.DocumentFraming.Normalized()
		if err := framing.Validate(); err != nil || framing.Disposition != runtimecontract.RuntimeProviderDocumentIncomplete {
			return RuntimeProviderRequestRecoveryFact{}, common.ErrInvalidArgument
		}
		fact.DocumentFraming = &framing
		terminalInput.OutcomeClass = runtimecontract.RuntimeCandidateRequestOutcomeDocumentRecoveryExhausted
		terminalInput.TerminalCode = "candidate_document_recovery_exhausted"
		terminalInput.TerminalSignature = "candidate-document-recovery-exhausted"
		terminalInput.Message = "provider candidate output remained incomplete after its activation-sealed complete-document recovery budget was exhausted"
		if input.DocumentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan {
			terminalInput.TerminalCode = "semantic_document_recovery_exhausted"
			terminalInput.TerminalSignature = "semantic-document-recovery-exhausted"
			terminalInput.Message = "semantic document remained incomplete after its sealed whole-document recovery budget"
		}
	}
	terminal, err := NormalizeRuntimeProviderRequestTerminal(terminalInput)
	if err != nil {
		return RuntimeProviderRequestRecoveryFact{}, err
	}
	fact.Terminal = terminal
	if err := fact.Validate(); err != nil {
		return RuntimeProviderRequestRecoveryFact{}, err
	}
	return fact, nil
}

func (f RuntimeProviderRequestRecoveryFact) Validate() error {
	f.DocumentKind = runtimecontract.RuntimeProviderDocumentKind(strings.TrimSpace(string(f.DocumentKind)))
	if f.DocumentKind != runtimecontract.RuntimeProviderDocumentOperations && f.DocumentKind != runtimecontract.RuntimeProviderDocumentSemanticPlan {
		return common.ErrInvalidArgument
	}
	if (f.TransportOutcome == nil) == (f.DocumentFraming == nil) || f.Terminal.DocumentKind != f.DocumentKind {
		return common.ErrInvalidArgument
	}
	if err := f.Terminal.Validate(); err != nil {
		return err
	}
	if f.TransportOutcome != nil {
		outcome := f.TransportOutcome.Normalized()
		if err := outcome.Validate(); err != nil || outcome.Class != runtimecontract.RuntimeCandidateRequestOutcomeProviderTransient || outcome.Code == "" || outcome.Signature == "" || f.Terminal.RequestOutcome != outcome || !providerTransientRecoveryIdentityMatches(outcome, f.Terminal.Failure.OriginalDiagnostic) || strings.TrimSpace(f.Terminal.Failure.OriginalDiagnostic.OriginalMessage) != strings.TrimSpace(f.Terminal.Failure.Message) {
			return common.ErrInvalidArgument
		}
		expectedCode := "provider_transport_exhausted"
		if f.DocumentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan {
			expectedCode = "semantic_provider_transport_exhausted"
		} else if f.Terminal.Failure.Class != runtimecontract.FailureClassProviderTransient {
			return common.ErrInvalidArgument
		}
		if f.Terminal.TerminalCode != expectedCode {
			return common.ErrInvalidArgument
		}
		return nil
	}
	framing := f.DocumentFraming.Normalized()
	if err := framing.Validate(); err != nil || framing.Disposition != runtimecontract.RuntimeProviderDocumentIncomplete || f.Terminal.RequestOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomeDocumentRecoveryExhausted {
		return common.ErrInvalidArgument
	}
	expectedCode := "candidate_document_recovery_exhausted"
	if f.DocumentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan {
		expectedCode = "semantic_document_recovery_exhausted"
	}
	if f.Terminal.TerminalCode != expectedCode {
		return common.ErrInvalidArgument
	}
	return nil
}

func providerTransientRecoveryIdentityMatches(outcome runtimecontract.RuntimeCandidateRequestOutcome, diagnostic *runtimecontract.RuntimeOriginalDiagnostic) bool {
	outcome = outcome.Normalized()
	if diagnostic == nil || diagnostic.Validate() != nil {
		return false
	}
	canonical := diagnostic.Normalized()
	return canonical.OwnerModuleID == runtimecontract.RuntimeModuleLLM &&
		canonical.OriginalClass == string(runtimecontract.RuntimeCandidateFailureClassProviderTransient) &&
		canonical.OriginalCode == outcome.Code && canonical.Signature == outcome.Signature
}

// coordinateProviderRequestRecovery is the explicit SM-3/SM-4 recovery
// transaction with an optional SM-2 terminal successor. Physical-call budget
// settlement, request-session progression, provider-transport budget and an
// exhausted terminal are validated and committed under one aggregate CAS.
func (c *planningTransactionComponent) coordinateProviderRequestRecovery(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, logicalRequestID, physicalCallID string, fact RuntimeProviderRequestRecoveryFact, budgetSettlement ...*RuntimeProviderBudgetSettlement) (RuntimeAggregate, error) {
	logicalRequestID = strings.TrimSpace(logicalRequestID)
	physicalCallID = strings.TrimSpace(physicalCallID)
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || logicalRequestID == "" || physicalCallID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	if err := fact.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	idx, session, found := runtimeCandidateRequestSessionByID(aggregate.CandidateRequestSessions, logicalRequestID)
	if !found {
		return RuntimeAggregate{}, fmt.Errorf("%w: runtime candidate request session not found", common.ErrNotFound)
	}
	if session.DocumentKind != fact.DocumentKind {
		return RuntimeAggregate{}, fmt.Errorf("%w: provider recovery document kind does not match request session", common.ErrConflict)
	}
	settlement, err := candidateRequestBudgetSettlementForIdentity(session, budgetSettlement...)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if settlement.Status != RuntimeProviderBudgetSettlementConfirmed {
		return RuntimeAggregate{}, fmt.Errorf("%w: recoverable provider result requires confirmed physical submission", common.ErrConflict)
	}
	if providerRequestRecoveryDuplicateApplied(aggregate, session, physicalCallID, fact) && providerRequestBudgetSettlementApplied(aggregate, settlement) {
		return aggregate, nil
	}
	if session.State == runtimecontract.RuntimeCandidateRequestSessionTerminal {
		return RuntimeAggregate{}, fmt.Errorf("%w: conflicting provider request recovery terminal fact", common.ErrConflict)
	}
	if err := rejectRuntimeProgressAfterTermination(aggregate); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := candidateRequestSessionActionAllowed(aggregate, session.DocumentKind); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := validateCurrentCandidateRequestSession(aggregate, session); err != nil {
		return RuntimeAggregate{}, err
	}
	if session.State == runtimecontract.RuntimeCandidateRequestSessionDocumentComplete || session.State == runtimecontract.RuntimeCandidateRequestSessionDocumentConformanceRejected {
		return RuntimeAggregate{}, fmt.Errorf("%w: complete provider request cannot accept recovery", common.ErrConflict)
	}
	if !session.ProviderSubmitted || session.LastPhysicalCallID != physicalCallID {
		return RuntimeAggregate{}, fmt.Errorf("%w: provider recovery does not match submitted call", common.ErrConflict)
	}
	if fact.DocumentFraming != nil && (session.ExpectedDocument == nil || fact.DocumentFraming.ExpectedDocumentHash != session.ExpectedDocument.Hash) {
		return RuntimeAggregate{}, fmt.Errorf("%w: incomplete provider document does not match submitted expected document", common.ErrConflict)
	}

	expectedRevision := aggregate.StateRevision
	if _, err := applyCandidateRequestBudgetSettlement(&aggregate, settlement); err != nil {
		return RuntimeAggregate{}, err
	}

	updated := &aggregate.CandidateRequestSessions[idx]
	updated.ProviderSubmitted = false
	terminal := false
	if fact.TransportOutcome != nil {
		updated.State = runtimecontract.RuntimeCandidateRequestSessionTransportPending
		updated.LastOutcome = fact.TransportOutcome.Normalized()
		updated.LastDocumentFraming = nil
		updated.NextSequence++
		updated.TransportOutcomes++
		ledger, consumed, consumeErr := consumeRecoveryBudget(aggregate.Facts.Recovery, runtimecontract.RuntimeRecoveryBudgetProviderTransport, aggregate.Facts.RecoveryPolicy)
		if consumeErr != nil {
			return RuntimeAggregate{}, consumeErr
		}
		if consumed {
			aggregate.Facts.Recovery = ledger
		} else {
			terminal = true
		}
	} else {
		framing := fact.DocumentFraming.Normalized()
		updated.State = runtimecontract.RuntimeCandidateRequestSessionRecoveryPending
		updated.LastOutcome = runtimecontract.RuntimeCandidateRequestOutcome{Class: runtimecontract.RuntimeCandidateRequestOutcomeDocumentIncomplete, Code: framing.Code, Signature: framing.Signature}.Normalized()
		updated.LastDocumentFraming = &framing
		updated.NextSequence++
		updated.IncompleteOutcomes++
		terminal = updated.ExpectedDocument == nil || updated.IncompleteOutcomes > updated.ExpectedDocument.MaxRecoveryAttempts
	}
	if terminal {
		aggregate, err = applyProviderRequestTerminalToAggregate(aggregate, idx, fact.Terminal)
		if err != nil {
			return RuntimeAggregate{}, err
		}
	}
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func providerRequestBudgetSettlementApplied(aggregate RuntimeAggregate, settlement *RuntimeProviderBudgetSettlement) bool {
	if settlement == nil || settlement.Validate() != nil {
		return false
	}
	fact := settlement.Clone()
	matched := 0
	for _, allocation := range aggregate.ExecutionBudget.Allocations {
		allocation = allocation.Normalized()
		if allocation.LogicalConsumptionID != fact.LogicalRequestID || allocation.ScopeRef != fact.PhysicalCallID {
			continue
		}
		matched++
		switch fact.Status {
		case RuntimeProviderBudgetSettlementReleased:
			if allocation.State != runtimecontract.RuntimeBudgetAllocationReleased || allocation.OwnerFactRef != fact.OwnerFactRef {
				return false
			}
		case RuntimeProviderBudgetSettlementConfirmed, RuntimeProviderBudgetSettlementUnknown:
			wantState := runtimecontract.RuntimeBudgetAllocationConfirmed
			if fact.Status == RuntimeProviderBudgetSettlementUnknown || ((allocation.Dimension == runtimecontract.RuntimeBudgetDimensionProviderInputToken || allocation.Dimension == runtimecontract.RuntimeBudgetDimensionProviderOutputToken) && fact.Usage == nil) {
				wantState = runtimecontract.RuntimeBudgetAllocationUnknown
			}
			if allocation.State != wantState || allocation.OwnerFactRef != fact.OwnerFactRef ||
				!providerRequestBudgetConsumptionMatches(aggregate.ExecutionBudget.Consumptions, allocation, fact) {
				return false
			}
		default:
			return false
		}
	}
	return matched > 0
}

func providerRequestBudgetConsumptionMatches(values []runtimecontract.RuntimeBudgetConsumptionFact, allocation runtimecontract.RuntimeBudgetAllocation, settlement RuntimeProviderBudgetSettlement) bool {
	confirmed, unknown := int64(1), int64(0)
	if settlement.Status == RuntimeProviderBudgetSettlementUnknown {
		confirmed, unknown = 0, allocation.ReservedAmount
	} else {
		switch allocation.Dimension {
		case runtimecontract.RuntimeBudgetDimensionProviderInputToken:
			if settlement.Usage == nil {
				confirmed, unknown = 0, allocation.ReservedAmount
			} else {
				confirmed = int64(settlement.Usage.InputTokens)
			}
		case runtimecontract.RuntimeBudgetDimensionProviderOutputToken:
			if settlement.Usage == nil {
				confirmed, unknown = 0, allocation.ReservedAmount
			} else {
				confirmed = int64(settlement.Usage.OutputTokens)
			}
		}
	}
	matched := 0
	for _, consumption := range values {
		consumption = consumption.Normalized()
		if consumption.AllocationID != allocation.AllocationID {
			continue
		}
		matched++
		if consumption.Owner != runtimecontract.RuntimeModuleLLM || consumption.LogicalConsumptionID != settlement.LogicalRequestID ||
			consumption.PhysicalConsumptionID != settlement.PhysicalCallID || consumption.ConfirmedAmount != confirmed || consumption.UnknownAmount != unknown ||
			consumption.OwnerResultRef != settlement.OwnerFactRef || consumption.OwnerResultHash != settlement.OwnerFactHash {
			return false
		}
	}
	return matched == 1
}

func providerRequestRecoveryDuplicateApplied(aggregate RuntimeAggregate, session runtimecontract.RuntimeCandidateRequestSession, physicalCallID string, fact RuntimeProviderRequestRecoveryFact) bool {
	if session.LastPhysicalCallID != physicalCallID || session.ProviderSubmitted {
		return false
	}
	if session.State == runtimecontract.RuntimeCandidateRequestSessionTerminal {
		if session.LastOutcome != fact.Terminal.RequestOutcome || !providerRequestTerminalApplied(aggregate, fact.Terminal) {
			return false
		}
		if fact.DocumentFraming != nil {
			return session.LastDocumentFraming != nil && session.LastDocumentFraming.Normalized() == fact.DocumentFraming.Normalized() && session.ExpectedDocument != nil && session.IncompleteOutcomes > session.ExpectedDocument.MaxRecoveryAttempts
		}
		return session.TransportOutcomes > 0 && aggregate.Facts.Recovery.Used(runtimecontract.RuntimeRecoveryBudgetProviderTransport) >= aggregate.Facts.RecoveryPolicy.Limit(runtimecontract.RuntimeRecoveryBudgetProviderTransport)
	}
	if fact.TransportOutcome != nil {
		return session.State == runtimecontract.RuntimeCandidateRequestSessionTransportPending && session.LastOutcome == fact.TransportOutcome.Normalized()
	}
	return session.State == runtimecontract.RuntimeCandidateRequestSessionRecoveryPending && session.LastDocumentFraming != nil && session.LastDocumentFraming.Normalized() == fact.DocumentFraming.Normalized()
}
