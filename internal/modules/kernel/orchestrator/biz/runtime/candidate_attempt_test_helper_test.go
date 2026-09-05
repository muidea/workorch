package runtime

import (
	"context"
	"fmt"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func runtimeCorrectableMissingOperationsFailure(class runtimecontract.RuntimeCandidateFailureClass) runtimecontract.RuntimeCandidateFailure {
	return runtimecontract.RuntimeCandidateFailure{
		Class: class, Code: "candidate_schema_invalid", Signature: "candidate-schema-missing-operations",
		Reason: "candidate document is missing the required operations field",
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
			Path: "$.operations", Actual: "missing_operations", Required: []string{"operations"},
		},
	}
}

func runtimeTestCandidateAuxiliaryWireRuleIdentity(t testing.TB, manifest runtimecontract.RuntimeCandidateContractManifest, kind string, operationIndex int) *runtimecontract.RuntimeCandidateWireRuleIdentity {
	t.Helper()
	wire, err := runtimecontract.BuildRuntimeCandidateWireContract(manifest)
	if err != nil {
		t.Fatal(err)
	}
	rule, found := runtimecontract.RuntimeCandidateWireRuleForAuxiliaryKind(wire, kind)
	if !found {
		t.Fatalf("runtime test candidate manifest has no auxiliary wire rule for %q", kind)
	}
	identity, err := runtimecontract.RuntimeCandidateWireRuleIdentityForOperation(wire, rule, operationIndex)
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

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
	manifest, err := BuildRuntimeCandidateContractManifestForAggregate(aggregate)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	correctionHash := ""
	if attempt == 1 {
		correctionHash = fmt.Sprintf("test-correction-%d", attempt)
	}
	ensured, session, err := c.ensureCandidateRequestSession(ctx, workspaceID, runID, runtimecontract.RuntimeProviderDocumentOperations, stablePrefixHash, manifest.Hash, correctionHash)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if ensured.Decision.State == runtimecontract.ConvergenceStateFailed || ensured.Decision.State == runtimecontract.ConvergenceStateComplete {
		return ensured, nil
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
	maxOutputTokens := 0
	if aggregate.Facts.Resources.ProviderCeiling != nil {
		maxOutputTokens = aggregate.Facts.Resources.ProviderCeiling.EffectiveMaxOutputTokens
	}
	if maxOutputTokens <= 0 {
		maxOutputTokens = 8192
	}
	maxRecoveryAttempts := execution.CandidateDocumentRecovery.MaxAttempts
	if maxRecoveryAttempts <= 0 {
		maxRecoveryAttempts = 1
	}
	// Session-only fixtures may use an opaque manifest identity. Whenever the
	// aggregate can compile the real current manifest, bind the exact production
	// wire hash so later attempt/manifest persistence exercises the same fence.
	wireContractHash := "runtime-test-wire/" + manifestHash
	if manifest, err := BuildRuntimeCandidateContractManifestForAggregate(aggregate); err == nil && manifest.Hash == manifestHash {
		if wire, wireErr := runtimecontract.BuildRuntimeCandidateWireContract(manifest); wireErr == nil {
			wireContractHash = wire.Hash
		}
	}
	return runtimecontract.RuntimeExpectedDocumentContract{
		SchemaVersion:       runtimecontract.RuntimeExpectedDocumentSchemaCurrent,
		DocumentKind:        runtimecontract.RuntimeProviderDocumentOperations,
		ManifestHash:        manifestHash,
		WireContractHash:    wireContractHash,
		CorrectionAttempt:   correctionAttempt,
		MaxOutputTokens:     maxOutputTokens,
		MaxRecoveryAttempts: maxRecoveryAttempts,
	}
}

// runtimeTestPrepareUnboundActivationFacts completes only the immutable
// activation-side test declarations. It deliberately rejects every SM-2
// semantic projection: a CoverageEvidenceBinding, reconciliation row, or CWI
// binding can exist only after the initialized aggregate passes through the
// semantic compiler/seal transition below.
func runtimeTestPrepareUnboundActivationFacts(t *testing.T, facts runtimecontract.RuntimeFacts) runtimecontract.RuntimeFacts {
	t.Helper()
	assertUnbound := func(label string, focus runtimecontract.CoverageFocus) {
		if len(focus.CoverageEvidenceBindings) != 0 || focus.EvidenceDeclarationRef != "" {
			t.Fatalf("%s carries pre-seal semantic authority: %+v", label, focus)
		}
	}
	assertUnbound("activation focus", facts.Focus)
	for index, focus := range facts.RemainingFocuses {
		assertUnbound(fmt.Sprintf("remaining activation focus[%d]", index), focus)
	}
	for index, focus := range facts.CompletedFocuses {
		assertUnbound(fmt.Sprintf("completed activation focus[%d]", index), focus)
	}
	if facts.ReconciliationContract.BindingSetHash != "" || len(facts.ReconciliationContract.Bindings) != 0 {
		t.Fatalf("activation facts carry a pre-seal reconciliation contract: %+v", facts.ReconciliationContract)
	}
	for _, item := range facts.Coverage {
		if item.CurrentBindingRef != nil || len(item.BindingLineage) != 0 || len(item.BindingRetirementLineage) != 0 || item.FocusID != "" {
			t.Fatalf("activation CWI %q carries pre-seal semantic lineage: %+v", item.ID, item)
		}
	}
	runtimeTestSealCurrentPolicies(&facts)
	// Targeted fixtures describe their activation declaration through the
	// unbound Focus queue. Recompile the immutable per-CWI projection from that
	// queue so a stale generic controllerFacts declaration cannot erase a gate
	// the fixture just set.
	facts.DefinitionFocuses = nil
	facts.DefinitionFocuses = runtimeTestDefinitionFocuses(facts)
	bound, err := runtimeTestBindManagedValidationProofRequirements(facts)
	if err != nil {
		t.Fatalf("bind unbound activation validation proof requirements: %v", err)
	}
	facts = bound
	return facts
}

// runtimeTestInitializeAndSealSemanticContract follows the production test
// lifecycle exactly: persist unbound activation input first, then ask SM-2's
// semantic compiler/owner ledger helper to seal and persist the complete
// generation. Callers must use the returned aggregate (and its Facts) for
// candidate binding; the activation input is not post-seal authority.
func runtimeTestInitializeAndSealSemanticContract(
	t *testing.T,
	ctx context.Context,
	controller *RuntimeController,
	facts runtimecontract.RuntimeFacts,
	definition runtimecontract.RuntimeDefinitionContext,
	requirements []runtimecontract.RuntimeRequirement,
) RuntimeAggregate {
	t.Helper()
	if controller == nil {
		t.Fatal("semantic lifecycle fixture has no controller")
	}
	facts = runtimeTestPrepareUnboundActivationFacts(t, facts)
	// The production Application seals the same capability inventory into both
	// Definition and RuntimeFacts. Focused fixtures often start from a minimal
	// controllerDefinition, so make that activation preimage explicit here.
	for _, kind := range facts.Capabilities.StateChangeKinds {
		definition.CapabilityContract = runtimeTestAppendUnique(definition.CapabilityContract, kind)
	}
	for _, kind := range facts.Capabilities.ValidationKinds {
		definition.CapabilityContract = runtimeTestAppendUnique(definition.CapabilityContract, kind)
	}
	if _, err := controller.initialize(ctx, facts, definition, requirements); err != nil {
		t.Fatalf("initialize unbound semantic lifecycle fixture: %v", err)
	}
	sealed, err := controller.EnsureSemanticContractSealedForOperationTest(ctx, facts.WorkspaceID, facts.RunID)
	if err != nil {
		t.Fatalf("seal semantic lifecycle fixture: %v", err)
	}
	if sealed.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningSealed ||
		sealed.SemanticPlanning.Manifest == nil || sealed.SemanticPlanning.SealedBindingSetHash == "" {
		t.Fatalf("semantic lifecycle fixture did not persist a complete seal: %+v", sealed.SemanticPlanning)
	}
	return sealed
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
	// Catalog refresh retires the complete prior semantic generation before a
	// replacement is requested. Preserve that immutable history in the focused
	// provider fixture so historical candidate sessions remain reverse-bound to
	// their original provider/owner preimage instead of being laundered through
	// the replacement generation.
	previousPlanning := aggregate.SemanticPlanning.Clone()
	ledger.History = make([]runtimecontract.RuntimeSemanticPlanningGeneration, len(previousPlanning.History))
	for index := range previousPlanning.History {
		ledger.History[index] = previousPlanning.History[index].Clone()
	}
	ledger.CatalogRefreshAttempts = previousPlanning.CatalogRefreshAttempts
	ledger.FeasibilityGaps = append([]runtimecontract.RuntimeSemanticFeasibilityGap(nil), previousPlanning.FeasibilityGaps...)
	// Focused fixtures may enter with a handwritten reducer binding. Reproject
	// every such row from the exact compiler output of this ledger instead of
	// preserving an authority that has no provider/owner preimage.
	aggregate.SemanticPlanning = ledger
	aggregate.Facts = runtimeTestProjectSemanticOwnerCatalogFacts(aggregate.Facts, ledger)
	facts, focuses, err := runtimeTestProjectSemanticBindingPreimageFacts(aggregate)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	ledger.SealedBindingSetHash = facts.ReconciliationContract.BindingSetHash
	if len(ledger.SealedLineageRefs) == 0 {
		return RuntimeAggregate{}, fmt.Errorf("test semantic ledger has no seal lineage")
	}
	ledger.SealedLineageRefs[len(ledger.SealedLineageRefs)-1] = ledger.SealedBindingSetHash
	if err := ledger.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	facts.Capabilities = runtimeCapabilitiesWithSemanticProofs(facts.Capabilities, focuses)
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
	if len(manifest.Operations) != 0 && len(candidateRuntimeOperations(candidate)) != 0 {
		candidate, err = runtimecontract.DeriveRuntimeCandidateOperationAuthority(manifest, candidate)
		if err != nil {
			return CandidateAdmissionCommitResult{}, err
		}
	} else {
		candidate = runtimeTestRestorePersistedCandidateAuthority(aggregate, candidate)
	}
	return c.admitCandidateProposal(ctx, workspaceID, runID, attempt, manifest, logicalRequestID, candidate)
}

func (c *RuntimeController) AdmitAndSealCandidateAttemptForTest(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, candidate runtimecontract.RuntimeCandidate) (CandidateAdmissionCommitResult, error) {
	aggregate, manifest, logicalRequestID, err := candidateAttemptTestBinding(ctx, c, workspaceID, runID, attempt)
	if err != nil {
		return CandidateAdmissionCommitResult{}, err
	}
	candidate = runtimeTestBindCandidateToFocusBindings(aggregate.Facts.Focus.CoverageEvidenceBindings, candidate)
	if len(manifest.Operations) != 0 && len(candidateRuntimeOperations(candidate)) != 0 {
		if len(candidateRuntimeOperations(candidate)) != 0 {
			candidate, err = runtimecontract.DeriveRuntimeCandidateOperationAuthority(manifest, candidate)
			if err != nil {
				return CandidateAdmissionCommitResult{}, err
			}
		}
	} else {
		candidate = runtimeTestRestorePersistedCandidateAuthority(aggregate, candidate)
	}
	contractID := runtimecontract.StableAuthorityContractID(workspaceID, runID, aggregate.Decision.AuthorityTransaction())
	_, record, pending := pendingCandidateAttempt(aggregate.CandidateAttempts, workspaceID, runID, contractID, attempt)
	if pending && record.Proposal == nil {
		proposal, proposalErr := c.admitCandidateProposal(ctx, workspaceID, runID, attempt, manifest, logicalRequestID, candidate)
		if proposalErr != nil || !proposal.Admission.Accepted() {
			return proposal, proposalErr
		}
		aggregate, manifest, logicalRequestID, err = candidateAttemptTestBinding(ctx, c, workspaceID, runID, attempt)
		if err != nil {
			return CandidateAdmissionCommitResult{}, err
		}
		candidate = runtimeTestBindCandidateToFocusBindings(aggregate.Facts.Focus.CoverageEvidenceBindings, candidate)
		candidate, err = runtimecontract.DeriveRuntimeCandidateOperationAuthority(manifest, candidate)
		if err != nil {
			return CandidateAdmissionCommitResult{}, err
		}
	}
	return c.admitAndSealCandidateAttempt(ctx, workspaceID, runID, attempt, manifest, logicalRequestID, candidate)
}

func runtimeTestRestorePersistedCandidateAuthority(aggregate RuntimeAggregate, candidate runtimecontract.RuntimeCandidate) runtimecontract.RuntimeCandidate {
	if len(candidateRuntimeOperations(candidate)) == 0 {
		return candidate
	}
	wantShape := runtimecontract.StableRuntimeCandidateSemanticShape(candidate)
	for _, record := range aggregate.CandidateAttempts {
		if record.Proposal != nil && runtimecontract.StableRuntimeCandidateSemanticShape(*record.Proposal) == wantShape {
			return *record.Proposal
		}
	}
	if aggregate.ActiveDispatch != nil && runtimecontract.StableRuntimeCandidateSemanticShape(aggregate.ActiveDispatch.Candidate) == wantShape {
		return aggregate.ActiveDispatch.Candidate
	}
	for _, dispatch := range aggregate.CompletedDispatches {
		if runtimecontract.StableRuntimeCandidateSemanticShape(dispatch.Candidate) == wantShape {
			return dispatch.Candidate
		}
	}
	return candidate
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
			manifest, manifestErr := BuildRuntimeCandidateContractManifestForAggregate(aggregate)
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
	// Candidate admission consumes the sealed SM-2 target surface. Hand-written
	// tests must construct that binding explicitly instead of allowing a later
	// owner selection to mutate semantic authority.
	aggregate.Facts.Focus = runtimeTestAlignFocusTargetSurfaceFromBindings(aggregate.Facts.Focus)
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
	manifest, err := BuildRuntimeCandidateContractManifestForAggregate(aggregate)
	if err != nil {
		return rejectedCandidateAdmission(candidateAdmissionCodeInvalid, candidateAdmissionInvalidSignature(candidate), fmt.Sprintf("runtime candidate manifest contract: %v", err))
	}
	candidate, err = runtimecontract.DeriveRuntimeCandidateOperationAuthority(manifest, candidate)
	if err != nil {
		return rejectedCandidateAdmission(candidateAdmissionCodeInvalid, candidateAdmissionInvalidSignature(candidate), fmt.Sprintf("runtime candidate operation authority: %v", err))
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
	manifest, err := BuildRuntimeCandidateContractManifestForAggregate(*aggregate)
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
