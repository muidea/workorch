package run

import (
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type testWorkspaceRepositoryReader struct {
	repository.WorkspaceRepository
}

// NewService is intentionally test-only. Production Run assembly must use
// NewServiceWithEventHub so cross-component reads and commands cannot bypass
// the component EventHub contracts.
func NewService(runs repository.RunRepository, workspaces repository.WorkspaceRepository, orchestrator orchestrator, commandHub magicevent.Hub) *Service {
	var reader workspaceReader
	if workspaces != nil {
		reader = testWorkspaceRepositoryReader{WorkspaceRepository: workspaces}
	}
	return newServiceWithPorts(runs, reader, orchestrator, eventRunStateNotifier{hub: commandHub})
}
