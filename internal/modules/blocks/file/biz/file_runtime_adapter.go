package file

const (
	defaultMaxReadBytes                           = 64 * 1024
	defaultMaxEntries                             = 500
	runtimeFileSelectorWorkspaceRelativeNewTarget = "workspace-relative-new-file-target"
	runtimeFileTargetSelectorUniqueWorkspaceEntry = "file/unique_workspace_entry"
	runtimeFileTargetSelectorWorkspaceDeliverable = "file/workspace_deliverable"
	runtimeFileTargetSelectorProfileCandidates    = "file/profile_declared_candidates"
)

type RuntimeAdapter struct{}

func NewRuntimeAdapter() *RuntimeAdapter {
	return &RuntimeAdapter{}
}
