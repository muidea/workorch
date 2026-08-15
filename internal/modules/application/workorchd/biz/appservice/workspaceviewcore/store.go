package workspaceviewcore

import (
	"sync"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

type Store struct {
	mu      sync.RWMutex
	state   string
	errText string
	// revisionEpoch is a process-local floor for WorkspaceRevision/ActivityRevision.
	// It is assigned once at NewStore from a high-resolution wall-clock base so that
	// after server/store restart, newly assigned revisions always compare greater than
	// any pre-restart AppliedRevision held by a long-lived remote TUI (CR01).
	// Within a single process, advanceEntryRevision still guarantees monotonic growth
	// against the previous entry revision, source event revision, and activity revision.
	revisionEpoch int64
	// streamGeneration creates an explicit revision domain for this in-memory
	// projection store. It prevents clients from interpreting a restarted
	// process's sequence as an older sequence from the previous process.
	streamGeneration string
	// highWaterMark is the highest transaction revision assigned by this store
	// process. advanceEntryRevision never returns below highWaterMark+1, so
	// in-process counters stay monotonic even when wall-clock epoch is stagnant (FR07).
	highWaterMark           int64
	globalRevision          int64
	sourceRevision          int64
	projectedSourceRevision int64
	updatedAt               time.Time
	rebuiltAt               time.Time
	workspaceOrder          []string
	workspaces              map[string]*workspaceProjectionEntry
	globalStale             bool
	globalStaleReason       string
	// Current-generation projection health. Metrics counters never decide ready/degraded.
	globalFailure     *ProjectionFailure
	workspaceFailures map[string]ProjectionFailure
}

type workspaceProjectionEntry struct {
	Workspace    appmodelcore.Workspace
	HasWorkspace bool
	Snapshot     readmodelcore.WorkspaceSnapshotState
	HasSnapshot  bool
	// Activity is the store-owned bounded activity projection for this workspace.
	// ActivityRevision is assigned in the snapshot transaction and is the only
	// authority for activity dirty/stale gates. RuntimeView read only paginates.
	Activity                projectioncontract.WorkspaceActivityFeed
	HasActivity             bool
	ActivityRevision        int64
	Revision                int64
	SourceRevision          int64
	ProjectedSourceRevision int64
	Stale                   bool
	StaleReason             string
	Error                   string
	RebuiltAt               time.Time
}
