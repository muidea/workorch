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
	"github.com/muidea/workorch/internal/pkg/doditems"
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
	service, repo, eventRecords, err := goalDependenciesFromSession(session)
	if err != nil {
		return Goal{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	text = strings.TrimSpace(text)
	if text == "" {
		return Goal{}, common.ErrInvalidArgument
	}
	existing, err := repo.ListByWorkspaceID(ctx, common.WorkspaceID(workspaceID))
	if err != nil {
		return Goal{}, err
	}
	existing = orderedWorkspaceGoals(ctx, eventRecords, common.WorkspaceID(workspaceID), existing)
	if len(existing) > 0 {
		updatedItem, err := consolidateWorkspaceGoal(ctx, service, repo, common.WorkspaceID(workspaceID), existing, text)
		if err != nil {
			return Goal{}, err
		}
		return mapGoal(updatedItem), nil
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

func UpdateGoalWithSession(ctx context.Context, session *RuntimeSession, goalID string, text string) (ret Goal, err error) {
	service, repo, _, err := goalDependenciesFromSession(session)
	if err != nil {
		return Goal{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	text = strings.TrimSpace(text)
	if goalID == "" || text == "" {
		return Goal{}, common.ErrInvalidArgument
	}
	if err := service.UpdateGoal(ctx, workspacemodule.UpdateGoalCommand{
		GoalID: common.GoalID(goalID),
		Text:   text,
	}); err != nil {
		return Goal{}, err
	}
	item, err := repo.Get(ctx, common.GoalID(goalID))
	if err != nil {
		return Goal{}, err
	}
	return mapGoal(item), nil
}

func UpdateGoalWithBackend(ctx context.Context, backend string, dataRoot string, goalID string, text string) (Goal, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return Goal{}, err
	}
	return UpdateGoalWithSession(ctx, session, goalID, text)
}

func UpdateGoalFromEnv(ctx context.Context, goalID string, text string) (Goal, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return Goal{}, err
	}
	return UpdateGoalWithSession(ctx, session, goalID, text)
}

func ReadGoalWithSession(ctx context.Context, session *RuntimeSession, goalID string) (ret Goal, err error) {
	_, repo, _, err := goalDependenciesFromSession(session)
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
	service, repo, eventRecords, err := goalDependenciesFromSession(session)
	if err != nil {
		return nil, err
	}
	defer session.CloseOnReturn(ctx, &err)

	items, err := repo.ListByWorkspaceID(ctx, common.WorkspaceID(workspaceID))
	if err != nil {
		return nil, err
	}
	items = orderedWorkspaceGoals(ctx, eventRecords, common.WorkspaceID(workspaceID), items)
	if len(items) > 1 {
		item, err := consolidateWorkspaceGoal(ctx, service, repo, common.WorkspaceID(workspaceID), items, "")
		if err != nil {
			return nil, err
		}
		items = []*workspacemodel.Goal{item}
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
	service, repo, eventRecords, err := dodDependenciesFromSession(session)
	if err != nil {
		return DoD{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	items = normalizeDoDItems(items)
	if len(items) == 0 {
		return DoD{}, common.ErrInvalidArgument
	}
	existing, err := repo.ListByWorkspaceID(ctx, common.WorkspaceID(workspaceID))
	if err != nil {
		return DoD{}, err
	}
	existing = orderedWorkspaceDoDs(ctx, eventRecords, common.WorkspaceID(workspaceID), existing)
	if len(existing) > 0 {
		updatedItem, err := consolidateWorkspaceDoD(ctx, service, repo, common.WorkspaceID(workspaceID), existing, items)
		if err != nil {
			return DoD{}, err
		}
		return mapDoD(updatedItem), nil
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

func UpdateDoDWithSession(ctx context.Context, session *RuntimeSession, dodID string, items []string) (ret DoD, err error) {
	service, repo, _, err := dodDependenciesFromSession(session)
	if err != nil {
		return DoD{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	items = normalizeDoDItems(items)
	if dodID == "" || len(items) == 0 {
		return DoD{}, common.ErrInvalidArgument
	}
	if err := service.UpdateDoD(ctx, workspacemodule.UpdateDoDCommand{
		DoDID: common.DoDID(dodID),
		Items: items,
	}); err != nil {
		return DoD{}, err
	}
	item, err := repo.Get(ctx, common.DoDID(dodID))
	if err != nil {
		return DoD{}, err
	}
	return mapDoD(item), nil
}

func UpdateDoDWithBackend(ctx context.Context, backend string, dataRoot string, dodID string, items []string) (DoD, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return DoD{}, err
	}
	return UpdateDoDWithSession(ctx, session, dodID, items)
}

func UpdateDoDFromEnv(ctx context.Context, dodID string, items []string) (DoD, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return DoD{}, err
	}
	return UpdateDoDWithSession(ctx, session, dodID, items)
}

func ReadDoDWithSession(ctx context.Context, session *RuntimeSession, dodID string) (ret DoD, err error) {
	_, repo, _, err := dodDependenciesFromSession(session)
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
	service, repo, eventRecords, err := dodDependenciesFromSession(session)
	if err != nil {
		return nil, err
	}
	defer session.CloseOnReturn(ctx, &err)

	items, err := repo.ListByWorkspaceID(ctx, common.WorkspaceID(workspaceID))
	if err != nil {
		return nil, err
	}
	items = orderedWorkspaceDoDs(ctx, eventRecords, common.WorkspaceID(workspaceID), items)
	if len(items) > 1 {
		item, err := consolidateWorkspaceDoD(ctx, service, repo, common.WorkspaceID(workspaceID), items, nil)
		if err != nil {
			return nil, err
		}
		items = []*workspacemodel.DoD{item}
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

func goalDependenciesFromSession(session *RuntimeSession) (workspacemodule.Service, repository.GoalRepository, repository.EventRecordRepository, error) {
	exports, err := session.Exports()
	if err != nil {
		return nil, nil, nil, err
	}
	if exports.Services.WorkspaceService == nil || exports.Repositories.Goals == nil {
		return nil, nil, nil, common.ErrInvalidArgument
	}
	return exports.Services.WorkspaceService, exports.Repositories.Goals, exports.Repositories.EventRecords, nil
}

func dodDependenciesFromSession(session *RuntimeSession) (workspacemodule.Service, repository.DoDRepository, repository.EventRecordRepository, error) {
	exports, err := session.Exports()
	if err != nil {
		return nil, nil, nil, err
	}
	if exports.Services.WorkspaceService == nil || exports.Repositories.DoDs == nil {
		return nil, nil, nil, common.ErrInvalidArgument
	}
	return exports.Services.WorkspaceService, exports.Repositories.DoDs, exports.Repositories.EventRecords, nil
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

func mapGoals(items []*workspacemodel.Goal) []Goal {
	ret := make([]Goal, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, mapGoal(item))
	}
	return ret
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

func mapDoDs(items []*workspacemodel.DoD) []DoD {
	ret := make([]DoD, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, mapDoD(item))
	}
	return ret
}

func mergedGoalText(existing []*workspacemodel.Goal, appended string) string {
	seen := map[string]struct{}{}
	parts := make([]string, 0, len(existing)+1)
	for _, item := range existing {
		if item == nil {
			continue
		}
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		if _, ok := seen[text]; ok {
			continue
		}
		seen[text] = struct{}{}
		parts = append(parts, text)
	}
	appended = strings.TrimSpace(appended)
	if appended != "" {
		if _, ok := seen[appended]; !ok {
			parts = append(parts, appended)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func consolidateWorkspaceGoal(
	ctx context.Context,
	service workspacemodule.Service,
	repo repository.GoalRepository,
	workspaceID common.WorkspaceID,
	existing []*workspacemodel.Goal,
	appended string,
) (*workspacemodel.Goal, error) {
	canonical := latestWorkspaceGoal(existing)
	if canonical == nil {
		return nil, common.ErrInvalidArgument
	}
	merged := mergedGoalText(existing, appended)
	if merged == "" {
		return nil, common.ErrInvalidArgument
	}
	if err := service.UpdateGoal(ctx, workspacemodule.UpdateGoalCommand{
		GoalID: canonical.ID,
		Text:   merged,
	}); err != nil {
		return nil, err
	}
	for _, item := range existing {
		if item == nil || item.ID == canonical.ID || item.WorkspaceID != workspaceID {
			continue
		}
		if err := repo.Delete(ctx, item.ID); err != nil && !errors.Is(err, common.ErrNotFound) {
			return nil, err
		}
	}
	return repo.Get(ctx, canonical.ID)
}

func normalizeDoDItems(items []string) []string {
	return doditems.Normalize(items)
}

func mergedDoDItems(existing []*workspacemodel.DoD, appended []string) []string {
	seen := map[string]struct{}{}
	ret := make([]string, 0, len(appended))
	for _, item := range existing {
		if item == nil {
			continue
		}
		for _, entry := range normalizeDoDItems(item.Items) {
			if _, ok := seen[entry]; ok {
				continue
			}
			seen[entry] = struct{}{}
			ret = append(ret, entry)
		}
	}
	for _, entry := range normalizeDoDItems(appended) {
		if _, ok := seen[entry]; ok {
			continue
		}
		seen[entry] = struct{}{}
		ret = append(ret, entry)
	}
	return ret
}

func consolidateWorkspaceDoD(
	ctx context.Context,
	service workspacemodule.Service,
	repo repository.DoDRepository,
	workspaceID common.WorkspaceID,
	existing []*workspacemodel.DoD,
	appended []string,
) (*workspacemodel.DoD, error) {
	canonical := latestWorkspaceDoD(existing)
	if canonical == nil {
		return nil, common.ErrInvalidArgument
	}
	merged := mergedDoDItems(existing, appended)
	if len(merged) == 0 {
		return nil, common.ErrInvalidArgument
	}
	if err := service.UpdateDoD(ctx, workspacemodule.UpdateDoDCommand{
		DoDID: canonical.ID,
		Items: merged,
	}); err != nil {
		return nil, err
	}
	for _, item := range existing {
		if item == nil || item.ID == canonical.ID || item.WorkspaceID != workspaceID {
			continue
		}
		if err := repo.Delete(ctx, item.ID); err != nil && !errors.Is(err, common.ErrNotFound) {
			return nil, err
		}
	}
	return repo.Get(ctx, canonical.ID)
}

func generateBusinessID(prefix string) string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return prefix + "-" + hex.EncodeToString(raw[:])
	}
	return fmt.Sprintf("%s-%d", prefix, time.Now().UTC().UnixNano())
}
