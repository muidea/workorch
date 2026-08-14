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
func (c *RuntimeController) ensureCandidateRequestSessionWithExpected(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, documentKind runtimecontract.RuntimeProviderDocumentKind, stablePrefixHash, manifestHash, correctionHash string, expected *runtimecontract.RuntimeExpectedDocumentContract) (RuntimeAggregate, runtimecontract.RuntimeCandidateRequestSession, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || strings.TrimSpace(stablePrefixHash) == "" || strings.TrimSpace(manifestHash) == "" || expected == nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeCandidateRequestSession{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeCandidateRequestSession{}, err
	}
	if err := candidateRequestSessionActionAllowed(aggregate, documentKind); err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeCandidateRequestSession{}, err
	}
	transaction := aggregate.Decision.AuthorityTransaction()
	if documentKind == runtimecontract.RuntimeProviderDocumentOperations &&
		len(candidateAttemptsForContract(aggregate.CandidateAttempts, workspaceID, runID, runtimecontract.StableAuthorityContractID(workspaceID, runID, transaction))) == 0 &&
		!operationRequestSessionExistsForAuthority(aggregate.CandidateRequestSessions, transaction) &&
		equivalentOwnerInfeasibilityExists(aggregate.CandidateAttempts, transaction.AuthorityInputID) {
		if strings.TrimSpace(correctionHash) != "" {
			return RuntimeAggregate{}, runtimecontract.RuntimeCandidateRequestSession{}, fmt.Errorf("%w: owner infeasibility cannot authorize correction attempt 1", common.ErrConflict)
		}
		terminal, terminalErr := c.terminalizeEquivalentAuthorityNoProgress(ctx, aggregate)
		return terminal, runtimecontract.RuntimeCandidateRequestSession{}, terminalErr
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
		return RuntimeAggregate{}, runtimecontract.RuntimeCandidateRequestSession{}, fmt.Errorf("%w: invalid expected document contract", common.ErrInvalidArgument)
	}
	session.ExpectedDocument = &value
	if idx, existing, found := runtimeCandidateRequestSessionByID(aggregate.CandidateRequestSessions, session.LogicalRequestID); found {
		if err := validateCurrentCandidateRequestSession(aggregate, existing); err != nil {
			return RuntimeAggregate{}, runtimecontract.RuntimeCandidateRequestSession{}, err
		}
		if existing.DocumentKind != session.DocumentKind || existing.StablePrefixHash != session.StablePrefixHash || existing.ManifestHash != session.ManifestHash || existing.CorrectionHash != session.CorrectionHash || existing.Transaction != session.Transaction || !sameRuntimeExpectedDocument(existing.ExpectedDocument, session.ExpectedDocument) {
			return RuntimeAggregate{}, runtimecontract.RuntimeCandidateRequestSession{}, fmt.Errorf("%w: runtime candidate request session identity conflicts", common.ErrConflict)
		}
		return aggregate, aggregate.CandidateRequestSessions[idx].Clone(), nil
	}
	expectedRevision := aggregate.StateRevision
	aggregate.CandidateRequestSessions = append(aggregate.CandidateRequestSessions, session)
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeCandidateRequestSession{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeCandidateRequestSession{}, err
	}
	return aggregate, session.Clone(), nil
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
func (c *RuntimeController) markCandidateRequestSubmitted(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, logicalRequestID string, sequence int, physicalCallID string) (RuntimeAggregate, error) {
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

// recordCandidateRequestTransportOutcome preserves transient provider failure
// separately from candidate attempts and repair. The next physical call uses
// the same logical ID and the same sequence after recovery budget accounting.
func (c *RuntimeController) recordCandidateRequestTransportOutcome(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, logicalRequestID string, physicalCallID string, outcome runtimecontract.RuntimeCandidateRequestOutcome) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || strings.TrimSpace(logicalRequestID) == "" || strings.TrimSpace(physicalCallID) == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	outcome = outcome.Normalized()
	if err := outcome.Validate(); err != nil || outcome.Class != runtimecontract.RuntimeCandidateRequestOutcomeProviderTransient {
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
	if session.State == runtimecontract.RuntimeCandidateRequestSessionTransportPending && !session.ProviderSubmitted && session.LastOutcome == outcome {
		return aggregate, nil
	}
	if !session.ProviderSubmitted || session.LastPhysicalCallID != physicalCallID {
		return RuntimeAggregate{}, fmt.Errorf("%w: provider transport outcome does not match submitted call", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
	updated := &aggregate.CandidateRequestSessions[idx]
	updated.State = runtimecontract.RuntimeCandidateRequestSessionTransportPending
	updated.LastOutcome = outcome
	updated.ProviderSubmitted = false
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// recordCandidateRequestRecoveryRequired persists an LLM-owner incomplete
// document fact as SM-4 recovery authority. The next call starts the same
// sealed expected document from the beginning; no raw suffix is retained or
// assembled into Runtime authority.
func (c *RuntimeController) recordCandidateRequestRecoveryRequired(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, logicalRequestID, physicalCallID string, outcome runtimecontract.RuntimeProviderDocumentOutcome) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || strings.TrimSpace(logicalRequestID) == "" || strings.TrimSpace(physicalCallID) == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	outcome = outcome.Normalized()
	if err := outcome.Validate(); err != nil || outcome.Disposition != runtimecontract.RuntimeProviderDocumentIncomplete {
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
	if session.State == runtimecontract.RuntimeCandidateRequestSessionRecoveryPending && !session.ProviderSubmitted && session.LastPhysicalCallID == physicalCallID && session.LastDocumentOutcome != nil && session.LastDocumentOutcome.Normalized() == outcome {
		return aggregate, nil
	}
	if session.State != runtimecontract.RuntimeCandidateRequestSessionPrepared && session.State != runtimecontract.RuntimeCandidateRequestSessionRecoveryPending && session.State != runtimecontract.RuntimeCandidateRequestSessionTransportPending {
		return RuntimeAggregate{}, fmt.Errorf("%w: runtime candidate request session does not accept an incomplete provider outcome", common.ErrConflict)
	}
	if session.ExpectedDocument == nil || outcome.ExpectedDocumentHash != session.ExpectedDocument.Hash || !session.ProviderSubmitted || session.LastPhysicalCallID != physicalCallID {
		return RuntimeAggregate{}, fmt.Errorf("%w: incomplete provider document does not match submitted expected document", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
	updated := &aggregate.CandidateRequestSessions[idx]
	updated.State = runtimecontract.RuntimeCandidateRequestSessionRecoveryPending
	updated.NextSequence++
	updated.IncompleteOutcomes++
	updated.LastOutcome = runtimecontract.RuntimeCandidateRequestOutcome{Class: runtimecontract.RuntimeCandidateRequestOutcomeDocumentIncomplete, Code: outcome.Code, Signature: outcome.Signature}
	updated.LastDocumentOutcome = &outcome
	updated.ProviderSubmitted = false
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
func (c *RuntimeController) completeCandidateRequestDocument(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, logicalRequestID string, physicalCallID string) (RuntimeAggregate, error) {
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
		return aggregate, nil
	}
	if !session.ProviderSubmitted || session.LastPhysicalCallID != physicalCallID {
		return RuntimeAggregate{}, fmt.Errorf("%w: complete candidate document does not match submitted call", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
	updated := &aggregate.CandidateRequestSessions[idx]
	updated.State = runtimecontract.RuntimeCandidateRequestSessionDocumentComplete
	updated.LastOutcome = runtimecontract.RuntimeCandidateRequestOutcome{Class: runtimecontract.RuntimeCandidateRequestOutcomeDocumentComplete}
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

// terminateCandidateRequestSession closes non-candidate provider outcomes.
// The caller separately records the typed SM-2 failure fact; this method never
// reduces authority, changes a budget, or opens governance by itself.
func (c *RuntimeController) terminateCandidateRequestSession(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, logicalRequestID string, outcome runtimecontract.RuntimeCandidateRequestOutcome) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || strings.TrimSpace(logicalRequestID) == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	outcome = outcome.Normalized()
	if err := outcome.Validate(); err != nil || !candidateRequestTerminalOutcome(outcome.Class) {
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
	if session.State == runtimecontract.RuntimeCandidateRequestSessionTerminal && session.LastOutcome == outcome {
		return aggregate, nil
	}
	if session.State == runtimecontract.RuntimeCandidateRequestSessionDocumentComplete || session.State == runtimecontract.RuntimeCandidateRequestSessionTerminal {
		return RuntimeAggregate{}, fmt.Errorf("%w: runtime candidate request session is already closed", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
	aggregate.CandidateRequestSessions[idx].State = runtimecontract.RuntimeCandidateRequestSessionTerminal
	aggregate.CandidateRequestSessions[idx].LastOutcome = outcome
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func candidateRequestSessionActionAllowed(aggregate RuntimeAggregate, documentKind runtimecontract.RuntimeProviderDocumentKind) error {
	if err := rejectRuntimeProgressAfterTermination(aggregate); err != nil {
		return err
	}
	if aggregate.ActiveDispatch != nil || aggregate.Decision.Terminal != nil || isTerminalConvergenceState(aggregate.Decision.State) {
		return fmt.Errorf("%w: current runtime authority does not accept a candidate request", common.ErrConflict)
	}
	if documentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan {
		switch aggregate.SemanticPlanning.Phase {
		case runtimecontract.RuntimeSemanticPlanningRequired, runtimecontract.RuntimeSemanticPlanningOrderRequestOpen, runtimecontract.RuntimeSemanticPlanningRequestOpen, runtimecontract.RuntimeSemanticPlanningOrderAccepted, runtimecontract.RuntimeSemanticPlanningCorrectionRequired, runtimecontract.RuntimeSemanticPlanningProofBatchRequestOpen, runtimecontract.RuntimeSemanticPlanningProofBatchDocumentAccepted:
			return nil
		default:
			return fmt.Errorf("%w: current semantic phase does not accept a provider request", common.ErrConflict)
		}
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
	if session.State != runtimecontract.RuntimeCandidateRequestSessionDocumentComplete || session.ManifestHash != manifest.Hash || (strings.TrimSpace(stablePrefixHash) != "" && session.StablePrefixHash != strings.TrimSpace(stablePrefixHash)) {
		return fmt.Errorf("%w: candidate request session is not the complete document for this manifest", common.ErrConflict)
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

func candidateRequestTerminalOutcome(class runtimecontract.RuntimeCandidateRequestOutcomeClass) bool {
	switch class {
	case runtimecontract.RuntimeCandidateRequestOutcomeProviderAuthentication,
		runtimecontract.RuntimeCandidateRequestOutcomeProviderConfiguration,
		runtimecontract.RuntimeCandidateRequestOutcomeProviderProtocol,
		runtimecontract.RuntimeCandidateRequestOutcomeProviderSubmissionUnknown,
		runtimecontract.RuntimeCandidateRequestOutcomePrerequisiteStale,
		runtimecontract.RuntimeCandidateRequestOutcomeDocumentRecoveryExhausted:
		return true
	default:
		return false
	}
}
