package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func terminalSnapshotStepCandidates(steps []readmodelcore.StepView) []*readmodelcore.StepView {
	referenced := referencedSnapshotStepIDs(steps)
	var terminals []*readmodelcore.StepView
	for idx := range steps {
		if steps[idx].ID == "" || referenced[steps[idx].ID] {
			continue
		}
		terminals = append(terminals, &steps[idx])
	}
	return terminals
}
