package workorchruntimecontract

import "fmt"

// RuntimeCandidateWorkContent is the bounded provider view of the exact frozen
// scope. Unselected bodies stay out of the request; the full source is checked
// against the accepted intent before this view is derived.
type RuntimeCandidateWorkContent struct {
	ScopeRequestID   string           `json:"scopeRequestId"`
	ContentHash      string           `json:"contentHash"`
	ManifestRevision string           `json:"manifestRevision"`
	Snapshots        []TargetSnapshot `json:"snapshots"`
	AbsentTargets    []string         `json:"absentTargets,omitempty"`
}

func ProjectRuntimeCandidateWorkContent(c RuntimeCandidateContext, m RuntimeCandidateContractManifest) (*RuntimeCandidateWorkContent, error) {
	b := m.ImplementationWorkingBinding
	if b == nil {
		if c.ImplementationWorkContent != nil {
			return nil, fmt.Errorf("unexpected implementation work source")
		}
		return nil, nil
	}
	source := c.ImplementationWorkContent
	if source == nil || b.IntentBinding == nil || b.IntentBinding.Intent == nil || source.Hash() != b.IntentBinding.Intent.ContentHash {
		return nil, fmt.Errorf("implementation work source differs from accepted intent")
	}
	refs := make([]string, 0, len(source.Snapshots))
	seen := map[string]bool{}
	for _, s := range source.Snapshots {
		if seen[s.TargetRef] {
			return nil, fmt.Errorf("duplicate implementation source target")
		}
		seen[s.TargetRef] = true
		refs = append(refs, s.TargetRef)
	}
	if err := source.Validate(RuntimeImplementationWorkScopeObservation{Selection: RuntimeTargetSelection{ObservedRevision: source.ManifestRevision, TargetRefs: refs}}); err != nil {
		return nil, err
	}
	out := &RuntimeCandidateWorkContent{ScopeRequestID: b.IntentBinding.Source.ScopeRequestID, ContentHash: source.Hash(), ManifestRevision: source.ManifestRevision}
	selected := map[string]bool{}
	for _, required := range m.RequiredOperationSequence {
		if required.TargetRef == "" || selected[required.TargetRef] {
			return nil, fmt.Errorf("implementation operation lacks unique exact target")
		}
		selected[required.TargetRef] = true
		found := false
		for _, s := range source.Snapshots {
			if s.TargetRef == required.TargetRef {
				out.Snapshots = append(out.Snapshots, s)
				found = true
			}
		}
		if !found {
			prospective := false
			for _, p := range b.Proposals {
				for _, target := range p.Target.ProspectiveTargetRefs {
					if target == required.TargetRef {
						prospective = true
					}
				}
			}
			if !prospective {
				return nil, fmt.Errorf("implementation operation missing frozen existing target %q", required.TargetRef)
			}
			out.AbsentTargets = append(out.AbsentTargets, required.TargetRef)
		}
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("implementation work has no required targets")
	}
	return out, nil
}
