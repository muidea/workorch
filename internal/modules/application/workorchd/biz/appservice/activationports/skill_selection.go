package activationports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/skillselectcore"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

type WorkspaceSkillSelectionRequest struct {
	Hub             magicevent.Hub
	WorkspaceID     common.WorkspaceID
	Goal            *workspacemodel.Goal
	DoD             *workspacemodel.DoD
	RequestedIDs    []string
	Soul            *soulmodel.Soul
	SkillHubCatalog contextregistrycore.SkillHubSkillCatalog
}

type WorkspaceSkillSelectionResult struct {
	Skills   []workorchmanagementcontract.Skill
	IDs      []string
	Evidence skillselectcore.Evidence
}

func SelectWorkspaceExecutionSkills(ctx context.Context, request WorkspaceSkillSelectionRequest) (WorkspaceSkillSelectionResult, error) {
	requestedIDs := activationcore.NormalizeStringIDs(request.RequestedIDs)
	if len(requestedIDs) > 0 {
		items, err := SelectedSkills(ctx, request.Hub, requestedIDs, string(request.WorkspaceID), request.SkillHubCatalog)
		if err != nil {
			return WorkspaceSkillSelectionResult{}, err
		}
		return WorkspaceSkillSelectionResult{
			Skills:   items,
			IDs:      requestedIDs,
			Evidence: skillselectcore.ExplicitEvidence(skillselectcore.CandidatesFromSkills(items)),
		}, nil
	}
	candidates, err := SkillsForSelection(ctx, request.Hub, contextregistrycore.SkillQuery{WorkspaceID: string(request.WorkspaceID)}, request.SkillHubCatalog)
	if err != nil {
		return WorkspaceSkillSelectionResult{}, err
	}
	selection := skillselectcore.AutoSelect(
		skillselectcore.CandidatesFromSkills(candidates),
		skillselectcore.GoalText(request.Goal),
		skillselectcore.DoDItems(request.DoD),
		skillselectcore.AllowedSkillRefs(request.Soul),
		contextregistrycore.SkillScopeWorkspace,
		skillselectcore.DefaultMaxAutoSelectedSkills,
	)
	return WorkspaceSkillSelectionResult{
		Skills:   skillselectcore.SkillsBySelectedCandidates(candidates, selection.Candidates),
		IDs:      selection.IDs,
		Evidence: selection.Evidence,
	}, nil
}
