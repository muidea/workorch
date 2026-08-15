package readmodelprojection

import (
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

// WD5 mechanical consolidation: same import contract; source list follows.

// source: run_step_blockers.go

func runStatusStepIndex(steps []*runmodel.Step) map[common.StepID]*runmodel.Step {
	byID := make(map[common.StepID]*runmodel.Step, len(steps))
	for _, step := range steps {
		if step == nil || step.ID == "" {
			continue
		}
		byID[step.ID] = step
	}
	return byID
}

// source: run_step_terminal.go

func lastStep(steps []*runmodel.Step) *runmodel.Step {
	if len(steps) == 0 {
		return nil
	}
	return steps[len(steps)-1]
}

func latestNonTerminalRunStatusStep(steps []*runmodel.Step) *runmodel.Step {
	for idx := len(steps) - 1; idx >= 0; idx-- {
		step := steps[idx]
		if step == nil {
			continue
		}
		if step.State == common.StepStateExecuting || step.State == common.StepStatePending {
			return step
		}
	}
	return nil
}

func terminalRunStatusStep(steps []*runmodel.Step) *runmodel.Step {
	terminals := terminalRunStatusStepCandidates(steps)
	if len(terminals) == 0 {
		return nil
	}
	return selectTerminalRunStatusStep(terminals)
}

func terminalRunStatusStepWithCheckpoints(steps []*runmodel.Step, checkpoints []*runmodel.RuntimeCheckpoint) *runmodel.Step {
	terminals := terminalRunStatusStepCandidates(steps)
	if len(terminals) == 0 {
		return nil
	}
	if step := latestTerminalRunStatusStepFromCheckpoints(terminals, checkpoints); step != nil {
		return step
	}
	return selectTerminalRunStatusStep(terminals)
}

func selectTerminalRunStatusStep(terminals []*runmodel.Step) *runmodel.Step {
	for _, step := range terminals {
		if step.State == common.StepStateExecuting {
			return step
		}
	}
	for idx := len(terminals) - 1; idx >= 0; idx-- {
		if terminals[idx].State == common.StepStateFailed {
			return terminals[idx]
		}
	}
	return terminals[len(terminals)-1]
}

func latestTerminalRunStatusStepFromCheckpoints(terminals []*runmodel.Step, checkpoints []*runmodel.RuntimeCheckpoint) *runmodel.Step {
	if len(terminals) == 0 || len(checkpoints) == 0 {
		return nil
	}
	terminalByID := map[common.StepID]*runmodel.Step{}
	for _, step := range terminals {
		if step == nil {
			continue
		}
		terminalByID[step.ID] = step
	}
	var selected *runmodel.Step
	selectedAt := ""
	for _, checkpoint := range checkpoints {
		if checkpoint == nil || checkpoint.CurrentStepID == "" {
			continue
		}
		step := terminalByID[checkpoint.CurrentStepID]
		if step == nil || step.State != common.StepStateFailed {
			continue
		}
		occurredAt := checkpoint.OccurredAt
		if selected == nil || occurredAt > selectedAt {
			selected = step
			selectedAt = occurredAt
		}
	}
	return selected
}

// source: run_step_terminal_candidates.go

func terminalRunStatusStepCandidates(steps []*runmodel.Step) []*runmodel.Step {
	referenced := referencedPreviousRunStatusStepIDs(steps)
	var terminals []*runmodel.Step
	for _, step := range steps {
		if step == nil || referenced[step.ID] {
			continue
		}
		terminals = append(terminals, step)
	}
	return terminals
}

func referencedPreviousRunStatusStepIDs(steps []*runmodel.Step) map[common.StepID]bool {
	referenced := map[common.StepID]bool{}
	for _, step := range steps {
		if previous := previousStepID(step); previous != "" {
			referenced[previous] = true
		}
	}
	return referenced
}
