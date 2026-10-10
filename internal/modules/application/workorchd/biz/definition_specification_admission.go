package biz

import (
	"context"
	"fmt"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceports"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

// CompileWorkspaceDefinitionSpecification 从正式 owner 输入开始编译并准入，不创建 Run。
func (b *Biz) CompileWorkspaceDefinitionSpecification(ctx context.Context, workspaceID common.WorkspaceID, resources []workspace.GoalSpecificationResource, dimensions []workspace.GoalSpecificationDimension) (activationports.WorkspaceDefinitionAdmissionSnapshot, error) {
	if b == nil || b.EventHub() == nil || ctx == nil {
		return activationports.WorkspaceDefinitionAdmissionSnapshot{}, common.ErrInvalidArgument
	}
	input, err := activationports.PrepareDefinitionSpecification(ctx, b.EventHub(), workspaceID, resources, dimensions)
	if err != nil {
		return activationports.WorkspaceDefinitionAdmissionSnapshot{}, err
	}
	return b.CompileAndAdmitDefinitionSpecification(ctx, input)
}

// CompileAndAdmitDefinitionSpecification 将持久编译结果原样交给 Workspace 准入。
// 编译期间产生的 ledger revision 不可充当提交时的 Workspace CAS revision。
func (b *Biz) CompileAndAdmitDefinitionSpecification(ctx context.Context, input workspaceevents.BeginDefinitionSpecificationCommand) (activationports.WorkspaceDefinitionAdmissionSnapshot, error) {
	candidate, err := b.CompilePersistedDefinitionSpecification(ctx, input)
	if err != nil {
		return activationports.WorkspaceDefinitionAdmissionSnapshot{}, err
	}
	current, err := workspaceports.ReadWorkspaceRecord(ctx, b.EventHub(), candidate.WorkspaceID)
	if err != nil {
		return activationports.WorkspaceDefinitionAdmissionSnapshot{}, err
	}
	if workspace.WorkspaceConfigurationHash(current) != input.Normalized().ExpectedConfigurationHash {
		return activationports.WorkspaceDefinitionAdmissionSnapshot{}, fmt.Errorf("%w: workspace configuration changed during specification compilation", common.ErrConflict)
	}
	template, err := workspaceports.ReadWorkspaceTemplateRecord(ctx, b.EventHub(), current.TemplateID)
	if err != nil {
		return activationports.WorkspaceDefinitionAdmissionSnapshot{}, err
	}
	if workspace.WorkspaceTemplateSnapshotHash(template) != candidate.WorkspaceTemplateSnapshot.SnapshotHash {
		return activationports.WorkspaceDefinitionAdmissionSnapshot{}, fmt.Errorf("%w: template changed during specification compilation", common.ErrConflict)
	}
	return activationports.SubmitWorkspaceDefinitionAdmission(ctx, b.EventHub(), current, template, candidate)
}
