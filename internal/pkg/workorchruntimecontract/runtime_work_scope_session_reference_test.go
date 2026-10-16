package workorchruntimecontract

import (
	"fmt"
	"github.com/muidea/workorch/internal/pkg/common"
)

// Original complete ledger path before scope compiler session reuse.
func referenceWorkLedgerBeforeScopeSession(l RuntimeImplementationWorkLedger, s RuntimeImplementationWorkLedgerSources) error {
	if !implementationWorkExactRef(s.StartupInputHash) || !implementationWorkExactRef(string(s.WorkspaceID)) || !implementationWorkExactRef(string(s.RunID)) || s.MaxRecords < 1 || len(l.Rounds) > s.MaxRecords {
		return fmt.Errorf("implementation work ledger requires bounded exact startup sources")
	}
	if err := s.Seed.Validate(); err != nil {
		return err
	}
	if err := ValidateRuntimeDefinitionFocuses(s.Seed, s.Focuses); err != nil {
		return err
	}
	if err := ValidateRuntimeDefinitionImplementationWorkPolicies(s.Focuses, s.Profiles); err != nil {
		return err
	}
	previous := map[common.CoverageWorkItemID]RuntimeImplementationWorkRound{}
	requests, bindings, dispatches := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for index, r := range l.Rounds {
		if err := r.Validate(); err != nil {
			return fmt.Errorf("implementation work round %d: %w", index, err)
		}
		if r.WorkspaceID != s.WorkspaceID || r.RunID != s.RunID || r.StartupInputHash != s.StartupInputHash || r.DefinitionSeedHash != s.Seed.Hash {
			return fmt.Errorf("implementation work ledger round escapes sealed startup")
		}
		scopeSources := RuntimeImplementationWorkScopeSources{WorkspaceID: s.WorkspaceID, RunID: s.RunID, AuthoritySnapshotID: r.ScopeRequest.AuthoritySnapshotID,
			Seed: s.Seed, Focuses: s.Focuses, Profiles: s.Profiles, CoverageWorkItemID: r.CoverageWorkItemID, ManifestBinding: r.ScopeRequest.ManifestBinding}
		if err := referenceWorkScopeAgainstSources(r.ScopeRequest, scopeSources); err != nil {
			return err
		}
		if r.WorkingBinding != nil {
			for _, proposal := range r.WorkingBinding.Proposals {
				matched := false
				for _, focus := range s.Focuses {
					if len(focus.CoverageWorkItemIDs) == 1 && focus.CoverageWorkItemIDs[0] == r.CoverageWorkItemID && proposal.DeclarationRef == focus.EvidenceDeclarationRef {
						matched = true
					}
				}
				if !matched {
					return fmt.Errorf("implementation work proposal does not preserve original exact CWI declaration")
				}
			}
		}
		if r.CompletedReceiptSetHash != "" {
			matches := 0
			for _, set := range s.ReceiptSets {
				if set.DispatchID == r.DispatchID {
					matches++
					if _, err := ValidateRuntimeImplementationWorkReceiptSet(r, set.Members); err != nil {
						return err
					}
				}
			}
			if matches != 1 {
				return fmt.Errorf("implementation work completion has no unique independent dispatch receipt source")
			}
		}
		prior, found := previous[r.CoverageWorkItemID]
		if (!found && r.Ordinal != 1) || (found && (r.Ordinal != prior.Ordinal+1 || prior.CompletedReceiptSetHash == "" || r.PrecedingReceiptSetHash != prior.CompletedReceiptSetHash || r.PolicyHash != prior.PolicyHash)) {
			return fmt.Errorf("implementation work round does not follow durable same-CWI receipt lineage")
		}
		if found {
			if r.ScopeRequest.ManifestBinding.Manifest.Revision == prior.ScopeRequest.ManifestBinding.Manifest.Revision || r.ScopeRequest.AuthoritySnapshotID == prior.ScopeRequest.AuthoritySnapshotID {
				return fmt.Errorf("implementation work successor requires fresh owner scope and authority")
			}
			if r.WorkingBinding != nil && prior.WorkingBinding != nil && (r.WorkingBinding.BindingGeneration <= prior.WorkingBinding.BindingGeneration || r.WorkingBinding.LogicalRequestID == prior.WorkingBinding.LogicalRequestID) {
				return fmt.Errorf("implementation work successor requires new semantic/working binding generation")
			}
		}
		if requests[r.ScopeRequest.RequestID] || (r.WorkingBinding != nil && bindings[r.WorkingBinding.Hash]) || (r.DispatchID != "" && dispatches[r.DispatchID]) {
			return fmt.Errorf("implementation work ledger reuses scope/binding/dispatch identity")
		}
		requests[r.ScopeRequest.RequestID] = true
		if r.WorkingBinding != nil {
			bindings[r.WorkingBinding.Hash] = true
		}
		if r.DispatchID != "" {
			dispatches[r.DispatchID] = true
		}
		previous[r.CoverageWorkItemID] = r
	}
	return nil
}
