package runtime

import (
	"fmt"
	"reflect"

	"github.com/muidea/workorch/internal/pkg/common"
)

// Committed SM-2 results are immutable history. New owner observations can
// change current applicability and credit, but cannot rewrite an earlier
// assessment to make it agree with the new state. This compares the actual
// CAS preimage; it never re-runs a historical classifier.
func validateCommittedAssessmentResultTransition(previous, next RuntimeAggregate) error {
	count := len(previous.Facts.Assessments)
	if len(next.Facts.Assessments) < count || count > 0 && !reflect.DeepEqual(previous.Facts.Assessments, next.Facts.Assessments[:count]) {
		return fmt.Errorf("%w: committed runtime assessment facts are immutable", common.ErrConflict)
	}
	for i, original := range previous.CompletedDispatches {
		if original.Candidate.Assessment == nil {
			continue
		}
		if i >= len(next.CompletedDispatches) || !reflect.DeepEqual(original, next.CompletedDispatches[i]) {
			return fmt.Errorf("%w: committed runtime assessment dispatch %q is immutable", common.ErrConflict, original.ID)
		}
	}
	if len(next.Facts.Assessments) == count {
		return nil
	}
	// An appended result must settle the one already-sealed assessment. A
	// consumer cannot manufacture a different historical result or append one
	// while processing an unrelated operation.
	if len(next.Facts.Assessments) != count+1 || previous.ActiveDispatch == nil ||
		previous.ActiveDispatch.Candidate.Assessment == nil ||
		next.Facts.Assessments[count].ID != previous.ActiveDispatch.ID {
		return fmt.Errorf("%w: new runtime assessment result has no original active transaction", common.ErrConflict)
	}
	if next.ActiveDispatch != nil || len(next.CompletedDispatches) != len(previous.CompletedDispatches)+1 ||
		len(previous.CompletedDispatches) > 0 && !reflect.DeepEqual(previous.CompletedDispatches, next.CompletedDispatches[:len(previous.CompletedDispatches)]) {
		return fmt.Errorf("%w: new runtime assessment result must close its original dispatch", common.ErrConflict)
	}
	committed := cloneSealedDispatchRaw(next.CompletedDispatches[len(previous.CompletedDispatches)])
	if committed.State != DispatchStateCommitted || committed.AssessmentAdjudication == nil {
		return fmt.Errorf("%w: new runtime assessment result requires a committed adjudication", common.ErrConflict)
	}
	// SM-2 may add its result and close SM-3, but cannot alter the original
	// sealed input while doing so. Full classification belongs to the private
	// AssessmentObserved producer, never to this persistence consumer.
	committed.State = previous.ActiveDispatch.State
	committed.AssessmentAdjudication = nil
	committed.AssessmentAdjudicationHash = ""
	if !reflect.DeepEqual(committed, *previous.ActiveDispatch) {
		return fmt.Errorf("%w: new runtime assessment result changed its sealed input", common.ErrConflict)
	}
	return nil
}
