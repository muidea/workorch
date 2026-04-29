package appservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	skillSourceRepository = "repository"
	skillSourceSkillHub   = "skill-hub"

	skillScopeRepository = "repository"
	skillScopeGlobal     = "global"
	skillScopeWorkspace  = "workspace"

	skillManagedByWorkorch = "workorch"
	skillManagedBySkillHub = "skill-hub"
)

type SkillQuery struct {
	WorkspaceID string
}

type SkillHubCommandRunner interface {
	RunSkillHub(ctx context.Context, dir string, args ...string) ([]byte, error)
}

type skillHubExecCommandRunner struct{}

func (skillHubExecCommandRunner) RunSkillHub(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "skill-hub", args...)
	if strings.TrimSpace(dir) != "" {
		cmd.Dir = dir
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("skill-hub %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

var skillHubRunner SkillHubCommandRunner = skillHubExecCommandRunner{}

type skillHubStatus struct {
	Scope       string               `json:"scope"`
	GlobalPath  string               `json:"global_path"`
	ProjectPath string               `json:"project_path"`
	SkillCount  int                  `json:"skill_count"`
	Items       []skillHubStatusItem `json:"items"`
}

type skillHubStatusItem struct {
	SkillID          string `json:"skill_id"`
	Agent            string `json:"agent,omitempty"`
	Status           string `json:"status,omitempty"`
	Message          string `json:"message,omitempty"`
	SourceRepository string `json:"source_repository,omitempty"`
	Version          string `json:"version,omitempty"`
	SourceHash       string `json:"source_hash,omitempty"`
	AppliedHash      string `json:"applied_hash,omitempty"`
	ActualHash       string `json:"actual_hash,omitempty"`
	SourcePath       string `json:"source_path,omitempty"`
	TargetPath       string `json:"target_path,omitempty"`
	LocalVersion     string `json:"local_version,omitempty"`
	RepoVersion      string `json:"repo_version,omitempty"`
	LocalPath        string `json:"local_path,omitempty"`
	RepoPath         string `json:"repo_path,omitempty"`
}

func listSkillHubGlobalSkills(ctx context.Context) ([]Skill, error) {
	return listSkillHubSkills(ctx, "", skillScopeGlobal, "", "status", "--global", "--json", "--agent", "codex")
}

func listSkillHubWorkspaceSkills(ctx context.Context, workspaceID string, workingDirectory string) ([]Skill, error) {
	workingDirectory = strings.TrimSpace(workingDirectory)
	if workingDirectory == "" {
		return nil, nil
	}
	return listSkillHubSkills(ctx, workingDirectory, skillScopeWorkspace, workspaceID, "status", "--json")
}

func listSkillHubSkills(ctx context.Context, dir string, scope string, workspaceID string, args ...string) ([]Skill, error) {
	if skillHubRunner == nil {
		return nil, nil
	}
	output, err := skillHubRunner.RunSkillHub(ctx, dir, args...)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	var status skillHubStatus
	if err := json.Unmarshal(output, &status); err != nil {
		return nil, fmt.Errorf("parse skill-hub status: %w", err)
	}
	ret := make([]Skill, 0, len(status.Items))
	seen := map[string]struct{}{}
	for _, item := range status.Items {
		skill := mapSkillHubStatusItem(item, scope, workspaceID, status)
		if skill.ID == "" {
			continue
		}
		if _, ok := seen[skill.ID]; ok {
			continue
		}
		seen[skill.ID] = struct{}{}
		ret = append(ret, skill)
	}
	return ret, nil
}

func mapSkillHubStatusItem(item skillHubStatusItem, scope string, workspaceID string, status skillHubStatus) Skill {
	sourceSkillID := strings.TrimSpace(item.SkillID)
	if sourceSkillID == "" {
		return Skill{}
	}
	instruction, metadata := readSkillHubInstructionAndMetadata(item)
	skill := Skill{
		ID:           skillHubSkillRef(scope, workspaceID, sourceSkillID),
		Name:         firstNonEmpty(metadata["name"], sourceSkillID),
		Version:      firstNonEmpty(item.Version, item.LocalVersion, item.RepoVersion, metadata["version"]),
		Description:  firstNonEmpty(metadata["description"], strings.TrimSpace(item.Message)),
		TriggerRules: frontMatterStringList(metadata["triggerRules"], metadata["trigger_rules"], metadata["triggers"]),
		Instruction:  instruction,
		RequiredToolKinds: frontMatterStringList(
			metadata["requiredToolKinds"],
			metadata["required_tool_kinds"],
			metadata["requiredTools"],
			metadata["required_tools"],
		),
		RequiredMCPServerRefs: frontMatterStringList(
			metadata["requiredMCPServerRefs"],
			metadata["required_mcp_server_refs"],
			metadata["requiredMcpServers"],
			metadata["required_mcp_servers"],
		),
		Constraints:      frontMatterStringList(metadata["constraints"]),
		Source:           skillSourceSkillHub,
		Scope:            scope,
		WorkspaceID:      strings.TrimSpace(workspaceID),
		Status:           item.Status,
		SourceRepository: item.SourceRepository,
		SourcePath:       item.SourcePath,
		TargetPath:       item.TargetPath,
		LocalPath:        item.LocalPath,
		RepoPath:         item.RepoPath,
		GlobalPath:       status.GlobalPath,
		ProjectPath:      status.ProjectPath,
		ContentHash:      firstNonEmpty(item.ActualHash, item.AppliedHash, item.SourceHash),
		SourceHash:       item.SourceHash,
		AppliedHash:      item.AppliedHash,
		ActualHash:       item.ActualHash,
		Agent:            item.Agent,
		ManagedBy:        skillManagedBySkillHub,
		SourceSkillID:    sourceSkillID,
	}
	return skill
}

func readSkillHubInstruction(item skillHubStatusItem) string {
	instruction, _ := readSkillHubInstructionAndMetadata(item)
	return instruction
}

func readSkillHubInstructionAndMetadata(item skillHubStatusItem) (string, map[string]string) {
	for _, candidate := range []string{item.TargetPath, item.LocalPath, item.SourcePath, item.RepoPath} {
		path := skillHubInstructionPath(candidate)
		if path == "" {
			continue
		}
		data, err := os.ReadFile(path)
		if err == nil {
			instruction := string(data)
			return instruction, parseSkillHubFrontMatter(instruction)
		}
	}
	return "", nil
}

func skillHubInstructionPath(candidate string) string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return ""
	}
	if filepath.Base(candidate) == "SKILL.md" {
		return candidate
	}
	return filepath.Join(candidate, "SKILL.md")
}

func skillHubSkillRef(scope string, workspaceID string, skillID string) string {
	switch scope {
	case skillScopeWorkspace:
		if strings.TrimSpace(workspaceID) != "" {
			return "skillhub:workspace:" + strings.TrimSpace(workspaceID) + ":" + strings.TrimSpace(skillID)
		}
		return "skillhub:workspace:" + strings.TrimSpace(skillID)
	default:
		return "skillhub:global:" + strings.TrimSpace(skillID)
	}
}

func parseSkillHubSkillRef(ref string) (scope string, workspaceID string, sourceSkillID string, ok bool) {
	ref = strings.TrimSpace(ref)
	if !strings.HasPrefix(ref, "skillhub:") {
		return "", "", "", false
	}
	parts := strings.Split(ref, ":")
	if len(parts) < 3 {
		return "", "", "", false
	}
	switch parts[1] {
	case skillScopeGlobal:
		if len(parts) != 3 || strings.TrimSpace(parts[2]) == "" {
			return "", "", "", false
		}
		return skillScopeGlobal, "", parts[2], true
	case skillScopeWorkspace:
		if len(parts) == 4 && strings.TrimSpace(parts[2]) != "" && strings.TrimSpace(parts[3]) != "" {
			return skillScopeWorkspace, parts[2], parts[3], true
		}
		if len(parts) == 3 && strings.TrimSpace(parts[2]) != "" {
			return skillScopeWorkspace, "", parts[2], true
		}
	}
	return "", "", "", false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parseSkillHubFrontMatter(content string) map[string]string {
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
				ret[currentKey] += trimSkillHubScalar(value)
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
		ret[key] = trimSkillHubScalar(value)
	}
	return ret
}

func frontMatterStringList(values ...string) []string {
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
			part = trimSkillHubScalar(part)
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

func trimSkillHubScalar(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	return strings.TrimSpace(value)
}
