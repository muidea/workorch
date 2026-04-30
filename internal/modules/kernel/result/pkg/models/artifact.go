package result

import "github.com/muidea/workorch/internal/pkg/common"

type Artifact struct {
	ID                 common.ArtifactID
	WorkspaceID        common.WorkspaceID
	RunID              common.RunID
	ResultID           common.ResultID
	StepID             common.StepID
	ToolCallID         common.ToolCallID
	PatchID            common.PatchID
	ValidationResultID common.ValidationResultID
	ArtifactType       string
	Category           string
	Name               string
	Summary            string
	URI                string
	ContentType        string
	State              common.ArtifactState
	EvidenceRefs       []string
	Metadata           map[string]any
	CreatedAt          string
	UpdatedAt          string
}
