package contextregistrycore

const (
	SkillSourceRepository = "repository"
	SkillScopeRepository  = "repository"
	SkillManagedBy        = "workorch"
)

type SkillQuery struct {
	WorkspaceID string
}

func FirstSkillQuery(query []SkillQuery) SkillQuery {
	if len(query) == 0 {
		return SkillQuery{}
	}
	return query[0]
}
