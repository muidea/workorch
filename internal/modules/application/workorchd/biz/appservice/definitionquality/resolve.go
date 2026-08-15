package definitionquality

import (
	"context"
	"errors"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

func ResolveGoal(
	ctx context.Context,
	readByID func(context.Context, common.GoalID) (string, common.GoalID, error),
	readLatest func(context.Context, common.WorkspaceID) (string, common.GoalID, error),
	workspaceID common.WorkspaceID,
	requestedID common.GoalID,
	text string,
) (string, common.GoalID, error) {
	text = strings.TrimSpace(text)
	if text != "" {
		return text, requestedID, nil
	}
	if requestedID != "" {
		text, id, err := readByID(ctx, requestedID)
		if err != nil {
			return "", "", err
		}
		if id == "" {
			return "", "", common.ErrNotFound
		}
		return strings.TrimSpace(text), id, nil
	}
	text, id, err := readLatest(ctx, workspaceID)
	if err != nil {
		if errors.Is(err, common.ErrInvalidArgument) || errors.Is(err, common.ErrConflict) {
			return "", "", nil
		}
		return "", "", err
	}
	if id == "" {
		return "", "", nil
	}
	return strings.TrimSpace(text), id, nil
}
