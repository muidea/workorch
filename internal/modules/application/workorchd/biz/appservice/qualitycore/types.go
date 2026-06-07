package qualitycore

type Command struct {
	WorkspaceID string   `json:"workspaceId"`
	GoalID      string   `json:"goalId,omitempty"`
	DoDID       string   `json:"doDId,omitempty"`
	GoalText    string   `json:"goalText,omitempty"`
	DoDItems    []string `json:"doDItems,omitempty"`
}

type Issue struct {
	Code       string `json:"code"`
	Severity   string `json:"severity"`
	Target     string `json:"target"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

type Check struct {
	WorkspaceID        string   `json:"workspaceId"`
	GoalID             string   `json:"goalId,omitempty"`
	DoDID              string   `json:"doDId,omitempty"`
	GoalText           string   `json:"goalText,omitempty"`
	DoDItems           []string `json:"doDItems,omitempty"`
	Executable         bool     `json:"executable"`
	Verifiable         bool     `json:"verifiable"`
	ExecutionScore     int      `json:"executionScore"`
	VerificationScore  int      `json:"verificationScore"`
	Status             string   `json:"status"`
	MissingConstraints []string `json:"missingConstraints,omitempty"`
	Suggestions        []string `json:"suggestions,omitempty"`
	Issues             []Issue  `json:"issues,omitempty"`
}
