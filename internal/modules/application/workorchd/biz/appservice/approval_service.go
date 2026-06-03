package appservice

import (
	"context"
	"strings"

	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	governancecommon "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type Approval struct {
	ID             string                 `json:"id"`
	WorkspaceID    string                 `json:"workspaceId"`
	RunID          string                 `json:"runId"`
	StepID         string                 `json:"stepId,omitempty"`
	Reason         string                 `json:"reason"`
	Action         string                 `json:"action,omitempty"`
	Context        map[string]any         `json:"context,omitempty"`
	State          string                 `json:"state"`
	AutoGovernance *AutoGovernanceSummary `json:"autoGovernance,omitempty"`
	CreatedAt      string                 `json:"createdAt,omitempty"`
	UpdatedAt      string                 `json:"updatedAt,omitempty"`
}

type ApprovalQuery struct {
	WorkspaceID   string
	RunID         string
	CursorAfterID string
	Limit         int
}

type ApprovalPage struct {
	WorkspaceID       string     `json:"workspaceId,omitempty"`
	RunID             string     `json:"runId,omitempty"`
	Approvals         []Approval `json:"approvals,omitempty"`
	Limit             int        `json:"limit"`
	CursorAfterID     string     `json:"cursorAfterId,omitempty"`
	NextCursorAfterID string     `json:"nextCursorAfterId,omitempty"`
	HasMore           bool       `json:"hasMore"`
}

type BatchResolveApprovalCommand struct {
	ApprovalIDs []string `json:"approvalIds"`
	Action      string   `json:"action"`
}

func ReadApprovalWithSession(ctx context.Context, session *RuntimeSession, approvalID string) (ret Approval, err error) {
	hub, err := approvalDependenciesFromSession(session)
	if err != nil {
		return Approval{}, err
	}
	traces := autoGovernanceTraceReaderFromSession(session)
	defer session.CloseOnReturn(ctx, &err)

	item, err := readApprovalByOwnerCommand(ctx, hub, common.ApprovalID(approvalID))
	if err != nil {
		return Approval{}, err
	}
	if err := ensureApprovalInRuntimeWorkspace(ctx, item); err != nil {
		return Approval{}, err
	}
	autoGovernance, err := latestAutoGovernanceSummary(ctx, traces, item.WorkspaceID, item.RunID, "approval", string(item.ID))
	if err != nil {
		return Approval{}, err
	}
	return mapApproval(item, autoGovernance), nil
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
	hub, err := approvalDependenciesFromSession(session)
	if err != nil {
		return nil, err
	}
	traces := autoGovernanceTraceReaderFromSession(session)
	defer session.CloseOnReturn(ctx, &err)

	items, err := approvalsForQuery(ctx, hub, workspaceID, runID)
	if err != nil {
		return nil, err
	}
	ret = make([]Approval, 0, len(items))
	for _, item := range items {
		autoGovernance, autoErr := latestAutoGovernanceSummary(ctx, traces, item.WorkspaceID, item.RunID, "approval", string(item.ID))
		if autoErr != nil {
			return nil, autoErr
		}
		ret = append(ret, mapApproval(item, autoGovernance))
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

func ListApprovalPageWithSession(ctx context.Context, session *RuntimeSession, query ApprovalQuery) (ret ApprovalPage, err error) {
	hub, err := approvalDependenciesFromSession(session)
	if err != nil {
		return ApprovalPage{}, err
	}
	traces := autoGovernanceTraceReaderFromSession(session)
	defer session.CloseOnReturn(ctx, &err)

	filtered, err := approvalsForQuery(ctx, hub, query.WorkspaceID, query.RunID)
	if err != nil {
		return ApprovalPage{}, err
	}
	pageApprovals, limit, cursorAfterID, nextCursor, hasMore := pageApprovals(filtered, query)
	approvals := make([]Approval, 0, len(pageApprovals))
	for _, item := range pageApprovals {
		summary, err := latestAutoGovernanceSummary(ctx, traces, item.WorkspaceID, item.RunID, "approval", string(item.ID))
		if err != nil {
			return ApprovalPage{}, err
		}
		approvals = append(approvals, projectApprovalView(mapApproval(item, summary)))
	}
	return ApprovalPage{
		WorkspaceID:       strings.TrimSpace(query.WorkspaceID),
		RunID:             strings.TrimSpace(query.RunID),
		Approvals:         approvals,
		Limit:             limit,
		CursorAfterID:     cursorAfterID,
		NextCursorAfterID: nextCursor,
		HasMore:           hasMore,
	}, nil
}

func ListApprovalPageWithBackend(ctx context.Context, backend string, dataRoot string, query ApprovalQuery) (ApprovalPage, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return ApprovalPage{}, err
	}
	return ListApprovalPageWithSession(ctx, session, query)
}

func ListApprovalsFromEnv(ctx context.Context, workspaceID string, runID string) ([]Approval, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return nil, err
	}
	return ListApprovalsWithSession(ctx, session, workspaceID, runID)
}

func ApproveApprovalWithSession(ctx context.Context, session *RuntimeSession, approvalID string) (ret Approval, err error) {
	hub, err := approvalDependenciesFromSession(session)
	if err != nil {
		return Approval{}, err
	}
	traces := autoGovernanceTraceReaderFromSession(session)
	defer session.CloseOnReturn(ctx, &err)

	id := common.ApprovalID(approvalID)
	if common.RuntimeWorkspaceIDFromContext(ctx) != "" {
		existing, err := readApprovalByOwnerCommand(ctx, hub, id)
		if err != nil {
			return Approval{}, err
		}
		if err := ensureApprovalInRuntimeWorkspace(ctx, existing); err != nil {
			return Approval{}, err
		}
	}
	item, err := resolveApprovalByEventHub(ctx, hub, events.TopicApprovalCommandEnsureApproved, id)
	if err != nil {
		return Approval{}, err
	}
	if err := ensureApprovalInRuntimeWorkspace(ctx, item); err != nil {
		return Approval{}, err
	}
	autoGovernance, err := latestAutoGovernanceSummary(ctx, traces, item.WorkspaceID, item.RunID, "approval", string(item.ID))
	if err != nil {
		return Approval{}, err
	}
	return mapApproval(item, autoGovernance), nil
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
	hub, err := approvalDependenciesFromSession(session)
	if err != nil {
		return Approval{}, err
	}
	traces := autoGovernanceTraceReaderFromSession(session)
	defer session.CloseOnReturn(ctx, &err)

	id := common.ApprovalID(approvalID)
	if common.RuntimeWorkspaceIDFromContext(ctx) != "" {
		existing, err := readApprovalByOwnerCommand(ctx, hub, id)
		if err != nil {
			return Approval{}, err
		}
		if err := ensureApprovalInRuntimeWorkspace(ctx, existing); err != nil {
			return Approval{}, err
		}
	}
	item, err := resolveApprovalByEventHub(ctx, hub, events.TopicApprovalCommandEnsureRejected, id)
	if err != nil {
		return Approval{}, err
	}
	if err := ensureApprovalInRuntimeWorkspace(ctx, item); err != nil {
		return Approval{}, err
	}
	autoGovernance, err := latestAutoGovernanceSummary(ctx, traces, item.WorkspaceID, item.RunID, "approval", string(item.ID))
	if err != nil {
		return Approval{}, err
	}
	return mapApproval(item, autoGovernance), nil
}

func BatchResolveApprovalsWithSession(ctx context.Context, session *RuntimeSession, cmd BatchResolveApprovalCommand) (ret []Approval, err error) {
	hub, err := approvalDependenciesFromSession(session)
	if err != nil {
		return nil, err
	}
	traces := autoGovernanceTraceReaderFromSession(session)
	defer session.CloseOnReturn(ctx, &err)

	if len(cmd.ApprovalIDs) == 0 {
		return nil, common.ErrInvalidArgument
	}
	semantic := governancecommon.NormalizeApprovalAction(cmd.Action, cmd.Action, "")
	action := strings.TrimSpace(strings.ToLower(cmd.Action))
	if action != "approve" && action != "reject" {
		action = semantic.Action
	}
	if action != "approve" && action != "reject" {
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
	}
	type approvalRead struct {
		item *approvalmodel.Approval
		err  error
	}
	reads := make([]approvalRead, len(cmd.ApprovalIDs))
	for i, approvalID := range cmd.ApprovalIDs {
		reads[i].item, reads[i].err = readApprovalByOwnerCommand(ctx, hub, common.ApprovalID(approvalID))
	}
	for _, r := range reads {
		if r.err != nil {
			return nil, r.err
		}
		if err := ensureApprovalInRuntimeWorkspace(ctx, r.item); err != nil {
			return nil, err
		}
		if r.item.State != common.ApprovalStatePending {
			return nil, common.ErrConflict
		}
	}

	ret = make([]Approval, 0, len(cmd.ApprovalIDs))
	for _, approvalID := range cmd.ApprovalIDs {
		id := common.ApprovalID(approvalID)
		var (
			item *approvalmodel.Approval
			err  error
		)
		if action == "approve" {
			item, err = resolveApprovalByEventHub(ctx, hub, events.TopicApprovalCommandEnsureApproved, id)
		} else {
			item, err = resolveApprovalByEventHub(ctx, hub, events.TopicApprovalCommandEnsureRejected, id)
		}
		if err != nil {
			return nil, err
		}
		if err := ensureApprovalInRuntimeWorkspace(ctx, item); err != nil {
			return nil, err
		}
		autoGovernance, autoErr := latestAutoGovernanceSummary(ctx, traces, item.WorkspaceID, item.RunID, "approval", string(item.ID))
		if autoErr != nil {
			return nil, autoErr
		}
		ret = append(ret, mapApproval(item, autoGovernance))
	}
	return ret, nil
}

func ensureApprovalInRuntimeWorkspace(ctx context.Context, item *approvalmodel.Approval) error {
	if item == nil {
		return common.ErrNotFound
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if workspaceID == "" || item.WorkspaceID == workspaceID {
		return nil
	}
	return common.ErrNotFound
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

func approvalDependenciesFromSession(session *RuntimeSession) (events.Hub, error) {
	exports, err := session.Exports()
	if err != nil {
		return nil, err
	}
	if exports.Infrastructure.EventHub == nil {
		return nil, common.ErrInvalidArgument
	}
	return exports.Infrastructure.EventHub, nil
}

func approvalsForQuery(ctx context.Context, hub events.Hub, workspaceID string, runID string) ([]*approvalmodel.Approval, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	runID = strings.TrimSpace(runID)
	var (
		items []*approvalmodel.Approval
		err   error
	)
	switch {
	case runID != "":
		items, err = listApprovalsByRunOwnerCommand(ctx, hub, common.RunID(runID))
	case workspaceID != "":
		items, err = listApprovalsByWorkspaceOwnerCommand(ctx, hub, common.WorkspaceID(workspaceID))
	default:
		items, err = listApprovalsByOwnerCommand(ctx, hub)
	}
	if err != nil {
		return nil, err
	}
	filtered := make([]*approvalmodel.Approval, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		if workspaceID != "" && string(item.WorkspaceID) != workspaceID {
			continue
		}
		if runID != "" && string(item.RunID) != runID {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered, nil
}

func resolveApprovalByEventHub(ctx context.Context, hub events.Hub, topic string, approvalID common.ApprovalID) (*approvalmodel.Approval, error) {
	response, err := publishOwnerCommandWithResponse(ctx, hub, topic, "approval", string(approvalID), "", "", map[string]any{
		events.PayloadKeyApprovalID: string(approvalID),
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyApproval].(*approvalmodel.Approval)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func mapApproval(item *approvalmodel.Approval, autoGovernance *AutoGovernanceSummary) Approval {
	if item == nil {
		return Approval{}
	}
	return Approval{
		ID:             string(item.ID),
		WorkspaceID:    string(item.WorkspaceID),
		RunID:          string(item.RunID),
		StepID:         string(item.StepID),
		Reason:         item.Reason,
		Action:         item.Action,
		Context:        cloneApprovalMap(item.Context),
		State:          string(item.State),
		AutoGovernance: autoGovernance,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	}
}

func cloneApprovalMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}
