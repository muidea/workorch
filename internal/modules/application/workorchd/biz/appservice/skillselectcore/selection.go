package skillselectcore

const DefaultMaxAutoSelectedSkills = 5

func AutoSelect(candidates []Candidate, goalText string, dodItems []string, allowedRefs []string, workspaceScope string, maxSelected int) Selection {
	if maxSelected <= 0 {
		maxSelected = DefaultMaxAutoSelectedSkills
	}
	searchText := SearchText(goalText, dodItems)
	scores, scopeCounts := collectAutoCandidateScores(candidates, searchText, allowedRefs, workspaceScope)
	if len(scores) > maxSelected {
		scores = scores[:maxSelected]
	}
	return autoSelectionFromScores(scores, scopeCounts)
}
