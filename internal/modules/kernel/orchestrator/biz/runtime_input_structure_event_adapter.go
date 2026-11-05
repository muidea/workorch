package orchestrator

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Compose two independent read-only owner facts inside the existing candidate
// transaction. File still owns feasibility/CAS; Shell alone interprets inputs.
func inspectRuntimeInputRevisionPostimage(ctx context.Context, hub magicevent.Hub, c rc.RuntimeOperationFeasibilityRequest, response rc.RuntimeOperationFeasibilityObservation) (rc.RuntimeOperationFeasibilityObservation, error) {
	constraint, required, err := rc.RuntimeInputStructureConstraintForOperation(c.Operation, c.OperationAuthorization)
	if err != nil {
		return rc.RuntimeOperationFeasibilityObservation{}, err
	}
	if !required {
		return response, nil
	}
	if response.ReplayProof == nil || response.Postimage == nil {
		return rc.RuntimeOperationFeasibilityObservation{}, fmt.Errorf("input revision lacks File postimage")
	}
	request := rc.RuntimeInputStructureRequest(string(c.WorkspaceID), string(c.RunID), c.Transaction, c.CandidateManifestHash, c.Operation, c.OperationAuthorization.InputRevision, *response.ReplayProof, *response.Postimage)
	if err := request.Validate(); err != nil {
		return rc.RuntimeOperationFeasibilityObservation{}, err
	}
	result := hub.Send(magicevent.NewEventWithContext(shellevents.TopicInspectRuntimeValidationInputStructure, ModuleID, rc.RuntimeModuleShell, magicevent.NewHeader(), ctx, shellevents.InspectRuntimeValidationInputStructureCommand(request)))
	if result == nil {
		return rc.RuntimeOperationFeasibilityObservation{}, common.ErrInvalidArgument
	}
	if err := prerequisiteResultError(result); err != nil {
		return rc.RuntimeOperationFeasibilityObservation{}, err
	}
	data, magicErr := result.Get()
	if magicErr != nil {
		return rc.RuntimeOperationFeasibilityObservation{}, prerequisiteMagicError(magicErr)
	}
	value, ok := data.(shellevents.InspectRuntimeValidationInputStructureResult)
	if !ok {
		return rc.RuntimeOperationFeasibilityObservation{}, fmt.Errorf("input structure owner returned invalid type")
	}
	fact := value.Observation
	if err := fact.ValidateForRequest(request); err != nil {
		return rc.RuntimeOperationFeasibilityObservation{}, err
	}
	response.InputStructure = fact.Clone()
	// Postimage is retained only in this exact fact, not duplicated in storage.
	response.Postimage = nil
	if fact.Code != "" {

		violation := rc.RuntimeOperationCandidateConstraintViolation{ConstraintRef: constraint.Ref, ParameterName: constraint.ParameterName, RequiredShape: constraint.RequiredShape, ActualShape: rc.RuntimeOperationCandidateActualShapeInputStructureInvalid, TargetRef: request.Postimage.TargetRef, ObservedRevision: c.Admissibility.ObservedRevision}
		rejected, err := rc.NewRuntimeOperationCandidateConstraintViolationObservation(c, violation)
		if err != nil {
			return rc.RuntimeOperationFeasibilityObservation{}, err
		}
		rejected.OwnerPolicy = response.OwnerPolicy
		rejected.InputStructure = fact.Clone()
		return rejected, nil
	}
	return response, nil
}
