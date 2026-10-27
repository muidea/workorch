package skillselectcore

import "github.com/muidea/workorch/internal/pkg/skillselection"

func SearchText(goal string, dod []string) string { return skillselection.SearchText(goal, dod) }
func ScoreCandidate(c skillselection.Candidate, search string) (int, []string) {
	return skillselection.ScoreCandidate(c, search)
}
func NormalizeMatchText(value string) string { return skillselection.NormalizeMatchText(value) }
