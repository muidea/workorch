package workorchruntimecontract

import (
	"fmt"
	"iter"

	"github.com/muidea/workorch/internal/pkg/common"
)

// Segments are complete immutable generation bodies in the SAME opaque Run
// record as the active ledger. Rollover and receipt settlement use one CAS;
// there are no independently committed files or hash-only acceptance proofs.
const RuntimeSemanticHistorySegments = "runtime-semantic-history-segments"

type RuntimeSemanticHistoryArchive struct {
	ProtocolRef     string
	GenerationCount int
	Segments        [][]RuntimeSemanticPlanningGeneration
}

// AllHistory visits original append order, including archived generations.
// Values are borrowed read-only. Consumers must clone before changing them.
func (l RuntimeSemanticPlanningLedger) AllHistory() iter.Seq2[int, RuntimeSemanticPlanningGeneration] {
	return func(yield func(int, RuntimeSemanticPlanningGeneration) bool) {
		index := 0
		if l.HistoryArchive != nil {
			for _, segment := range l.HistoryArchive.Segments {
				for _, generation := range segment {
					if !yield(index, generation) {
						return
					}
					index++
				}
			}
		}
		for _, generation := range l.History {
			if !yield(index, generation) {
				return
			}
			index++
		}
	}
}

func (l RuntimeSemanticPlanningLedger) HistoryCount() int {
	n := len(l.History)
	if l.HistoryArchive != nil {
		for _, segment := range l.HistoryArchive.Segments {
			n += len(segment)
		}
	}
	return n
}

// WithoutHistory returns a shallow current-generation view, suitable for
// cloning into a retirement snapshot without recursively duplicating history.
func (l RuntimeSemanticPlanningLedger) WithoutHistory() RuntimeSemanticPlanningLedger {
	l.History, l.HistoryArchive = nil, nil
	return l
}

// ValidateHistoryStorage checks packing, not semantic authority. Full ledger
// validation still independently validates every body and cross-generation
// lineage, including scope, definition, order and duplicates. There is no
// generation quota: execution remains bounded by the original sealed budgets.
func (l RuntimeSemanticPlanningLedger) ValidateHistoryStorage() error {
	if len(l.History) > RuntimeSemanticPlanningHistoryLimit {
		return fmt.Errorf("runtime semantic planning history tail exceeds segment size %d", RuntimeSemanticPlanningHistoryLimit)
	}
	if l.HistoryArchive == nil {
		return nil
	}

	if l.HistoryArchive.ProtocolRef != RuntimeSemanticHistorySegments || len(l.HistoryArchive.Segments) == 0 || len(l.History) == 0 {
		return fmt.Errorf("runtime semantic history archive requires current protocol, full segments and nonempty tail")
	}
	for i, segment := range l.HistoryArchive.Segments {
		if len(segment) != RuntimeSemanticPlanningHistoryLimit {
			return fmt.Errorf("runtime semantic history segment %d requires %d complete generations", i, RuntimeSemanticPlanningHistoryLimit)
		}
	}
	if l.HistoryArchive.GenerationCount != l.HistoryCount()-len(l.History) {
		return fmt.Errorf("runtime semantic history archive generation count mismatch")
	}
	return nil
}

// Opaque Run-state validation and Orchestrator admission both retain the
// original owner scope gate for archived bodies, independently of hashes.
func ValidateRuntimeSemanticHistoryScope(l RuntimeSemanticPlanningLedger, workspace common.WorkspaceID, run common.RunID) error {

	for i, generation := range l.AllHistory() {
		manifest := generation.Snapshot.Manifest
		if manifest == nil || manifest.WorkspaceID != workspace || manifest.RunID != run {
			return fmt.Errorf("runtime semantic history %d scope mismatch", i)
		}
	}
	return nil
}

// AppendHistory is copy-on-write for the storage containers and new body.
// Existing immutable bodies are retained verbatim; Clone/CloneRaw detach them
// at repository boundaries. No aggregate transition or I/O occurs here.
func (l RuntimeSemanticPlanningLedger) AppendHistory(g RuntimeSemanticPlanningGeneration) (RuntimeSemanticPlanningLedger, error) {
	if err := l.ValidateHistoryStorage(); err != nil {
		return RuntimeSemanticPlanningLedger{}, err
	}
	if err := g.Validate(); err != nil {
		return RuntimeSemanticPlanningLedger{}, err
	}
	if len(l.History) == RuntimeSemanticPlanningHistoryLimit {
		archive := RuntimeSemanticHistoryArchive{ProtocolRef: RuntimeSemanticHistorySegments}
		if l.HistoryArchive != nil {
			archive.Segments = append(archive.Segments, l.HistoryArchive.Segments...)
		}
		archive.Segments = append(archive.Segments, append([]RuntimeSemanticPlanningGeneration(nil), l.History...))
		archive.GenerationCount = len(archive.Segments) * RuntimeSemanticPlanningHistoryLimit
		l.HistoryArchive, l.History = &archive, nil
	}
	l.History = append(append([]RuntimeSemanticPlanningGeneration(nil), l.History...), g.Clone())
	return l, nil
}
