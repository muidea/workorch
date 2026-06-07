package activationcore

import (
	"strings"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type WorkspaceExecutionContextBase struct {
	WorkspaceID            common.WorkspaceID
	Goal                   *workspacemodel.Goal
	DoD                    *workspacemodel.DoD
	GoalContexts           []map[string]any
	DoDContexts            []map[string]any
	DefinitionChangeImpact map[string]any
	ExecutionContract      map[string]any
	DialogLanguage         string
	EvidencePolicies       []map[string]any
}

func WorkspaceExecutionContextBaseContext(input WorkspaceExecutionContextBase) map[string]any {
	goalID := common.GoalID("")
	if input.Goal != nil {
		goalID = input.Goal.ID
	}
	doDID := common.DoDID("")
	if input.DoD != nil {
		doDID = input.DoD.ID
	}
	ret := map[string]any{
		"activation":  "workspace_execution",
		"workspaceId": string(input.WorkspaceID),
		"goalId":      string(goalID),
		"doDId":       string(doDID),
	}
	if len(input.ExecutionContract) > 0 {
		ret["executionContract"] = cloneContextMapDeep(input.ExecutionContract)
	}
	if input.Goal != nil {
		ret["goal"] = GoalContext(input.Goal)
	}
	if len(input.GoalContexts) > 0 {
		ret["goals"] = CloneContextMapSlice(input.GoalContexts)
		ret["goalDefinitionPolicy"] = "workspace_goals_are_cumulative_latest_refines_previous"
	}
	if input.DoD != nil {
		ret["dod"] = DoDContext(input.DoD)
	}
	if len(input.DoDContexts) > 0 {
		ret["dods"] = CloneContextMapSlice(input.DoDContexts)
		ret["doDDefinitionPolicy"] = "workspace_dods_are_cumulative_latest_refines_previous"
	}
	if len(input.DefinitionChangeImpact) > 0 {
		ret["definitionChangeImpact"] = cloneContextMapDeep(input.DefinitionChangeImpact)
	}
	if language := strings.TrimSpace(input.DialogLanguage); language != "" {
		ret["dialogLanguage"] = language
	}
	if len(input.EvidencePolicies) > 0 {
		ret["evidencePolicies"] = CloneContextMapSlice(input.EvidencePolicies)
	}
	return ret
}

func MergeGovernanceContinuationContext(contextData map[string]any, definitionChangeImpact map[string]any, governanceHistory []map[string]any, rejectedContinuation map[string]any) {
	if contextData == nil {
		return
	}
	definitionChanged := DefinitionChangeImpactChanged(definitionChangeImpact)
	if len(governanceHistory) > 0 {
		contextData["governanceHistory"] = CloneContextMapSlice(governanceHistory)
		if !definitionChanged {
			if replanContinuation := ReplanContinuationFromGovernanceHistory(governanceHistory); len(replanContinuation) > 0 {
				contextData["replanContinuation"] = replanContinuation
			}
		}
	}
	if definitionChanged {
		contextData["definitionChangeContinuationPolicy"] = "definition_change_supersedes_prior_actionable_continuation"
		return
	}
	if len(rejectedContinuation) > 0 {
		contextData["rejectedResultContinuation"] = cloneContextMapDeep(rejectedContinuation)
	}
}
