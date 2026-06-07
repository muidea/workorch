package readmodelcore

type DoDEvidenceTrace struct {
	ResultID  string            `json:"resultId"`
	DoDID     string            `json:"doDId,omitempty"`
	Satisfied bool              `json:"satisfied"`
	Summary   string            `json:"summary,omitempty"`
	Items     []DoDEvidenceItem `json:"items,omitempty"`
}

type DoDEvidenceItem struct {
	Item                string   `json:"item"`
	State               string   `json:"state"`
	FailureReason       string   `json:"failureReason,omitempty"`
	Confidence          float64  `json:"confidence,omitempty"`
	SourceNote          string   `json:"sourceNote,omitempty"`
	EvidenceRefs        []string `json:"evidenceRefs,omitempty"`
	EvidenceStatus      string   `json:"evidenceStatus,omitempty"`
	MissingEvidenceRefs []string `json:"missingEvidenceRefs,omitempty"`
}
