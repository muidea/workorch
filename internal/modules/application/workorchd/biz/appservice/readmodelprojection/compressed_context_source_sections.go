package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func compressedContextSourceSections(snapshot readmodelcore.RunSnapshotState, recentEvidence []projectioncontract.ContextEvidence) []projectioncontract.ContextSection {
	sections := compressedContextBaseSourceSections()
	sections = appendCompressedContextDynamicSourceSections(sections, snapshot, recentEvidence)
	sections = append(sections, projectioncontract.ContextSection{ID: "recovery", Priority: 100})
	return CompactContextSections(sections)
}

func compressedContextBaseSourceSections() []projectioncontract.ContextSection {
	return []projectioncontract.ContextSection{
		{ID: "workspace", Priority: 100},
		{ID: "run", Priority: 100},
	}
}
