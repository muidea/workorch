package orchestrator

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

// ProbeRuntimeSemanticProofFeasibility is the sole cross-owner adapter for an
// SM-2 semantic feasibility fact. It rejects every scope or identity mismatch
// before the observation can become an Orchestrator-private internal event.
func ProbeRuntimeSemanticProofFeasibility(ctx context.Context, hub magicevent.Hub, command orchestratorevents.ProbeRuntimeSemanticProofFeasibilityCommand) (orchestratorevents.ProbeRuntimeSemanticProofFeasibilityResult, error) {
	owner := command.Proposal.Normalized().OwnerModuleID
	if hub == nil || owner == "" {
		return orchestratorevents.ProbeRuntimeSemanticProofFeasibilityResult{}, common.ErrInvalidArgument
	}
	if err := orchestratorevents.ValidateRuntimeSemanticProofFeasibilityCommand(command, owner); err != nil {
		return orchestratorevents.ProbeRuntimeSemanticProofFeasibilityResult{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorProbeRuntimeSemanticProofFeasibility,
		ModuleID,
		owner,
		magicevent.NewHeader(),
		ctx,
		command,
	))
	if result == nil {
		return orchestratorevents.ProbeRuntimeSemanticProofFeasibilityResult{}, common.ErrInvalidArgument
	}
	if err := prerequisiteResultError(result); err != nil {
		return orchestratorevents.ProbeRuntimeSemanticProofFeasibilityResult{}, err
	}
	data, magicErr := result.Get()
	if magicErr != nil {
		return orchestratorevents.ProbeRuntimeSemanticProofFeasibilityResult{}, prerequisiteMagicError(magicErr)
	}
	response, ok := data.(orchestratorevents.ProbeRuntimeSemanticProofFeasibilityResult)
	if !ok || response.FeasibilityBatchID != command.FeasibilityBatchID {
		return orchestratorevents.ProbeRuntimeSemanticProofFeasibilityResult{}, fmt.Errorf("runtime semantic feasibility probe returned an invalid batch result")
	}
	if err := response.Observation.ValidateForProposal(command.Manifest, command.Proposal); err != nil {
		return orchestratorevents.ProbeRuntimeSemanticProofFeasibilityResult{}, err
	}
	if response.Observation.OwnerModuleID != owner || response.Observation.ManifestHash != command.Manifest.Hash ||
		response.Observation.DefinitionSeedHash != command.Manifest.DefinitionSeedHash {
		return orchestratorevents.ProbeRuntimeSemanticProofFeasibilityResult{}, fmt.Errorf("runtime semantic feasibility probe returned mismatched scope or owner identity")
	}
	return response, nil
}
