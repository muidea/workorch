package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/localizationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func MapLocalizedWorkspaceTaskProgress(snapshot readmodelcore.WorkspaceSnapshotState) projectioncontract.TaskProgress {
	language := WorkspaceSnapshotLanguage(snapshot)
	return MapWorkspaceTaskProgress(snapshot, WorkspaceTaskProgressLabels{
		NoActiveRun:      localizationcore.WorkspaceProgressText(language, "no_active_run"),
		ActiveRun:        localizationcore.WorkspaceProgressText(language, "active_run"),
		WorkspaceDefined: localizationcore.WorkspaceProgressText(language, "workspace_defined"),
		FormalResult:     localizationcore.MilestoneTitle(language, "formal_result"),
		Runs:             localizationcore.WorkspaceProgressText(language, "runs"),
		Results:          localizationcore.WorkspaceProgressText(language, "results"),
		Artifacts:        localizationcore.WorkspaceProgressText(language, "artifacts"),
	})
}

func MapLocalizedTaskProgress(snapshot readmodelcore.RunSnapshotState) projectioncontract.TaskProgress {
	language := SnapshotLanguage(snapshot)
	return MapTaskProgress(snapshot, RunTaskProgressLabels{
		RecoveryNextAction: localizationcore.RecoveryNextAction(language),
		RunCreated:         localizationcore.MilestoneTitle(language, "run_created"),
		ExecutionSteps:     localizationcore.MilestoneTitle(language, "execution_steps"),
		Governance:         localizationcore.MilestoneTitle(language, "governance"),
		Validation:         localizationcore.MilestoneTitle(language, "validation"),
		FormalResult:       localizationcore.MilestoneTitle(language, "formal_result"),
	})
}
