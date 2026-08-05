package events

import (
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

type CLIStepViewInfo struct {
	ID            string
	Kind          string
	State         string
	Input         workorcheventpayload.Payload
	Output        workorcheventpayload.Payload
	ContractAudit workorcheventpayload.Payload
}

type CLIGapTodoItemInfo struct {
	ID                    string
	Source                string
	SourceRef             string
	CoverageWorkItemIDs   []string
	RequiredEvidenceKinds []string
	Status                string
	Priority              int
	Reason                string
	Metadata              workorcheventpayload.Payload
}

type CLIGapBaselineInfo struct {
	WorkspaceID           string
	RunID                 string
	DefinitionRevision    string
	BaselineRevision      string
	CurrentFocus          workorcheventpayload.Payload
	OpenCoverageWorkItems []workorcheventpayload.Payload
	AcceptedEvidenceRefs  []string
	RejectedEvidenceRefs  []string
	TodoItems             []CLIGapTodoItemInfo
	NextAction            projectioncontract.GapNextAction
	Diagnostics           []projectioncontract.GapDiagnostic
}
