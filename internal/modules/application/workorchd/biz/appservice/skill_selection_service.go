package appservice

import (
	"context"
	"sort"
	"strings"

	soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const maxAutoSelectedSkills = 5

type skillSelectionEvidence struct {
	Policy           string              `json:"policy"`
	Status           string              `json:"status"`
	SelectedSkillIDs []string            `json:"selectedSkillIds,omitempty"`
	Matches          []map[string]any    `json:"matches,omitempty"`
	Reason           string              `json:"reason,omitempty"`
	ScopeCounts      map[string]int      `json:"scopeCounts,omitempty"`
	Metadata         map[string][]string `json:"metadata,omitempty"`
}

type skillCandidateScore struct {
	Skill   Skill
	Score   int
	Reasons []string
}

func selectSkillsForWorkspaceExecution(ctx context.Context, session *RuntimeSession, workspaceID common.WorkspaceID, goal *workspacemodel.Goal, dod *workspacemodel.DoD, requestedIDs []string, soul *soulmodel.Soul) ([]Skill, []string, skillSelectionEvidence, error) {
	requestedIDs = normalizeStringIDs(requestedIDs)
	if len(requestedIDs) > 0 {
		items, err := selectedSkills(ctx, session, requestedIDs, string(workspaceID))
		if err != nil {
			return nil, nil, skillSelectionEvidence{}, err
		}
		return items, requestedIDs, explicitSkillSelectionEvidence(items), nil
	}
	items, ids, evidence, err := autoSelectSkills(ctx, session, workspaceID, goal, dod, soul)
	if err != nil {
		return nil, nil, skillSelectionEvidence{}, err
	}
	return items, ids, evidence, nil
}

func autoSelectSkills(ctx context.Context, session *RuntimeSession, workspaceID common.WorkspaceID, goal *workspacemodel.Goal, dod *workspacemodel.DoD, soul *soulmodel.Soul) ([]Skill, []string, skillSelectionEvidence, error) {
	candidates, err := listSkillsForSelection(ctx, session, SkillQuery{WorkspaceID: string(workspaceID)})
	if err != nil {
		return nil, nil, skillSelectionEvidence{}, err
	}
	searchText := skillSelectionSearchText(goal, dod)
	allowed := allowedSkillRefSet(soul)
	scores := make([]skillCandidateScore, 0, len(candidates))
	scopeCounts := map[string]int{}
	for _, candidate := range candidates {
		if candidate.ID == "" {
			continue
		}
		scopeCounts[firstNonEmpty(candidate.Scope, "unknown")]++
		if allowed != nil {
			if _, ok := allowed[candidate.ID]; !ok {
				if candidate.SourceSkillID == "" {
					continue
				}
				if _, ok := allowed[candidate.SourceSkillID]; !ok {
					continue
				}
			}
		}
		score, reasons := scoreSkillCandidate(candidate, searchText)
		if score <= 0 {
			continue
		}
		scores = append(scores, skillCandidateScore{Skill: candidate, Score: score, Reasons: reasons})
	}
	sort.SliceStable(scores, func(i, j int) bool {
		if scores[i].Score != scores[j].Score {
			return scores[i].Score > scores[j].Score
		}
		if scores[i].Skill.Scope != scores[j].Skill.Scope {
			return scores[i].Skill.Scope == skillScopeWorkspace
		}
		return scores[i].Skill.ID < scores[j].Skill.ID
	})
	if len(scores) > maxAutoSelectedSkills {
		scores = scores[:maxAutoSelectedSkills]
	}
	items := make([]Skill, 0, len(scores))
	ids := make([]string, 0, len(scores))
	matches := make([]map[string]any, 0, len(scores))
	for _, item := range scores {
		items = append(items, item.Skill)
		ids = append(ids, item.Skill.ID)
		matches = append(matches, map[string]any{
			"skillId":       item.Skill.ID,
			"sourceSkillId": item.Skill.SourceSkillID,
			"scope":         item.Skill.Scope,
			"source":        item.Skill.Source,
			"score":         item.Score,
			"reasons":       item.Reasons,
		})
	}
	status := "none"
	reason := "no goal/dod trigger matched available skills"
	if len(items) > 0 {
		status = "selected"
		reason = ""
	}
	return items, ids, skillSelectionEvidence{
		Policy:           "goal_dod_auto_match",
		Status:           status,
		SelectedSkillIDs: ids,
		Matches:          matches,
		Reason:           reason,
		ScopeCounts:      scopeCounts,
	}, nil
}

func listSkillsForSelection(ctx context.Context, session *RuntimeSession, query SkillQuery) ([]Skill, error) {
	exports, err := session.Exports()
	if err != nil {
		return nil, err
	}
	if exports.Infrastructure.EventHub == nil {
		return nil, common.ErrInvalidArgument
	}
	items, err := listSkillsByOwnerCommand(ctx, exports.Infrastructure.EventHub)
	if err != nil {
		return nil, err
	}
	ret := make([]Skill, 0, len(items))
	for _, item := range items {
		ret = append(ret, mapSkill(item))
	}
	ret = append(ret, listSkillHubSkillsForQuery(ctx, session, query)...)
	return ret, nil
}

func explicitSkillSelectionEvidence(items []Skill) skillSelectionEvidence {
	ids := make([]string, 0, len(items))
	matches := make([]map[string]any, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
		matches = append(matches, map[string]any{
			"skillId":       item.ID,
			"sourceSkillId": item.SourceSkillID,
			"scope":         item.Scope,
			"source":        item.Source,
		})
	}
	return skillSelectionEvidence{
		Policy:           "explicit_skill_selection",
		Status:           "selected",
		SelectedSkillIDs: ids,
		Matches:          matches,
	}
}

func skillSelectionSearchText(goal *workspacemodel.Goal, dod *workspacemodel.DoD) string {
	parts := []string{}
	if goal != nil {
		parts = append(parts, goal.Text)
	}
	if dod != nil {
		parts = append(parts, dod.Items...)
	}
	return normalizeSkillMatchText(strings.Join(parts, " "))
}

func scoreSkillCandidate(skill Skill, searchText string) (int, []string) {
	if searchText == "" {
		return 0, nil
	}
	score := 0
	reasons := []string{}
	for _, rule := range skill.TriggerRules {
		ruleText := normalizeSkillMatchText(rule)
		if ruleText == "" {
			continue
		}
		if strings.Contains(searchText, ruleText) {
			score += 10
			reasons = append(reasons, "trigger:"+rule)
		}
	}
	for _, value := range []string{skill.Name, skill.SourceSkillID} {
		valueText := normalizeSkillMatchText(value)
		if valueText == "" {
			continue
		}
		if strings.Contains(searchText, valueText) {
			score += 4
			reasons = append(reasons, "name:"+value)
		}
	}
	return score, reasons
}

func normalizeSkillMatchText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer("-", " ", "_", " ", "/", " ", ".", " ", ",", " ", ":", " ", ";", " ", "\n", " ", "\t", " ")
	value = replacer.Replace(value)
	return strings.Join(strings.Fields(value), " ")
}

func allowedSkillRefSet(soul *soulmodel.Soul) map[string]struct{} {
	if soul == nil {
		return nil
	}
	ret := map[string]struct{}{}
	for _, value := range soul.AllowedSkillRefs {
		value = strings.TrimSpace(value)
		if value != "" {
			ret[value] = struct{}{}
		}
	}
	return ret
}
