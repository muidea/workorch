package result

import (
	"context"
	"errors"
	"log/slog"
	"strings"
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
	OwnerRepositories repository.ResultOwnerRepositories
	EventHub          magicevent.Hub
	BackgroundRoutine task.BackgroundRoutine
}

type Biz struct {
	basebiz.Base
	service                    *service
	topics                     []string
	started                    bool
	startComplete              bool
	lifecycle                  basebiz.Lifecycle
	shutdownMu                 sync.Mutex
	releasing                  bool
	released                   bool
	notificationRecoveryMu     sync.Mutex
	notificationRecoveryCancel context.CancelFunc
	workspaceDeletions         repository.ResultWorkspaceDeletionRepository
	workspaceLocksMu           sync.Mutex
	workspaceLocks             map[common.WorkspaceID]*sync.Mutex
}

const (
	resultNotificationRecoveryInterval = time.Minute
	resultNotificationRecoveryMaxPages = 1000
)

func New(h magicevent.Hub, b task.BackgroundRoutine, deps Dependencies) (*Biz, error) {
	if deps.EventHub == nil || h == nil {
		return nil, common.ErrInvalidArgument
	}
	if err := deps.OwnerRepositories.Validate(); err != nil {
		return nil, err
	}
	return &Biz{
		Base: basebiz.New(modulecommon.ModuleID, h, b), service: newService(deps.OwnerRepositories.Results(), deps.OwnerRepositories.Artifacts()),
		workspaceDeletions: deps.OwnerRepositories.WorkspaceDeletions(), workspaceLocks: map[common.WorkspaceID]*sync.Mutex{},
	}, nil
}

func (m *Biz) Start() error {
	if m == nil || m.service == nil || m.EventHub() == nil {
		return common.ErrInvalidArgument
	}
	leave, err := m.enterOwner(false)
	if err != nil {
		return err
	}
	defer leave()
	if m.startComplete {
		return nil
	}
	if len(m.topics) != 0 {
		return common.ErrConflict
	}
	topics := []string{
		resultevents.TopicResultCommandGet,
		resultevents.TopicResultCommandListByWorkspace,
		resultevents.TopicResultCommandListByRun,
		resultevents.TopicArtifactCommandListByWorkspace,
		resultevents.TopicArtifactCommandListByRun,
		resultevents.TopicArtifactCommandListByResult,
		resultevents.TopicArtifactCommandReadContent,
	}
	for _, topic := range topics {
		if err := m.subscribeOwner(topic, m.handleCommand); err != nil {
			return err
		}
	}
	for _, topic := range m.lifecycleTopics() {
		if err := m.subscribeOwner(topic, m.handleLifecycleCommand); err != nil {
			return err
		}
	}
	m.started = true
	if err := m.replayTerminalNotifications(context.Background()); err != nil {
		slog.Warn("workorch Result notification recovery failed", "err", err)
	}
	if m.BackgroundRoutine() != nil {
		recoveryCtx, cancel := context.WithCancel(context.Background())
		m.shutdownMu.Lock()
		m.notificationRecoveryCancel = cancel
		leaveSchedule, scheduleErr := m.lifecycle.Enter()
		m.shutdownMu.Unlock()
		if scheduleErr != nil {
			cancel()
			return scheduleErr
		}
		defer leaveSchedule()
		if err := m.Timer(recoveryCtx, resultNotificationRecoveryInterval, 0, func() {
			leave, err := m.enterOwner(false)
			if err != nil {
				return
			}
			defer leave()
			if err := m.replayTerminalNotifications(recoveryCtx); err != nil && recoveryCtx.Err() == nil {
				slog.Warn("workorch Result notification recovery failed", "err", err)
			}
		}); err != nil {
			// 注册失败不等于已接纳 callback 完成；保留部分启动账本供清理。
			return err
		}
	}
	m.startComplete = true
	return nil
}

func (m *Biz) Teardown(ctx context.Context) error {
	if m == nil {
		return nil
	}
	m.BeginShutdown(ctx)
	if err := m.Quiesce(ctx); err != nil {
		return err
	}
	m.shutdownMu.Lock()
	if m.releasing {
		m.shutdownMu.Unlock()
		return common.ErrConflict
	}
	m.releasing = true
	m.shutdownMu.Unlock()
	defer func() { m.shutdownMu.Lock(); m.releasing = false; m.shutdownMu.Unlock() }()
	if err := m.Quiesce(ctx); err != nil {
		return err
	}
	for len(m.topics) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		last := len(m.topics) - 1
		if err := m.UnsubscribeFuncChecked(m.topics[last]); err != nil {
			return err
		}
		m.topics = m.topics[:last]
	}
	m.service = nil
	m.started = false
	m.startComplete = false
	m.workspaceDeletions = nil
	m.workspaceLocks = nil
	m.shutdownMu.Lock()
	m.notificationRecoveryCancel = nil
	m.released = true
	m.shutdownMu.Unlock()
	return nil
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
			unlock := m.lockWorkspaceMutation(item.WorkspaceID)
			fenceErr := m.workspaceDeletions.WithWorkspaceActiveFence(ctx, item.WorkspaceID, func(context.Context) error {
				switch item.State {
				case common.ResultStateAccepted, common.ResultStateRejected, common.ResultStateSuperseded:
					m.publishResultChanged(ctx, item)
				}
				return nil
			})
			unlock()
			if fenceErr != nil {
				if errors.Is(fenceErr, common.ErrNotFound) {
					continue
				}
				return fenceErr
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
	workspaceID, ok := resultQueryWorkspaceID(ev)
	if !ok {
		setCommandResult(result, struct{}{}, common.ErrInvalidArgument)
		return
	}
	unlock := m.lockWorkspaceMutation(workspaceID)
	defer unlock()
	if err := m.rejectFencedWorkspace(ev.Context(), workspaceID); err != nil {
		setCommandResult(result, struct{}{}, err)
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
	case resultevents.TopicArtifactCommandListByWorkspace:
		response, err := m.listArtifactsByWorkspace(ev)
		setCommandResult(result, response, err)
	case resultevents.TopicArtifactCommandListByRun:
		response, err := m.listArtifactsByRun(ev)
		setCommandResult(result, response, err)
	case resultevents.TopicArtifactCommandListByResult:
		response, err := m.listArtifactsByResult(ev)
		setCommandResult(result, response, err)
	case resultevents.TopicArtifactCommandReadContent:
		response, err := m.readArtifactContent(ev)
		setCommandResult(result, response, err)
	default:
		setCommandResult(result, struct{}{}, common.ErrInvalidArgument)
	}
}

func resultQueryWorkspaceID(ev magicevent.Event) (common.WorkspaceID, bool) {
	if ev == nil {
		return "", false
	}
	switch command := ev.Data().(type) {
	case resultevents.GetResultCommand:
		return command.WorkspaceID, command.WorkspaceID != ""
	case resultevents.ListResultsByWorkspaceCommand:
		return command.WorkspaceID, command.WorkspaceID != ""
	case resultevents.ListResultsByRunCommand:
		return command.WorkspaceID, command.WorkspaceID != ""
	case resultevents.ListArtifactsByWorkspaceCommand:
		return command.WorkspaceID, command.WorkspaceID != ""
	case resultevents.ListArtifactsByRunCommand:
		return command.WorkspaceID, command.WorkspaceID != ""
	case resultevents.ListArtifactsByResultCommand:
		return command.WorkspaceID, command.WorkspaceID != ""
	case resultevents.ReadArtifactContentCommand:
		return command.WorkspaceID, command.WorkspaceID != ""
	default:
		return "", false
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

func (m *Biz) readArtifactContent(ev magicevent.Event) (resultevents.ReadArtifactContentResult, error) {
	cmd, ok := ev.Data().(resultevents.ReadArtifactContentCommand)
	if !ok || cmd.WorkspaceID == "" || cmd.RunID == "" || strings.TrimSpace(cmd.URI) == "" {
		return resultevents.ReadArtifactContentResult{}, common.ErrInvalidArgument
	}
	ctx, err := resultCommandContext(ev.Context(), cmd.WorkspaceID)
	if err != nil {
		return resultevents.ReadArtifactContentResult{}, err
	}
	return m.service.ReadArtifactContent(ctx, cmd.WorkspaceID, cmd.RunID, cmd.URI)
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
