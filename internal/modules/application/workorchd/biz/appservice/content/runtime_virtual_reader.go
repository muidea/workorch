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
	readCtx := common.ContextWithRuntimeWorkspaceID(ctx, parsed.WorkspaceID)
	item, err := ownerreadports.ReadRuntimeCheckpointByOwnerCommand(readCtx, r.eventHub, common.CheckpointID(parsed.ItemPath))
	if err != nil {
		return RuntimeRawContent{}, err
	}
	if item.WorkspaceID != parsed.WorkspaceID || item.RunID != parsed.RunID {
		return RuntimeRawContent{}, common.ErrNotFound
	}
	body, err := json.MarshalIndent(item.Data, "", "  ")
	if err != nil {
		return RuntimeRawContent{}, err
	}
	return RuntimeRawContent{Ref: parsed.Ref, FileName: parsed.FileName, ContentType: RuntimeRawContentType(parsed.FileName), Content: body}, nil
}

func (r *Reader) readResultRaw(ctx context.Context, parsed RuntimeRawRef) (RuntimeRawContent, error) {
	if r.eventHub == nil {
		return RuntimeRawContent{}, common.ErrNotFound
	}
	readCtx := common.ContextWithRuntimeWorkspaceID(ctx, parsed.WorkspaceID)
	item, err := ownerreadports.ReadResultByOwnerCommand(readCtx, r.eventHub, common.ResultID(parsed.ItemPath))
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
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return RuntimeRawContent{}, err
	}
	return RuntimeRawContent{Ref: parsed.Ref, FileName: parsed.FileName, ContentType: RuntimeRawContentType(parsed.FileName), Content: body}, nil
}
