package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"sync"

	magicevent "github.com/muidea/magicCommon/event"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/common"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspaceModel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type goalDoDManager struct {
	workspaces   repository.WorkspaceRepository
	goals        repository.GoalRepository
	dods         repository.DoDRepository
	publish      func(magicevent.Event)
	definitionMu sync.Mutex
}

func (m *goalDoDManager) requireMutableWorkspace(ctx context.Context, workspaceID common.WorkspaceID) error {
	if m == nil || m.workspaces == nil || workspaceID == "" {
		return common.ErrInvalidArgument
	}
	item, err := m.workspaces.Get(ctx, workspaceID)
	if err != nil {
		return err
	}
	if err := workspaceDeletionRejectsMutation(item); err != nil {
		return err
	}
	if item.DefinitionAuthoring != nil {
		return fmt.Errorf("%w: edit the authoring draft instead of formal definitions", common.ErrConflict)
	}
	return nil
}

func newGoalDoDManager(workspaces repository.WorkspaceRepository, goals repository.GoalRepository, dods repository.DoDRepository) *goalDoDManager {
	return &goalDoDManager{workspaces: workspaces, goals: goals, dods: dods}
}

func newGoalDoDManagerWithPublisher(workspaces repository.WorkspaceRepository, goals repository.GoalRepository, dods repository.DoDRepository, publish func(magicevent.Event)) *goalDoDManager {
	return &goalDoDManager{workspaces: workspaces, goals: goals, dods: dods, publish: publish}
}

func (m *goalDoDManager) CreateWorkspace(ctx context.Context, cmd workspaceevents.CreateWorkspaceCommand) error {
	if cmd.WorkspaceID == "" || strings.TrimSpace(cmd.Title) == "" || strings.TrimSpace(cmd.DeliverableDirectory) == "" {
		return common.ErrInvalidArgument
	}
	workspace, err := m.submitWorkspaceCreationInternalEvent(ctx, workspaceCreationInternalEvent{
		Kind: workspaceCreationInternalEventRequested,
		Requested: &workspaceCreationRequestedFact{
			WorkspaceID: cmd.WorkspaceID, Title: cmd.Title, Description: cmd.Description,
			DeliverableDirectory: cmd.DeliverableDirectory, InternalDirectory: cmd.InternalDirectory,
		},
	})
	if err != nil {
		return err
	}
	return m.publishWorkspaceEvent(ctx, workspaceevents.TopicWorkspaceCreated, workspace)
}

func (m *goalDoDManager) DefineGoal(ctx context.Context, cmd workspaceevents.DefineGoalCommand) error {
	if cmd.GoalID == "" || cmd.WorkspaceID == "" || strings.TrimSpace(cmd.Text) == "" {
		return common.ErrInvalidArgument
	}
	var err error
	ctx, err = common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
	if err != nil {
		return err
	}
	m.definitionMu.Lock()
	defer m.definitionMu.Unlock()
	goal, err := m.defineGoal(ctx, cmd)
	if err != nil {
		return err
	}
	return m.publishGoalEvent(ctx, goal)
}

func (m *goalDoDManager) defineGoal(ctx context.Context, cmd workspaceevents.DefineGoalCommand) (*workspaceModel.Goal, error) {
	if err := m.requireMutableWorkspace(ctx, cmd.WorkspaceID); err != nil {
		return nil, err
	}
	items, err := m.goals.ListByWorkspaceID(ctx, cmd.WorkspaceID)
	if err != nil {
		return nil, err
	}
	previous, sequence, err := nextGoalDefinition(items, cmd.WorkspaceID)
	if err != nil {
		return nil, err
	}
	goal := &workspaceModel.Goal{
		ID:                 cmd.GoalID,
		WorkspaceID:        cmd.WorkspaceID,
		Text:               cmd.Text,
		DefinitionRevision: definitionRevisionForText("goal", sequence, cmd.Text),
		LineageState:       "active",
	}
	if previous != nil {
		goal.PreviousGoalID = previous.ID
		goal.PreviousDefinitionHash = definitionHashForText(previous.Text)
	}
	if err := m.goals.Create(ctx, goal); err != nil {
		return nil, err
	}
	return goal, nil
}

func (m *goalDoDManager) UpdateGoal(ctx context.Context, cmd workspaceevents.UpdateGoalCommand) error {
	if cmd.WorkspaceID == "" || cmd.GoalID == "" || strings.TrimSpace(cmd.Text) == "" {
		return common.ErrInvalidArgument
	}
	var err error
	ctx, err = common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
	if err != nil {
		return err
	}
	m.definitionMu.Lock()
	defer m.definitionMu.Unlock()
	goal, changed, err := m.updateGoal(ctx, cmd)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	return m.publishGoalEvent(ctx, goal)
}

func (m *goalDoDManager) updateGoal(ctx context.Context, cmd workspaceevents.UpdateGoalCommand) (*workspaceModel.Goal, bool, error) {
	if err := m.requireMutableWorkspace(ctx, cmd.WorkspaceID); err != nil {
		return nil, false, err
	}
	items, err := m.goals.ListByWorkspaceID(ctx, cmd.WorkspaceID)
	if err != nil {
		return nil, false, err
	}
	current, sequence, err := currentGoalDefinition(items, cmd.WorkspaceID)
	if err != nil {
		return nil, false, err
	}
	if current.ID != cmd.GoalID {
		return nil, false, fmt.Errorf("%w: goal %s is not the current canonical definition", common.ErrConflict, cmd.GoalID)
	}
	if strings.TrimSpace(current.LineageState) != "active" {
		return nil, false, fmt.Errorf("%w: goal definition lineage is outside the current data contract", common.ErrConflict)
	}
	if definitionHashForText(current.Text) == definitionHashForText(cmd.Text) {
		return current, false, nil
	}
	goal := workspaceModel.CloneGoal(current)
	goal.PreviousDefinitionHash = definitionHashForText(goal.Text)
	goal.Text = cmd.Text
	goal.DefinitionRevision = definitionRevisionForText("goal", sequence, cmd.Text)
	if err := m.goals.Update(ctx, goal); err != nil {
		return nil, false, err
	}
	return goal, true, nil
}

func (m *goalDoDManager) DefineDoD(ctx context.Context, cmd workspaceevents.DefineDoDCommand) error {
	if cmd.DoDID == "" || cmd.WorkspaceID == "" || len(cmd.Items) == 0 {
		return common.ErrInvalidArgument
	}
	for _, item := range cmd.Items {
		if strings.TrimSpace(item) == "" {
			return common.ErrInvalidArgument
		}
	}
	var err error
	ctx, err = common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
	if err != nil {
		return err
	}
	m.definitionMu.Lock()
	defer m.definitionMu.Unlock()
	dod, err := m.defineDoD(ctx, cmd)
	if err != nil {
		return err
	}
	return m.publishDoDEvent(ctx, dod)
}

func (m *goalDoDManager) defineDoD(ctx context.Context, cmd workspaceevents.DefineDoDCommand) (*workspaceModel.DoD, error) {
	if err := m.requireMutableWorkspace(ctx, cmd.WorkspaceID); err != nil {
		return nil, err
	}
	items, err := m.dods.ListByWorkspaceID(ctx, cmd.WorkspaceID)
	if err != nil {
		return nil, err
	}
	previous, sequence, err := nextDoDDefinition(items, cmd.WorkspaceID)
	if err != nil {
		return nil, err
	}
	dod := &workspaceModel.DoD{
		ID:                 cmd.DoDID,
		WorkspaceID:        cmd.WorkspaceID,
		Items:              append([]string(nil), cmd.Items...),
		State:              common.DoDStateReady,
		DefinitionRevision: definitionRevisionForItems("dod", sequence, cmd.Items),
		LineageState:       "active",
	}
	if previous != nil {
		dod.PreviousDoDID = previous.ID
		dod.PreviousDefinitionHash = definitionHashForItems(previous.Items)
	}
	if err := m.dods.Create(ctx, dod); err != nil {
		return nil, err
	}
	return dod, nil
}

func (m *goalDoDManager) UpdateDoD(ctx context.Context, cmd workspaceevents.UpdateDoDCommand) error {
	if cmd.WorkspaceID == "" || cmd.DoDID == "" || len(cmd.Items) == 0 {
		return common.ErrInvalidArgument
	}
	for _, item := range cmd.Items {
		if strings.TrimSpace(item) == "" {
			return common.ErrInvalidArgument
		}
	}
	var err error
	ctx, err = common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
	if err != nil {
		return err
	}
	m.definitionMu.Lock()
	defer m.definitionMu.Unlock()
	dod, changed, err := m.updateDoD(ctx, cmd)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	return m.publishDoDEvent(ctx, dod)
}

func (m *goalDoDManager) updateDoD(ctx context.Context, cmd workspaceevents.UpdateDoDCommand) (*workspaceModel.DoD, bool, error) {
	if err := m.requireMutableWorkspace(ctx, cmd.WorkspaceID); err != nil {
		return nil, false, err
	}
	items, err := m.dods.ListByWorkspaceID(ctx, cmd.WorkspaceID)
	if err != nil {
		return nil, false, err
	}
	current, sequence, err := currentDoDDefinition(items, cmd.WorkspaceID)
	if err != nil {
		return nil, false, err
	}
	if current.ID != cmd.DoDID {
		return nil, false, fmt.Errorf("%w: DoD %s is not the current canonical definition", common.ErrConflict, cmd.DoDID)
	}
	if strings.TrimSpace(current.LineageState) != "active" {
		return nil, false, fmt.Errorf("%w: DoD definition lineage is outside the current data contract", common.ErrConflict)
	}
	if definitionHashForItems(current.Items) == definitionHashForItems(cmd.Items) && current.State == common.DoDStateReady {
		return current, false, nil
	}
	dod := workspaceModel.CloneDoD(current)
	dod.PreviousDefinitionHash = definitionHashForItems(dod.Items)
	dod.Items = append([]string(nil), cmd.Items...)
	dod.State = common.DoDStateReady
	dod.DefinitionRevision = definitionRevisionForItems("dod", sequence, cmd.Items)
	if err := m.dods.Update(ctx, dod); err != nil {
		return nil, false, err
	}
	return dod, true, nil
}

func (m *goalDoDManager) DeleteGoal(ctx context.Context, cmd workspaceevents.DeleteGoalCommand) error {
	if cmd.WorkspaceID == "" || cmd.GoalID == "" {
		return common.ErrInvalidArgument
	}
	var err error
	ctx, err = common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
	if err != nil {
		return err
	}
	m.definitionMu.Lock()
	defer m.definitionMu.Unlock()
	if err := m.requireMutableWorkspace(ctx, cmd.WorkspaceID); err != nil {
		return err
	}
	items, err := m.goals.ListByWorkspaceID(ctx, cmd.WorkspaceID)
	if err != nil {
		return err
	}
	current, _, err := currentGoalDefinition(items, cmd.WorkspaceID)
	if err != nil {
		return err
	}
	if current.ID == cmd.GoalID {
		return fmt.Errorf("%w: current canonical goal cannot be deleted", common.ErrConflict)
	}
	if !containsGoal(items, cmd.WorkspaceID, cmd.GoalID) {
		return common.ErrNotFound
	}
	return m.goals.Delete(ctx, cmd.GoalID)
}

func (m *goalDoDManager) DeleteDoD(ctx context.Context, cmd workspaceevents.DeleteDoDCommand) error {
	if cmd.WorkspaceID == "" || cmd.DoDID == "" {
		return common.ErrInvalidArgument
	}
	var err error
	ctx, err = common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
	if err != nil {
		return err
	}
	m.definitionMu.Lock()
	defer m.definitionMu.Unlock()
	if err := m.requireMutableWorkspace(ctx, cmd.WorkspaceID); err != nil {
		return err
	}
	items, err := m.dods.ListByWorkspaceID(ctx, cmd.WorkspaceID)
	if err != nil {
		return err
	}
	current, _, err := currentDoDDefinition(items, cmd.WorkspaceID)
	if err != nil {
		return err
	}
	if current.ID == cmd.DoDID {
		return fmt.Errorf("%w: current canonical DoD cannot be deleted", common.ErrConflict)
	}
	if !containsDoD(items, cmd.WorkspaceID, cmd.DoDID) {
		return common.ErrNotFound
	}
	return m.dods.Delete(ctx, cmd.DoDID)
}

func containsGoal(items []*workspaceModel.Goal, workspaceID common.WorkspaceID, goalID common.GoalID) bool {
	for _, item := range items {
		if item != nil && item.WorkspaceID == workspaceID && item.ID == goalID {
			return true
		}
	}
	return false
}

func containsDoD(items []*workspaceModel.DoD, workspaceID common.WorkspaceID, doDID common.DoDID) bool {
	for _, item := range items {
		if item != nil && item.WorkspaceID == workspaceID && item.ID == doDID {
			return true
		}
	}
	return false
}

func (m *goalDoDManager) publishWorkspaceEvent(ctx context.Context, topic string, workspace *workspaceModel.Workspace) error {
	if m.publish == nil || workspace == nil {
		return nil
	}
	data := workspaceChangedNotification(workspace)
	event := magicevent.NewEventWithContext(topic, modulecommon.ModuleID, workspaceevents.NotificationDestination, magicevent.NewHeader(), ctx, data)
	event.BindLaneKey(events.EventHubLaneKeyFor(workspaceevents.Contracts(), topic, eventcontract.LaneScope{SubjectType: "workspace", SubjectID: string(workspace.ID), WorkspaceID: string(workspace.ID)}))
	m.publish(event)
	return nil
}

func (m *goalDoDManager) publishWorkspaceDeleted(ctx context.Context, workspace *workspaceModel.Workspace) error {
	if m.publish == nil || workspace == nil || workspace.Deletion == nil || !workspace.Deletion.Completed {
		return common.ErrInvalidArgument
	}
	deletion := workspace.Deletion
	data := workspaceevents.WorkspaceDeletedData{
		WorkspaceID:       workspace.ID,
		DeletionID:        deletion.DeletionID,
		RequestID:         deletion.RequestID,
		Mode:              deletion.Mode,
		Disposition:       workspaceevents.DeleteWorkspaceCompleted,
		DeletionRevision:  deletion.Revision,
		WorkspaceRevision: workspace.Revision,
		CompletedAt:       deletion.CompletedAt.UTC(),
	}
	if err := data.Validate(); err != nil {
		return err
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceDeleted, modulecommon.ModuleID, workspaceevents.NotificationDestination, magicevent.NewHeader(), ctx, data)
	event.BindLaneKey(events.EventHubLaneKeyFor(workspaceevents.Contracts(), workspaceevents.TopicWorkspaceDeleted, eventcontract.LaneScope{SubjectType: "workspace", SubjectID: string(workspace.ID), WorkspaceID: string(workspace.ID)}))
	m.publish(event)
	return nil
}

func (m *goalDoDManager) publishGoalEvent(ctx context.Context, goal *workspaceModel.Goal) error {
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

func nextGoalDefinition(items []*workspaceModel.Goal, workspaceID common.WorkspaceID) (*workspaceModel.Goal, uint64, error) {
	sequence, count, err := nextDefinitionSequence("goal", len(items), func(index int) (common.WorkspaceID, string) {
		if items[index] == nil {
			return "", ""
		}
		return items[index].WorkspaceID, items[index].DefinitionRevision
	}, workspaceID)
	if err != nil {
		return nil, 0, err
	}
	if count == 0 {
		return nil, sequence, nil
	}
	current := currentGoalLeaf(items, workspaceID)
	if current == nil {
		return nil, 0, fmt.Errorf("%w: goal definition lineage is unavailable or ambiguous", common.ErrConflict)
	}
	return current, sequence, nil
}

func currentGoalDefinition(items []*workspaceModel.Goal, workspaceID common.WorkspaceID) (*workspaceModel.Goal, uint64, error) {
	current, sequence, err := nextGoalDefinition(items, workspaceID)
	if err != nil {
		return nil, 0, err
	}
	if current == nil {
		return nil, 0, common.ErrNotFound
	}
	return current, sequence, nil
}

func nextDoDDefinition(items []*workspaceModel.DoD, workspaceID common.WorkspaceID) (*workspaceModel.DoD, uint64, error) {
	sequence, count, err := nextDefinitionSequence("dod", len(items), func(index int) (common.WorkspaceID, string) {
		if items[index] == nil {
			return "", ""
		}
		return items[index].WorkspaceID, items[index].DefinitionRevision
	}, workspaceID)
	if err != nil {
		return nil, 0, err
	}
	if count == 0 {
		return nil, sequence, nil
	}
	current := currentDoDLeaf(items, workspaceID)
	if current == nil {
		return nil, 0, fmt.Errorf("%w: DoD definition lineage is unavailable or ambiguous", common.ErrConflict)
	}
	return current, sequence, nil
}

func currentDoDDefinition(items []*workspaceModel.DoD, workspaceID common.WorkspaceID) (*workspaceModel.DoD, uint64, error) {
	current, sequence, err := nextDoDDefinition(items, workspaceID)
	if err != nil {
		return nil, 0, err
	}
	if current == nil {
		return nil, 0, common.ErrNotFound
	}
	return current, sequence, nil
}

func nextDefinitionSequence(prefix string, itemCount int, itemAt func(int) (common.WorkspaceID, string), workspaceID common.WorkspaceID) (uint64, int, error) {
	var maximum uint64
	count := 0
	for index := 0; index < itemCount; index++ {
		itemWorkspaceID, revision := itemAt(index)
		if itemWorkspaceID != workspaceID {
			continue
		}
		count++
		sequence, ok := definitionRevisionSequence(prefix, revision)
		if !ok {
			return 0, count, fmt.Errorf("%w: %s definition revision is outside the current data contract", common.ErrConflict, prefix)
		}
		if sequence > maximum {
			maximum = sequence
		}
	}
	if maximum == ^uint64(0) {
		return 0, count, fmt.Errorf("%w: %s definition revision exhausted", common.ErrConflict, prefix)
	}
	return maximum + 1, count, nil
}

func definitionRevisionSequence(prefix, revision string) (uint64, bool) {
	revision = strings.TrimSpace(revision)
	parts := strings.SplitN(revision, ":", 3)
	if len(parts) != 3 || parts[0] != strings.TrimSpace(prefix) || parts[1] == "" || parts[2] == "" {
		return 0, false
	}
	sequence, err := strconv.ParseUint(parts[1], 10, 64)
	return sequence, err == nil && sequence > 0
}

func definitionRevisionForText(prefix string, sequence uint64, text string) string {
	hash := definitionHashForText(text)
	if sequence == 0 || hash == "" {
		return ""
	}
	return fmt.Sprintf("%s:%020d:%s", prefix, sequence, hash)
}

func definitionHashForText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func definitionRevisionForItems(prefix string, sequence uint64, items []string) string {
	hash := definitionHashForItems(items)
	if sequence == 0 || hash == "" {
		return ""
	}
	return fmt.Sprintf("%s:%020d:%s", prefix, sequence, hash)
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

func (m *goalDoDManager) publishDoDEvent(ctx context.Context, dod *workspaceModel.DoD) error {
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
