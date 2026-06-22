package readmodelprojection

import ()

func referencedSnapshotStepIDs(steps []StepView) map[string]bool {
	ret := map[string]bool{}
	for _, step := range steps {
		if previous := PreviousStepIDSnapshot(step); previous != "" {
			ret[previous] = true
		}
	}
	return ret
}
