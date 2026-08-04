package workorchprojectioncontract

// DoDAssessment is the owner-neutral read projection of already-judged DoD
// coverage. It carries no assessment or Runtime transition authority.
type DoDAssessment struct {
	Satisfied            bool                `json:"satisfied"`
	Items                []DoDAssessmentItem `json:"items,omitempty"`
	GoalCoverage         *DoDCoverage        `json:"goalCoverage,omitempty"`
	AcceptanceDimensions *DoDCoverage        `json:"acceptanceDimensions,omitempty"`
	DeliverableCoverage  *DoDCoverage        `json:"deliverableCoverage,omitempty"`
	Summary              string              `json:"summary,omitempty"`
}

type DoDAssessmentItem struct {
	Item          string   `json:"item"`
	State         string   `json:"state"`
	EvidenceRefs  []string `json:"evidenceRefs,omitempty"`
	FailureReason string   `json:"failureReason,omitempty"`
	Confidence    float64  `json:"confidence,omitempty"`
	SourceNote    string   `json:"sourceNote,omitempty"`
}

type DoDCoverage struct {
	Satisfied bool              `json:"satisfied"`
	Items     []DoDCoverageItem `json:"items,omitempty"`
	Summary   string            `json:"summary,omitempty"`
}

type DoDCoverageItem struct {
	ID           string   `json:"id,omitempty"`
	Deliverable  string   `json:"deliverable,omitempty"`
	Name         string   `json:"name,omitempty"`
	Path         string   `json:"path,omitempty"`
	Goal         string   `json:"goal,omitempty"`
	GoalItem     string   `json:"goalItem,omitempty"`
	Item         string   `json:"item,omitempty"`
	Requirement  string   `json:"requirement,omitempty"`
	State        string   `json:"state"`
	EvidenceRefs []string `json:"evidenceRefs,omitempty"`
	ArtifactRefs []string `json:"artifactRefs,omitempty"`
}
