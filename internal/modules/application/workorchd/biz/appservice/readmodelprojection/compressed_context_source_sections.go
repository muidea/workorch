package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func compressedContextSourceSections(snapshot readmodelcore.RunSnapshot, recentEvidence []readmodelcore.ContextEvidence) []readmodelcore.ContextSection {
	sections := compressedContextBaseSourceSections()
	sections = appendCompressedContextDynamicSourceSections(sections, snapshot, recentEvidence)
	sections = append(sections, readmodelcore.ContextSection{ID: "recovery", Priority: 100})
	return CompactContextSections(sections)
}

func compressedContextBaseSourceSections() []readmodelcore.ContextSection {
	return []readmodelcore.ContextSection{
		{ID: "workspace", Priority: 100},
		{ID: "run", Priority: 100},
	}
}
