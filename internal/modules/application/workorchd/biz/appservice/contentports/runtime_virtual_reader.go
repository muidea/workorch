package contentports

import (
	"context"
	"encoding/json"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contentcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (r *Reader) readRuntimeCheckpointRaw(ctx context.Context, parsed contentcore.RuntimeRawRef) (contentcore.RuntimeRawContent, error) {
	if r.eventHub == nil {
		return contentcore.RuntimeRawContent{}, common.ErrNotFound
	}
	item, err := ownerreadports.ReadRuntimeCheckpointByOwnerCommand(ctx, r.eventHub, common.CheckpointID(parsed.ItemPath))
	if err != nil {
		return contentcore.RuntimeRawContent{}, err
	}
	if item.WorkspaceID != parsed.WorkspaceID || item.RunID != parsed.RunID {
		return contentcore.RuntimeRawContent{}, common.ErrNotFound
	}
	content, err := json.MarshalIndent(item.Data, "", "  ")
	if err != nil {
		return contentcore.RuntimeRawContent{}, err
	}
	return contentcore.RuntimeRawContent{Ref: parsed.Ref, FileName: parsed.FileName, ContentType: contentcore.RuntimeRawContentType(parsed.FileName), Content: content}, nil
}

func (r *Reader) readResultRaw(ctx context.Context, parsed contentcore.RuntimeRawRef) (contentcore.RuntimeRawContent, error) {
	if r.eventHub == nil {
		return contentcore.RuntimeRawContent{}, common.ErrNotFound
	}
	item, err := ownerreadports.ReadResultByOwnerCommand(ctx, r.eventHub, common.ResultID(parsed.ItemPath))
	if err != nil {
		return contentcore.RuntimeRawContent{}, err
	}
	if item.WorkspaceID != parsed.WorkspaceID || item.RunID != parsed.RunID {
		return contentcore.RuntimeRawContent{}, common.ErrNotFound
	}
	var payload any
	switch parsed.FileName {
	case "dod-assessment.json":
		payload = item.DoDAssessment
	case "summary-detail.json":
		payload = item.SummaryDetail
	default:
		return contentcore.RuntimeRawContent{}, common.ErrInvalidArgument
	}
	content, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return contentcore.RuntimeRawContent{}, err
	}
	return contentcore.RuntimeRawContent{Ref: parsed.Ref, FileName: parsed.FileName, ContentType: contentcore.RuntimeRawContentType(parsed.FileName), Content: content}, nil
}
