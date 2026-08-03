package runtime

import (
	"context"
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// The helpers in this file deliberately construct the same persisted complete
// session used by production. They keep focused reducer/controller fixtures
// concise without retaining an unbound production admission API.
func (c *RuntimeController) BeginCandidateAttemptForTest(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, stablePrefixHash string) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.EnsureSemanticContractSealedForOperationTest(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	switch aggregate.Decision.State {
	case runtimecontract.ConvergenceStateNeedStateChange, runtimecontract.ConvergenceStateNeedValidation, runtimecontract.ConvergenceStateNeedAssessment:
	default:
		return RuntimeAggregate{}, fmt.Errorf("%w: current runtime action does not request a candidate", common.ErrConflict)
	}
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(
		aggregate.Decision,
		runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(
			runtimecontract.RuntimeCandidateContextForDecision(aggregate.Facts, aggregate.Decision, nil),
		),
	)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	correctionHash := ""
	if attempt == 1 {
		correctionHash = fmt.Sprintf("test-correction-%d", attempt)
	}
	_, session, err := c.ensureCandidateRequestSession(ctx, workspaceID, runID, runtimecontract.RuntimeProviderDocumentOperations, stablePrefixHash, manifest.Hash, correctionHash)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if session.State != runtimecontract.RuntimeCandidateRequestSessionDocumentComplete {
		physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, session.NextSequence)
		if _, err := c.markCandidateRequestSubmitted(ctx, workspaceID, runID, session.LogicalRequestID, session.NextSequence, physicalCallID); err != nil {
			return RuntimeAggregate{}, err
		}
		_, err = c.completeCandidateRequestDocument(ctx, workspaceID, runID, session.LogicalRequestID, physicalCallID)
		if err != nil {
			return RuntimeAggregate{}, err
		}
	}
	return c.beginCandidateAttempt(ctx, workspaceID, runID, attempt, stablePrefixHash, manifest, session.LogicalRequestID)
}

func (c *RuntimeController) EnsureSemanticContractSealedForOperationTest(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeAggregate, error) {
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.SemanticPlanning.Phase == runtimecontract.RuntimeSemanticPlanningSealed {
		return aggregate, nil
	}
	ledger, err := runtimeTestBuildSealedSemanticPlanning(aggregate)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	expectedRevision := aggregate.StateRevision
	aggregate.SemanticPlanning = ledger
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func (c *RuntimeController) AdmitCandidateProposalForTest(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, candidate runtimecontract.RuntimeCandidate) (CandidateAdmissionCommitResult, error) {
	aggregate, manifest, logicalRequestID, err := candidateAttemptTestBinding(ctx, c, workspaceID, runID, attempt)
	if err != nil {
		return CandidateAdmissionCommitResult{}, err
	}
	candidate = runtimeTestBindCandidateToFocusBindings(aggregate.Facts.Focus.CoverageEvidenceBindings, candidate)
	return c.admitCandidateProposal(ctx, workspaceID, runID, attempt, manifest, logicalRequestID, candidate)
}

func (c *RuntimeController) AdmitAndSealCandidateAttemptForTest(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, candidate runtimecontract.RuntimeCandidate) (CandidateAdmissionCommitResult, error) {
	aggregate, manifest, logicalRequestID, err := candidateAttemptTestBinding(ctx, c, workspaceID, runID, attempt)
	if err != nil {
		return CandidateAdmissionCommitResult{}, err
	}
	candidate = runtimeTestBindCandidateToFocusBindings(aggregate.Facts.Focus.CoverageEvidenceBindings, candidate)
	return c.admitAndSealCandidateAttempt(ctx, workspaceID, runID, attempt, manifest, logicalRequestID, candidate)
}

func candidateAttemptTestBinding(ctx context.Context, c *RuntimeController, workspaceID common.WorkspaceID, runID common.RunID, attempt int) (RuntimeAggregate, runtimecontract.RuntimeCandidateContractManifest, string, error) {
	if c == nil || c.repository == nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeCandidateContractManifest{}, "", common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeCandidateContractManifest{}, "", err
	}
	var fallback *runtimecontract.RuntimeCandidateAttemptRecord
	for _, record := range aggregate.CandidateAttempts {
		if record.Attempt != attempt {
			continue
		}
		if record.Transaction == aggregate.Decision.AuthorityTransaction() {
			manifest, manifestErr := runtimecontract.BuildRuntimeCandidateContractManifest(
				aggregate.Decision,
				runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(
					runtimecontract.RuntimeCandidateContextForDecision(aggregate.Facts, aggregate.Decision, nil),
				),
			)
			if manifestErr != nil {
				// Terminal duplicate delivery does not recompile authority: the
				// persisted record/session hash is its complete identity.
				return aggregate, runtimecontract.RuntimeCandidateContractManifest{Hash: record.ManifestHash}, record.LogicalRequestID, nil
			}
			return aggregate, manifest, record.LogicalRequestID, nil
		}
		copyRecord := record
		fallback = &copyRecord
	}
	if fallback != nil {
		return aggregate, runtimecontract.RuntimeCandidateContractManifest{Hash: fallback.ManifestHash}, fallback.LogicalRequestID, nil
	}
	return RuntimeAggregate{}, runtimecontract.RuntimeCandidateContractManifest{}, "", fmt.Errorf("%w: candidate attempt %d has no test binding", common.ErrConflict, attempt)
}

func EvaluateCandidateAdmission(aggregate RuntimeAggregate, candidate runtimecontract.RuntimeCandidate) CandidateAdmissionResult {
	candidate = runtimeTestBindCandidateToFocusBindings(aggregate.Facts.Focus.CoverageEvidenceBindings, candidate)
	// Test fixtures sometimes mutate Facts (FailureTarget, TargetSelection, focus
	// TargetRefs) after the initial Reduce. Production always builds the Decision
	// from current Facts before compiling the manifest. Realign only when grounded
	// targets diverge; do not re-reduce merely because evidence/repair ledgers
	// changed, or admission tests that intentionally keep a candidate Decision
	// would flip to validation/assessment and obscure the policy under test.
	if candidateDecisionNeedsGroundingRealign(aggregate) {
		if decision, err := ReduceRuntimeState(aggregate.Facts, nil); err == nil &&
			(decision.State == runtimecontract.ConvergenceStateNeedStateChange ||
				decision.State == runtimecontract.ConvergenceStateNeedValidation ||
				decision.State == runtimecontract.ConvergenceStateNeedAssessment ||
				decision.State == runtimecontract.ConvergenceStateNeedGovernance) {
			aggregate.Decision = decision
		}
	}
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(
		aggregate.Decision,
		runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(
			runtimecontract.RuntimeCandidateContextForDecision(aggregate.Facts, aggregate.Decision, nil),
		),
	)
	if err != nil {
		return rejectedCandidateAdmission(candidateAdmissionCodeInvalid, candidateAdmissionInvalidSignature(candidate), fmt.Sprintf("runtime candidate manifest contract: %v", err))
	}
	return EvaluateCandidateAdmissionWithManifest(aggregate, manifest, candidate)
}

func candidateDecisionNeedsGroundingRealign(aggregate RuntimeAggregate) bool {
	grounded := GroundedRepairTargets(aggregate.Facts)
	current := aggregate.Decision.RequiredOutput.Normalized().TargetRefs
	if sameRuntimeTargetRefs(grounded, current) {
		return false
	}
	// An empty grounded set with non-empty decision targets still needs realign
	// when focus TargetRefs were cleared in favor of selection/failure authority.
	return true
}

func bindRuntimeCandidateAttemptForTest(t interface {
	Helper()
	Fatal(...any)
}, aggregate *RuntimeAggregate, index int) {
	t.Helper()
	if aggregate == nil || index < 0 || index >= len(aggregate.CandidateAttempts) {
		t.Fatal("candidate attempt test binding index is invalid")
	}
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(
		aggregate.Decision,
		runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(
			runtimecontract.RuntimeCandidateContextForDecision(aggregate.Facts, aggregate.Decision, nil),
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	record := &aggregate.CandidateAttempts[index]
	correctionHash := fmt.Sprintf("fixture-%d", record.Attempt)
	logicalRequestID := runtimecontract.StableRuntimeCandidateRequestSessionID(runtimecontract.RuntimeProviderDocumentOperations, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, record.Transaction, record.StablePrefixHash, manifest.Hash, correctionHash)
	for _, session := range aggregate.CandidateRequestSessions {
		if session.LogicalRequestID == logicalRequestID {
			record.ManifestHash = manifest.Hash
			record.LogicalRequestID = logicalRequestID
			return
		}
	}
	aggregate.CandidateRequestSessions = append(aggregate.CandidateRequestSessions, runtimecontract.RuntimeCandidateRequestSession{
		LogicalRequestID: logicalRequestID, DocumentKind: runtimecontract.RuntimeProviderDocumentOperations, Transaction: record.Transaction, StablePrefixHash: record.StablePrefixHash, ManifestHash: manifest.Hash,
		CorrectionHash: correctionHash, State: runtimecontract.RuntimeCandidateRequestSessionDocumentComplete, NextSequence: 1,
		LastPhysicalCallID: runtimecontract.StableRuntimeCandidatePhysicalCallID(logicalRequestID, 1), ProviderSubmitted: true,
		LastOutcome: runtimecontract.RuntimeCandidateRequestOutcome{Class: runtimecontract.RuntimeCandidateRequestOutcomeDocumentComplete},
	})
	record.ManifestHash = manifest.Hash
	record.LogicalRequestID = logicalRequestID
}
