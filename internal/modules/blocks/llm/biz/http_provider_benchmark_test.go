package llm

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
)

var (
	providerLimitsBenchmarkResult    providerResponseLimits
	providerStreamBenchmarkResult    string
	providerEmbeddingBenchmarkResult []float64
)

func BenchmarkProviderCompletionResponseLimitsLargeCapacity(b *testing.B) {
	config := testAIProxyProviderConfig("")
	config.InferenceProfile.MaxOutputTokens = 1_000_000
	config.ContextProfile.EstimatedTokensPerByte = 0.125
	b.ReportAllocs()
	for range b.N {
		limits, err := providerCompletionResponseLimits(config, llmmodels.ProviderRequest{})
		if err != nil {
			b.Fatal(err)
		}
		providerLimitsBenchmarkResult = limits
	}
}

func BenchmarkReadResponsesStreamLong(b *testing.B) {
	const eventCount = 4_096
	var input strings.Builder
	for idx := 0; idx < eventCount; idx++ {
		input.WriteString("data: {\"type\":\"response.output_text.delta\",\"delta\":\"abcdefgh\"}\n\n")
	}
	input.WriteString("data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
	payload := input.String()
	limits := providerStreamLimits{MaxContentBytes: eventCount * 8, MaxDataEvents: eventCount + 1, MaxLineBytes: 1024}
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		content, _, err := readResponsesStream(strings.NewReader(payload), nil, limits)
		if err != nil {
			b.Fatal(err)
		}
		providerStreamBenchmarkResult = content
	}
}

func BenchmarkDecodeLargeEmbeddingVector(b *testing.B) {
	const dimensions = 8_192
	var input strings.Builder
	input.WriteString(`{"data":[{"embedding":[`)
	for idx := 0; idx < dimensions; idx++ {
		if idx > 0 {
			input.WriteByte(',')
		}
		input.WriteString(strconv.FormatFloat(float64(idx)/dimensions, 'f', 6, 64))
	}
	input.WriteString(`]}],"usage":{"prompt_tokens":16,"total_tokens":16}}`)
	payload := []byte(input.String())
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		var decoded embeddingResponse
		if err := json.Unmarshal(payload, &decoded); err != nil {
			b.Fatal(err)
		}
		vector := decoded.FirstEmbedding()
		if len(vector) != dimensions {
			b.Fatalf("vector length = %d, want %d", len(vector), dimensions)
		}
		providerEmbeddingBenchmarkResult = vector
	}
}
