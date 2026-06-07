package contextregistrycore

import (
	"os"
	"path/filepath"
	"strings"
)

func ReadSkillHubInstructionAndMetadata(item SkillHubStatusItem) (string, map[string]string) {
	for _, candidate := range []string{item.TargetPath, item.LocalPath, item.SourcePath, item.RepoPath} {
		path := SkillHubInstructionPath(candidate)
		if path == "" {
			continue
		}
		data, err := os.ReadFile(path)
		if err == nil {
			instruction := string(data)
			return instruction, ParseSkillHubFrontMatter(instruction)
		}
	}
	return "", nil
}

func SkillHubInstructionPath(candidate string) string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return ""
	}
	if filepath.Base(candidate) == "SKILL.md" {
		return candidate
	}
	return filepath.Join(candidate, "SKILL.md")
}
