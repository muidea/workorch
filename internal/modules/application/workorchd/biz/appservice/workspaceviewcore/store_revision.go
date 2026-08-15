package workspaceviewcore

import (
	"strings"
	"time"
)

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
