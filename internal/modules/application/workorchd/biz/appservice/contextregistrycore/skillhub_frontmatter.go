package contextregistrycore

import "strings"

func ParseSkillHubFrontMatter(content string) map[string]string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil
	}
	ret := map[string]string{}
	var currentKey string
	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			break
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "- ") && currentKey != "" {
			value := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			if value != "" {
				if ret[currentKey] != "" {
					ret[currentKey] += ","
				}
				ret[currentKey] += TrimSkillHubScalar(value)
			}
			continue
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		currentKey = key
		ret[key] = TrimSkillHubScalar(value)
	}
	return ret
}

func FrontMatterStringList(values ...string) []string {
	ret := make([]string, 0)
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
			value = strings.TrimPrefix(strings.TrimSuffix(value, "]"), "[")
		}
		for _, part := range strings.Split(value, ",") {
			part = TrimSkillHubScalar(part)
			if part == "" {
				continue
			}
			if _, ok := seen[part]; ok {
				continue
			}
			seen[part] = struct{}{}
			ret = append(ret, part)
		}
	}
	return ret
}

func TrimSkillHubScalar(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	return strings.TrimSpace(value)
}
