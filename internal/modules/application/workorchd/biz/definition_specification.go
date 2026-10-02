package biz

import (
	"context"
	"fmt"
	"reflect"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/specifications"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
)

// CompileDefinitionSpecification 获取提案及六次独立审查。
// 返回值仅是创作输入，仍由 Workspace owner 持久化、校验和晋升；
// 此用例不触发 Runtime 状态转换。
func (b *Biz) CompileDefinitionSpecification(ctx context.Context, candidate workspace.DefinitionAdmissionCandidate, resources []workspace.GoalSpecificationResource, dimensions []workspace.GoalSpecificationDimension) (workspace.DefinitionAdmissionCandidate, error) {
	if b == nil || b.EventHub() == nil || ctx == nil {
		return workspace.DefinitionAdmissionCandidate{}, common.ErrInvalidArgument
	}
	command, err := specifications.ProposalCommand(candidate, resources, dimensions)
	if err != nil {
		return workspace.DefinitionAdmissionCandidate{}, err
	}
	payload, err := b.completeDefinitionDocument(ctx, command)
	if err != nil {
		return workspace.DefinitionAdmissionCandidate{}, err
	}
	spec, err := specifications.ParseProposal(payload, candidate.GoalInput, candidate.DoDInputs)
	if err != nil {
		return workspace.DefinitionAdmissionCandidate{}, err
	}
	if !reflect.DeepEqual(spec.Resources, resources) || !reflect.DeepEqual(spec.Dimensions, dimensions) {
		return workspace.DefinitionAdmissionCandidate{}, fmt.Errorf("%w: specification changed resource or dimension facts", common.ErrConflict)
	}
	for _, kind := range specifications.ReviewKinds() {
		command, err := specifications.ReviewCommand(candidate, spec, kind)
		if err != nil {
			return workspace.DefinitionAdmissionCandidate{}, err
		}
		payload, err := b.completeDefinitionDocument(ctx, command)
		if err != nil {
			return workspace.DefinitionAdmissionCandidate{}, err
		}
		review, err := specifications.ParseReview(payload, kind, spec)
		if err != nil {
			return workspace.DefinitionAdmissionCandidate{}, err
		}
		spec.Reviews = append(spec.Reviews, review)
	}
	candidate = candidate.Clone()
	candidate.Specification = &spec
	candidate = candidate.WithHash()
	if err := candidate.Validate(); err != nil {
		return workspace.DefinitionAdmissionCandidate{}, err
	}
	return candidate, nil
}

// completeDefinitionDocument returns exact provider Content bytes; callers decide how to parse them.
func (b *Biz) completeDefinitionDocument(ctx context.Context, command llmevents.CompleteProviderCommand) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ev := magicevent.NewEventWithContext(llmevents.TopicLLMCommandCompleteProvider, ModuleID, llmevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(llmevents.Contracts(), llmevents.TopicLLMCommandCompleteProvider, eventcontract.LaneScope{WorkspaceID: command.Request.WorkspaceID, SubjectType: "workspace_specification"}))
	result := b.SendEvent(ev)
	if result == nil {
		return nil, fmt.Errorf("%w: missing specification provider result", common.ErrConflict)
	}
	if err := result.Error(); err != nil {
		return nil, err
	}
	data, err := result.Get()
	if err != nil {
		return nil, err
	}
	response, ok := data.(llmevents.CompleteProviderResult)
	if !ok {
		return nil, fmt.Errorf("%w: unexpected specification provider result", common.ErrConflict)
	}
	if err := response.ValidateFor(command); err != nil {
		return nil, err
	}
	return []byte(response.Completion.Content), nil
}
