package contextregistryports

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

type SkillHubCommandRunner interface {
	RunSkillHub(ctx context.Context, dir string, args ...string) ([]byte, error)
}

type SkillHubExecCommandRunner struct{}

func (SkillHubExecCommandRunner) RunSkillHub(ctx context.Context, dir string, args ...string) ([]byte, error) {
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

func ListSkillHubGlobalSkills(ctx context.Context, runner SkillHubCommandRunner) ([]workorchmanagementcontract.Skill, error) {
	return ListSkillHubSkills(ctx, runner, "", contextregistrycore.SkillScopeGlobal, "", "status", "--global", "--json", "--agent", "codex")
}

func ListSkillHubWorkspaceSkills(ctx context.Context, runner SkillHubCommandRunner, workspaceID string, deliverableDirectory string) ([]workorchmanagementcontract.Skill, error) {
	deliverableDirectory = strings.TrimSpace(deliverableDirectory)
	if deliverableDirectory == "" {
		return nil, nil
	}
	skillDir := filepath.Join(deliverableDirectory, ".agents", "skills")
	info, err := os.Stat(skillDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, nil
	}
	return ListSkillHubSkills(ctx, runner, deliverableDirectory, contextregistrycore.SkillScopeWorkspace, workspaceID, "status", "--json")
}

func ListSkillHubSkills(ctx context.Context, runner SkillHubCommandRunner, dir string, scope string, workspaceID string, args ...string) ([]workorchmanagementcontract.Skill, error) {
	if runner == nil {
		return nil, nil
	}
	output, err := runner.RunSkillHub(ctx, dir, args...)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	var status contextregistrycore.SkillHubStatus
	if err := json.Unmarshal(output, &status); err != nil {
		return nil, fmt.Errorf("parse skill-hub status: %w", err)
	}
	ret := make([]workorchmanagementcontract.Skill, 0, len(status.Items))
	seen := map[string]struct{}{}
	for _, item := range status.Items {
		skill := contextregistrycore.MapSkillHubStatusItemWithLocalInstruction(item, scope, workspaceID, status)
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
