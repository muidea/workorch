package contextregistrycore

import "strings"

func SkillHubSkillRef(scope string, workspaceID string, skillID string) string {
	switch scope {
	case SkillScopeWorkspace:
		if strings.TrimSpace(workspaceID) != "" {
			return "skillhub:workspace:" + strings.TrimSpace(workspaceID) + ":" + strings.TrimSpace(skillID)
		}
		return "skillhub:workspace:" + strings.TrimSpace(skillID)
	default:
		return "skillhub:global:" + strings.TrimSpace(skillID)
	}
}

func ParseSkillHubSkillRef(ref string) (scope string, workspaceID string, sourceSkillID string, ok bool) {
	ref = strings.TrimSpace(ref)
	if !strings.HasPrefix(ref, "skillhub:") {
		return "", "", "", false
	}
	parts := strings.Split(ref, ":")
	if len(parts) < 3 {
		return "", "", "", false
	}
	switch parts[1] {
	case SkillScopeGlobal:
		if len(parts) != 3 || strings.TrimSpace(parts[2]) == "" {
			return "", "", "", false
		}
		return SkillScopeGlobal, "", parts[2], true
	case SkillScopeWorkspace:
		if len(parts) == 4 && strings.TrimSpace(parts[2]) != "" && strings.TrimSpace(parts[3]) != "" {
			return SkillScopeWorkspace, parts[2], parts[3], true
		}
		if len(parts) == 3 && strings.TrimSpace(parts[2]) != "" {
			return SkillScopeWorkspace, "", parts[2], true
		}
	}
	return "", "", "", false
}

func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
