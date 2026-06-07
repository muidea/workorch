package governancecore

import governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"

func LatestAutoGovernanceTrace(list []*governancemodel.AutoGovernanceTrace) *governancemodel.AutoGovernanceTrace {
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

func MapAutoGovernanceSummary(item *governancemodel.AutoGovernanceTrace) *AutoGovernanceSummary {
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
