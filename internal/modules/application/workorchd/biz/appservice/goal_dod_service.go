package appservice

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/doditems"
	"github.com/muidea/workorch/internal/pkg/events"
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
	eventRecords, hub, err := goalDependenciesFromSession(session)
	if err != nil {
		return Goal{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	text = strings.TrimSpace(text)
	if text == "" {
		return Goal{}, common.ErrInvalidArgument
	}
	text = formatGoalDefinitionText(text)
	if text == "" {
		return Goal{}, common.ErrInvalidArgument
	}
	existing, err := listGoalsByWorkspaceOwnerCommand(ctx, hub, common.WorkspaceID(workspaceID))
	if err != nil {
		return Goal{}, err
	}
	existing = orderedWorkspaceGoals(ctx, eventRecords, common.WorkspaceID(workspaceID), existing)
	if len(existing) > 0 {
		updatedItem, err := consolidateWorkspaceGoal(ctx, hub, common.WorkspaceID(workspaceID), existing, text)
		if err != nil {
			return Goal{}, err
		}
		return mapGoal(updatedItem), nil
	}
	for attempt := 0; attempt < 8; attempt++ {
		id := common.GoalID(generateBusinessID("goal"))
		response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicWorkspaceCommandDefineGoal, "goal", string(id), common.WorkspaceID(workspaceID), "", map[string]any{
			events.PayloadKeyGoalID:      string(id),
			events.PayloadKeyWorkspaceID: workspaceID,
			events.PayloadKeyText:        text,
		})
		if err != nil {
			if errors.Is(err, common.ErrConflict) {
				continue
			}
			return Goal{}, err
		}
		item, ok := response.Payload[events.PayloadKeyGoal].(*workspacemodel.Goal)
		if !ok || item == nil {
			return Goal{}, common.ErrInvalidArgument
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
	_, hub, err := goalDependenciesFromSession(session)
	if err != nil {
		return Goal{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	text = strings.TrimSpace(text)
	if goalID == "" || text == "" {
		return Goal{}, common.ErrInvalidArgument
	}
	text = formatGoalDefinitionText(text)
	if text == "" {
		return Goal{}, common.ErrInvalidArgument
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicWorkspaceCommandUpdateGoal, "goal", goalID, "", "", map[string]any{
		events.PayloadKeyGoalID: goalID,
		events.PayloadKeyText:   text,
	})
	if err != nil {
		return Goal{}, err
	}
	item, ok := response.Payload[events.PayloadKeyGoal].(*workspacemodel.Goal)
	if !ok || item == nil {
		return Goal{}, common.ErrInvalidArgument
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
	_, hub, err := goalDependenciesFromSession(session)
	if err != nil {
		return Goal{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	item, err := readGoalByOwnerCommand(ctx, hub, common.GoalID(goalID))
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
	eventRecords, hub, err := goalDependenciesFromSession(session)
	if err != nil {
		return nil, err
	}
	defer session.CloseOnReturn(ctx, &err)

	items, err := listGoalsByWorkspaceOwnerCommand(ctx, hub, common.WorkspaceID(workspaceID))
	if err != nil {
		return nil, err
	}
	items = orderedWorkspaceGoals(ctx, eventRecords, common.WorkspaceID(workspaceID), items)
	if len(items) > 1 {
		item, err := consolidateWorkspaceGoal(ctx, hub, common.WorkspaceID(workspaceID), items, "")
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
	eventRecords, hub, err := dodDependenciesFromSession(session)
	if err != nil {
		return DoD{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	items = normalizeDoDItems(items)
	if len(items) == 0 {
		return DoD{}, common.ErrInvalidArgument
	}
	existing, err := listDoDsByWorkspaceOwnerCommand(ctx, hub, common.WorkspaceID(workspaceID))
	if err != nil {
		return DoD{}, err
	}
	existing = orderedWorkspaceDoDs(ctx, eventRecords, common.WorkspaceID(workspaceID), existing)
	if len(existing) > 0 {
		updatedItem, err := consolidateWorkspaceDoD(ctx, hub, common.WorkspaceID(workspaceID), existing, items)
		if err != nil {
			return DoD{}, err
		}
		return mapDoD(updatedItem), nil
	}
	for attempt := 0; attempt < 8; attempt++ {
		id := common.DoDID(generateBusinessID("dod"))
		response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicWorkspaceCommandDefineDoD, "dod", string(id), common.WorkspaceID(workspaceID), "", map[string]any{
			events.PayloadKeyDoDID:       string(id),
			events.PayloadKeyWorkspaceID: workspaceID,
			events.PayloadKeyItems:       append([]string(nil), items...),
		})
		if err != nil {
			if errors.Is(err, common.ErrConflict) {
				continue
			}
			return DoD{}, err
		}
		item, ok := response.Payload[events.PayloadKeyDoD].(*workspacemodel.DoD)
		if !ok || item == nil {
			return DoD{}, common.ErrInvalidArgument
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
	_, hub, err := dodDependenciesFromSession(session)
	if err != nil {
		return DoD{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	items = normalizeDoDItems(items)
	if dodID == "" || len(items) == 0 {
		return DoD{}, common.ErrInvalidArgument
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicWorkspaceCommandUpdateDoD, "dod", dodID, "", "", map[string]any{
		events.PayloadKeyDoDID: dodID,
		events.PayloadKeyItems: append([]string(nil), items...),
	})
	if err != nil {
		return DoD{}, err
	}
	item, ok := response.Payload[events.PayloadKeyDoD].(*workspacemodel.DoD)
	if !ok || item == nil {
		return DoD{}, common.ErrInvalidArgument
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
	_, hub, err := dodDependenciesFromSession(session)
	if err != nil {
		return DoD{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	item, err := readDoDByOwnerCommand(ctx, hub, common.DoDID(dodID))
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
	eventRecords, hub, err := dodDependenciesFromSession(session)
	if err != nil {
		return nil, err
	}
	defer session.CloseOnReturn(ctx, &err)

	items, err := listDoDsByWorkspaceOwnerCommand(ctx, hub, common.WorkspaceID(workspaceID))
	if err != nil {
		return nil, err
	}
	items = orderedWorkspaceDoDs(ctx, eventRecords, common.WorkspaceID(workspaceID), items)
	if len(items) > 1 {
		item, err := consolidateWorkspaceDoD(ctx, hub, common.WorkspaceID(workspaceID), items, nil)
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

func goalDependenciesFromSession(session *RuntimeSession) (readModelEventRecordReader, events.Hub, error) {
	exports, err := session.Exports()
	if err != nil {
		return nil, nil, err
	}
	if exports.Infrastructure.EventHub == nil {
		return nil, nil, common.ErrInvalidArgument
	}
	return eventHubReadModelEventRecordReader{hub: exports.Infrastructure.EventHub}, exports.Infrastructure.EventHub, nil
}

func dodDependenciesFromSession(session *RuntimeSession) (readModelEventRecordReader, events.Hub, error) {
	exports, err := session.Exports()
	if err != nil {
		return nil, nil, err
	}
	if exports.Infrastructure.EventHub == nil {
		return nil, nil, common.ErrInvalidArgument
	}
	return eventHubReadModelEventRecordReader{hub: exports.Infrastructure.EventHub}, exports.Infrastructure.EventHub, nil
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
		text := formatGoalDefinitionText(item.Text)
		if text == "" {
			continue
		}
		if _, ok := seen[text]; ok {
			continue
		}
		seen[text] = struct{}{}
		parts = append(parts, text)
	}
	for _, entry := range normalizeGoalTextAdditionEntries(appended) {
		if _, ok := seen[entry]; !ok {
			seen[entry] = struct{}{}
			parts = append(parts, entry)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func formatGoalDefinitionText(text string) string {
	entries := normalizeGoalTextEntries(text)
	return strings.TrimSpace(strings.Join(entries, "\n"))
}

func normalizeGoalTextAdditionEntries(text string) []string {
	return normalizeGoalTextEntries(text)
}

func normalizeGoalTextEntries(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	text = normalizeGoalNewlines(text)
	lines := strings.Split(text, "\n")
	hasStructure := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "+ ") || strings.HasPrefix(trimmed, "• ") {
			hasStructure = true
			break
		}
	}
	if hasStructure {
		if formatted := formatStructuredGoalMarkdown(lines); formatted != "" {
			return []string{formatted}
		}
		return nil
	}
	parts := make([]string, 0, len(lines))
	for _, line := range lines {
		for _, part := range strings.FieldsFunc(line, func(r rune) bool {
			return r == ';' || r == '；'
		}) {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		return nil
	}
	ret := make([]string, 0, len(parts))
	for _, part := range parts {
		ret = append(ret, "- "+part)
	}
	return ret
}

func normalizeGoalNewlines(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func formatStructuredGoalMarkdown(lines []string) string {
	ret := make([]string, 0, len(lines))
	blank := false
	for _, line := range lines {
		trimmedRight := strings.TrimRight(line, " \t")
		trimmed := strings.TrimSpace(trimmedRight)
		if trimmed == "" {
			if len(ret) > 0 {
				blank = true
			}
			continue
		}
		if blank {
			ret = append(ret, "")
			blank = false
		}
		ret = append(ret, trimmedRight)
	}
	return strings.TrimSpace(strings.Join(ret, "\n"))
}

func consolidateWorkspaceGoal(
	ctx context.Context,
	hub events.Hub,
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
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicWorkspaceCommandUpdateGoal, "goal", string(canonical.ID), workspaceID, "", map[string]any{
		events.PayloadKeyGoalID: string(canonical.ID),
		events.PayloadKeyText:   merged,
	})
	if err != nil {
		return nil, err
	}
	updated, ok := response.Payload[events.PayloadKeyGoal].(*workspacemodel.Goal)
	if !ok || updated == nil {
		return nil, common.ErrInvalidArgument
	}
	for _, item := range existing {
		if item == nil || item.ID == canonical.ID || item.WorkspaceID != workspaceID {
			continue
		}
		if err := publishOwnerCommand(ctx, hub, events.TopicWorkspaceCommandDeleteGoal, "goal", string(item.ID), workspaceID, "", map[string]any{
			events.PayloadKeyGoalID: string(item.ID),
		}); err != nil && !errors.Is(err, common.ErrNotFound) {
			return nil, err
		}
	}
	return updated, nil
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
	hub events.Hub,
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
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicWorkspaceCommandUpdateDoD, "dod", string(canonical.ID), workspaceID, "", map[string]any{
		events.PayloadKeyDoDID: string(canonical.ID),
		events.PayloadKeyItems: append([]string(nil), merged...),
	})
	if err != nil {
		return nil, err
	}
	updated, ok := response.Payload[events.PayloadKeyDoD].(*workspacemodel.DoD)
	if !ok || updated == nil {
		return nil, common.ErrInvalidArgument
	}
	for _, item := range existing {
		if item == nil || item.ID == canonical.ID || item.WorkspaceID != workspaceID {
			continue
		}
		if err := publishOwnerCommand(ctx, hub, events.TopicWorkspaceCommandDeleteDoD, "dod", string(item.ID), workspaceID, "", map[string]any{
			events.PayloadKeyDoDID: string(item.ID),
		}); err != nil && !errors.Is(err, common.ErrNotFound) {
			return nil, err
		}
	}
	return updated, nil
}

func generateBusinessID(prefix string) string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return prefix + "-" + hex.EncodeToString(raw[:])
	}
	return fmt.Sprintf("%s-%d", prefix, time.Now().UTC().UnixNano())
}
