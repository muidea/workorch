package common

import "strings"

var writableMemoryTypesBySource = map[string]map[string]struct{}{
	MemorySourceUser: {
		MemoryTypeProjectContext: {},
		MemoryTypeUserPreference: {},
	},
	MemorySourceRun: {
		MemoryTypeProjectContext:  {},
		MemoryTypeUserPreference:  {},
		MemoryTypeExecutionLesson: {},
		MemoryTypeSkillFeedback:   {},
		MemoryTypeToolFeedback:    {},
		MemoryTypeRunSummary:      {},
	},
}

var sensitiveMemoryPhrases = []string{
	"password",
	"passwd",
	"secret",
	"api key",
	"apikey",
	"access token",
	"refresh token",
	"bearer ",
	"authorization",
	"credential",
	"private key",
	"ssh-rsa",
	"ssh-ed25519",
	"-----begin",
	"cookie",
	"session",
}

func IsWritableMemoryTypeForSource(sourceType, memoryType string) bool {
	allowed, ok := writableMemoryTypesBySource[strings.TrimSpace(sourceType)]
	if !ok {
		return false
	}
	_, ok = allowed[strings.TrimSpace(memoryType)]
	return ok
}

func ContainsSensitiveMemoryText(values ...string) bool {
	combined := strings.ToLower(strings.Join(values, "\n"))
	if combined == "" {
		return false
	}
	for _, phrase := range sensitiveMemoryPhrases {
		if strings.Contains(combined, phrase) {
			return true
		}
	}
	return false
}
