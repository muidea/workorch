package llm

import (
	"testing"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
)

var runtimeSemanticProviderBenchmarkResult llmmodels.ProviderRequest

func BenchmarkRuntimeSemanticPlanProviderRequest(b *testing.B) {
	request := runtimeSemanticProviderTestRequest(b)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		providerRequest, err := runtimeSemanticPlanProviderRequest(request)
		if err != nil {
			b.Fatal(err)
		}
		runtimeSemanticProviderBenchmarkResult = providerRequest
	}
}
