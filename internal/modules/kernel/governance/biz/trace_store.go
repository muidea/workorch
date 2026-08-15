package governance

import (
	"context"
	"strings"

	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func (s *Service) countResolvedTicketTraces(ctx context.Context, runID common.RunID) int {
	if s == nil || s.traces == nil || runID == "" {
		return 0
	}
	list, err := s.traces.List(ctx, workorchquerycontract.AutoGovernanceTraceQuery{RunID: runID, ItemType: "ticket"})
	if err != nil {
		return 0
	}
	count := 0
	for _, item := range list {
		if item == nil {
			continue
		}
		if strings.EqualFold(item.Status, "resolved") && strings.EqualFold(item.Action, "answer") {
			count++
		}
	}
	return count
}

func (s *Service) countResolvedApprovalTraces(ctx context.Context, runID common.RunID) int {
	if s == nil || s.traces == nil || runID == "" {
		return 0
	}
	list, err := s.traces.List(ctx, workorchquerycontract.AutoGovernanceTraceQuery{RunID: runID, ItemType: "approval"})
	if err != nil {
		return 0
	}
	count := 0
	for _, item := range list {
		if item == nil {
			continue
		}
		if strings.EqualFold(item.Status, "resolved") && strings.EqualFold(item.Action, "approve") {
			count++
		}
	}
	return count
}

func (s *Service) blockedBySharedBudgetPolicy(ctx context.Context, governanceCfg workorchconfig.GovernanceConfig, runID common.RunID, itemType string, currentItemID string, promptKey string) (string, bool) {
	traces, err := s.loadTracesByRunAndType(ctx, runID, itemType)
	if err != nil {
		return "", false
	}
	maxConsecutiveFailures := governanceCfg.Budget.MaxConsecutiveFailures
	if maxConsecutiveFailures > 0 {
		consecutive := 0
		for i := len(traces) - 1; i >= 0; i-- {
			item := traces[i]
			if item == nil {
				continue
			}
			if strings.TrimSpace(item.ItemID) == strings.TrimSpace(currentItemID) {
				continue
			}
			if strings.EqualFold(item.Status, "resolved") {
				break
			}
			if strings.EqualFold(item.Status, "escalated") || strings.EqualFold(item.Status, "blocked") {
				consecutive++
				continue
			}
		}
		if consecutive >= maxConsecutiveFailures {
			return "consecutive_failures_exhausted", true
		}
	}
	maxRepeatedPrompt := governanceCfg.Budget.MaxRepeatedSamePrompt
	if maxRepeatedPrompt > 0 && promptKey != "" {
		repeated := 0
		for _, item := range traces {
			if item == nil {
				continue
			}
			if strings.TrimSpace(item.ItemID) == strings.TrimSpace(currentItemID) {
				continue
			}
			if strings.TrimSpace(item.PromptFingerprint) == promptKey {
				repeated++
			}
		}
		if repeated >= maxRepeatedPrompt {
			return "repeated_prompt_exhausted", true
		}
	}
	return "", false
}

func (s *Service) loadTracesByRunAndType(ctx context.Context, runID common.RunID, itemType string) ([]*governancemodel.AutoGovernanceTrace, error) {
	if s == nil || s.traces == nil || runID == "" || strings.TrimSpace(itemType) == "" {
		return nil, common.ErrInvalidArgument
	}
	return s.traces.List(ctx, workorchquerycontract.AutoGovernanceTraceQuery{
		RunID:    runID,
		ItemType: itemType,
	})
}

func (s *Service) hasAutoResolvedTrace(ctx context.Context, runID common.RunID, itemType string, itemID string) bool {
	if s == nil || s.traces == nil || runID == "" || itemType == "" || itemID == "" {
		return false
	}
	list, err := s.traces.List(ctx, workorchquerycontract.AutoGovernanceTraceQuery{
		RunID:    runID,
		ItemType: itemType,
		ItemID:   itemID,
		Status:   "resolved",
	})
	if err != nil {
		return false
	}
	for _, item := range list {
		if item == nil {
			continue
		}
		if strings.EqualFold(item.Action, "approve") || strings.EqualFold(item.Action, "answer") {
			return true
		}
	}
	return false
}

func (s *Service) hasClosedTrace(ctx context.Context, itemType string, itemID string) bool {
	if s == nil || s.traces == nil || itemType == "" || itemID == "" {
		return false
	}
	list, err := s.traces.List(ctx, workorchquerycontract.AutoGovernanceTraceQuery{
		ItemType: itemType,
		ItemID:   itemID,
	})
	if err != nil {
		return false
	}
	for _, item := range list {
		if item == nil {
			continue
		}
		if strings.EqualFold(item.Status, "resolved") || strings.EqualFold(item.Status, "blocked") {
			return true
		}
	}
	return false
}
