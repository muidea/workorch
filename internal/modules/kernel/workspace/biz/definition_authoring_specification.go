package workspace

import (
	"errors"
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
	if err := readWorkspaceAuthoringCommitted(w); err != nil {
		return definitionSpecificationTransition{}, err
	}
	var next workspace.DefinitionAuthoringRecord
	var request workspace.DefinitionAuthoringSpecificationRequest
	var fresh bool
	var err error
	if cmd := e.AuthoringSpecReserve; cmd != nil {
		if cmd.Validate() != nil || cmd.WorkspaceID != w.ID || cmd.InputHash != record.InputHash || o.template == nil || o.template.ID != record.Input.TemplateID {
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
			next, err = record.CompleteSpecificationMapping(cmd.PermitID, *cmd.Mapping, *cmd.SuccessAudit)
			var rejection *workspace.DefinitionAuthoringMappingRejection
			if errors.As(err, &rejection) {
				diagnostic := workspace.MappingFailureDiagnostic(rejection.Cause, *cmd.SuccessAudit)
				next, err = record.CompleteMappingFailure(cmd.PermitID, *cmd.Mapping, *cmd.SuccessAudit, diagnostic)
			} else if err == nil {
				for _, item := range next.SpecificationRequests {
					if item.PermitID != cmd.PermitID {
						continue
					}
					if item.Proposal != nil {
						if cmd.Proposal != nil && !reflect.DeepEqual(*item.Proposal, *cmd.Proposal) {
							return definitionSpecificationTransition{}, common.ErrConflict
						}
						// Applicable template policy is a separate current responsibility; the
						// mapping's common semantic rules have already been judged once.
						priorComplete := false
						for _, prior := range record.SpecificationRequests {
							if prior.PermitID == cmd.PermitID && (prior.Proposal != nil || prior.FailureCode != "") {
								priorComplete = true
							}
						}
						if !priorComplete {
							if semanticErr := validateAuthoringVerificationAgainstSealedTemplate(workspace.WorkspaceTemplateSnapshotHash(&derivation.Template), *derivation, *item.Proposal); semanticErr != nil {
								diagnostic := workspace.MappingFailureDiagnostic(semanticErr, *cmd.SuccessAudit)
								next, err = record.CompleteMappingFailure(cmd.PermitID, *cmd.Mapping, *cmd.SuccessAudit, diagnostic)
							}
						}
					}
					break
				}
			}
		} else if cmd.Review != nil && cmd.SuccessAudit != nil {
			next, err = record.CompleteReviewDocument(cmd.PermitID, *cmd.Review, *cmd.SuccessAudit)
		} else {
			if cmd.FailureCode == "invalid_proposal" {
				if err := admitAuthoringParserFailure(*record, cmd.PermitID, cmd.Diagnostic); err != nil {
					return definitionSpecificationTransition{}, err
				}
			}
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
	if err := next.ReadCommittedRecord(); err != nil {
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

// Parser failures arriving as new completions are checked against their exact
// original source once. Semantic failures must enter as the typed mapping so
// the owner computes their reason; consumers never re-adjudicate this outcome.
func admitAuthoringParserFailure(record workspace.DefinitionAuthoringRecord, permit string, diagnostic *workspace.DefinitionAuthoringDiagnostic) error {
	if diagnostic == nil || diagnostic.ProviderCode != "" || diagnostic.ProviderCause != nil || diagnostic.ProviderDocument == nil || diagnostic.ProviderWire == nil || !diagnostic.ProviderDocument.Observed || !diagnostic.ProviderDocument.Complete || !diagnostic.ProviderDocument.Saved || !reflect.DeepEqual(diagnostic.Payload, diagnostic.ProviderDocument.Content) {
		return common.ErrConflict
	}
	for _, request := range record.SpecificationRequests {
		if request.PermitID != permit {
			continue
		}
		for _, d := range record.DerivationRequests {
			if d.PermitID != request.DerivationPermitID {
				continue
			}
			_, parseErr := workspace.DecodeAuthoringMappingFor(d, diagnostic.ProviderDocument.Content)
			if parseErr == nil {
				return common.ErrConflict
			}
			expected := workspace.NewDefinitionAuthoringDiagnostic(parseErr.Error(), diagnostic.ProviderDocument.Content)
			document, wire := diagnostic.ProviderDocument.Clone(), diagnostic.ProviderWire.Clone()
			expected.ProviderDocument, expected.ProviderWire = &document, &wire
			if !reflect.DeepEqual(expected, *diagnostic) {
				return common.ErrConflict
			}
			return nil
		}
	}
	return common.ErrConflict
}
