package activationcore

import (
	"strings"

	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
)

func RejectedContinuationCoverageGaps(coverage *resultmodel.DoDCoverage, scope string) []map[string]any {
	if coverage == nil || len(coverage.Items) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, len(coverage.Items))
	for _, item := range coverage.Items {
		if RejectedContinuationCoverageItemSatisfied(item, scope) {
			continue
		}
		mapped := map[string]any{}
		for key, value := range RejectedContinuationCoverageIdentity(item) {
			mapped[key] = CompactContextValue(value, 1)
		}
		mapped["state"] = strings.TrimSpace(item.State)
		if refs := CompactStringRefs(item.EvidenceRefs, 10); len(refs) > 0 {
			mapped["evidenceRefs"] = refs
		}
		if refs := CompactArtifactRefs(item.ArtifactRefs, 10); len(refs) > 0 {
			mapped["artifactRefs"] = refs
		}
		ret = append(ret, mapped)
		if len(ret) >= 20 {
			break
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func RejectedContinuationCoverageItemSatisfied(item resultmodel.DoDCoverageItem, scope string) bool {
	if !strings.EqualFold(strings.TrimSpace(item.State), "satisfied") {
		return false
	}
	if scope == "deliverable" {
		return len(CompactStringRefs(item.EvidenceRefs, 1)) > 0 || len(CompactArtifactRefs(item.ArtifactRefs, 1)) > 0
	}
	return len(CompactStringRefs(item.EvidenceRefs, 1)) > 0
}

func RejectedContinuationCoverageIdentity(item resultmodel.DoDCoverageItem) map[string]string {
	ret := map[string]string{}
	for key, value := range map[string]string{
		"id":          item.ID,
		"deliverable": item.Deliverable,
		"name":        item.Name,
		"path":        item.Path,
		"goal":        item.Goal,
		"goalItem":    item.GoalItem,
		"item":        item.Item,
		"requirement": item.Requirement,
	} {
		if strings.TrimSpace(value) != "" {
			ret[key] = strings.TrimSpace(value)
		}
	}
	return ret
}
