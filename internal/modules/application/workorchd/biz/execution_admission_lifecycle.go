package biz

import (
	"context"
	"errors"
	"sync/atomic"
)

const executionAdmissionStoppingBit uint64 = 1 << 63

// executionAdmissionLifecycle is a process-lifecycle barrier shared by every
// Workorchd execution ingress. The low bits count handlers that already
// entered; the high bit permanently rejects new entries during Stop. It is not
// a business queue, retry ledger, admission outcome, or Runtime state.
type executionAdmissionLifecycle struct {
	value    atomic.Uint64
	signaled atomic.Bool
	drained  chan struct{}
}

func newExecutionAdmissionLifecycle() *executionAdmissionLifecycle {
	return &executionAdmissionLifecycle{drained: make(chan struct{})}
}

func (l *executionAdmissionLifecycle) begin() bool {
	if l == nil {
		return false
	}
	for {
		current := l.value.Load()
		if current&executionAdmissionStoppingBit != 0 {
			return false
		}
		if current == executionAdmissionStoppingBit-1 {
			return false
		}
		if l.value.CompareAndSwap(current, current+1) {
			return true
		}
	}
}

func (l *executionAdmissionLifecycle) end() {
	if l == nil {
		return
	}
	for {
		current := l.value.Load()
		active := current &^ executionAdmissionStoppingBit
		if active == 0 {
			return
		}
		next := current - 1
		if !l.value.CompareAndSwap(current, next) {
			continue
		}
		if next == executionAdmissionStoppingBit {
			l.signalDrained()
		}
		return
	}
}

func (l *executionAdmissionLifecycle) stop() bool {
	if l == nil {
		return false
	}
	for {
		current := l.value.Load()
		if current&executionAdmissionStoppingBit != 0 {
			if current == executionAdmissionStoppingBit {
				l.signalDrained()
			}
			return false
		}
		next := current | executionAdmissionStoppingBit
		if !l.value.CompareAndSwap(current, next) {
			continue
		}
		if next == executionAdmissionStoppingBit {
			l.signalDrained()
		}
		return true
	}
}

func (l *executionAdmissionLifecycle) wait() {
	_ = l.waitContext(context.Background())
}

// 停机等待超时不代表已接纳 handler 退出；保留排空信号供下一次等待。
func (l *executionAdmissionLifecycle) waitContext(ctx context.Context) error {
	if l == nil || l.drained == nil {
		return nil
	}
	if ctx == nil {
		return errors.New("execution admission wait context is nil")
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

func (l *executionAdmissionLifecycle) stopping() bool {
	return l != nil && l.value.Load()&executionAdmissionStoppingBit != 0
}

func (l *executionAdmissionLifecycle) signalDrained() {
	if l == nil || l.drained == nil {
		return
	}
	if l.signaled.CompareAndSwap(false, true) {
		close(l.drained)
	}
}
