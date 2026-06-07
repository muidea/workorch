package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
)

func terminalSnapshotStepCandidates(steps []presenter.StepView) []*presenter.StepView {
	referenced := referencedSnapshotStepIDs(steps)
	var terminals []*presenter.StepView
	for idx := range steps {
		if steps[idx].ID == "" || referenced[steps[idx].ID] {
			continue
		}
		terminals = append(terminals, &steps[idx])
	}
	return terminals
}
