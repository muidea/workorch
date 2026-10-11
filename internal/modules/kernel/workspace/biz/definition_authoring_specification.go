package workspace

import (
	"reflect"

	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func reduceAuthoringSpecification(o definitionSpecificationObservation, e definitionSpecificationInternalEvent) (definitionSpecificationTransition, error) {
	w := o.workspace
	if w == nil || w.ID != e.WorkspaceID {
		return definitionSpecificationTransition{}, common.ErrConflict
	}
	inputHash, permitID := "", ""
	if e.AuthoringSpecReserve != nil {
		inputHash = e.AuthoringSpecReserve.InputHash
	} else if e.AuthoringSpecComplete != nil {
		permitID = e.AuthoringSpecComplete.PermitID
	}
	record, candidate, selectErr := authoringMutationRecord(w, inputHash, permitID, true)
	if selectErr != nil {
		return definitionSpecificationTransition{}, selectErr
	}
	if candidate && e.AuthoringSpecReserve != nil && e.ObservedChangeInputHash != record.InputHash {
		return definitionSpecificationTransition{}, common.ErrConflict
	}
	if err := validateWorkspaceAuthoringMappingOutcomes(w); err != nil {
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
		if cmd.Mapping != nil {
			var derivation *workspace.DefinitionDerivationRequest
			for _, item := range record.SpecificationRequests {
				if item.PermitID != cmd.PermitID {
					continue
				}
				if item.Phase != workspace.DefinitionSpecificationProposal {
					return definitionSpecificationTransition{}, common.ErrConflict
				}
				for index := range record.DerivationRequests {
					candidate := &record.DerivationRequests[index]
					if candidate.PermitID == item.DerivationPermitID {
						derivation = candidate
						break
					}
				}
				break
			}
			if derivation == nil {
				return definitionSpecificationTransition{}, common.ErrConflict
			}
			decoded, decodeErr := workspace.DecodeDefinitionAuthoringMapping(cmd.SuccessAudit.Document.Content)
			if decodeErr != nil || !reflect.DeepEqual(decoded, *cmd.Mapping) {
				return definitionSpecificationTransition{}, common.ErrConflict
			}
			proposal, semanticErr := workspace.CompileDefinitionAuthoringMapping(*derivation, decoded)
			if semanticErr == nil && cmd.Proposal != nil && !reflect.DeepEqual(proposal, *cmd.Proposal) {
				return definitionSpecificationTransition{}, common.ErrConflict
			}
			if semanticErr == nil {
				semanticErr = validateAuthoringVerificationAgainstSealedTemplate(workspace.WorkspaceTemplateSnapshotHash(&derivation.Template), *derivation, proposal)
			}
			if semanticErr != nil {
				diagnostic := workspace.MappingFailureDiagnostic(semanticErr, *cmd.SuccessAudit)
				next, err = record.CompleteMappingFailure(cmd.PermitID, decoded, *cmd.SuccessAudit, diagnostic)
			} else {
				next, err = record.CompleteSpecificationMapping(cmd.PermitID, decoded, *cmd.SuccessAudit)
			}
		} else {
			next, err = record.CompleteSpecification(cmd.PermitID, cmd.Proposal, cmd.Review, cmd.FailureCode, cmd.Diagnostic)
		}
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
	if err := validateAuthoringMappingOutcomes(next); err != nil {
		return definitionSpecificationTransition{}, err
	}
	result := workspaceevents.AuthoringSpecificationRequestResult{Request: request.Metadata(), Consumed: uint32(len(next.SpecificationRequests)), Remaining: uint32(workspace.DefinitionAuthoringSpecificationRequestLimit - len(next.SpecificationRequests))}
	if fresh {
		result.FreshPermitID = request.PermitID
	}
	transition := definitionSpecificationTransition{AuthoringSpecification: &result}
	if !reflect.DeepEqual(*record, next) {
		transition.Next = withAuthoringMutationRecord(w, next, candidate)
	}
	return transition, nil
}
