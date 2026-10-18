package biz

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/modules/application/workorchd/service/server"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

// authoringTaskLifecycle 只管理进程任务，不保存业务结果或恢复队列。
// 重启后的重试必须重新查询 owner，已 reserve 的请求不能再调用 provider。
type authoringTaskLifecycle struct {
	mu          sync.Mutex
	stopping    bool
	active      int
	ctx         context.Context
	cancel      context.CancelFunc
	drained     chan struct{}
	drainCtx    context.Context
	drainCancel context.CancelFunc
	drainTimer  *time.Timer
	stopCaller  func() bool
}

func newAuthoringTaskLifecycle() *authoringTaskLifecycle {
	ctx, cancel := context.WithCancel(context.Background())
	drainCtx, drainCancel := context.WithCancel(context.Background())
	return &authoringTaskLifecycle{ctx: ctx, cancel: cancel, drained: make(chan struct{}), drainCtx: drainCtx, drainCancel: drainCancel}
}

const authoringSettlementTimeout = 5 * time.Second

func (l *authoringTaskLifecycle) stop() {
	ctx, cancel := context.WithTimeout(context.Background(), authoringSettlementTimeout)
	defer cancel()
	l.stopContext(ctx)
}

func (l *authoringTaskLifecycle) stopContext(ctx context.Context) error {
	if l == nil {
		return nil
	}
	l.beginStop(ctx)
	select {
	case <-l.drained:
		return nil
	default:
	}
	select {
	case <-l.drained:
		return nil
	case <-l.drainCtx.Done():
		// finishDrain 同时取消排空 root；取消分支被选中时仍须核对真实退出。
		select {
		case <-l.drained:
			return nil
		default:
			return l.drainCtx.Err()
		}
	}
}

// 等待真实退出使用本次停机预算；已封存的 settlement 窗口不会被重试延长。
// settlement root 取消只是停止工作的请求，不能替代 active 归零的事实。
func (l *authoringTaskLifecycle) waitContext(ctx context.Context) error {
	if l == nil {
		return nil
	}
	if ctx == nil {
		return common.ErrInvalidArgument
	}
	select {
	case <-l.drained:
		return nil
	default:
	}
	select {
	case <-l.drained:
		return nil
	case <-ctx.Done():
		select {
		case <-l.drained:
			return nil
		default:
			return ctx.Err()
		}
	}
}

// 首次 Stop 封存唯一排空窗口；重复 Stop 不延长 deadline。
func (l *authoringTaskLifecycle) beginStop(ctx context.Context) {
	if l == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.stopping {
		return
	}
	l.stopping = true
	l.cancel()
	deadline := time.Now().Add(authoringSettlementTimeout)
	if callerDeadline, ok := ctx.Deadline(); ok && callerDeadline.Before(deadline) {
		deadline = callerDeadline
	}
	// 所有已经创建的 settlement 也受这一排空 root 约束。
	l.drainTimer = time.AfterFunc(time.Until(deadline), l.drainCancel)
	l.stopCaller = context.AfterFunc(ctx, l.drainCancel)
	if ctx.Err() != nil {
		l.drainCancel()
	}
	if l.active == 0 {
		l.finishDrain()
	}
}

// 调用方持有 mu；active 首次归零后释放计时器与 caller cancellation 订阅。
func (l *authoringTaskLifecycle) finishDrain() {
	if l.drainTimer != nil {
		l.drainTimer.Stop()
	}
	if l.stopCaller != nil {
		l.stopCaller()
	}
	close(l.drained)
	l.drainCancel()
}

type authoringCallKey struct{}
type authoringCallScope struct{ lifecycle *authoringTaskLifecycle }

func (l *authoringTaskLifecycle) accept(ctx context.Context) (context.Context, func(), error) {
	if l == nil || ctx == nil {
		return ctx, nil, common.ErrConflict
	}
	if scope, ok := ctx.Value(authoringCallKey{}).(*authoringCallScope); ok && scope.lifecycle == l {
		l.mu.Lock()
		stopping := l.stopping
		l.mu.Unlock()
		if stopping || ctx.Err() != nil {
			return ctx, nil, common.ErrConflict
		}
		return ctx, func() {}, nil
	}
	l.mu.Lock()
	if l.stopping || l.active >= 32 {
		l.mu.Unlock()
		return ctx, nil, common.ErrConflict
	}
	l.active++
	l.mu.Unlock()
	callCtx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(l.ctx, cancel)
	callCtx = context.WithValue(callCtx, authoringCallKey{}, &authoringCallScope{lifecycle: l})
	var once sync.Once
	release := func() {
		once.Do(func() {
			stop()
			cancel()
			l.mu.Lock()
			defer l.mu.Unlock()
			l.active--
			if l.stopping && l.active == 0 {
				l.finishDrain()
			}
		})
	}
	return callCtx, release, nil
}

type authoringSettlementKey struct{}

func authoringSettlementContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if settlement, ok := ctx.Value(authoringSettlementKey{}).(context.Context); ok {
		return settlement, func() {}
	}
	settlement, cancel := context.WithTimeout(context.WithoutCancel(ctx), authoringSettlementTimeout)
	if scope, ok := ctx.Value(authoringCallKey{}).(*authoringCallScope); ok {
		stop := context.AfterFunc(scope.lifecycle.drainCtx, cancel)
		return settlement, func() { stop(); cancel() }
	}
	return settlement, cancel
}

func (l *authoringTaskLifecycle) schedule(ctx context.Context, background task.BackgroundRoutine, run func(context.Context)) error {
	if l == nil || background == nil || run == nil {
		return common.ErrConflict
	}
	callCtx, release, err := l.accept(l.ctx)
	if err != nil {
		return err
	}
	accepted := &authoringBackgroundTask{ctx: callCtx, run: run, done: release}
	if err := background.AsyncTaskContext(ctx, accepted); err != nil {
		release()
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
	if scope, ok := t.ctx.Value(authoringCallKey{}).(*authoringCallScope); ok && scope.lifecycle.ctx.Err() != nil {
		return
	}
	if t.ctx.Err() == nil {
		t.run(t.ctx)
	}
}

func (b *Biz) configureAuthoringBackgroundPorts(ports *server.ApplicationPorts) {
	ports.Definition.CompileDefinitionAuthoring = func(ctx context.Context, id string, request workorchmanagementcontract.DefinitionAuthoringCompile) (workorchmanagementcontract.DefinitionAuthoringView, error) {
		return b.StartDefinitionAuthoringCompilation(ctx, common.WorkspaceID(id), request)
	}
}

func (b *Biz) StartDefinitionAuthoringCompilation(ctx context.Context, id common.WorkspaceID, request workorchmanagementcontract.DefinitionAuthoringCompile) (workorchmanagementcontract.DefinitionAuthoringView, error) {
	if request.ExpectedInputHash == "" || request.ExpectedTemplateSnapshotHash == "" {
		return workorchmanagementcontract.DefinitionAuthoringView{}, common.ErrInvalidArgument
	}
	view, err := b.ReadDefinitionAuthoringView(ctx, id)
	if err != nil {
		return view, err
	}
	if !authoringCompileMatches(view, request) {
		return view, common.ErrConflict
	}
	switch view.State {
	case "ready_to_confirm", "review_blocked", "failed", "outcome_unresolved", "confirming", "admitting", "admitted", "clarification_required":
		return view, nil
	}
	hash := view.InputHash
	if err := b.authoringTasks.schedule(ctx, b.BackgroundRoutine(), func(taskCtx context.Context) {
		current, err := b.ReadDefinitionAuthoringView(taskCtx, id)
		if err != nil || !authoringCompileMatches(current, request) {
			return
		}
		if _, err := b.deriveDefinitionAcceptanceBound(taskCtx, id, "", &request); err != nil {
			b.logAuthoringCompilationFailure(taskCtx, id, hash, "derivation", err)
			return
		}
		current, err = b.ReadDefinitionAuthoringView(taskCtx, id)
		if err != nil || !authoringCompileMatches(current, request) {
			return
		}
		if _, err := b.compileAuthoringSpecificationBound(taskCtx, id, "", &request); err != nil {
			b.logAuthoringCompilationFailure(taskCtx, id, hash, "specification", err)
		}
	}); err != nil {
		return view, err
	}
	// 仅表示托管任务已接受；不伪造 owner 的 compiling／成功状态。
	view.CompilationAccepted = true
	return view, nil
}

func authoringCompileMatches(view workorchmanagementcontract.DefinitionAuthoringView, request workorchmanagementcontract.DefinitionAuthoringCompile) bool {
	return view.InputHash != "" && view.TemplateCurrent && view.InputHash == request.ExpectedInputHash && view.TemplateSnapshotHash == request.ExpectedTemplateSnapshotHash
}

func (b *Biz) logAuthoringCompilationFailure(ctx context.Context, id common.WorkspaceID, hash, phase string, err error) {
	attributes := []any{"workspace", id, "inputHash", hash, "phase", phase}
	var preparation *authoringPreparationError
	if errors.As(err, &preparation) {
		attributes = append(attributes, "stage", preparation.Stage, "phaseCode", preparation.PhaseCode, "diagnosticID", preparation.DiagnosticID, "diagnosticPersisted", preparation.Persisted, "persistenceFailureClass", preparation.PersistenceFailureClass)
		if preparation.Persisted {
			slog.Warn("authoring preparation stopped; diagnostic persisted and owner-queryable", attributes...)
		} else {
			slog.Warn("authoring preparation stopped; diagnostic persistence not confirmed", attributes...)
		}
		return
	}
	ownerObserved := false
	if observed, readErr := b.ReadDefinitionAuthoringView(ctx, id); readErr == nil && observed.InputHash == hash {
		ownerObserved = true
		attributes = append(attributes, "state", observed.State)
		for _, request := range observed.Requests {
			if request.State == "failed" || request.State == "outcome_unresolved" {
				attributes = append(attributes, "requestPhase", request.Phase, "permit", request.PermitID, "failureCode", request.FailureCode)
				break
			}
		}
	}
	attributes = append(attributes, "ownerObserved", ownerObserved)
	// 原始错误和可变 provider 配置都不能进入通用日志。
	slog.Warn("authoring compilation stopped; consult owner state before any explicit action", attributes...)
}
