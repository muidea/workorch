package appservice

import (
	"context"

	approvalmodule "github.com/muidea/workorch/internal/modules/blocks/approval"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type Approval struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspaceId"`
	RunID       string `json:"runId"`
	StepID      string `json:"stepId,omitempty"`
	Reason      string `json:"reason"`
	State       string `json:"state"`
}

type BatchResolveApprovalCommand struct {
	ApprovalIDs []string `json:"approvalIds"`
	Action      string   `json:"action"`
}

func ReadApprovalWithSession(ctx context.Context, session *RuntimeSession, approvalID string) (ret Approval, err error) {
	_, repo, err := approvalDependenciesFromSession(session)
	if err != nil {
		return Approval{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	item, err := repo.Get(ctx, common.ApprovalID(approvalID))
	if err != nil {
		return Approval{}, err
	}
	return mapApproval(item), nil
}

func ReadApprovalWithBackend(ctx context.Context, backend string, dataRoot string, approvalID string) (Approval, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return Approval{}, err
	}
	return ReadApprovalWithSession(ctx, session, approvalID)
}

func ReadApprovalFromEnv(ctx context.Context, approvalID string) (Approval, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return Approval{}, err
	}
	return ReadApprovalWithSession(ctx, session, approvalID)
}

func ListApprovalsWithSession(ctx context.Context, session *RuntimeSession, workspaceID string, runID string) (ret []Approval, err error) {
	_, repo, err := approvalDependenciesFromSession(session)
	if err != nil {
		return nil, err
	}
	defer session.CloseOnReturn(ctx, &err)

	items, err := repo.List(ctx)
	if err != nil {
		return nil, err
	}
	ret = make([]Approval, 0, len(items))
	for _, item := range items {
		if workspaceID != "" && string(item.WorkspaceID) != workspaceID {
			continue
		}
		if runID != "" && string(item.RunID) != runID {
			continue
		}
		ret = append(ret, mapApproval(item))
	}
	return ret, nil
}

func ListApprovalsWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string, runID string) ([]Approval, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return nil, err
	}
	return ListApprovalsWithSession(ctx, session, workspaceID, runID)
}

func ListApprovalsFromEnv(ctx context.Context, workspaceID string, runID string) ([]Approval, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return nil, err
	}
	return ListApprovalsWithSession(ctx, session, workspaceID, runID)
}

func ApproveApprovalWithSession(ctx context.Context, session *RuntimeSession, approvalID string) (ret Approval, err error) {
	service, repo, err := approvalDependenciesFromSession(session)
	if err != nil {
		return Approval{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	id := common.ApprovalID(approvalID)
	if err := service.EnsureApproved(ctx, approvalmodule.ResolveApprovalCommand{ApprovalID: id}); err != nil {
		return Approval{}, err
	}

	item, err := repo.Get(ctx, id)
	if err != nil {
		return Approval{}, err
	}
	return mapApproval(item), nil
}

func ApproveApprovalWithBackend(ctx context.Context, backend string, dataRoot string, approvalID string) (Approval, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return Approval{}, err
	}
	return ApproveApprovalWithSession(ctx, session, approvalID)
}

func ApproveApprovalFromEnv(ctx context.Context, approvalID string) (Approval, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return Approval{}, err
	}
	return ApproveApprovalWithSession(ctx, session, approvalID)
}

func RejectApprovalWithSession(ctx context.Context, session *RuntimeSession, approvalID string) (ret Approval, err error) {
	service, repo, err := approvalDependenciesFromSession(session)
	if err != nil {
		return Approval{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	id := common.ApprovalID(approvalID)
	if err := service.EnsureRejected(ctx, approvalmodule.ResolveApprovalCommand{ApprovalID: id}); err != nil {
		return Approval{}, err
	}

	item, err := repo.Get(ctx, id)
	if err != nil {
		return Approval{}, err
	}
	return mapApproval(item), nil
}

func BatchResolveApprovalsWithSession(ctx context.Context, session *RuntimeSession, cmd BatchResolveApprovalCommand) (ret []Approval, err error) {
	service, repo, err := approvalDependenciesFromSession(session)
	if err != nil {
		return nil, err
	}
	defer session.CloseOnReturn(ctx, &err)

	if len(cmd.ApprovalIDs) == 0 || (cmd.Action != "approve" && cmd.Action != "reject") {
		return nil, common.ErrInvalidArgument
	}
	seen := make(map[string]struct{}, len(cmd.ApprovalIDs))
	for _, approvalID := range cmd.ApprovalIDs {
		if approvalID == "" {
			return nil, common.ErrInvalidArgument
		}
		if _, ok := seen[approvalID]; ok {
			return nil, common.ErrInvalidArgument
		}
		seen[approvalID] = struct{}{}
		item, err := repo.Get(ctx, common.ApprovalID(approvalID))
		if err != nil {
			return nil, err
		}
		if item.State != common.ApprovalStatePending {
			return nil, common.ErrConflict
		}
	}

	ret = make([]Approval, 0, len(cmd.ApprovalIDs))
	for _, approvalID := range cmd.ApprovalIDs {
		id := common.ApprovalID(approvalID)
		var err error
		if cmd.Action == "approve" {
			err = service.EnsureApproved(ctx, approvalmodule.ResolveApprovalCommand{ApprovalID: id})
		} else {
			err = service.EnsureRejected(ctx, approvalmodule.ResolveApprovalCommand{ApprovalID: id})
		}
		if err != nil {
			return nil, err
		}
		item, err := repo.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		ret = append(ret, mapApproval(item))
	}
	return ret, nil
}

func RejectApprovalWithBackend(ctx context.Context, backend string, dataRoot string, approvalID string) (Approval, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return Approval{}, err
	}
	return RejectApprovalWithSession(ctx, session, approvalID)
}

func BatchResolveApprovalsWithBackend(ctx context.Context, backend string, dataRoot string, cmd BatchResolveApprovalCommand) ([]Approval, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return nil, err
	}
	return BatchResolveApprovalsWithSession(ctx, session, cmd)
}

func RejectApprovalFromEnv(ctx context.Context, approvalID string) (Approval, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return Approval{}, err
	}
	return RejectApprovalWithSession(ctx, session, approvalID)
}

func BatchResolveApprovalsFromEnv(ctx context.Context, cmd BatchResolveApprovalCommand) ([]Approval, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return nil, err
	}
	return BatchResolveApprovalsWithSession(ctx, session, cmd)
}

func approvalDependenciesFromSession(session *RuntimeSession) (approvalmodule.Service, repository.ApprovalRepository, error) {
	exports, err := session.Exports()
	if err != nil {
		return nil, nil, err
	}
	if exports.Services.ApprovalService == nil || exports.Repositories.Approvals == nil {
		return nil, nil, common.ErrInvalidArgument
	}
	return exports.Services.ApprovalService, exports.Repositories.Approvals, nil
}

func mapApproval(item *approvalmodel.Approval) Approval {
	if item == nil {
		return Approval{}
	}
	return Approval{
		ID:          string(item.ID),
		WorkspaceID: string(item.WorkspaceID),
		RunID:       string(item.RunID),
		StepID:      string(item.StepID),
		Reason:      item.Reason,
		State:       string(item.State),
	}
}
