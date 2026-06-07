package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
)

func webSearchTrace(step presenter.StepView) readmodelcore.WebSourceTrace {
	output := step.Output
	evidenceRef := step.ID
	results := readModelMapSliceValue(output["results"])
	sources := make([]readmodelcore.WebSource, 0, len(results))
	for _, result := range results {
		item := readmodelcore.WebSource{
			Title:       readModelStringValue(result["title"]),
			URL:         readModelStringValue(result["url"]),
			Snippet:     readModelStringValue(result["snippet"]),
			Source:      readModelStringValue(result["source"]),
			PublishedAt: readModelStringValue(result["publishedAt"]),
			Rank:        readModelIntValue(result["rank"]),
			EvidenceRef: evidenceRef,
		}
		if item.URL != "" {
			sources = append(sources, item)
		}
	}
	return readmodelcore.WebSourceTrace{
		Query:          readModelStringValue(output["query"]),
		ProviderID:     readModelStringValue(output["provider"]),
		SearchedAt:     readModelStringValue(output["searchedAt"]),
		Sources:        sources,
		EvidenceRefs:   []string{evidenceRef},
		PolicyDecision: readModelPolicyDecision(output["policy"]),
	}
}
