package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// ensureCandidateRequestSession CAS-creates the provider-I/O ledger for one
// sealed authority transaction. It intentionally allocates no candidate
// attempt: only a complete document may enter the candidate admission path.
type candidateRequestSessionEnsureResult struct {
	Aggregate        RuntimeAggregate
	Session          runtimecontract.RuntimeCandidateRequestSession
	AuthorityFailure *runtimecontract.FailureFact
}

func (c *planningTransactionComponent) ensureCandidateRequestSessionWithExpected(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, documentKind runtimecontract.RuntimeProviderDocumentKind, stablePrefixHash, manifestHash, correctionHash string, expected *runtimecontract.RuntimeExpectedDocumentContract) (candidateRequestSessionEnsureResult, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || strings.TrimSpace(stablePrefixHash) == "" || strings.TrimSpace(manifestHash) == "" || expected == nil {
		return candidateRequestSessionEnsureResult{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return candidateRequestSessionEnsureResult{}, err
	}
	if err := candidateRequestSessionActionAllowed(aggregate, documentKind); err != nil {
		return candidateRequestSessionEnsureResult{}, err
	}
	transaction := aggregate.Decision.AuthorityTransaction()
	if documentKind == runtimecontract.RuntimeProviderDocumentOperations &&
		len(candidateAttemptsForContract(aggregate.CandidateAttempts, workspaceID, runID, runtimecontract.StableAuthorityContractID(workspaceID, runID, transaction))) == 0 &&
		!operationRequestSessionExistsForAuthority(aggregate.CandidateRequestSessions, transaction) &&
		equivalentOwnerInfeasibilityExists(aggregate.CandidateAttempts, transaction.AuthorityInputID) {
		if strings.TrimSpace(correctionHash) != "" {
			return candidateRequestSessionEnsureResult{}, fmt.Errorf("%w: owner infeasibility cannot authorize correction attempt 1", common.ErrConflict)
		}
		failure := equivalentAuthorityNoProgressFailure(aggregate)
		return candidateRequestSessionEnsureResult{AuthorityFailure: &failure}, nil
	}
	session := runtimecontract.RuntimeCandidateRequestSession{
		DocumentKind: documentKind,
		Transaction:  transaction, StablePrefixHash: strings.TrimSpace(stablePrefixHash), ManifestHash: strings.TrimSpace(manifestHash),
		CorrectionHash: strings.TrimSpace(correctionHash), State: runtimecontract.RuntimeCandidateRequestSessionPrepared, NextSequence: 1,
	}
	session.LogicalRequestID = runtimecontract.StableRuntimeCandidateRequestSessionID(documentKind, workspaceID, runID, transaction, session.StablePrefixHash, session.ManifestHash, session.CorrectionHash)
	value := expected.Clone()
	value.SchemaVersion = runtimecontract.RuntimeExpectedDocumentSchemaCurrent
	value.DocumentKind = documentKind
	value.ManifestHash = session.ManifestHash
	value.LogicalRequestID = session.LogicalRequestID
	value.StablePrefixHash = session.StablePrefixHash
	value.Hash = runtimecontract.StableRuntimeExpectedDocumentHash(value)
	if err := value.Validate(); err != nil {
		return candidateRequestSessionEnsureResult{}, fmt.Errorf("%w: invalid expected document contract", common.ErrInvalidArgument)
	}
	session.ExpectedDocument = &value
	if idx, existing, found := runtimeCandidateRequestSessionByID(aggregate.CandidateRequestSessions, session.LogicalRequestID); found {
		if err := validateCurrentCandidateRequestSession(aggregate, existing); err != nil {
			return candidateRequestSessionEnsureResult{}, err
		}
		if existing.DocumentKind != session.DocumentKind || existing.StablePrefixHash != session.StablePrefixHash || existing.ManifestHash != session.ManifestHash || existing.CorrectionHash != session.CorrectionHash || existing.Transaction != session.Transaction || !sameRuntimeExpectedDocument(existing.ExpectedDocument, session.ExpectedDocument) {
			return candidateRequestSessionEnsureResult{}, fmt.Errorf("%w: runtime candidate request session identity conflicts", common.ErrConflict)
		}
		return candidateRequestSessionEnsureResult{Aggregate: aggregate, Session: aggregate.CandidateRequestSessions[idx].Clone()}, nil
	}
	expectedRevision := aggregate.StateRevision
	aggregate.CandidateRequestSessions = append(aggregate.CandidateRequestSessions, session)
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return candidateRequestSessionEnsureResult{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return candidateRequestSessionEnsureResult{}, err
	}
	return candidateRequestSessionEnsureResult{Aggregate: aggregate, Session: session.Clone()}, nil
}

func equivalentAuthorityNoProgressFailure(aggregate RuntimeAggregate) runtimecontract.FailureFact {
	transaction := aggregate.Decision.AuthorityTransaction()
	return runtimecontract.FailureFact{
		Class:     runtimecontract.FailureClassNoProgressExhausted,
		Code:      "equivalent_authority_no_progress",
		Signature: "equivalent-authority/" + transaction.AuthorityInputID,
		Message:   "unchanged authority inputs already produced the same owner infeasibility",
	}
}

func operationRequestSessionExistsForAuthority(sessions []runtimecontract.RuntimeCandidateRequestSession, transaction runtimecontract.AuthorityTransaction) bool {
	for _, session := range sessions {
		if session.DocumentKind == runtimecontract.RuntimeProviderDocumentOperations && runtimecontract.SameAuthorityContract(session.Transaction, transaction) {
			return true
		}
	}
	return false
}

func sameRuntimeExpectedDocument(left, right *runtimecontract.RuntimeExpectedDocumentContract) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Normalized().Hash == right.Normalized().Hash
}

// markCandidateRequestSubmitted durably records the physical-call identity
// before observing a provider outcome. A restart with this marker but without
// an outcome must be classified as submission-unknown by the drive; it must
// never create an attempt or resend a known call as a new candidate request.
func (c *planningTransactionComponent) markCandidateRequestSubmitted(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, logicalRequestID string, sequence int, physicalCallID string) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || strings.TrimSpace(logicalRequestID) == "" || sequence <= 0 || strings.TrimSpace(physicalCallID) == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if err := rejectRuntimeProgressAfterTermination(aggregate); err != nil {
		return RuntimeAggregate{}, err
	}
	idx, session, err := currentCandidateRequestSession(aggregate, logicalRequestID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	physicalCallID = strings.TrimSpace(physicalCallID)
	if session.NextSequence != sequence {
		return RuntimeAggregate{}, fmt.Errorf("%w: runtime candidate request sequence does not match session", common.ErrConflict)
	}
	if session.ProviderSubmitted {
		if session.LastPhysicalCallID == physicalCallID {
			return aggregate, nil
		}
		return RuntimeAggregate{}, fmt.Errorf("%w: runtime candidate request already has an unresolved physical call", common.ErrConflict)
	}
	if session.State != runtimecontract.RuntimeCandidateRequestSessionPrepared && session.State != runtimecontract.RuntimeCandidateRequestSessionRecoveryPending && session.State != runtimecontract.RuntimeCandidateRequestSessionTransportPending {
		return RuntimeAggregate{}, fmt.Errorf("%w: runtime candidate request session does not accept a physical call", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
	if err := reserveCandidateRequestExecutionBudget(&aggregate, session, sequence, physicalCallID); err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.CandidateRequestSessions[idx].ProviderSubmitted = true
	aggregate.CandidateRequestSessions[idx].LastPhysicalCallID = physicalCallID
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// completeCandidateRequestDocument commits only the fact that an opaque,
// complete document is now available in the Orchestrator-owned content store.
// Candidate attempt allocation remains a separate subsequent transition.
func (c *planningTransactionComponent) completeCandidateRequestDocument(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, logicalRequestID string, physicalCallID string, budgetSettlement ...*RuntimeProviderBudgetSettlement) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || strings.TrimSpace(logicalRequestID) == "" || strings.TrimSpace(physicalCallID) == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	idx, session, err := currentCandidateRequestSession(aggregate, logicalRequestID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	physicalCallID = strings.TrimSpace(physicalCallID)
	if session.State == runtimecontract.RuntimeCandidateRequestSessionDocumentComplete && session.LastPhysicalCallID == physicalCallID {
		settlement, settlementErr := candidateRequestBudgetSettlementForIdentity(session, budgetSettlement...)
		if settlementErr != nil {
			return RuntimeAggregate{}, settlementErr
		}
		if !providerRequestBudgetSettlementApplied(aggregate, settlement) {
			return RuntimeAggregate{}, fmt.Errorf("%w: duplicate complete candidate document supplied a non-durable budget settlement", common.ErrConflict)
		}
		return aggregate, nil
	}
	if !session.ProviderSubmitted || session.LastPhysicalCallID != physicalCallID {
		return RuntimeAggregate{}, fmt.Errorf("%w: complete candidate document does not match submitted call", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
	settlement, err := candidateRequestBudgetSettlementForTransition(session, budgetSettlement...)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if _, err := applyCandidateRequestBudgetSettlement(&aggregate, settlement); err != nil {
		return RuntimeAggregate{}, err
	}
	updated := &aggregate.CandidateRequestSessions[idx]
	updated.State = runtimecontract.RuntimeCandidateRequestSessionDocumentComplete
	updated.LastOutcome = runtimecontract.RuntimeCandidateRequestOutcome{Class: runtimecontract.RuntimeCandidateRequestOutcomeDocumentComplete}
	updated.LastDocumentFraming = nil
	updated.ProviderSubmitted = true
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// rejectCandidateRequestDocumentConformance settles one authoritative LLM
// framing fact without turning an empty/invalid body into a provider protocol
// terminal. The durable session is the restart fence; the existing SM-3
// candidate/semantic attempt 0/1 ledger decides correction versus exhaustion.
func (c *planningTransactionComponent) rejectCandidateRequestDocumentConformance(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, logicalRequestID, physicalCallID string, framing runtimecontract.RuntimeProviderDocumentFraming, budgetSettlement ...*RuntimeProviderBudgetSettlement) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || strings.TrimSpace(logicalRequestID) == "" || strings.TrimSpace(physicalCallID) == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	framing = framing.Normalized()
	if err := framing.Validate(); err != nil || (framing.Disposition != runtimecontract.RuntimeProviderDocumentInvalid && framing.Disposition != runtimecontract.RuntimeProviderDocumentEmpty) {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	idx, session, found := runtimeCandidateRequestSessionByID(aggregate.CandidateRequestSessions, logicalRequestID)
	if !found {
		return RuntimeAggregate{}, fmt.Errorf("%w: runtime candidate request session not found", common.ErrNotFound)
	}
	physicalCallID = strings.TrimSpace(physicalCallID)
	settlement, err := candidateRequestBudgetSettlementForIdentity(session, budgetSettlement...)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if settlement.Status != RuntimeProviderBudgetSettlementConfirmed {
		return RuntimeAggregate{}, fmt.Errorf("%w: document conformance result requires confirmed physical submission", common.ErrConflict)
	}
	if session.State == runtimecontract.RuntimeCandidateRequestSessionDocumentConformanceRejected {
		if session.LastPhysicalCallID == physicalCallID && session.LastDocumentFraming != nil && session.LastDocumentFraming.Normalized() == framing && providerRequestBudgetSettlementApplied(aggregate, settlement) {
			return aggregate, nil
		}
		return RuntimeAggregate{}, fmt.Errorf("%w: conflicting candidate document conformance fact", common.ErrConflict)
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
	if session.State == runtimecontract.RuntimeCandidateRequestSessionDocumentComplete || session.State == runtimecontract.RuntimeCandidateRequestSessionTerminal {
		return RuntimeAggregate{}, fmt.Errorf("%w: closed provider request cannot accept a document conformance rejection", common.ErrConflict)
	}
	if !session.ProviderSubmitted || session.LastPhysicalCallID != physicalCallID || session.ExpectedDocument == nil || framing.ExpectedDocumentHash != session.ExpectedDocument.Hash {
		return RuntimeAggregate{}, fmt.Errorf("%w: document conformance result does not match submitted expected document", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
	if _, err := applyCandidateRequestBudgetSettlement(&aggregate, settlement); err != nil {
		return RuntimeAggregate{}, err
	}
	updated := &aggregate.CandidateRequestSessions[idx]
	updated.State = runtimecontract.RuntimeCandidateRequestSessionDocumentConformanceRejected
	updated.LastOutcome = runtimecontract.RuntimeCandidateRequestOutcome{
		Class: runtimecontract.RuntimeCandidateRequestOutcomeDocumentConformanceRejected,
		Code:  framing.Code, Signature: framing.Signature,
	}.Normalized()
	updated.LastDocumentFraming = &framing
	updated.ProviderSubmitted = true
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func firstRuntimeProviderBudgetSettlement(values []*RuntimeProviderBudgetSettlement) *RuntimeProviderBudgetSettlement {
	if len(values) == 0 {
		return nil
	}
	return values[0]
}

// candidateRequestBudgetSettlementForTransition makes every submitted provider
// authorization leave one terminal reservation fact. Production supplies the
// exact owner-derived settlement. Absence of a trusted owner fact is always
// unknown; it is never inferred as confirmed from a response shape or bytes.
func candidateRequestBudgetSettlementForTransition(session runtimecontract.RuntimeCandidateRequestSession, values ...*RuntimeProviderBudgetSettlement) (*RuntimeProviderBudgetSettlement, error) {
	session = session.Clone()
	if !session.ProviderSubmitted {
		if firstRuntimeProviderBudgetSettlement(values) != nil {
			return nil, fmt.Errorf("%w: provider budget settlement has no submitted call", common.ErrConflict)
		}
		return nil, nil
	}
	if session.LogicalRequestID == "" || session.LastPhysicalCallID == "" {
		return nil, fmt.Errorf("%w: submitted candidate request has incomplete budget identity", common.ErrConflict)
	}
	return candidateRequestBudgetSettlementForIdentity(session, values...)
}

func candidateRequestBudgetSettlementForIdentity(session runtimecontract.RuntimeCandidateRequestSession, values ...*RuntimeProviderBudgetSettlement) (*RuntimeProviderBudgetSettlement, error) {
	session = session.Clone()
	provided := firstRuntimeProviderBudgetSettlement(values)
	if provided == nil {
		settlement, err := NewRuntimeProviderBudgetUnknownSettlement(session.LogicalRequestID, session.LastPhysicalCallID)
		if err != nil {
			return nil, err
		}
		return &settlement, nil
	}
	settlement := provided.Clone()
	if err := settlement.Validate(); err != nil {
		return nil, err
	}
	if settlement.LogicalRequestID != session.LogicalRequestID || settlement.PhysicalCallID != session.LastPhysicalCallID {
		return nil, fmt.Errorf("%w: provider budget settlement does not match submitted candidate request", common.ErrConflict)
	}
	return &settlement, nil
}

func candidateRequestSessionActionAllowed(aggregate RuntimeAggregate, documentKind runtimecontract.RuntimeProviderDocumentKind) error {
	if err := rejectRuntimeProgressAfterTermination(aggregate); err != nil {
		return err
	}
	if aggregate.ActiveDispatch != nil {
		return fmt.Errorf("%w: current runtime authority does not accept a candidate request", common.ErrConflict)
	}
	if documentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan {
		// Catalog retirement can make the binding-free reducer view terminal
		// before the next semantic generation has sealed the current CWI. That
		// provisional SM-2 view is not an SM-4 terminal and may open only a
		// semantic-plan request. If an execution-locked binding still makes
		// operation authority usable, the terminal is already authoritative.
		if isTerminalConvergenceState(aggregate.Decision.State) {
			if aggregate.Decision.Terminal == nil || aggregate.Decision.Terminal.Code != "target_grounding_unresolved" || SemanticOperationAuthorityReady(aggregate) {
				return fmt.Errorf("%w: current runtime authority does not accept a candidate request", common.ErrConflict)
			}
		}
		switch aggregate.SemanticPlanning.Phase {
		case runtimecontract.RuntimeSemanticPlanningRequired, runtimecontract.RuntimeSemanticPlanningOrderRequestOpen, runtimecontract.RuntimeSemanticPlanningRequestOpen, runtimecontract.RuntimeSemanticPlanningOrderAccepted, runtimecontract.RuntimeSemanticPlanningCorrectionRequired, runtimecontract.RuntimeSemanticPlanningProofBatchRequestOpen, runtimecontract.RuntimeSemanticPlanningProofBatchDocumentAccepted:
			return nil
		default:
			return fmt.Errorf("%w: current semantic phase does not accept a provider request", common.ErrConflict)
		}
	}
	if aggregate.Decision.Terminal != nil || isTerminalConvergenceState(aggregate.Decision.State) {
		return fmt.Errorf("%w: current runtime authority does not accept a candidate request", common.ErrConflict)
	}
	if documentKind != runtimecontract.RuntimeProviderDocumentOperations || !SemanticOperationAuthorityReady(aggregate) {
		return fmt.Errorf("%w: operations request requires a sealed semantic contract", common.ErrConflict)
	}
	switch aggregate.Decision.State {
	case runtimecontract.ConvergenceStateNeedStateChange, runtimecontract.ConvergenceStateNeedValidation, runtimecontract.ConvergenceStateNeedAssessment, runtimecontract.ConvergenceStateNeedGovernance:
		return nil
	default:
		return fmt.Errorf("%w: current convergence state does not request a candidate", common.ErrConflict)
	}
}

// validateCandidateAttemptSessionBinding is the SM-3 identity fence between a
// complete provider document and a candidate attempt. The manifest remains a
// pure authority projection; this function only proves that the caller is
// using the persisted document/session that carried that exact projection.
func validateCandidateAttemptSessionBinding(aggregate RuntimeAggregate, stablePrefixHash string, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string) error {
	return validateCandidateAttemptSessionBindingState(aggregate, stablePrefixHash, manifest, logicalRequestID, false)
}

func validateCandidateAttemptRejectionSessionBinding(aggregate RuntimeAggregate, stablePrefixHash string, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string) error {
	return validateCandidateAttemptSessionBindingState(aggregate, stablePrefixHash, manifest, logicalRequestID, true)
}

func validateCandidateAttemptSessionBindingState(aggregate RuntimeAggregate, stablePrefixHash string, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string, allowConformanceRejected bool) error {
	if err := runtimecontract.ValidateRuntimeCandidateContractManifestForDecision(manifest, aggregate.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(runtimecontract.RuntimeCandidateContextForDecision(aggregate.Facts, aggregate.Decision, nil))); err != nil {
		return fmt.Errorf("%w: candidate manifest does not match current sealed authority: %v", common.ErrConflict, err)
	}
	_, session, found := runtimeCandidateRequestSessionByID(aggregate.CandidateRequestSessions, logicalRequestID)
	if !found {
		return fmt.Errorf("%w: candidate request session is missing", common.ErrConflict)
	}
	if err := validateCurrentCandidateRequestSession(aggregate, session); err != nil {
		return err
	}
	stateAllowed := session.State == runtimecontract.RuntimeCandidateRequestSessionDocumentComplete
	if allowConformanceRejected {
		stateAllowed = stateAllowed || session.State == runtimecontract.RuntimeCandidateRequestSessionDocumentConformanceRejected
	}
	if !stateAllowed || session.ManifestHash != manifest.Hash || (strings.TrimSpace(stablePrefixHash) != "" && session.StablePrefixHash != strings.TrimSpace(stablePrefixHash)) {
		return fmt.Errorf("%w: candidate request session is not the complete document for this manifest", common.ErrConflict)
	}
	if session.State == runtimecontract.RuntimeCandidateRequestSessionDocumentConformanceRejected &&
		(session.DocumentKind != runtimecontract.RuntimeProviderDocumentOperations || session.LastDocumentFraming == nil) {
		return fmt.Errorf("%w: candidate request session has no operations document-conformance fact", common.ErrConflict)
	}
	return nil
}

func currentCandidateRequestSession(aggregate RuntimeAggregate, logicalRequestID string) (int, runtimecontract.RuntimeCandidateRequestSession, error) {
	idx, session, found := runtimeCandidateRequestSessionByID(aggregate.CandidateRequestSessions, logicalRequestID)
	if !found {
		return 0, runtimecontract.RuntimeCandidateRequestSession{}, fmt.Errorf("%w: runtime candidate request session not found", common.ErrNotFound)
	}
	if err := candidateRequestSessionActionAllowed(aggregate, session.DocumentKind); err != nil {
		return 0, runtimecontract.RuntimeCandidateRequestSession{}, err
	}
	if err := validateCurrentCandidateRequestSession(aggregate, session); err != nil {
		return 0, runtimecontract.RuntimeCandidateRequestSession{}, err
	}
	return idx, session, nil
}

func validateCurrentCandidateRequestSession(aggregate RuntimeAggregate, session runtimecontract.RuntimeCandidateRequestSession) error {
	session = session.Normalized()
	if err := session.Validate(); err != nil {
		return err
	}
	transaction := aggregate.Decision.AuthorityTransaction()
	if session.ExpectedDocument == nil || session.Transaction != transaction || session.LogicalRequestID != runtimecontract.StableRuntimeCandidateRequestSessionID(session.DocumentKind, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, transaction, session.StablePrefixHash, session.ManifestHash, session.CorrectionHash) {
		return fmt.Errorf("%w: runtime candidate request session does not match current authority", common.ErrConflict)
	}
	if session.DocumentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan {
		ledger := aggregate.SemanticPlanning.Normalized()
		if ledger.Manifest == nil || ledger.Manifest.Hash != session.ManifestHash || ledger.StablePrefixHash != session.StablePrefixHash {
			return fmt.Errorf("%w: semantic request session does not match planning authority", common.ErrConflict)
		}
	}
	return nil
}

func runtimeCandidateRequestSessionByID(values []runtimecontract.RuntimeCandidateRequestSession, logicalRequestID string) (int, runtimecontract.RuntimeCandidateRequestSession, bool) {
	for idx, session := range values {
		if session.Normalized().LogicalRequestID == strings.TrimSpace(logicalRequestID) {
			return idx, session.Normalized(), true
		}
	}
	return 0, runtimecontract.RuntimeCandidateRequestSession{}, false
}
