package appmodelcore

type Goal struct {
	ID                     string `json:"id"`
	WorkspaceID            string `json:"workspaceId"`
	Text                   string `json:"text"`
	DefinitionRevision     string `json:"definitionRevision,omitempty"`
	PreviousDefinitionHash string `json:"previousDefinitionHash,omitempty"`
	PreviousGoalID         string `json:"previousGoalId,omitempty"`
	LineageState           string `json:"lineageState,omitempty"`
}

type DoD struct {
	ID                     string   `json:"id"`
	WorkspaceID            string   `json:"workspaceId"`
	Items                  []string `json:"items"`
	State                  string   `json:"state"`
	DefinitionRevision     string   `json:"definitionRevision,omitempty"`
	PreviousDefinitionHash string   `json:"previousDefinitionHash,omitempty"`
	PreviousDoDID          string   `json:"previousDoDId,omitempty"`
	LineageState           string   `json:"lineageState,omitempty"`
}
