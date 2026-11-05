package workorchruntimecontract

import "github.com/muidea/workorch/internal/pkg/common"

// These complete pure consumer results live only in the lexical owner-state
// transaction. Each consumer keeps one detached raw preimage, not a hash or a
// normalized projection. Current aggregate joins, budgets and authority gates
// still run. The historical compiler's retention allowlist excludes this field.
type runtimeCandidateSourceCompiler struct {
	attempts *runtimeCandidateAttemptSources
	requests *runtimeCandidateRequestSources
	lineage  *runtimeCandidateManifestSources
}

type runtimeCandidateAttemptSources struct {
	values []RuntimeCandidateAttemptRecord
}

type runtimeCandidateRequestSources struct {
	values []RuntimeCandidateRequestSession
}

type runtimeCandidateManifestSources struct {
	active    *RuntimeDispatchRecord
	completed []RuntimeDispatchRecord
	attempts  []RuntimeCandidateAttemptRecord
	sessions  []RuntimeCandidateRequestSession
}

func validateRuntimeCandidateAttemptSourcesInSession(workspace common.WorkspaceID, run common.RunID, values []RuntimeCandidateAttemptRecord, s *runtimeSemanticValidationSession) error {
	if err := validateRuntimeCandidateAttemptRecordsInSession(values, s); err != nil {
		return err
	}
	// Scope is a current join, not part of the reusable intrinsic computation.
	// Even an exact raw hit must check the caller's actual workspace/run.
	return validateRuntimeCandidateAttemptRecordScope(workspace, run, values)
}

func validateRuntimeCandidateAttemptRecordsInSession(values []RuntimeCandidateAttemptRecord, s *runtimeSemanticValidationSession) error {
	if s != nil && s.committedPreimage != nil {
		return validateRuntimeCandidateAttemptRecordsTransition(values, s.committedPreimage)
	}
	if s == nil || s.candidateSources == nil {
		return ValidateRuntimeCandidateAttemptRecords(values)
	}
	c := s.candidateSources
	if old := c.attempts; old != nil && runtimeRawEqual(old.values, values) {
		return nil
	}
	if err := ValidateRuntimeCandidateAttemptRecords(values); err != nil {
		return err
	}
	c.attempts = &runtimeCandidateAttemptSources{values: cloneRuntimeStateRawSlice(values, cloneRuntimeCandidateAttemptRecordRaw)}
	return nil
}

func validateRuntimeCandidateRequestSourcesInSession(values []RuntimeCandidateRequestSession, s *runtimeSemanticValidationSession) error {
	if s != nil && s.committedPreimage != nil {
		return validateRuntimeCandidateRequestSessionsTransition(values, s.committedPreimage)
	}
	if s == nil || s.candidateSources == nil {
		return ValidateRuntimeCandidateRequestSessions(values)
	}
	c := s.candidateSources
	if old := c.requests; old != nil && runtimeRawEqual(old.values, values) {
		return nil
	}
	if err := ValidateRuntimeCandidateRequestSessions(values); err != nil {
		return err
	}
	c.requests = &runtimeCandidateRequestSources{values: cloneRuntimeStateRawSlice(values, cloneRuntimeCandidateRequestSessionRaw)}
	return nil
}

func validateRuntimeCandidateManifestSourcesInSession(active *RuntimeDispatchRecord, completed []RuntimeDispatchRecord, attempts []RuntimeCandidateAttemptRecord, sessions []RuntimeCandidateRequestSession, s *runtimeSemanticValidationSession) error {
	if s == nil || s.candidateSources == nil {
		return validateRuntimeAcceptedCandidateManifestLineageInSession(active, completed, attempts, sessions, s)
	}
	c := s.candidateSources
	if old := c.lineage; old != nil && runtimeRawEqual(old.active, active) && runtimeRawEqual(old.completed, completed) && runtimeRawEqual(old.attempts, attempts) && runtimeRawEqual(old.sessions, sessions) {
		return nil
	}
	if err := validateRuntimeAcceptedCandidateManifestLineageInSession(active, completed, attempts, sessions, s); err != nil {
		return err
	}
	c.lineage = &runtimeCandidateManifestSources{
		active:    cloneRuntimeStateRawPointer(active, cloneRuntimeDispatchRecordRaw),
		completed: cloneRuntimeStateRawSlice(completed, cloneRuntimeDispatchRecordRaw),
		attempts:  cloneRuntimeStateRawSlice(attempts, cloneRuntimeCandidateAttemptRecordRaw),
		sessions:  cloneRuntimeStateRawSlice(sessions, cloneRuntimeCandidateRequestSessionRaw),
	}
	return nil
}
