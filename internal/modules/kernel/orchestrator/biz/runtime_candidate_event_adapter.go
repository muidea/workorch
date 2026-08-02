package orchestrator

// Candidate adapter is internal to the Runtime owner Biz.

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
	if err := validateRuntimeCandidatePreflightCommand(command); err != nil {
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
	if response.DocumentKind != command.DocumentKind {
		return orchestratorevents.PreflightRuntimeCandidateResult{}, fmt.Errorf("runtime candidate preflight document kind does not match request")
	}
	if command.DocumentKind == runtimecontract.RuntimeProviderDocumentOperations {
		if err := runtimecontract.ValidateAuthorityTransaction(command.Decision, response.Transaction); err != nil {
			return orchestratorevents.PreflightRuntimeCandidateResult{}, err
		}
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
	if err := validateRuntimeCandidateCommand(command); err != nil {
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
	if response.DocumentKind != command.DocumentKind {
		return orchestratorevents.RequestRuntimeCandidateResult{}, fmt.Errorf("runtime candidate result document kind does not match request")
	}
	if command.DocumentKind == runtimecontract.RuntimeProviderDocumentOperations {
		if err := runtimecontract.ValidateAuthorityTransaction(command.Decision, response.Transaction); err != nil {
			return orchestratorevents.RequestRuntimeCandidateResult{}, err
		}
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
// must carry its complete persisted-session identity.
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

func validateRuntimeCandidatePreflightCommand(command orchestratorevents.PreflightRuntimeCandidateCommand) error {
	switch command.DocumentKind {
	case runtimecontract.RuntimeProviderDocumentOperations:
		if command.SemanticManifest.Hash != "" || command.SemanticContext.WorkspaceID != "" || command.SemanticContext.RunID != "" || command.PreviousSemanticRejection != nil {
			return fmt.Errorf("operations candidate preflight must not carry semantic plan fields")
		}
		if command.Decision.AuthoritySnapshot.WorkspaceID != command.WorkspaceID || command.Decision.AuthoritySnapshot.RunID != command.RunID {
			return fmt.Errorf("runtime candidate preflight scope does not match authority snapshot")
		}
		if err := runtimecontract.ValidateAuthorityTransaction(command.Decision, command.Transaction); err != nil {
			return err
		}
		if err := command.DefinitionContext.Validate(); err != nil {
			return err
		}
		if command.DefinitionContext.Normalized().Revision != command.Transaction.DefinitionRevision {
			return fmt.Errorf("runtime candidate preflight definition revision does not match authority transaction")
		}
		if err := command.CandidateContext.Validate(command.Decision); err != nil {
			return err
		}
		if err := runtimecontract.ValidateRuntimeCandidateContractManifestForDecision(command.CandidateManifest, command.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(command.CandidateContext)); err != nil {
			return err
		}
		return validateRuntimeCandidateRetryContract(command.CandidateAttempt, command.PreviousFailure)
	case runtimecontract.RuntimeProviderDocumentSemanticPlan:
		if command.CandidateManifest.Hash != "" || command.CandidateContext.WorkspaceID != "" || command.CandidateContext.RunID != "" || command.DefinitionContext.Revision != "" || command.PreviousFailure != nil {
			return fmt.Errorf("semantic plan preflight must not carry operations fields")
		}
		if command.SemanticContext.WorkspaceID != command.WorkspaceID || command.SemanticContext.RunID != command.RunID {
			return fmt.Errorf("runtime semantic plan preflight scope mismatch")
		}
		if err := command.SemanticContext.Validate(command.SemanticManifest); err != nil {
			return err
		}
		return validateRuntimeSemanticRetryContract(command.CandidateAttempt, command.PreviousSemanticRejection)
	default:
		return fmt.Errorf("runtime candidate preflight requires explicit document kind")
	}
}

func validateRuntimeCandidateCommand(command orchestratorevents.RequestRuntimeCandidateCommand) error {
	preflight := orchestratorevents.PreflightRuntimeCandidateCommand{
		WorkspaceID: command.WorkspaceID, RunID: command.RunID, DocumentKind: command.DocumentKind,
		Decision: command.Decision, Transaction: command.Transaction, CandidateAttempt: command.CandidateAttempt,
		PreviousFailure: command.PreviousFailure, StablePrefixHash: command.StablePrefixHash,
		CandidateManifest: command.CandidateManifest, DefinitionContext: command.DefinitionContext, CandidateContext: command.CandidateContext,
		SemanticManifest: command.SemanticManifest, SemanticContext: command.SemanticContext, PreviousSemanticRejection: command.PreviousSemanticRejection,
	}
	if err := validateRuntimeCandidatePreflightCommand(preflight); err != nil {
		return err
	}
	if command.OutputContinuation != nil {
		if err := command.OutputContinuation.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func validateRuntimeSemanticRetryContract(attempt int, rejection *runtimecontract.RuntimeOriginalDiagnostic) error {
	if attempt < 0 || attempt > 1 {
		return fmt.Errorf("runtime semantic plan attempt must be 0 or 1")
	}
	if attempt == 0 {
		if rejection != nil {
			return fmt.Errorf("initial runtime semantic plan attempt must not carry a previous rejection")
		}
		return nil
	}
	if rejection == nil {
		return fmt.Errorf("runtime semantic plan retry requires the previous rejection")
	}
	return rejection.Validate()
}
