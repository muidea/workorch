package appservice

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	workspacemodule "github.com/muidea/workorch/internal/modules/kernel/workspace"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type Goal struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspaceId"`
	Text        string `json:"text"`
}

type DoD struct {
	ID          string   `json:"id"`
	WorkspaceID string   `json:"workspaceId"`
	Items       []string `json:"items"`
	State       string   `json:"state"`
}

func DefineGoalWithSession(ctx context.Context, session *RuntimeSession, workspaceID string, text string) (ret Goal, err error) {
	service, repo, err := goalDependenciesFromSession(session)
	if err != nil {
		return Goal{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	text = strings.TrimSpace(text)
	if text == "" {
		return Goal{}, common.ErrInvalidArgument
	}
	for attempt := 0; attempt < 8; attempt++ {
		id := common.GoalID(generateBusinessID("goal"))
		if err := service.DefineGoal(ctx, workspacemodule.DefineGoalCommand{
			GoalID:      id,
			WorkspaceID: common.WorkspaceID(workspaceID),
			Text:        text,
		}); err != nil {
			if errors.Is(err, common.ErrConflict) {
				continue
			}
			return Goal{}, err
		}
		item, err := repo.Get(ctx, id)
		if err != nil {
			return Goal{}, err
		}
		return mapGoal(item), nil
	}
	return Goal{}, common.ErrConflict
}

func DefineGoalWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string, text string) (Goal, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return Goal{}, err
	}
	return DefineGoalWithSession(ctx, session, workspaceID, text)
}

func DefineGoalFromEnv(ctx context.Context, workspaceID string, text string) (Goal, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return Goal{}, err
	}
	return DefineGoalWithSession(ctx, session, workspaceID, text)
}

func ReadGoalWithSession(ctx context.Context, session *RuntimeSession, goalID string) (ret Goal, err error) {
	_, repo, err := goalDependenciesFromSession(session)
	if err != nil {
		return Goal{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	item, err := repo.Get(ctx, common.GoalID(goalID))
	if err != nil {
		return Goal{}, err
	}
	return mapGoal(item), nil
}

func ReadGoalWithBackend(ctx context.Context, backend string, dataRoot string, goalID string) (Goal, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return Goal{}, err
	}
	return ReadGoalWithSession(ctx, session, goalID)
}

func ListGoalsWithSession(ctx context.Context, session *RuntimeSession, workspaceID string) (ret []Goal, err error) {
	_, repo, err := goalDependenciesFromSession(session)
	if err != nil {
		return nil, err
	}
	defer session.CloseOnReturn(ctx, &err)

	items, err := repo.ListByWorkspaceID(ctx, common.WorkspaceID(workspaceID))
	if err != nil {
		return nil, err
	}
	ret = make([]Goal, 0, len(items))
	for _, item := range items {
		ret = append(ret, mapGoal(item))
	}
	return ret, nil
}

func ListGoalsWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string) ([]Goal, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return nil, err
	}
	return ListGoalsWithSession(ctx, session, workspaceID)
}

func DefineDoDWithSession(ctx context.Context, session *RuntimeSession, workspaceID string, items []string) (ret DoD, err error) {
	service, repo, err := dodDependenciesFromSession(session)
	if err != nil {
		return DoD{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	items = normalizeDoDItems(items)
	if len(items) == 0 {
		return DoD{}, common.ErrInvalidArgument
	}
	for attempt := 0; attempt < 8; attempt++ {
		id := common.DoDID(generateBusinessID("dod"))
		if err := service.DefineDoD(ctx, workspacemodule.DefineDoDCommand{
			DoDID:       id,
			WorkspaceID: common.WorkspaceID(workspaceID),
			Items:       items,
		}); err != nil {
			if errors.Is(err, common.ErrConflict) {
				continue
			}
			return DoD{}, err
		}
		item, err := repo.Get(ctx, id)
		if err != nil {
			return DoD{}, err
		}
		return mapDoD(item), nil
	}
	return DoD{}, common.ErrConflict
}

func DefineDoDWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string, items []string) (DoD, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return DoD{}, err
	}
	return DefineDoDWithSession(ctx, session, workspaceID, items)
}

func DefineDoDFromEnv(ctx context.Context, workspaceID string, items []string) (DoD, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return DoD{}, err
	}
	return DefineDoDWithSession(ctx, session, workspaceID, items)
}

func ReadDoDWithSession(ctx context.Context, session *RuntimeSession, dodID string) (ret DoD, err error) {
	_, repo, err := dodDependenciesFromSession(session)
	if err != nil {
		return DoD{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	item, err := repo.Get(ctx, common.DoDID(dodID))
	if err != nil {
		return DoD{}, err
	}
	return mapDoD(item), nil
}

func ReadDoDWithBackend(ctx context.Context, backend string, dataRoot string, dodID string) (DoD, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return DoD{}, err
	}
	return ReadDoDWithSession(ctx, session, dodID)
}

func ListDoDsWithSession(ctx context.Context, session *RuntimeSession, workspaceID string) (ret []DoD, err error) {
	_, repo, err := dodDependenciesFromSession(session)
	if err != nil {
		return nil, err
	}
	defer session.CloseOnReturn(ctx, &err)

	items, err := repo.ListByWorkspaceID(ctx, common.WorkspaceID(workspaceID))
	if err != nil {
		return nil, err
	}
	ret = make([]DoD, 0, len(items))
	for _, item := range items {
		ret = append(ret, mapDoD(item))
	}
	return ret, nil
}

func ListDoDsWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string) ([]DoD, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return nil, err
	}
	return ListDoDsWithSession(ctx, session, workspaceID)
}

func goalDependenciesFromSession(session *RuntimeSession) (workspacemodule.Service, repository.GoalRepository, error) {
	exports, err := session.Exports()
	if err != nil {
		return nil, nil, err
	}
	if exports.Services.WorkspaceService == nil || exports.Repositories.Goals == nil {
		return nil, nil, common.ErrInvalidArgument
	}
	return exports.Services.WorkspaceService, exports.Repositories.Goals, nil
}

func dodDependenciesFromSession(session *RuntimeSession) (workspacemodule.Service, repository.DoDRepository, error) {
	exports, err := session.Exports()
	if err != nil {
		return nil, nil, err
	}
	if exports.Services.WorkspaceService == nil || exports.Repositories.DoDs == nil {
		return nil, nil, common.ErrInvalidArgument
	}
	return exports.Services.WorkspaceService, exports.Repositories.DoDs, nil
}

func mapGoal(item *workspacemodel.Goal) Goal {
	if item == nil {
		return Goal{}
	}
	return Goal{
		ID:          string(item.ID),
		WorkspaceID: string(item.WorkspaceID),
		Text:        item.Text,
	}
}

func mapDoD(item *workspacemodel.DoD) DoD {
	if item == nil {
		return DoD{}
	}
	return DoD{
		ID:          string(item.ID),
		WorkspaceID: string(item.WorkspaceID),
		Items:       append([]string(nil), item.Items...),
		State:       string(item.State),
	}
}

func normalizeDoDItems(items []string) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		ret = append(ret, item)
	}
	return ret
}

func generateBusinessID(prefix string) string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return prefix + "-" + hex.EncodeToString(raw[:])
	}
	return fmt.Sprintf("%s-%d", prefix, time.Now().UTC().UnixNano())
}
