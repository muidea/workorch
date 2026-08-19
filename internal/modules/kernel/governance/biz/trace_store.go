package governance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

const (
	autoGovernanceActor           = "auto_governance"
	autoGovernanceStatusEvaluated = "evaluated"
	autoGovernanceStatusResolving = "resolving"
	autoGovernanceStatusResolved  = "resolved"
	autoGovernanceStatusEscalated = "escalated"
	autoGovernanceStatusBlocked   = "blocked"
)

func autoGovernanceTraceID(workspaceID common.WorkspaceID, itemType string, itemID string) common.AutoGovernanceTraceID {
	workspace := strings.TrimSpace(string(workspaceID))
	kind := strings.ToLower(strings.TrimSpace(itemType))
	item := strings.TrimSpace(itemID)
	if workspace == "" || kind == "" || item == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(workspace + "\x00" + kind + "\x00" + item))
	return common.AutoGovernanceTraceID("auto-governance/" + hex.EncodeToString(sum[:]))
}

func autoGovernanceApprovalTraceID(workspaceID common.WorkspaceID, runID common.RunID, approvalID common.ApprovalID) common.AutoGovernanceTraceID {
	workspace := strings.TrimSpace(string(workspaceID))
	run := strings.TrimSpace(string(runID))
	approval := strings.TrimSpace(string(approvalID))
	if workspace == "" || run == "" || approval == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(workspace + "\x00" + run + "\x00approval\x00" + approval))
	return common.AutoGovernanceTraceID("auto-governance/" + hex.EncodeToString(sum[:]))
}

func autoGovernanceResolutionID(workspaceID common.WorkspaceID, ticketID common.TicketID) common.ResolutionID {
	workspace := strings.TrimSpace(string(workspaceID))
	ticket := strings.TrimSpace(string(ticketID))
	if workspace == "" || ticket == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(workspace + "\x00ticket\x00" + ticket))
	return common.ResolutionID("auto-governance-resolution/" + hex.EncodeToString(sum[:]))
}

func (s *Service) countResolvedTicketTraces(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (int, error) {
	return s.countResolvedTraces(ctx, workspaceID, runID, "ticket", "answer")
}

func (s *Service) countResolvedApprovalTraces(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (int, error) {
	return s.countResolvedTraces(ctx, workspaceID, runID, "approval", "approve")
}

func (s *Service) countResolvedTraces(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, itemType string, action string) (int, error) {
	if s == nil || s.traces == nil || workspaceID == "" || runID == "" || strings.TrimSpace(itemType) == "" || strings.TrimSpace(action) == "" {
		return 0, common.ErrInvalidArgument
	}
	list, err := s.traces.List(ctx, workorchquerycontract.AutoGovernanceTraceQuery{
		WorkspaceID: workspaceID,
		RunID:       runID,
		ItemType:    itemType,
	})
	if err != nil {
		return 0, err
	}
	count := 0
	for _, item := range list {
		if item == nil {
			continue
		}
		if strings.EqualFold(item.Status, autoGovernanceStatusResolved) && strings.EqualFold(item.Action, action) {
			count++
		}
	}
	return count, nil
}

func (s *Service) blockedBySharedBudgetPolicy(ctx context.Context, governanceCfg workorchconfig.GovernanceConfig, workspaceID common.WorkspaceID, runID common.RunID, itemType string, currentItemID string, promptKey string) (string, bool, error) {
	traces, err := s.loadTracesByRunAndType(ctx, workspaceID, runID, itemType)
	if err != nil {
		return "", false, err
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
			if strings.EqualFold(item.Status, autoGovernanceStatusResolved) {
				break
			}
			if strings.EqualFold(item.Status, autoGovernanceStatusEscalated) || strings.EqualFold(item.Status, autoGovernanceStatusBlocked) {
				consecutive++
				continue
			}
		}
		if consecutive >= maxConsecutiveFailures {
			return "consecutive_failures_exhausted", true, nil
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
			return "repeated_prompt_exhausted", true, nil
		}
	}
	return "", false, nil
}

func (s *Service) loadTracesByRunAndType(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, itemType string) ([]*governancemodel.AutoGovernanceTrace, error) {
	if s == nil || s.traces == nil || workspaceID == "" || runID == "" || strings.TrimSpace(itemType) == "" {
		return nil, common.ErrInvalidArgument
	}
	return s.traces.List(ctx, workorchquerycontract.AutoGovernanceTraceQuery{
		WorkspaceID: workspaceID,
		RunID:       runID,
		ItemType:    itemType,
	})
}

func (s *Service) hasClosedTrace(ctx context.Context, workspaceID common.WorkspaceID, itemType string, itemID string) (bool, error) {
	if s == nil || s.traces == nil || workspaceID == "" || strings.TrimSpace(itemType) == "" || strings.TrimSpace(itemID) == "" {
		return false, common.ErrInvalidArgument
	}
	list, err := s.traces.List(ctx, workorchquerycontract.AutoGovernanceTraceQuery{
		WorkspaceID: workspaceID,
		ItemType:    itemType,
		ItemID:      itemID,
	})
	if err != nil {
		return false, err
	}
	for _, item := range list {
		if item == nil {
			continue
		}
		if strings.EqualFold(item.Status, autoGovernanceStatusResolved) || strings.EqualFold(item.Status, autoGovernanceStatusBlocked) {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) hasClosedApprovalTrace(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, approvalID common.ApprovalID) (bool, error) {
	if s == nil || s.traces == nil || workspaceID == "" || runID == "" || approvalID == "" {
		return false, common.ErrInvalidArgument
	}
	list, err := s.traces.List(ctx, workorchquerycontract.AutoGovernanceTraceQuery{
		WorkspaceID: workspaceID,
		RunID:       runID,
		ItemType:    "approval",
		ItemID:      string(approvalID),
	})
	if err != nil {
		return false, err
	}
	for _, item := range list {
		if item == nil {
			continue
		}
		if strings.EqualFold(item.Status, autoGovernanceStatusResolved) || strings.EqualFold(item.Status, autoGovernanceStatusBlocked) {
			return true, nil
		}
	}
	return false, nil
}
