package readmodelprojection

import "strings"

func isDoDLeafCovered(requiredNormalized string, covered []string) bool {
	for _, actual := range covered {
		if actual == "" {
			continue
		}
		if strings.Contains(actual, requiredNormalized) || strings.Contains(requiredNormalized, actual) {
			return true
		}
	}
	return false
}
