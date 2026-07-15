package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
)

func llmUsageCacheEstimatedEntry(item readmodelcore.LLMUsageTrace) bool {
	value, _ := item.CacheUsage.Bool("estimated")
	return value
}

func llmUsageCacheProviderNativeEntry(item readmodelcore.LLMUsageTrace) bool {
	value, _ := item.CacheUsage.String("source")
	return strings.TrimSpace(value) == llmcommon.UsageSourceProviderNative
}
