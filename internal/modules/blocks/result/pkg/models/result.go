package result

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchresultcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type Result struct {
	// Revision is the owner-local persistence revision. Terminal delivery is
	// created exactly once; later owner updates must compare this revision.
	Revision        uint64
	ID              common.ResultID
	WorkspaceID     common.WorkspaceID
	RunID           common.RunID
	GoalID          common.GoalID
	DoDID           common.DoDID
	State           common.ResultState
	Title           string
	Summary         string
	SourceType      string
	SourceRef       string
	EvidenceRefs    []string
	ArtifactRefs    []common.ArtifactID
	DoDAssessment   workorchresultcontract.DoDAssessment
	SummaryDetail   workorchresultcontract.ResultSummary
	AcceptedAt      string
	RejectedAt      string
	AcceptedBy      string
	CreatedAt       string
	UpdatedAt       string
	RuntimeCloseout *runtimecontract.RuntimeResultCloseout
	// TerminalDelivery is the owner-local immutable fence for the Runtime
	// Result delivery. It distinguishes a replay of the same sealed terminal
	// from a conflicting attempt to reuse the Result identifier.
	TerminalDelivery *runtimecontract.RuntimeTerminalDeliveryIdentity
}
