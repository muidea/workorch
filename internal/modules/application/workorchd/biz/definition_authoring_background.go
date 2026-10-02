package biz

import (
	"context"
	"log/slog"
	"sync"

	"github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/modules/application/workorchd/service/server"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

// authoringTaskLifecycle 只管理进程任务，不保存业务结果或恢复队列。
// 重启后的重试必须重新查询 owner，已 reserve 的请求不能再调用 provider。
type authoringTaskLifecycle struct {
	mu       sync.Mutex
	stopping bool
	active   int
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

func newAuthoringTaskLifecycle() *authoringTaskLifecycle {
	ctx, cancel := context.WithCancel(context.Background())
	return &authoringTaskLifecycle{ctx: ctx, cancel: cancel}
}

func (l *authoringTaskLifecycle) stop() {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.stopping = true
	l.cancel()
	l.mu.Unlock()
	l.wg.Wait()
}

func (l *authoringTaskLifecycle) schedule(ctx context.Context, background task.BackgroundRoutine, run func(context.Context)) error {
	if l == nil || background == nil || run == nil {
		return common.ErrConflict
	}
	l.mu.Lock()
	if l.stopping || l.active >= 32 {
		l.mu.Unlock()
		return common.ErrConflict
	}
	l.active++
	l.wg.Add(1)
	l.mu.Unlock()
	done := func() {
		l.mu.Lock()
		l.active--
		l.mu.Unlock()
		l.wg.Done()
	}
	accepted := &authoringBackgroundTask{ctx: l.ctx, run: run, done: done}
	if err := background.AsyncTaskContext(ctx, accepted); err != nil {
		done()
		return err
	}
	return nil
}

type authoringBackgroundTask struct {
	ctx  context.Context
	run  func(context.Context)
	done func()
}

func (t *authoringBackgroundTask) Run() {
	defer t.done()
	if t.ctx.Err() == nil {
		t.run(t.ctx)
	}
}

func (b *Biz) configureAuthoringBackgroundPorts(ports *server.ApplicationPorts) {
	ports.Definition.CompileDefinitionAuthoring = func(ctx context.Context, id string) (workorchmanagementcontract.DefinitionAuthoringView, error) {
		return b.StartDefinitionAuthoringCompilation(ctx, common.WorkspaceID(id))
	}
}

func (b *Biz) StartDefinitionAuthoringCompilation(ctx context.Context, id common.WorkspaceID) (workorchmanagementcontract.DefinitionAuthoringView, error) {
	view, err := b.ReadDefinitionAuthoringView(ctx, id)
	if err != nil {
		return view, err
	}
	if view.InputHash == "" || !view.TemplateCurrent {
		return view, common.ErrConflict
	}
	switch view.State {
	case "ready_to_confirm", "review_blocked", "failed", "outcome_unresolved", "confirming", "admitting", "admitted", "clarification_required":
		return view, nil
	}
	hash := view.InputHash
	if err := b.authoringTasks.schedule(ctx, b.BackgroundRoutine(), func(taskCtx context.Context) {
		current, err := b.ReadDefinitionAuthoringView(taskCtx, id)
		if err != nil || current.InputHash != hash {
			return
		}
		if _, err := b.DeriveDefinitionAuthoringView(taskCtx, id); err != nil {
			slog.Warn("authoring derivation stopped; durable owner state remains queryable", "workspace", id)
			return
		}
		current, err = b.ReadDefinitionAuthoringView(taskCtx, id)
		if err != nil || current.InputHash != hash {
			return
		}
		if _, err := b.CompileDefinitionAuthoringView(taskCtx, id); err != nil {
			attributes := []any{"workspace", id, "inputHash", hash}
			if observed, readErr := b.ReadDefinitionAuthoringView(taskCtx, id); readErr == nil && observed.InputHash == hash {
				attributes = append(attributes, "state", observed.State)
				for _, request := range observed.Requests {
					if request.State == "failed" {
						attributes = append(attributes, "phase", request.Phase, "permit", request.PermitID, "failureCode", request.FailureCode)
						break
					}
				}
			}
			slog.Warn("authoring compilation stopped; durable owner state remains queryable", attributes...)
		}
	}); err != nil {
		return view, err
	}
	// 仅表示托管任务已接受；不伪造 owner 的 compiling／成功状态。
	view.CompilationAccepted = true
	return view, nil
}
