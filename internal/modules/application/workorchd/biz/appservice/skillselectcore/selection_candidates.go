package skillselectcore

import "sort"

type candidateScore struct {
	Candidate Candidate
	Score     int
	Reasons   []string
}

func collectAutoCandidateScores(candidates []Candidate, searchText string, allowedRefs []string, workspaceScope string) ([]candidateScore, map[string]int) {
	allowed := allowedRefSet(allowedRefs)
	scores := make([]candidateScore, 0, len(candidates))
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
		score, reasons := ScoreCandidate(candidate, searchText)
		if score <= 0 {
			continue
		}
		scores = append(scores, candidateScore{Candidate: candidate, Score: score, Reasons: reasons})
	}
	sort.SliceStable(scores, func(i, j int) bool {
		if scores[i].Score != scores[j].Score {
			return scores[i].Score > scores[j].Score
		}
		if scores[i].Candidate.Scope != scores[j].Candidate.Scope {
			return scores[i].Candidate.Scope == workspaceScope
		}
		return scores[i].Candidate.ID < scores[j].Candidate.ID
	})
	return scores, scopeCounts
}

func autoSelectionFromScores(scores []candidateScore, scopeCounts map[string]int) Selection {
	items := make([]Candidate, 0, len(scores))
	ids := make([]string, 0, len(scores))
	matches := make([]map[string]any, 0, len(scores))
	for _, item := range scores {
		candidate := item.Candidate
		items = append(items, candidate)
		ids = append(ids, candidate.ID)
		matches = append(matches, map[string]any{
			"skillId":       candidate.ID,
			"sourceSkillId": candidate.SourceSkillID,
			"scope":         candidate.Scope,
			"source":        candidate.Source,
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
	return Selection{
		Candidates: items,
		IDs:        ids,
		Evidence: Evidence{
			Policy:           "goal_dod_auto_match",
			Status:           status,
			SelectedSkillIDs: ids,
			Matches:          matches,
			Reason:           reason,
			ScopeCounts:      scopeCounts,
		},
	}
}
