package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// RuntimeRepairSurfaceConfirmationPlan is the Orchestrator-owned, read-only
// handoff plan for one validation-owner observation. It is derived from the
// persisted aggregate and never becomes a second state machine or an external
// owner command. A zero value means that no owner query is currently allowed.
type RuntimeRepairSurfaceConfirmationPlan struct {
	RequestID               string
	IssuedAtStateRevision   uint64
	Observation             runtimecontract.RuntimeValidationFailureObservation
	OwnerModuleID           string
	Protocol                runtimecontract.RuntimeRepairSurfaceConfirmationProtocol
	SubjectDomain           runtimecontract.RuntimeResourceDomain
	SubjectRef              string
	ExpectedSubjectRevision string
}

// Required reports whether the current persisted SM-2 authority permits one
// read-only repair-surface query.
func (p RuntimeRepairSurfaceConfirmationPlan) Required() bool {
	return p.RequestID != "" && p.IssuedAtStateRevision != 0 && p.Observation.Validate() == nil && p.OwnerModuleID != "" &&
		p.Protocol != runtimecontract.RuntimeRepairSurfaceConfirmationProtocolNone && runtimecontract.ValidRuntimeResourceDomain(p.SubjectDomain) && p.SubjectRef != "" && p.ExpectedSubjectRevision != ""
}

// CurrentRuntimeRepairSurfaceConfirmationPlan is the sole phase classifier for
// the validation-observation -> resource-owner confirmation handoff.
//
// FailureTarget is mutable target authority, not a progress fence. The exact
// confirmation ledger terminates one observation sequence: stale observations
// may be refreshed after their prerequisites converge, while confirmed or
// rejected observations can never issue another owner query. A current repair
// directive likewise owns the next action and excludes this handoff.
func CurrentRuntimeRepairSurfaceConfirmationPlan(aggregate RuntimeAggregate) (RuntimeRepairSurfaceConfirmationPlan, error) {
	if aggregate.Decision.State != runtimecontract.ConvergenceStateNeedValidation || len(aggregate.Facts.Prerequisites.Pending) != 0 {
		return RuntimeRepairSurfaceConfirmationPlan{}, nil
	}
	plan, refreshLineage, err := runtimeRepairSurfaceConfirmationObligation(aggregate)
	if err != nil || plan.OwnerModuleID == "" {
		return RuntimeRepairSurfaceConfirmationPlan{}, err
	}
	expectedSubjectRevision, err := currentRuntimeRepairSurfaceSubjectRevision(aggregate, plan.OwnerModuleID, plan.SubjectDomain, plan.SubjectRef)
	if err != nil {
		return RuntimeRepairSurfaceConfirmationPlan{}, err
	}
	plan.ExpectedSubjectRevision = expectedSubjectRevision
	plan.IssuedAtStateRevision = aggregate.StateRevision
	plan.RequestID = stableRuntimeRepairSurfaceConfirmationRequestID(plan.Observation, plan.OwnerModuleID, plan.Protocol, plan.SubjectRef, expectedSubjectRevision, aggregate.StateRevision, refreshLineage)
	return plan, nil
}

// runtimeRepairSurfaceConfirmationObligation proves the pending handoff without
// consulting the decision it must help SM-2 derive. It carries no query fence or
// target authority; the public plan adds those only after prerequisites settle.
// Reducer, query admission and candidate guard share this exact receipt/sequence
// classifier, including after a newer validation fails under a refreshed catalog.
func runtimeRepairSurfaceConfirmationObligation(aggregate RuntimeAggregate) (RuntimeRepairSurfaceConfirmationPlan, string, error) {
	if err := runtimecontract.ValidateRuntimeRepairSurfaceConfirmationRecords(aggregate.RepairSurfaceConfirmations); err != nil {
		return RuntimeRepairSurfaceConfirmationPlan{}, "", fmt.Errorf("runtime repair surface confirmation plan: %w", err)
	}
	if aggregate.Termination != nil || aggregate.Decision.Terminal != nil || aggregate.ActiveDispatch != nil || aggregate.Facts.RepairDirective != nil ||
		!runtimeValidationFailureObservationIsTargetless(aggregate.Facts) {
		return RuntimeRepairSurfaceConfirmationPlan{}, "", nil
	}
	if _, determined, err := reduceDeterminedFailure(runtimecontract.RuntimeDecision{Focus: aggregate.Facts.Focus}, aggregate.Facts); err != nil || determined {
		return RuntimeRepairSurfaceConfirmationPlan{}, "", err
	}

	failure := aggregate.Facts.Failure.Normalized()
	observation := failure.ValidationFailureObservation.Clone()
	if err := observation.Validate(); err != nil {
		return RuntimeRepairSurfaceConfirmationPlan{}, "", fmt.Errorf("runtime repair surface confirmation plan observation: %w", err)
	}
	if !sealedValidationFailureObservationMatches(aggregate, observation) || !runtimeValidationFailureObservationHasPersistedReceipt(aggregate, observation) {
		return RuntimeRepairSurfaceConfirmationPlan{}, "", fmt.Errorf("runtime repair surface confirmation obligation has no exact committed validation receipt")
	}
	for _, dispatch := range aggregate.CompletedDispatches {
		if dispatch.ID != observation.DispatchID {
			continue
		}
		operation, found := candidateOperationByID(dispatch.Candidate, observation.OperationID)
		coverageIDs := make([]common.CoverageWorkItemID, 0, len(operation.BindingRefs))
		for _, binding := range dispatch.CoverageBindings {
			if containsString(operation.BindingRefs, binding.ID) {
				coverageIDs = append(coverageIDs, binding.CoverageWorkItemID)
			}
		}
		if !found || dispatch.Transaction.FocusID != aggregate.Facts.Focus.ID || dispatch.Transaction.DefinitionRevision != aggregate.Facts.DefinitionRevision ||
			!confirmedRepairSourceUsesCurrentBinding(aggregate.Facts, operation.BindingRefs, coverageIDs) {
			return RuntimeRepairSurfaceConfirmationPlan{}, "", fmt.Errorf("runtime repair surface confirmation obligation is not bound to the current focus and definition proof")
		}
	}
	refreshLineage := "initial"
	if latest, latestSignature, found := runtimeLatestRepairSurfaceConfirmation(aggregate, observation); found {
		switch latest.Disposition {
		case runtimecontract.RuntimeRepairSurfaceStale:
			// A stale read-only fact is refreshable only after the normal SM-2
			// prerequisite path has returned the aggregate to need_validation.
			// The enclosing record was validated above, so its persisted exact
			// signature is already the canonical focused-proof identity; do not
			// reconstruct that comparison through the broad contract package.
			refreshLineage = latestSignature
			if refreshLineage == "" {
				return RuntimeRepairSurfaceConfirmationPlan{}, "", fmt.Errorf("runtime repair surface confirmation plan stale lineage is absent")
			}
		case runtimecontract.RuntimeRepairSurfaceConfirmed, runtimecontract.RuntimeRepairSurfaceRejected:
			return RuntimeRepairSurfaceConfirmationPlan{}, "", nil
		default:
			return RuntimeRepairSurfaceConfirmationPlan{}, "", fmt.Errorf("runtime repair surface confirmation plan has unsupported disposition %q", latest.Disposition)
		}
	}

	ownerModuleID, protocol, err := currentRuntimeRepairSurfaceConfirmationContract(aggregate, observation)
	if err != nil {
		return RuntimeRepairSurfaceConfirmationPlan{}, "", err
	}
	if ownerModuleID == "" || protocol == runtimecontract.RuntimeRepairSurfaceConfirmationProtocolNone {
		return RuntimeRepairSurfaceConfirmationPlan{}, "", fmt.Errorf("runtime repair surface confirmation plan has no concrete owner protocol")
	}
	descriptor, found := runtimecontract.RuntimeRepairSurfaceConfirmationDescriptorForProtocol(protocol)
	if !found {
		return RuntimeRepairSurfaceConfirmationPlan{}, "", fmt.Errorf("runtime repair surface confirmation plan has no subject for protocol %q", protocol)
	}
	return RuntimeRepairSurfaceConfirmationPlan{
		Observation: observation, OwnerModuleID: ownerModuleID, Protocol: protocol, SubjectDomain: descriptor.ResourceDomain, SubjectRef: descriptor.SubjectRef,
	}, refreshLineage, nil
}

// currentRuntimeRepairSurfaceSubjectRevision returns the one exact current
// resource-owner revision already admitted as SM-2 prerequisite authority.
// Multiple prerequisite observations may depend on the same protocol-bound
// resource subject;
// they agree on one revision or the plan fails closed. The plan may not query
// File from an unbound mutable manifest pointer or catalog snapshot.
func currentRuntimeRepairSurfaceSubjectRevision(aggregate RuntimeAggregate, ownerModuleID string, subjectDomain runtimecontract.RuntimeResourceDomain, subjectRef string) (string, error) {
	revisions := make(map[string]struct{})
	for _, rawObservation := range aggregate.Facts.Resources.PrerequisiteObservations {
		observation := rawObservation.Normalized()
		if observation.OwnerModuleID != ownerModuleID {
			continue
		}
		for _, dependency := range observation.Dependencies {
			dependency = dependency.Normalized()
			if dependency.Domain == subjectDomain && dependency.OwnerModuleID == ownerModuleID &&
				dependency.SubjectRef == subjectRef && dependency.Revision != "" {
				revisions[dependency.Revision] = struct{}{}
				break
			}
		}
	}
	if len(revisions) != 1 {
		return "", fmt.Errorf("runtime repair surface confirmation plan requires one current owner-bound resource-subject revision")
	}
	var revision string
	for value := range revisions {
		revision = value
	}
	return revision, nil
}

// stableRuntimeRepairSurfaceConfirmationRequestID binds one read-only owner
// request to the exact currently persisted confirmation window. It is stable
// across reload of that aggregate, but changes after a stale response and its
// prerequisite-refresh lineage reopen the same validation observation.
func stableRuntimeRepairSurfaceConfirmationRequestID(
	observation runtimecontract.RuntimeValidationFailureObservation,
	ownerModuleID string,
	protocol runtimecontract.RuntimeRepairSurfaceConfirmationProtocol,
	subjectRef string,
	expectedSubjectRevision string,
	stateRevision uint64,
	refreshLineage string,
) string {
	identity := struct {
		Observation             runtimecontract.RuntimeValidationFailureObservation      `json:"observation"`
		OwnerModuleID           string                                                   `json:"ownerModuleId"`
		Protocol                runtimecontract.RuntimeRepairSurfaceConfirmationProtocol `json:"protocol"`
		SubjectRef              string                                                   `json:"subjectRef"`
		ExpectedSubjectRevision string                                                   `json:"expectedSubjectRevision"`
		StateRevision           uint64                                                   `json:"stateRevision"`
		RefreshLineage          string                                                   `json:"refreshLineage"`
	}{observation.Normalized(), ownerModuleID, protocol, subjectRef, expectedSubjectRevision, stateRevision, refreshLineage}
	encoded, _ := json.Marshal(identity)
	digest := sha256.Sum256(encoded)
	return "runtime-repair-surface-confirmation-" + hex.EncodeToString(digest[:])[:20]
}

func stableRuntimeRepairSurfaceObservationSequenceID(observation runtimecontract.RuntimeValidationFailureObservation) string {
	encoded, _ := json.Marshal(observation.Normalized())
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func rejectCandidateBeforeRepairSurfaceConfirmation(aggregate RuntimeAggregate) error {
	obligation, _, err := runtimeRepairSurfaceConfirmationObligation(aggregate)
	if err != nil {
		return err
	}
	if obligation.OwnerModuleID != "" {
		return fmt.Errorf("%w: candidate authority awaits exact repair-owner confirmation", common.ErrConflict)
	}
	return nil
}
