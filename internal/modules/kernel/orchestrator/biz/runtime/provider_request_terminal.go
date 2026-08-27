package runtime

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// RuntimeProviderRequestTerminalInput is an Orchestrator-private normalization
// input. It carries the already validated LLM fact but contains no provider
// body and no requested Runtime action.
type RuntimeProviderRequestTerminalInput struct {
	DocumentKind       runtimecontract.RuntimeProviderDocumentKind
	OutcomeClass       runtimecontract.RuntimeCandidateRequestOutcomeClass
	TerminalCode       string
	TerminalSignature  string
	Message            string
	OriginalDiagnostic *runtimecontract.RuntimeOriginalDiagnostic
}

// RuntimeProviderRequestTerminalFact is the deterministic successor shared by
// semantic-plan and operations provider requests. RequestOutcome closes SM-3;
// Failure is the exact SM-2 fact applied in the same aggregate CAS.
type RuntimeProviderRequestTerminalFact struct {
	DocumentKind   runtimecontract.RuntimeProviderDocumentKind
	TerminalCode   string
	RequestOutcome runtimecontract.RuntimeCandidateRequestOutcome
	Failure        runtimecontract.FailureFact
}

// NormalizeRuntimeProviderRequestTerminal is the sole provider-terminal
// normalizer for both semantic-plan and operations documents. Producer-authored
// diagnostics are cloned without rewriting; the bounded request outcome keeps
// the producer's exact code/signature while the SM-2 failure retains its stable
// Orchestrator classification.
func NormalizeRuntimeProviderRequestTerminal(input RuntimeProviderRequestTerminalInput) (RuntimeProviderRequestTerminalFact, error) {
	input.DocumentKind = runtimecontract.RuntimeProviderDocumentKind(strings.TrimSpace(string(input.DocumentKind)))
	input.TerminalCode = strings.TrimSpace(input.TerminalCode)
	input.TerminalSignature = strings.TrimSpace(input.TerminalSignature)
	input.Message = strings.TrimSpace(input.Message)
	if input.DocumentKind != runtimecontract.RuntimeProviderDocumentSemanticPlan && input.DocumentKind != runtimecontract.RuntimeProviderDocumentOperations {
		return RuntimeProviderRequestTerminalFact{}, common.ErrInvalidArgument
	}
	if input.TerminalCode == "" || input.Message == "" {
		return RuntimeProviderRequestTerminalFact{}, common.ErrInvalidArgument
	}
	if input.TerminalSignature == "" {
		input.TerminalSignature = strings.ReplaceAll(input.TerminalCode, "_", "-")
	}
	original := input.OriginalDiagnostic.CloneOriginal()
	if original != nil {
		if err := original.Validate(); err != nil {
			return RuntimeProviderRequestTerminalFact{}, err
		}
	}
	outcomeClass := providerRequestTerminalOutcomeClass(input.OutcomeClass, input.TerminalCode, original)
	if !providerRequestTerminalOutcome(outcomeClass) {
		return RuntimeProviderRequestTerminalFact{}, common.ErrInvalidArgument
	}
	outcomeCode, outcomeSignature := input.TerminalCode, input.TerminalSignature
	if original != nil {
		outcomeCode = original.OriginalCode
		outcomeSignature = original.Signature
	}
	outcome := runtimecontract.RuntimeCandidateRequestOutcome{Class: outcomeClass, Code: outcomeCode, Signature: outcomeSignature}.Normalized()
	if err := outcome.Validate(); err != nil || outcome.Code == "" || outcome.Signature == "" {
		return RuntimeProviderRequestTerminalFact{}, common.ErrInvalidArgument
	}

	fact := RuntimeProviderRequestTerminalFact{
		DocumentKind:   input.DocumentKind,
		TerminalCode:   input.TerminalCode,
		RequestOutcome: outcome,
	}
	if input.DocumentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan {
		if original == nil {
			original = &runtimecontract.RuntimeOriginalDiagnostic{
				SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
				OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator,
				OriginalClass: providerRequestTerminalOriginalClass(outcomeClass),
				OriginalCode:  input.TerminalCode, OriginalMessage: input.Message, Signature: input.TerminalSignature,
			}
		}
		if err := original.Validate(); err != nil {
			return RuntimeProviderRequestTerminalFact{}, err
		}
		fact.Failure = semanticContractFailureFact(input.TerminalCode, *original)
	} else {
		failureClass, ok := providerRequestTerminalFailureClass(outcomeClass)
		if !ok {
			return RuntimeProviderRequestTerminalFact{}, common.ErrInvalidArgument
		}
		fact.Failure = runtimecontract.FailureFact{
			Class: failureClass, Code: input.TerminalCode, Signature: input.TerminalSignature,
			Message: input.Message, OriginalDiagnostic: original,
		}
	}
	if err := fact.Validate(); err != nil {
		return RuntimeProviderRequestTerminalFact{}, err
	}
	return fact, nil
}

func (f RuntimeProviderRequestTerminalFact) Validate() error {
	f.DocumentKind = runtimecontract.RuntimeProviderDocumentKind(strings.TrimSpace(string(f.DocumentKind)))
	f.TerminalCode = strings.TrimSpace(f.TerminalCode)
	f.RequestOutcome = f.RequestOutcome.Normalized()
	f.Failure = f.Failure.Normalized()
	if (f.DocumentKind != runtimecontract.RuntimeProviderDocumentSemanticPlan && f.DocumentKind != runtimecontract.RuntimeProviderDocumentOperations) || f.TerminalCode == "" || !providerRequestTerminalOutcome(f.RequestOutcome.Class) {
		return common.ErrInvalidArgument
	}
	if err := f.RequestOutcome.Validate(); err != nil || f.RequestOutcome.Code == "" || f.RequestOutcome.Signature == "" || f.Failure.Class == "" || f.Failure.Code == "" || f.Failure.Signature == "" || f.Failure.Message == "" {
		return common.ErrInvalidArgument
	}
	if f.Failure.OriginalDiagnostic != nil {
		if err := f.Failure.OriginalDiagnostic.Validate(); err != nil {
			return err
		}
	}
	if f.DocumentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan {
		if f.Failure.Code != "semantic_contract_failed" || f.Failure.OriginalDiagnostic == nil {
			return common.ErrInvalidArgument
		}
		expected := semanticContractFailureFact(f.TerminalCode, *f.Failure.OriginalDiagnostic)
		if !sameRuntimeFailureFact(expected, f.Failure) {
			return common.ErrInvalidArgument
		}
		return nil
	}
	expectedClass, ok := providerRequestTerminalFailureClass(f.RequestOutcome.Class)
	if !ok || f.Failure.Class != expectedClass {
		return common.ErrInvalidArgument
	}
	return nil
}

func providerRequestTerminalOutcomeClass(explicit runtimecontract.RuntimeCandidateRequestOutcomeClass, code string, original *runtimecontract.RuntimeOriginalDiagnostic) runtimecontract.RuntimeCandidateRequestOutcomeClass {
	if explicit != runtimecontract.RuntimeCandidateRequestOutcomeNone {
		return explicit
	}
	if original != nil {
		switch runtimecontract.RuntimeCandidateFailureClass(strings.TrimSpace(original.OriginalClass)) {
		case runtimecontract.RuntimeCandidateFailureClassProviderAuthentication:
			return runtimecontract.RuntimeCandidateRequestOutcomeProviderAuthentication
		case runtimecontract.RuntimeCandidateFailureClassProviderConfiguration:
			return runtimecontract.RuntimeCandidateRequestOutcomeProviderConfiguration
		}
	}
	switch strings.TrimSpace(code) {
	case "provider_submission_unknown", "semantic_provider_submission_unknown":
		return runtimecontract.RuntimeCandidateRequestOutcomeProviderSubmissionUnknown
	case "candidate_document_recovery_exhausted", "semantic_document_recovery_exhausted":
		return runtimecontract.RuntimeCandidateRequestOutcomeDocumentRecoveryExhausted
	default:
		return runtimecontract.RuntimeCandidateRequestOutcomeProviderProtocol
	}
}

func providerRequestTerminalOutcome(class runtimecontract.RuntimeCandidateRequestOutcomeClass) bool {
	switch class {
	case runtimecontract.RuntimeCandidateRequestOutcomeProviderAuthentication,
		runtimecontract.RuntimeCandidateRequestOutcomeProviderConfiguration,
		runtimecontract.RuntimeCandidateRequestOutcomeProviderTransient,
		runtimecontract.RuntimeCandidateRequestOutcomeProviderProtocol,
		runtimecontract.RuntimeCandidateRequestOutcomeProviderSubmissionUnknown,
		runtimecontract.RuntimeCandidateRequestOutcomeDocumentRecoveryExhausted:
		return true
	default:
		return false
	}
}

func providerRequestTerminalFailureClass(class runtimecontract.RuntimeCandidateRequestOutcomeClass) (runtimecontract.FailureClass, bool) {
	switch class {
	case runtimecontract.RuntimeCandidateRequestOutcomeProviderAuthentication:
		return runtimecontract.FailureClassProviderAuthentication, true
	case runtimecontract.RuntimeCandidateRequestOutcomeProviderConfiguration:
		return runtimecontract.FailureClassProviderConfiguration, true
	case runtimecontract.RuntimeCandidateRequestOutcomeProviderTransient:
		return runtimecontract.FailureClassProviderTransient, true
	case runtimecontract.RuntimeCandidateRequestOutcomeProviderProtocol:
		return runtimecontract.FailureClassProviderProtocol, true
	case runtimecontract.RuntimeCandidateRequestOutcomeProviderSubmissionUnknown:
		return runtimecontract.FailureClassProviderSubmissionUnknown, true
	case runtimecontract.RuntimeCandidateRequestOutcomeDocumentRecoveryExhausted:
		return runtimecontract.FailureClassCandidateDocumentRecoveryExhausted, true
	default:
		return "", false
	}
}

func providerRequestTerminalOriginalClass(class runtimecontract.RuntimeCandidateRequestOutcomeClass) string {
	switch class {
	case runtimecontract.RuntimeCandidateRequestOutcomeProviderAuthentication:
		return string(runtimecontract.RuntimeCandidateFailureClassProviderAuthentication)
	case runtimecontract.RuntimeCandidateRequestOutcomeProviderConfiguration:
		return string(runtimecontract.RuntimeCandidateFailureClassProviderConfiguration)
	case runtimecontract.RuntimeCandidateRequestOutcomeProviderTransient:
		return string(runtimecontract.RuntimeCandidateFailureClassProviderTransient)
	case runtimecontract.RuntimeCandidateRequestOutcomeProviderSubmissionUnknown:
		return string(runtimecontract.RuntimeCandidateRequestOutcomeProviderSubmissionUnknown)
	case runtimecontract.RuntimeCandidateRequestOutcomeDocumentRecoveryExhausted:
		return string(runtimecontract.RuntimeCandidateRequestOutcomeDocumentRecoveryExhausted)
	default:
		return string(runtimecontract.RuntimeCandidateFailureClassProviderProtocol)
	}
}

// coordinateProviderRequestTerminal is the explicit SM-3 -> SM-2 owner-local
// transaction. The request close and its deterministic successor failure are
// validated and committed under one Runtime aggregate revision.
func (c *planningTransactionComponent) coordinateProviderRequestTerminal(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, logicalRequestID string, fact RuntimeProviderRequestTerminalFact, budgetSettlement ...*RuntimeProviderBudgetSettlement) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || strings.TrimSpace(logicalRequestID) == "" {
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
		return RuntimeAggregate{}, fmt.Errorf("%w: provider terminal document kind does not match request session", common.ErrConflict)
	}
	var settlement *RuntimeProviderBudgetSettlement
	providedSettlement := firstRuntimeProviderBudgetSettlement(budgetSettlement)
	if session.ProviderSubmitted {
		settlement, err = candidateRequestBudgetSettlementForTransition(session, budgetSettlement...)
		if err != nil {
			return RuntimeAggregate{}, err
		}
	} else if providedSettlement != nil {
		settlement, err = candidateRequestBudgetSettlementForIdentity(session, budgetSettlement...)
		if err != nil {
			return RuntimeAggregate{}, err
		}
		if !providerRequestBudgetSettlementApplied(aggregate, settlement) {
			return RuntimeAggregate{}, fmt.Errorf("%w: closed provider request supplied a non-durable budget settlement", common.ErrConflict)
		}
	} else if !providerRequestSessionBudgetSettled(aggregate, session) {
		return RuntimeAggregate{}, fmt.Errorf("%w: provider terminal has no settled physical request", common.ErrConflict)
	}
	if session.State == runtimecontract.RuntimeCandidateRequestSessionTerminal {
		if session.LastOutcome == fact.RequestOutcome && providerRequestTerminalApplied(aggregate, fact) && (settlement == nil || providerRequestBudgetSettlementApplied(aggregate, settlement)) {
			return aggregate, nil
		}
		return RuntimeAggregate{}, fmt.Errorf("%w: conflicting provider request terminal fact", common.ErrConflict)
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
		return RuntimeAggregate{}, fmt.Errorf("%w: complete provider request cannot accept a terminal failure", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
	if session.ProviderSubmitted {
		if _, err := applyCandidateRequestBudgetSettlement(&aggregate, settlement); err != nil {
			return RuntimeAggregate{}, err
		}
	}
	aggregate, err = applyProviderRequestTerminalToAggregate(aggregate, idx, fact)
	if err != nil {
		return RuntimeAggregate{}, err
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

// providerRequestSessionBudgetSettled proves that an owner-local successor
// decision (for example recovery exhaustion) is not inventing another
// physical outcome. Every reservation for the session's last physical call
// must already have left reserved state under the earlier coordinated CAS.
func providerRequestSessionBudgetSettled(aggregate RuntimeAggregate, session runtimecontract.RuntimeCandidateRequestSession) bool {
	session = session.Clone()
	if session.LogicalRequestID == "" || session.LastPhysicalCallID == "" {
		return false
	}
	matched := 0
	for _, allocation := range aggregate.ExecutionBudget.Allocations {
		allocation = allocation.Normalized()
		if allocation.LogicalConsumptionID != session.LogicalRequestID || allocation.ScopeRef != session.LastPhysicalCallID {
			continue
		}
		matched++
		if allocation.State == runtimecontract.RuntimeBudgetAllocationReserved {
			return false
		}
	}
	return matched > 0
}

// applyProviderRequestTerminalToAggregate is the pure terminal half shared by
// direct provider terminals and the recovery coordinator. Callers own request
// observation/budget settlement and commit the combined aggregate once.
func applyProviderRequestTerminalToAggregate(aggregate RuntimeAggregate, sessionIndex int, fact RuntimeProviderRequestTerminalFact) (RuntimeAggregate, error) {
	if sessionIndex < 0 || sessionIndex >= len(aggregate.CandidateRequestSessions) {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	if err := fact.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.CandidateRequestSessions[sessionIndex].DocumentKind != fact.DocumentKind {
		return RuntimeAggregate{}, fmt.Errorf("%w: provider terminal document kind does not match request session", common.ErrConflict)
	}
	aggregate.CandidateRequestSessions[sessionIndex].State = runtimecontract.RuntimeCandidateRequestSessionTerminal
	aggregate.CandidateRequestSessions[sessionIndex].LastOutcome = fact.RequestOutcome
	aggregate.CandidateRequestSessions[sessionIndex].ProviderSubmitted = false
	if fact.RequestOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomeDocumentRecoveryExhausted {
		aggregate.CandidateRequestSessions[sessionIndex].LastDocumentFraming = nil
	}
	if fact.DocumentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan {
		return applySemanticContractFailureToAggregate(aggregate, fact.TerminalCode, *fact.Failure.OriginalDiagnostic)
	}
	return applyFailureFactToAggregate(aggregate, fact.Failure, false)
}

func providerRequestTerminalApplied(aggregate RuntimeAggregate, fact RuntimeProviderRequestTerminalFact) bool {
	if !sameRuntimeFailureFactValue(aggregate.Facts.Failure, fact.Failure) {
		return false
	}
	if fact.DocumentKind != runtimecontract.RuntimeProviderDocumentSemanticPlan {
		return true
	}
	ledger := aggregate.SemanticPlanning.Normalized()
	return ledger.Phase == runtimecontract.RuntimeSemanticPlanningFailed &&
		reflect.DeepEqual(ledger.Failure.Normalized(), fact.Failure.OriginalDiagnostic.Normalized())
}

// ProviderRequestTerminalSessionApplied proves that a persisted terminal
// request still has its coordinated SM-2 successor after reload. The session
// intentionally carries only bounded request identity; exact producer detail
// remains in the failure fact/semantic ledger. Incomplete or torn shapes fail
// closed because they cannot be replayed without inventing that detail.
func ProviderRequestTerminalSessionApplied(aggregate RuntimeAggregate, session runtimecontract.RuntimeCandidateRequestSession) bool {
	session = session.Clone()
	if session.State != runtimecontract.RuntimeCandidateRequestSessionTerminal || !providerRequestTerminalOutcome(session.LastOutcome.Class) || aggregate.Facts.Failure == nil {
		return false
	}
	failure := aggregate.Facts.Failure.Normalized()
	if failure.OriginalDiagnostic != nil {
		diagnostic := failure.OriginalDiagnostic.Normalized()
		if diagnostic.OriginalCode != session.LastOutcome.Code || diagnostic.Signature != session.LastOutcome.Signature {
			return false
		}
	} else if failure.Code != session.LastOutcome.Code || failure.Signature != session.LastOutcome.Signature {
		return false
	}
	if session.DocumentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan {
		ledger := aggregate.SemanticPlanning.Normalized()
		return failure.Code == "semantic_contract_failed" && ledger.Phase == runtimecontract.RuntimeSemanticPlanningFailed &&
			failure.OriginalDiagnostic != nil && reflect.DeepEqual(ledger.Failure.Normalized(), failure.OriginalDiagnostic.Normalized())
	}
	expectedClass, ok := providerRequestTerminalFailureClass(session.LastOutcome.Class)
	return ok && failure.Class == expectedClass
}

func sameRuntimeFailureFact(left, right runtimecontract.FailureFact) bool {
	return reflect.DeepEqual(left.Normalized(), right.Normalized())
}

func sameRuntimeFailureFactValue(left *runtimecontract.FailureFact, right runtimecontract.FailureFact) bool {
	return left != nil && sameRuntimeFailureFact(*left, right)
}
