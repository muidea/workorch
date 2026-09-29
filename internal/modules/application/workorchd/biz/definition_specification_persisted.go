package biz

import (
	"context"
	"fmt"
	"reflect"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/specifications"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

// CompilePersistedDefinitionSpecification 先封存输入和调用额度，再获取提案及六次独立审查。
// 重放只读取已保存结果；已授权但结果不明的请求阻塞，不重复调用 provider。
func (b *Biz) CompilePersistedDefinitionSpecification(ctx context.Context, input workspaceevents.BeginDefinitionSpecificationCommand) (workspace.DefinitionAdmissionCandidate, error) {
	sealed, err := b.BeginDefinitionSpecification(ctx, input)
	if err != nil {
		return workspace.DefinitionAdmissionCandidate{}, err
	}
	candidate := sealed.Snapshot.Clone()
	command, err := specifications.ProposalCommand(candidate, sealed.Resources, sealed.Dimensions)
	if err != nil {
		return workspace.DefinitionAdmissionCandidate{}, err
	}
	identity := workspace.DefinitionSpecificationRequestIdentity{
		WorkspaceID: candidate.WorkspaceID, CandidateID: candidate.CandidateID, CandidateRevision: candidate.CandidateRevision,
		CandidateHash: candidate.CandidateHash, CompilationInputHash: sealed.CompilationInputHash,
		Phase: workspace.DefinitionSpecificationProposal, LogicalRequestID: "proposal-v1",
	}
	completion, err := b.completePersistedSpecificationDocument(ctx, identity, command, candidate, nil, sealed)
	if err != nil {
		return workspace.DefinitionAdmissionCandidate{}, err
	}
	spec := completion.Proposal.Clone()
	ref := workspace.DefinitionSpecificationCompilationRef{InputCandidateHash: candidate.CandidateHash,
		CompilationInputHash: sealed.CompilationInputHash, ProposalPermitID: identity.PermitID(), ProposalHash: spec.ContentHash()}
	for index, kind := range specifications.ReviewKinds() {
		command, err := specifications.ReviewCommand(candidate, spec, kind)
		if err != nil {
			return workspace.DefinitionAdmissionCandidate{}, err
		}
		identity.Phase, identity.ReviewKind, identity.ProposalHash = workspace.DefinitionSpecificationCritic, kind, ref.ProposalHash
		identity.LogicalRequestID = "critic-v1/" + kind
		completion, err := b.completePersistedSpecificationDocument(ctx, identity, command, candidate, &spec, sealed)
		if err != nil {
			return workspace.DefinitionAdmissionCandidate{}, err
		}
		spec.Reviews = append(spec.Reviews, *completion.Review)
		ref.CriticPermitIDs[index] = identity.PermitID()
	}
	candidate.Specification, candidate.CompilationRef = &spec, &ref
	candidate = candidate.WithHash()
	if err := candidate.Validate(); err != nil {
		return workspace.DefinitionAdmissionCandidate{}, err
	}
	return candidate, nil
}

func (b *Biz) completePersistedSpecificationDocument(ctx context.Context, identity workspace.DefinitionSpecificationRequestIdentity, command llmevents.CompleteProviderCommand, candidate workspace.DefinitionAdmissionCandidate, proposal *workspace.GoalSpecification, sealed workspaceevents.BeginDefinitionSpecificationResult) (workspace.DefinitionSpecificationRequestCompletion, error) {
	reservation, err := b.reserveDefinitionSpecification(ctx, workspaceevents.ReserveDefinitionSpecificationRequestCommand{Identity: identity})
	if err != nil {
		return workspace.DefinitionSpecificationRequestCompletion{}, err
	}
	var completion workspace.DefinitionSpecificationRequestCompletion
	switch reservation.Disposition {
	case workspaceevents.DefinitionSpecificationAlreadyReserved:
		return completion, fmt.Errorf("%w: specification request outcome unresolved; refusing repeated provider call", common.ErrConflict)
	case workspaceevents.DefinitionSpecificationAlreadyCompleted:
		completion = reservation.Completed.Clone()
	case workspaceevents.DefinitionSpecificationPermitted:
		payload, callErr := b.completeDefinitionDocument(ctx, command)
		if callErr != nil {
			completion.FailureCode = "provider_failed"
		} else if identity.Phase == workspace.DefinitionSpecificationProposal {
			value, parseErr := specifications.ParseProposal(payload, candidate.GoalInput, candidate.DoDInputs)
			if parseErr != nil {
				completion.FailureCode = "invalid_proposal"
			} else {
				completion.Proposal = &value
			}
		} else {
			value, parseErr := specifications.ParseReview(payload, identity.ReviewKind, *proposal)
			if parseErr != nil {
				completion.FailureCode = "invalid_review"
			} else {
				completion.Review = &value
			}
		}
		completion = completion.Normalized()
		if validateSpecificationCompletionInput(completion, identity, candidate, proposal, sealed) != nil {
			completion = workspace.DefinitionSpecificationRequestCompletion{FailureCode: "invalid_completion"}
		}
		// context 取消时不另起无界写入；owner 中 reserved 仍计费，恢复不得重发。
		if err := b.persistDefinitionSpecificationCompletion(ctx, workspaceevents.CompleteDefinitionSpecificationRequestCommand{Identity: identity, PermitID: reservation.PermitID, Completion: completion}); err != nil {
			return workspace.DefinitionSpecificationRequestCompletion{}, err
		}
	default:
		return completion, common.ErrConflict
	}
	if err := validateSpecificationCompletionInput(completion, identity, candidate, proposal, sealed); err != nil {
		return workspace.DefinitionSpecificationRequestCompletion{}, err
	}
	if completion.FailureCode != "" {
		return workspace.DefinitionSpecificationRequestCompletion{}, fmt.Errorf("%w: specification request failed (%s)", common.ErrConflict, completion.FailureCode)
	}
	return completion, nil
}

func validateSpecificationCompletionInput(completion workspace.DefinitionSpecificationRequestCompletion, identity workspace.DefinitionSpecificationRequestIdentity, candidate workspace.DefinitionAdmissionCandidate, proposal *workspace.GoalSpecification, sealed workspaceevents.BeginDefinitionSpecificationResult) error {
	if err := completion.ValidateFor(identity, candidate, sealed.Resources, sealed.Dimensions, proposal); err != nil {
		return err
	}
	if completion.Proposal != nil {
		normalized := completion.Proposal.Normalized()
		if !reflect.DeepEqual(normalized.Resources, sealed.Resources) || !reflect.DeepEqual(normalized.Dimensions, sealed.Dimensions) {
			return fmt.Errorf("%w: proposal changed sealed compilation input", common.ErrConflict)
		}
	}
	return nil
}
