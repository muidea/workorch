package readmodelcore

import projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

type ResultPage struct {
	RunID             string                      `json:"runId"`
	Results           []projectioncontract.Result `json:"results,omitempty"`
	Limit             int                         `json:"limit"`
	CursorAfterID     string                      `json:"cursorAfterId,omitempty"`
	NextCursorAfterID string                      `json:"nextCursorAfterId,omitempty"`
	HasMore           bool                        `json:"hasMore"`
}

type PatchPage struct {
	RunID             string                     `json:"runId"`
	Patches           []projectioncontract.Patch `json:"patches,omitempty"`
	Limit             int                        `json:"limit"`
	CursorAfterID     string                     `json:"cursorAfterId,omitempty"`
	NextCursorAfterID string                     `json:"nextCursorAfterId,omitempty"`
	HasMore           bool                       `json:"hasMore"`
}

type ValidationResultPage struct {
	RunID             string                                `json:"runId"`
	ValidationResults []projectioncontract.ValidationResult `json:"validationResults,omitempty"`
	Limit             int                                   `json:"limit"`
	CursorAfterID     string                                `json:"cursorAfterId,omitempty"`
	NextCursorAfterID string                                `json:"nextCursorAfterId,omitempty"`
	HasMore           bool                                  `json:"hasMore"`
}

type ArtifactPage struct {
	RunID             string                        `json:"runId"`
	Artifacts         []projectioncontract.Artifact `json:"artifacts,omitempty"`
	Limit             int                           `json:"limit"`
	CursorAfterID     string                        `json:"cursorAfterId,omitempty"`
	NextCursorAfterID string                        `json:"nextCursorAfterId,omitempty"`
	HasMore           bool                          `json:"hasMore"`
}
