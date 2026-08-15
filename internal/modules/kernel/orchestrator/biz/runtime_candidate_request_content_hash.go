package orchestrator

import "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/requestcontent"

// These are opaque-content helpers for the persisted C4 request-content
// carrier. They intentionally contain no continuation state or provider flow.
func runtimeCandidateOutputHash(value string) string {
	return requestcontent.ContentHash(value)
}

func runtimeCandidateOutputTail(value string, max int) string {
	return requestcontent.ContentTail(value, max)
}
