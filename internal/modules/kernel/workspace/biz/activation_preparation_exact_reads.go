package workspace

import (
	"context"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/startuppreparation"
	"reflect"
)

type preparationExactReadsKey struct{}

// Owner-private, one settlement invocation only. No external validated flag,
// hash-only hit, repository handle, cross-request or cross-Workspace cache.
// Access is serialized by the same definition mutex used by every exact read.
type preparationExactReads struct {
	workspace *workspace.Workspace
	goal      *workspace.Goal
	dod       *workspace.DoD
	snapshot  startuppreparation.ExactConfirmedStartupSnapshot
	hits      int
}

func withPreparationExactReads(ctx context.Context) context.Context {
	return context.WithValue(ctx, preparationExactReadsKey{}, &preparationExactReads{})
}
func (r *preparationExactReads) matches(w *workspace.Workspace, g *workspace.Goal, d *workspace.DoD) bool {
	return r != nil && r.workspace != nil && reflect.DeepEqual(r.workspace, w) && reflect.DeepEqual(r.goal, g) && reflect.DeepEqual(r.dod, d)
}
