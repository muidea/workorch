package content

import "strings"

func AllowedRuntimeRawContentFile(collection string, fileName string) bool {
	collection = strings.TrimSpace(collection)
	switch strings.TrimSpace(fileName) {
	case "request.json", "response.json", "response.sse", "usage.json", "repair-response.json":
		return collection == "llm-calls"
	case "input.json":
		return collection == "steps"
	case "arguments.json":
		return collection == "tool-calls"
	case "output.json":
		return collection == "steps" || collection == "tool-calls"
	case "data.json":
		return collection == "runtime-checkpoints"
	case "dod-assessment.json", "summary-detail.json":
		return collection == "results"
	default:
		return false
	}
}

func RuntimeRawContentType(fileName string) string {
	switch strings.TrimSpace(fileName) {
	case "response.sse":
		return "text/event-stream; charset=utf-8"
	case "request.json", "response.json", "usage.json", "repair-response.json", "input.json", "output.json", "arguments.json", "data.json", "dod-assessment.json", "summary-detail.json":
		return "application/json; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}
