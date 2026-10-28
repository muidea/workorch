package workspace

import (
	"reflect"

	workspaceeventmapping "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/eventmapping"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func reduceDefinitionDerivation(o definitionSpecificationObservation, e definitionSpecificationInternalEvent) (definitionSpecificationTransition, error) {
	w := o.workspace
	if w == nil || w.ID != e.WorkspaceID {
		return definitionSpecificationTransition{}, common.ErrConflict
	}
	inputHash, permitID := "", ""
	if e.DeriveReserve != nil {
		inputHash = e.DeriveReserve.InputHash
	} else if e.DeriveComplete != nil {
		permitID = e.DeriveComplete.PermitID
	}
	record, candidate, selectErr := authoringMutationRecord(w, inputHash, permitID, false)
	if selectErr != nil {
		return definitionSpecificationTransition{}, selectErr
	}
	if candidate && e.DeriveReserve != nil && e.ObservedChangeInputHash != record.InputHash {
		return definitionSpecificationTransition{}, common.ErrConflict
	}
	if err := validateWorkspaceAuthoringMappingOutcomes(w); err != nil {
		return definitionSpecificationTransition{}, err
	}
	var next workspace.DefinitionAuthoringRecord
	var request workspace.DefinitionDerivationRequest
	var fresh bool
	var err error
	if cmd := e.DeriveReserve; cmd != nil {
		if cmd.Validate() != nil || cmd.InputHash != record.InputHash || cmd.WorkspaceID != w.ID || o.template == nil || workspace.WorkspaceTemplateSnapshotHash(o.template) != record.TemplateSnapshotHash || workspace.ValidateDefinitionAuthoringResources(record.Input, cmd.Resources) != nil || (len(cmd.Resources) > 0 && (!e.VerifiedResources || cmd.WorkspaceRevision != w.Revision)) {
			return definitionSpecificationTransition{}, common.ErrConflict
		}
		next, request, fresh, err = record.ReserveDerivation(cmd.RequestID, cmd.ProviderBindingHash, cmd.Resources, cmd.WireContractRef)
	} else if cmd := e.DeriveComplete; cmd != nil {
		if cmd.Validate() != nil || cmd.WorkspaceID != w.ID {
			return definitionSpecificationTransition{}, common.ErrInvalidArgument
		}
		next, err = record.CompleteDerivationDocument(cmd.PermitID, cmd.Proposal, cmd.FailureCode, cmd.SuccessAudit, cmd.Diagnostic)
		if err == nil {
			for _, item := range next.DerivationRequests {
				if item.PermitID == cmd.PermitID {
					request = item.Clone()
					break
				}
			}
		}
	} else {
		return definitionSpecificationTransition{}, common.ErrInvalidArgument
	}
	if err != nil {
		return definitionSpecificationTransition{}, err
	}
	if err := validateAuthoringMappingOutcomes(next); err != nil {
		return definitionSpecificationTransition{}, err
	}
	result := workspaceevents.DefinitionDerivationRequestResult{Request: workspaceeventmapping.DefinitionDerivationRequestDTO(request), Consumed: uint32(len(next.DerivationRequests)), Remaining: uint32(workspace.DefinitionDerivationRequestLimit - len(next.DerivationRequests))}
	if fresh {
		result.FreshPermitID = request.PermitID
	}
	transition := definitionSpecificationTransition{Derivation: &result}
	if !reflect.DeepEqual(*record, next) {
		transition.Next = withAuthoringMutationRecord(w, next, candidate)
	}
	return transition, nil
}
