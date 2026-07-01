package runtimeevent

import "sync/atomic"

type RuntimeEventPublishMetrics struct {
	AcceptedPublishFailuresTotal     uint64
	TerminalPublishFailuresTotal     uint64
	ProgressPublishFailuresTotal     uint64
	WorkerLifecycleFailuresTotal     uint64
	WorkerLostPublishFailuresTotal   uint64
	HeartbeatTimerStartFailuresTotal uint64
}

var runtimeEventAcceptedPublishFailuresTotal atomic.Uint64
var runtimeEventTerminalPublishFailuresTotal atomic.Uint64
var runtimeEventProgressPublishFailuresTotal atomic.Uint64
var runtimeEventWorkerLifecycleFailuresTotal atomic.Uint64
var runtimeEventWorkerLostPublishFailuresTotal atomic.Uint64
var runtimeEventHeartbeatTimerStartFailuresTotal atomic.Uint64

func RuntimeEventPublishMetricsSnapshot() RuntimeEventPublishMetrics {
	return RuntimeEventPublishMetrics{
		AcceptedPublishFailuresTotal:     runtimeEventAcceptedPublishFailuresTotal.Load(),
		TerminalPublishFailuresTotal:     runtimeEventTerminalPublishFailuresTotal.Load(),
		ProgressPublishFailuresTotal:     runtimeEventProgressPublishFailuresTotal.Load(),
		WorkerLifecycleFailuresTotal:     runtimeEventWorkerLifecycleFailuresTotal.Load(),
		WorkerLostPublishFailuresTotal:   runtimeEventWorkerLostPublishFailuresTotal.Load(),
		HeartbeatTimerStartFailuresTotal: runtimeEventHeartbeatTimerStartFailuresTotal.Load(),
	}
}

func recordRuntimeHeartbeatTimerStartFailure() {
	runtimeEventHeartbeatTimerStartFailuresTotal.Add(1)
}
