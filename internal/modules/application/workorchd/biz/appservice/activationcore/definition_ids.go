package activationcore

import "strings"

func LatestDefinitionID(ids []string, effectiveID string) string {
	if strings.TrimSpace(effectiveID) != "" {
		return strings.TrimSpace(effectiveID)
	}
	if len(ids) == 0 {
		return ""
	}
	return ids[len(ids)-1]
}

func PreviousDefinitionIDs(ids []string, effectiveID string) []string {
	ret := make([]string, 0, len(ids))
	effectiveID = strings.TrimSpace(effectiveID)
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || id == effectiveID {
			continue
		}
		ret = append(ret, id)
	}
	return ret
}
