package orchestrator

// Runtime V2 mainline is owned by Biz.

import (
	"context"
	"fmt"
	"sort"
	"strings"

	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const runtimeV2DefaultTargetSnapshotBytes = 256 * 1024

func (m *Biz) prepareRuntimeV2Prerequisites(ctx context.Context, aggregate runtimev2.RuntimeAggregate) (runtimev2.RuntimeAggregate, error) {
	if m == nil || m.bizPtr == nil || m.EventHub() == nil {
		return runtimev2.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	workspace, err := (workspaceEventRepository{hub: m.EventHub()}).Get(ctx, aggregate.Facts.WorkspaceID)
	if err != nil {
		return runtimev2.RuntimeAggregate{}, err
	}
	for aggregate.Decision.Action == runtimecontract.RuntimeActionAcquirePrerequisite {
		pending := append([]runtimecontract.RuntimePrerequisite(nil), aggregate.Facts.Prerequisites.Pending...)
		if len(pending) == 0 {
			return runtimev2.RuntimeAggregate{}, fmt.Errorf("runtime prerequisite action has no pending prerequisite")
		}
		for _, prerequisite := range runtimeV2OrderedPrerequisites(pending) {
			result, resolveErr := m.resolveRuntimeV2Prerequisite(ctx, aggregate, workspace.DeliverableDirectory, prerequisite)
			if resolveErr != nil {
				return runtimev2.RuntimeAggregate{}, resolveErr
			}
			aggregate, err = m.bizPtr.CommitRuntimeV2Prerequisite(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, result)
			if err != nil {
				return runtimev2.RuntimeAggregate{}, err
			}
		}
	}
	return aggregate, nil
}

func (m *Biz) resolveRuntimeV2Prerequisite(
	ctx context.Context,
	aggregate runtimev2.RuntimeAggregate,
	deliverableDirectory string,
	prerequisite runtimecontract.RuntimePrerequisite,
) (runtimev2.PrerequisiteResult, error) {
	commandScope := struct {
		workspaceID common.WorkspaceID
		runID       common.RunID
		snapshotID  string
	}{aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.Decision.AuthoritySnapshot.ID}
	switch prerequisite.Kind {
	case runtimecontract.PrerequisiteKindManifest:
		facts, err := AcquireAndCommitWorkspaceManifestPrerequisite(ctx, m.EventHub(), aggregate.Facts, events.AcquireWorkspaceManifestCommand{
			WorkspaceID: commandScope.workspaceID, RunID: commandScope.runID, AuthoritySnapshotID: commandScope.snapshotID,
			PrerequisiteID: prerequisite.ID, WorkspaceDeliverableDirectory: deliverableDirectory,
		})
		if err != nil {
			return runtimev2.PrerequisiteResult{}, err
		}
		observation, _ := runtimecontract.RuntimePrerequisiteObservationForID(facts.Resources.PrerequisiteObservations, prerequisite.ID)
		return runtimev2.PrerequisiteResult{PrerequisiteID: prerequisite.ID, Manifest: facts.Resources.Manifest, Observation: &observation}, nil
	case runtimecontract.PrerequisiteKindTargetSnapshot:
		maxBytes := prerequisite.MinimumBytes
		if maxBytes <= 0 {
			maxBytes = runtimeV2DefaultTargetSnapshotBytes
		}
		result, err := AcquireTargetSnapshotPrerequisite(ctx, m.EventHub(), events.AcquireTargetSnapshotCommand{
			WorkspaceID: commandScope.workspaceID, RunID: commandScope.runID, AuthoritySnapshotID: commandScope.snapshotID,
			PrerequisiteID: prerequisite.ID, WorkspaceDeliverableDirectory: deliverableDirectory,
			TargetRef: prerequisite.SubjectRef, MaxBytes: maxBytes,
		})
		if err != nil {
			return runtimev2.PrerequisiteResult{}, err
		}
		return runtimev2.PrerequisiteResult{PrerequisiteID: prerequisite.ID, TargetSnapshot: &result.Snapshot, Observation: result.Observation}, nil
	case runtimecontract.PrerequisiteKindFailureTarget:
		facts, err := ResolveFailureTargetPrerequisite(aggregate.Facts, prerequisite.ID)
		if err != nil {
			return runtimev2.PrerequisiteResult{}, err
		}
		return runtimev2.PrerequisiteResult{PrerequisiteID: prerequisite.ID, FailureTarget: facts.Resources.FailureTarget}, nil
	case runtimecontract.PrerequisiteKindValidationScratch:
		facts, err := AcquireValidationScratchPrerequisite(aggregate.Facts, prerequisite.ID)
		if err != nil {
			return runtimev2.PrerequisiteResult{}, err
		}
		return runtimev2.PrerequisiteResult{PrerequisiteID: prerequisite.ID, ValidationScratch: facts.Resources.ValidationScratch}, nil
	case runtimecontract.PrerequisiteKindProviderCeiling:
		facts, err := AcquireAndCommitProviderCeilingPrerequisite(ctx, m.EventHub(), aggregate.Facts, events.AcquireProviderCeilingCommand{
			WorkspaceID: commandScope.workspaceID, RunID: commandScope.runID, AuthoritySnapshotID: commandScope.snapshotID,
			PrerequisiteID: prerequisite.ID, Target: aggregate.Facts.Activation.ProviderTarget, MinimumBytes: prerequisite.MinimumBytes,
		})
		if err != nil {
			return runtimev2.PrerequisiteResult{}, err
		}
		observation, ok := runtimecontract.RuntimePrerequisiteObservationForID(facts.Resources.PrerequisiteObservations, prerequisite.ID)
		if !ok {
			return runtimev2.PrerequisiteResult{}, fmt.Errorf("provider ceiling prerequisite %q did not return an observation", prerequisite.ID)
		}
		return runtimev2.PrerequisiteResult{PrerequisiteID: prerequisite.ID, ProviderCeiling: facts.Resources.ProviderCeiling, Observation: &observation}, nil
	case runtimecontract.PrerequisiteKindCapability:
		if prerequisite.OwnerModuleID == "" {
			return runtimev2.PrerequisiteResult{}, fmt.Errorf("runtime capability prerequisite %q has no owner module", prerequisite.ID)
		}
		facts, err := AcquireAndCommitRuntimeCapabilitiesPrerequisite(ctx, m.EventHub(), aggregate.Facts, events.AcquireRuntimeCapabilitiesCommand{
			WorkspaceID: commandScope.workspaceID, RunID: commandScope.runID, AuthoritySnapshotID: commandScope.snapshotID,
			PrerequisiteID: prerequisite.ID, ModuleID: prerequisite.OwnerModuleID,
			CapabilityClass: prerequisite.CapabilityClass, RequiredKinds: prerequisite.RequiredCapabilities,
		})
		if err != nil {
			return runtimev2.PrerequisiteResult{}, err
		}
		capabilities := facts.Capabilities
		var observation *runtimecontract.RuntimePrerequisiteObservation
		if value, observed := runtimecontract.RuntimePrerequisiteObservationForID(facts.Resources.PrerequisiteObservations, prerequisite.ID); observed {
			observation = &value
		}
		return runtimev2.PrerequisiteResult{PrerequisiteID: prerequisite.ID, Capabilities: &capabilities, Observation: observation}, nil
	case runtimecontract.PrerequisiteKindOperationAdmissibility:
		binding, bindingErr := runtimeV2CapabilityStateBinding(aggregate.Facts, prerequisite.OwnerModuleID)
		if bindingErr != nil {
			return runtimev2.PrerequisiteResult{}, bindingErr
		}
		result, err := AcquireRuntimeOperationAdmissibilityPrerequisite(ctx, m.EventHub(), events.AcquireRuntimeOperationAdmissibilityCommand{
			WorkspaceID: commandScope.workspaceID, RunID: commandScope.runID, AuthoritySnapshotID: commandScope.snapshotID,
			PrerequisiteID: prerequisite.ID, OwnerModuleID: prerequisite.OwnerModuleID,
			WorkspaceDeliverableDirectory: deliverableDirectory, RequiredKinds: append([]string(nil), prerequisite.RequiredCapabilities...),
			RequiredTargetRefs: append([]string(nil), aggregate.Decision.Focus.TargetRefs...), CapabilityStateBinding: binding,
			MaxItems: aggregate.Facts.ConvergencePolicy.MaxCandidateContextItems,
		})
		if err != nil {
			return runtimev2.PrerequisiteResult{}, err
		}
		admissibility := result.Admissibility
		return runtimev2.PrerequisiteResult{PrerequisiteID: prerequisite.ID, OperationAdmissibility: &admissibility, Observation: result.Observation}, nil
	case runtimecontract.PrerequisiteKindCapabilityState:
		state, err := AcquireGitCapabilityStatePrerequisite(ctx, m.EventHub(), prerequisite.OwnerModuleID, events.AcquireGitCapabilityStateCommand{
			WorkspaceID: commandScope.workspaceID, RunID: commandScope.runID, AuthoritySnapshotID: commandScope.snapshotID,
			PrerequisiteID: prerequisite.ID, WorkspaceDeliverableDirectory: deliverableDirectory, CapabilityStateKind: prerequisite.CapabilityStateKind, SubjectRef: prerequisite.SubjectRef,
			AcquisitionKey: stableRuntimeV2CapabilityStateAcquisitionKey(commandScope.workspaceID, commandScope.runID, prerequisite.ID, runtimeV2CapabilityStateAcquisitionGeneration(aggregate.Facts.Recovery)),
		})
		if err != nil {
			return runtimev2.PrerequisiteResult{}, err
		}
		return runtimev2.PrerequisiteResult{PrerequisiteID: prerequisite.ID, CapabilityState: &state.Snapshot, Observation: state.Observation}, nil
	default:
		return runtimev2.PrerequisiteResult{}, fmt.Errorf("unsupported runtime prerequisite kind %q", prerequisite.Kind)
	}
}

// runtimeV2OrderedPrerequisites makes an owner capability-state acquisition
// available before that same owner's admissibility acquisition. It does not
// encode Git sequence semantics: it only preserves the generic fact dependency
// that an opaque binding must already be persisted before another prerequisite
// can reference it.
func runtimeV2OrderedPrerequisites(values []runtimecontract.RuntimePrerequisite) []runtimecontract.RuntimePrerequisite {
	ret := append([]runtimecontract.RuntimePrerequisite(nil), values...)
	sort.SliceStable(ret, func(i, j int) bool {
		rank := func(value runtimecontract.RuntimePrerequisite) int {
			switch value.Kind {
			case runtimecontract.PrerequisiteKindCapabilityState:
				return 0
			case runtimecontract.PrerequisiteKindOperationAdmissibility:
				return 1
			default:
				return 0
			}
		}
		return rank(ret[i]) < rank(ret[j])
	})
	return ret
}

// runtimeV2CapabilityStateBinding returns the only opaque acquisition binding
// an owner admissibility request may cite. Only Git currently declares this
// repository-state relationship; all other owners remain unbound.
func runtimeV2CapabilityStateBinding(facts runtimecontract.RuntimeFacts, owner string) (string, error) {
	if owner != runtimecontract.RuntimeModuleGit {
		return "", nil
	}
	for _, snapshot := range facts.Resources.CapabilityStates {
		if snapshot.OwnerModuleID != owner || snapshot.CapabilityStateKind != "repository_state" {
			continue
		}
		binding := strings.TrimSpace(snapshot.AcquisitionBinding)
		if binding == "" {
			binding = strings.TrimSpace(snapshot.AcquisitionKey)
		}
		if binding != "" {
			return binding, nil
		}
	}
	return "", fmt.Errorf("runtime operation admissibility owner %q has no committed capability-state acquisition binding", owner)
}

func stableRuntimeV2CapabilityStateAcquisitionKey(workspaceID common.WorkspaceID, runID common.RunID, prerequisiteID string, refreshGeneration int) string {
	return fmt.Sprintf("capability-state/%s/%s/%s/%d", workspaceID, runID, strings.TrimSpace(prerequisiteID), refreshGeneration)
}

// runtimeV2CapabilityStateAcquisitionGeneration advances whenever an event
// invalidates capability-state freshness. Both read-only stale recovery and
// accepted owner resource changes require a new owner query; unrelated retry
// ledgers must not change the durable acquisition key.
func runtimeV2CapabilityStateAcquisitionGeneration(recovery runtimecontract.RuntimeRecoveryLedger) int {
	recovery = recovery.Normalized()
	return recovery.PrerequisiteRefreshes + recovery.PrerequisiteResourceChangeRefreshes
}

func (m *Biz) requestAndSealRuntimeV2Candidate(
	ctx context.Context,
	aggregate runtimev2.RuntimeAggregate,
	stablePrefixHash string,
	candidateAttempt int,
) (runtimev2.RuntimeAggregate, error) {
	if m == nil || m.bizPtr == nil || m.EventHub() == nil {
		return runtimev2.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	switch aggregate.Decision.Action {
	case runtimecontract.RuntimeActionProduceStateChange, runtimecontract.RuntimeActionProduceValidation, runtimecontract.RuntimeActionProduceAssessment, runtimecontract.RuntimeActionRequestGovernance:
	default:
		return runtimev2.RuntimeAggregate{}, fmt.Errorf("runtime action %s does not request an LLM candidate", aggregate.Decision.Action)
	}
	if err := runtimeV2EnsureCandidateFeasible(aggregate); err != nil {
		return runtimev2.RuntimeAggregate{}, err
	}
	aggregate, err := m.bizPtr.BeginRuntimeV2CandidateAttempt(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, candidateAttempt, stablePrefixHash)
	if err != nil {
		return runtimev2.RuntimeAggregate{}, err
	}
	transaction := aggregate.Decision.AuthorityTransaction()
	candidate, proposalPersisted := runtimeV2PendingCandidateProposal(aggregate, candidateAttempt)
	if !proposalPersisted {
		previousFailure, previousErr := runtimeV2PreviousCandidateFailure(aggregate, candidateAttempt)
		if previousErr != nil {
			return runtimev2.RuntimeAggregate{}, previousErr
		}
		response, requestErr := RequestRuntimeCandidate(ctx, m.EventHub(), events.RequestRuntimeCandidateCommand{
			WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
			Decision: aggregate.Decision, Transaction: transaction, CandidateAttempt: candidateAttempt,
			PreviousFailure:  previousFailure,
			StablePrefixHash: stablePrefixHash, DefinitionContext: aggregate.Definition,
			CandidateContext: runtimecontract.RuntimeCandidateContextForDecision(
				aggregate.Facts,
				aggregate.Decision,
				runtimev2.ActiveRejectedOperationShapes(aggregate),
			),
		})
		if requestErr != nil {
			return m.bizPtr.FailRuntimeV2CandidateAttempt(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, candidateAttempt, runtimecontract.RuntimeCandidateFailure{
				Class: runtimecontract.RuntimeCandidateFailureClassInternal, Reason: requestErr.Error(),
			})
		}
		if response.Transaction != transaction {
			return m.bizPtr.FailRuntimeV2CandidateAttempt(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, candidateAttempt, runtimecontract.RuntimeCandidateFailure{
				Class: runtimecontract.RuntimeCandidateFailureClassInternal, Reason: "runtime candidate response changed the sealed authority transaction",
			})
		}
		if response.Stale != nil {
			return m.bizPtr.InvalidateRuntimeV2CandidateAttemptForPrerequisiteStale(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, candidateAttempt, *response.Stale)
		}
		if response.Failure != nil {
			if response.Failure.Class == runtimecontract.RuntimeCandidateFailureClassProviderTransient {
				recovered, consumed, budgetErr := m.bizPtr.ConsumeRuntimeV2RecoveryBudget(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, runtimecontract.RuntimeRecoveryBudgetProviderTransport)
				if budgetErr != nil {
					return runtimev2.RuntimeAggregate{}, budgetErr
				}
				if consumed {
					return recovered, nil
				}
				return m.bizPtr.RecordRuntimeV2FailureFact(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, runtimecontract.FailureFact{
					Class: runtimecontract.FailureClassProviderTransient, Code: "provider_transport_exhausted",
					Signature: "provider-transport-exhausted", Message: strings.TrimSpace(response.Failure.Reason),
				})
			}
			failed, failErr := m.bizPtr.FailRuntimeV2CandidateAttempt(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, candidateAttempt, *response.Failure)
			if failErr != nil {
				return runtimev2.RuntimeAggregate{}, failErr
			}
			if fact, ok := runtimeV2ProviderGovernanceFailure(*response.Failure); ok {
				return m.bizPtr.RecordRuntimeV2FailureFact(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, fact)
			}
			return failed, nil
		}
		proposal, proposalErr := m.bizPtr.AdmitRuntimeV2CandidateProposal(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, candidateAttempt, response.Candidate)
		if proposalErr != nil {
			return runtimev2.RuntimeAggregate{}, proposalErr
		}
		aggregate = proposal.Aggregate
		if !proposal.Admission.Accepted() {
			return aggregate, nil
		}
		candidate, proposalPersisted = runtimeV2PendingCandidateProposal(aggregate, candidateAttempt)
		if !proposalPersisted {
			return runtimev2.RuntimeAggregate{}, fmt.Errorf("persisted runtime candidate proposal is missing")
		}
	}
	if len(aggregate.Facts.Resources.OperationAdmissibilities) > 0 {
		workspace, workspaceErr := (workspaceEventRepository{hub: m.EventHub()}).Get(ctx, aggregate.Facts.WorkspaceID)
		if workspaceErr != nil {
			return m.bizPtr.FailRuntimeV2CandidateAttempt(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, candidateAttempt, runtimecontract.RuntimeCandidateFailure{
				Class: runtimecontract.RuntimeCandidateFailureClassInternal, Code: "workspace_resolution_failed", Reason: workspaceErr.Error(),
			})
		}
		if feasibilityFailure, stale, probeErr := m.probeRuntimeV2CandidateFeasibility(ctx, aggregate, workspace.DeliverableDirectory, candidate, candidateAttempt); probeErr != nil {
			return m.bizPtr.FailRuntimeV2CandidateAttempt(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, candidateAttempt, runtimecontract.RuntimeCandidateFailure{
				Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "operation_feasibility_probe_invalid", Reason: probeErr.Error(),
			})
		} else if stale != nil {
			return m.bizPtr.InvalidateRuntimeV2CandidateAttemptForPrerequisiteStale(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, candidateAttempt, *stale)
		} else if feasibilityFailure != nil {
			return m.bizPtr.FailRuntimeV2CandidateAttempt(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, candidateAttempt, *feasibilityFailure)
		}
	}
	sealed, sealErr := m.bizPtr.AdmitAndSealRuntimeV2CandidateAttempt(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, candidateAttempt, candidate)
	if sealErr != nil {
		return runtimev2.RuntimeAggregate{}, sealErr
	}
	return sealed.Aggregate, nil
}

func runtimeV2PendingCandidateProposal(aggregate runtimev2.RuntimeAggregate, attempt int) (runtimecontract.RuntimeCandidate, bool) {
	contractID := runtimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.Decision.AuthorityTransaction())
	for _, record := range aggregate.CandidateAttempts {
		if runtimecontract.CandidateAttemptAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, record) != contractID || record.Attempt != attempt || record.State != runtimecontract.RuntimeCandidateAttemptStatePending || record.Proposal == nil {
			continue
		}
		return *record.Proposal, true
	}
	return runtimecontract.RuntimeCandidate{}, false
}

func (m *Biz) probeRuntimeV2CandidateFeasibility(ctx context.Context, aggregate runtimev2.RuntimeAggregate, workspaceDirectory string, candidate runtimecontract.RuntimeCandidate, candidateAttempt int) (*runtimecontract.RuntimeCandidateFailure, *runtimecontract.RuntimePrerequisiteStale, error) {
	if m == nil || m.bizPtr == nil || m.EventHub() == nil {
		return nil, nil, common.ErrInvalidArgument
	}
	// Assessment candidates and governance candidates contain no executable
	// owner operation. Their authority is validated by their own seal contract,
	// so an unrelated persisted inventory must not make assessment impossible.
	if candidate.StateChange == nil && candidate.Validation == nil {
		return nil, nil, nil
	}
	if len(aggregate.Facts.Resources.OperationAdmissibilities) == 0 {
		if runtimeV2RequiresOperationAdmissibility(aggregate.Facts.Prerequisites) {
			return nil, nil, fmt.Errorf("runtime candidate has no persisted owner admissibility inventory")
		}
		// Focused reducer/receipt fixtures can construct facts directly without a
		// bootstrap prerequisite. Production activation always seals this
		// prerequisite, so such fixtures cannot bypass the production gate.
		return nil, nil, nil
	}
	operations, err := orderedRuntimeV2CandidateOperations(candidate)
	if err != nil {
		return nil, nil, err
	}
	for _, operation := range operations {
		descriptor, ok := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
		if !ok || descriptor.OwnerModuleID == "" {
			return nil, nil, fmt.Errorf("candidate operation %q has no canonical owner", operation.Kind)
		}
		inventory, found := runtimeV2OperationAdmissibilityForKind(aggregate.Facts.Resources.OperationAdmissibilities, descriptor.OwnerModuleID, operation.Kind)
		if !found {
			return nil, nil, fmt.Errorf("candidate operation %q has no persisted owner admissibility inventory", operation.Kind)
		}
		if persisted, found := runtimeV2CandidateFeasibilityRecord(aggregate, candidateAttempt, operation.ID); found {
			if persisted.AdmissibilitySnapshotID != inventory.SnapshotID || persisted.AcquisitionBinding != inventory.AcquisitionBinding {
				return nil, nil, fmt.Errorf("candidate operation %q feasibility snapshot does not match persisted owner inventory", operation.ID)
			}
			if persisted.Feasible {
				continue
			}
			if persisted.Stale != nil {
				return nil, persisted.Stale, nil
			}
			return runtimeCandidateFailureFromFeasibility(persisted), nil, nil
		}
		observation, observed := runtimecontract.RuntimePrerequisiteObservationForID(aggregate.Facts.Resources.PrerequisiteObservations, inventory.PrerequisiteID)
		if !observed {
			return nil, nil, fmt.Errorf("candidate operation %q has no freshness observation for owner admissibility", operation.Kind)
		}
		response, probeErr := ProbeRuntimeOperationFeasibility(ctx, m.EventHub(), events.ProbeRuntimeOperationFeasibilityCommand{
			WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, Transaction: aggregate.Decision.AuthorityTransaction(),
			WorkspaceDeliverableDirectory: workspaceDirectory, Operation: operation, Admissibility: inventory,
			CapabilityStateBinding: inventory.AcquisitionBinding, Observation: &observation,
		})
		if probeErr != nil {
			return nil, nil, probeErr
		}
		recorded, recordErr := m.bizPtr.RecordRuntimeV2CandidateFeasibility(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, candidateAttempt, runtimecontract.RuntimeCandidateFeasibilityRecord{
			OperationID: operation.ID, AdmissibilitySnapshotID: inventory.SnapshotID, AcquisitionBinding: inventory.AcquisitionBinding,
			Feasible: response.Feasible, Failure: response.Failure, Stale: response.Stale,
		})
		if recordErr != nil {
			return nil, nil, recordErr
		}
		aggregate = recorded
		if response.Feasible {
			continue
		}
		if response.Stale != nil {
			return nil, response.Stale, nil
		}
		return runtimeCandidateFailureFromFeasibility(runtimecontract.RuntimeCandidateFeasibilityRecord{
			OperationID: operation.ID, AdmissibilitySnapshotID: inventory.SnapshotID, AcquisitionBinding: inventory.AcquisitionBinding,
			Feasible: false, Failure: response.Failure,
		}), nil, nil
	}
	return nil, nil, nil
}

func runtimeV2CandidateFeasibilityRecord(aggregate runtimev2.RuntimeAggregate, attempt int, operationID string) (runtimecontract.RuntimeCandidateFeasibilityRecord, bool) {
	contractID := runtimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.Decision.AuthorityTransaction())
	for _, candidateAttempt := range aggregate.CandidateAttempts {
		if runtimecontract.CandidateAttemptAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, candidateAttempt) != contractID || candidateAttempt.Attempt != attempt {
			continue
		}
		for _, record := range candidateAttempt.Feasibilities {
			if record.OperationID == operationID {
				return record, true
			}
		}
	}
	return runtimecontract.RuntimeCandidateFeasibilityRecord{}, false
}

func runtimeCandidateFailureFromFeasibility(record runtimecontract.RuntimeCandidateFeasibilityRecord) *runtimecontract.RuntimeCandidateFailure {
	if record.Feasible || record.Failure == nil {
		return nil
	}
	failure := record.Failure
	return &runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassInfeasible,
		Code:  failure.Code, Signature: failure.Signature, Reason: failure.Message,
		TargetRefs: append([]string(nil), failure.TargetRefs...),
	}
}

func runtimeV2RequiresOperationAdmissibility(state runtimecontract.PrerequisiteState) bool {
	for _, prerequisite := range append(append([]runtimecontract.RuntimePrerequisite(nil), state.Pending...), state.Satisfied...) {
		if prerequisite.Kind == runtimecontract.PrerequisiteKindOperationAdmissibility {
			return true
		}
	}
	return false
}

func runtimeV2OperationAdmissibilityForKind(values []runtimecontract.RuntimeOperationAdmissibility, owner, kind string) (runtimecontract.RuntimeOperationAdmissibility, bool) {
	canonical := runtimecontract.CanonicalRuntimeBlockKind(kind)
	for _, value := range values {
		value = value.Normalized()
		if value.OwnerModuleID != owner {
			continue
		}
		for _, available := range value.OperationKinds {
			if available == canonical {
				return value, true
			}
		}
	}
	return runtimecontract.RuntimeOperationAdmissibility{}, false
}
func orderedRuntimeV2CandidateOperations(candidate runtimecontract.RuntimeCandidate) ([]runtimecontract.RuntimeOperation, error) {
	operations := []runtimecontract.RuntimeOperation{}
	if candidate.StateChange != nil {
		for _, value := range candidate.StateChange.Operations {
			operations = append(operations, value.Operation.Normalized())
		}
	} else if candidate.Validation != nil {
		for _, value := range candidate.Validation.Operations {
			operations = append(operations, value.Operation.Normalized())
		}
	}
	if len(operations) == 0 {
		return nil, fmt.Errorf("runtime candidate has no executable operations")
	}
	byID := make(map[string]runtimecontract.RuntimeOperation, len(operations))
	for _, operation := range operations {
		if operation.ID == "" {
			return nil, fmt.Errorf("runtime candidate operation requires id")
		}
		byID[operation.ID] = operation
	}
	completed := map[string]struct{}{}
	ordered := make([]runtimecontract.RuntimeOperation, 0, len(operations))
	for len(ordered) < len(byID) {
		ready := []string{}
		for id, operation := range byID {
			if _, done := completed[id]; done {
				continue
			}
			allDependenciesComplete := true
			for _, dependency := range operation.DependsOn {
				if _, exists := byID[dependency]; !exists {
					return nil, fmt.Errorf("runtime candidate operation %q depends on unknown operation %q", id, dependency)
				}
				if _, done := completed[dependency]; !done {
					allDependenciesComplete = false
					break
				}
			}
			if allDependenciesComplete {
				ready = append(ready, id)
			}
		}
		if len(ready) == 0 {
			return nil, fmt.Errorf("runtime candidate operation dependency graph contains a cycle")
		}
		sort.Strings(ready)
		for _, id := range ready {
			ordered = append(ordered, byID[id])
			completed[id] = struct{}{}
		}
	}
	return ordered, nil
}

func runtimeV2PreviousCandidateFailure(aggregate runtimev2.RuntimeAggregate, candidateAttempt int) (*runtimecontract.RuntimeCandidateFailure, error) {
	if candidateAttempt == 0 {
		return nil, nil
	}
	if candidateAttempt != 1 {
		return nil, fmt.Errorf("runtime candidate attempt must be 0 or 1")
	}
	contractID := runtimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.Decision.AuthorityTransaction())
	for idx := range aggregate.CandidateAttempts {
		record := aggregate.CandidateAttempts[idx]
		if runtimecontract.CandidateAttemptAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, record) != contractID || record.Attempt != 0 {
			continue
		}
		if record.State != runtimecontract.RuntimeCandidateAttemptStateFailed || record.Failure == nil {
			return nil, fmt.Errorf("runtime candidate retry requires a failed first attempt")
		}
		failure := *record.Failure
		return &failure, nil
	}
	return nil, fmt.Errorf("runtime candidate retry has no first attempt failure")
}

func (m *Biz) requestAndSealRuntimeV2CandidateWithRetry(
	ctx context.Context,
	aggregate runtimev2.RuntimeAggregate,
	stablePrefixHash string,
) (runtimev2.RuntimeAggregate, error) {
	for range 2 {
		if aggregate.ActiveDispatch != nil {
			if aggregate.ActiveDispatch.Candidate.Governance != nil {
				return m.prepareRuntimeV2GovernanceRequest(ctx, aggregate)
			}
			return aggregate, nil
		}
		if aggregate.Decision.Action == runtimecontract.RuntimeActionFail || aggregate.Decision.Action == runtimecontract.RuntimeActionComplete {
			return aggregate, nil
		}
		// A candidate proposal or its read-only owner feasibility probe may
		// invalidate a mutable prerequisite. That transition belongs to SM-2;
		// return it to the outer drive so prerequisite acquisition runs before
		// any new SM-3 candidate attempt is requested.
		if aggregate.Decision.Action == runtimecontract.RuntimeActionAcquirePrerequisite {
			return aggregate, nil
		}
		if aggregate.Decision.Action == runtimecontract.RuntimeActionRequestGovernance {
			next, err := m.sealRuntimeV2GovernanceCandidate(ctx, aggregate, stablePrefixHash)
			if err != nil {
				return runtimev2.RuntimeAggregate{}, err
			}
			aggregate = next
			continue
		}
		attempt, ok := nextRuntimeV2CandidateAttempt(aggregate)
		if !ok {
			if runtimeV2CandidateContractExhausted(aggregate) {
				return m.bizPtr.RecordRuntimeV2FailureFact(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, runtimecontract.FailureFact{
					Class: runtimecontract.FailureClassCandidateExhausted, Code: "candidate_retry_exhausted",
					Signature: "candidate-retry-exhausted", Message: "candidate authority contract exhausted its sealed attempt 0/1 budget",
				})
			}
			return runtimev2.RuntimeAggregate{}, fmt.Errorf("%w: no candidate attempt is available for authority snapshot %s contract %s", common.ErrConflict, aggregate.Decision.AuthoritySnapshot.ID, runtimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.Decision.AuthorityTransaction()))
		}
		next, err := m.requestAndSealRuntimeV2Candidate(ctx, aggregate, stablePrefixHash, attempt)
		if err != nil {
			return runtimev2.RuntimeAggregate{}, err
		}
		aggregate = next
	}
	return aggregate, nil
}

func runtimeV2ProviderGovernanceFailure(failure runtimecontract.RuntimeCandidateFailure) (runtimecontract.FailureFact, bool) {
	class := runtimecontract.FailureClass("")
	code := ""
	switch failure.Class {
	case runtimecontract.RuntimeCandidateFailureClassProviderAuthentication:
		class = runtimecontract.FailureClassProviderAuthentication
		code = "provider_authentication"
	case runtimecontract.RuntimeCandidateFailureClassProviderConfiguration:
		class = runtimecontract.FailureClassProviderConfiguration
		code = "provider_configuration"
	default:
		return runtimecontract.FailureFact{}, false
	}
	return runtimecontract.FailureFact{Class: class, Code: code, Signature: code, Message: strings.TrimSpace(failure.Reason)}, true
}

func (m *Biz) sealRuntimeV2GovernanceCandidate(ctx context.Context, aggregate runtimev2.RuntimeAggregate, stablePrefixHash string) (runtimev2.RuntimeAggregate, error) {
	attempt, ok := nextRuntimeV2CandidateAttempt(aggregate)
	if !ok {
		return runtimev2.RuntimeAggregate{}, fmt.Errorf("%w: no governance candidate attempt is available", common.ErrConflict)
	}
	dependency := runtimeV2GovernanceDependency(aggregate)
	if strings.TrimSpace(dependency.Kind) == "" || strings.TrimSpace(dependency.Reason) == "" {
		return runtimev2.RuntimeAggregate{}, fmt.Errorf("runtime governance decision has no typed dependency")
	}
	pending, err := m.bizPtr.BeginRuntimeV2CandidateAttempt(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, attempt, stablePrefixHash)
	if err != nil {
		return runtimev2.RuntimeAggregate{}, err
	}
	sealed, err := m.bizPtr.AdmitAndSealRuntimeV2CandidateAttempt(ctx, pending.Facts.WorkspaceID, pending.Facts.RunID, attempt, runtimecontract.RuntimeCandidate{
		Governance: &runtimecontract.GovernanceCandidate{ExternalDependency: dependency},
	})
	if err != nil {
		return runtimev2.RuntimeAggregate{}, err
	}
	return sealed.Aggregate, nil
}

func runtimeV2GovernanceDependency(aggregate runtimev2.RuntimeAggregate) runtimecontract.ExternalDependency {
	facts := aggregate.Facts.Normalized()
	reason := "runtime requires governance"
	kind := string(runtimecontract.GovernanceDecision)
	subject := facts.Focus.ID
	// Operation-scoped governance already has an exact persisted dependency in
	// the suspended binding. Preserve it when sealing the governance candidate;
	// falling back to the generic decision kind would make the ticket impossible
	// to bind back to the suspended approval/config/external operation.
	for idx := len(aggregate.OperationGovernanceBindings) - 1; idx >= 0; idx-- {
		binding := aggregate.OperationGovernanceBindings[idx].Normalized()
		if binding.State != runtimecontract.RuntimeOperationGovernanceBindingSuspended {
			continue
		}
		if facts.Failure != nil {
			reason = firstNonEmptyString(facts.Failure.Message, reason)
		}
		return runtimecontract.ExternalDependency{
			Kind: strings.TrimSpace(binding.DependencyKind), SubjectRef: strings.TrimSpace(binding.SubjectRef), Reason: strings.TrimSpace(reason),
		}
	}
	if facts.Failure != nil {
		reason = firstNonEmptyString(facts.Failure.Message, reason)
		if len(facts.Failure.TargetRefs) > 0 {
			subject = facts.Failure.TargetRefs[0]
		}
		switch facts.Failure.Class {
		case runtimecontract.FailureClassProviderAuthentication, runtimecontract.FailureClassProviderConfiguration:
			kind = string(runtimecontract.GovernanceConfiguration)
			if facts.Resources.ProviderCeiling != nil {
				target := facts.Resources.ProviderCeiling.Target.Normalized()
				subject = target.ProviderID + "/" + target.ModelID
			}
		case runtimecontract.FailureClassOperationOutcomeUnknown:
			kind = string(runtimecontract.GovernanceExternal)
		}
	} else if facts.Feasibility != nil {
		reason = firstNonEmptyString(facts.Feasibility.Reason, reason)
	}
	return runtimecontract.ExternalDependency{Kind: kind, SubjectRef: strings.TrimSpace(subject), Reason: strings.TrimSpace(reason)}
}

func nextRuntimeV2CandidateAttempt(aggregate runtimev2.RuntimeAggregate) (int, bool) {
	contractID := runtimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.Decision.AuthorityTransaction())
	var first *runtimecontract.RuntimeCandidateAttemptRecord
	var second *runtimecontract.RuntimeCandidateAttemptRecord
	matched := 0
	for idx := range aggregate.CandidateAttempts {
		record := &aggregate.CandidateAttempts[idx]
		if runtimecontract.CandidateAttemptAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, *record) != contractID {
			continue
		}
		matched++
		if record.Attempt == 0 {
			if first != nil {
				return 0, false
			}
			first = record
		} else if record.Attempt == 1 {
			if second != nil {
				return 0, false
			}
			second = record
		}
	}
	if matched > 2 {
		return 0, false
	}
	if first == nil {
		return 0, true
	}
	switch first.State {
	case runtimecontract.RuntimeCandidateAttemptStatePending:
		return 0, true
	case runtimecontract.RuntimeCandidateAttemptStateAccepted:
		return 0, false
	case runtimecontract.RuntimeCandidateAttemptStateFailed:
		if second == nil || second.State == runtimecontract.RuntimeCandidateAttemptStatePending {
			return 1, true
		}
		return 0, false
	case runtimecontract.RuntimeCandidateAttemptStateInvalidated:
		return 0, true
	default:
		return 0, false
	}
}

func runtimeV2CandidateContractExhausted(aggregate runtimev2.RuntimeAggregate) bool {
	contractID := runtimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.Decision.AuthorityTransaction())
	count := 0
	for _, record := range aggregate.CandidateAttempts {
		if runtimecontract.CandidateAttemptAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, record) != contractID {
			continue
		}
		count++
		if record.State == runtimecontract.RuntimeCandidateAttemptStateAccepted {
			return false
		}
		if record.Attempt == 1 && record.State == runtimecontract.RuntimeCandidateAttemptStateFailed {
			return true
		}
	}
	return count >= 2
}

// runtimeV2EnsureCandidateFeasible blocks provider requests when the sealed required
// output has no concrete executable operation kind under current capabilities.
// Assessment/governance candidates are LLM-produced and skip this gate.
func runtimeV2EnsureCandidateFeasible(aggregate runtimev2.RuntimeAggregate) error {
	fact, err := runtimeV2FeasibilityFact(aggregate)
	if err != nil || fact == nil {
		return err
	}
	return fmt.Errorf("%s", fact.Reason)
}

// runtimeV2FeasibilityFact produces a persisted SM-2 input for a blocked
// candidate request. Callers must RecordFeasibilityFact and re-drive rather
// than returning this error to the process boundary.
func runtimeV2FeasibilityFact(aggregate runtimev2.RuntimeAggregate) (*runtimecontract.RuntimeFeasibilityFact, error) {
	switch aggregate.Decision.Action {
	case runtimecontract.RuntimeActionProduceStateChange, runtimecontract.RuntimeActionProduceValidation:
	default:
		return nil, nil
	}
	required := aggregate.Decision.RequiredOutput.Normalized()
	if aggregate.Decision.Action == runtimecontract.RuntimeActionProduceValidation {
		if missing := runtimeV2MissingValidationDiscoverySources(aggregate.Facts); len(missing) > 0 {
			return &runtimecontract.RuntimeFeasibilityFact{
				FocusID: aggregate.Facts.Focus.ID, Action: aggregate.Decision.Action,
				Reason: fmt.Sprintf("runtime validation discovery prerequisites are not fresh: %s", strings.Join(missing, ", ")),
			}, nil
		}
	}
	if len(required.AcceptedKinds) == 0 {
		return &runtimecontract.RuntimeFeasibilityFact{
			FocusID: aggregate.Facts.Focus.ID, Action: aggregate.Decision.Action,
			Reason: fmt.Sprintf("runtime action %s has no accepted operation kinds", aggregate.Decision.Action),
		}, nil
	}
	feasible := runtimecontract.FeasibleOperationKindsWithActivation(required, aggregate.Facts.Capabilities, aggregate.Facts.Activation.MCPCapabilities)
	if len(feasible) == 0 {
		return &runtimecontract.RuntimeFeasibilityFact{
			FocusID: aggregate.Facts.Focus.ID, Action: aggregate.Decision.Action,
			RequiredOperationKinds: append([]string(nil), required.AcceptedKinds...),
			MissingCapabilityKinds: append([]string(nil), required.AcceptedKinds...),
			Reason:                 fmt.Sprintf("runtime action %s has no feasible operation kinds under sealed capabilities", aggregate.Decision.Action),
		}, nil
	}
	return nil, nil
}

// runtimeV2MissingValidationDiscoverySources evaluates the profile-selected,
// owner-neutral discovery vocabulary against committed prerequisite facts. The
// bootstrap already requests each of these facts through typed owner commands;
// this gate makes a missing/stale observation a persisted SM-2 feasibility
// input instead of allowing an ungrounded provider request.
func runtimeV2MissingValidationDiscoverySources(facts runtimecontract.RuntimeFacts) []string {
	required := runtimecontract.RequiredRuntimeValidationDiscoverySources(
		facts.Activation.ExecutionContract.ValidationPolicies,
		runtimecontract.EvidenceKindValidation,
	)
	if len(required) == 0 {
		return nil
	}
	facts = facts.Normalized()
	missing := make([]string, 0, len(required))
	for _, source := range required {
		if runtimeV2ValidationDiscoverySourceSatisfied(facts, source) {
			continue
		}
		missing = append(missing, source)
	}
	return missing
}

func runtimeV2ValidationDiscoverySourceSatisfied(facts runtimecontract.RuntimeFacts, source string) bool {
	hasObservation := func(prerequisiteID string, kind runtimecontract.PrerequisiteKind) bool {
		observation, found := runtimecontract.RuntimePrerequisiteObservationForID(facts.Resources.PrerequisiteObservations, prerequisiteID)
		return found && observation.Kind == kind && observation.ContentHash != ""
	}
	switch strings.TrimSpace(source) {
	case runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest:
		if facts.Resources.Manifest == nil || strings.TrimSpace(facts.Resources.Manifest.Revision) == "" {
			return false
		}
		for _, prerequisite := range facts.Prerequisites.Satisfied {
			if prerequisite.Kind == runtimecontract.PrerequisiteKindManifest && hasObservation(prerequisite.ID, prerequisite.Kind) {
				return true
			}
		}
		return false
	case runtimecontract.RuntimeValidationDiscoverySourceProjectFiles:
		for _, prerequisite := range facts.Prerequisites.Satisfied {
			if prerequisite.Kind != runtimecontract.PrerequisiteKindManifest || !hasObservation(prerequisite.ID, prerequisite.Kind) {
				continue
			}
			observation, _ := runtimecontract.RuntimePrerequisiteObservationForID(facts.Resources.PrerequisiteObservations, prerequisite.ID)
			for _, dependency := range observation.Dependencies {
				if dependency.Domain == runtimecontract.RuntimeResourceWorkspaceTree {
					return true
				}
			}
		}
		return false
	case runtimecontract.RuntimeValidationDiscoverySourceDeclaredCommands:
		for _, inventory := range facts.Resources.OperationAdmissibilities {
			if len(inventory.OperationKinds) > 0 && hasObservation(inventory.PrerequisiteID, runtimecontract.PrerequisiteKindOperationAdmissibility) {
				return true
			}
		}
		return false
	case runtimecontract.RuntimeValidationDiscoverySourceAvailableCapabilities:
		if len(facts.Capabilities.StateChangeKinds)+len(facts.Capabilities.ValidationKinds)+len(facts.Capabilities.GovernanceKinds) == 0 {
			return false
		}
		for _, prerequisite := range facts.Prerequisites.Satisfied {
			if prerequisite.Kind == runtimecontract.PrerequisiteKindCapability && hasObservation(prerequisite.ID, prerequisite.Kind) {
				return true
			}
		}
		return false
	default:
		// An unknown profile source cannot be inferred by core. It remains
		// unsatisfied until a typed owner observation contract is introduced.
		return false
	}
}
