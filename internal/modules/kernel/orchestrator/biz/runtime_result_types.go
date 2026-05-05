package orchestrator

import "github.com/muidea/workorch/internal/pkg/common"

type resultEvidenceDetail struct {
	Ref      string
	Kind     string
	Command  string
	ExitCode string
	Stdout   string
	Stderr   string
	Summary  string
}

type resultPatchDetail struct {
	ID        common.PatchID
	PatchType string
	TargetRef string
	Summary   string
	State     common.PatchState
	Content   string
	Diff      string
}
