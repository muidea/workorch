package events

import (
	"fmt"
	"strings"

	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventrecordcontract"
)

// EventRecordData is the Eventstream-owned EventHub value. The named type keeps
// the owner contract explicit while sharing the immutable field definition.
type EventRecordData workorcheventrecordcontract.Record

func (item EventRecordData) Validate() error {
	if item.ID == "" || strings.TrimSpace(item.Version) == "" || strings.TrimSpace(item.Topic) == "" ||
		strings.TrimSpace(item.OccurredAt) == "" || strings.TrimSpace(item.SourceModule) == "" ||
		strings.TrimSpace(item.SubjectType) == "" || strings.TrimSpace(item.SubjectID) == "" {
		return fmt.Errorf("%w: event record requires immutable identity and source", common.ErrInvalidArgument)
	}
	if item.WorkspaceID != "" && item.Revision < 1 {
		return fmt.Errorf("%w: workspace event record requires positive revision", common.ErrInvalidArgument)
	}
	if item.StreamPosition < 1 {
		return fmt.Errorf("%w: event record requires positive stream position", common.ErrInvalidArgument)
	}
	canonicalOccurredAt, err := workorcheventrecordcontract.CanonicalOccurredAt(item.OccurredAt)
	if err != nil {
		return err
	}
	if canonicalOccurredAt != item.OccurredAt {
		return fmt.Errorf("%w: event record occurredAt must use canonical UTC RFC3339", common.ErrInvalidArgument)
	}
	if !recordpayload.SanitizeAuditPayload(item.Payload).Equal(item.Payload) {
		return fmt.Errorf("%w: event record payload is not sanitized and bounded", common.ErrInvalidArgument)
	}
	if !recordpayload.SanitizeAuditPayload(item.PayloadSummary).Equal(item.PayloadSummary) {
		return fmt.Errorf("%w: event record payload summary is not sanitized and bounded", common.ErrInvalidArgument)
	}
	return nil
}

func EventRecordDataFromModel(item *eventstreammodel.EventRecord) (EventRecordData, error) {
	if item == nil {
		return EventRecordData{}, common.ErrInvalidArgument
	}
	ret := EventRecordData(*item)
	if !recordpayload.SanitizeAuditPayload(item.Payload).Equal(item.Payload) ||
		!recordpayload.SanitizeAuditPayload(item.PayloadSummary).Equal(item.PayloadSummary) {
		return EventRecordData{}, fmt.Errorf("%w: persisted event record payload is not sanitized and bounded", common.ErrInvalidArgument)
	}
	ret.Payload = item.Payload.Clone()
	ret.PayloadSummary = item.PayloadSummary.Clone()
	if err := ret.Validate(); err != nil {
		return EventRecordData{}, err
	}
	return ret, nil
}

func (item EventRecordData) Model() *eventstreammodel.EventRecord {
	ret := eventstreammodel.EventRecord(item)
	ret.Payload = item.Payload.Clone()
	ret.PayloadSummary = item.PayloadSummary.Clone()
	return &ret
}
