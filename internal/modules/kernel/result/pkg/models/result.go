package result

import "github.com/muidea/workorch/internal/pkg/common"

type Result struct {
	ID           common.ResultID
	WorkspaceID  common.WorkspaceID
	RunID        common.RunID
	GoalID       common.GoalID
	DoDID        common.DoDID
	State        common.ResultState
	Title        string
	Summary      string
	SourceType   string
	SourceRef    string
	EvidenceRefs []string
	ArtifactRefs []common.ArtifactID
	CreatedAt    string
	UpdatedAt    string
}
