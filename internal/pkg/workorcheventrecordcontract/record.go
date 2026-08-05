// Package workorcheventrecordcontract contains the immutable event record
// value shared by eventstream persistence and its typed EventHub contract. It
// defines no topic, command, repository or projection behavior.
package workorcheventrecordcontract

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

// Record is the complete immutable value appended by the Eventstream owner.
// Protocol projections may redact or reshape it but must not post it back as
// authority.
type Record struct {
	ID             common.EventRecordID
	Version        string
	Topic          string
	OccurredAt     string
	SourceModule   string
	SubjectType    string
	SubjectID      string
	WorkspaceID    common.WorkspaceID
	RunID          common.RunID
	StepID         common.StepID
	Revision       int64
	Payload        workorcheventpayload.Payload
	PayloadSummary workorcheventpayload.Payload
	PayloadRef     string
}
