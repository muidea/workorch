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
		return RuntimeAggregate{}, fmt.Errorf("%w: current runtime action %q does not request a candidate (prerequisites=%+v)", common.ErrConflict, aggregate.Decision.State, aggregate.Decision.Prerequisites)
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

func (c *RuntimeController) ensureCandidateRequestSession(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, documentKind runtimecontract.RuntimeProviderDocumentKind, stablePrefixHash, manifestHash, correctionHash string) (RuntimeAggregate, runtimecontract.RuntimeCandidateRequestSession, error) {
	if documentKind != runtimecontract.RuntimeProviderDocumentOperations || c == nil || c.repository == nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeCandidateRequestSession{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeCandidateRequestSession{}, err
	}
	correctionAttempt := 0
	if correctionHash != "" {
		correctionAttempt = 1
	}
	expected := runtimeTestOperationsExpectedDocument(aggregate, manifestHash, correctionAttempt)
	return c.ensureCandidateRequestSessionWithExpected(ctx, workspaceID, runID, documentKind, stablePrefixHash, manifestHash, correctionHash, &expected)
}

func runtimeTestOperationsExpectedDocument(aggregate RuntimeAggregate, manifestHash string, correctionAttempt int) runtimecontract.RuntimeExpectedDocumentContract {
	execution := aggregate.Facts.Activation.ExecutionContract.Normalized()
	maxOutputTokens := execution.CandidateMaxOutputTokens
	if maxOutputTokens <= 0 && aggregate.Facts.Resources.ProviderCeiling != nil {
		maxOutputTokens = aggregate.Facts.Resources.ProviderCeiling.EffectiveMaxOutputTokens
	}
	if maxOutputTokens <= 0 {
		maxOutputTokens = 8192
	}
	maxRecoveryAttempts := execution.CandidateDocumentRecovery.MaxAttempts
	if maxRecoveryAttempts <= 0 {
		maxRecoveryAttempts = 1
	}
	return runtimecontract.RuntimeExpectedDocumentContract{
		SchemaVersion:       runtimecontract.RuntimeExpectedDocumentSchemaCurrent,
		DocumentKind:        runtimecontract.RuntimeProviderDocumentOperations,
		ManifestHash:        manifestHash,
		CorrectionAttempt:   correctionAttempt,
		MaxOutputTokens:     maxOutputTokens,
		MaxRecoveryAttempts: maxRecoveryAttempts,
	}
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
	if runtimeTestFactsAlreadyHaveCurrentCWIBindings(aggregate.Facts) {
		// Some focused reducer fixtures explicitly project a post-semantic
		// authority before entering this helper. Keep that tested binding rather
		// than creating a second semantic generation over the same CWI.
		facts, rebindErr := runtimeTestBindCurrentCWIRowsToSemanticManifest(aggregate.Facts, ledger.Manifest.Hash)
		if rebindErr != nil {
			return RuntimeAggregate{}, rebindErr
		}
		aggregate.Facts = facts
		ledger.SealedBindingSetHash = aggregate.Facts.ReconciliationContract.BindingSetHash
		if len(ledger.SealedLineageRefs) == 0 {
			return RuntimeAggregate{}, fmt.Errorf("test semantic ledger has no seal lineage")
		}
		ledger.SealedLineageRefs[len(ledger.SealedLineageRefs)-1] = ledger.SealedBindingSetHash
		if err := ledger.Validate(); err != nil {
			return RuntimeAggregate{}, err
		}
		facts, decision, reduceErr := ReduceRuntimeStateWithDirective(aggregate.Facts, nil, aggregate.CoverageProofLineage)
		if reduceErr != nil {
			return RuntimeAggregate{}, reduceErr
		}
		expectedRevision := aggregate.StateRevision
		aggregate.SemanticPlanning = ledger
		aggregate.Facts = facts
		aggregate.Decision = decision
		aggregate.StateRevision++
		if err := aggregate.Validate(); err != nil {
			return RuntimeAggregate{}, err
		}
		if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
			return RuntimeAggregate{}, err
		}
		return aggregate, nil
	}
	sourceFocuses := append([]runtimecontract.CoverageFocus{aggregate.Facts.Focus}, aggregate.Facts.RemainingFocuses...)
	sourceFocuses = append(sourceFocuses, aggregate.Facts.CompletedFocuses...)
	focuses, _, err := runtimecontract.CompileRuntimeSemanticBindingsWithSourceFocuses(*ledger.Manifest, *ledger.Document, ledger.Feasibilities, sourceFocuses)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	pending, err := semanticPendingFocuses(aggregate.Facts, ledger.PendingCoverageIDs, focuses)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	facts, err := sealRuntimeCWIBindings(aggregate.Facts, ledger.Manifest.Hash, focuses, pending)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	reconciliation, err := canonicalRuntimeCWIReconciliationContract(facts)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	ledger.SealedBindingSetHash = reconciliation.BindingSetHash
	if len(ledger.SealedLineageRefs) == 0 {
		return RuntimeAggregate{}, fmt.Errorf("test semantic ledger has no seal lineage")
	}
	ledger.SealedLineageRefs[len(ledger.SealedLineageRefs)-1] = reconciliation.BindingSetHash
	if err := ledger.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	facts.Capabilities = runtimeCapabilitiesWithSemanticProofs(facts.Capabilities, focuses)
	facts.ReconciliationContract = reconciliation
	// Production acquires target snapshots through the File owner before an
	// operation request. Focused operation tests deliberately bypass that
	// EventHub round-trip, so seed the same owner fact here rather than letting
	// a synthetic semantic focus leave SM-2 in prerequisite state.
	facts = runtimeTestSeedSemanticTargetSnapshots(facts)
	facts, decision, err := ReduceRuntimeStateWithDirective(facts, nil, aggregate.CoverageProofLineage)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	expectedRevision := aggregate.StateRevision
	aggregate.SemanticPlanning = ledger
	aggregate.Facts = facts
	aggregate.Decision = decision
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func runtimeTestBindCurrentCWIRowsToSemanticManifest(facts runtimecontract.RuntimeFacts, manifestHash string) (runtimecontract.RuntimeFacts, error) {
	facts = facts.Normalized()
	if manifestHash == "" {
		return runtimecontract.RuntimeFacts{}, fmt.Errorf("test semantic manifest hash is empty")
	}
	focusByCoverage := make(map[common.CoverageWorkItemID]runtimecontract.CoverageFocus, len(facts.Coverage))
	for _, focus := range append(append([]runtimecontract.CoverageFocus{facts.Focus}, facts.RemainingFocuses...), facts.CompletedFocuses...) {
		focus = focus.Normalized()
		for _, coverageID := range focus.CoverageWorkItemIDs {
			focusByCoverage[coverageID] = focus
		}
	}
	for index := range facts.Coverage {
		item := &facts.Coverage[index]
		if item.CurrentBindingRef == nil {
			continue
		}
		focus, found := focusByCoverage[item.ID]
		if !found {
			return runtimecontract.RuntimeFacts{}, fmt.Errorf("test semantic manifest has no focus for CWI %q", item.ID)
		}
		binding, found := coverageBindingForRuntimeCWI(focus, item.ID)
		if !found {
			return runtimecontract.RuntimeFacts{}, fmt.Errorf("test semantic manifest has no binding for CWI %q", item.ID)
		}
		ref := runtimecontract.RuntimeCWIBindingRefForCoverageBinding(binding, manifestHash)
		item.CurrentBindingRef = &ref
		if len(item.BindingLineage) == 0 {
			item.BindingLineage = []runtimecontract.RuntimeCWIBindingRef{ref}
		} else {
			item.BindingLineage[len(item.BindingLineage)-1] = ref
		}
	}
	facts.Coverage = runtimecontract.NormalizeRuntimeCoverageFacts(facts.Coverage)
	if err := runtimecontract.ValidateRuntimeCWILedger(facts); err != nil {
		return runtimecontract.RuntimeFacts{}, err
	}
	return facts, nil
}

func runtimeTestSeedSemanticTargetSnapshots(facts runtimecontract.RuntimeFacts) runtimecontract.RuntimeFacts {
	facts = facts.Normalized()
	existing := make(map[string]struct{}, len(facts.Resources.TargetSnapshots))
	for _, snapshot := range facts.Resources.TargetSnapshots {
		if snapshot.Complete() {
			existing[snapshot.TargetRef] = struct{}{}
		}
	}
	// A current failure target is as authoritative as a declarative focus target
	// for candidate admission. Production obtains both through the File owner;
	// focused tests seed the same bounded fact so they do not bypass the
	// target-snapshot prerequisite merely because a semantic fixture is direct.
	targetRefs := append([]string(nil), facts.Focus.TargetRefs...)
	targetRefs = append(targetRefs, GroundedRepairTargets(facts)...)
	for _, targetRef := range targetRefs {
		if _, found := existing[targetRef]; found {
			continue
		}
		facts.Resources.TargetSnapshots = append(facts.Resources.TargetSnapshots, runtimecontract.TargetSnapshot{
			TargetRef: targetRef, Content: "test semantic target", ContentHash: "test-semantic-snapshot/" + targetRef,
			ContentState: runtimecontract.SnapshotContentStateComplete,
		})
	}
	return facts.Normalized()
}

func runtimeTestFactsAlreadyHaveCurrentCWIBindings(facts runtimecontract.RuntimeFacts) bool {
	if facts.CWILedgerSchemaVersion != runtimecontract.RuntimeCWILedgerSchemaCurrent || facts.ReconciliationContract.SchemaVersion == "" {
		return false
	}
	for _, item := range facts.Coverage {
		if item.CurrentBindingRef != nil {
			return true
		}
	}
	return false
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
	expected := runtimeTestOperationsExpectedDocument(*aggregate, manifest.Hash, record.Attempt)
	expected.LogicalRequestID = logicalRequestID
	expected.StablePrefixHash = record.StablePrefixHash
	expected.Hash = runtimecontract.StableRuntimeExpectedDocumentHash(expected)
	aggregate.CandidateRequestSessions = append(aggregate.CandidateRequestSessions, runtimecontract.RuntimeCandidateRequestSession{
		LogicalRequestID: logicalRequestID, DocumentKind: runtimecontract.RuntimeProviderDocumentOperations, Transaction: record.Transaction, StablePrefixHash: record.StablePrefixHash, ManifestHash: manifest.Hash,
		CorrectionHash: correctionHash, State: runtimecontract.RuntimeCandidateRequestSessionDocumentComplete, NextSequence: 1,
		LastPhysicalCallID: runtimecontract.StableRuntimeCandidatePhysicalCallID(logicalRequestID, 1), ProviderSubmitted: true,
		LastOutcome:      runtimecontract.RuntimeCandidateRequestOutcome{Class: runtimecontract.RuntimeCandidateRequestOutcomeDocumentComplete},
		ExpectedDocument: &expected,
	})
	record.ManifestHash = manifest.Hash
	record.LogicalRequestID = logicalRequestID
}
