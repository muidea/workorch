package definitionquality

import (
	"context"
	"errors"

	"github.com/muidea/workorch/internal/pkg/common"
)

func ResolveDoD(
	ctx context.Context,
	readByID func(context.Context, common.DoDID) ([]string, common.DoDID, error),
	readLatest func(context.Context, common.WorkspaceID) ([]string, common.DoDID, error),
	workspaceID common.WorkspaceID,
	requestedID common.DoDID,
	items []string,
) ([]string, common.DoDID, error) {
	items = NormalizeDoDItems(items)
	if len(items) > 0 {
		return items, requestedID, nil
	}
	if requestedID != "" {
		items, id, err := readByID(ctx, requestedID)
		if err != nil {
			return nil, "", err
		}
		if id == "" {
			return nil, "", common.ErrNotFound
		}
		return NormalizeDoDItems(items), id, nil
	}
	items, id, err := readLatest(ctx, workspaceID)
	if err != nil {
		if errors.Is(err, common.ErrInvalidArgument) || errors.Is(err, common.ErrConflict) {
			return nil, "", nil
		}
		return nil, "", err
	}
	if id == "" {
		return nil, "", nil
	}
	return NormalizeDoDItems(items), id, nil
}
