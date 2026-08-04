package workspaceviewcore

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
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

func (e *workspaceProjectionEntry) syncSnapshotWorkspace() {
	if e == nil || !e.HasSnapshot || !e.HasWorkspace {
		return
	}
	e.Snapshot.Workspace = workspaceWithSnapshotGovernance(e.Workspace, e.Snapshot)
	e.Snapshot = readmodelprojection.NormalizeWorkspaceGovernanceProjection(e.Snapshot)
	e.Workspace = cloneWorkspace(e.Snapshot.Workspace)
}

func workspaceWithSnapshotGovernance(workspace appmodelcore.Workspace, snapshot readmodelcore.WorkspaceSnapshotState) appmodelcore.Workspace {
	next := cloneWorkspace(workspace)
	if strings.TrimSpace(next.ID) == "" {
		next.ID = snapshot.Workspace.ID
	}
	activeRunID := strings.TrimSpace(next.ActiveRunID)
	if activeRunID == "" && snapshot.Run != nil {
		activeRunID = strings.TrimSpace(snapshot.Run.ID)
	}
	blockerRunID := activeRunID
	if blockerRunID == "" {
		blockerRunID = strings.TrimSpace(snapshot.Workspace.ActiveRunID)
	}
	if ticketID, ok := snapshotDecisionBlocker(snapshot, blockerRunID); ok {
		next.State = string(common.WorkspaceStateActive)
		if blockerRunID != "" {
			next.ActiveRunID = blockerRunID
		}
		next.BlockingTicketID = ticketID
		next.BlockingApprovalID = ""
		return next
	}
	if approvalID, ok := snapshotApprovalBlocker(snapshot, blockerRunID); ok {
		next.State = string(common.WorkspaceStateBlockedOnApproval)
		if blockerRunID != "" {
			next.ActiveRunID = blockerRunID
		}
		next.BlockingApprovalID = approvalID
		next.BlockingTicketID = ""
		return next
	}
	if activeRunID != "" {
		next.ActiveRunID = activeRunID
	}
	return next
}

func snapshotDecisionBlocker(snapshot readmodelcore.WorkspaceSnapshotState, activeRunID string) (string, bool) {
	workspaceID := strings.TrimSpace(snapshot.Workspace.ID)
	activeRunID = strings.TrimSpace(activeRunID)
	if activeRunID == "" && snapshot.Run != nil {
		activeRunID = strings.TrimSpace(snapshot.Run.ID)
	}
	preferredID := firstNonEmptyViewString(snapshot.Workspace.BlockingTicketID)
	if snapshot.Run != nil {
		preferredID = firstNonEmptyViewString(preferredID, snapshot.Run.BlockingTicketID)
	}
	if readmodelprojection.HasOpenDecisionBlocker(snapshot.Decisions, workspaceID, activeRunID, preferredID) {
		return preferredID, true
	}
	for _, item := range snapshot.Decisions {
		if strings.TrimSpace(item.WorkspaceID) == workspaceID &&
			strings.TrimSpace(item.RunID) == activeRunID &&
			strings.TrimSpace(item.State) == string(common.DecisionTicketStateOpen) &&
			strings.TrimSpace(item.ID) != "" {
			return strings.TrimSpace(item.ID), true
		}
	}
	return "", false
}

func snapshotApprovalBlocker(snapshot readmodelcore.WorkspaceSnapshotState, activeRunID string) (string, bool) {
	workspaceID := strings.TrimSpace(snapshot.Workspace.ID)
	activeRunID = strings.TrimSpace(activeRunID)
	if activeRunID == "" && snapshot.Run != nil {
		activeRunID = strings.TrimSpace(snapshot.Run.ID)
	}
	preferredID := firstNonEmptyViewString(snapshot.Workspace.BlockingApprovalID)
	if snapshot.Run != nil {
		preferredID = firstNonEmptyViewString(preferredID, snapshot.Run.BlockingApprovalID)
	}
	if readmodelprojection.HasOpenApprovalBlocker(snapshot.Approvals, workspaceID, activeRunID, preferredID) {
		return preferredID, true
	}
	for _, item := range snapshot.Approvals {
		if strings.TrimSpace(item.WorkspaceID) == workspaceID &&
			strings.TrimSpace(item.RunID) == activeRunID &&
			strings.TrimSpace(item.State) == string(common.ApprovalStatePending) &&
			strings.TrimSpace(item.ID) != "" {
			return strings.TrimSpace(item.ID), true
		}
	}
	return "", false
}

func firstNonEmptyViewString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func NewStore() *Store {
	return NewStoreWithRevisionFloor(newProjectionRevisionEpoch())
}

// NewStoreWithRevisionFloor constructs a store whose process-local revision
// epoch is at least floor. Used by production NewStore (wall-clock) and by
// tests/injection to model clock skew, future epochs, and durable floors (FR07).
func NewStoreWithRevisionFloor(floor int64) *Store {
	if floor < 1 {
		floor = 1
	}
	return &Store{
		state:             StateBuilding,
		revisionEpoch:     floor,
		streamGeneration:  newProjectionStreamGeneration(),
		highWaterMark:     floor - 1, // first assign yields >= floor
		globalRevision:    floor - 1, // FR05: stream/GlobalRevision shares process floor so SSE IDs do not restart at 1
		workspaces:        map[string]*workspaceProjectionEntry{},
		workspaceFailures: map[string]ProjectionFailure{},
	}
}

func newProjectionStreamGeneration() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err == nil {
		return hex.EncodeToString(bytes)
	}
	// crypto/rand failure is exceptionally rare. The timestamp is only a
	// uniqueness fallback for the generation token; it is never used as a
	// comparable revision authority.
	return fmt.Sprintf("fallback-%d", time.Now().UTC().UnixNano())
}

// SeedRevisionFloor raises the process revision floor without resetting
// highWaterMark downward. Call after observing durable source revisions or a
// persisted checkpoint so restart cannot assign below known history (FR07).
func (s *Store) SeedRevisionFloor(floor int64) {
	if s == nil || floor < 1 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if floor > s.revisionEpoch {
		s.revisionEpoch = floor
	}
	if floor-1 > s.highWaterMark {
		s.highWaterMark = floor - 1
	}
	// FR05: keep stream/GlobalRevision at or above the seeded floor so SSE
	// identities cannot restart below an observed pre-restart Last-Event-ID.
	if floor-1 > s.globalRevision {
		s.globalRevision = floor - 1
	}
}

// newProjectionRevisionEpoch returns a process-local revision floor.
// Wall-clock nanoseconds are a convenience seed only; advanceEntryRevision
// also floors against highWaterMark, previous entry revisions, source event
// revision, and RuntimeStateRevision so assignment does not rely solely on
// wall-clock progression (FR07).
func newProjectionRevisionEpoch() int64 {
	epoch := time.Now().UTC().UnixNano()
	if epoch < 1 {
		return 1
	}
	return epoch
}

// advanceEntryRevision assigns the next workspace transaction revision for entry.
// Floor sources (max of):
//  1. process revisionEpoch (restart continuity seed)
//  2. store highWaterMark + 1 (strict in-process monotonic, FR07)
//  3. previous entry.Revision + 1
//  4. previous ActivityRevision + 1
//  5. SourceRevision / store sourceRevision (durable eventstream floor)
//  6. RuntimeStateRevision from the current snapshot when present
//
// The result is always > 0, never lower than any floor, and updates highWaterMark.
func (s *Store) advanceEntryRevision(entry *workspaceProjectionEntry) int64 {
	if entry == nil {
		return 0
	}
	next := int64(1)
	if s != nil && s.revisionEpoch > 0 {
		next = s.revisionEpoch
	}
	if s != nil && s.highWaterMark+1 > next {
		next = s.highWaterMark + 1
	}
	if entry.Revision+1 > next {
		next = entry.Revision + 1
	}
	if entry.ActivityRevision+1 > next {
		next = entry.ActivityRevision + 1
	}
	if entry.SourceRevision > next {
		next = entry.SourceRevision
	}
	if s != nil && s.sourceRevision > next {
		next = s.sourceRevision
	}
	if entry.HasSnapshot {
		if runtime := entry.Snapshot.TaskProgress.Runtime; runtime != nil {
			if r := int64(runtime.StateRevision); r > next {
				next = r
			}
		}
	}
	if next < 1 {
		next = 1
	}
	entry.Revision = next
	if s != nil && next > s.highWaterMark {
		s.highWaterMark = next
	}
	return next
}

// advanceGlobalRevision advances the projection stream/global revision using the
// same process floor and high-water rules as entry transaction revisions (FR05).
// This keeps SSE Last-Event-ID / streamRevision from silently restarting at small
// counters after server restart.
func (s *Store) advanceGlobalRevision() int64 {
	if s == nil {
		return 0
	}
	next := s.globalRevision + 1
	if s.revisionEpoch > next {
		next = s.revisionEpoch
	}
	if s.highWaterMark+1 > next {
		next = s.highWaterMark + 1
	}
	if next < 1 {
		next = 1
	}
	s.globalRevision = next
	if next > s.highWaterMark {
		s.highWaterMark = next
	}
	return next
}

func (s *Store) Status() Status {
	if s == nil {
		return Status{State: StateFailed, Error: "workspace projection store is not configured"}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.statusLocked()
}

func (s *Store) StatusForWorkspace(workspaceID string) Status {
	if s == nil {
		return Status{State: StateFailed, Error: "workspace projection store is not configured"}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.statusForWorkspaceLocked(workspaceID)
}

func (s *Store) statusLocked() Status {
	state := s.state
	failedIDs, latestError, degradedSince := s.currentGenerationFailuresLocked()
	degraded := s.globalFailure != nil || len(failedIDs) > 0 || s.hasWorkspaceErrorLocked()
	// Lifecycle states take priority; ready/stale may upgrade to degraded from current failures.
	switch state {
	case StateStopped, StateFailed, StateBuilding:
		// keep lifecycle state
	default:
		if degraded {
			state = StateDegraded
		} else if s.globalStale || s.hasStaleWorkspaceLocked() {
			state = StateStale
		} else if state == StateDegraded {
			// sticky degraded without current failures recovers to ready
			state = StateReady
		}
	}
	errText := s.errText
	if errText == "" {
		errText = latestError
	}
	return Status{
		State:                   state,
		StreamGeneration:        s.streamGeneration,
		GlobalRevision:          s.globalRevision,
		SourceEventRevision:     s.sourceRevision,
		ProjectedSourceRevision: s.projectedSourceRevision,
		WorkspaceTotal:          s.workspaceTotalLocked(),
		UpdatedAt:               formatProjectionTime(s.updatedAt),
		RebuiltAt:               formatProjectionTime(s.rebuiltAt),
		Degraded:                degraded || state == StateDegraded,
		StaleReason:             s.globalStaleReason,
		Error:                   errText,
		FailedWorkspaceIds:      failedIDs,
		DegradedSince:           formatProjectionTime(degradedSince),
	}
}

func (s *Store) statusForWorkspaceLocked(workspaceID string) Status {
	if workspaceID == "" {
		return s.statusLocked()
	}
	state := s.state
	if state == StateDegraded || state == StateFailed {
		state = StateReady
	}
	entry := s.workspaces[workspaceID]
	if (entry == nil || !entry.HasWorkspace) && s.state != StateBuilding && s.state != StateStopped {
		state = StateFailed
	}
	var errText, staleReason string
	var workspaceRevision, sourceRevision, projectedSourceRevision int64
	var rebuiltAt time.Time
	var stale bool
	if entry != nil {
		errText = entry.Error
		staleReason = entry.StaleReason
		workspaceRevision = entry.Revision
		sourceRevision = entry.SourceRevision
		projectedSourceRevision = entry.ProjectedSourceRevision
		rebuiltAt = entry.RebuiltAt
		stale = entry.Stale
	}
	if sourceRevision == 0 {
		sourceRevision = s.sourceRevision
	}
	if projectedSourceRevision == 0 {
		projectedSourceRevision = s.projectedSourceRevision
		if !stale && (projectedSourceRevision == 0 || (strings.TrimSpace(errText) == "" && projectedSourceRevision < sourceRevision)) {
			projectedSourceRevision = sourceRevision
		}
	}
	degraded := strings.TrimSpace(errText) != ""
	if degraded {
		state = StateDegraded
	} else if stale && state == StateReady {
		state = StateStale
	}
	var failedIDs []string
	var degradedSince time.Time
	if degraded {
		failedIDs = []string{workspaceID}
		if failure, ok := s.workspaceFailures[workspaceID]; ok && failure.Generation == s.streamGeneration {
			degradedSince = failure.Since
			if errText == "" {
				errText = failure.Error
			}
		}
	}
	return Status{
		State:                   state,
		StreamGeneration:        s.streamGeneration,
		GlobalRevision:          s.globalRevision,
		WorkspaceRevision:       workspaceRevision,
		SourceEventRevision:     sourceRevision,
		ProjectedSourceRevision: projectedSourceRevision,
		WorkspaceTotal:          s.workspaceTotalLocked(),
		UpdatedAt:               formatProjectionTime(s.updatedAt),
		RebuiltAt:               formatProjectionTime(rebuiltAt),
		Degraded:                degraded,
		StaleReason:             staleReason,
		Error:                   errText,
		FailedWorkspaceIds:      failedIDs,
		DegradedSince:           formatProjectionTime(degradedSince),
	}
}

func (s *Store) setBuilding() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.workspaceTotalLocked() > 0 {
		s.state = StateStale
	} else {
		s.state = StateBuilding
	}
	s.errText = ""
	s.updatedAt = time.Now().UTC()
}

func (s *Store) setStopped() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = StateStopped
	s.updatedAt = time.Now().UTC()
}

func (s *Store) MarkStale(workspaceID string, sourceRevision int64) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if sourceRevision > s.sourceRevision {
		s.sourceRevision = sourceRevision
	}
	if strings.TrimSpace(workspaceID) == "" {
		s.globalStale = true
		s.globalStaleReason = "source_event"
	} else {
		entry := s.entryForUpdateLocked(workspaceID)
		entry.Stale = true
		entry.StaleReason = "source_event"
		if sourceRevision > entry.SourceRevision {
			entry.SourceRevision = sourceRevision
		}
	}
	s.updatedAt = time.Now().UTC()
}

func (s *Store) WorkspaceIDs() []string {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	ret := append([]string(nil), s.workspaceOrder...)
	sort.Strings(ret)
	return ret
}

// ObserveSourceHighWatermark compares the projection with the EventStream
// owner's authoritative persisted revision. It returns true only when a
// workspace rebuild is required.
func (s *Store) ObserveSourceHighWatermark(workspaceID string, sourceRevision int64) bool {
	if s == nil || strings.TrimSpace(workspaceID) == "" || sourceRevision < 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.entryForUpdateLocked(workspaceID)
	if sourceRevision > s.sourceRevision {
		s.sourceRevision = sourceRevision
	}
	if sourceRevision > entry.SourceRevision {
		entry.SourceRevision = sourceRevision
	}
	if sourceRevision <= entry.ProjectedSourceRevision {
		return false
	}
	entry.Stale = true
	entry.StaleReason = "source_high_watermark"
	s.updatedAt = time.Now().UTC()
	return true
}

func (s *Store) ReplaceAll(workspaces []appmodelcore.Workspace, snapshots map[string]readmodelcore.WorkspaceSnapshotState, degraded error) {
	if s == nil {
		return
	}
	now := time.Now().UTC()
	nextWorkspaces := make(map[string]*workspaceProjectionEntry, len(workspaces))
	nextOrder := make([]string, 0, len(workspaces))
	for _, item := range workspaces {
		if strings.TrimSpace(item.ID) == "" {
			continue
		}
		entry := s.cloneEntryForReplace(item.ID)
		if snapshot, ok := snapshots[item.ID]; ok && strings.TrimSpace(snapshot.Workspace.ID) != "" {
			snapshot = readmodelprojection.ProjectWorkspaceSnapshotDetail(snapshot)
			item = snapshot.Workspace
			entry.Snapshot = cloneWorkspaceSnapshot(snapshot)
			entry.HasSnapshot = true
			entry.ProjectedSourceRevision = entry.SourceRevision
			entry.RebuiltAt = now
			entry.Stale = false
			entry.StaleReason = ""
			entry.Error = ""
		} else if degraded != nil {
			entry.Stale = true
			entry.StaleReason = "rebuild_failed"
			entry.Error = degraded.Error()
		}
		entry.Workspace = cloneWorkspace(item)
		entry.HasWorkspace = true
		s.advanceEntryRevision(entry)
		nextWorkspaces[item.ID] = entry
		nextOrder = append(nextOrder, item.ID)
	}
	sort.Strings(nextOrder)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.workspaces = nextWorkspaces
	s.workspaceOrder = nextOrder
	s.advanceGlobalRevision()
	s.refreshProjectedSourceRevisionLocked()
	s.globalStale = false
	s.globalStaleReason = ""
	if degraded != nil {
		s.state = StateReady // statusLocked upgrades to degraded from failure set
		s.errText = degraded.Error()
		s.recordGlobalFailureLocked(degraded)
		// Mark remaining workspace failures for entries without snapshots.
		for id, entry := range s.workspaces {
			if entry != nil && strings.TrimSpace(entry.Error) != "" {
				s.recordWorkspaceFailureLocked(id, fmt.Errorf("%s", entry.Error))
			}
		}
	} else {
		s.state = StateReady
		s.clearAllCurrentFailuresLocked()
	}
	s.updatedAt = now
	s.rebuiltAt = now
}

func (s *Store) ReplaceWorkspaceList(workspaces []appmodelcore.Workspace, degraded error) {
	if s == nil {
		return
	}
	now := time.Now().UTC()
	nextWorkspaces := make(map[string]*workspaceProjectionEntry, len(workspaces))
	nextOrder := make([]string, 0, len(workspaces))
	for _, item := range workspaces {
		if strings.TrimSpace(item.ID) == "" {
			continue
		}
		entry := s.cloneEntryForReplace(item.ID)
		entry.Workspace = cloneWorkspace(item)
		entry.HasWorkspace = true
		entry.syncSnapshotWorkspace()
		s.advanceEntryRevision(entry)
		nextWorkspaces[item.ID] = entry
		nextOrder = append(nextOrder, item.ID)
	}
	sort.Strings(nextOrder)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.workspaces = nextWorkspaces
	s.workspaceOrder = nextOrder
	s.advanceGlobalRevision()
	s.globalStale = false
	s.globalStaleReason = ""
	// Drop failure records for workspaces no longer present in the list.
	for workspaceID := range s.workspaceFailures {
		if _, ok := nextWorkspaces[workspaceID]; !ok {
			delete(s.workspaceFailures, workspaceID)
		}
	}
	if degraded != nil {
		s.state = StateReady
		s.errText = degraded.Error()
		s.recordGlobalFailureLocked(degraded)
	} else {
		s.state = StateReady
		// List-only refresh keeps remaining workspace failures; only clears global list loader failure.
		s.globalFailure = nil
		if s.errText != "" && (s.workspaceFailures == nil || len(s.workspaceFailures) == 0) {
			s.errText = ""
		}
	}
	s.updatedAt = now
	s.rebuiltAt = now
}

func (s *Store) setFailed(err error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = StateFailed
	if err != nil {
		s.errText = err.Error()
		s.recordGlobalFailureLocked(err)
	}
	s.updatedAt = time.Now().UTC()
}

func (s *Store) UpsertSnapshot(snapshot readmodelcore.WorkspaceSnapshotState) {
	if s == nil || strings.TrimSpace(snapshot.Workspace.ID) == "" {
		return
	}
	snapshot = readmodelprojection.ProjectWorkspaceSnapshotDetail(snapshot)
	workspaceID := snapshot.Workspace.ID
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	entry := s.entryForUpdateLocked(workspaceID)
	if snapshot.RunCommand == nil && entry.HasSnapshot && entry.Snapshot.RunCommand != nil {
		command := *entry.Snapshot.RunCommand
		snapshot.RunCommand = &command
	}
	// Capture previous activity before replacing snapshot so revision can stay stable
	// when content is unchanged within this projection transaction.
	previousActivity := entry.Activity
	entry.Workspace = cloneWorkspace(snapshot.Workspace)
	entry.HasWorkspace = true
	if !stringSliceContains(s.workspaceOrder, workspaceID) {
		s.workspaceOrder = append(s.workspaceOrder, workspaceID)
		sort.Strings(s.workspaceOrder)
	}
	entry.Snapshot = cloneWorkspaceSnapshot(snapshot)
	entry.HasSnapshot = true
	s.advanceEntryRevision(entry)
	s.advanceGlobalRevision()
	// Commit activity feed in the same transaction as the snapshot revision.
	// Content changes advance ActivityRevision to this entry.Revision; unchanged keeps previous.
	candidateContent := readmodelprojection.BuildWorkspaceActivityFeedContent(entry.Snapshot)
	committed := readmodelprojection.CommitActivityFeedTransaction(previousActivity, candidateContent, entry.Revision)
	committed.StreamGeneration = s.streamGeneration
	entry.Activity = cloneWorkspaceActivityFeed(committed)
	entry.HasActivity = true
	entry.ActivityRevision = committed.ActivityRevision
	entry.ProjectedSourceRevision = entry.SourceRevision
	entry.RebuiltAt = now
	entry.Stale = false
	entry.StaleReason = ""
	entry.Error = ""
	s.clearWorkspaceFailureLocked(workspaceID)
	s.refreshProjectedSourceRevisionLocked()
	s.globalStale = false
	s.globalStaleReason = ""
	s.globalFailure = nil
	if s.state != StateStopped && s.state != StateFailed {
		s.state = StateReady
	}
	if len(s.workspaceFailures) == 0 {
		s.errText = ""
	}
	s.updatedAt = now
}

// GetActivity returns the store-owned activity feed for a workspace.
// RuntimeView read must page this feed and must not regenerate ActivityRevision.
func (s *Store) GetActivity(workspaceID string) (projectioncontract.WorkspaceActivityFeed, bool) {
	if s == nil {
		return projectioncontract.WorkspaceActivityFeed{}, false
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return projectioncontract.WorkspaceActivityFeed{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry := s.workspaces[workspaceID]
	if entry == nil || !entry.HasActivity {
		return projectioncontract.WorkspaceActivityFeed{}, false
	}
	return cloneWorkspaceActivityFeed(entry.Activity), true
}

func (s *Store) UpsertWorkspace(workspace appmodelcore.Workspace) {
	if s == nil || strings.TrimSpace(workspace.ID) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	entry := s.entryForUpdateLocked(workspace.ID)
	entry.Workspace = cloneWorkspace(workspace)
	entry.HasWorkspace = true
	entry.syncSnapshotWorkspace()
	if !stringSliceContains(s.workspaceOrder, workspace.ID) {
		s.workspaceOrder = append(s.workspaceOrder, workspace.ID)
		sort.Strings(s.workspaceOrder)
	}
	s.advanceEntryRevision(entry)
	s.advanceGlobalRevision()
	entry.ProjectedSourceRevision = entry.SourceRevision
	entry.RebuiltAt = now
	entry.Stale = false
	entry.StaleReason = ""
	entry.Error = ""
	s.clearWorkspaceFailureLocked(workspace.ID)
	s.refreshProjectedSourceRevisionLocked()
	s.globalStale = false
	s.globalStaleReason = ""
	s.globalFailure = nil
	if s.state != StateStopped && s.state != StateFailed {
		s.state = StateReady
	}
	if len(s.workspaceFailures) == 0 {
		s.errText = ""
	}
	s.updatedAt = now
}

func (s *Store) ApplyRunCommandAccepted(update RunCommandAcceptedUpdate) bool {
	if s == nil || strings.TrimSpace(update.WorkspaceID) == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.workspaces[update.WorkspaceID]
	if entry == nil || !entry.HasWorkspace {
		return false
	}
	now := time.Now().UTC()
	snapshot := entry.Snapshot
	if !entry.HasSnapshot || strings.TrimSpace(snapshot.Workspace.ID) == "" {
		snapshot.Workspace = cloneWorkspace(entry.Workspace)
	}
	acceptedAt := strings.TrimSpace(update.AcceptedAt)
	if acceptedAt == "" {
		acceptedAt = formatProjectionTime(now)
	}
	queuedAt := ""
	if snapshot.RunCommand != nil && strings.TrimSpace(snapshot.RunCommand.RequestID) == strings.TrimSpace(update.RequestID) {
		queuedAt = strings.TrimSpace(snapshot.RunCommand.QueuedAt)
	}
	snapshot.RunCommand = &projectioncontract.RunCommandStatus{
		WorkspaceID:        strings.TrimSpace(update.WorkspaceID),
		RunID:              strings.TrimSpace(update.RunID),
		State:              "accepted",
		Intent:             strings.TrimSpace(update.Intent),
		Reason:             strings.TrimSpace(update.Reason),
		Disposition:        strings.TrimSpace(update.Disposition),
		Diagnostic:         strings.TrimSpace(update.Diagnostic),
		RequestID:          strings.TrimSpace(update.RequestID),
		IdempotencyKey:     strings.TrimSpace(update.IdempotencyKey),
		Source:             strings.TrimSpace(update.Source),
		DefinitionRevision: strings.TrimSpace(update.DefinitionRevision),
		DecisionID:         strings.TrimSpace(update.DecisionID),
		QueuedAt:           queuedAt,
		AcceptedAt:         acceptedAt,
		Revision:           update.Revision,
	}
	entry.Snapshot = snapshot
	entry.Workspace = cloneWorkspace(snapshot.Workspace)
	entry.HasWorkspace = true
	entry.HasSnapshot = true
	s.advanceEntryRevision(entry)
	s.advanceGlobalRevision()
	if update.Revision > entry.SourceRevision {
		entry.SourceRevision = update.Revision
	}
	entry.ProjectedSourceRevision = entry.SourceRevision
	entry.RebuiltAt = now
	entry.Stale = false
	entry.StaleReason = ""
	entry.Error = ""
	s.refreshProjectedSourceRevisionLocked()
	s.globalStale = false
	s.globalStaleReason = ""
	s.state = StateReady
	s.errText = ""
	s.updatedAt = now
	return true
}

func (s *Store) ApplyRunCommandQueued(update RunCommandQueuedUpdate) bool {
	if s == nil || strings.TrimSpace(update.WorkspaceID) == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.workspaces[update.WorkspaceID]
	if entry == nil || !entry.HasWorkspace {
		return false
	}
	now := time.Now().UTC()
	snapshot := entry.Snapshot
	if !entry.HasSnapshot || strings.TrimSpace(snapshot.Workspace.ID) == "" {
		snapshot.Workspace = cloneWorkspace(entry.Workspace)
	}
	queuedAt := strings.TrimSpace(update.QueuedAt)
	if queuedAt == "" {
		queuedAt = formatProjectionTime(now)
	}
	snapshot.RunCommand = &projectioncontract.RunCommandStatus{
		WorkspaceID:        strings.TrimSpace(update.WorkspaceID),
		RunID:              strings.TrimSpace(update.RunID),
		State:              "queued",
		Intent:             strings.TrimSpace(update.Intent),
		Reason:             strings.TrimSpace(update.Reason),
		RequestID:          strings.TrimSpace(update.RequestID),
		IdempotencyKey:     strings.TrimSpace(update.IdempotencyKey),
		Source:             strings.TrimSpace(update.Source),
		DefinitionRevision: strings.TrimSpace(update.DefinitionRevision),
		DecisionID:         strings.TrimSpace(update.DecisionID),
		QueuedAt:           queuedAt,
		Revision:           update.Revision,
	}
	entry.Snapshot = snapshot
	entry.Workspace = cloneWorkspace(snapshot.Workspace)
	entry.HasWorkspace = true
	entry.HasSnapshot = true
	s.advanceEntryRevision(entry)
	s.advanceGlobalRevision()
	if update.Revision > entry.SourceRevision {
		entry.SourceRevision = update.Revision
	}
	entry.ProjectedSourceRevision = entry.SourceRevision
	entry.RebuiltAt = now
	entry.Stale = false
	entry.StaleReason = ""
	entry.Error = ""
	s.refreshProjectedSourceRevisionLocked()
	s.globalStale = false
	s.globalStaleReason = ""
	s.state = StateReady
	s.errText = ""
	s.updatedAt = now
	return true
}

func (s *Store) ApplyRunCommandRejected(update RunCommandRejectedUpdate) bool {
	if s == nil || strings.TrimSpace(update.WorkspaceID) == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.workspaces[update.WorkspaceID]
	if entry == nil || !entry.HasWorkspace {
		return false
	}
	now := time.Now().UTC()
	snapshot := entry.Snapshot
	if !entry.HasSnapshot || strings.TrimSpace(snapshot.Workspace.ID) == "" {
		snapshot.Workspace = cloneWorkspace(entry.Workspace)
	}
	rejectedAt := strings.TrimSpace(update.RejectedAt)
	if rejectedAt == "" {
		rejectedAt = formatProjectionTime(now)
	}
	queuedAt := ""
	if snapshot.RunCommand != nil && strings.TrimSpace(snapshot.RunCommand.RequestID) == strings.TrimSpace(update.RequestID) {
		queuedAt = strings.TrimSpace(snapshot.RunCommand.QueuedAt)
	}
	snapshot.RunCommand = &projectioncontract.RunCommandStatus{
		WorkspaceID:        strings.TrimSpace(update.WorkspaceID),
		RunID:              strings.TrimSpace(update.RunID),
		State:              "rejected",
		Intent:             strings.TrimSpace(update.Intent),
		Reason:             strings.TrimSpace(update.Reason),
		Disposition:        strings.TrimSpace(update.Disposition),
		Diagnostic:         strings.TrimSpace(update.Diagnostic),
		RequestID:          strings.TrimSpace(update.RequestID),
		IdempotencyKey:     strings.TrimSpace(update.IdempotencyKey),
		Source:             strings.TrimSpace(update.Source),
		DefinitionRevision: strings.TrimSpace(update.DefinitionRevision),
		DecisionID:         strings.TrimSpace(update.DecisionID),
		QueuedAt:           queuedAt,
		RejectedAt:         rejectedAt,
		Error:              strings.TrimSpace(update.Error),
		Revision:           update.Revision,
	}
	entry.Snapshot = snapshot
	entry.Workspace = cloneWorkspace(snapshot.Workspace)
	entry.HasWorkspace = true
	entry.HasSnapshot = true
	s.advanceEntryRevision(entry)
	s.advanceGlobalRevision()
	if update.Revision > entry.SourceRevision {
		entry.SourceRevision = update.Revision
	}
	entry.ProjectedSourceRevision = entry.SourceRevision
	entry.RebuiltAt = now
	entry.Stale = false
	entry.StaleReason = ""
	entry.Error = ""
	s.refreshProjectedSourceRevisionLocked()
	s.globalStale = false
	s.globalStaleReason = ""
	s.state = StateReady
	s.errText = ""
	s.updatedAt = now
	return true
}

func (s *Store) MarkWorkspaceDegraded(workspaceID string, err error) {
	if s == nil || strings.TrimSpace(workspaceID) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.entryForUpdateLocked(workspaceID)
	if err != nil {
		entry.Error = err.Error()
		s.recordWorkspaceFailureLocked(workspaceID, err)
	}
	entry.Stale = true
	entry.StaleReason = "rebuild_failed"
	s.advanceGlobalRevision()
	// Do not sticky-set StateDegraded; statusLocked derives degraded from current failure set.
	if s.state != StateBuilding && s.state != StateStopped && s.state != StateFailed {
		s.state = StateReady
	}
	s.updatedAt = time.Now().UTC()
}

func (s *Store) DeleteWorkspace(workspaceID string) {
	if s == nil || strings.TrimSpace(workspaceID) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.workspaces, workspaceID)
	s.clearWorkspaceFailureLocked(workspaceID)
	nextOrder := s.workspaceOrder[:0]
	for _, id := range s.workspaceOrder {
		if id != workspaceID {
			nextOrder = append(nextOrder, id)
		}
	}
	s.workspaceOrder = nextOrder
	s.advanceGlobalRevision()
	s.refreshProjectedSourceRevisionLocked()
	s.globalStale = false
	s.globalStaleReason = ""
	if s.state != StateStopped && s.state != StateFailed {
		s.state = StateReady
	}
	if len(s.workspaceFailures) == 0 && s.globalFailure == nil {
		s.errText = ""
	}
	s.updatedAt = time.Now().UTC()
}

func (s *Store) List() ([]appmodelcore.Workspace, Status, error) {
	if s == nil {
		return nil, Status{State: StateFailed}, common.ErrConflict
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	status := s.statusLocked()
	if s.state == StateBuilding || s.state == StateStopped {
		return nil, status, fmt.Errorf("%w: workspace projection %s", common.ErrConflict, s.state)
	}
	if s.state == StateFailed {
		return nil, status, fmt.Errorf("%w: workspace projection failed: %s", common.ErrConflict, s.errText)
	}
	ret := make([]appmodelcore.Workspace, 0, len(s.workspaceOrder))
	for _, id := range s.workspaceOrder {
		if entry := s.workspaces[id]; entry != nil && entry.HasWorkspace {
			item := cloneWorkspace(entry.Workspace)
			if entry.HasSnapshot {
				snapshot := normalizedEntryListSnapshot(entry)
				item = cloneWorkspace(snapshot.Workspace)
				item = compactWorkspaceListOverview(item)
				item.State = snapshot.Workspace.State
				item.BlockingTicketID = snapshot.Workspace.BlockingTicketID
				item.BlockingApprovalID = snapshot.Workspace.BlockingApprovalID
				item.ViewModel = appWorkspaceViewModelFromSnapshot(snapshot, s.statusForWorkspaceLocked(id))
			} else {
				item = compactWorkspaceListOverview(item)
				item.ViewModel = appWorkspaceViewModelFromStatus(item, s.statusForWorkspaceLocked(id))
			}
			ret = append(ret, item)
		}
	}
	return ret, status, nil
}

func compactWorkspaceListOverview(item appmodelcore.Workspace) appmodelcore.Workspace {
	item.ContextHints = nil
	item.EvidencePolicies = nil
	return item
}

func (s *Store) GetSnapshot(workspaceID string) (readmodelcore.WorkspaceSnapshotState, Status, error) {
	if s == nil {
		return readmodelcore.WorkspaceSnapshotState{}, Status{State: StateFailed}, common.ErrConflict
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	status := s.statusForWorkspaceLocked(workspaceID)
	if s.state == StateBuilding || s.state == StateStopped {
		return readmodelcore.WorkspaceSnapshotState{}, status, fmt.Errorf("%w: workspace projection %s", common.ErrConflict, s.state)
	}
	if s.state == StateFailed {
		return readmodelcore.WorkspaceSnapshotState{}, status, fmt.Errorf("%w: workspace projection failed: %s", common.ErrConflict, s.errText)
	}
	entry := s.workspaces[workspaceID]
	if entry == nil || !entry.HasSnapshot {
		if entry != nil && entry.HasWorkspace {
			return readmodelcore.WorkspaceSnapshotState{}, status, fmt.Errorf("%w: workspace projection detail not ready", common.ErrConflict)
		}
		return readmodelcore.WorkspaceSnapshotState{}, status, common.ErrNotFound
	}
	snapshot := normalizedEntrySnapshot(entry)
	snapshot.ViewModel = workspaceViewModelFromStatus(snapshot, status)
	return snapshot, status, nil
}

func (s *Store) GetSummarySnapshot(workspaceID string) (readmodelcore.WorkspaceSnapshotState, Status, error) {
	if s == nil {
		return readmodelcore.WorkspaceSnapshotState{}, Status{State: StateFailed}, common.ErrConflict
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	status := s.statusForWorkspaceLocked(workspaceID)
	if s.state == StateBuilding || s.state == StateStopped {
		return readmodelcore.WorkspaceSnapshotState{}, status, fmt.Errorf("%w: workspace projection %s", common.ErrConflict, s.state)
	}
	if s.state == StateFailed {
		return readmodelcore.WorkspaceSnapshotState{}, status, fmt.Errorf("%w: workspace projection failed: %s", common.ErrConflict, s.errText)
	}
	entry := s.workspaces[workspaceID]
	if entry == nil || !entry.HasWorkspace {
		return readmodelcore.WorkspaceSnapshotState{}, status, common.ErrNotFound
	}
	snapshot := readmodelcore.WorkspaceSnapshotState{Workspace: cloneWorkspace(entry.Workspace)}
	if entry.HasSnapshot {
		snapshot = normalizedEntrySummarySnapshot(entry)
	}
	snapshot.ViewModel = workspaceViewModelFromStatus(snapshot, status)
	return snapshot, status, nil
}

func normalizedEntryListSnapshot(entry *workspaceProjectionEntry) readmodelcore.WorkspaceSnapshotState {
	if entry == nil {
		return readmodelcore.WorkspaceSnapshotState{}
	}
	snapshot := readmodelcore.WorkspaceSnapshotState{Workspace: cloneWorkspace(entry.Workspace)}
	if !entry.HasSnapshot {
		return snapshot
	}
	source := entry.Snapshot
	snapshot.Workspace = cloneWorkspace(source.Workspace)
	snapshot.Run = source.Run
	snapshot.RunHeartbeat = source.RunHeartbeat
	snapshot.TaskProgress = source.TaskProgress
	snapshot.Decisions = append([]projectioncontract.Decision(nil), source.Decisions...)
	snapshot.Approvals = append([]projectioncontract.Approval(nil), source.Approvals...)
	if entry.HasWorkspace {
		snapshot.Workspace = workspaceWithSnapshotGovernance(entry.Workspace, snapshot)
	}
	return readmodelprojection.NormalizeWorkspaceGovernanceProjection(snapshot)
}

func normalizedEntrySummarySnapshot(entry *workspaceProjectionEntry) readmodelcore.WorkspaceSnapshotState {
	snapshot := normalizedEntryListSnapshot(entry)
	if entry == nil || !entry.HasSnapshot {
		return readmodelprojection.ProjectWorkspaceSnapshotSummary(snapshot)
	}
	source := entry.Snapshot
	snapshot.Outcome = source.Outcome
	snapshot.CoverageSummary = cloneStringAnyMap(source.CoverageSummary)
	snapshot.ConvergenceStatus = source.ConvergenceStatus
	if source.RunCommand != nil {
		status := *source.RunCommand
		snapshot.RunCommand = &status
	}
	snapshot.SchedulerDiagnostics = source.SchedulerDiagnostics
	snapshot.LLMUsageSummary = source.LLMUsageSummary
	return readmodelprojection.ProjectWorkspaceSnapshotSummary(snapshot)
}

func normalizedEntrySnapshot(entry *workspaceProjectionEntry) readmodelcore.WorkspaceSnapshotState {
	if entry == nil || !entry.HasSnapshot {
		return readmodelcore.WorkspaceSnapshotState{}
	}
	snapshot := cloneWorkspaceSnapshot(entry.Snapshot)
	if entry.HasWorkspace {
		snapshot.Workspace = workspaceWithSnapshotGovernance(entry.Workspace, snapshot)
	}
	return readmodelprojection.NormalizeWorkspaceGovernanceProjection(snapshot)
}

func (s *Store) NeedsSnapshot(workspaceID string) bool {
	if s == nil {
		return false
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.state == StateBuilding || s.state == StateStopped || s.state == StateFailed {
		return false
	}
	entry := s.workspaces[workspaceID]
	return entry != nil && entry.HasWorkspace && !entry.HasSnapshot
}

func workspaceViewModelFromStatus(snapshot readmodelcore.WorkspaceSnapshotState, status Status) projectioncontract.WorkspaceViewModel {
	revision := status.WorkspaceRevision
	if revision == 0 {
		revision = status.GlobalRevision
	}
	ret := projectioncontract.WorkspaceViewModel{
		WorkspaceID:             snapshot.Workspace.ID,
		State:                   status.State,
		PromptMode:              workspacePromptMode(snapshot),
		Revision:                revision,
		StreamGeneration:        status.StreamGeneration,
		GlobalRevision:          status.GlobalRevision,
		WorkspaceRevision:       status.WorkspaceRevision,
		SourceEventRevision:     status.SourceEventRevision,
		ProjectedSourceRevision: status.ProjectedSourceRevision,
		UpdatedAt:               status.UpdatedAt,
		RebuiltAt:               status.RebuiltAt,
		Degraded:                status.Degraded,
		StaleReason:             status.StaleReason,
		Error:                   status.Error,
		FailedWorkspaceIds:      append([]string(nil), status.FailedWorkspaceIds...),
		DegradedSince:           status.DegradedSince,
		Sections:                map[string]projectioncontract.WorkspaceViewModelSection{},
	}
	for _, name := range []string{
		"list",
		"activeWorkspace",
		"runState",
		"promptMode",
		"commands",
		"coverage",
		"currentFocus",
		"latestDiagnostic",
	} {
		ret.Sections[name] = projectioncontract.WorkspaceViewModelSection{
			Name:                    name,
			Revision:                revision,
			WorkspaceRevision:       status.WorkspaceRevision,
			SourceEventRevision:     status.SourceEventRevision,
			ProjectedSourceRevision: status.ProjectedSourceRevision,
			State:                   status.State,
		}
	}
	return ret
}

func appWorkspaceViewModelFromStatus(workspace appmodelcore.Workspace, status Status) projectioncontract.WorkspaceViewModel {
	return workspaceViewModelFromStatus(readmodelcore.WorkspaceSnapshotState{Workspace: workspace}, status)
}

func appWorkspaceViewModelFromSnapshot(snapshot readmodelcore.WorkspaceSnapshotState, status Status) projectioncontract.WorkspaceViewModel {
	return workspaceViewModelFromStatus(snapshot, status)
}

func workspacePromptMode(snapshot readmodelcore.WorkspaceSnapshotState) string {
	return readmodelprojection.WorkspacePromptMode(snapshot)
}

func (s *Store) RunHeartbeatMetrics(now time.Time) Metrics {
	if s == nil {
		return Metrics{}
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var ret Metrics
	for _, entry := range s.workspaces {
		if entry == nil || !entry.HasSnapshot {
			continue
		}
		addConvergenceMetrics(&ret, entry.Snapshot.Acceptance)
		heartbeat := entry.Snapshot.RunHeartbeat
		if strings.TrimSpace(heartbeat.RunID) == "" || !activeRunHeartbeatState(heartbeat.State) {
			continue
		}
		ret.ActiveRunHeartbeatTotal++
		if heartbeat.Stalled || strings.TrimSpace(heartbeat.ActivityState) == "stalled" {
			ret.StalledRunTotal++
		}
		if heartbeat.IneffectiveRetry || strings.TrimSpace(heartbeat.ActivityState) == "ineffective_retry" {
			ret.IneffectiveRetryRunTotal++
		}
		if age := runHeartbeatAgeSeconds(heartbeat, now); age > ret.ActiveRunHeartbeatAgeSecondsMax {
			ret.ActiveRunHeartbeatAgeSecondsMax = age
		}
	}
	return ret
}

func addConvergenceMetrics(ret *Metrics, acceptance projectioncontract.AcceptanceProjection) {
	if ret == nil {
		return
	}
	ret.ConvergenceActiveSlices += acceptance.Summary.ActiveSlices
	if acceptance.CurrentFocus != nil && activeCoverageFocusState(acceptance.CurrentFocus.State) {
		ret.ConvergenceOpenFocuses++
	}
	if acceptance.Budget != nil {
		ret.ConvergenceRepairAttemptsTotal += acceptance.Budget.RepairAttempts
		ret.ConvergenceNoDeltaRoundsTotal += acceptance.Budget.NoCoverageDeltaRounds
		if convergenceBudgetEscalated(acceptance.Budget.Decision) {
			ret.ConvergenceBudgetEscalations++
		}
		return
	}
	if acceptance.CurrentFocus != nil {
		ret.ConvergenceRepairAttemptsTotal += acceptance.CurrentFocus.Attempt
	}
}

func activeCoverageFocusState(state string) bool {
	state = strings.TrimSpace(state)
	return state == "" || state == "active" || state == "pending" || state == "blocked"
}

func convergenceBudgetEscalated(decision string) bool {
	switch strings.TrimSpace(decision) {
	case "", "resume":
		return false
	default:
		return true
	}
}

func (s *Store) cloneEntryForReplace(workspaceID string) *workspaceProjectionEntry {
	if s == nil {
		return &workspaceProjectionEntry{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if entry := s.workspaces[workspaceID]; entry != nil {
		return cloneWorkspaceProjectionEntry(entry)
	}
	return &workspaceProjectionEntry{}
}

func (s *Store) entryForUpdateLocked(workspaceID string) *workspaceProjectionEntry {
	entry := s.workspaces[workspaceID]
	if entry == nil {
		entry = &workspaceProjectionEntry{}
		s.workspaces[workspaceID] = entry
	}
	return entry
}

func (s *Store) workspaceTotalLocked() int {
	total := 0
	for _, entry := range s.workspaces {
		if entry != nil && entry.HasWorkspace {
			total++
		}
	}
	return total
}

func (s *Store) currentGenerationFailuresLocked() (failedIDs []string, latestError string, degradedSince time.Time) {
	if s.globalFailure != nil && s.globalFailure.Generation == s.streamGeneration {
		latestError = s.globalFailure.Error
		degradedSince = s.globalFailure.Since
	}
	for workspaceID, failure := range s.workspaceFailures {
		if failure.Generation != s.streamGeneration {
			continue
		}
		failedIDs = append(failedIDs, workspaceID)
		if latestError == "" {
			latestError = failure.Error
		}
		if degradedSince.IsZero() || (!failure.Since.IsZero() && failure.Since.Before(degradedSince)) {
			degradedSince = failure.Since
		}
	}
	// Also surface entry.Error for current entries as failure IDs when not already tracked
	// for the current stream generation.
	for workspaceID, entry := range s.workspaces {
		if entry == nil || strings.TrimSpace(entry.Error) == "" {
			continue
		}
		if failure, ok := s.workspaceFailures[workspaceID]; ok && failure.Generation == s.streamGeneration {
			continue
		}
		failedIDs = append(failedIDs, workspaceID)
		if latestError == "" {
			latestError = entry.Error
		}
	}
	if len(failedIDs) > 0 {
		sort.Strings(failedIDs)
	}
	return failedIDs, latestError, degradedSince
}

func (s *Store) recordWorkspaceFailureLocked(workspaceID string, err error) {
	if s == nil || strings.TrimSpace(workspaceID) == "" || err == nil {
		return
	}
	if s.workspaceFailures == nil {
		s.workspaceFailures = map[string]ProjectionFailure{}
	}
	now := time.Now().UTC()
	if existing, ok := s.workspaceFailures[workspaceID]; ok && existing.Generation == s.streamGeneration && !existing.Since.IsZero() {
		now = existing.Since
	}
	s.workspaceFailures[workspaceID] = ProjectionFailure{
		Generation: s.streamGeneration,
		Error:      err.Error(),
		Since:      now,
	}
}

func (s *Store) clearWorkspaceFailureLocked(workspaceID string) {
	if s == nil || s.workspaceFailures == nil || strings.TrimSpace(workspaceID) == "" {
		return
	}
	delete(s.workspaceFailures, workspaceID)
}

func (s *Store) recordGlobalFailureLocked(err error) {
	if s == nil || err == nil {
		return
	}
	now := time.Now().UTC()
	if s.globalFailure != nil && s.globalFailure.Generation == s.streamGeneration && !s.globalFailure.Since.IsZero() {
		now = s.globalFailure.Since
	}
	s.globalFailure = &ProjectionFailure{
		Generation: s.streamGeneration,
		Error:      err.Error(),
		Since:      now,
	}
}

func (s *Store) clearAllCurrentFailuresLocked() {
	if s == nil {
		return
	}
	s.globalFailure = nil
	s.workspaceFailures = map[string]ProjectionFailure{}
	s.errText = ""
}

func (s *Store) hasWorkspaceErrorLocked() bool {
	for _, entry := range s.workspaces {
		if entry != nil && strings.TrimSpace(entry.Error) != "" {
			return true
		}
	}
	return false
}

func (s *Store) hasStaleWorkspaceLocked() bool {
	for _, entry := range s.workspaces {
		if entry != nil && entry.Stale {
			return true
		}
	}
	return false
}

func (s *Store) refreshProjectedSourceRevisionLocked() {
	var source, projected int64
	for _, entry := range s.workspaces {
		if entry == nil {
			continue
		}
		if entry.SourceRevision > source {
			source = entry.SourceRevision
		}
		if entry.ProjectedSourceRevision > projected {
			projected = entry.ProjectedSourceRevision
		}
	}
	if projected > source {
		source = projected
	}
	s.sourceRevision = source
	s.projectedSourceRevision = projected
}

func formatProjectionTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func activeRunHeartbeatState(state string) bool {
	switch common.RunState(strings.TrimSpace(state)) {
	case common.RunStateCompleted,
		common.RunStateFailed,
		common.RunStateCancelled,
		common.RunStateSuperseded,
		common.RunStateBlockedOnDecision,
		common.RunStateBlockedOnApproval:
		return false
	default:
		return true
	}
}

func runHeartbeatAgeSeconds(heartbeat projectioncontract.RunHeartbeat, now time.Time) int64 {
	for _, candidate := range []string{
		heartbeat.LatestEffectiveProgressAt,
		heartbeat.LatestEventAt,
		heartbeat.LatestLLMUsageAt,
		heartbeat.LatestResultAt,
		heartbeat.LatestArtifactAt,
		heartbeat.LatestPatchAt,
	} {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339Nano, candidate)
		if err != nil {
			continue
		}
		age := now.Sub(parsed.UTC()).Seconds()
		if age < 0 {
			return 0
		}
		return int64(age)
	}
	return 0
}

func stringSliceContains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func cloneWorkspace(item appmodelcore.Workspace) appmodelcore.Workspace {
	if item.DefaultLLMTarget != nil {
		target := item.DefaultLLMTarget.Clone()
		item.DefaultLLMTarget = &target
	}
	item.SoulBindings = cloneStringStringMap(item.SoulBindings)
	item.ContextHints = cloneStringAnyMap(item.ContextHints)
	item.EvidencePolicies = cloneMapSlice(item.EvidencePolicies)
	item.ViewModel.Sections = cloneWorkspaceViewModelSections(item.ViewModel.Sections)
	return item
}

func cloneWorkspaceSnapshot(item readmodelcore.WorkspaceSnapshotState) readmodelcore.WorkspaceSnapshotState {
	item.Workspace = cloneWorkspace(item.Workspace)
	item.ViewModel.Sections = cloneWorkspaceViewModelSections(item.ViewModel.Sections)
	item.Goals = append([]workorchmanagementcontract.Goal(nil), item.Goals...)
	item.DoDs = append([]workorchmanagementcontract.DoD(nil), item.DoDs...)
	item.Runs = append([]appmodelcore.Run(nil), item.Runs...)
	item.Results = append([]projectioncontract.Result(nil), item.Results...)
	item.Artifacts = append([]projectioncontract.Artifact(nil), item.Artifacts...)
	item.ArtifactContinuations = append([]projectioncontract.ArtifactContinuation(nil), item.ArtifactContinuations...)
	item.Patches = append([]projectioncontract.Patch(nil), item.Patches...)
	item.WebSources = append([]projectioncontract.WebSourceTrace(nil), item.WebSources...)
	item.LLMContextBudgets = append([]projectioncontract.LLMContextBudgetTrace(nil), item.LLMContextBudgets...)
	item.LLMUsageTraces = append([]projectioncontract.LLMUsageTrace(nil), item.LLMUsageTraces...)
	item.EventRecords = append([]projectioncontract.EventRecord(nil), item.EventRecords...)
	item.Decisions = append([]projectioncontract.Decision(nil), item.Decisions...)
	item.Approvals = append([]projectioncontract.Approval(nil), item.Approvals...)
	item.TaskDecompositions = append([]projectioncontract.TaskDecomposition(nil), item.TaskDecompositions...)
	item.ExecutionSections = append([]projectioncontract.ExecutionSection(nil), item.ExecutionSections...)
	item.MemoryRecallTraces = append([]projectioncontract.MemoryRecallTrace(nil), item.MemoryRecallTraces...)
	if item.ContextBundle != nil {
		bundle := cloneContextBundle(*item.ContextBundle)
		item.ContextBundle = &bundle
	}
	if item.RunCommand != nil {
		status := *item.RunCommand
		item.RunCommand = &status
	}
	if item.GapBaseline != nil {
		baseline := cloneGapBaseline(*item.GapBaseline)
		item.GapBaseline = &baseline
	}
	item.AuthorityDiagnostics = append([]projectioncontract.GapDiagnostic(nil), item.AuthorityDiagnostics...)
	return item
}

func cloneWorkspaceViewModelSections(input map[string]projectioncontract.WorkspaceViewModelSection) map[string]projectioncontract.WorkspaceViewModelSection {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]projectioncontract.WorkspaceViewModelSection, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func cloneGapBaseline(item projectioncontract.GapBaseline) projectioncontract.GapBaseline {
	item.CurrentFocus = cloneStringAnyMap(item.CurrentFocus)
	item.OpenCoverageWorkItems = cloneMapSlice(item.OpenCoverageWorkItems)
	item.AcceptedEvidenceRefs = append([]string(nil), item.AcceptedEvidenceRefs...)
	item.RejectedEvidenceRefs = append([]string(nil), item.RejectedEvidenceRefs...)
	item.TodoItems = append([]projectioncontract.GapTodoItem(nil), item.TodoItems...)
	item.Diagnostics = append([]projectioncontract.GapDiagnostic(nil), item.Diagnostics...)
	for idx := range item.TodoItems {
		item.TodoItems[idx].CoverageWorkItemIDs = append([]string(nil), item.TodoItems[idx].CoverageWorkItemIDs...)
		item.TodoItems[idx].RequiredEvidenceKinds = append([]string(nil), item.TodoItems[idx].RequiredEvidenceKinds...)
		item.TodoItems[idx].Metadata = cloneStringAnyMap(item.TodoItems[idx].Metadata)
	}
	return item
}

func cloneWorkspaceProjectionEntry(item *workspaceProjectionEntry) *workspaceProjectionEntry {
	if item == nil {
		return &workspaceProjectionEntry{}
	}
	ret := *item
	if item.HasWorkspace {
		ret.Workspace = cloneWorkspace(item.Workspace)
	}
	if item.HasSnapshot {
		ret.Snapshot = cloneWorkspaceSnapshot(item.Snapshot)
	}
	if item.HasActivity {
		ret.Activity = cloneWorkspaceActivityFeed(item.Activity)
	}
	return &ret
}

func cloneWorkspaceActivityFeed(feed projectioncontract.WorkspaceActivityFeed) projectioncontract.WorkspaceActivityFeed {
	ret := feed
	if feed.Items != nil {
		ret.Items = append([]projectioncontract.WorkspaceActivityItem(nil), feed.Items...)
	}
	return ret
}

func cloneContextBundle(item projectioncontract.ContextBundle) projectioncontract.ContextBundle {
	item.Sections = append([]projectioncontract.ContextSection(nil), item.Sections...)
	for idx := range item.Sections {
		item.Sections[idx].EvidenceRefs = append([]string(nil), item.Sections[idx].EvidenceRefs...)
		item.Sections[idx].Data = cloneStringAnyMap(item.Sections[idx].Data)
	}
	item.RecentEvidence = append([]projectioncontract.ContextEvidence(nil), item.RecentEvidence...)
	for idx := range item.RecentEvidence {
		item.RecentEvidence[idx].EvidenceRefs = append([]string(nil), item.RecentEvidence[idx].EvidenceRefs...)
		item.RecentEvidence[idx].Data = cloneStringAnyMap(item.RecentEvidence[idx].Data)
	}
	item.CompressionCandidates = append([]projectioncontract.ContextCompressionCandidate(nil), item.CompressionCandidates...)
	for idx := range item.CompressionCandidates {
		item.CompressionCandidates[idx].SourceRefs = append([]string(nil), item.CompressionCandidates[idx].SourceRefs...)
	}
	item.CompressedContext.Milestones = append([]projectioncontract.ProgressMilestone(nil), item.CompressedContext.Milestones...)
	item.CompressedContext.Blockers = append([]projectioncontract.ProgressBlocker(nil), item.CompressedContext.Blockers...)
	item.CompressedContext.RecentEvidence = append([]projectioncontract.ContextEvidence(nil), item.CompressedContext.RecentEvidence...)
	for idx := range item.CompressedContext.RecentEvidence {
		item.CompressedContext.RecentEvidence[idx].EvidenceRefs = append([]string(nil), item.CompressedContext.RecentEvidence[idx].EvidenceRefs...)
		item.CompressedContext.RecentEvidence[idx].Data = cloneStringAnyMap(item.CompressedContext.RecentEvidence[idx].Data)
	}
	item.CompressedContext.ResultRefs = append([]string(nil), item.CompressedContext.ResultRefs...)
	item.CompressedContext.SourceSectionRefs = append([]string(nil), item.CompressedContext.SourceSectionRefs...)
	item.CompressedContext.SourceEvidenceRefs = append([]string(nil), item.CompressedContext.SourceEvidenceRefs...)
	return item
}

func cloneStringStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	ret := make(map[string]string, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func cloneStringAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func cloneMapSlice(input []map[string]any) []map[string]any {
	if input == nil {
		return nil
	}
	ret := make([]map[string]any, 0, len(input))
	for _, item := range input {
		ret = append(ret, cloneStringAnyMap(item))
	}
	return ret
}
