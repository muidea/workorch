package run

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

// RuntimeActivationDisposition is an administrative SM-1 outcome owned by
// blocks/run. It deliberately does not mirror Orchestrator convergence,
// dispatch, recovery, or delivery state.
type RuntimeActivationDisposition string

const (
	RuntimeActivationDispositionFreshActivationRequired RuntimeActivationDisposition = "fresh_activation_required"
)

// RuntimeActivationStatus records why this Run cannot be resumed by the
// current binary. RuntimeState remains opaque to Run: the status stores only
// its persisted identity and the producer's bounded diagnostic.
type RuntimeActivationStatus struct {
	Disposition            RuntimeActivationDisposition
	RuntimeStateRef        string
	ActualSchemaVersion    uint32
	SupportedSchemaVersion uint32
	Feedback               *runtimecontract.RuntimeFeedback
}

// Clone returns an independent exact copy without normalizing the original
// diagnostic carried by Feedback.
func (s *RuntimeActivationStatus) Clone() *RuntimeActivationStatus {
	if s == nil {
		return nil
	}
	clone := *s
	if s.Feedback != nil {
		feedback := s.Feedback.CloneOriginal()
		clone.Feedback = &feedback
	}
	return &clone
}
