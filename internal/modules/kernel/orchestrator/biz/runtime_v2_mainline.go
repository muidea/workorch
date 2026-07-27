package orchestrator

// Runtime V2 mainline is owned by Biz.

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const runtimeV2DefaultTargetSnapshotBytes = 256 * 1024

type runtimeV2PrerequisiteResolverClass string

const (
	runtimeV2PrerequisiteResolverExternal runtimeV2PrerequisiteResolverClass = "external_owner"
	runtimeV2PrerequisiteResolverLocal    runtimeV2PrerequisiteResolverClass = "orchestrator_local"
	runtimeV2PrerequisiteResolverUnknown  runtimeV2PrerequisiteResolverClass = "unreachable"
)

// runtimeV2PrerequisiteResolverClassification is the production inventory
// boundary for resolver cases. Keep it adjacent to the resolver so a newly
// added PrerequisiteKind cannot become an undocumented fourth path.
func runtimeV2PrerequisiteResolverClassification(kind runtimecontract.PrerequisiteKind) runtimeV2PrerequisiteResolverClass {
	switch kind {
	case runtimecontract.PrerequisiteKindManifest, runtimecontract.PrerequisiteKindTargetSnapshot,
		runtimecontract.PrerequisiteKindTargetSelection, runtimecontract.PrerequisiteKindProviderCeiling,
		runtimecontract.PrerequisiteKindCapability, runtimecontract.PrerequisiteKindOperationAdmissibility,
		runtimecontract.PrerequisiteKindCapabilityState:
		return runtimeV2PrerequisiteResolverExternal
	case runtimecontract.PrerequisiteKindFailureTarget, runtimecontract.PrerequisiteKindValidationScratch:
		return runtimeV2PrerequisiteResolverLocal
	default:
		return runtimeV2PrerequisiteResolverUnknown
	}
}

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
			result, resolveErr := m.resolveRuntimeV2Prerequisite(ctx, aggregate, workspace.DeliverableDirectory, workspace.InternalDirectory, prerequisite)
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
	workspaceInternalDirectory string,
	prerequisite runtimecontract.RuntimePrerequisite,
) (runtimev2.PrerequisiteResult, error) {
	commandScope := struct {
		workspaceID common.WorkspaceID
		runID       common.RunID
		snapshotID  string
	}{aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.Decision.AuthoritySnapshot.ID}
	if runtimeV2PrerequisiteResolverClassification(prerequisite.Kind) == runtimeV2PrerequisiteResolverUnknown {
		return runtimev2.PrerequisiteResult{}, fmt.Errorf("unsupported runtime prerequisite kind %q", prerequisite.Kind)
	}
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
	case runtimecontract.PrerequisiteKindTargetSelection:
		result, err := AcquireRuntimeTargetSelectionPrerequisite(ctx, m.EventHub(), events.AcquireRuntimeTargetSelectionCommand{
			WorkspaceID: commandScope.workspaceID, RunID: commandScope.runID, AuthoritySnapshotID: commandScope.snapshotID,
			PrerequisiteID: prerequisite.ID, WorkspaceDeliverableDirectory: deliverableDirectory, TargetRole: prerequisite.SubjectRef, TargetSelectorRef: prerequisite.TargetSelectorRef, TargetSelectorOwnerRef: prerequisite.TargetSelectorOwnerRef, TargetSelectorSubjectRef: prerequisite.TargetSelectorSubjectRef, TargetSelectorCandidateRefs: prerequisite.TargetSelectorCandidateRefs, ProspectiveTargetRef: prerequisite.ProspectiveTargetRef,
			// Target selection seals a complete File-owner feasibility fact for
			// SM-2.  It must not inherit the much smaller per-request candidate
			// context budget, otherwise a normal workspace with more files than
			// coverage items is incorrectly reported as ambiguous.
			MaxItems: runtimecontract.DefaultRuntimeOwnerInventoryBudget().MaxItems,
		})
		if err != nil {
			return runtimev2.PrerequisiteResult{}, err
		}
		selection := result.Selection
		return runtimev2.PrerequisiteResult{PrerequisiteID: prerequisite.ID, TargetSelection: &selection, Observation: result.Observation}, nil
	case runtimecontract.PrerequisiteKindFailureTarget:
		facts, err := ResolveFailureTargetPrerequisite(aggregate.Facts, prerequisite.ID)
		if err != nil {
			return runtimev2.PrerequisiteResult{}, err
		}
		return runtimev2.PrerequisiteResult{PrerequisiteID: prerequisite.ID, FailureTarget: facts.Resources.FailureTarget}, nil
	case runtimecontract.PrerequisiteKindValidationScratch:
		facts, err := AcquireValidationScratchPrerequisite(aggregate.Facts, workspaceInternalDirectory, prerequisite.ID)
		if err != nil {
			return runtimev2.PrerequisiteResult{}, err
		}
		observation, ok := runtimecontract.RuntimePrerequisiteObservationForID(facts.Resources.PrerequisiteObservations, prerequisite.ID)
		if !ok {
			return runtimev2.PrerequisiteResult{}, fmt.Errorf("validation scratch prerequisite %q did not return an observation", prerequisite.ID)
		}
		return runtimev2.PrerequisiteResult{PrerequisiteID: prerequisite.ID, ValidationScratch: facts.Resources.ValidationScratch, Observation: &observation}, nil
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
			RequiredTargetRefs: runtimev2.GroundedRepairTargets(aggregate.Facts), CapabilityStateBinding: binding,
			// Operation admissibility is another owner-grounded SM-2 inventory.
			// In particular, it must include a file created by a prior accepted
			// operation so the next sealed validation can read it. Candidate
			// context compression must not remove that owner authority.
			MaxItems: runtimecontract.DefaultRuntimeOwnerInventoryBudget().MaxItems,
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
	candidateContext := runtimecontract.RuntimeCandidateContextForDecision(
		aggregate.Facts,
		aggregate.Decision,
		runtimev2.ActiveRejectedOperationShapes(aggregate),
	)
	manifest, manifestErr := runtimecontract.BuildRuntimeCandidateContractManifest(aggregate.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(candidateContext.Resources))
	if manifestErr != nil {
		return m.bizPtr.RecordRuntimeV2FailureFact(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_candidate_manifest_invalid",
			Signature: "runtime-candidate-manifest-invalid", Message: manifestErr.Error(),
		})
	}
	// This is the persisted-authority half of the P0/P2/P4 preflight. It runs
	// before BeginRuntimeV2CandidateAttempt so an internally incomplete sealed
	// context cannot spend the model correction budget. The LLM Block performs
	// the second, rendered-section audit gate immediately before provider I/O.
	if err := aggregate.Definition.Validate(); err != nil {
		return m.bizPtr.RecordRuntimeV2FailureFact(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_context_integrity",
			Signature: "runtime-context-integrity", Message: err.Error(),
		})
	}
	if err := candidateContext.Validate(aggregate.Decision); err != nil {
		return m.bizPtr.RecordRuntimeV2FailureFact(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_context_integrity",
			Signature: "runtime-context-integrity", Message: err.Error(),
		})
	}
	// The LLM owner is the sole renderer, so the rendered C0-C5/P0-P4 integrity
	// gate must run through its typed EventHub contract before SM-3 creates an
	// attempt. A persisted proposal/output continuation has already passed this
	// boundary; re-rendering it on restart would add a non-authoritative failure
	// point to an otherwise idempotent seal path.
	// Runtime V2 preflight acquisition normally guarantees a sealed provider
	// ceiling before this action. Keep legacy owner-local state-machine fixtures
	// that intentionally omit provider binding on the non-provider path outside
	// the rendered LLM gate; production candidate requests with a ceiling always
	// take this branch.
	if candidateContext.Resources.ProviderCeiling != nil {
		if _, proposalPersisted := runtimeV2PendingCandidateProposal(aggregate, candidateAttempt); !proposalPersisted {
			previousFailure, previousErr := runtimeV2PreviousCandidateFailure(aggregate, candidateAttempt)
			if previousErr != nil {
				return runtimev2.RuntimeAggregate{}, previousErr
			}
			preflight, preflightErr := PreflightRuntimeCandidate(ctx, m.EventHub(), events.PreflightRuntimeCandidateCommand{
				WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
				Decision: aggregate.Decision, Transaction: aggregate.Decision.AuthorityTransaction(), CandidateAttempt: candidateAttempt,
				PreviousFailure:  previousFailure,
				StablePrefixHash: stablePrefixHash, CandidateManifest: manifest, DefinitionContext: aggregate.Definition, CandidateContext: candidateContext,
			})
			if preflightErr != nil {
				return m.bizPtr.RecordRuntimeV2FailureFact(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, runtimecontract.FailureFact{
					Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_context_integrity",
					Signature: "runtime-context-integrity", Message: preflightErr.Error(),
				})
			}
			if !preflight.Ready {
				return m.bizPtr.RecordRuntimeV2FailureFact(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, runtimecontract.FailureFact{
					Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_context_integrity",
					Signature: "runtime-context-integrity", Message: preflight.Reason,
				})
			}
		}
	}
	candidate, proposalPersisted := runtimeV2PendingCandidateProposal(aggregate, candidateAttempt)
	if !proposalPersisted {
		var candidateReady bool
		var requestErr error
		aggregate, candidate, candidateReady, requestErr = m.requestRuntimeV2CandidateDocument(ctx, aggregate, candidateContext, manifest, stablePrefixHash, candidateAttempt)
		if requestErr != nil {
			return runtimev2.RuntimeAggregate{}, requestErr
		}
		if !candidateReady {
			return aggregate, nil
		}
	}
	if len(aggregate.Facts.Resources.OperationAdmissibilities) > 0 {
		workspace, workspaceErr := (workspaceEventRepository{hub: m.EventHub()}).Get(ctx, aggregate.Facts.WorkspaceID)
		if workspaceErr != nil {
			return m.bizPtr.FailRuntimeV2CandidateAttempt(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, candidateAttempt, runtimecontract.RuntimeCandidateFailure{
				Class: runtimecontract.RuntimeCandidateFailureClassInternal, Code: "workspace_resolution_failed", Reason: workspaceErr.Error(),
			})
		}
		if feasibilityFailure, stale, prerequisiteRequest, probeErr := m.probeRuntimeV2CandidateFeasibility(ctx, aggregate, workspace.DeliverableDirectory, candidate, manifest, candidateAttempt); probeErr != nil {
			return m.bizPtr.FailRuntimeV2CandidateAttempt(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, candidateAttempt, runtimecontract.RuntimeCandidateFailure{
				Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "operation_feasibility_probe_invalid", Reason: probeErr.Error(),
			})
		} else if stale != nil {
			return m.bizPtr.InvalidateRuntimeV2CandidateAttemptForPrerequisiteStale(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, candidateAttempt, *stale)
		} else if prerequisiteRequest != nil {
			return m.bizPtr.InvalidateRuntimeV2CandidateAttemptForFeasibilityPrerequisite(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, candidateAttempt, *prerequisiteRequest)
		} else if feasibilityFailure != nil {
			return m.bizPtr.FailRuntimeV2CandidateAttempt(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, candidateAttempt, *feasibilityFailure)
		}
	}
	record, bound := runtimeV2CandidateAttemptRecord(aggregate, candidateAttempt)
	if !bound {
		return runtimev2.RuntimeAggregate{}, fmt.Errorf("%w: pending candidate attempt has no persisted request-session binding", common.ErrConflict)
	}
	sealed, sealErr := m.bizPtr.AdmitAndSealRuntimeV2CandidateAttempt(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, candidateAttempt, manifest, record.LogicalRequestID, candidate)
	if sealErr != nil {
		return runtimev2.RuntimeAggregate{}, sealErr
	}
	return sealed.Aggregate, nil
}

// requestRuntimeV2CandidateDocument owns the provider-I/O part of one sealed
// candidate request. C4 deliberately persists the session and physical call
// before EventHub provider I/O; no candidate attempt exists until a complete
// document is stored. It returns candidateReady only after the subsequent C5
// admission transition has persisted a proposal.
func (m *Biz) requestRuntimeV2CandidateDocument(
	ctx context.Context,
	aggregate runtimev2.RuntimeAggregate,
	candidateContext runtimecontract.RuntimeCandidateContext,
	manifest runtimecontract.RuntimeCandidateContractManifest,
	stablePrefixHash string,
	candidateAttempt int,
) (runtimev2.RuntimeAggregate, runtimecontract.RuntimeCandidate, bool, error) {
	previousFailure, err := runtimeV2PreviousCandidateFailure(aggregate, candidateAttempt)
	if err != nil {
		return runtimev2.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, err
	}
	if err := runtimecontract.ValidateRuntimeCandidateContractManifestForDecision(manifest, aggregate.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(candidateContext.Resources)); err != nil {
		return runtimev2.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, err
	}
	correctionHash := runtimecontract.StableRuntimeCandidateRequestCorrectionHash(previousFailure)
	ensured, session, err := m.bizPtr.EnsureRuntimeV2CandidateRequestSession(
		ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, stablePrefixHash, manifest.Hash, correctionHash,
	)
	if err != nil {
		return runtimev2.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, err
	}
	aggregate = ensured
	if session.State == runtimecontract.RuntimeCandidateRequestSessionDocumentComplete {
		return m.admitRuntimeV2CompletedCandidateDocument(ctx, aggregate, manifest, session.LogicalRequestID, candidateAttempt, nil)
	}
	if session.State == runtimecontract.RuntimeCandidateRequestSessionTerminal {
		return aggregate, runtimecontract.RuntimeCandidate{}, false, nil
	}
	if session.ProviderSubmitted {
		terminated, terminateErr := m.bizPtr.TerminateRuntimeV2CandidateRequestSession(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, runtimecontract.RuntimeCandidateRequestOutcome{
			Class: runtimecontract.RuntimeCandidateRequestOutcomeProviderSubmissionUnknown,
			Code:  "provider_submission_unknown", Signature: "provider-submission-unknown",
		})
		if terminateErr != nil {
			return runtimev2.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, terminateErr
		}
		failed, failureErr := m.bizPtr.RecordRuntimeV2FailureFact(ctx, terminated.Facts.WorkspaceID, terminated.Facts.RunID, runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassProviderSubmissionUnknown, Code: "provider_submission_unknown",
			Signature: "provider-submission-unknown", Message: "provider request was submitted without a durable outcome",
		})
		return failed, runtimecontract.RuntimeCandidate{}, false, failureErr
	}

	sequence := session.NextSequence
	physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, sequence)
	var continuation *runtimecontract.RuntimeCandidateOutputContinuationRequest
	if len(session.Fragments) > 0 {
		policy := aggregate.Facts.Activation.ExecutionContract.Normalized().CandidateOutputContinuation
		if policy.TailBytes <= 0 {
			return runtimev2.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, fmt.Errorf("runtime candidate continuation requires a positive sealed tail size")
		}
		request, nextPhysicalCallID, continuationErr := prepareRuntimeCandidateRequestContinuation(ctx, m.bizPtr.EvidenceRepositories().ArtifactContinuations, aggregate, session.LogicalRequestID, policy.TailBytes)
		if continuationErr != nil {
			return runtimev2.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, continuationErr
		}
		continuation = &request
		physicalCallID = nextPhysicalCallID
	}
	if err := runtimecontract.ValidateRuntimeCandidateRequestIdentity(session.LogicalRequestID, sequence, physicalCallID); err != nil {
		return runtimev2.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, err
	}
	submitted, err := m.bizPtr.MarkRuntimeV2CandidateRequestSubmitted(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, sequence, physicalCallID)
	if err != nil {
		return runtimev2.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, err
	}
	aggregate = submitted
	response, requestErr := RequestRuntimeCandidate(ctx, m.EventHub(), events.RequestRuntimeCandidateCommand{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		LogicalRequestID: session.LogicalRequestID, RequestSequence: sequence, PhysicalCallID: physicalCallID,
		Decision: aggregate.Decision, Transaction: aggregate.Decision.AuthorityTransaction(), CandidateAttempt: candidateAttempt,
		PreviousFailure: previousFailure, StablePrefixHash: stablePrefixHash, CandidateManifest: manifest, DefinitionContext: aggregate.Definition,
		CandidateContext: candidateContext, OutputContinuation: continuation,
	})
	if requestErr != nil {
		return m.terminateRuntimeV2CandidateRequestProtocol(ctx, aggregate, session.LogicalRequestID, "provider_request_contract_error", requestErr.Error())
	}
	if response.Transaction != aggregate.Decision.AuthorityTransaction() {
		return m.terminateRuntimeV2CandidateRequestProtocol(ctx, aggregate, session.LogicalRequestID, "provider_response_transaction_mismatch", "runtime candidate response changed the sealed authority transaction")
	}
	if response.Stale != nil {
		updated, staleErr := m.bizPtr.InvalidateRuntimeV2CandidateRequestSessionForPrerequisiteStale(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, *response.Stale)
		return updated, runtimecontract.RuntimeCandidate{}, false, staleErr
	}
	if response.OutputFragment != nil {
		updated, _, fragmentErr := m.persistAndRecordRuntimeCandidateRequestFragment(ctx, aggregate, session.LogicalRequestID, sequence, physicalCallID, response.OutputFragment.Content, response.OutputFragment.ReachedProviderLimit)
		if fragmentErr != nil {
			return runtimev2.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, fragmentErr
		}
		assembled, assembleErr := assembleRuntimeCandidateRequestFragments(ctx, m.bizPtr.EvidenceRepositories().ArtifactContinuations, updated, session.LogicalRequestID)
		if assembleErr != nil {
			return runtimev2.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, assembleErr
		}
		_, decodeErr := runtimecontract.DecodeRuntimeCandidatePayloadWithManifest(manifest, assembled)
		if decodeErr != nil && runtimecontract.RuntimeCandidateDocumentDispositionForError(decodeErr) == runtimecontract.RuntimeCandidateDocumentIncomplete {
			updatedSession, found := runtimeV2CandidateRequestSession(updated, session.LogicalRequestID)
			if !found {
				return runtimev2.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, fmt.Errorf("runtime candidate request session %q disappeared after fragment commit", session.LogicalRequestID)
			}
			// MaxChunks counts suffix calls after the initial incomplete document.
			// The initial fragment is persisted before this check, so a zero budget
			// still leaves a recoverable audit trail while never allocating a
			// candidate attempt. A final suffix may still complete and be admitted
			// above; only another incomplete assembled document exhausts the budget.
			continuationChunks := len(updatedSession.Fragments) - 1
			maxChunks := updated.Facts.Activation.ExecutionContract.Normalized().CandidateOutputContinuation.MaxChunks
			if continuationChunks >= maxChunks {
				return m.terminateRuntimeV2CandidateRequestContinuationExhausted(ctx, updated, session.LogicalRequestID)
			}
			return updated, runtimecontract.RuntimeCandidate{}, false, nil
		}
		completed, _, _, completeErr := m.persistAndCompleteRuntimeCandidateRequestDocument(ctx, updated, session.LogicalRequestID, sequence, physicalCallID, assembled)
		if completeErr != nil {
			return runtimev2.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, completeErr
		}
		if decodeErr != nil {
			return m.admitRuntimeV2CompletedCandidateDocument(ctx, completed, manifest, session.LogicalRequestID, candidateAttempt, runtimeV2CandidateSchemaFailure(decodeErr))
		}
		return m.admitRuntimeV2CompletedCandidateDocument(ctx, completed, manifest, session.LogicalRequestID, candidateAttempt, nil)
	}
	if response.Failure != nil {
		if len(response.DocumentContent) > 0 {
			completed, _, _, completeErr := m.persistAndCompleteRuntimeCandidateRequestDocument(ctx, aggregate, session.LogicalRequestID, sequence, physicalCallID, response.DocumentContent)
			if completeErr != nil {
				return runtimev2.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, completeErr
			}
			return m.admitRuntimeV2CompletedCandidateDocument(ctx, completed, manifest, session.LogicalRequestID, candidateAttempt, response.Failure)
		}
		if response.Failure.Class == runtimecontract.RuntimeCandidateFailureClassProviderTransient {
			updated, recordErr := m.bizPtr.RecordRuntimeV2CandidateRequestTransportOutcome(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, physicalCallID, runtimecontract.RuntimeCandidateRequestOutcome{
				Class: runtimecontract.RuntimeCandidateRequestOutcomeProviderTransient, Code: firstNonEmptyString(response.Failure.Code, "provider_transient"), Signature: firstNonEmptyString(response.Failure.Signature, "provider-transient"),
			})
			if recordErr != nil {
				return runtimev2.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, recordErr
			}
			recovered, consumed, budgetErr := m.bizPtr.ConsumeRuntimeV2RecoveryBudget(ctx, updated.Facts.WorkspaceID, updated.Facts.RunID, runtimecontract.RuntimeRecoveryBudgetProviderTransport)
			if budgetErr != nil {
				return runtimev2.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, budgetErr
			}
			if consumed {
				return recovered, runtimecontract.RuntimeCandidate{}, false, nil
			}
			failed, failureErr := m.bizPtr.RecordRuntimeV2FailureFact(ctx, updated.Facts.WorkspaceID, updated.Facts.RunID, runtimecontract.FailureFact{
				Class: runtimecontract.FailureClassProviderTransient, Code: "provider_transport_exhausted", Signature: "provider-transport-exhausted", Message: strings.TrimSpace(response.Failure.Reason),
			})
			return failed, runtimecontract.RuntimeCandidate{}, false, failureErr
		}
		if fact, ok := runtimeV2ProviderGovernanceFailure(*response.Failure); ok {
			outcome := runtimecontract.RuntimeCandidateRequestOutcome{Code: firstNonEmptyString(response.Failure.Code, string(response.Failure.Class)), Signature: firstNonEmptyString(response.Failure.Signature, string(response.Failure.Class))}
			if response.Failure.Class == runtimecontract.RuntimeCandidateFailureClassProviderAuthentication {
				outcome.Class = runtimecontract.RuntimeCandidateRequestOutcomeProviderAuthentication
			} else {
				outcome.Class = runtimecontract.RuntimeCandidateRequestOutcomeProviderConfiguration
			}
			terminated, terminateErr := m.bizPtr.TerminateRuntimeV2CandidateRequestSession(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, outcome)
			if terminateErr != nil {
				return runtimev2.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, terminateErr
			}
			failed, failureErr := m.bizPtr.RecordRuntimeV2FailureFact(ctx, terminated.Facts.WorkspaceID, terminated.Facts.RunID, fact)
			return failed, runtimecontract.RuntimeCandidate{}, false, failureErr
		}
		return m.terminateRuntimeV2CandidateRequestProtocol(ctx, aggregate, session.LogicalRequestID, "provider_protocol_failure", response.Failure.Reason)
	}
	if len(response.DocumentContent) == 0 {
		return m.terminateRuntimeV2CandidateRequestProtocol(ctx, aggregate, session.LogicalRequestID, "provider_document_missing", "complete candidate response did not carry its raw document")
	}
	completed, _, _, completeErr := m.persistAndCompleteRuntimeCandidateRequestDocument(ctx, aggregate, session.LogicalRequestID, sequence, physicalCallID, response.DocumentContent)
	if completeErr != nil {
		return runtimev2.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, completeErr
	}
	return m.admitRuntimeV2CompletedCandidateDocument(ctx, completed, manifest, session.LogicalRequestID, candidateAttempt, nil)
}

// admitRuntimeV2CompletedCandidateDocument is the C5 boundary used by C4's
// production switch. The raw document must already be durable and the session
// must already be document_complete before attempt 0/1 is allocated.
func (m *Biz) admitRuntimeV2CompletedCandidateDocument(ctx context.Context, aggregate runtimev2.RuntimeAggregate, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string, candidateAttempt int, completeFailure *runtimecontract.RuntimeCandidateFailure) (runtimev2.RuntimeAggregate, runtimecontract.RuntimeCandidate, bool, error) {
	session, found := runtimeV2CandidateRequestSession(aggregate, logicalRequestID)
	if !found {
		return runtimev2.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, fmt.Errorf("runtime candidate request session %q is missing", logicalRequestID)
	}
	if session.ManifestHash != manifest.Hash {
		return runtimev2.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, fmt.Errorf("%w: completed candidate document manifest does not match its request session", common.ErrConflict)
	}
	document, err := loadCompletedRuntimeCandidateRequestDocument(ctx, m.bizPtr.EvidenceRepositories().ArtifactContinuations, aggregate, logicalRequestID)
	if err != nil {
		failed, failureErr := m.bizPtr.RecordRuntimeV2FailureFact(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "provider_document_content_invalid",
			Signature: "provider-document-content-invalid", Message: "persisted complete provider document is unavailable or fails its integrity check",
		})
		return failed, runtimecontract.RuntimeCandidate{}, false, failureErr
	}
	pending, err := m.bizPtr.BeginRuntimeV2CandidateAttempt(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, candidateAttempt, session.StablePrefixHash, manifest, session.LogicalRequestID)
	if err != nil {
		return runtimev2.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, err
	}
	if completeFailure != nil {
		failed, failureErr := m.bizPtr.FailRuntimeV2CandidateAttempt(ctx, pending.Facts.WorkspaceID, pending.Facts.RunID, candidateAttempt, *completeFailure)
		return failed, runtimecontract.RuntimeCandidate{}, false, failureErr
	}
	candidateContext := runtimecontract.RuntimeCandidateContextForDecision(pending.Facts, pending.Decision, runtimev2.ActiveRejectedOperationShapes(pending))
	if err := runtimecontract.ValidateRuntimeCandidateContractManifestForDecision(manifest, pending.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(candidateContext.Resources)); err != nil {
		return runtimev2.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, err
	}
	candidate, decodeErr := runtimecontract.DecodeRuntimeCandidatePayloadWithManifest(manifest, document)
	if decodeErr != nil {
		failed, failureErr := m.bizPtr.FailRuntimeV2CandidateAttempt(ctx, pending.Facts.WorkspaceID, pending.Facts.RunID, candidateAttempt, *runtimeV2CandidateSchemaFailure(decodeErr))
		return failed, runtimecontract.RuntimeCandidate{}, false, failureErr
	}
	if validateErr := candidate.ValidateWithActivation(pending.Decision, candidateContext.Activation); validateErr != nil {
		failed, failureErr := m.bizPtr.FailRuntimeV2CandidateAttempt(ctx, pending.Facts.WorkspaceID, pending.Facts.RunID, candidateAttempt, runtimecontract.RuntimeCandidateFailure{
			Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "candidate_contract_invalid", Signature: "candidate-contract-invalid", Reason: validateErr.Error(),
		})
		return failed, runtimecontract.RuntimeCandidate{}, false, failureErr
	}
	proposal, proposalErr := m.bizPtr.AdmitRuntimeV2CandidateProposal(ctx, pending.Facts.WorkspaceID, pending.Facts.RunID, candidateAttempt, manifest, session.LogicalRequestID, candidate)
	if proposalErr != nil {
		return runtimev2.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, proposalErr
	}
	if !proposal.Admission.Accepted() {
		return proposal.Aggregate, runtimecontract.RuntimeCandidate{}, false, nil
	}
	persisted, ok := runtimeV2PendingCandidateProposal(proposal.Aggregate, candidateAttempt)
	if !ok {
		return runtimev2.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, fmt.Errorf("persisted runtime candidate proposal is missing")
	}
	return proposal.Aggregate, persisted, true, nil
}

func runtimeV2CandidateRequestSession(aggregate runtimev2.RuntimeAggregate, logicalRequestID string) (runtimecontract.RuntimeCandidateRequestSession, bool) {
	for _, session := range aggregate.CandidateRequestSessions {
		if session.LogicalRequestID == logicalRequestID {
			return session.Clone(), true
		}
	}
	return runtimecontract.RuntimeCandidateRequestSession{}, false
}

func (m *Biz) terminateRuntimeV2CandidateRequestProtocol(ctx context.Context, aggregate runtimev2.RuntimeAggregate, logicalRequestID, code, message string) (runtimev2.RuntimeAggregate, runtimecontract.RuntimeCandidate, bool, error) {
	terminated, err := m.bizPtr.TerminateRuntimeV2CandidateRequestSession(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, logicalRequestID, runtimecontract.RuntimeCandidateRequestOutcome{
		Class: runtimecontract.RuntimeCandidateRequestOutcomeProviderProtocol, Code: code, Signature: strings.ReplaceAll(code, "_", "-"),
	})
	if err != nil {
		return runtimev2.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, err
	}
	failed, failureErr := m.bizPtr.RecordRuntimeV2FailureFact(ctx, terminated.Facts.WorkspaceID, terminated.Facts.RunID, runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassProviderProtocol, Code: code, Signature: strings.ReplaceAll(code, "_", "-"), Message: strings.TrimSpace(message),
	})
	return failed, runtimecontract.RuntimeCandidate{}, false, failureErr
}

// terminateRuntimeV2CandidateRequestContinuationExhausted closes a request
// session that has consumed its activation-sealed suffix budget without a
// complete document. This is request I/O state, not a candidate rejection:
// no candidate attempt, repair, dispatch, or governance ticket may be added.
func (m *Biz) terminateRuntimeV2CandidateRequestContinuationExhausted(ctx context.Context, aggregate runtimev2.RuntimeAggregate, logicalRequestID string) (runtimev2.RuntimeAggregate, runtimecontract.RuntimeCandidate, bool, error) {
	const (
		code      = "candidate_output_continuation_exhausted"
		signature = "candidate-output-continuation-exhausted"
		message   = "provider candidate output remained incomplete after its activation-sealed continuation budget was exhausted"
	)
	terminated, err := m.bizPtr.TerminateRuntimeV2CandidateRequestSession(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, logicalRequestID, runtimecontract.RuntimeCandidateRequestOutcome{
		Class: runtimecontract.RuntimeCandidateRequestOutcomeContinuationExhausted, Code: code, Signature: signature,
	})
	if err != nil {
		return runtimev2.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, err
	}
	failed, failureErr := m.bizPtr.RecordRuntimeV2FailureFact(ctx, terminated.Facts.WorkspaceID, terminated.Facts.RunID, runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassCandidateOutputExhausted, Code: code, Signature: signature, Message: message,
	})
	if failureErr != nil {
		return runtimev2.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, failureErr
	}
	if err := m.markRuntimeCandidateRequestContentFailed(ctx, failed, logicalRequestID); err != nil {
		return failed, runtimecontract.RuntimeCandidate{}, false, err
	}
	return failed, runtimecontract.RuntimeCandidate{}, false, nil
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

func runtimeV2CandidateAttemptRecord(aggregate runtimev2.RuntimeAggregate, attempt int) (runtimecontract.RuntimeCandidateAttemptRecord, bool) {
	transaction := aggregate.Decision.AuthorityTransaction()
	for _, record := range aggregate.CandidateAttempts {
		if record.Attempt == attempt && record.Transaction == transaction {
			return record, true
		}
	}
	return runtimecontract.RuntimeCandidateAttemptRecord{}, false
}

func (m *Biz) probeRuntimeV2CandidateFeasibility(ctx context.Context, aggregate runtimev2.RuntimeAggregate, workspaceDirectory string, candidate runtimecontract.RuntimeCandidate, manifest runtimecontract.RuntimeCandidateContractManifest, candidateAttempt int) (*runtimecontract.RuntimeCandidateFailure, *runtimecontract.RuntimePrerequisiteStale, *runtimecontract.RuntimeOperationPrerequisiteRequest, error) {
	if m == nil || m.bizPtr == nil || m.EventHub() == nil {
		return nil, nil, nil, common.ErrInvalidArgument
	}
	// Assessment candidates and governance candidates contain no executable
	// owner operation. Their authority is validated by their own seal contract,
	// so an unrelated persisted inventory must not make assessment impossible.
	if candidate.StateChange == nil && candidate.Validation == nil {
		return nil, nil, nil, nil
	}
	if len(aggregate.Facts.Resources.OperationAdmissibilities) == 0 {
		if runtimeV2RequiresOperationAdmissibility(aggregate.Facts.Prerequisites) {
			return nil, nil, nil, fmt.Errorf("runtime candidate has no persisted owner admissibility inventory")
		}
		// Focused reducer/receipt fixtures can construct facts directly without a
		// bootstrap prerequisite. Production activation always seals this
		// prerequisite, so such fixtures cannot bypass the production gate.
		return nil, nil, nil, nil
	}
	operations, err := orderedRuntimeV2CandidateOperations(candidate)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, operation := range operations {
		descriptor, ok := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
		if !ok || descriptor.OwnerModuleID == "" {
			return nil, nil, nil, fmt.Errorf("candidate operation %q has no canonical owner", operation.Kind)
		}
		inventory, found := runtimeV2OperationAdmissibilityForKind(aggregate.Facts.Resources.OperationAdmissibilities, descriptor.OwnerModuleID, operation.Kind)
		if !found {
			return nil, nil, nil, fmt.Errorf("candidate operation %q has no persisted owner admissibility inventory", operation.Kind)
		}
		if persisted, found := runtimeV2CandidateFeasibilityRecord(aggregate, candidateAttempt, operation.ID); found {
			if persisted.AdmissibilitySnapshotID != inventory.SnapshotID || persisted.AcquisitionBinding != inventory.AcquisitionBinding {
				return nil, nil, nil, fmt.Errorf("candidate operation %q feasibility snapshot does not match persisted owner inventory", operation.ID)
			}
			if persisted.Feasible {
				continue
			}
			if persisted.Stale != nil {
				return nil, persisted.Stale, nil, nil
			}
			if persisted.PrerequisiteRequest != nil {
				return nil, nil, persisted.PrerequisiteRequest, nil
			}
			return runtimeCandidateFailureFromFeasibility(persisted, operation, manifest, inventory, candidate), nil, nil, nil
		}
		observation, observed := runtimecontract.RuntimePrerequisiteObservationForID(aggregate.Facts.Resources.PrerequisiteObservations, inventory.PrerequisiteID)
		if !observed {
			return nil, nil, nil, fmt.Errorf("candidate operation %q has no freshness observation for owner admissibility", operation.Kind)
		}
		authorization, authorized := runtimeV2OperationAuthorizationForKind(manifest, operation.Kind)
		if !authorized {
			return nil, nil, nil, fmt.Errorf("candidate operation %q has no sealed operation authorization", operation.Kind)
		}
		response, probeErr := ProbeRuntimeOperationFeasibility(ctx, m.EventHub(), events.ProbeRuntimeOperationFeasibilityCommand{
			WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, Transaction: aggregate.Decision.AuthorityTransaction(),
			WorkspaceDeliverableDirectory: workspaceDirectory, Operation: operation, Admissibility: inventory,
			CapabilityStateBinding: inventory.AcquisitionBinding, CandidateManifestHash: manifest.Hash,
			OperationAuthorization: authorization, Observation: &observation,
			CurrentTargetSnapshot: runtimeV2CurrentTargetSnapshot(aggregate.Facts, operation),
		})
		if probeErr != nil {
			return nil, nil, nil, probeErr
		}
		recorded, recordErr := m.bizPtr.RecordRuntimeV2CandidateFeasibility(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, candidateAttempt, runtimecontract.RuntimeCandidateFeasibilityRecord{
			OperationID: operation.ID, AdmissibilitySnapshotID: inventory.SnapshotID, AcquisitionBinding: inventory.AcquisitionBinding,
			Feasible: response.Feasible, ReplayProof: response.ReplayProof, Failure: response.Failure, Stale: response.Stale, PrerequisiteRequest: response.PrerequisiteRequest,
		})
		if recordErr != nil {
			return nil, nil, nil, recordErr
		}
		aggregate = recorded
		if response.Feasible {
			continue
		}
		if response.Stale != nil {
			return nil, response.Stale, nil, nil
		}
		if response.PrerequisiteRequest != nil {
			return nil, nil, response.PrerequisiteRequest, nil
		}
		return runtimeCandidateFailureFromFeasibility(runtimecontract.RuntimeCandidateFeasibilityRecord{
			OperationID: operation.ID, AdmissibilitySnapshotID: inventory.SnapshotID, AcquisitionBinding: inventory.AcquisitionBinding,
			Feasible: false, Failure: response.Failure, PrerequisiteRequest: response.PrerequisiteRequest,
		}, operation, manifest, inventory, candidate), nil, nil, nil
	}
	return nil, nil, nil, nil
}

func runtimeV2CurrentTargetSnapshot(facts runtimecontract.RuntimeFacts, operation runtimecontract.RuntimeOperation) *runtimecontract.TargetSnapshot {
	if !runtimecontract.RuntimeOperationRequiresCurrentTargetSnapshot(operation) {
		return nil
	}
	for _, targetRef := range operation.Normalized().TargetRefs {
		for _, snapshot := range facts.Resources.TargetSnapshots {
			if strings.TrimSpace(snapshot.TargetRef) == targetRef && snapshot.Complete() {
				copySnapshot := snapshot
				return &copySnapshot
			}
		}
	}
	return nil
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

func runtimeCandidateFailureFromFeasibility(record runtimecontract.RuntimeCandidateFeasibilityRecord, operation runtimecontract.RuntimeOperation, manifest runtimecontract.RuntimeCandidateContractManifest, admissibility runtimecontract.RuntimeOperationAdmissibility, candidate runtimecontract.RuntimeCandidate) *runtimecontract.RuntimeCandidateFailure {
	if record.Feasible || record.Failure == nil {
		return nil
	}
	failure := record.Failure
	converted := &runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassInfeasible,
		Code:  failure.Code, Signature: failure.Signature, Reason: failure.Message,
		TargetRefs: append([]string(nil), failure.TargetRefs...),
		ValidationDiagnostic: func() *runtimecontract.ValidationFailureDiagnostic {
			if failure.ValidationDiagnostic == nil {
				return nil
			}
			diagnostic := failure.ValidationDiagnostic.Clone()
			return &diagnostic
		}(),
	}
	converted.Diagnostic = runtimecontract.RuntimeCandidateFailureDiagnosticForFeasibility(failure, operation, manifest, admissibility, candidate)
	return converted
}

func runtimeV2RequiresOperationAdmissibility(state runtimecontract.PrerequisiteState) bool {
	for _, prerequisite := range append(append([]runtimecontract.RuntimePrerequisite(nil), state.Pending...), state.Satisfied...) {
		if prerequisite.Kind == runtimecontract.PrerequisiteKindOperationAdmissibility {
			return true
		}
	}
	return false
}

func runtimeV2OperationAuthorizationForKind(manifest runtimecontract.RuntimeCandidateContractManifest, kind string) (runtimecontract.RuntimeCandidateOperationManifest, bool) {
	manifest = manifest.Normalized()
	kind = runtimecontract.CanonicalRuntimeBlockKind(kind)
	for _, operation := range manifest.Operations {
		if operation.Kind == kind {
			return operation, true
		}
	}
	return runtimecontract.RuntimeCandidateOperationManifest{}, false
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

// runtimeV2CandidateSchemaFailure preserves the decoder's provider-safe,
// manifest-derived diagnostic for the persisted SM-3 attempt. The retry
// policy remains in requestAndSealRuntimeV2CandidateWithRetry; this helper
// only translates typed contract diagnostics into the candidate failure DTO.
func runtimeV2CandidateSchemaFailure(decodeErr error) *runtimecontract.RuntimeCandidateFailure {
	failure := &runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid",
		Signature: "candidate-schema-invalid", Reason: decodeErr.Error(),
	}
	var payloadDiagnostic *runtimecontract.RuntimeCandidatePayloadDiagnostic
	if errors.As(decodeErr, &payloadDiagnostic) {
		failure.Diagnostic = runtimecontract.RuntimeCandidateFailureDiagnosticFromPayloadDiagnostic(payloadDiagnostic)
		return failure
	}
	var contractDiagnostic *runtimecontract.RuntimeCandidateContractDiagnostic
	if errors.As(decodeErr, &contractDiagnostic) {
		failure.Diagnostic = runtimecontract.RuntimeCandidateFailureDiagnosticFromContractDiagnostic(contractDiagnostic)
	}
	return failure
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
		if attempt == 1 {
			failure, failureErr := runtimeV2PreviousCandidateFailure(aggregate, attempt)
			if failureErr != nil {
				return runtimev2.RuntimeAggregate{}, failureErr
			}
			// N4: complete rejections are sealed as correctable_once or
			// rejected_final. Missing objectives are runtime_contract_defect,
			// never candidate_correction_undefined.
			if !runtimecontract.RuntimeCandidateFailureIsCorrectableOnce(failure) {
				sealed, directive, sealErr := runtimecontract.SealRuntimeCandidateRejection(failure, runtimecontract.RuntimeCandidateContractManifest{})
				if sealErr != nil {
					return runtimev2.RuntimeAggregate{}, sealErr
				}
				code := "runtime_contract_defect"
				message := "complete candidate rejection is not correctable under the sealed manifest"
				if directive != nil && directive.TerminalCode != "" {
					code = directive.TerminalCode
				}
				if sealed != nil && strings.TrimSpace(sealed.Reason) != "" {
					message = sealed.Reason
				}
				class := runtimecontract.FailureClassRuntimeContractMisalignment
				if code == "candidate_retry_exhausted" {
					class = runtimecontract.FailureClassCandidateExhausted
				}
				return m.bizPtr.RecordRuntimeV2FailureFact(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, runtimecontract.FailureFact{
					Class: class, Code: code, Signature: code, Message: message,
				})
			}
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
	_ = stablePrefixHash // Governance has no provider request or candidate attempt.
	dependency, found := runtimev2.GovernanceDependencyForAggregate(aggregate)
	if !found {
		// A reducer request_governance state must be backed by a persisted
		// external dependency. Never synthesize a generic decision ticket from
		// a focus, error string, or feasibility summary; make the missing
		// contract visible to SM-2 as a deterministic terminal instead.
		return m.bizPtr.RecordRuntimeV2FailureFact(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "governance_dependency_missing",
			Signature: "governance-dependency-missing", Message: "runtime governance requires a sealed external dependency",
		})
	}
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(aggregate.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(runtimecontract.RuntimeCandidateContextForDecision(aggregate.Facts, aggregate.Decision, nil).Resources))
	if err != nil {
		return runtimev2.RuntimeAggregate{}, err
	}
	sealed, err := m.bizPtr.SealRuntimeV2GovernanceDispatch(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, manifest, runtimecontract.RuntimeCandidate{
		Governance: &runtimecontract.GovernanceCandidate{ExternalDependency: dependency},
	})
	if err != nil {
		return runtimev2.RuntimeAggregate{}, err
	}
	return sealed, nil
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
