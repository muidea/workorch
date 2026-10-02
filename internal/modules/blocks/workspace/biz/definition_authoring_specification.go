package workspace

import (
	"reflect"

	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func reduceAuthoringSpecification(o definitionSpecificationObservation, e definitionSpecificationInternalEvent) (definitionSpecificationTransition, error) {
	w := o.workspace
	if w == nil || w.ID != e.WorkspaceID || w.DefinitionAuthoring == nil || w.StartupInput != nil || w.ActivationAdmission != nil || w.ActiveRunID != "" || w.DefinitionAuthoringConfirmation != nil || len(w.DefinitionAdmissionCandidates) > 0 && !authoringSuccessorDraft(w) {
		return definitionSpecificationTransition{}, common.ErrConflict
	}
	record := w.DefinitionAuthoring
	if err := record.Validate(); err != nil {
		return definitionSpecificationTransition{}, err
	}
	var next workspace.DefinitionAuthoringRecord
	var request workspace.DefinitionAuthoringSpecificationRequest
	var fresh bool
	var err error
	if cmd := e.AuthoringSpecReserve; cmd != nil {
		if cmd.Validate() != nil || cmd.WorkspaceID != w.ID || cmd.InputHash != record.InputHash || o.template == nil || workspace.WorkspaceTemplateSnapshotHash(o.template) != record.TemplateSnapshotHash {
			return definitionSpecificationTransition{}, common.ErrConflict
		}
		next, request, fresh, err = record.ReserveSpecification(cmd.Request())
	} else if cmd := e.AuthoringSpecComplete; cmd != nil {
		if cmd.Validate() != nil || cmd.WorkspaceID != w.ID {
			return definitionSpecificationTransition{}, common.ErrInvalidArgument
		}
		next, err = record.CompleteSpecification(cmd.PermitID, cmd.Proposal, cmd.Review, cmd.FailureCode, cmd.Diagnostic)
		if err == nil {
			for _, item := range next.SpecificationRequests {
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
	result := workspaceevents.AuthoringSpecificationRequestResult{Request: request.Clone(), Consumed: uint32(len(next.SpecificationRequests)), Remaining: uint32(workspace.DefinitionAuthoringSpecificationRequestLimit - len(next.SpecificationRequests))}
	if fresh {
		result.FreshPermitID = request.PermitID
	}
	transition := definitionSpecificationTransition{AuthoringSpecification: &result}
	if !reflect.DeepEqual(*record, next) {
		updated := *w
		updated.DefinitionAuthoring = &next
		transition.Next = &updated
	}
	return transition, nil
}
