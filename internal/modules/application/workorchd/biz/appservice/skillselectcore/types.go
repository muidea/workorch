package skillselectcore

type Candidate struct {
	ID            string
	Name          string
	TriggerRules  []string
	SourceSkillID string
	Scope         string
	Source        string
}

type Evidence struct {
	Policy           string              `json:"policy"`
	Status           string              `json:"status"`
	SelectedSkillIDs []string            `json:"selectedSkillIds,omitempty"`
	Matches          []map[string]any    `json:"matches,omitempty"`
	Reason           string              `json:"reason,omitempty"`
	ScopeCounts      map[string]int      `json:"scopeCounts,omitempty"`
	Metadata         map[string][]string `json:"metadata,omitempty"`
}

type Selection struct {
	Candidates []Candidate
	IDs        []string
	Evidence   Evidence
}
