package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func CompressedContextSourceSectionRefs(snapshot readmodelcore.RunSnapshot, recentEvidence []readmodelcore.ContextEvidence) []string {
	sections := compressedContextSourceSections(snapshot, recentEvidence)
	refs := make([]string, 0, len(sections))
	for _, section := range sections {
		refs = append(refs, section.ID)
	}
	return compactUniqueStrings(refs)
}

func HasNonDefinitionExecutionContext(executionContext map[string]any) bool {
	for key := range executionContext {
		if key != "goal" && key != "dod" && key != "compressedContext" {
			return true
		}
	}
	return false
}
