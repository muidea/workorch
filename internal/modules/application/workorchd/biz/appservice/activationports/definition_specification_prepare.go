package activationports

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceports"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

// PrepareDefinitionSpecification 从 owner 读取正式定义，构建未晋升的编译输入。
// 调用方不能自报 Goal/DoD revision、候选来源或模板快照。
func PrepareDefinitionSpecification(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, resources []workspace.GoalSpecificationResource, dimensions []workspace.GoalSpecificationDimension) (workspaceevents.BeginDefinitionSpecificationCommand, error) {
	if ctx == nil || hub == nil || workspaceID == "" {
		return workspaceevents.BeginDefinitionSpecificationCommand{}, common.ErrInvalidArgument
	}
	current, err := workspaceports.ReadWorkspaceRecord(ctx, hub, workspaceID)
	if err != nil {
		return workspaceevents.BeginDefinitionSpecificationCommand{}, err
	}
	goal, dod, err := selectWorkspaceExecutionDefinitions(ctx, hub, workspaceID)
	if err != nil {
		return workspaceevents.BeginDefinitionSpecificationCommand{}, err
	}
	template, err := workspaceports.ReadWorkspaceTemplateRecord(ctx, hub, current.TemplateID)
	if err != nil {
		return workspaceevents.BeginDefinitionSpecificationCommand{}, err
	}
	effective, err := workspace.WorkspaceForCurrentTemplate(current, template)
	if err != nil {
		return workspaceevents.BeginDefinitionSpecificationCommand{}, err
	}
	// Begin 不物化配置；配置必须与 owner 当前值一致，否则先走显式配置更新。
	if workspace.WorkspaceConfigurationHash(effective) != workspace.WorkspaceConfigurationHash(current) {
		return workspaceevents.BeginDefinitionSpecificationCommand{}, fmt.Errorf("%w: current template configuration requires materialization before compilation", common.ErrConflict)
	}
	revision := uint64(1)
	for _, archived := range current.DefinitionAdmissionCandidates {
		if archived.CandidateID != "workspace-definition" || archived.CandidateRevision == ^uint64(0) {
			return workspaceevents.BeginDefinitionSpecificationCommand{}, common.ErrConflict
		}
		if archived.CandidateRevision >= revision {
			revision = archived.CandidateRevision + 1
		}
	}
	baseRevision, baseHash := "", ""
	if current.StartupInput != nil {
		baseRevision, baseHash = current.StartupInput.DefinitionRevision, current.StartupInput.StartupInputHash
	}
	candidate, err := definitionAdmissionCandidateForIdentity(effective, goal, dod, template, "workspace-definition", revision, baseRevision, baseHash)
	if err != nil {
		return workspaceevents.BeginDefinitionSpecificationCommand{}, err
	}
	command := workspaceevents.BeginDefinitionSpecificationCommand{Candidate: candidate, Resources: resources, Dimensions: dimensions,
		ExpectedWorkspaceRevision: current.Revision, ExpectedConfigurationHash: workspace.WorkspaceConfigurationHash(current)}.Normalized()
	if err := command.Validate(); err != nil {
		return workspaceevents.BeginDefinitionSpecificationCommand{}, err
	}
	return command, nil
}
