package readmodelprojection

import (
	"strings"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
)

func llmUsageCacheEstimatedEntry(item projectioncontract.LLMUsageTrace) bool {
	value, _ := item.CacheUsage.Bool("estimated")
	return value
}

func llmUsageCacheProviderNativeEntry(item projectioncontract.LLMUsageTrace) bool {
	value, _ := item.CacheUsage.String("source")
	return strings.TrimSpace(value) == llmcommon.UsageSourceProviderNative
}
