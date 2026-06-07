package readmodelprojection

import (
	"strings"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
)

func llmUsageTraceSource(usageSource string, estimated bool) string {
	if strings.TrimSpace(usageSource) != "" {
		return usageSource
	}
	if estimated {
		return llmcommon.UsageSourceEstimated
	}
	return llmcommon.UsageSourceProviderNative
}
