package workspace

import (
	"reflect"
	"slices"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

const PreparationAssessmentVersion = "workorch-preparation-all-of-assessment/v1"

// The aggregate of the original checklist, not a resource freshness lease,
// creation completion or permission to activate. Even checks_passed must still
// pass pure bootstrap compilation and the creation-completion transaction.
type PreparationAssessment struct {
	Version             string
	ManifestHash        string
	Outcome             string
	RequiredCount       int
	PassedRequiredCount int
	Outstanding         []PreparationOutstandingCheck
}

type PreparationOutstandingCheck struct {
	ID, Owner, State, Reason string
}

type PreparationCheckRequirement struct {
	ID, Owner string
	Required  bool
}

// Shared checklist identity contract. Both manifest compilation and persisted
// aggregate readback use the original candidate, not a filtered result slice.
func PreparationCheckRequirements(candidate DefinitionAdmissionCandidate) ([]PreparationCheckRequirement, error) {
	if candidate.Specification == nil {
		return nil, common.ErrConflict
	}
	r := []PreparationCheckRequirement{
		{"exact_seal", "workspace", true}, {"directory_boundary", "file", true},
		{"managed_configuration", "file", true}, {"soul_selection", "soul", true},
		{"skill_selection", "skill", true}, {"mcp_selection", "mcpregistry", true},
		{"memory_selection", "memory", true}, {"provider_cache", "llm", true},
		{"persistence_integrity", "workspace", true}, {"first_activation_no_run", "run", true},
	}
	for _, resource := range candidate.Specification.Resources {
		if !boundedPreparationIdentity(resource.ID) {
			return nil, common.ErrConflict
		}
		r = append(r, PreparationCheckRequirement{"resource/" + resource.ID, "file", resource.Required})
	}
	slices.SortFunc(r, func(a, b PreparationCheckRequirement) int { return strings.Compare(a.ID, b.ID) })
	if len(r) > 128 {
		return nil, common.ErrConflict
	}
	for i, c := range r {
		if !boundedPreparationIdentity(c.ID) || i > 0 && c.ID == r[i-1].ID {
			return nil, common.ErrConflict
		}
	}
	return r, nil
}

func (s PreparationAssessment) Clone() PreparationAssessment {
	s.Outstanding = slices.Clone(s.Outstanding)
	return s
}

// AssessPreparationChecks is pure aggregation, not an evidence consumer. The
// private settlement must independently validate all owner material before
// saving this result. Unknown/missing/duplicate/substituted checks are errors;
// optional failures stay visible in Checks but cannot block required all_of.
func AssessPreparationChecks(manifestHash string, requirements []PreparationCheckRequirement, checks []WorkspacePreparationCheckResult) (PreparationAssessment, error) {
	if !boundedPreparationIdentity(manifestHash) || len(requirements) == 0 || len(requirements) > 128 || len(checks) != len(requirements) {
		return PreparationAssessment{}, common.ErrConflict
	}
	s := PreparationAssessment{Version: PreparationAssessmentVersion, ManifestHash: manifestHash, Outcome: "checks_passed", Outstanding: []PreparationOutstandingCheck{}}
	last := ""
	for i, requirement := range requirements {
		c := checks[i]
		if requirement.ID <= last || !boundedPreparationIdentity(requirement.ID) || !boundedPreparationIdentity(requirement.Owner) || c.ID != requirement.ID || c.Owner != requirement.Owner || c.Required != requirement.Required || !boundedPreparationIdentity(c.Reason) {
			return PreparationAssessment{}, common.ErrConflict
		}
		last = requirement.ID
		if c.State != "passed" && c.State != "blocked" && c.State != "failed" && c.State != "pending" {
			return PreparationAssessment{}, common.ErrConflict
		}
		if !c.Required {
			continue
		}
		s.RequiredCount++
		if c.State == "passed" {
			s.PassedRequiredCount++
		} else {
			s.Outcome = "blocked"
			s.Outstanding = append(s.Outstanding, PreparationOutstandingCheck{c.ID, c.Owner, string(c.State), c.Reason})
		}
	}
	if s.RequiredCount == 0 {
		return PreparationAssessment{}, common.ErrConflict
	}
	return s, nil
}

func (s PreparationAssessment) ValidateFor(manifestHash string, checks []WorkspacePreparationCheckResult) error {
	requirements := make([]PreparationCheckRequirement, len(checks))
	for i, c := range checks {
		requirements[i] = PreparationCheckRequirement{c.ID, c.Owner, c.Required}
	}
	expected, err := AssessPreparationChecks(manifestHash, requirements, checks)
	if err != nil || !reflect.DeepEqual(s, expected) {
		return common.ErrConflict
	}
	return nil
}

// Repository backstop: reproduce membership from the immutable original
// candidate. A self-consistent summary over a truncated result is not enough.
func validatePreparationAssessmentPersistence(w *Workspace) error {
	if w == nil {
		return common.ErrConflict
	}
	if w.StartupPreparation == nil {
		return nil
	}
	for _, a := range w.StartupPreparation.Attempts {
		if a.Assessment == nil {
			continue // Older history is never backfilled on GET/recovery.
		}
		expected, err := CompilePreparationAssessment(w, a)
		if err != nil || !reflect.DeepEqual(expected, *a.Assessment) {
			return common.ErrConflict
		}
	}
	return nil
}

// Compile only after the original owner settlement has evaluated its complete
// archived material. This function cannot observe resources or grant M3.
func CompilePreparationAssessment(w *Workspace, a WorkspacePreparationAttempt) (PreparationAssessment, error) {
	if w == nil || a.State != WorkspacePreparationBlocked || a.EvaluationVersion != PreparationCheckEvaluationVersion {
		return PreparationAssessment{}, common.ErrConflict
	}
	var candidate *DefinitionAdmissionCandidate
	for i := range w.DefinitionAdmissionCandidates {
		c := &w.DefinitionAdmissionCandidates[i]
		if c.CandidateID == a.Binding.CandidateID && c.CandidateRevision == a.Binding.CandidateRevision && c.CandidateHash == a.Binding.CandidateHash {
			if candidate != nil || c.AuthoringRef == nil || *c.AuthoringRef != a.Binding.AuthoringRef {
				return PreparationAssessment{}, common.ErrConflict
			}
			candidate = c
		}
	}
	if candidate == nil {
		return PreparationAssessment{}, common.ErrConflict
	}
	requirements, err := PreparationCheckRequirements(*candidate)
	if err != nil {
		return PreparationAssessment{}, err
	}
	return AssessPreparationChecks(a.Binding.ManifestHash, requirements, a.Checks)
}
