package runtimecore

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

func CompactMemoryIDs(input []common.MemoryID) []common.MemoryID {
	if len(input) == 0 {
		return nil
	}
	ret := make([]common.MemoryID, 0, len(input))
	seen := map[common.MemoryID]struct{}{}
	for _, item := range input {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		ret = append(ret, item)
	}
	return ret
}

func CompactStringRefs(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	ret := make([]string, 0, len(input))
	seen := map[string]struct{}{}
	for _, item := range input {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		ret = append(ret, item)
	}
	return ret
}
