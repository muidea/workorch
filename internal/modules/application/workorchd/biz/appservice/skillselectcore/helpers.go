package skillselectcore

import "strings"

func allowedRefSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	ret := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			ret[value] = struct{}{}
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
