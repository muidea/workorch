package qualitycore

import (
	"strings"
)

func NormalizeDoDItems(items []string) []string {
	ret := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		ret = append(ret, item)
	}
	return ret
}
