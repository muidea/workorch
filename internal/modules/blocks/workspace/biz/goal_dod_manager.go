package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/common"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspaceModel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type GoalDoDManager struct {
	workspaces repository.WorkspaceRepository
	goals      repository.GoalRepository
	dods       repository.DoDRepository
	publish    func(magicevent.Event)
}

func NewGoalDoDManager(workspaces repository.WorkspaceRepository, goals repository.GoalRepository, dods repository.DoDRepository) *GoalDoDManager {
	return &GoalDoDManager{workspaces: workspaces, goals: goals, dods: dods}
}

func NewGoalDoDManagerWithPublisher(workspaces repository.WorkspaceRepository, goals repository.GoalRepository, dods repository.DoDRepository, publish func(magicevent.Event)) *GoalDoDManager {
	return &GoalDoDManager{workspaces: workspaces, goals: goals, dods: dods, publish: publish}
}

func (m *GoalDoDManager) CreateWorkspace(ctx context.Context, cmd workspaceevents.CreateWorkspaceCommand) error {
	if cmd.WorkspaceID == "" || strings.TrimSpace(cmd.Title) == "" || strings.TrimSpace(cmd.DeliverableDirectory) == "" {
		return common.ErrInvalidArgument
	}

	workspace := &workspaceModel.Workspace{
		ID:                   cmd.WorkspaceID,
		Title:                cmd.Title,
		Description:          cmd.Description,
		DeliverableDirectory: cmd.DeliverableDirectory,
		InternalDirectory:    cmd.InternalDirectory,
		State:                common.WorkspaceStateActive,
	}
	if err := m.workspaces.Create(ctx, workspace); err != nil {
		return err
	}
	return m.publishWorkspaceEvent(ctx, workspaceevents.TopicWorkspaceCreated, workspace)
}

func (m *GoalDoDManager) DeleteWorkspace(ctx context.Context, id common.WorkspaceID) error {
	if id == "" {
		return common.ErrInvalidArgument
	}
	workspace, err := m.workspaces.Get(ctx, id)
	if err != nil {
		return err
	}
	goals, err := m.goals.ListByWorkspaceID(ctx, id)
	if err != nil {
		return err
	}
	dods, err := m.dods.ListByWorkspaceID(ctx, id)
	if err != nil {
		return err
	}
	for _, goal := range goals {
		if goal == nil {
			continue
		}
		if err := m.goals.Delete(ctx, goal.ID); err != nil {
			return err
		}
	}
	for _, dod := range dods {
		if dod == nil {
			continue
		}
		if err := m.dods.Delete(ctx, dod.ID); err != nil {
			return err
		}
	}
	if err := m.workspaces.Delete(ctx, id); err != nil {
		return err
	}
	return m.publishWorkspaceEvent(ctx, workspaceevents.TopicWorkspaceDeleted, workspace)
}

func (m *GoalDoDManager) DefineGoal(ctx context.Context, cmd workspaceevents.DefineGoalCommand) error {
	if cmd.GoalID == "" || cmd.WorkspaceID == "" || strings.TrimSpace(cmd.Text) == "" {
		return common.ErrInvalidArgument
	}
	_, err := m.workspaces.Get(ctx, cmd.WorkspaceID)
	if err != nil {
		return err
	}
	goal := &workspaceModel.Goal{
		ID:                 cmd.GoalID,
		WorkspaceID:        cmd.WorkspaceID,
		Text:               cmd.Text,
		DefinitionRevision: definitionRevisionForText("goal", cmd.Text),
		LineageState:       "active",
	}
	m.attachGoalLineage(ctx, goal)
	if err := m.goals.Create(ctx, goal); err != nil {
		return err
	}
	return m.publishGoalEvent(ctx, goal)
}

func (m *GoalDoDManager) UpdateGoal(ctx context.Context, cmd workspaceevents.UpdateGoalCommand) error {
	if cmd.GoalID == "" || strings.TrimSpace(cmd.Text) == "" {
		return common.ErrInvalidArgument
	}
	goal, err := m.goals.Get(ctx, cmd.GoalID)
	if err != nil {
		return err
	}
	_, err = m.workspaces.Get(ctx, goal.WorkspaceID)
	if err != nil {
		return err
	}
	goal.PreviousDefinitionHash = definitionHashForText(goal.Text)
	goal.Text = cmd.Text
	goal.DefinitionRevision = definitionRevisionForText("goal", cmd.Text)
	if strings.TrimSpace(goal.LineageState) == "" {
		goal.LineageState = "active"
	}
	if err := m.goals.Update(ctx, goal); err != nil {
		return err
	}
	return m.publishGoalEvent(ctx, goal)
}

func (m *GoalDoDManager) DefineDoD(ctx context.Context, cmd workspaceevents.DefineDoDCommand) error {
	if cmd.DoDID == "" || cmd.WorkspaceID == "" || len(cmd.Items) == 0 {
		return common.ErrInvalidArgument
	}
	for _, item := range cmd.Items {
		if strings.TrimSpace(item) == "" {
			return common.ErrInvalidArgument
		}
	}
	_, err := m.workspaces.Get(ctx, cmd.WorkspaceID)
	if err != nil {
		return err
	}
	dod := &workspaceModel.DoD{
		ID:                 cmd.DoDID,
		WorkspaceID:        cmd.WorkspaceID,
		Items:              append([]string(nil), cmd.Items...),
		State:              common.DoDStateReady,
		DefinitionRevision: definitionRevisionForItems("dod", cmd.Items),
		LineageState:       "active",
	}
	m.attachDoDLineage(ctx, dod)
	if err := m.dods.Create(ctx, dod); err != nil {
		return err
	}
	return m.publishDoDEvent(ctx, dod)
}

func (m *GoalDoDManager) UpdateDoD(ctx context.Context, cmd workspaceevents.UpdateDoDCommand) error {
	if cmd.DoDID == "" || len(cmd.Items) == 0 {
		return common.ErrInvalidArgument
	}
	for _, item := range cmd.Items {
		if strings.TrimSpace(item) == "" {
			return common.ErrInvalidArgument
		}
	}
	dod, err := m.dods.Get(ctx, cmd.DoDID)
	if err != nil {
		return err
	}
	_, err = m.workspaces.Get(ctx, dod.WorkspaceID)
	if err != nil {
		return err
	}
	dod.PreviousDefinitionHash = definitionHashForItems(dod.Items)
	dod.Items = append([]string(nil), cmd.Items...)
	dod.State = common.DoDStateReady
	dod.DefinitionRevision = definitionRevisionForItems("dod", cmd.Items)
	if strings.TrimSpace(dod.LineageState) == "" {
		dod.LineageState = "active"
	}
	if err := m.dods.Update(ctx, dod); err != nil {
		return err
	}
	return m.publishDoDEvent(ctx, dod)
}

func (m *GoalDoDManager) publishWorkspaceEvent(ctx context.Context, topic string, workspace *workspaceModel.Workspace) error {
	if m.publish == nil || workspace == nil {
		return nil
	}
	data := workspaceevents.WorkspaceChangedData{
		WorkspaceID:          workspace.ID,
		Title:                workspace.Title,
		Description:          workspace.Description,
		DeliverableDirectory: workspace.DeliverableDirectory,
		InternalDirectory:    workspace.InternalDirectory,
		State:                workspace.State,
		ActiveRunID:          workspace.ActiveRunID,
	}
	event := magicevent.NewEventWithContext(topic, modulecommon.ModuleID, workspaceevents.NotificationDestination, magicevent.NewHeader(), ctx, data)
	event.BindLaneKey(events.EventHubLaneKeyFor(workspaceevents.Contracts(), topic, eventcontract.LaneScope{SubjectType: "workspace", SubjectID: string(workspace.ID), WorkspaceID: string(workspace.ID)}))
	m.publish(event)
	return nil
}

func (m *GoalDoDManager) publishGoalEvent(ctx context.Context, goal *workspaceModel.Goal) error {
	if m.publish == nil {
		return nil
	}
	data := workspaceevents.GoalDefinedData{
		GoalID:                 goal.ID,
		WorkspaceID:            goal.WorkspaceID,
		Text:                   goal.Text,
		DefinitionRevision:     goal.DefinitionRevision,
		PreviousDefinitionHash: goal.PreviousDefinitionHash,
		PreviousGoalID:         goal.PreviousGoalID,
		LineageState:           goal.LineageState,
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicGoalDefined, modulecommon.ModuleID, workspaceevents.NotificationDestination, magicevent.NewHeader(), ctx, data)
	event.BindLaneKey(events.EventHubLaneKeyFor(workspaceevents.Contracts(), workspaceevents.TopicGoalDefined, eventcontract.LaneScope{SubjectType: "goal", SubjectID: string(goal.ID), WorkspaceID: string(goal.WorkspaceID)}))
	m.publish(event)
	return nil
}

func (m *GoalDoDManager) attachGoalLineage(ctx context.Context, goal *workspaceModel.Goal) {
	if m == nil || m.goals == nil || goal == nil {
		return
	}
	items, err := m.goals.ListByWorkspaceID(ctx, goal.WorkspaceID)
	if err != nil || len(items) == 0 {
		return
	}
	sort.SliceStable(items, func(i, j int) bool {
		return string(items[i].ID) < string(items[j].ID)
	})
	previous := items[len(items)-1]
	if previous == nil {
		return
	}
	goal.PreviousGoalID = previous.ID
	goal.PreviousDefinitionHash = definitionHashForText(previous.Text)
}

func (m *GoalDoDManager) attachDoDLineage(ctx context.Context, dod *workspaceModel.DoD) {
	if m == nil || m.dods == nil || dod == nil {
		return
	}
	items, err := m.dods.ListByWorkspaceID(ctx, dod.WorkspaceID)
	if err != nil || len(items) == 0 {
		return
	}
	sort.SliceStable(items, func(i, j int) bool {
		return string(items[i].ID) < string(items[j].ID)
	})
	previous := items[len(items)-1]
	if previous == nil {
		return
	}
	dod.PreviousDoDID = previous.ID
	dod.PreviousDefinitionHash = definitionHashForItems(previous.Items)
}

func definitionRevisionForText(prefix string, text string) string {
	hash := definitionHashForText(text)
	if hash == "" {
		return ""
	}
	return fmt.Sprintf("%s:%s", prefix, hash)
}

func definitionHashForText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func definitionRevisionForItems(prefix string, items []string) string {
	hash := definitionHashForItems(items)
	if hash == "" {
		return ""
	}
	return fmt.Sprintf("%s:%s", prefix, hash)
}

func definitionHashForItems(items []string) string {
	normalized := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			normalized = append(normalized, item)
		}
	}
	if len(normalized) == 0 {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.Join(normalized, "\n")))
	return hex.EncodeToString(sum[:])
}

func (m *GoalDoDManager) publishDoDEvent(ctx context.Context, dod *workspaceModel.DoD) error {
	if m.publish == nil {
		return nil
	}
	data := workspaceevents.DoDDefinedData{
		DoDID:                  dod.ID,
		WorkspaceID:            dod.WorkspaceID,
		Items:                  append([]string(nil), dod.Items...),
		State:                  dod.State,
		DefinitionRevision:     dod.DefinitionRevision,
		PreviousDefinitionHash: dod.PreviousDefinitionHash,
		PreviousDoDID:          dod.PreviousDoDID,
		LineageState:           dod.LineageState,
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicDoDDefined, modulecommon.ModuleID, workspaceevents.NotificationDestination, magicevent.NewHeader(), ctx, data)
	event.BindLaneKey(events.EventHubLaneKeyFor(workspaceevents.Contracts(), workspaceevents.TopicDoDDefined, eventcontract.LaneScope{SubjectType: "dod", SubjectID: string(dod.ID), WorkspaceID: string(dod.WorkspaceID)}))
	m.publish(event)
	return nil
}
