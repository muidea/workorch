package contextregistrycore

const (
	SkillSourceRepository = "repository"
	SkillScopeRepository  = "repository"
	SkillManagedBy        = "workorch"
)

type Soul struct {
	ID                          string   `json:"id"`
	Name                        string   `json:"name"`
	Description                 string   `json:"description,omitempty"`
	BusinessDomains             []string `json:"businessDomains,omitempty"`
	SoulMD                      string   `json:"soulMd"`
	AllowedSkillRefs            []string `json:"allowedSkillRefs,omitempty"`
	AllowedMCPServerRefs        []string `json:"allowedMCPServerRefs,omitempty"`
	AllowedCapabilityRefs       []string `json:"allowedCapabilityRefs,omitempty"`
	AllowedCapabilityCategories []string `json:"allowedCapabilityCategories,omitempty"`
	AllowedToolKinds            []string `json:"allowedToolKinds,omitempty"`
}

type Skill struct {
	ID                     string   `json:"id"`
	Name                   string   `json:"name"`
	Version                string   `json:"version"`
	Description            string   `json:"description,omitempty"`
	TriggerRules           []string `json:"triggerRules,omitempty"`
	Instruction            string   `json:"instruction"`
	ReferenceRefs          []string `json:"referenceRefs,omitempty"`
	RequiredToolKinds      []string `json:"requiredToolKinds,omitempty"`
	RequiredMCPServerRefs  []string `json:"requiredMCPServerRefs,omitempty"`
	RequiredCapabilityRefs []string `json:"requiredCapabilityRefs,omitempty"`
	Constraints            []string `json:"constraints,omitempty"`
	Source                 string   `json:"source,omitempty"`
	Scope                  string   `json:"scope,omitempty"`
	WorkspaceID            string   `json:"workspaceId,omitempty"`
	Status                 string   `json:"status,omitempty"`
	SourceRepository       string   `json:"sourceRepository,omitempty"`
	SourcePath             string   `json:"sourcePath,omitempty"`
	TargetPath             string   `json:"targetPath,omitempty"`
	LocalPath              string   `json:"localPath,omitempty"`
	RepoPath               string   `json:"repoPath,omitempty"`
	GlobalPath             string   `json:"globalPath,omitempty"`
	ProjectPath            string   `json:"projectPath,omitempty"`
	ContentHash            string   `json:"contentHash,omitempty"`
	SourceHash             string   `json:"sourceHash,omitempty"`
	AppliedHash            string   `json:"appliedHash,omitempty"`
	ActualHash             string   `json:"actualHash,omitempty"`
	Agent                  string   `json:"agent,omitempty"`
	ManagedBy              string   `json:"managedBy,omitempty"`
	SourceSkillID          string   `json:"sourceSkillId,omitempty"`
}

type SkillQuery struct {
	WorkspaceID string
}

func FirstSkillQuery(query []SkillQuery) SkillQuery {
	if len(query) == 0 {
		return SkillQuery{}
	}
	return query[0]
}
