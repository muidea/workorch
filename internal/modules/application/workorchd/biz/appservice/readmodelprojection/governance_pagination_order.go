package readmodelprojection

import (
	"sort"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
)

func OrderedApprovalsForGovernancePage(items []*approvalmodel.Approval) []*approvalmodel.Approval {
	ordered := append([]*approvalmodel.Approval(nil), items...)
	sort.SliceStable(ordered, func(i, j int) bool {
		left, right := ordered[i], ordered[j]
		if left == nil || right == nil {
			return right != nil
		}
		return governancePageCreatedIDBefore(left.CreatedAt, right.CreatedAt, string(left.ID), string(right.ID))
	})
	return ordered
}

func OrderedDecisionTicketsForGovernancePage(items []*decisionmodel.Ticket) []*decisionmodel.Ticket {
	ordered := append([]*decisionmodel.Ticket(nil), items...)
	sort.SliceStable(ordered, func(i, j int) bool {
		left, right := ordered[i], ordered[j]
		if left == nil || right == nil {
			return right != nil
		}
		return governancePageCreatedIDBefore(left.CreatedAt, right.CreatedAt, string(left.ID), string(right.ID))
	})
	return ordered
}
