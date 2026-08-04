package readmodelprojection

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func ContextTaskProgressSection(progress projectioncontract.TaskProgress) projectioncontract.ContextSection {
	return projectioncontract.ContextSection{
		ID:           "task_progress",
		Kind:         "task_progress",
		Title:        "Task progress",
		Priority:     90,
		State:        progress.State,
		Summary:      progress.Summary,
		EvidenceRefs: progress.EvidenceRefs,
		Data: map[string]any{
			"currentMilestone": progress.CurrentMilestone,
			"milestones":       progress.Milestones,
			"blockers":         progress.Blockers,
			"recovery":         progress.Recovery,
		},
	}
}
