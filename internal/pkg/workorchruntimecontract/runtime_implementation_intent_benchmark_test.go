package workorchruntimecontract

import (
	"encoding/json"
	"os"
	"testing"
)

// Explicit owner archive input only; no live access or state rewriting. This
// keeps capacity measurements reproducible without committing runtime data.
func BenchmarkImplementationIntentOwnerArchive(b *testing.B) {
	path := os.Getenv("WORKORCH_IMPLEMENTATION_BENCHMARK_ARCHIVE")
	if path == "" {
		b.Skip("explicit read-only synthetic owner archive required")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	var runs map[string]struct{ RuntimeState *RuntimeStateRecord }
	if err := json.Unmarshal(raw, &runs); err != nil {
		b.Fatal(err)
	}
	if len(runs) != 1 {
		b.Fatal("benchmark requires one exact Run")
	}
	for _, run := range runs {
		if run.RuntimeState == nil {
			b.Fatal("missing Runtime state")
		}
		state := run.RuntimeState
		if err := state.Validate(); err != nil {
			b.Fatal(err)
		}
		b.Run("load", func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(raw)))
			for range b.N {
				var decoded map[string]struct{ RuntimeState *RuntimeStateRecord }
				if err := json.Unmarshal(raw, &decoded); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run("validate", func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				if err := state.Validate(); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run("clone", func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				_ = state.Clone()
			}
		})
	}
}
