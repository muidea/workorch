package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func TaskProgressEvidenceRefs(snapshot readmodelcore.RunSnapshot) []string {
	results := RunScopedResults(snapshot)
	artifacts := RunScopedArtifacts(snapshot)
	refs := []string{snapshot.Run.ID}
	if snapshot.Step != nil {
		refs = append(refs, snapshot.Step.ID)
	}
	refs = append(refs, IDsFromSteps(snapshot.Steps)...)
	refs = append(refs, IDsFromToolCalls(snapshot.ToolCalls)...)
	refs = append(refs, IDsFromPatches(snapshot.Patches)...)
	refs = append(refs, IDsFromValidations(snapshot.ValidationResults)...)
	refs = append(refs, IDsFromCheckpoints(snapshot.RuntimeCheckpoints)...)
	refs = append(refs, IDsFromResults(results)...)
	refs = append(refs, IDsFromArtifacts(artifacts)...)
	refs = append(refs, GovernanceEvidenceRefs(snapshot)...)
	return compactUniqueStrings(refs)
}
