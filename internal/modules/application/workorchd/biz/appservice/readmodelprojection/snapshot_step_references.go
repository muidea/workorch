package readmodelprojection

import projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

func referencedSnapshotStepIDs(steps []projectioncontract.StepView) map[string]bool {
	ret := map[string]bool{}
	for _, step := range steps {
		if previous := PreviousStepIDSnapshot(step); previous != "" {
			ret[previous] = true
		}
	}
	return ret
}
