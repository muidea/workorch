package workorchprojectioncontract

type ResultQualityTrace struct {
	ResultID     string               `json:"resultId"`
	DoDID        string               `json:"doDId,omitempty"`
	State        string               `json:"state"`
	Summary      string               `json:"summary,omitempty"`
	EvidenceRefs []string             `json:"evidenceRefs,omitempty"`
	Issues       []ResultQualityIssue `json:"issues,omitempty"`
}

type ResultQualityIssue struct {
	Code         string   `json:"code"`
	Severity     string   `json:"severity"`
	Message      string   `json:"message"`
	Suggestion   string   `json:"suggestion,omitempty"`
	EvidenceRefs []string `json:"evidenceRefs,omitempty"`
}
