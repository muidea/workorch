package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
)

func llmUsageCacheEstimatedEntry(item readmodelcore.LLMUsageTrace) bool {
	return readModelBoolValue(item.CacheUsage["estimated"])
}

func llmUsageCacheProviderNativeEntry(item readmodelcore.LLMUsageTrace) bool {
	return strings.TrimSpace(readModelStringValue(item.CacheUsage["source"])) == llmcommon.UsageSourceProviderNative
}
