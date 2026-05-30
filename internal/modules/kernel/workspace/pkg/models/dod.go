package workspace

import "github.com/muidea/workorch/internal/pkg/common"

type DoD struct {
	ID                     common.DoDID
	WorkspaceID            common.WorkspaceID
	Items                  []string
	State                  common.DoDState
	DefinitionRevision     string
	PreviousDefinitionHash string
	PreviousDoDID          common.DoDID
	LineageState           string
}
