// Package bootstrapcompiler compiles detached declarations without owner I/O or execution authority.
package bootstrapcompiler

import "github.com/muidea/workorch/internal/pkg/common"

type GoalInput struct {
	ID          common.GoalID
	WorkspaceID common.WorkspaceID
	Text        string
}
type DoDInput struct {
	ID          common.DoDID
	WorkspaceID common.WorkspaceID
	Items       []string
}
