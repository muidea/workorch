package biz

import (
	"context"
	"errors"
	"sync"
)

var (
	ErrNilLifecycle               = errors.New("lifecycle is nil")
	ErrNilLifecycleContext        = errors.New("lifecycle context is nil")
	ErrLifecycleClosing           = errors.New("lifecycle is closing")
	ErrLifecycleNotClosing        = errors.New("lifecycle shutdown has not begun")
	ErrLifecycleNotDrained        = errors.New("previous lifecycle generation has not shut down and drained")
	ErrLifecycleGenerationChanged = errors.New("lifecycle generation changed while quiescing")
)

// Lifecycle 由具体 Biz 持有，仅管理本 owner 的调用准入和真实在途计数。
// 零值可用，初始允许 fresh 调用；首次使用后不得复制。
// 它不拥有 Hub、后台任务、订阅、业务状态或资源，也不推断 Source/topic。
//
// owner 在开始调用或提交任务前 Enter，失败则不执行；成功后必须在实际
// handler/任务结束时调用返回的 leave，不能因 Send、SyncTask 或 context
// 等待超时而提前释放。提交失败且确认未执行时也必须 leave。
//
// BeginShutdown 只停止 fresh 准入。owner 显式分类并验证已接纳工作的
// nested Send、receipt、query、CAS 等收尾路径后，才可使用 EnterDrain；
// 不得把它作为 Enter 被拒后的 fallback，更不能用它接纳新 root 工作。
// 跨 owner 的合法 drain 可能在本 owner 暂时没有在途调用时到达，故不能
// 以本地计数或调用来源判断 drain 资格。
//
// Quiesce 只证明本 owner 在一个线性化点真实归零，不封闭 drain 入口。
// 最终取消订阅、释放依赖还必须经过应用的共享 Hub/任务排空屏障；
// timeout 不授权清空状态、释放依赖或开始下一代。
//
// 典型接线：Biz 持有 lifecycle Lifecycle；Start 检查 lifecycle.Reset()；
// fresh handler 执行 leave, err := lifecycle.Enter() 并在成功后 defer leave()；
// 明确的 drain handler 使用 EnterDrain；BeginShutdown 调用同名方法并由
// owner 停止调度；Quiesce(ctx) 委托同名方法，失败保留所有未完成依赖。
// 异步任务应在入队前取得 leave，把释放放在真实任务体的 defer 中。
// 同一个任务不能同时在等待方和执行方释放。
type Lifecycle struct {
	mu          sync.Mutex
	initialized bool
	closing     bool
	generation  uint64
	inflight    int
	idle        chan struct{}
}

// Reset 开启一代。除尚未使用的零值外，只有已经关门且真实在途为零
// 才能重置；重复 Start 不能重新打开仍在工作的代次。
func (l *Lifecycle) Reset() error {
	if l == nil {
		return ErrNilLifecycle
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.initialized && (!l.closing || l.inflight != 0) {
		return ErrLifecycleNotDrained
	}
	l.initialized = true
	l.closing = false
	l.generation++
	l.idle = nil
	return nil
}

// BeginShutdown 与 fresh 注册共用一把短锁，关门后不再接纳新 root。
// 它幂等且不等待在途调用，也不取消任务或释放任何资源。
func (l *Lifecycle) BeginShutdown() {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.initialized = true
	l.closing = true
	l.mu.Unlock()
}

// Enter 接纳 fresh 调用。成功返回的 leave 幂等且可并发调用，拒绝时
// leave 为 nil。leave 表示实际完成，而非调用方已经停止等待。
func (l *Lifecycle) Enter() (leave func(), err error) {
	return l.enter(false)
}

// EnterDrain 接纳 owner 已验证为既有工作的收尾调用，关门后仍可使用。
// owner 必须显式选择这一入口；该方法不赋予任何新业务工作的权限。
func (l *Lifecycle) EnterDrain() (leave func(), err error) {
	return l.enter(true)
}

func (l *Lifecycle) enter(drain bool) (func(), error) {
	if l == nil {
		return nil, ErrNilLifecycle
	}
	l.mu.Lock()
	if l.closing && !drain {
		l.mu.Unlock()
		return nil, ErrLifecycleClosing
	}
	l.initialized = true
	if l.inflight == 0 {
		l.idle = make(chan struct{})
	}
	l.inflight++
	l.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			l.inflight--
			if l.inflight == 0 {
				close(l.idle)
			}
			l.mu.Unlock()
		})
	}, nil
}

// Quiesce 在 ctx 预算内等待关门代次的真实在途归零，不修改任何状态。
// 调用前必须 BeginShutdown；timeout/cancel 后可以用新预算重试。
// 等待期间若合法 drain 再次进入，会继续等待，不能把旧 idle 通知当作
// 当前已排空；若代次已 Reset，返回明确错误而不替新代次宣告成功。
func (l *Lifecycle) Quiesce(ctx context.Context) error {
	if l == nil {
		return ErrNilLifecycle
	}
	if ctx == nil {
		return ErrNilLifecycleContext
	}

	l.mu.Lock()
	generation := l.generation
	l.mu.Unlock()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		l.mu.Lock()
		if l.generation != generation {
			l.mu.Unlock()
			return ErrLifecycleGenerationChanged
		}
		if !l.closing {
			l.mu.Unlock()
			return ErrLifecycleNotClosing
		}
		if l.inflight == 0 {
			l.mu.Unlock()
			return nil
		}
		idle := l.idle
		l.mu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-idle:
		}
	}
}
