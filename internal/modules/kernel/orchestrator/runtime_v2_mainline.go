package orchestrator

import (
	"context"
	"fmt"
	"strings"

	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const runtimeV2DefaultTargetSnapshotBytes = 256 * 1024

func (m *Module) prepareRuntimeV2Prerequisites(ctx context.Context, aggregate runtimev2.RuntimeAggregate) (runtimev2.RuntimeAggregate, error) {
	if m == nil || m.bizPtr == nil || m.rawEventHub == nil {
		return runtimev2.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	workspace, err := (workspaceEventRepository{hub: m.rawEventHub}).Get(ctx, aggregate.Facts.WorkspaceID)
	if err != nil {
		return runtimev2.RuntimeAggregate{}, err
	}
	for aggregate.Decision.Action == runtimecontract.RuntimeActionAcquirePrerequisite {
		pending := append([]runtimecontract.RuntimePrerequisite(nil), aggregate.Facts.Prerequisites.Pending...)
		if len(pending) == 0 {
			return runtimev2.RuntimeAggregate{}, fmt.Errorf("runtime prerequisite action has no pending prerequisite")
		}
		for _, prerequisite := range pending {
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

func (m *Module) resolveRuntimeV2Prerequisite(
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
		facts, err := AcquireAndCommitWorkspaceManifestPrerequisite(ctx, m.rawEventHub, aggregate.Facts, events.AcquireWorkspaceManifestCommand{
			WorkspaceID: commandScope.workspaceID, RunID: commandScope.runID, AuthoritySnapshotID: commandScope.snapshotID,
			PrerequisiteID: prerequisite.ID, WorkspaceDeliverableDirectory: deliverableDirectory,
		})
		if err != nil {
			return runtimev2.PrerequisiteResult{}, err
		}
		return runtimev2.PrerequisiteResult{PrerequisiteID: prerequisite.ID, Manifest: facts.Resources.Manifest}, nil
	case runtimecontract.PrerequisiteKindTargetSnapshot:
		maxBytes := prerequisite.MinimumBytes
		if maxBytes <= 0 {
			maxBytes = runtimeV2DefaultTargetSnapshotBytes
		}
		result, err := AcquireTargetSnapshotPrerequisite(ctx, m.rawEventHub, events.AcquireTargetSnapshotCommand{
			WorkspaceID: commandScope.workspaceID, RunID: commandScope.runID, AuthoritySnapshotID: commandScope.snapshotID,
			PrerequisiteID: prerequisite.ID, WorkspaceDeliverableDirectory: deliverableDirectory,
			TargetRef: prerequisite.SubjectRef, MaxBytes: maxBytes,
		})
		if err != nil {
			return runtimev2.PrerequisiteResult{}, err
		}
		return runtimev2.PrerequisiteResult{PrerequisiteID: prerequisite.ID, TargetSnapshot: &result.Snapshot}, nil
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
		facts, err := AcquireAndCommitProviderCeilingPrerequisite(ctx, m.rawEventHub, aggregate.Facts, events.AcquireProviderCeilingCommand{
			WorkspaceID: commandScope.workspaceID, RunID: commandScope.runID, AuthoritySnapshotID: commandScope.snapshotID,
			PrerequisiteID: prerequisite.ID, Target: aggregate.Facts.Activation.ProviderTarget, MinimumBytes: prerequisite.MinimumBytes,
		})
		if err != nil {
			return runtimev2.PrerequisiteResult{}, err
		}
		return runtimev2.PrerequisiteResult{PrerequisiteID: prerequisite.ID, ProviderCeiling: facts.Resources.ProviderCeiling}, nil
	case runtimecontract.PrerequisiteKindCapability:
		if prerequisite.OwnerModuleID == "" {
			return runtimev2.PrerequisiteResult{}, fmt.Errorf("runtime capability prerequisite %q has no owner module", prerequisite.ID)
		}
		facts, err := AcquireAndCommitRuntimeCapabilitiesPrerequisite(ctx, m.rawEventHub, aggregate.Facts, events.AcquireRuntimeCapabilitiesCommand{
			WorkspaceID: commandScope.workspaceID, RunID: commandScope.runID, AuthoritySnapshotID: commandScope.snapshotID,
			PrerequisiteID: prerequisite.ID, ModuleID: prerequisite.OwnerModuleID,
			CapabilityClass: prerequisite.CapabilityClass, RequiredKinds: prerequisite.RequiredCapabilities,
		})
		if err != nil {
			return runtimev2.PrerequisiteResult{}, err
		}
		capabilities := facts.Capabilities
		return runtimev2.PrerequisiteResult{PrerequisiteID: prerequisite.ID, Capabilities: &capabilities}, nil
	default:
		return runtimev2.PrerequisiteResult{}, fmt.Errorf("unsupported runtime prerequisite kind %q", prerequisite.Kind)
	}
}

func (m *Module) requestAndSealRuntimeV2Candidate(
	ctx context.Context,
	aggregate runtimev2.RuntimeAggregate,
	stablePrefixHash string,
	candidateAttempt int,
) (runtimev2.RuntimeAggregate, error) {
	if m == nil || m.bizPtr == nil || m.rawEventHub == nil {
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
	previousFailure, err := runtimeV2PreviousCandidateFailure(aggregate, candidateAttempt)
	if err != nil {
		return runtimev2.RuntimeAggregate{}, err
	}
	response, err := RequestRuntimeCandidate(ctx, m.rawEventHub, events.RequestRuntimeCandidateCommand{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		Decision: aggregate.Decision, Transaction: transaction, CandidateAttempt: candidateAttempt,
		PreviousFailure:  previousFailure,
		StablePrefixHash: stablePrefixHash, DefinitionContext: aggregate.Definition,
		CandidateContext: runtimecontract.RuntimeCandidateContextForDecision(
			aggregate.Facts,
			aggregate.Decision,
			runtimeV2CompletedDispatchRecords(aggregate.CompletedDispatches),
		),
	})
	if err != nil {
		return m.bizPtr.FailRuntimeV2CandidateAttempt(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, candidateAttempt, runtimecontract.RuntimeCandidateFailure{
			Class: runtimecontract.RuntimeCandidateFailureClassInternal, Reason: err.Error(),
		})
	}
	if response.Transaction != transaction {
		return m.bizPtr.FailRuntimeV2CandidateAttempt(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, candidateAttempt, runtimecontract.RuntimeCandidateFailure{
			Class: runtimecontract.RuntimeCandidateFailureClassInternal, Reason: "runtime candidate response changed the sealed authority transaction",
		})
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
	accepted, err := m.bizPtr.AcceptRuntimeV2CandidateAttempt(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, candidateAttempt, response.Candidate)
	if err != nil {
		if isRuntimeCandidateReplayError(err) {
			return m.bizPtr.FailRuntimeV2CandidateAttempt(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, candidateAttempt, runtimecontract.RuntimeCandidateFailure{
				Class: runtimecontract.RuntimeCandidateFailureClassContract, Reason: err.Error(),
			})
		}
		return runtimev2.RuntimeAggregate{}, err
	}
	return accepted, nil
}

func runtimeV2CompletedDispatchRecords(values []runtimev2.SealedDispatch) []runtimecontract.RuntimeDispatchRecord {
	ret := make([]runtimecontract.RuntimeDispatchRecord, len(values))
	for idx := range values {
		ret[idx] = runtimeDispatchRecordFromAggregate(values[idx])
	}
	return ret
}

func isRuntimeCandidateReplayError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "replays active rejected operation shape") ||
		strings.Contains(message, "replays rejected operation shape") ||
		strings.Contains(message, "replay rejected shape") ||
		strings.Contains(message, "only validation evidence is missing")
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

func (m *Module) requestAndSealRuntimeV2CandidateWithRetry(
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
			return runtimev2.RuntimeAggregate{}, fmt.Errorf("%w: no candidate attempt is available for authority snapshot %s", common.ErrConflict, aggregate.Decision.AuthoritySnapshot.ID)
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

func (m *Module) sealRuntimeV2GovernanceCandidate(ctx context.Context, aggregate runtimev2.RuntimeAggregate, stablePrefixHash string) (runtimev2.RuntimeAggregate, error) {
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
	return m.bizPtr.AcceptRuntimeV2CandidateAttempt(ctx, pending.Facts.WorkspaceID, pending.Facts.RunID, attempt, runtimecontract.RuntimeCandidate{
		Governance: &runtimecontract.GovernanceCandidate{ExternalDependency: dependency},
	})
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
