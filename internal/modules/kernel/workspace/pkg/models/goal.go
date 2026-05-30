package workspace

import "github.com/muidea/workorch/internal/pkg/common"

type Goal struct {
	ID                     common.GoalID
	WorkspaceID            common.WorkspaceID
	Text                   string
	DefinitionRevision     string
	PreviousDefinitionHash string
	PreviousGoalID         common.GoalID
	LineageState           string
}
