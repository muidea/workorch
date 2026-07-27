package result

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
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
	DoDAssessment   DoDAssessment
	SummaryDetail   ResultSummary
	AcceptedAt      string
	RejectedAt      string
	AcceptedBy      string
	CreatedAt       string
	UpdatedAt       string
	RuntimeCloseout *runtimecontract.RuntimeResultCloseout
	// TerminalDelivery is the owner-local immutable fence for the Runtime V2
	// Result delivery. It distinguishes a replay of the same sealed terminal
	// from a conflicting attempt to reuse the Result identifier.
	TerminalDelivery *runtimecontract.RuntimeTerminalDeliveryIdentity
}

type ResultSummary struct {
	GoalOutcome           string
	DoDItems              []DoDAssessmentItem
	ImplementationSummary string
	VerificationSummary   string
	EvidenceRefs          []string
	ArtifactRefs          []common.ArtifactID
	KnownLimitations      []string
	ResidualRisks         []string
	NextActions           []string
	Diagnostics           workorcheventpayload.Payload
}

type DoDAssessment struct {
	Satisfied            bool
	Items                []DoDAssessmentItem
	GoalCoverage         *DoDCoverage
	AcceptanceDimensions *DoDCoverage
	DeliverableCoverage  *DoDCoverage
	Summary              string
}

type DoDAssessmentItem struct {
	Item          string
	State         string
	EvidenceRefs  []string
	FailureReason string
	Confidence    float64
	SourceNote    string
}

type DoDCoverage struct {
	Satisfied bool
	Items     []DoDCoverageItem
	Summary   string
}

type DoDCoverageItem struct {
	ID           string
	Deliverable  string
	Name         string
	Path         string
	Goal         string
	GoalItem     string
	Item         string
	Requirement  string
	State        string
	EvidenceRefs []string
	ArtifactRefs []common.ArtifactID
}
