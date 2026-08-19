package contextregistrycore

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	"github.com/muidea/workorch/internal/pkg/workorchskillcontract"
)

const (
	SkillSourceSkillHub    = workorchskillcontract.SourceSkillHub
	SkillScopeGlobal       = workorchskillcontract.ScopeGlobal
	SkillScopeWorkspace    = workorchskillcontract.ScopeWorkspace
	SkillManagedBySkillHub = "skill-hub"
)

type SkillHubStatus struct {
	Scope       string               `json:"scope"`
	GlobalPath  string               `json:"global_path"`
	ProjectPath string               `json:"project_path"`
	SkillCount  int                  `json:"skill_count"`
	Items       []SkillHubStatusItem `json:"items"`
}

type SkillHubStatusItem struct {
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

type SkillHubSkillCatalog interface {
	ListGlobalSkills(ctx context.Context) ([]workorchmanagementcontract.Skill, error)
	ListWorkspaceSkills(ctx context.Context, workspaceID string) ([]workorchmanagementcontract.Skill, error)
}
