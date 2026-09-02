package orchestrator

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// ValidateRuntimeOperationCandidateConformance obtains one side-effect-free
// capability-owner fact. The adapter selects only the receiver-owned concrete
// EventHub contract; retry/terminal policy remains inside Orchestrator SM-3.
func ValidateRuntimeOperationCandidateConformance(ctx context.Context, hub magicevent.Hub, command runtimecontract.RuntimeOperationCandidateConformanceRequest) (runtimecontract.RuntimeOperationCandidateConformanceObservation, error) {
	if hub == nil {
		return runtimecontract.RuntimeOperationCandidateConformanceObservation{}, common.ErrInvalidArgument
	}
	command = command.Normalized()
	owner := command.OperationAuthorization.OwnerModuleID
	if err := runtimecontract.ValidateRuntimeOperationCandidateConformanceRequest(command, owner); err != nil {
		return runtimecontract.RuntimeOperationCandidateConformanceObservation{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var result magicevent.Result
	switch owner {
	case runtimecontract.RuntimeModuleFile:
		result = hub.Send(magicevent.NewEventWithContext(
			fileevents.TopicValidateRuntimeOperationCandidateConformance,
			ModuleID, owner, magicevent.NewHeader(), ctx,
			fileevents.ValidateRuntimeOperationCandidateConformanceCommand(command),
		))
	default:
		return runtimecontract.RuntimeOperationCandidateConformanceObservation{}, fmt.Errorf("runtime operation candidate conformance owner %q has no concrete receiver contract", owner)
	}
	if result == nil {
		return runtimecontract.RuntimeOperationCandidateConformanceObservation{}, common.ErrInvalidArgument
	}
	if err := prerequisiteResultError(result); err != nil {
		return runtimecontract.RuntimeOperationCandidateConformanceObservation{}, err
	}
	data, magicErr := result.Get()
	if magicErr != nil {
		return runtimecontract.RuntimeOperationCandidateConformanceObservation{}, prerequisiteMagicError(magicErr)
	}
	response, ok := data.(fileevents.ValidateRuntimeOperationCandidateConformanceResult)
	if !ok {
		return runtimecontract.RuntimeOperationCandidateConformanceObservation{}, fmt.Errorf("runtime operation candidate conformance owner %q returned an invalid result", owner)
	}
	observation := runtimecontract.RuntimeOperationCandidateConformanceObservation(response).Normalized()
	if err := observation.ValidateForRequest(command, owner); err != nil {
		return runtimecontract.RuntimeOperationCandidateConformanceObservation{}, fmt.Errorf("runtime operation candidate conformance owner %q returned an invalid result: %w", owner, err)
	}
	return observation.Clone(), nil
}
