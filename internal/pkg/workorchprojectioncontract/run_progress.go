package workorchprojectioncontract

type ClosureTrace struct {
	StepID                              string           `json:"stepId"`
	Kind                                string           `json:"kind,omitempty"`
	State                               string           `json:"state,omitempty"`
	GraphFollowUp                       bool             `json:"graphFollowUp,omitempty"`
	AssessmentOnly                      bool             `json:"assessmentOnly,omitempty"`
	GraphExpansionLimitFollowUp         bool             `json:"graphExpansionLimitFollowUp,omitempty"`
	GraphFollowUpExpansionLimitExceeded bool             `json:"graphFollowUpExpansionLimitExceeded,omitempty"`
	GraphFollowUpExpansionCount         int              `json:"graphFollowUpExpansionCount,omitempty"`
	GraphFollowUpExpansionLimit         int              `json:"graphFollowUpExpansionLimit,omitempty"`
	IgnoredExecutionGraph               bool             `json:"ignoredExecutionGraph,omitempty"`
	Reason                              string           `json:"reason,omitempty"`
	MissingDoDItems                     []map[string]any `json:"missingDoDItems,omitempty"`
	EvidenceRefs                        []string         `json:"evidenceRefs,omitempty"`
}

type RunProgress struct {
	State            string `json:"state"`
	CurrentStepID    string `json:"currentStepId,omitempty"`
	CurrentStepKind  string `json:"currentStepKind,omitempty"`
	CurrentStepState string `json:"currentStepState,omitempty"`
	StepTotal        int    `json:"stepTotal"`
	StepDispatched   int    `json:"stepDispatched"`
	StepCompleted    int    `json:"stepCompleted"`
	StepFailed       int    `json:"stepFailed"`
	Blocked          bool   `json:"blocked"`
	Terminal         bool   `json:"terminal"`
}
