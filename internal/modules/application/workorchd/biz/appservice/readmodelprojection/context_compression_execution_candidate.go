package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func executionContextCompressionCandidate(sections []readmodelcore.ContextSection) (readmodelcore.ContextCompressionCandidate, bool) {
	for _, section := range sections {
		if section.ID != "execution_context" || len(section.Data) <= 4 {
			continue
		}
		return readmodelcore.ContextCompressionCandidate{
			SectionID:     section.ID,
			Reason:        "execution context catalog can be summarized after activation",
			Retention:     "retain selected ids and policy evidence, summarize verbose catalog entries",
			SuggestedMode: "catalog_summary",
		}, true
	}
	return readmodelcore.ContextCompressionCandidate{}, false
}
