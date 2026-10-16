package content

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func (r *Reader) ReadRuntimeRaw(ctx context.Context, ref string) (RuntimeRawContent, error) {
	if r == nil || r.eventHub == nil {
		return RuntimeRawContent{}, common.ErrNotFound
	}
	parsed, err := ParseRuntimeRawRef(ref)
	if err != nil {
		return RuntimeRawContent{}, fmt.Errorf("parse runtime payload ref: %w", err)
	}
	readCtx, err := common.BindRuntimeWorkspaceContext(ctx, parsed.WorkspaceID)
	if err != nil {
		return RuntimeRawContent{}, err
	}
	switch parsed.Collection {
	case "steps":
		return r.readStepPayload(readCtx, parsed)
	case "tool-calls":
		return r.readToolCallPayload(readCtx, parsed)
	case "runtime-checkpoints":
		return r.readRuntimeCheckpointRaw(readCtx, parsed)
	case "results":
		return r.readResultRaw(readCtx, parsed)
	case "validation-output":
		return r.readRuntimeValidationOutput(readCtx, parsed)
	default:
		return RuntimeRawContent{}, common.ErrInvalidArgument
	}
}

func (r *Reader) readStepPayload(ctx context.Context, parsed RuntimeRawRef) (RuntimeRawContent, error) {
	item, err := ownerreadports.ReadStepByOwnerCommand(ctx, r.eventHub, common.StepID(parsed.ItemID))
	if err != nil {
		return RuntimeRawContent{}, err
	}
	if item == nil || item.RunID != parsed.RunID {
		return RuntimeRawContent{}, common.ErrNotFound
	}
	payload := item.Output
	if parsed.FileName == "input.json" {
		payload = item.Input
	} else if parsed.FileName != "output.json" {
		return RuntimeRawContent{}, common.ErrInvalidArgument
	}
	return runtimePayloadContent(parsed, payload)
}

func (r *Reader) readToolCallPayload(ctx context.Context, parsed RuntimeRawRef) (RuntimeRawContent, error) {
	items, err := ownerreadports.ListToolCallsByRunOwnerCommand(ctx, r.eventHub, parsed.RunID)
	if err != nil {
		return RuntimeRawContent{}, fmt.Errorf("read tool-call payload owner list: %w", err)
	}
	var item *runtimeevidencemodel.ToolCall
	for _, candidate := range items {
		if candidate != nil && candidate.ID == common.ToolCallID(parsed.ItemID) && candidate.WorkspaceID == parsed.WorkspaceID && candidate.RunID == parsed.RunID {
			item = candidate
			break
		}
	}
	if item == nil {
		return RuntimeRawContent{}, common.ErrNotFound
	}
	payload := item.Output
	if parsed.FileName == "arguments.json" {
		payload = item.Arguments
	} else if parsed.FileName != "output.json" {
		return RuntimeRawContent{}, common.ErrInvalidArgument
	}
	return runtimePayloadContent(parsed, payload)
}

func runtimePayloadContent(parsed RuntimeRawRef, payload workorcheventpayload.Payload) (RuntimeRawContent, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return RuntimeRawContent{}, err
	}
	return newRuntimeRawContent(parsed, body)
}
