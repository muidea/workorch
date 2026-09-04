package file

import (
	"io/fs"
	"path/filepath"
	"sync"
)

const (
	defaultMaxReadBytes                                = 64 * 1024
	defaultMaxEntries                                  = 500
	runtimeFileSelectorWorkspaceRelativeNewTarget      = "workspace-relative-new-file-target"
	runtimeFileSelectorWorkspaceRelativeExistingTarget = "workspace-relative-existing-file-target"
	runtimeFileTargetSelectorUniqueWorkspaceEntry      = "file/unique_workspace_entry"
	runtimeFileTargetSelectorWorkspaceDeliverable      = "file/workspace_deliverable"
	runtimeFileTargetSelectorProfileCandidates         = "file/profile_declared_candidates"
)

type RuntimeAdapter struct {
	walkDir           func(string, fs.WalkDirFunc) error
	streamHashAndType func(string) (string, bool, error)
	workspaceEffects  sync.Map // canonical workspace root -> *sync.Mutex
}

func NewRuntimeAdapter() *RuntimeAdapter {
	return &RuntimeAdapter{
		walkDir:           filepath.WalkDir,
		streamHashAndType: streamFileHashAndBinary,
	}
}

func (a *RuntimeAdapter) lockWorkspaceEffect(workspaceDirectory string) func() {
	key, err := filepath.Abs(workspaceDirectory)
	if err != nil {
		key = filepath.Clean(workspaceDirectory)
	}
	value, _ := a.workspaceEffects.LoadOrStore(key, &sync.Mutex{})
	lock := value.(*sync.Mutex)
	lock.Lock()
	return lock.Unlock
}
