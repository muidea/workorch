package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func referencedSnapshotStepIDs(steps []readmodelcore.StepView) map[string]bool {
	ret := map[string]bool{}
	for _, step := range steps {
		if previous := PreviousStepIDSnapshot(step); previous != "" {
			ret[previous] = true
		}
	}
	return ret
}
