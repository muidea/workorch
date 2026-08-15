package workspaceviewcore

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

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

func (s *Store) Status() projectioncontract.ProjectionStatus {
	if s == nil {
		return projectioncontract.ProjectionStatus{State: StateFailed, Error: "workspace projection store is not configured"}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.statusLocked()
}

func (s *Store) StatusForWorkspace(workspaceID string) projectioncontract.ProjectionStatus {
	if s == nil {
		return projectioncontract.ProjectionStatus{State: StateFailed, Error: "workspace projection store is not configured"}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.statusForWorkspaceLocked(workspaceID)
}

func (s *Store) statusLocked() projectioncontract.ProjectionStatus {
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
	return projectioncontract.ProjectionStatus{
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

func (s *Store) statusForWorkspaceLocked(workspaceID string) projectioncontract.ProjectionStatus {
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
	return projectioncontract.ProjectionStatus{
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

func formatProjectionTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
