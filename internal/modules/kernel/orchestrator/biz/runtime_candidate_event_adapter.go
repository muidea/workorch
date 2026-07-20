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
	if err := validateRuntimeCandidateRetryContract(command.CandidateAttempt, command.PreviousFailure); err != nil {
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
	if response.Stale != nil {
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
		if response.Failure.Class == "" || response.Failure.Reason == "" {
			return orchestratorevents.RequestRuntimeCandidateResult{}, fmt.Errorf("LLM block returned an invalid runtime candidate failure")
		}
		return response, nil
	}
	// Revalidate at the EventHub boundary with the same activation-sealed MCP
	// metadata that the LLM boundary used.  Calling Validate without activation
	// discards the exact non-durable server/tool declaration and incorrectly
	// treats every MCP tool call as mutating.
	if err := response.Candidate.ValidateWithActivation(command.Decision, command.CandidateContext.Activation); err != nil {
		return orchestratorevents.RequestRuntimeCandidateResult{}, err
	}
	return response, nil
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
