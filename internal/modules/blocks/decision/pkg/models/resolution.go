package decision

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

type Resolution struct {
	ID                 common.ResolutionID
	TicketID           common.TicketID
	Type               string
	Payload            workorcheventpayload.Payload
	ContinuationAction string
	ResumeRunID        common.RunID
	SuccessorRunID     common.RunID
}
