package contextregistrycore

import "github.com/muidea/workorch/internal/pkg/workorchskillcontract"

const (
	SkillSourceWorkOrchRegistry = workorchskillcontract.SourceWorkOrchRegistry
	SkillScopeWorkOrchRegistry  = workorchskillcontract.ScopeGlobal
	SkillManagedBy              = "workorch"
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
