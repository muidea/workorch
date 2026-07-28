package orchestrator

// Candidate adapter is internal to the Runtime V2 owner Biz.

import (
	"context"
	"fmt"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// PreflightRuntimeCandidate asks the LLM owner to render the sealed request
// and validate C0-C5/P0-P4 before a candidate attempt is persisted. It is a
// typed read-only EventHub exchange: no provider call and no candidate can be
// produced by this path.
func PreflightRuntimeCandidate(ctx context.Context, hub magicevent.Hub, command orchestratorevents.PreflightRuntimeCandidateCommand) (orchestratorevents.PreflightRuntimeCandidateResult, error) {
	if hub == nil || command.WorkspaceID == "" || command.RunID == "" {
		return orchestratorevents.PreflightRuntimeCandidateResult{}, common.ErrInvalidArgument
	}
	if command.Decision.AuthoritySnapshot.WorkspaceID != command.WorkspaceID || command.Decision.AuthoritySnapshot.RunID != command.RunID {
		return orchestratorevents.PreflightRuntimeCandidateResult{}, fmt.Errorf("runtime candidate preflight scope does not match authority snapshot")
	}
	if err := runtimecontract.ValidateAuthorityTransaction(command.Decision, command.Transaction); err != nil {
		return orchestratorevents.PreflightRuntimeCandidateResult{}, err
	}
	if err := command.DefinitionContext.Validate(); err != nil {
		return orchestratorevents.PreflightRuntimeCandidateResult{}, err
	}
	if command.DefinitionContext.Normalized().Revision != command.Transaction.DefinitionRevision {
		return orchestratorevents.PreflightRuntimeCandidateResult{}, fmt.Errorf("runtime candidate preflight definition revision does not match authority transaction")
	}
	if err := command.CandidateContext.Validate(command.Decision); err != nil {
		return orchestratorevents.PreflightRuntimeCandidateResult{}, err
	}
	if err := runtimecontract.ValidateRuntimeCandidateContractManifestForDecision(command.CandidateManifest, command.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(command.CandidateContext.Resources)); err != nil {
		return orchestratorevents.PreflightRuntimeCandidateResult{}, err
	}
	if err := validateRuntimeCandidateRetryContract(command.CandidateAttempt, command.PreviousFailure); err != nil {
		return orchestratorevents.PreflightRuntimeCandidateResult{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorPreflightRuntimeCandidate,
		ModuleID,
		runtimecontract.RuntimeModuleLLM,
		magicevent.NewHeader(),
		ctx,
		command,
	))
	if result == nil {
		return orchestratorevents.PreflightRuntimeCandidateResult{}, common.ErrInvalidArgument
	}
	if err := prerequisiteResultError(result); err != nil {
		return orchestratorevents.PreflightRuntimeCandidateResult{}, err
	}
	data, magicErr := result.Get()
	if magicErr != nil {
		return orchestratorevents.PreflightRuntimeCandidateResult{}, prerequisiteMagicError(magicErr)
	}
	response, ok := data.(orchestratorevents.PreflightRuntimeCandidateResult)
	if !ok {
		return orchestratorevents.PreflightRuntimeCandidateResult{}, fmt.Errorf("LLM block returned an invalid runtime candidate preflight result")
	}
	if err := runtimecontract.ValidateAuthorityTransaction(command.Decision, response.Transaction); err != nil {
		return orchestratorevents.PreflightRuntimeCandidateResult{}, err
	}
	if response.Ready {
		if strings.TrimSpace(response.Reason) != "" {
			return orchestratorevents.PreflightRuntimeCandidateResult{}, fmt.Errorf("ready runtime candidate preflight returned a failure reason")
		}
	} else if strings.TrimSpace(response.Reason) == "" {
		return orchestratorevents.PreflightRuntimeCandidateResult{}, fmt.Errorf("runtime candidate preflight was not ready without a reason")
	}
	if response.Audit != nil && response.Audit.ProviderSubmitted {
		return orchestratorevents.PreflightRuntimeCandidateResult{}, fmt.Errorf("runtime candidate preflight must not submit a provider request")
	}
	return response, nil
}

func RequestRuntimeCandidate(ctx context.Context, hub magicevent.Hub, command orchestratorevents.RequestRuntimeCandidateCommand) (orchestratorevents.RequestRuntimeCandidateResult, error) {
	if hub == nil || command.WorkspaceID == "" || command.RunID == "" {
		return orchestratorevents.RequestRuntimeCandidateResult{}, common.ErrInvalidArgument
	}
	if command.Decision.AuthoritySnapshot.WorkspaceID != command.WorkspaceID || command.Decision.AuthoritySnapshot.RunID != command.RunID {
		return orchestratorevents.RequestRuntimeCandidateResult{}, fmt.Errorf("runtime candidate command scope does not match authority snapshot")
	}
	if err := runtimecontract.ValidateAuthorityTransaction(command.Decision, command.Transaction); err != nil {
		return orchestratorevents.RequestRuntimeCandidateResult{}, err
	}
	if err := command.DefinitionContext.Validate(); err != nil {
		return orchestratorevents.RequestRuntimeCandidateResult{}, err
	}
	if command.DefinitionContext.Normalized().Revision != command.Transaction.DefinitionRevision {
		return orchestratorevents.RequestRuntimeCandidateResult{}, fmt.Errorf("runtime candidate definition revision does not match authority transaction")
	}
	if err := command.CandidateContext.Validate(command.Decision); err != nil {
		return orchestratorevents.RequestRuntimeCandidateResult{}, err
	}
	if err := runtimecontract.ValidateRuntimeCandidateContractManifestForDecision(command.CandidateManifest, command.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(command.CandidateContext.Resources)); err != nil {
		return orchestratorevents.RequestRuntimeCandidateResult{}, err
	}
	if command.OutputContinuation != nil {
		if err := command.OutputContinuation.Validate(); err != nil {
			return orchestratorevents.RequestRuntimeCandidateResult{}, err
		}
	}
	if err := validateRuntimeCandidateRetryContract(command.CandidateAttempt, command.PreviousFailure); err != nil {
		return orchestratorevents.RequestRuntimeCandidateResult{}, err
	}
	if err := validateRuntimeCandidateRequestIdentity(command.LogicalRequestID, command.RequestSequence, command.PhysicalCallID); err != nil {
		return orchestratorevents.RequestRuntimeCandidateResult{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorRequestRuntimeCandidate,
		ModuleID,
		runtimecontract.RuntimeModuleLLM,
		magicevent.NewHeader(),
		ctx,
		command,
	))
	if result == nil {
		return orchestratorevents.RequestRuntimeCandidateResult{}, common.ErrInvalidArgument
	}
	if err := prerequisiteResultError(result); err != nil {
		return orchestratorevents.RequestRuntimeCandidateResult{}, err
	}
	data, magicErr := result.Get()
	if magicErr != nil {
		return orchestratorevents.RequestRuntimeCandidateResult{}, prerequisiteMagicError(magicErr)
	}
	response, ok := data.(orchestratorevents.RequestRuntimeCandidateResult)
	if !ok {
		return orchestratorevents.RequestRuntimeCandidateResult{}, fmt.Errorf("LLM block returned an invalid runtime candidate result")
	}
	if err := runtimecontract.ValidateAuthorityTransaction(command.Decision, response.Transaction); err != nil {
		return orchestratorevents.RequestRuntimeCandidateResult{}, err
	}
	if response.LogicalRequestID != command.LogicalRequestID || response.RequestSequence != command.RequestSequence || response.PhysicalCallID != command.PhysicalCallID {
		return orchestratorevents.RequestRuntimeCandidateResult{}, fmt.Errorf("runtime candidate result identity does not match request")
	}
	if response.Stale != nil {
		if len(response.DocumentContent) != 0 {
			return orchestratorevents.RequestRuntimeCandidateResult{}, fmt.Errorf("LLM block returned stale together with a complete candidate document")
		}
		if response.Failure != nil {
			return orchestratorevents.RequestRuntimeCandidateResult{}, fmt.Errorf("LLM block returned stale together with a candidate failure")
		}
		stale := response.Stale.Normalized()
		if err := stale.Validate(); err != nil {
			return orchestratorevents.RequestRuntimeCandidateResult{}, fmt.Errorf("LLM block returned an invalid runtime prerequisite stale: %w", err)
		}
		response.Stale = &stale
		return response, nil
	}
	if response.Failure != nil {
		if response.OutputFragment != nil {
			return orchestratorevents.RequestRuntimeCandidateResult{}, fmt.Errorf("LLM block returned an output fragment together with a candidate failure")
		}
		if response.Failure.Class == "" || response.Failure.Reason == "" {
			return orchestratorevents.RequestRuntimeCandidateResult{}, fmt.Errorf("LLM block returned an invalid runtime candidate failure")
		}
		if len(response.DocumentContent) != 0 {
			switch response.Failure.Class {
			case runtimecontract.RuntimeCandidateFailureClassSchema, runtimecontract.RuntimeCandidateFailureClassContract:
			default:
				return orchestratorevents.RequestRuntimeCandidateResult{}, fmt.Errorf("LLM block returned a complete candidate document for non-candidate failure %s", response.Failure.Class)
			}
		}
		return response, nil
	}
	if response.OutputFragment != nil {
		if len(response.DocumentContent) != 0 {
			return orchestratorevents.RequestRuntimeCandidateResult{}, fmt.Errorf("LLM block returned an output fragment together with a complete candidate document")
		}
		if err := response.OutputFragment.Validate(); err != nil {
			return orchestratorevents.RequestRuntimeCandidateResult{}, fmt.Errorf("LLM block returned an invalid runtime candidate output fragment: %w", err)
		}
		// An initial fragment is selected by the typed decoder's structural
		// incomplete-document outcome. ReachedProviderLimit is audit telemetry,
		// not a state-machine gate: a mid-token EOF must continue exactly like a
		// token-limit EOF.
		return response, nil
	}
	if command.OutputContinuation != nil {
		return orchestratorevents.RequestRuntimeCandidateResult{}, fmt.Errorf("runtime candidate output continuation returned neither a fragment nor a failure")
	}
	// A complete document is the authoritative C4 handoff. Semantic decode and
	// candidate admission are exclusively owned by SM-3 after persistence.
	if len(response.DocumentContent) != 0 {
		return response, nil
	}
	return orchestratorevents.RequestRuntimeCandidateResult{}, fmt.Errorf("runtime candidate response returned neither a complete document nor an outcome")
}

// validateRuntimeCandidateRequestIdentity keeps the EventHub boundary on the
// same pure value contract as the LLM owner. Every physical provider request
// must carry its persisted-session identity; empty compatibility triples are
// not a Runtime V2 contract.
func validateRuntimeCandidateRequestIdentity(logicalRequestID string, sequence int, physicalCallID string) error {
	return runtimecontract.ValidateRuntimeCandidateRequestIdentity(logicalRequestID, sequence, physicalCallID)
}

func validateRuntimeCandidateRetryContract(attempt int, previousFailure *runtimecontract.RuntimeCandidateFailure) error {
	if attempt < 0 || attempt > 1 {
		return fmt.Errorf("runtime candidate attempt must be 0 or 1")
	}
	if attempt == 0 {
		if previousFailure != nil {
			return fmt.Errorf("initial runtime candidate attempt must not carry a previous failure")
		}
		return nil
	}
	if previousFailure == nil || previousFailure.Class == "" || strings.TrimSpace(previousFailure.Reason) == "" {
		return fmt.Errorf("runtime candidate retry requires the previous failure")
	}
	return nil
}
