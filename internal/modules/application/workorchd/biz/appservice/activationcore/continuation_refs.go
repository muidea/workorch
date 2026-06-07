package activationcore

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

func CompactStringRefs(input []string, limit int) []any {
	if len(input) == 0 || limit <= 0 {
		return nil
	}
	ret := make([]any, 0, min(len(input), limit))
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
		ret = append(ret, CompactContextValue(item, 1))
		if len(ret) >= limit {
			break
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func CompactArtifactRefs(input []common.ArtifactID, limit int) []any {
	if len(input) == 0 || limit <= 0 {
		return nil
	}
	ret := make([]any, 0, min(len(input), limit))
	seen := map[common.ArtifactID]struct{}{}
	for _, item := range input {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		ret = append(ret, CompactContextValue(string(item), 1))
		if len(ret) >= limit {
			break
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}
