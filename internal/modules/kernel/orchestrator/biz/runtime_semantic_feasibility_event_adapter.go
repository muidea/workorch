package orchestrator

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	gitevents "github.com/muidea/workorch/internal/modules/blocks/git/pkg/events"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/events"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	sshevents "github.com/muidea/workorch/internal/modules/blocks/ssh/pkg/events"
	webevents "github.com/muidea/workorch/internal/modules/blocks/web/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// ProbeRuntimeSemanticProofFeasibility is the sole cross-owner adapter for an
// SM-2 semantic feasibility fact. It rejects every scope or identity mismatch
// before the observation can become an Orchestrator-private internal event.
func ProbeRuntimeSemanticProofFeasibility(ctx context.Context, hub magicevent.Hub, command runtimecontract.RuntimeSemanticProofFeasibilityRequest) (runtimecontract.RuntimeSemanticProofFeasibilityObservation, error) {
	owner := command.Proposal.Normalized().OwnerModuleID
	if hub == nil || owner == "" {
		return runtimecontract.RuntimeSemanticProofFeasibilityObservation{}, common.ErrInvalidArgument
	}
	if err := runtimecontract.ValidateRuntimeSemanticProofFeasibilityRequest(command, owner); err != nil {
		return runtimecontract.RuntimeSemanticProofFeasibilityObservation{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result := sendRuntimeSemanticProofFeasibilityOwnerCommand(ctx, hub, command)
	if result == nil {
		return runtimecontract.RuntimeSemanticProofFeasibilityObservation{}, common.ErrInvalidArgument
	}
	if err := prerequisiteResultError(result); err != nil {
		return runtimecontract.RuntimeSemanticProofFeasibilityObservation{}, err
	}
	data, magicErr := result.Get()
	if magicErr != nil {
		return runtimecontract.RuntimeSemanticProofFeasibilityObservation{}, prerequisiteMagicError(magicErr)
	}
	response, ok := normalizeRuntimeSemanticProofFeasibilityOwnerResult(owner, data)
	if !ok || response.FeasibilityBatchID != command.FeasibilityBatchID {
		return runtimecontract.RuntimeSemanticProofFeasibilityObservation{}, fmt.Errorf("runtime semantic feasibility probe returned an invalid batch result")
	}
	if err := response.Observation.ValidateForProposal(command.Manifest, command.Proposal); err != nil {
		return runtimecontract.RuntimeSemanticProofFeasibilityObservation{}, err
	}
	if response.Observation.OwnerModuleID != owner || response.Observation.ManifestHash != command.Manifest.Hash ||
		response.Observation.DefinitionSeedHash != command.Manifest.DefinitionSeedHash {
		return runtimecontract.RuntimeSemanticProofFeasibilityObservation{}, fmt.Errorf("runtime semantic feasibility probe returned mismatched scope or owner identity")
	}
	return response, nil
}

func sendRuntimeSemanticProofFeasibilityOwnerCommand(ctx context.Context, hub magicevent.Hub, c runtimecontract.RuntimeSemanticProofFeasibilityRequest) magicevent.Result {
	owner := c.Proposal.Normalized().OwnerModuleID
	switch owner {
	case runtimecontract.RuntimeModuleFile:
		return hub.Send(magicevent.NewEventWithContext(fileevents.TopicProbeRuntimeSemanticProofFeasibility, ModuleID, owner, magicevent.NewHeader(), ctx, fileRuntimeSemanticProofFeasibilityCommand(c)))
	case runtimecontract.RuntimeModuleGit:
		return hub.Send(magicevent.NewEventWithContext(gitevents.TopicProbeRuntimeSemanticProofFeasibility, ModuleID, owner, magicevent.NewHeader(), ctx, gitRuntimeSemanticProofFeasibilityCommand(c)))
	case runtimecontract.RuntimeModuleMCP:
		return hub.Send(magicevent.NewEventWithContext(mcpevents.TopicProbeRuntimeSemanticProofFeasibility, ModuleID, owner, magicevent.NewHeader(), ctx, mcpRuntimeSemanticProofFeasibilityCommand(c)))
	case runtimecontract.RuntimeModuleShell:
		return hub.Send(magicevent.NewEventWithContext(shellevents.TopicProbeRuntimeSemanticProofFeasibility, ModuleID, owner, magicevent.NewHeader(), ctx, shellRuntimeSemanticProofFeasibilityCommand(c)))
	case runtimecontract.RuntimeModuleSSH:
		return hub.Send(magicevent.NewEventWithContext(sshevents.TopicProbeRuntimeSemanticProofFeasibility, ModuleID, owner, magicevent.NewHeader(), ctx, sshRuntimeSemanticProofFeasibilityCommand(c)))
	case runtimecontract.RuntimeModuleWeb:
		return hub.Send(magicevent.NewEventWithContext(webevents.TopicProbeRuntimeSemanticProofFeasibility, ModuleID, owner, magicevent.NewHeader(), ctx, webRuntimeSemanticProofFeasibilityCommand(c)))
	default:
		return nil
	}
}

func fileRuntimeSemanticProofFeasibilityCommand(c runtimecontract.RuntimeSemanticProofFeasibilityRequest) fileevents.ProbeRuntimeSemanticProofFeasibilityCommand {
	return fileevents.ProbeRuntimeSemanticProofFeasibilityCommand(c)
}
func gitRuntimeSemanticProofFeasibilityCommand(c runtimecontract.RuntimeSemanticProofFeasibilityRequest) gitevents.ProbeRuntimeSemanticProofFeasibilityCommand {
	return gitevents.ProbeRuntimeSemanticProofFeasibilityCommand(c)
}
func mcpRuntimeSemanticProofFeasibilityCommand(c runtimecontract.RuntimeSemanticProofFeasibilityRequest) mcpevents.ProbeRuntimeSemanticProofFeasibilityCommand {
	return mcpevents.ProbeRuntimeSemanticProofFeasibilityCommand(c)
}
func shellRuntimeSemanticProofFeasibilityCommand(c runtimecontract.RuntimeSemanticProofFeasibilityRequest) shellevents.ProbeRuntimeSemanticProofFeasibilityCommand {
	return shellevents.ProbeRuntimeSemanticProofFeasibilityCommand(c)
}
func sshRuntimeSemanticProofFeasibilityCommand(c runtimecontract.RuntimeSemanticProofFeasibilityRequest) sshevents.ProbeRuntimeSemanticProofFeasibilityCommand {
	return sshevents.ProbeRuntimeSemanticProofFeasibilityCommand(c)
}
func webRuntimeSemanticProofFeasibilityCommand(c runtimecontract.RuntimeSemanticProofFeasibilityRequest) webevents.ProbeRuntimeSemanticProofFeasibilityCommand {
	return webevents.ProbeRuntimeSemanticProofFeasibilityCommand(c)
}

func normalizeRuntimeSemanticProofFeasibilityOwnerResult(owner string, data any) (runtimecontract.RuntimeSemanticProofFeasibilityObservation, bool) {
	switch owner {
	case runtimecontract.RuntimeModuleFile:
		v, ok := data.(fileevents.ProbeRuntimeSemanticProofFeasibilityResult)
		return runtimecontract.RuntimeSemanticProofFeasibilityObservation(v), ok
	case runtimecontract.RuntimeModuleGit:
		v, ok := data.(gitevents.ProbeRuntimeSemanticProofFeasibilityResult)
		return runtimecontract.RuntimeSemanticProofFeasibilityObservation(v), ok
	case runtimecontract.RuntimeModuleMCP:
		v, ok := data.(mcpevents.ProbeRuntimeSemanticProofFeasibilityResult)
		return runtimecontract.RuntimeSemanticProofFeasibilityObservation(v), ok
	case runtimecontract.RuntimeModuleShell:
		v, ok := data.(shellevents.ProbeRuntimeSemanticProofFeasibilityResult)
		return runtimecontract.RuntimeSemanticProofFeasibilityObservation(v), ok
	case runtimecontract.RuntimeModuleSSH:
		v, ok := data.(sshevents.ProbeRuntimeSemanticProofFeasibilityResult)
		return runtimecontract.RuntimeSemanticProofFeasibilityObservation(v), ok
	case runtimecontract.RuntimeModuleWeb:
		v, ok := data.(webevents.ProbeRuntimeSemanticProofFeasibilityResult)
		return runtimecontract.RuntimeSemanticProofFeasibilityObservation(v), ok
	default:
		return runtimecontract.RuntimeSemanticProofFeasibilityObservation{}, false
	}
}
