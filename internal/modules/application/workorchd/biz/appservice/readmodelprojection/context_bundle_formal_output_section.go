package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func ContextFormalOutputSection(snapshot readmodelcore.RunSnapshot) readmodelcore.ContextSection {
	results := RunScopedResults(snapshot)
	artifacts := RunScopedArtifacts(snapshot)
	if len(results) == 0 && len(artifacts) == 0 {
		return readmodelcore.ContextSection{}
	}
	return readmodelcore.ContextSection{
		ID:           "formal_outputs",
		Kind:         "formal_outputs",
		Title:        "Formal outputs",
		Priority:     90,
		State:        ResultProgressState(snapshot),
		Summary:      FormalOutputSummary(results),
		EvidenceRefs: append(IDsFromResults(results), IDsFromArtifacts(artifacts)...),
		Data: map[string]any{
			"results":   results,
			"artifacts": artifacts,
		},
	}
}
