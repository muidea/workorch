package result

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

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
	Metadata           workorcheventpayload.Payload
	// SourceRef is the opaque Runtime artifact reference from which this child
	// metadata record was materialized. Artifact content remains outside the
	// EventHub payload and is addressed through URI/SourceRef.
	SourceRef          string
	TerminalDeliveryID string
	MaterialHash       string
	CreatedAt          string
	UpdatedAt          string
}
