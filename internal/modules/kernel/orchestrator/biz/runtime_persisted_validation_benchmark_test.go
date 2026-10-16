package orchestrator

import (
	"encoding/json"
	"os"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
)

// Opt-in, read-only profiling of an owner-managed diagnostic snapshot. No
// fixture bytes are committed, no authority is reconstructed from projections,
// and this benchmark neither drives nor mutates the recorded Run.
func BenchmarkPersistedRuntimeValidation(b *testing.B) {
	file := os.Getenv("WORKORCH_RUNTIME_VALIDATION_BENCHMARK_FILE")
	if file == "" {
		b.Skip("requires an explicit owner Run snapshot")
	}
	body, err := os.ReadFile(file)
	if err != nil {
		b.Fatal(err)
	}
	var runs map[string]*runmodel.Run
	if err := json.Unmarshal(body, &runs); err != nil {
		b.Fatal(err)
	}
	if len(runs) != 1 {
		b.Fatal("requires exactly one Run")
	}
	for _, run := range runs {
		if run.RuntimeState == nil || run.RuntimeBootstrap == nil {
			b.Fatal("missing original Run state/bootstrap")
		}
		if err := run.RuntimeBootstrap.Validate(); err != nil {
			b.Fatal(err)
		}
		aggregate := runtimeAggregateFromRecord(*run.RuntimeState)
		if err := aggregate.Validate(); err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			if err := aggregate.Validate(); err != nil {
				b.Fatal(err)
			}
		}
	}
}
