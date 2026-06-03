package appservice

import (
	"context"
	"errors"

	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type AutoGovernanceSummary struct {
	TraceID           string   `json:"traceId"`
	Action            string   `json:"action"`
	Status            string   `json:"status"`
	RiskLevel         string   `json:"riskLevel,omitempty"`
	PolicyMatched     string   `json:"policyMatched,omitempty"`
	PolicyBlocked     string   `json:"policyBlocked,omitempty"`
	Confidence        float64  `json:"confidence,omitempty"`
	Reason            string   `json:"reason,omitempty"`
	EvidenceRefs      []string `json:"evidenceRefs,omitempty"`
	PromptFingerprint string   `json:"promptFingerprint,omitempty"`
	OccurredAt        string   `json:"occurredAt,omitempty"`
}

func autoGovernanceTraceReaderFromSession(session *RuntimeSession) readModelAutoGovernanceTraceReader {
	if session == nil {
		return nil
	}
	exports, err := session.Exports()
	if err != nil || exports.Infrastructure.EventHub == nil {
		return nil
	}
	return eventHubReadModelAutoGovernanceTraceReader{hub: exports.Infrastructure.EventHub}
}

func autoGovernanceTraceReaderFromHub(hub events.Hub) readModelAutoGovernanceTraceReader {
	if hub == nil {
		return nil
	}
	return eventHubReadModelAutoGovernanceTraceReader{hub: hub}
}

func latestAutoGovernanceSummary(
	ctx context.Context,
	traces readModelAutoGovernanceTraceReader,
	workspaceID common.WorkspaceID,
	runID common.RunID,
	itemType string,
	itemID string,
) (*AutoGovernanceSummary, error) {
	if traces == nil || itemType == "" || itemID == "" {
		return nil, nil
	}
	list, err := traces.List(ctx, repository.AutoGovernanceTraceQuery{
		WorkspaceID: workspaceID,
		RunID:       runID,
		ItemType:    itemType,
		ItemID:      itemID,
		Limit:       10,
	})
	if err != nil {
		if errors.Is(err, common.ErrInvalidArgument) {
			return nil, nil
		}
		return nil, err
	}
	if len(list) == 0 {
		return nil, err
	}
	latest := latestAutoGovernanceTrace(list)
	if latest == nil {
		return nil, nil
	}
	return mapAutoGovernanceSummary(latest), nil
}

func latestAutoGovernanceTrace(list []*governancemodel.AutoGovernanceTrace) *governancemodel.AutoGovernanceTrace {
	var latest *governancemodel.AutoGovernanceTrace
	for _, item := range list {
		if item == nil {
			continue
		}
		if latest == nil || item.OccurredAt > latest.OccurredAt || (item.OccurredAt == latest.OccurredAt && item.ID > latest.ID) {
			latest = item
		}
	}
	return latest
}

func mapAutoGovernanceSummary(item *governancemodel.AutoGovernanceTrace) *AutoGovernanceSummary {
	if item == nil {
		return nil
	}
	ret := &AutoGovernanceSummary{
		TraceID:           string(item.ID),
		Action:            item.Action,
		Status:            item.Status,
		RiskLevel:         item.RiskLevel,
		PolicyMatched:     item.PolicyMatched,
		PolicyBlocked:     item.PolicyBlocked,
		Confidence:        item.Confidence,
		Reason:            item.Reason,
		PromptFingerprint: item.PromptFingerprint,
		OccurredAt:        item.OccurredAt,
	}
	if len(item.EvidenceRefs) > 0 {
		ret.EvidenceRefs = append([]string(nil), item.EvidenceRefs...)
	}
	return ret
}
