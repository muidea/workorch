package appservice

import (
	"strings"
	"unicode"

	"github.com/muidea/workorch/internal/pkg/common"
)

func runRequiredDoDLeafItems(snapshot RunSnapshot) []string {
	dod := readModelMapValue(snapshot.Run.ExecutionContext["dod"])
	items := readModelStringSliceValue(dod["items"])
	if len(items) == 0 {
		for _, item := range readModelMapSliceValue(snapshot.Run.ExecutionContext["dods"]) {
			items = append(items, readModelStringSliceValue(item["items"])...)
		}
	}
	if len(items) == 0 {
		return nil
	}
	ret := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, raw := range items {
		raw = strings.TrimSpace(raw)
		if raw == "" || !readModelDoDItemIsLeaf(raw) {
			continue
		}
		normalized := normalizeReadModelDoDText(raw)
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		ret = append(ret, raw)
	}
	return ret
}

func missingRequiredDoDLeafItems(required []string, assessed []DoDAssessmentItem) []string {
	if len(required) == 0 {
		return nil
	}
	covered := make([]string, 0, len(assessed))
	for _, item := range assessed {
		if strings.TrimSpace(item.State) != "satisfied" {
			continue
		}
		normalized := normalizeReadModelDoDText(item.Item)
		if normalized != "" {
			covered = append(covered, normalized)
		}
	}
	missing := make([]string, 0)
	for _, raw := range required {
		requiredNormalized := normalizeReadModelDoDText(raw)
		if requiredNormalized == "" {
			continue
		}
		matched := false
		for _, actual := range covered {
			if actual == "" {
				continue
			}
			if strings.Contains(actual, requiredNormalized) || strings.Contains(requiredNormalized, actual) {
				matched = true
				break
			}
		}
		if !matched {
			missing = append(missing, raw)
		}
	}
	return compactUniqueStrings(missing)
}

func weakValidationEvidenceRefIndex(snapshot RunSnapshot) map[string]bool {
	ret := map[string]bool{}
	for _, item := range snapshot.ValidationResults {
		quality := strings.TrimSpace(item.EvidenceQuality)
		if !item.ZeroWork && (quality == "" || quality == string(common.EvidenceQualityValid)) {
			continue
		}
		if ref := strings.TrimSpace(item.ID); ref != "" {
			ret[ref] = true
		}
		if ref := strings.TrimSpace(item.EvidenceRef); ref != "" {
			ret[ref] = true
		}
		if ref := strings.TrimSpace(item.StepID); ref != "" {
			ret[ref] = true
		}
	}
	return ret
}

func allEvidenceRefsWeakValidation(refs []string, weakRefIndex map[string]bool) bool {
	if len(refs) == 0 || len(weakRefIndex) == 0 {
		return false
	}
	seen := false
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		seen = true
		if !weakRefIndex[ref] {
			return false
		}
	}
	return seen
}

func readModelDoDItemIsLeaf(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "#") {
		return false
	}
	content := trimReadModelDoDItemText(trimmed)
	return !(content == "" || strings.HasSuffix(content, ":") || strings.HasSuffix(content, "："))
}

func normalizeReadModelDoDText(value string) string {
	value = trimReadModelDoDItemText(value)
	value = strings.NewReplacer("**", "", "__", "", "`", "", "（", "(", "）", ")").Replace(value)
	var builder strings.Builder
	for _, r := range strings.ToLower(value) {
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			continue
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

func trimReadModelDoDItemText(value string) string {
	value = strings.TrimSpace(value)
	for {
		next := strings.TrimSpace(value)
		lower := strings.ToLower(next)
		switch {
		case strings.HasPrefix(lower, "- [ ]"):
			value = strings.TrimSpace(next[len("- [ ]"):])
		case strings.HasPrefix(lower, "- [x]"):
			value = strings.TrimSpace(next[len("- [x]"):])
		case strings.HasPrefix(lower, "* [ ]"):
			value = strings.TrimSpace(next[len("* [ ]"):])
		case strings.HasPrefix(lower, "* [x]"):
			value = strings.TrimSpace(next[len("* [x]"):])
		case strings.HasPrefix(next, "-"), strings.HasPrefix(next, "*"), strings.HasPrefix(next, "•"):
			value = strings.TrimSpace(next[1:])
		default:
			return strings.TrimSpace(value)
		}
	}
}

func runEvidenceRefIndex(snapshot RunSnapshot) map[string]struct{} {
	ret := map[string]struct{}{}
	add := func(ref string) {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			return
		}
		ret[ref] = struct{}{}
	}
	add(snapshot.Run.ID)
	for _, item := range snapshot.Steps {
		add(item.ID)
	}
	for _, item := range snapshot.ToolCalls {
		add(item.ID)
		add(item.StepID)
	}
	for _, item := range snapshot.Patches {
		add(item.ID)
		add(item.StepID)
	}
	for _, item := range snapshot.ValidationResults {
		add(item.ID)
		add(item.StepID)
		add(item.EvidenceRef)
	}
	for _, item := range snapshot.RuntimeCheckpoints {
		add(item.ID)
		add(item.CurrentStepID)
	}
	for _, item := range snapshot.Results {
		add(item.ID)
		for _, ref := range item.EvidenceRefs {
			add(ref)
		}
	}
	for _, item := range snapshot.Artifacts {
		add(item.ID)
		add(item.StepID)
		add(item.ResultID)
		add(item.ToolCallID)
		add(item.PatchID)
		add(item.ValidationResultID)
		for _, ref := range item.EvidenceRefs {
			add(ref)
		}
	}
	for _, item := range snapshot.Decisions {
		add(item.ID)
		add(item.StepID)
	}
	for _, item := range snapshot.Approvals {
		add(item.ID)
		add(item.StepID)
	}
	for _, item := range snapshot.MemoryRecallTraces {
		add(item.ID)
		add(item.StepID)
		add(item.EvidenceRef)
	}
	return ret
}

func coverageSummaryFromRunSnapshot(snapshot RunSnapshot) map[string]any {
	ctx := snapshot.CoverageWorkItemContext
	if len(ctx) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{"summary", "currentFocus", "omittedCount", "fullLedgerRef"} {
		if value, ok := ctx[key]; ok && value != nil {
			ret[key] = value
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}
