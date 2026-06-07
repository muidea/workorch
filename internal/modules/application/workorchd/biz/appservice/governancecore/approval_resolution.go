package governancecore

import (
	"strings"

	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	governancecommon "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
)

func NormalizeApprovalBatchAction(action string) (string, error) {
	semantic := governancecommon.NormalizeApprovalAction(action, action, "")
	normalized := strings.TrimSpace(strings.ToLower(action))
	if normalized != "approve" && normalized != "reject" {
		normalized = semantic.Action
	}
	if normalized != "approve" && normalized != "reject" {
		return "", common.ErrInvalidArgument
	}
	return normalized, nil
}

func ValidateApprovalIDs(ids []string) error {
	if len(ids) == 0 {
		return common.ErrInvalidArgument
	}
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			return common.ErrInvalidArgument
		}
		if _, ok := seen[id]; ok {
			return common.ErrInvalidArgument
		}
		seen[id] = struct{}{}
	}
	return nil
}

func CloneApprovalMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func EnsureApprovalWorkspaceScope(runtimeWorkspaceID common.WorkspaceID, item *approvalmodel.Approval) error {
	if item == nil {
		return common.ErrNotFound
	}
	if runtimeWorkspaceID == "" || item.WorkspaceID == runtimeWorkspaceID {
		return nil
	}
	return common.ErrNotFound
}
