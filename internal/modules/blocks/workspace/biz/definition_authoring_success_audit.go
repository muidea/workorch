package workspace

import (
	"context"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

// 受限 owner 查询，仅由显式审计读取适配器消费，不进入普通管理视图、projection 或 Runtime。
// 重载及重编译只使用已完整保存的原 document。
func (m *Biz) queryAuthoringSuccessAudit(ctx context.Context, c workspaceevents.QueryAuthoringSuccessAuditCommand) (workspaceevents.QueryAuthoringSuccessAuditResult, error) {
	if c.Validate() != nil || m == nil || m.workspaces == nil || m.manager == nil || ctx == nil {
		return workspaceevents.QueryAuthoringSuccessAuditResult{}, common.ErrInvalidArgument
	}
	m.manager.definitionMu.Lock()
	defer m.manager.definitionMu.Unlock()
	m.templateMutationMu.Lock()
	defer m.templateMutationMu.Unlock()
	bound, err := common.BindRuntimeWorkspaceContext(ctx, c.WorkspaceID)
	if err != nil {
		return workspaceevents.QueryAuthoringSuccessAuditResult{}, err
	}
	w, err := m.workspaces.Get(bound, c.WorkspaceID)
	if err != nil {
		return workspaceevents.QueryAuthoringSuccessAuditResult{}, err
	}
	if err := workspaceDeletionRejectsMutation(w); err != nil {
		return workspaceevents.QueryAuthoringSuccessAuditResult{}, err
	}
	if err := validateWorkspaceAuthoringMappingOutcomes(w); err != nil {
		return workspaceevents.QueryAuthoringSuccessAuditResult{}, err
	}
	if w.ID != c.WorkspaceID || w.DefinitionAuthoring == nil || w.DefinitionAuthoring.Validate() != nil || w.ValidateDefinitionAuthoringHistory() != nil {
		return workspaceevents.QueryAuthoringSuccessAuditResult{}, common.ErrConflict
	}
	for _, request := range w.DefinitionAuthoring.SpecificationRequests {
		if request.PermitID != c.PermitID {
			continue
		}
		if request.Proposal == nil || request.SuccessAudit == nil {
			return workspaceevents.QueryAuthoringSuccessAuditResult{}, common.ErrNotFound
		}
		r := workspaceevents.QueryAuthoringSuccessAuditResult{WorkspaceID: c.WorkspaceID, PermitID: c.PermitID, CanonicalHash: request.Proposal.ContentHash(), Audit: request.SuccessAudit.Clone()}
		if r.ValidateFor(c) != nil {
			return workspaceevents.QueryAuthoringSuccessAuditResult{}, common.ErrConflict
		}
		return r, nil
	}
	return workspaceevents.QueryAuthoringSuccessAuditResult{}, common.ErrNotFound
}
