package orchestrator

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// ProbeRuntimeOwnerFactProspectiveSatisfaction is the only cross-owner adapter
// for the second half of an SM-2 prospective owner-fact proof. File has already
// authored the hypothetical manifest delta; the required-fact owner now judges
// that immutable value without executing an operation or transitioning Runtime.
func ProbeRuntimeOwnerFactProspectiveSatisfaction(ctx context.Context, hub magicevent.Hub, command runtimecontract.RuntimeOwnerFactProspectiveSatisfactionRequest) (runtimecontract.RuntimeOwnerFactProspectiveSatisfactionObservation, error) {
	command = command.Clone()
	if hub == nil {
		return runtimecontract.RuntimeOwnerFactProspectiveSatisfactionObservation{}, common.ErrInvalidArgument
	}
	if err := command.Validate(); err != nil {
		return runtimecontract.RuntimeOwnerFactProspectiveSatisfactionObservation{}, err
	}
	if command.Requirement.OwnerModuleID != runtimecontract.RuntimeModuleShell {
		return runtimecontract.RuntimeOwnerFactProspectiveSatisfactionObservation{}, fmt.Errorf("runtime prospective satisfaction owner %q has no typed adapter", command.Requirement.OwnerModuleID)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result := hub.Send(magicevent.NewEventWithContext(
		shellevents.TopicProbeRuntimeOwnerFactProspectiveSatisfaction,
		ModuleID,
		command.Requirement.OwnerModuleID,
		magicevent.NewHeader(),
		ctx,
		shellevents.ProbeRuntimeOwnerFactProspectiveSatisfactionCommand(command),
	))
	if result == nil {
		return runtimecontract.RuntimeOwnerFactProspectiveSatisfactionObservation{}, common.ErrInvalidArgument
	}
	if err := prerequisiteResultError(result); err != nil {
		return runtimecontract.RuntimeOwnerFactProspectiveSatisfactionObservation{}, err
	}
	data, magicErr := result.Get()
	if magicErr != nil {
		return runtimecontract.RuntimeOwnerFactProspectiveSatisfactionObservation{}, prerequisiteMagicError(magicErr)
	}
	ownerResult, ok := data.(shellevents.ProbeRuntimeOwnerFactProspectiveSatisfactionResult)
	if !ok {
		return runtimecontract.RuntimeOwnerFactProspectiveSatisfactionObservation{}, fmt.Errorf("runtime prospective satisfaction probe returned an invalid result")
	}
	observation := ownerResult.Observation.Clone()
	if err := observation.ValidateForRequest(command); err != nil {
		return runtimecontract.RuntimeOwnerFactProspectiveSatisfactionObservation{}, err
	}
	return observation, nil
}
