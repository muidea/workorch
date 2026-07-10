package readmodelprojection

func terminalSnapshotStepCandidates(steps []StepView) []*StepView {
	referenced := referencedSnapshotStepIDs(steps)
	var terminals []*StepView
	for idx := range steps {
		if steps[idx].ID == "" || referenced[steps[idx].ID] {
			continue
		}
		terminals = append(terminals, &steps[idx])
	}
	return terminals
}
