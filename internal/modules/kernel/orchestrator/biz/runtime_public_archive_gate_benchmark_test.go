package orchestrator

import (
	"encoding/json"
	"os"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Explicit diagnostic archive only. These benchmarks do not start a Run,
// replace a complete gate, acquire owner facts or mutate the archived bytes.
func BenchmarkRuntimePublicArchiveCompleteGates(b *testing.B) {
	path := os.Getenv("WORKORCH_WORK_COMPILER_ARCHIVE")
	if path == "" {
		b.Skip("explicit read-only archive required")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	var runs map[string]struct {
		RuntimeState runtimecontract.RuntimeStateRecord
	}
	if err := json.Unmarshal(body, &runs); err != nil || len(runs) == 0 {
		b.Fatalf("invalid or empty archive: %v", err)
	}
	for id, run := range runs {
		record := run.RuntimeState
		aggregate := runtimeAggregateFromRecord(record)
		b.Run(id+"/record", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if err := record.Validate(); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(id+"/aggregate", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if err := aggregate.Validate(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
