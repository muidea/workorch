package readmodelcore

type ResultQuery struct {
	WorkspaceID   string
	RunID         string
	CursorAfterID string
	Limit         int
}

type ResultPage struct {
	RunID             string   `json:"runId"`
	Results           []Result `json:"results,omitempty"`
	Limit             int      `json:"limit"`
	CursorAfterID     string   `json:"cursorAfterId,omitempty"`
	NextCursorAfterID string   `json:"nextCursorAfterId,omitempty"`
	HasMore           bool     `json:"hasMore"`
}

type PatchQuery struct {
	WorkspaceID   string
	RunID         string
	CursorAfterID string
	Limit         int
}

type PatchPage struct {
	RunID             string  `json:"runId"`
	Patches           []Patch `json:"patches,omitempty"`
	Limit             int     `json:"limit"`
	CursorAfterID     string  `json:"cursorAfterId,omitempty"`
	NextCursorAfterID string  `json:"nextCursorAfterId,omitempty"`
	HasMore           bool    `json:"hasMore"`
}

type ValidationResultQuery struct {
	WorkspaceID   string
	RunID         string
	CursorAfterID string
	Limit         int
}

type ValidationResultPage struct {
	RunID             string             `json:"runId"`
	ValidationResults []ValidationResult `json:"validationResults,omitempty"`
	Limit             int                `json:"limit"`
	CursorAfterID     string             `json:"cursorAfterId,omitempty"`
	NextCursorAfterID string             `json:"nextCursorAfterId,omitempty"`
	HasMore           bool               `json:"hasMore"`
}

type ArtifactQuery struct {
	WorkspaceID   string
	RunID         string
	CursorAfterID string
	Limit         int
}

type ArtifactPage struct {
	RunID             string     `json:"runId"`
	Artifacts         []Artifact `json:"artifacts,omitempty"`
	Limit             int        `json:"limit"`
	CursorAfterID     string     `json:"cursorAfterId,omitempty"`
	NextCursorAfterID string     `json:"nextCursorAfterId,omitempty"`
	HasMore           bool       `json:"hasMore"`
}
