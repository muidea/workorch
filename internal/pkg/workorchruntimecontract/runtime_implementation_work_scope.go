package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

var ErrRuntimeImplementationWorkScopeStale = errors.New("implementation work scope owner revision changed since request")

// RuntimeImplementationWorkScopeRequest is a read-only observation request,
// not an EventHub command, working binding, or permission to execute. Its exact
// CWI and sealed policy are independent of the terminal proof's target selector.
type RuntimeImplementationWorkScopeRequest struct {
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	AuthoritySnapshotID string
	DefinitionSeedHash  string
	CoverageWorkItemID  common.CoverageWorkItemID
	Policy              RuntimeImplementationWorkPolicy
	ManifestBinding     RuntimeWorkspaceManifestBinding
	RequestID           string
}

// RuntimeImplementationWorkScopeSources must come from the owner's current
// sealed definition and authority, never from the request being checked. It is
// compiler input, not another persisted authority or an execution permission.
type RuntimeImplementationWorkScopeSources struct {
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	AuthoritySnapshotID string
	Seed                RuntimeDefinitionSeed
	Focuses             []RuntimeDefinitionFocus
	Profiles            []RuntimeProfileContract
	CoverageWorkItemID  common.CoverageWorkItemID
	ManifestBinding     RuntimeWorkspaceManifestBinding
}

func (s RuntimeImplementationWorkScopeSources) Compile() (RuntimeImplementationWorkScopeRequest, error) {
	return CompileRuntimeImplementationWorkScopeRequest(s.WorkspaceID, s.RunID, s.AuthoritySnapshotID,
		s.Seed, s.Focuses, s.Profiles, s.CoverageWorkItemID, s.ManifestBinding)
}

// ValidateForSources independently recompiles the expected request. A valid
// self-hash alone cannot authorize a changed policy, CWI, or authority.
func (r RuntimeImplementationWorkScopeRequest) ValidateForSources(s RuntimeImplementationWorkScopeSources) error {
	if err := r.Validate(); err != nil {
		return err
	}
	expected, err := s.Compile()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(r, expected) {
		return fmt.Errorf("implementation work scope request does not match current sealed sources")
	}
	return nil
}

func (o RuntimeImplementationWorkScopeObservation) ValidateForSources(s RuntimeImplementationWorkScopeSources) error {
	expected, err := s.Compile()
	if err != nil {
		return err
	}
	return o.ValidateForRequest(expected)
}

func CompileRuntimeImplementationWorkScopeRequest(workspaceID common.WorkspaceID, runID common.RunID, authoritySnapshotID string,
	seed RuntimeDefinitionSeed, focuses []RuntimeDefinitionFocus, profiles []RuntimeProfileContract, coverageID common.CoverageWorkItemID, manifest RuntimeWorkspaceManifestBinding,
) (RuntimeImplementationWorkScopeRequest, error) {
	return compileRuntimeWorkScopeRequestInSession(workspaceID, runID, authoritySnapshotID, seed, focuses, profiles, coverageID, manifest, nil)
}

func compileRuntimeWorkScopeRequestInSession(workspaceID common.WorkspaceID, runID common.RunID, authoritySnapshotID string,
	seed RuntimeDefinitionSeed, focuses []RuntimeDefinitionFocus, profiles []RuntimeProfileContract, coverageID common.CoverageWorkItemID, manifest RuntimeWorkspaceManifestBinding,
	session *runtimeSemanticValidationSession,
) (RuntimeImplementationWorkScopeRequest, error) {
	if err := manifest.Validate(); err != nil {
		return RuntimeImplementationWorkScopeRequest{}, err
	}
	if err := validateRuntimeWorkDefinitionSourcesInSession(seed, focuses, profiles, session); err != nil {
		return RuntimeImplementationWorkScopeRequest{}, err
	}
	var selected *RuntimeImplementationWorkPolicy
	for _, focus := range focuses {
		if len(focus.CoverageWorkItemIDs) != 1 || focus.CoverageWorkItemIDs[0] != coverageID {
			continue
		}
		if selected != nil {
			return RuntimeImplementationWorkScopeRequest{}, fmt.Errorf("implementation work scope requires one exact CWI declaration")
		}
		selected = focus.ImplementationWorkPolicy
	}
	if selected == nil {
		return RuntimeImplementationWorkScopeRequest{}, fmt.Errorf("implementation work scope has no policy for CWI %q", coverageID)
	}
	request := RuntimeImplementationWorkScopeRequest{
		WorkspaceID: workspaceID, RunID: runID, AuthoritySnapshotID: authoritySnapshotID,
		DefinitionSeedHash: seed.Hash, CoverageWorkItemID: coverageID, Policy: selected.Normalized(), ManifestBinding: manifest.Clone(),
	}
	// Observation time is audit metadata, not a new scope/revision identity.
	request.ManifestBinding.Observation.ObservedAt = ""
	request.RequestID = implementationWorkScopeRequestID(request)
	if err := request.Validate(); err != nil {
		return RuntimeImplementationWorkScopeRequest{}, err
	}
	return request, nil
}

func (r RuntimeImplementationWorkScopeRequest) Clone() RuntimeImplementationWorkScopeRequest {
	r.Policy = r.Policy.Clone()
	r.ManifestBinding.Manifest.TargetRefs = slices.Clone(r.ManifestBinding.Manifest.TargetRefs)
	r.ManifestBinding.Observation.Dependencies = slices.Clone(r.ManifestBinding.Observation.Dependencies)
	return r
}

func (r RuntimeImplementationWorkScopeRequest) Validate() error {
	for _, ref := range []string{string(r.WorkspaceID), string(r.RunID), r.AuthoritySnapshotID, r.DefinitionSeedHash, string(r.CoverageWorkItemID)} {
		if !validRuntimeImplementationWorkRef(ref) || strings.TrimSpace(ref) != ref {
			return fmt.Errorf("implementation work scope requires exact bounded request identity")
		}
	}
	if err := r.Policy.Validate(); err != nil {
		return err
	}
	if err := r.ManifestBinding.Validate(); err != nil {
		return err
	}
	if r.ManifestBinding.Observation.OwnerModuleID != r.Policy.OwnerModuleID {
		return fmt.Errorf("implementation work scope requires its resource owner's manifest")
	}
	if r.RequestID != implementationWorkScopeRequestID(r) {
		return fmt.Errorf("implementation work scope request identity mismatch")
	}
	return nil
}

func implementationWorkScopeRequestID(r RuntimeImplementationWorkScopeRequest) string {
	r.RequestID = ""
	r.Policy = r.Policy.Normalized()
	data, _ := json.Marshal(r)
	hash := sha256.Sum256(data)
	return "implementation-work-scope:" + hex.EncodeToString(hash[:])
}

// RuntimeImplementationWorkScopeObservation retains complete owner facts.
// In particular a selected target may still be a protected validation input:
// this record never replaces ordinary mutation exclusions or owner feasibility.
// It must not be committed into Resources.TargetSelection (terminal proof).
type RuntimeImplementationWorkScopeObservation struct {
	Request     RuntimeImplementationWorkScopeRequest
	Selection   RuntimeTargetSelection
	Observation RuntimePrerequisiteObservation
}

func (o RuntimeImplementationWorkScopeObservation) Clone() RuntimeImplementationWorkScopeObservation {
	o.Request = o.Request.Clone()
	o.Selection.SelectorCandidateRefs = slices.Clone(o.Selection.SelectorCandidateRefs)
	o.Selection.ProspectiveTargetAllOfConstraintRefs = slices.Clone(o.Selection.ProspectiveTargetAllOfConstraintRefs)
	o.Selection.TargetRefs = slices.Clone(o.Selection.TargetRefs)
	o.Selection.CandidateTargetRefs = slices.Clone(o.Selection.CandidateTargetRefs)
	o.Observation.Dependencies = slices.Clone(o.Observation.Dependencies)
	return o
}

func (o RuntimeImplementationWorkScopeObservation) ValidateForRequest(request RuntimeImplementationWorkScopeRequest) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if err := o.Request.Validate(); err != nil {
		return err
	}
	if !reflect.DeepEqual(o.Request, request) {
		return fmt.Errorf("implementation work scope response differs from exact request")
	}
	if err := o.Selection.Validate(); err != nil {
		return err
	}
	s := o.Selection
	p := request.Policy
	if len(s.TargetRefs) > RuntimeOwnerInventoryItemsMaxCount || len(s.CandidateTargetRefs) > RuntimeOwnerInventoryItemsMaxCount {
		return fmt.Errorf("implementation work scope observation exceeds owner inventory limit")
	}
	if s.PrerequisiteID != request.RequestID || s.OwnerModuleID != p.OwnerModuleID || s.TargetRole != "implementation_work" ||
		s.SelectorRef != p.TargetSelectorRef || s.SelectorOwnerRef != p.TargetSelectorOwnerRef || s.SelectorSubjectRef != p.TargetSelectorSubjectRef ||
		!SameRuntimeTargetSelectorCandidateRefs(s.SelectorCandidateRefs, p.TargetSelectorCandidateRefs) ||
		!SameRuntimeProspectiveTargetAllOfConstraintRefs(s.ProspectiveTargetAllOfConstraintRefs, p.ProspectiveTargetAllOfConstraintRefs) || s.ProspectiveTargetRef != "" {
		return fmt.Errorf("implementation work scope observation differs from sealed selector")
	}
	if err := o.Observation.Validate(); err != nil {
		return err
	}
	fact := o.Observation
	if fact.PrerequisiteID != request.RequestID || fact.OwnerModuleID != p.OwnerModuleID || fact.Kind != PrerequisiteKindTargetSelection ||
		fact.ContentHash != s.SnapshotID ||
		!runtimeObservationDependsOnWorkspaceTreeRevision(fact, p.OwnerModuleID, s.ObservedRevision) {
		return fmt.Errorf("implementation work scope observation has no matching owner revision preimage")
	}
	// Unsupported is a negative selector fact, not a workspace observation or
	// a usable scope. Preserve it without pretending its revision is the tree.
	if s.Disposition != RuntimeTargetSelectionUnsupported && s.ObservedRevision != request.ManifestBinding.Manifest.Revision {
		return ErrRuntimeImplementationWorkScopeStale
	}
	for _, target := range append(append([]string(nil), s.TargetRefs...), s.CandidateTargetRefs...) {
		if !slices.Contains(request.ManifestBinding.Manifest.TargetRefs, target) {
			return fmt.Errorf("implementation work scope target is absent from exact owner manifest")
		}
	}
	return nil
}
