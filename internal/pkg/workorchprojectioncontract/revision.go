// Package workorchprojectioncontract defines immutable values shared by
// projection producers and process-facing projection consumers.
package workorchprojectioncontract

// WorkspaceRefreshRevision keeps independent projection revision domains in
// one value. It carries no comparison or state-transition behavior.
type WorkspaceRefreshRevision struct {
	StreamGeneration        string `json:"streamGeneration,omitempty"`
	StreamRevision          int64  `json:"streamRevision,omitempty"`
	WorkspaceRevision       int64  `json:"workspaceRevision,omitempty"`
	ProjectedSourceRevision int64  `json:"projectedSourceRevision,omitempty"`
	RuntimeStateRevision    uint64 `json:"runtimeStateRevision,omitempty"`
	EvidenceSequence        uint64 `json:"evidenceSequence,omitempty"`
	ActivityRevision        int64  `json:"activityRevision,omitempty"`
}
