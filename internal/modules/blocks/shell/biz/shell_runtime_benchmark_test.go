package shell

import (
	"context"
	"testing"
)

var runtimeShellBenchmarkResult RuntimeCommandResult

func BenchmarkRuntimeAdapterExecuteBoundedCommand(b *testing.B) {
	request := RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: b.TempDir(),
		Command:                       "true",
	}
	adapter := NewRuntimeAdapter()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		result, err := adapter.ExecuteOperation(context.Background(), request)
		if err != nil || result.Status != "completed" || result.ExitCode != 0 {
			b.Fatalf("bounded command result = %+v err=%v", result, err)
		}
		runtimeShellBenchmarkResult = result
	}
}
