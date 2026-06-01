package appservice

import (
	"sort"
	"strings"

	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
)

func pageDecisionTickets(items []*decisionmodel.Ticket, query DecisionQuery) ([]*decisionmodel.Ticket, int, string, string, bool) {
	limit := query.Limit
	if limit <= 0 {
		limit = readModelDecisionPageLimit
	}
	if limit > readModelMaxDecisionPageLimit {
		limit = readModelMaxDecisionPageLimit
	}
	items = orderedDecisionTicketsForGovernancePage(items)
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	started := cursorAfterID == ""
	selected := make([]*decisionmodel.Ticket, 0, min(limit, len(items)))
	hasMore := false
	for _, item := range items {
		if item == nil {
			continue
		}
		if !started {
			if string(item.ID) == cursorAfterID {
				started = true
			}
			continue
		}
		if len(selected) >= limit {
			hasMore = true
			break
		}
		selected = append(selected, item)
	}
	nextCursor := ""
	if len(selected) > 0 {
		nextCursor = string(selected[len(selected)-1].ID)
	}
	return selected, limit, cursorAfterID, nextCursor, hasMore
}

func orderedDecisionTicketsForGovernancePage(items []*decisionmodel.Ticket) []*decisionmodel.Ticket {
	ordered := append([]*decisionmodel.Ticket(nil), items...)
	sort.SliceStable(ordered, func(i, j int) bool {
		left, right := ordered[i], ordered[j]
		if left == nil || right == nil {
			return right != nil
		}
		leftCreated := strings.TrimSpace(left.CreatedAt)
		rightCreated := strings.TrimSpace(right.CreatedAt)
		if leftCreated != "" || rightCreated != "" {
			if leftCreated != rightCreated {
				return leftCreated > rightCreated
			}
		}
		return string(left.ID) > string(right.ID)
	})
	return ordered
}

func projectDecisionView(item Decision) Decision {
	return Decision{
		ID:             item.ID,
		WorkspaceID:    item.WorkspaceID,
		RunID:          item.RunID,
		StepID:         item.StepID,
		Reason:         item.Reason,
		DecisionKind:   item.DecisionKind,
		Question:       item.Question,
		State:          item.State,
		ResolutionID:   item.ResolutionID,
		ResolutionType: item.ResolutionType,
		AutoGovernance: item.AutoGovernance,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	}
}

func pageApprovals(items []*approvalmodel.Approval, query ApprovalQuery) ([]*approvalmodel.Approval, int, string, string, bool) {
	limit := query.Limit
	if limit <= 0 {
		limit = readModelApprovalPageLimit
	}
	if limit > readModelMaxApprovalPageLimit {
		limit = readModelMaxApprovalPageLimit
	}
	items = orderedApprovalsForGovernancePage(items)
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	started := cursorAfterID == ""
	selected := make([]*approvalmodel.Approval, 0, min(limit, len(items)))
	hasMore := false
	for _, item := range items {
		if item == nil {
			continue
		}
		if !started {
			if string(item.ID) == cursorAfterID {
				started = true
			}
			continue
		}
		if len(selected) >= limit {
			hasMore = true
			break
		}
		selected = append(selected, item)
	}
	nextCursor := ""
	if len(selected) > 0 {
		nextCursor = string(selected[len(selected)-1].ID)
	}
	return selected, limit, cursorAfterID, nextCursor, hasMore
}

func orderedApprovalsForGovernancePage(items []*approvalmodel.Approval) []*approvalmodel.Approval {
	ordered := append([]*approvalmodel.Approval(nil), items...)
	sort.SliceStable(ordered, func(i, j int) bool {
		left, right := ordered[i], ordered[j]
		if left == nil || right == nil {
			return right != nil
		}
		leftCreated := strings.TrimSpace(left.CreatedAt)
		rightCreated := strings.TrimSpace(right.CreatedAt)
		if leftCreated != "" || rightCreated != "" {
			if leftCreated != rightCreated {
				return leftCreated > rightCreated
			}
		}
		return string(left.ID) > string(right.ID)
	})
	return ordered
}

func projectApprovalView(item Approval) Approval {
	return Approval{
		ID:             item.ID,
		WorkspaceID:    item.WorkspaceID,
		RunID:          item.RunID,
		StepID:         item.StepID,
		Reason:         item.Reason,
		Action:         item.Action,
		State:          item.State,
		AutoGovernance: item.AutoGovernance,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	}
}
