package runtime

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/grounding"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// RuntimeSemanticCatalogFreshnessGap is an Orchestrator-local comparison
// result. It is deliberately a detached fact: it does not select a retry or
// mutate any owner state. A gap means the active semantic manifest no longer
// has the exact owner catalog observation from which it was sealed.
type RuntimeSemanticCatalogFreshnessGap struct {
	PrerequisiteID        string
	ManifestSnapshotID    string
	CurrentSnapshotID     string
	ManifestObservationID string
	CurrentObservationID  string
}

// RuntimeSemanticCatalogScopeGap is an Orchestrator-local SM-2 fact. It says
// that an already satisfied owner catalog cannot authorize the concrete
// targets of the next semantic focus. It does not claim that the owner
// resource revision is stale and does not select a provider repair action.
type RuntimeSemanticCatalogScopeGap struct {
	PrerequisiteID     string
	ObservationID      string
	OwnerModuleID      string
	CatalogSnapshotID  string
	FocusID            string
	RequiredTargetRefs []string
}

// NextRuntimeSemanticCatalogScopeGap returns the first deterministic catalog
// that must be reacquired before compiling a semantic manifest for the current
// unbound focus. Only catalogs containing an operation kind declared by the
// focus participate. A settled CWI's binding/effect/evidence is deliberately
// outside this comparison and remains immutable lineage.
func NextRuntimeSemanticCatalogScopeGap(aggregate RuntimeAggregate) (RuntimeSemanticCatalogScopeGap, bool) {
	facts := aggregate.Facts.Normalized()
	if aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningRequired ||
		len(facts.Focus.CoverageEvidenceBindings) != 0 || len(facts.Focus.TargetRefs) == 0 {
		return RuntimeSemanticCatalogScopeGap{}, false
	}
	requiredTargets := normalizeRuntimeTargetRefs(facts.Focus.TargetRefs)
	acceptedKinds := make(map[string]struct{}, len(facts.Focus.AcceptedStepKinds))
	for _, kind := range facts.Focus.AcceptedStepKinds {
		if canonical := runtimecontract.CanonicalRuntimeBlockKind(kind); canonical != "" {
			acceptedKinds[canonical] = struct{}{}
		}
	}
	for _, catalog := range facts.Resources.OperationAdmissibilities {
		catalog = catalog.Normalized()
		relevant := false
		for _, kind := range catalog.OperationKinds {
			if _, found := acceptedKinds[kind]; found {
				relevant = true
				break
			}
		}
		if !relevant || grounding.CatalogCoversFocusTargets(catalog, facts.Focus, requiredTargets) {
			continue
		}
		observation, found := runtimecontract.RuntimePrerequisiteObservationForID(facts.Resources.PrerequisiteObservations, catalog.PrerequisiteID)
		if !found || observation.Kind != runtimecontract.PrerequisiteKindOperationAdmissibility ||
			observation.OwnerModuleID != catalog.OwnerModuleID || observation.ContentHash != catalog.SnapshotID {
			continue
		}
		return RuntimeSemanticCatalogScopeGap{
			PrerequisiteID: catalog.PrerequisiteID, ObservationID: observation.ObservationID,
			OwnerModuleID: catalog.OwnerModuleID, CatalogSnapshotID: catalog.SnapshotID,
			FocusID: facts.Focus.ID, RequiredTargetRefs: requiredTargets,
		}, true
	}
	return RuntimeSemanticCatalogScopeGap{}, false
}

// runtimeSemanticCatalogCoversFocusTargets distinguishes an exact target
// catalog from a closed owner selector. Git, MCP and similar owners may
// authorize an existing owner scope without enumerating workspace target
// refs; repeatedly reacquiring that unchanged catalog cannot make its target
// list grow. The selector is authority only when the exact accepted
// kind/variant publishes the typed policy.
func runtimeSemanticCatalogCoversFocusTargets(catalog runtimecontract.RuntimeOperationAdmissibility, focus runtimecontract.CoverageFocus, requiredTargets []string) bool {
	return grounding.CatalogCoversFocusTargets(catalog, focus, requiredTargets)
}

func allRuntimeTargetRefsContained(required, available []string) bool {
	return grounding.TargetRefsContained(required, available)
}

func runtimeSemanticCatalogScopeGapSignature(gap RuntimeSemanticCatalogScopeGap) string {
	return fmt.Sprintf("semantic-catalog-focus-scope/%s/%s/%s/%s", strings.TrimSpace(gap.FocusID), strings.TrimSpace(gap.OwnerModuleID), strings.TrimSpace(gap.PrerequisiteID), strings.TrimSpace(gap.CatalogSnapshotID))
}

// invalidateSemanticCatalogForFocusScope is the sole mutation for the scope
// gap above. It atomically removes only the exact current catalog observation,
// moves its prerequisite back to pending, and records an idempotency fence.
// No recovery budget is consumed: focus advance is expected authority
// progression, not an owner resource failure.
func SemanticManifestCatalogFreshnessGaps(aggregate RuntimeAggregate, manifest runtimecontract.RuntimeSemanticPlanManifest) []RuntimeSemanticCatalogFreshnessGap {
	manifest = manifest.Normalized()
	sealedCatalogs := append([]runtimecontract.RuntimeOperationAdmissibility(nil), manifest.OwnerCatalogs...)
	sealedCatalogs = append(sealedCatalogs, manifest.OwnerFactDependencyCatalogs...)
	compared := grounding.CompareCatalogFreshness(sealedCatalogs, aggregate.Facts)
	gaps := make([]RuntimeSemanticCatalogFreshnessGap, 0, len(compared))
	for _, gap := range compared {
		gaps = append(gaps, RuntimeSemanticCatalogFreshnessGap{
			PrerequisiteID: gap.PrerequisiteID, ManifestSnapshotID: gap.ManifestSnapshotID,
			CurrentSnapshotID: gap.CurrentSnapshotID, ManifestObservationID: gap.ManifestObservationID,
			CurrentObservationID: gap.CurrentObservationID,
		})
	}
	return gaps
}

// SemanticProspectiveFeasibilityFreshnessGaps compares the exact observation
// identities persisted by one composite File→required-owner fact. A catalog
// snapshot can remain byte-identical while its workspace dependency is
// re-observed, so snapshot-only freshness is insufficient at CAS and seal.
func SemanticProspectiveFeasibilityFreshnessGaps(aggregate RuntimeAggregate, manifest runtimecontract.RuntimeSemanticPlanManifest, feasibility runtimecontract.RuntimeSemanticProofFeasibility) []RuntimeSemanticCatalogFreshnessGap {
	manifest = manifest.Normalized()
	feasibility = feasibility.Normalized()
	if feasibility.ProspectiveWorkspaceManifestDelta == nil || feasibility.OwnerFactProspectiveSatisfactionObservation == nil {
		return nil
	}
	gaps := make([]RuntimeSemanticCatalogFreshnessGap, 0, 2)
	if catalog, found := runtimeSemanticManifestCatalogForOwner(manifest, feasibility.OwnerModuleID); found {
		current, currentFound := CurrentSemanticCatalogObservation(aggregate, catalog)
		if !currentFound || current.ObservationID != feasibility.CatalogObservationID {
			gap := RuntimeSemanticCatalogFreshnessGap{PrerequisiteID: catalog.PrerequisiteID, ManifestSnapshotID: catalog.SnapshotID, ManifestObservationID: feasibility.CatalogObservationID}
			if currentFound {
				gap.CurrentSnapshotID, gap.CurrentObservationID = catalog.SnapshotID, current.ObservationID
			}
			gaps = append(gaps, gap)
		}
	}
	prospective := feasibility.OwnerFactProspectiveSatisfactionObservation.Normalized()
	requiredOwnerID := ""
	requiredObservationID := ""
	if prospective.Satisfaction != nil {
		requiredOwnerID = prospective.Satisfaction.OwnerModuleID
		requiredObservationID = prospective.Satisfaction.CatalogObservationID
	} else if prospective.Stale != nil {
		requiredOwnerID = prospective.Stale.OwnerModuleID
		requiredObservationID = prospective.Stale.ObservationID
	}
	if catalog, found := runtimeSemanticManifestDependencyCatalogForOwner(manifest, requiredOwnerID); found {
		current, currentFound := CurrentSemanticCatalogObservation(aggregate, catalog)
		if !currentFound || current.ObservationID != requiredObservationID {
			gap := RuntimeSemanticCatalogFreshnessGap{PrerequisiteID: catalog.PrerequisiteID, ManifestSnapshotID: catalog.SnapshotID, ManifestObservationID: requiredObservationID}
			if currentFound {
				gap.CurrentSnapshotID, gap.CurrentObservationID = catalog.SnapshotID, current.ObservationID
			}
			gaps = append(gaps, gap)
		}
	}
	sort.SliceStable(gaps, func(i, j int) bool { return gaps[i].PrerequisiteID < gaps[j].PrerequisiteID })
	return gaps
}

func runtimeSemanticManifestCatalogForOwner(manifest runtimecontract.RuntimeSemanticPlanManifest, ownerModuleID string) (runtimecontract.RuntimeOperationAdmissibility, bool) {
	ownerModuleID = strings.TrimSpace(ownerModuleID)
	for _, catalog := range manifest.OwnerCatalogs {
		if catalog.OwnerModuleID == ownerModuleID {
			return catalog.Normalized(), true
		}
	}
	return runtimecontract.RuntimeOperationAdmissibility{}, false
}

func runtimeSemanticManifestDependencyCatalogForOwner(manifest runtimecontract.RuntimeSemanticPlanManifest, ownerModuleID string) (runtimecontract.RuntimeOperationAdmissibility, bool) {
	ownerModuleID = strings.TrimSpace(ownerModuleID)
	for _, catalog := range manifest.OwnerFactDependencyCatalogs {
		if catalog.OwnerModuleID == ownerModuleID {
			return catalog.Normalized(), true
		}
	}
	return runtimecontract.RuntimeOperationAdmissibility{}, false
}

func currentOperationAdmissibility(facts runtimecontract.RuntimeFacts, wanted runtimecontract.RuntimeOperationAdmissibility) (runtimecontract.RuntimeOperationAdmissibility, bool) {
	return grounding.CurrentOperationAdmissibility(facts, wanted)
}

// CurrentSemanticCatalogObservation returns the exact current observation for
// one manifest catalog. It intentionally rejects an observation that merely
// shares a prerequisite ID while carrying a different snapshot hash.
func CurrentSemanticCatalogObservation(aggregate RuntimeAggregate, catalog runtimecontract.RuntimeOperationAdmissibility) (runtimecontract.RuntimePrerequisiteObservation, bool) {
	return grounding.CurrentCatalogObservation(aggregate.Facts.Resources.PrerequisiteObservations, catalog)
}

// invalidatePrerequisitesForReadOnlyStale applies an owner-observed freshness
// change before any current-authority candidate work or dispatch exists. It is
// the SM-2 route for a prerequisite probe: the owner supplies only the typed
// stale identity, while the controller atomically retires every
// exactly-intersecting observation and records the read-only refresh ledger
// entry. Historical sessions/attempts from an already-invalidated dispatch are
// immutable audit lineage, not live SM-3 work; rejecting them would make a
// later local prerequisite stale unable to converge after an external receipt
// resource change.
func runtimeAggregateHasCurrentCandidateWork(aggregate RuntimeAggregate) bool {
	transaction := aggregate.Decision.AuthorityTransaction()
	if transaction.AuthoritySnapshotID == "" {
		return false
	}
	for _, session := range aggregate.CandidateRequestSessions {
		if session.Transaction == transaction && session.State != runtimecontract.RuntimeCandidateRequestSessionTerminal {
			return true
		}
	}
	for _, attempt := range aggregate.CandidateAttempts {
		if attempt.Transaction != transaction {
			continue
		}
		switch attempt.State {
		case runtimecontract.RuntimeCandidateAttemptStatePending, runtimecontract.RuntimeCandidateAttemptStateAccepted:
			return true
		}
	}
	return false
}

// invalidateCandidateRequestSessionForPrerequisiteStale is the session-first
// stale route for a provider response or freshness observation received before
// C5 admission. A C4 document may already be complete, but it is not yet a
// candidate attempt and must never be decoded/admitted after the authority
// turns stale. The method deliberately has no candidate-attempt parameter:
// stale authority is an SM-2 prerequisite fact, not an SM-3 candidate
// rejection. The session is retained as terminal audit lineage while the
// reducer seals a new authority around the reacquired active prerequisite.
func runtimeBindingContradictionIdentity(aggregate RuntimeAggregate, record runtimecontract.RuntimeCandidateAttemptRecord, feasibility runtimecontract.RuntimeCandidateFeasibilityRecord, observation runtimecontract.RuntimeOwnerFeasibilityObservation) (runtimecontract.RuntimeBindingContradictionIdentity, common.CoverageWorkItemID, runtimecontract.RuntimeCWIBindingRef, error) {
	observation = observation.Normalized()
	if err := validateRuntimeBindingContradictionObservation(record, feasibility, observation); err != nil {
		return runtimecontract.RuntimeBindingContradictionIdentity{}, "", runtimecontract.RuntimeCWIBindingRef{}, common.ErrInvalidArgument
	}
	operation, err := runtimeBindingContradictionOperation(record, feasibility, observation)
	if err != nil {
		return runtimecontract.RuntimeBindingContradictionIdentity{}, "", runtimecontract.RuntimeCWIBindingRef{}, common.ErrInvalidArgument
	}
	coverageID, ref, err := runtimeBindingContradictionCWI(aggregate, record.Transaction.FocusID, operation.BindingRefs[0])
	if err != nil {
		return runtimecontract.RuntimeBindingContradictionIdentity{}, "", runtimecontract.RuntimeCWIBindingRef{}, common.ErrInvalidArgument
	}
	if err := validateRuntimeBindingContradictionReconciliation(aggregate, coverageID, ref, operation, observation); err != nil {
		return runtimecontract.RuntimeBindingContradictionIdentity{}, "", runtimecontract.RuntimeCWIBindingRef{}, common.ErrInvalidArgument
	}
	diagnostic, err := runtimeBindingContradictionDiagnostic(aggregate, record, operation, observation)
	if err != nil {
		return runtimecontract.RuntimeBindingContradictionIdentity{}, "", runtimecontract.RuntimeCWIBindingRef{}, common.ErrInvalidArgument
	}
	identity := runtimecontract.RuntimeBindingContradictionIdentity{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		AuthorityInputID: record.Transaction.AuthorityInputID,
		BindingID:        ref.BindingID, BindingSetHash: aggregate.Facts.ReconciliationContract.BindingSetHash,
		ProposalHash:    runtimecontract.StableRuntimeCandidateHash(*record.Proposal),
		ObservationHash: "binding-contradiction/" + string(observation.ConstraintImpact) + "/" + diagnostic.Signature + "/" + diagnostic.ObservedRevision,
		OwnerPolicy:     observation.OwnerPolicy, ObservedRevision: diagnostic.ObservedRevision,
	}.Normalized()
	if err := identity.Validate(); err != nil {
		return runtimecontract.RuntimeBindingContradictionIdentity{}, "", runtimecontract.RuntimeCWIBindingRef{}, err
	}
	return identity, coverageID, ref, nil
}

func validateRuntimeBindingContradictionObservation(record runtimecontract.RuntimeCandidateAttemptRecord, feasibility runtimecontract.RuntimeCandidateFeasibilityRecord, observation runtimecontract.RuntimeOwnerFeasibilityObservation) error {
	if record.Proposal == nil || !observation.SealedTargetAuthorizationContradicted() || observation.Feedback == nil || observation.Feedback.OriginalDiagnostic == nil || feasibility.Observation == nil {
		return common.ErrInvalidArgument
	}
	if !reflect.DeepEqual(feasibility.Observation.Clone(), observation) || strings.TrimSpace(feasibility.AdmissibilitySnapshotID) != observation.OwnerPolicy.CatalogSnapshotID {
		return common.ErrInvalidArgument
	}
	manifestHash := strings.TrimSpace(feasibility.CandidateManifestHash)
	if manifestHash == "" || manifestHash != strings.TrimSpace(record.ManifestHash) {
		return common.ErrInvalidArgument
	}
	return nil
}

func runtimeBindingContradictionOperation(record runtimecontract.RuntimeCandidateAttemptRecord, feasibility runtimecontract.RuntimeCandidateFeasibilityRecord, observation runtimecontract.RuntimeOwnerFeasibilityObservation) (runtimecontract.RuntimeOperation, error) {
	operation, found := candidateOperationByID(*record.Proposal, feasibility.OperationID)
	descriptor, descriptorFound := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
	if !found || !descriptorFound || descriptor.OwnerModuleID != observation.OwnerPolicy.OwnerModuleID || len(operation.BindingRefs) != 1 || record.Transaction.FocusID == "" {
		return runtimecontract.RuntimeOperation{}, common.ErrInvalidArgument
	}
	return operation, nil
}

func runtimeBindingContradictionCWI(aggregate RuntimeAggregate, focusID, bindingID string) (common.CoverageWorkItemID, runtimecontract.RuntimeCWIBindingRef, error) {
	var coverageID common.CoverageWorkItemID
	var ref runtimecontract.RuntimeCWIBindingRef
	matches := 0
	for _, item := range aggregate.Facts.Coverage {
		if item.CurrentBindingRef != nil && item.CurrentBindingRef.BindingID == bindingID {
			coverageID, ref = item.ID, item.CurrentBindingRef.Clone()
			matches++
		}
	}
	if matches != 1 || ref.FocusID != focusID || !runtimeSemanticPlanningContainsManifest(aggregate.SemanticPlanning, ref.SemanticManifestHash) {
		return "", runtimecontract.RuntimeCWIBindingRef{}, common.ErrInvalidArgument
	}
	return coverageID, ref, nil
}

func validateRuntimeBindingContradictionReconciliation(aggregate RuntimeAggregate, coverageID common.CoverageWorkItemID, ref runtimecontract.RuntimeCWIBindingRef, operation runtimecontract.RuntimeOperation, observation runtimecontract.RuntimeOwnerFeasibilityObservation) error {
	var matched runtimecontract.RuntimeCWIReconciliationBindingRef
	matches := 0
	for _, binding := range aggregate.Facts.ReconciliationContract.Bindings {
		binding = binding.Normalized()
		if binding.CoverageWorkItemID == coverageID && binding.BindingID == ref.BindingID && binding.BindingHash == ref.BindingHash {
			matched, matches = binding, matches+1
		}
	}
	if matches != 1 || !containsString(matched.OwnerModuleIDs, observation.OwnerPolicy.OwnerModuleID) || !containsString(matched.AllowedOperationKinds, operation.Kind) {
		return common.ErrInvalidArgument
	}
	return nil
}

func runtimeBindingContradictionDiagnostic(aggregate RuntimeAggregate, record runtimecontract.RuntimeCandidateAttemptRecord, operation runtimecontract.RuntimeOperation, observation runtimecontract.RuntimeOwnerFeasibilityObservation) (runtimecontract.RuntimeOriginalDiagnostic, error) {
	feedback := observation.Feedback.Normalized()
	diagnostic := feedback.OriginalDiagnostic.Normalized()
	if feedback.WorkspaceID != aggregate.Facts.WorkspaceID || feedback.RunID != aggregate.Facts.RunID || feedback.AuthoritySnapshotID != record.Transaction.AuthoritySnapshotID || feedback.DispatchID != "" {
		return runtimecontract.RuntimeOriginalDiagnostic{}, common.ErrInvalidArgument
	}
	if diagnostic.OperationID != operation.ID || observation.OwnerPolicy.OwnerModuleID != diagnostic.OwnerModuleID || strings.TrimSpace(diagnostic.ObservedRevision) == "" || len(diagnostic.TargetRefs) == 0 || !sameRuntimeStringSet(operation.TargetRefs, diagnostic.TargetRefs) {
		return runtimecontract.RuntimeOriginalDiagnostic{}, common.ErrInvalidArgument
	}
	return *diagnostic, nil
}

func runtimeSemanticPlanningContainsManifest(ledger runtimecontract.RuntimeSemanticPlanningLedger, manifestHash string) bool {
	manifestHash = strings.TrimSpace(manifestHash)
	if manifestHash == "" {
		return false
	}
	if ledger.Manifest != nil && strings.TrimSpace(ledger.Manifest.Hash) == manifestHash {
		return true
	}
	for _, generation := range ledger.History {
		if generation.Snapshot.Manifest != nil && strings.TrimSpace(generation.Snapshot.Manifest.Hash) == manifestHash {
			return true
		}
	}
	return false
}

func sameRuntimeStringSet(left, right []string) bool {
	left = append([]string(nil), left...)
	right = append([]string(nil), right...)
	sort.Strings(left)
	sort.Strings(right)
	return reflect.DeepEqual(left, right)
}

func candidateOperationByID(candidate runtimecontract.RuntimeCandidate, operationID string) (runtimecontract.RuntimeOperation, bool) {
	operationID = strings.TrimSpace(operationID)
	for _, operation := range candidateRuntimeOperations(candidate) {
		if operation.Normalized().ID == operationID {
			return operation.Normalized(), true
		}
	}
	return runtimecontract.RuntimeOperation{}, false
}

func runtimeOperationTargetsRef(operation runtimecontract.RuntimeOperation, targetRef string) bool {
	targetRef = strings.TrimSpace(targetRef)
	for _, value := range operation.Normalized().TargetRefs {
		if value == targetRef {
			return true
		}
	}
	return false
}

// factsWithoutTargetSnapshot clears exactly the owner resource that will be
// reacquired. It deliberately does not use stale-refresh bookkeeping: no
// previously accepted owner observation was disproved, and the repair/candidate
// contract had simply not requested this input before the feasibility proof.
func factsWithoutTargetSnapshot(facts runtimecontract.RuntimeFacts, request runtimecontract.RuntimeOperationPrerequisiteRequest) runtimecontract.RuntimeFacts {
	prerequisiteID := runtimePrerequisiteID(runtimecontract.RuntimeRequirement{
		Kind: request.Kind, OwnerModuleID: request.OwnerModuleID, SubjectRef: request.SubjectRef, MinimumBytes: request.MinimumBytes,
	})
	snapshots := make([]runtimecontract.TargetSnapshot, 0, len(facts.Resources.TargetSnapshots))
	for _, snapshot := range facts.Resources.TargetSnapshots {
		if strings.TrimSpace(snapshot.TargetRef) != request.SubjectRef {
			snapshots = append(snapshots, snapshot)
		}
	}
	facts.Resources.TargetSnapshots = snapshots
	satisfied := make([]runtimecontract.RuntimePrerequisite, 0, len(facts.Prerequisites.Satisfied))
	for _, prerequisite := range facts.Prerequisites.Satisfied {
		if prerequisite.ID != prerequisiteID {
			satisfied = append(satisfied, prerequisite)
		}
	}
	facts.Prerequisites.Satisfied = satisfied
	observations := make([]runtimecontract.RuntimePrerequisiteObservation, 0, len(facts.Resources.PrerequisiteObservations))
	for _, observation := range facts.Resources.PrerequisiteObservations {
		if observation.PrerequisiteID != prerequisiteID {
			observations = append(observations, observation)
		}
	}
	facts.Resources.PrerequisiteObservations = observations
	return facts.Normalized()
}

func addFeasibilityPrerequisite(facts runtimecontract.RuntimeFacts, request runtimecontract.RuntimeOperationPrerequisiteRequest) runtimecontract.RuntimeFacts {
	request = request.Normalized()
	requirement := runtimecontract.RuntimeRequirement{
		Kind: request.Kind, OwnerModuleID: request.OwnerModuleID, SubjectRef: request.SubjectRef,
		MinimumBytes: request.MinimumBytes, Reason: request.Reason,
	}
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID: runtimePrerequisiteID(requirement), Kind: requirement.Kind, OwnerModuleID: requirement.OwnerModuleID,
		SubjectRef: requirement.SubjectRef, MinimumBytes: requirement.MinimumBytes, Reason: requirement.Reason,
	}
	for _, pending := range facts.Prerequisites.Pending {
		if pending.ID == prerequisite.ID {
			return facts.Normalized()
		}
	}
	facts.Prerequisites.Pending = append(facts.Prerequisites.Pending, prerequisite)
	return facts.Normalized()
}

// invalidateActiveDispatchForPrerequisiteStale closes a sealed dispatch after
// an owner has persisted a no-effect stale receipt. The stale receipt remains
// diagnostic lineage only; it cannot create rejected evidence or repair debt.
func applyReceiptResourceChangeInvalidation(aggregate *RuntimeAggregate, dispatchID, operationID string, receipt runtimecontract.RuntimeOperationReceipt) (bool, error) {
	if aggregate == nil {
		return false, common.ErrInvalidArgument
	}
	receipt = receipt.Normalized()
	if len(receipt.ResourceChanges) == 0 {
		return false, nil
	}
	receiptHash := runtimecontract.StableRuntimeOperationReceiptHash(receipt)
	signature := runtimecontract.StableRuntimeResourceChangeInvalidationSignature(receiptHash, receipt.ResourceChanges)
	if prerequisiteInvalidationExists(*aggregate, signature) {
		return false, nil
	}
	facts, prerequisiteIDs, observationIDs, invalidatedDependencies, err := invalidatePrerequisiteObservationsForChanges(aggregate.Facts, receipt.ResourceChanges)
	if err != nil {
		return false, err
	}
	if len(prerequisiteIDs) == 0 {
		return false, nil
	}
	aggregate.Facts = facts
	aggregate.PrerequisiteInvalidations = append(aggregate.PrerequisiteInvalidations, runtimecontract.RuntimePrerequisiteInvalidationRecord{
		Signature: signature, PrerequisiteIDs: prerequisiteIDs, ObservationIDs: observationIDs,
		InvalidatedDependencies: invalidatedDependencies,
		DispatchID:              strings.TrimSpace(dispatchID), OperationID: strings.TrimSpace(operationID), ReceiptHash: receiptHash,
		ResourceChanges: receipt.ResourceChanges,
	})
	return true, nil
}

func prerequisiteInvalidationExists(aggregate RuntimeAggregate, signature string) bool {
	signature = strings.TrimSpace(signature)
	for _, value := range aggregate.PrerequisiteInvalidations {
		if value.Normalized().Signature == signature {
			return true
		}
	}
	return false
}

// validatePrerequisiteResultFreshness prevents an owner response produced for
// a previous instance of a stable prerequisite ID from satisfying the current
// re-acquisition. Prerequisites intentionally retain their semantic ID after
// invalidation so reducer ordering and owner contracts stay stable; the
// persisted owner observation is consequently the generation fence.
//
// This is an Orchestrator admission rule, not a DTO transition: capability
// owners continue to report only their current observation, while SM-2 decides
// whether that observation may re-enter the aggregate.
func validatePrerequisiteResultFreshness(result PrerequisiteResult, invalidations []runtimecontract.RuntimePrerequisiteInvalidationRecord) error {
	prerequisiteID := strings.TrimSpace(result.PrerequisiteID)
	if prerequisiteID == "" {
		return common.ErrInvalidArgument
	}
	for _, invalidation := range invalidations {
		invalidation = invalidation.Normalized()
		if !containsFreshnessString(invalidation.PrerequisiteIDs, prerequisiteID) {
			continue
		}
		if result.Observation == nil {
			return fmt.Errorf("%w: prerequisite %q was invalidated and requires a fresh owner observation", common.ErrConflict, prerequisiteID)
		}
		observation := result.Observation.Normalized()
		for _, dependency := range observation.Dependencies {
			for _, invalidatedDependency := range invalidation.InvalidatedDependencies {
				invalidatedDependency = invalidatedDependency.Normalized()
				if dependency.Domain == invalidatedDependency.Domain && dependency.OwnerModuleID == invalidatedDependency.OwnerModuleID &&
					dependency.SubjectRef == invalidatedDependency.SubjectRef && dependency.Revision == invalidatedDependency.Revision {
					return fmt.Errorf("%w: prerequisite %q result retains an invalidated owner freshness dependency", common.ErrConflict, prerequisiteID)
				}
			}
			if invalidation.Stale != nil {
				stale := invalidation.Stale.Normalized()
				if dependency.Domain == stale.Domain && dependency.OwnerModuleID == stale.OwnerModuleID &&
					dependency.SubjectRef == stale.SubjectRef && dependency.Revision == stale.ExpectedRevision {
					return fmt.Errorf("%w: prerequisite %q result retains superseded %s revision", common.ErrConflict, prerequisiteID, stale.Domain)
				}
			}
			for _, change := range invalidation.ResourceChanges {
				if runtimecontract.RuntimeFreshnessDependencyIntersectsChange(dependency, change) &&
					dependency.Revision == change.BeforeRevision {
					return fmt.Errorf("%w: prerequisite %q result retains resource revision superseded by durable receipt", common.ErrConflict, prerequisiteID)
				}
			}
		}
	}
	return nil
}

func containsFreshnessString(values []string, value string) bool {
	value = strings.TrimSpace(value)
	for _, candidate := range values {
		if strings.TrimSpace(candidate) == value {
			return true
		}
	}
	return false
}

func invalidatePrerequisiteObservationsForStale(facts runtimecontract.RuntimeFacts, stale runtimecontract.RuntimePrerequisiteStale) (runtimecontract.RuntimeFacts, []string, []string, []runtimecontract.RuntimeFreshnessDependency, error) {
	facts = facts.Normalized()
	stale = stale.Normalized()
	observation, found := runtimecontract.RuntimePrerequisiteObservationForID(facts.Resources.PrerequisiteObservations, stale.PrerequisiteID)
	if !found || observation.ObservationID != stale.ObservationID {
		return runtimecontract.RuntimeFacts{}, nil, nil, nil, fmt.Errorf("%w: stale prerequisite observation is not current", common.ErrConflict)
	}
	matchedDependency := false
	for _, dependency := range observation.Dependencies {
		if dependency.Domain == stale.Domain && dependency.OwnerModuleID == stale.OwnerModuleID && dependency.SubjectRef == stale.SubjectRef && dependency.Revision == stale.ExpectedRevision {
			matchedDependency = true
			break
		}
	}
	if !matchedDependency {
		return runtimecontract.RuntimeFacts{}, nil, nil, nil, fmt.Errorf("%w: stale prerequisite dependency is not current", common.ErrConflict)
	}
	// A stale owner observation proves that the exact dependency revision is no
	// longer authoritative, not merely that the responder which reported it is
	// stale. Retire every current observation with that same typed dependency.
	// This matters for a Git admissibility inventory: it is bound to a Git
	// repository-state snapshot that carries the identical repository revision.
	// Keeping that snapshot would force re-acquisition to reuse the obsolete
	// binding and make the next owner result uncommittable.
	affected := make([]runtimecontract.RuntimePrerequisiteObservation, 0, 1)
	for _, candidate := range facts.Resources.PrerequisiteObservations {
		for _, dependency := range candidate.Normalized().Dependencies {
			if dependency.Domain == stale.Domain && dependency.OwnerModuleID == stale.OwnerModuleID &&
				dependency.SubjectRef == stale.SubjectRef && dependency.Revision == stale.ExpectedRevision {
				affected = append(affected, candidate)
				break
			}
		}
	}
	updated, prerequisiteIDs, observationIDs, dependencies, err := invalidatePrerequisiteObservations(facts, affected)
	if err != nil {
		return runtimecontract.RuntimeFacts{}, nil, nil, nil, err
	}
	return updated, prerequisiteIDs, observationIDs, freshnessDependenciesMatchingStale(dependencies, stale), nil
}

func invalidatePrerequisiteObservationsForChanges(facts runtimecontract.RuntimeFacts, changes []runtimecontract.RuntimeResourceChange) (runtimecontract.RuntimeFacts, []string, []string, []runtimecontract.RuntimeFreshnessDependency, error) {
	facts = facts.Normalized()
	changes = runtimecontract.NormalizeRuntimeResourceChanges(changes)
	affected := make([]runtimecontract.RuntimePrerequisiteObservation, 0)
	for _, observation := range facts.Resources.PrerequisiteObservations {
		for _, dependency := range observation.Dependencies {
			for _, change := range changes {
				if runtimecontract.RuntimeFreshnessDependencyIntersectsChange(dependency, change) {
					affected = append(affected, observation)
					goto nextObservation
				}
			}
		}
	nextObservation:
	}
	if len(affected) == 0 {
		return facts, nil, nil, nil, nil
	}
	updated, prerequisiteIDs, observationIDs, dependencies, err := invalidatePrerequisiteObservations(facts, affected)
	if err != nil {
		return runtimecontract.RuntimeFacts{}, nil, nil, nil, err
	}
	return updated, prerequisiteIDs, observationIDs, freshnessDependenciesIntersectingChanges(dependencies, changes), nil
}

func freshnessDependenciesMatchingStale(values []runtimecontract.RuntimeFreshnessDependency, stale runtimecontract.RuntimePrerequisiteStale) []runtimecontract.RuntimeFreshnessDependency {
	stale = stale.Normalized()
	ret := make([]runtimecontract.RuntimeFreshnessDependency, 0, 1)
	for _, dependency := range values {
		dependency = dependency.Normalized()
		if dependency.Domain == stale.Domain && dependency.OwnerModuleID == stale.OwnerModuleID &&
			dependency.SubjectRef == stale.SubjectRef && dependency.Revision == stale.ExpectedRevision {
			ret = append(ret, dependency)
		}
	}
	return runtimecontract.NormalizeRuntimeFreshnessDependencies(ret)
}

func freshnessDependenciesIntersectingChanges(values []runtimecontract.RuntimeFreshnessDependency, changes []runtimecontract.RuntimeResourceChange) []runtimecontract.RuntimeFreshnessDependency {
	changes = runtimecontract.NormalizeRuntimeResourceChanges(changes)
	ret := make([]runtimecontract.RuntimeFreshnessDependency, 0, len(values))
	for _, dependency := range values {
		for _, change := range changes {
			if runtimecontract.RuntimeFreshnessDependencyIntersectsChange(dependency, change) {
				ret = append(ret, dependency)
				break
			}
		}
	}
	return runtimecontract.NormalizeRuntimeFreshnessDependencies(ret)
}

func invalidatePrerequisiteObservations(facts runtimecontract.RuntimeFacts, observations []runtimecontract.RuntimePrerequisiteObservation) (runtimecontract.RuntimeFacts, []string, []string, []runtimecontract.RuntimeFreshnessDependency, error) {
	facts = facts.Normalized()
	byID := map[string]runtimecontract.RuntimePrerequisiteObservation{}
	for _, observation := range observations {
		observation = observation.Normalized()
		byID[observation.PrerequisiteID] = observation
	}
	if len(byID) == 0 {
		return facts, nil, nil, nil, nil
	}
	satisfied := make(map[string]runtimecontract.RuntimePrerequisite, len(facts.Prerequisites.Satisfied))
	for _, prerequisite := range facts.Prerequisites.Satisfied {
		satisfied[prerequisite.ID] = prerequisite
	}
	prerequisiteIDs := make([]string, 0, len(byID))
	observationIDs := make([]string, 0, len(byID))
	invalidatedDependencies := make([]runtimecontract.RuntimeFreshnessDependency, 0)
	for prerequisiteID, observation := range byID {
		prerequisite, found := satisfied[prerequisiteID]
		if !found || prerequisite.Kind != observation.Kind || prerequisite.OwnerModuleID != observation.OwnerModuleID {
			return runtimecontract.RuntimeFacts{}, nil, nil, nil, fmt.Errorf("%w: observation %q has no current satisfied prerequisite", common.ErrConflict, prerequisiteID)
		}
		if err := removePrerequisiteResourceFact(&facts, prerequisite); err != nil {
			return runtimecontract.RuntimeFacts{}, nil, nil, nil, err
		}
		prerequisiteIDs = append(prerequisiteIDs, prerequisiteID)
		observationIDs = append(observationIDs, observation.ObservationID)
		invalidatedDependencies = append(invalidatedDependencies, observation.Dependencies...)
	}
	// A failure target observed by a capability is only authoritative while the
	// owner catalog revision that produced it remains fresh. The invalidated
	// prerequisite already carries that owner-neutral dependency; do not retain
	// its target as a second, stale source of repair authority.
	invalidateFailureTargetObservations(&facts, byID)
	keptSatisfied := make([]runtimecontract.RuntimePrerequisite, 0, len(facts.Prerequisites.Satisfied))
	for _, prerequisite := range facts.Prerequisites.Satisfied {
		if _, invalidated := byID[prerequisite.ID]; !invalidated {
			keptSatisfied = append(keptSatisfied, prerequisite)
		}
	}
	facts.Prerequisites.Satisfied = keptSatisfied
	for _, prerequisiteID := range prerequisiteIDs {
		facts.Prerequisites.Pending = append(facts.Prerequisites.Pending, satisfied[prerequisiteID])
	}
	keptObservations := make([]runtimecontract.RuntimePrerequisiteObservation, 0, len(facts.Resources.PrerequisiteObservations))
	for _, observation := range facts.Resources.PrerequisiteObservations {
		if _, invalidated := byID[observation.PrerequisiteID]; !invalidated {
			keptObservations = append(keptObservations, observation)
		}
	}
	facts.Resources.PrerequisiteObservations = keptObservations
	facts.Feasibility = nil
	return facts.Normalized(), normalizedFreshnessStrings(prerequisiteIDs), normalizedFreshnessStrings(observationIDs), runtimecontract.NormalizeRuntimeFreshnessDependencies(invalidatedDependencies), nil
}

func invalidateFailureTargetObservations(facts *runtimecontract.RuntimeFacts, invalidated map[string]runtimecontract.RuntimePrerequisiteObservation) {
	if facts == nil || facts.Resources.FailureTarget == nil || facts.Resources.FailureTarget.TargetObservation == nil {
		return
	}
	targetObservation := facts.Resources.FailureTarget.TargetObservation.Normalized()
	if !failureTargetObservationDependsOnInvalidatedOwnerRevision(targetObservation, invalidated) {
		return
	}
	facts.Resources.FailureTarget = nil
	if facts.Failure != nil && sameFailureTargetObservationBinding(facts.Failure.TargetObservation, &targetObservation) {
		facts.Failure.TargetRefs = nil
		facts.Failure.TargetObservation = nil
	}
	// The directive is a sealed SM-2 decision derived from the same stale
	// failure fact. ReduceRuntimeStateWithDirective will rebuild it from the
	// remaining fresh facts after re-acquisition; retaining it would make its
	// target refs an unauthorized fallback.
	if facts.RepairDirective != nil && facts.RepairDirective.CurrentFailure != nil &&
		sameFailureTargetObservationBinding(facts.RepairDirective.CurrentFailure.TargetObservation, &targetObservation) {
		facts.RepairDirective = nil
	}
}

func failureTargetObservationDependsOnInvalidatedOwnerRevision(target runtimecontract.RuntimeFailureTargetObservation, invalidated map[string]runtimecontract.RuntimePrerequisiteObservation) bool {
	target = target.Normalized()
	for _, observation := range invalidated {
		for _, dependency := range observation.Normalized().Dependencies {
			if dependency.OwnerModuleID == target.OwnerModuleID && dependency.Revision == target.ObservedRevision {
				return true
			}
		}
	}
	return false
}

func sameFailureTargetObservationBinding(left *runtimecontract.RuntimeFailureTargetObservation, right *runtimecontract.RuntimeFailureTargetObservation) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	l := left.Normalized()
	r := right.Normalized()
	if l.OwnerModuleID != r.OwnerModuleID || l.SubjectRef != r.SubjectRef || l.TargetRole != r.TargetRole || l.ObservedRevision != r.ObservedRevision ||
		l.DiagnosticClass != r.DiagnosticClass || l.ContentHash != r.ContentHash || !sameRuntimeTargetRefs(l.EvidenceRefs, r.EvidenceRefs) || len(l.TargetRefs) != len(r.TargetRefs) {
		return false
	}
	for idx := range l.TargetRefs {
		if l.TargetRefs[idx] != r.TargetRefs[idx] {
			return false
		}
	}
	return true
}

func normalizedFreshnessStrings(values []string) []string {
	ret := []string{}
	for _, value := range values {
		ret = appendNormalizedString(ret, value)
	}
	return ret
}

func removePrerequisiteResourceFact(facts *runtimecontract.RuntimeFacts, prerequisite runtimecontract.RuntimePrerequisite) error {
	if facts == nil {
		return common.ErrInvalidArgument
	}
	switch prerequisite.Kind {
	case runtimecontract.PrerequisiteKindManifest:
		facts.Resources.Manifest = nil
	case runtimecontract.PrerequisiteKindTargetSnapshot:
		kept := make([]runtimecontract.TargetSnapshot, 0, len(facts.Resources.TargetSnapshots))
		for _, snapshot := range facts.Resources.TargetSnapshots {
			if snapshot.TargetRef != prerequisite.SubjectRef {
				kept = append(kept, snapshot)
			}
		}
		facts.Resources.TargetSnapshots = kept
	case runtimecontract.PrerequisiteKindTargetSelection:
		facts.Resources.TargetSelection = nil
	case runtimecontract.PrerequisiteKindCapabilityState:
		kept := make([]runtimecontract.RuntimeCapabilityStateSnapshot, 0, len(facts.Resources.CapabilityStates))
		for _, snapshot := range facts.Resources.CapabilityStates {
			if snapshot.RequirementID != prerequisite.ID {
				kept = append(kept, snapshot)
			}
		}
		facts.Resources.CapabilityStates = kept
	case runtimecontract.PrerequisiteKindValidationScratch:
		facts.Resources.ValidationScratch = nil
	case runtimecontract.PrerequisiteKindProviderCeiling:
		facts.Resources.ProviderCeiling = nil
	case runtimecontract.PrerequisiteKindOperationAdmissibility:
		kept := make([]runtimecontract.RuntimeOperationAdmissibility, 0, len(facts.Resources.OperationAdmissibilities))
		for _, inventory := range facts.Resources.OperationAdmissibilities {
			if inventory.PrerequisiteID != prerequisite.ID {
				kept = append(kept, inventory)
			}
		}
		facts.Resources.OperationAdmissibilities = kept
	default:
		return fmt.Errorf("freshness invalidation does not support prerequisite kind %q", prerequisite.Kind)
	}
	return nil
}
