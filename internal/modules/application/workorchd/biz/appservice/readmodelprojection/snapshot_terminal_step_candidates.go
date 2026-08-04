package readmodelprojection

import projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

func terminalSnapshotStepCandidates(steps []projectioncontract.StepView) []*projectioncontract.StepView {
	referenced := referencedSnapshotStepIDs(steps)
	var terminals []*projectioncontract.StepView
	for idx := range steps {
		if steps[idx].ID == "" || referenced[steps[idx].ID] {
			continue
		}
		terminals = append(terminals, &steps[idx])
	}
	return terminals
}
