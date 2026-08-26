package decision

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

type Resolution struct {
	ID                 common.ResolutionID
	TicketID           common.TicketID
	ResolvedAt         string
	Type               string
	Payload            workorcheventpayload.Payload
	ContinuationAction string
	ResumeRunID        common.RunID
	SuccessorRunID     common.RunID
}

func CloneResolution(item *Resolution) *Resolution {
	if item == nil {
		return nil
	}
	cloned := *item
	cloned.Payload = item.Payload.Clone()
	return &cloned
}

func CloneResolutions(items []*Resolution) []*Resolution {
	if items == nil {
		return nil
	}
	ret := make([]*Resolution, 0, len(items))
	for _, item := range items {
		ret = append(ret, CloneResolution(item))
	}
	return ret
}
