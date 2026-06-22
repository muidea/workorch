package decision

import "github.com/muidea/workorch/internal/pkg/common"

type Resolution struct {
	ID                 common.ResolutionID
	TicketID           common.TicketID
	Type               string
	Payload            map[string]any
	ContinuationAction string
	ResumeRunID        common.RunID
	SuccessorRunID     common.RunID
}
