package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func TaskProgressMilestones(snapshot readmodelcore.RunSnapshot, labels RunTaskProgressLabels, results []readmodelcore.Result, artifacts []readmodelcore.Artifact) []readmodelcore.ProgressMilestone {
	return []readmodelcore.ProgressMilestone{
		{ID: "run_created", Title: labels.RunCreated, State: "completed", EvidenceRefs: []string{snapshot.Run.ID}},
		{ID: "execution_steps", Title: labels.ExecutionSteps, State: StepProgressState(snapshot), EvidenceRefs: IDsFromSteps(snapshot.Steps)},
		{ID: "governance", Title: labels.Governance, State: GovernanceProgressState(snapshot), EvidenceRefs: GovernanceEvidenceRefs(snapshot)},
		{ID: "validation", Title: labels.Validation, State: ValidationProgressState(snapshot), EvidenceRefs: IDsFromValidations(snapshot.ValidationResults)},
		{ID: "formal_result", Title: labels.FormalResult, State: ResultProgressState(snapshot), EvidenceRefs: append(IDsFromResults(results), IDsFromArtifacts(artifacts)...)},
	}
}
