package runtimecore

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/localizationcore"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func ResolvedDecisionBlockerContext(item *runmodel.Run, language string) (projectioncontract.ProgressBlocker, projectioncontract.RecoveryPlan, string) {
	blocker := projectioncontract.ProgressBlocker{
		ID:          string(item.BlockingTicketID),
		Kind:        "decision",
		State:       string(common.DecisionTicketStateResolved),
		Reason:      localizationcore.BlockerReason(language, "decision", "resolved"),
		EvidenceRef: string(item.BlockingTicketID),
	}
	recovery := projectioncontract.RecoveryPlan{
		Mode:             "resume",
		NextAction:       "resume_run",
		RunID:            string(item.ID),
		BlockingTicketID: string(item.BlockingTicketID),
		Reason:           localizationcore.BlockerReason(language, "decision", "resolved"),
	}
	return blocker, recovery, localizationcore.BlockerReason(language, "decision", "resumed")
}

func ApprovedApprovalBlockerContext(item *runmodel.Run, language string) (projectioncontract.ProgressBlocker, projectioncontract.RecoveryPlan, string) {
	blocker := projectioncontract.ProgressBlocker{
		ID:          string(item.BlockingApprovalID),
		Kind:        "approval",
		State:       string(common.ApprovalStateApproved),
		Reason:      localizationcore.BlockerReason(language, "approval", "approved"),
		EvidenceRef: string(item.BlockingApprovalID),
	}
	recovery := projectioncontract.RecoveryPlan{
		Mode:               "resume",
		NextAction:         "resume_run",
		RunID:              string(item.ID),
		BlockingApprovalID: string(item.BlockingApprovalID),
		Reason:             localizationcore.BlockerReason(language, "approval", "approved"),
	}
	return blocker, recovery, localizationcore.BlockerReason(language, "approval", "resumed")
}

func BlockedRunResumeCompressedContext(item *runmodel.Run, blocker projectioncontract.ProgressBlocker, recovery projectioncontract.RecoveryPlan, summary string, language string) map[string]any {
	if item == nil || item.ID == "" || item.WorkspaceID == "" || blocker.ID == "" {
		return nil
	}
	return CompressedContextMap(projectioncontract.CompressedContext{
		Version:          "workorch.compressedContext",
		Mode:             "deterministic_recovery_summary",
		WorkspaceID:      string(item.WorkspaceID),
		RunID:            string(item.ID),
		Summary:          summary,
		CurrentMilestone: "execution_steps",
		Milestones: []projectioncontract.ProgressMilestone{
			{ID: "run_created", Title: localizationcore.MilestoneTitle(language, "run_created"), State: "completed", EvidenceRefs: []string{string(item.ID)}},
			{ID: "execution_steps", Title: localizationcore.MilestoneTitle(language, "execution_steps"), State: "in_progress"},
			{ID: "governance", Title: localizationcore.MilestoneTitle(language, "governance"), State: "in_progress", EvidenceRefs: []string{blocker.ID}},
			{ID: "validation", Title: localizationcore.MilestoneTitle(language, "validation"), State: "pending"},
			{ID: "formal_result", Title: localizationcore.MilestoneTitle(language, "formal_result"), State: "pending"},
		},
		Blockers:           []projectioncontract.ProgressBlocker{blocker},
		Recovery:           recovery,
		SourceSectionRefs:  []string{"run", "governance", "task_progress"},
		SourceEvidenceRefs: []string{string(item.ID), blocker.ID},
		RetentionPolicy:    localizationcore.RetentionPolicy(language),
	})
}
