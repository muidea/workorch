package workorchruntimecontract

import (
	"fmt"
	"sync"
)

// RuntimeOwnerInvariantSourceGate supplies complete pure validators only during
// the synchronous owner continuation. It is not a session or a successful
// product: every method requires all original inputs and executes every join.
// A nil gate is the independent cold path. A captured non-nil gate is revoked
// on continuation return, including panic. No gate is retained across images.
// runtime-owner-invariant-source-compiler/v1 changes wiring, not the historical
// compiler's allowlist, entry limit or authority.
type RuntimeOwnerInvariantSourceGate struct {
	state *runtimeOwnerInvariantSourceGateState
}

// Copies of the public method surface share revocation; copying the struct
// cannot manufacture an independently live gate or retain the private session.
type runtimeOwnerInvariantSourceGateState struct {
	mu       sync.Mutex
	semantic *runtimeSemanticValidationSession
	closed   bool
	first    error
}

func (g *RuntimeOwnerInvariantSourceGate) validate(body func(*runtimeSemanticValidationSession) error) error {
	if g == nil {
		return body(newRuntimeSemanticValidationSession())
	}
	s := g.state
	if s == nil {
		return fmt.Errorf("runtime owner invariant source gate is closed")
	}
	if !s.mu.TryLock() {
		return fmt.Errorf("runtime owner invariant source gate is already running")
	}
	defer s.mu.Unlock()
	if s.closed || s.semantic == nil {
		return fmt.Errorf("runtime owner invariant source gate is closed")
	}
	if s.first == nil {
		s.first = body(s.semantic)
	}
	return s.first
}

func withRuntimeOwnerInvariantSourceGate(semantic *runtimeSemanticValidationSession, body func(*RuntimeOwnerInvariantSourceGate) error) error {
	s := &runtimeOwnerInvariantSourceGateState{semantic: semantic}
	g := &RuntimeOwnerInvariantSourceGate{state: s}
	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.closed, s.semantic = true, nil
	}()
	err := body(g)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		return err // Preserve the owner's original contextual error prefix.
	}
	return s.first // Ignoring a failed pure gate cannot admit an image.
}

func (g *RuntimeOwnerInvariantSourceGate) ValidateDefinitionSources(ledger RuntimeSemanticPlanningLedger, seed RuntimeDefinitionSeed, focuses []RuntimeDefinitionFocus) error {
	return g.validate(func(s *runtimeSemanticValidationSession) error {
		return validateRuntimeSemanticLedgerDefinitionSourcesInSession(ledger, seed, focuses, s)
	})
}

func (g *RuntimeOwnerInvariantSourceGate) CompileAcceptedBatch(manifest RuntimeSemanticPlanManifest, batch RuntimeSemanticPlanBatch, document RuntimeSemanticPlanDocument, facts []RuntimeSemanticProofFeasibility) (focuses []CoverageFocus, reconciliation RuntimeCWIReconciliationContract, err error) {
	err = g.validate(func(s *runtimeSemanticValidationSession) error {
		focuses, reconciliation, err = compileRuntimeSemanticAcceptedBatchBindingsInSession(manifest, batch, document, facts, s)
		return err
	})
	return
}

func (g *RuntimeOwnerInvariantSourceGate) ValidateDocument(manifest RuntimeSemanticPlanManifest, document RuntimeSemanticPlanDocument) error {
	return g.validate(func(s *runtimeSemanticValidationSession) error {
		validated, err := validateRuntimeSemanticPlanManifestInSession(manifest, s)
		if err != nil {
			return err
		}
		return validateRuntimeSemanticPlanDocumentAgainstValidatedManifest(document, validated)
	})
}

func (g *RuntimeOwnerInvariantSourceGate) ValidateFeasibilities(manifest RuntimeSemanticPlanManifest, document RuntimeSemanticPlanDocument, facts []RuntimeSemanticProofFeasibility, complete bool) error {
	return g.validate(func(s *runtimeSemanticValidationSession) error {
		validated, err := validateRuntimeSemanticPlanManifestInSession(manifest, s)
		if err != nil {
			return err
		}
		if err := validateRuntimeSemanticPlanDocumentAgainstValidatedManifest(document, validated); err != nil {
			return err
		}
		return validateRuntimeSemanticFeasibilitySetWithValidatedManifest(validated, document, facts, complete)
	})
}

func (g *RuntimeOwnerInvariantSourceGate) ResolveObligation(obligation RuntimeSemanticProofObligation) (resolved RuntimeSemanticEvidenceObligation, err error) {
	err = g.validate(func(s *runtimeSemanticValidationSession) error {
		resolved, err = resolveRuntimeSemanticEvidenceObligationInSession(obligation, s)
		return err
	})
	return
}

// ValidateAssessmentSources takes the full ledger as well as every join input;
// it does not accept the public adapter's already-validated-ledger assumption.
// Only allowed historical leaves can reuse compilation. The entire ledger and
// accepted-attempt/session/dispatch joins execute at every invocation.
func (g *RuntimeOwnerInvariantSourceGate) ValidateAssessmentSources(ledger RuntimeSemanticPlanningLedger, active *RuntimeDispatchRecord, completed []RuntimeDispatchRecord, attempts []RuntimeCandidateAttemptRecord, sessions []RuntimeCandidateRequestSession) error {
	return g.validate(func(s *runtimeSemanticValidationSession) error {
		if err := validateRuntimeSemanticPlanningLedgerInSession(ledger, s); err != nil {
			return fmt.Errorf("accepted runtime assessment semantic ledger: %w", err)
		}
		return validateRuntimeAcceptedAssessmentPurposePreimagesInSession(ledger, active, completed, attempts, sessions, s)
	})
}
