package readmodelcore

import projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

type ResultPage struct {
	RunID   string                      `json:"runId"`
	Results []projectioncontract.Result `json:"results,omitempty"`
	projectioncontract.CursorPage
}

type PatchPage struct {
	RunID   string                     `json:"runId"`
	Patches []projectioncontract.Patch `json:"patches,omitempty"`
	projectioncontract.CursorPage
}

type ValidationResultPage struct {
	RunID             string                                `json:"runId"`
	ValidationResults []projectioncontract.ValidationResult `json:"validationResults,omitempty"`
	projectioncontract.CursorPage
}

type ArtifactPage struct {
	RunID     string                        `json:"runId"`
	Artifacts []projectioncontract.Artifact `json:"artifacts,omitempty"`
	projectioncontract.CursorPage
}
