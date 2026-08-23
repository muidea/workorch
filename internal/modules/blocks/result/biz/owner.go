package result

import (
	"context"
	"log/slog"
	"sync"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/result/pkg/common"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type Dependencies struct {
	Results           repository.ResultRepository
	Artifacts         repository.ArtifactRepository
	EventHub          magicevent.Hub
	BackgroundRoutine task.BackgroundRoutine
}

type Biz struct {
	basebiz.Base
	service                    *service
	topics                     []string
	started                    bool
	notificationRecoveryMu     sync.Mutex
	notificationRecoveryCancel context.CancelFunc
}

const (
	resultNotificationRecoveryInterval = time.Minute
	resultNotificationRecoveryMaxPages = 1000
)

func New(h magicevent.Hub, b task.BackgroundRoutine, deps Dependencies) (*Biz, error) {
	if deps.Results == nil || deps.Artifacts == nil || deps.EventHub == nil {
		return nil, common.ErrInvalidArgument
	}
	if h == nil {
		return nil, common.ErrInvalidArgument
	}
	return &Biz{Base: basebiz.New(modulecommon.ModuleID, h, b), service: newService(deps.Results, deps.Artifacts)}, nil
}

func (m *Biz) Start() error {
	if m == nil || m.service == nil || m.EventHub() == nil {
		return common.ErrInvalidArgument
	}
	if m.started {
		return nil
	}
	m.topics = []string{
		resultevents.TopicResultCommandGet,
		resultevents.TopicResultCommandListByWorkspace,
		resultevents.TopicResultCommandListByRun,
		resultevents.TopicResultCommandDelete,
		resultevents.TopicArtifactCommandListByWorkspace,
		resultevents.TopicArtifactCommandListByRun,
		resultevents.TopicArtifactCommandListByResult,
		resultevents.TopicArtifactCommandDelete,
	}
	for _, topic := range m.topics {
		m.SubscribeFunc(topic, m.handleCommand)
	}
	for _, topic := range m.lifecycleTopics() {
		m.SubscribeFunc(topic, m.handleLifecycleCommand)
		m.topics = append(m.topics, topic)
	}
	m.started = true
	if err := m.replayTerminalNotifications(context.Background()); err != nil {
		slog.Warn("workorch Result notification recovery failed", "err", err)
	}
	if m.BackgroundRoutine() != nil {
		recoveryCtx, cancel := context.WithCancel(context.Background())
		m.notificationRecoveryCancel = cancel
		m.Timer(recoveryCtx, resultNotificationRecoveryInterval, 0, func() {
			if err := m.replayTerminalNotifications(recoveryCtx); err != nil && recoveryCtx.Err() == nil {
				slog.Warn("workorch Result notification recovery failed", "err", err)
			}
		})
	}
	return nil
}

func (m *Biz) Teardown(context.Context) {
	if m == nil {
		return
	}
	if m.notificationRecoveryCancel != nil {
		m.notificationRecoveryCancel()
	}
	for idx := len(m.topics) - 1; idx >= 0; idx-- {
		m.UnsubscribeFunc(m.topics[idx])
	}
	m.notificationRecoveryMu.Lock()
	defer m.notificationRecoveryMu.Unlock()
	m.topics = nil
	m.service = nil
	m.started = false
	m.notificationRecoveryCancel = nil
}

func (m *Biz) replayTerminalNotifications(ctx context.Context) error {
	if m == nil {
		return common.ErrInvalidArgument
	}
	m.notificationRecoveryMu.Lock()
	defer m.notificationRecoveryMu.Unlock()
	if !m.started || m.service == nil || ctx.Err() != nil {
		return nil
	}
	inventory, ok := m.service.results.(repository.ResultRecoveryRepository)
	if !ok {
		return nil
	}
	cursor := ""
	seen := map[string]struct{}{}
	for page := 0; page < resultNotificationRecoveryMaxPages; page++ {
		items, next, err := inventory.ListForRecovery(ctx, cursor, resultevents.MaxListLimit)
		if err != nil {
			return err
		}
		for _, item := range items {
			if item == nil || resultevents.ResultNotificationID(item) == "" {
				continue
			}
			switch item.State {
			case common.ResultStateAccepted, common.ResultStateRejected, common.ResultStateSuperseded:
				m.publishResultChanged(ctx, item)
			}
		}
		if next == "" {
			return nil
		}
		if _, exists := seen[next]; exists {
			return common.ErrConflict
		}
		seen[next] = struct{}{}
		cursor = next
	}
	return common.ErrConflict
}

func (m *Biz) handleCommand(ev magicevent.Event, result magicevent.Result) {
	if m == nil || m.service == nil {
		setCommandResult(result, struct{}{}, common.ErrInvalidArgument)
		return
	}
	switch ev.ID() {
	case resultevents.TopicResultCommandGet:
		response, err := m.getResult(ev)
		setCommandResult(result, response, err)
	case resultevents.TopicResultCommandListByWorkspace:
		response, err := m.listResultsByWorkspace(ev)
		setCommandResult(result, response, err)
	case resultevents.TopicResultCommandListByRun:
		response, err := m.listResultsByRun(ev)
		setCommandResult(result, response, err)
	case resultevents.TopicResultCommandDelete:
		response, err := m.deleteResult(ev)
		setCommandResult(result, response, err)
	case resultevents.TopicArtifactCommandListByWorkspace:
		response, err := m.listArtifactsByWorkspace(ev)
		setCommandResult(result, response, err)
	case resultevents.TopicArtifactCommandListByRun:
		response, err := m.listArtifactsByRun(ev)
		setCommandResult(result, response, err)
	case resultevents.TopicArtifactCommandListByResult:
		response, err := m.listArtifactsByResult(ev)
		setCommandResult(result, response, err)
	case resultevents.TopicArtifactCommandDelete:
		response, err := m.deleteArtifact(ev)
		setCommandResult(result, response, err)
	default:
		setCommandResult(result, struct{}{}, common.ErrInvalidArgument)
	}
}

func (m *Biz) getResult(ev magicevent.Event) (resultevents.GetResultResult, error) {
	cmd, ok := ev.Data().(resultevents.GetResultCommand)
	if !ok || cmd.WorkspaceID == "" || cmd.ResultID == "" {
		return resultevents.GetResultResult{}, common.ErrInvalidArgument
	}
	ctx, err := resultCommandContext(ev.Context(), cmd.WorkspaceID)
	if err != nil {
		return resultevents.GetResultResult{}, err
	}
	item, err := m.service.GetResult(ctx, cmd.ResultID)
	if err != nil {
		return resultevents.GetResultResult{}, err
	}
	if item.WorkspaceID != cmd.WorkspaceID {
		return resultevents.GetResultResult{}, common.ErrConflict
	}
	return resultevents.GetResultResult{Result: resultevents.CloneResult(item)}, nil
}

func (m *Biz) listResultsByWorkspace(ev magicevent.Event) (resultevents.ListResultsByWorkspaceResult, error) {
	cmd, ok := ev.Data().(resultevents.ListResultsByWorkspaceCommand)
	if !ok || cmd.WorkspaceID == "" {
		return resultevents.ListResultsByWorkspaceResult{}, common.ErrInvalidArgument
	}
	ctx, err := resultCommandContext(ev.Context(), cmd.WorkspaceID)
	if err != nil {
		return resultevents.ListResultsByWorkspaceResult{}, err
	}
	items, err := m.service.ListResultsByWorkspace(ctx, cmd.WorkspaceID)
	if err != nil {
		return resultevents.ListResultsByWorkspaceResult{}, err
	}
	page, next, err := paginateResults(items, cmd.Cursor, cmd.Limit)
	return resultevents.ListResultsByWorkspaceResult{Results: page, NextCursor: next}, err
}

func (m *Biz) listResultsByRun(ev magicevent.Event) (resultevents.ListResultsByRunResult, error) {
	cmd, ok := ev.Data().(resultevents.ListResultsByRunCommand)
	if !ok || cmd.WorkspaceID == "" || cmd.RunID == "" {
		return resultevents.ListResultsByRunResult{}, common.ErrInvalidArgument
	}
	ctx, err := resultCommandContext(ev.Context(), cmd.WorkspaceID)
	if err != nil {
		return resultevents.ListResultsByRunResult{}, err
	}
	items, err := m.service.ListResultsByRun(ctx, cmd.RunID)
	if err != nil {
		return resultevents.ListResultsByRunResult{}, err
	}
	page, next, err := paginateResults(items, cmd.Cursor, cmd.Limit)
	return resultevents.ListResultsByRunResult{Results: page, NextCursor: next}, err
}

func (m *Biz) deleteResult(ev magicevent.Event) (resultevents.DeleteResultResult, error) {
	cmd, ok := ev.Data().(resultevents.DeleteResultCommand)
	if !ok || cmd.WorkspaceID == "" || cmd.ResultID == "" {
		return resultevents.DeleteResultResult{}, common.ErrInvalidArgument
	}
	ctx, err := resultCommandContext(ev.Context(), cmd.WorkspaceID)
	if err != nil {
		return resultevents.DeleteResultResult{}, err
	}
	if err := m.service.DeleteResult(ctx, cmd.WorkspaceID, cmd.ResultID); err != nil {
		return resultevents.DeleteResultResult{}, err
	}
	return resultevents.DeleteResultResult{Deleted: true}, nil
}

func (m *Biz) listArtifactsByWorkspace(ev magicevent.Event) (resultevents.ListArtifactsByWorkspaceResult, error) {
	cmd, ok := ev.Data().(resultevents.ListArtifactsByWorkspaceCommand)
	if !ok || cmd.WorkspaceID == "" {
		return resultevents.ListArtifactsByWorkspaceResult{}, common.ErrInvalidArgument
	}
	ctx, err := resultCommandContext(ev.Context(), cmd.WorkspaceID)
	if err != nil {
		return resultevents.ListArtifactsByWorkspaceResult{}, err
	}
	items, err := m.service.ListArtifactsByWorkspace(ctx, cmd.WorkspaceID)
	if err != nil {
		return resultevents.ListArtifactsByWorkspaceResult{}, err
	}
	page, next, err := paginateArtifacts(items, cmd.Cursor, cmd.Limit)
	return resultevents.ListArtifactsByWorkspaceResult{Artifacts: page, NextCursor: next}, err
}

func (m *Biz) listArtifactsByRun(ev magicevent.Event) (resultevents.ListArtifactsByRunResult, error) {
	cmd, ok := ev.Data().(resultevents.ListArtifactsByRunCommand)
	if !ok || cmd.WorkspaceID == "" || cmd.RunID == "" {
		return resultevents.ListArtifactsByRunResult{}, common.ErrInvalidArgument
	}
	ctx, err := resultCommandContext(ev.Context(), cmd.WorkspaceID)
	if err != nil {
		return resultevents.ListArtifactsByRunResult{}, err
	}
	items, err := m.service.ListArtifactsByRun(ctx, cmd.RunID)
	if err != nil {
		return resultevents.ListArtifactsByRunResult{}, err
	}
	page, next, err := paginateArtifacts(items, cmd.Cursor, cmd.Limit)
	return resultevents.ListArtifactsByRunResult{Artifacts: page, NextCursor: next}, err
}

func (m *Biz) listArtifactsByResult(ev magicevent.Event) (resultevents.ListArtifactsByResultResult, error) {
	cmd, ok := ev.Data().(resultevents.ListArtifactsByResultCommand)
	if !ok || cmd.WorkspaceID == "" || cmd.ResultID == "" {
		return resultevents.ListArtifactsByResultResult{}, common.ErrInvalidArgument
	}
	ctx, err := resultCommandContext(ev.Context(), cmd.WorkspaceID)
	if err != nil {
		return resultevents.ListArtifactsByResultResult{}, err
	}
	items, err := m.service.ListArtifactsByResult(ctx, cmd.ResultID)
	if err != nil {
		return resultevents.ListArtifactsByResultResult{}, err
	}
	page, next, err := paginateArtifacts(items, cmd.Cursor, cmd.Limit)
	return resultevents.ListArtifactsByResultResult{Artifacts: page, NextCursor: next}, err
}

func (m *Biz) deleteArtifact(ev magicevent.Event) (resultevents.DeleteArtifactResult, error) {
	cmd, ok := ev.Data().(resultevents.DeleteArtifactCommand)
	if !ok || cmd.WorkspaceID == "" || cmd.ArtifactID == "" {
		return resultevents.DeleteArtifactResult{}, common.ErrInvalidArgument
	}
	ctx, err := resultCommandContext(ev.Context(), cmd.WorkspaceID)
	if err != nil {
		return resultevents.DeleteArtifactResult{}, err
	}
	if err := m.service.DeleteArtifact(ctx, cmd.WorkspaceID, cmd.ArtifactID); err != nil {
		return resultevents.DeleteArtifactResult{}, err
	}
	return resultevents.DeleteArtifactResult{Deleted: true}, nil
}

func paginateResults(items []*resultmodel.Result, cursor string, limit int) ([]*resultmodel.Result, string, error) {
	cursor, limit, err := resultevents.NormalizeListWindow(cursor, limit)
	if err != nil {
		return nil, "", err
	}
	start := 0
	for start < len(items) && string(items[start].ID) <= cursor {
		start++
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}
	page := resultevents.CloneResults(items[start:end])
	next := ""
	if end < len(items) && len(page) > 0 {
		next = string(page[len(page)-1].ID)
	}
	return page, next, nil
}

func paginateArtifacts(items []*resultmodel.Artifact, cursor string, limit int) ([]*resultmodel.Artifact, string, error) {
	cursor, limit, err := resultevents.NormalizeListWindow(cursor, limit)
	if err != nil {
		return nil, "", err
	}
	start := 0
	for start < len(items) && string(items[start].ID) <= cursor {
		start++
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}
	page := resultevents.CloneArtifacts(items[start:end])
	next := ""
	if end < len(items) && len(page) > 0 {
		next = string(page[len(page)-1].ID)
	}
	return page, next, nil
}

func (m *Biz) publishResultChanged(ctx context.Context, item *resultmodel.Result) {
	if m == nil || m.EventHub() == nil || item == nil {
		return
	}
	topic := resultNotificationTopic(item.State)
	event := magicevent.NewEventWithContext(topic, ModuleID, resultevents.NotificationDestination, magicevent.NewHeader(), ctx, resultevents.ResultChangedDataFromModel(item))
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(resultevents.Contracts(), topic, eventcontract.LaneScope{
		SubjectType: "result", SubjectID: string(item.ID), WorkspaceID: string(item.WorkspaceID), RunID: string(item.RunID),
	}))
	m.PostEvent(event)
}

func resultNotificationTopic(state common.ResultState) string {
	switch state {
	case common.ResultStateDraft:
		return resultevents.TopicResultCreated
	case common.ResultStateRejected:
		return resultevents.TopicResultRejected
	case common.ResultStateSuperseded:
		return resultevents.TopicResultSuperseded
	default:
		return resultevents.TopicResultAccepted
	}
}

func resultCommandContext(ctx context.Context, workspaceID common.WorkspaceID) (context.Context, error) {
	return common.BindRuntimeWorkspaceContext(ctx, workspaceID)
}

func setCommandResult[Result any](result magicevent.Result, data Result, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(data, nil)
}
