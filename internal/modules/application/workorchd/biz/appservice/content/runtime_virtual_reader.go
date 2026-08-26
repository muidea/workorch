package content

import (
	"context"
	"encoding/json"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (r *Reader) readRuntimeCheckpointRaw(ctx context.Context, parsed RuntimeRawRef) (RuntimeRawContent, error) {
	if r.eventHub == nil {
		return RuntimeRawContent{}, common.ErrNotFound
	}
	item, err := ownerreadports.ReadRuntimeCheckpointByOwnerCommand(ctx, r.eventHub, common.CheckpointID(parsed.ItemID))
	if err != nil {
		return RuntimeRawContent{}, err
	}
	if item.WorkspaceID != parsed.WorkspaceID || item.RunID != parsed.RunID {
		return RuntimeRawContent{}, common.ErrNotFound
	}
	body, err := json.Marshal(item.Data)
	if err != nil {
		return RuntimeRawContent{}, err
	}
	return newRuntimeRawContent(parsed, body)
}

func (r *Reader) readResultRaw(ctx context.Context, parsed RuntimeRawRef) (RuntimeRawContent, error) {
	if r.eventHub == nil {
		return RuntimeRawContent{}, common.ErrNotFound
	}
	item, err := ownerreadports.ReadResultByOwnerCommand(ctx, r.eventHub, common.ResultID(parsed.ItemID))
	if err != nil {
		return RuntimeRawContent{}, err
	}
	if item.WorkspaceID != parsed.WorkspaceID || item.RunID != parsed.RunID {
		return RuntimeRawContent{}, common.ErrNotFound
	}
	var payload any
	switch parsed.FileName {
	case "dod-assessment.json":
		payload = item.DoDAssessment
	case "summary-detail.json":
		payload = item.SummaryDetail
	default:
		return RuntimeRawContent{}, common.ErrInvalidArgument
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return RuntimeRawContent{}, err
	}
	return newRuntimeRawContent(parsed, body)
}
