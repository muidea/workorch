package workorchruntimecontract

import (
	"maps"
	"slices"
	"sync"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeOwnerHistoricalCompiler is local to one Orchestrator controller. It
// exposes a complete lexical gate, never a session, success credential or raw
// product. It must not be passed through an owner port or serialized.
// runtime-owner-historical-subtree-compiler/v2 retains only detached pure
// semantic subresults, not current facts, aggregate invariants or CAS authority.
type RuntimeOwnerHistoricalCompiler struct {
	mu        sync.Mutex
	closed    bool
	workspace common.WorkspaceID
	run       common.RunID
	semantic  *runtimeSemanticValidationSession
}

// One scope and at most this many complete entries survive an owner call. Eviction
// is all-or-nothing and only loses an optimization; no source is truncated.
const runtimeOwnerHistoricalEntryLimit = 128

type runtimeOwnerHistoricalGeneration struct {
	raw, normalized RuntimeSemanticPlanningGeneration
}

type runtimeOwnerHistoricalPurpose struct {
	raw     RuntimeSemanticPlanningLedger
	entries []runtimeAssessmentPurposeBindingPreimage
}

type runtimeOwnerHistoricalSemanticCompiler struct {
	generations []runtimeOwnerHistoricalGeneration
	purposes    []runtimeOwnerHistoricalPurpose
	lineage     *runtimeOwnerLineageIndex
}

// This is invoked only for a retired generation's complete immutable snapshot,
// never for the active ledger. The entries stay private and read-only; public
// resolvers detach every returned purpose/binding as before.
func runtimeHistoricalPurposeInOwnerCompiler(ledger RuntimeSemanticPlanningLedger, session *runtimeSemanticValidationSession) ([]runtimeAssessmentPurposeBindingPreimage, error) {
	c := session.ownerHistory
	if c != nil {
		for _, entry := range c.purposes {
			if runtimeRawEqual(entry.raw, ledger) {
				return entry.entries, nil
			}
		}
	}
	entries, err := runtimeAssessmentPurposePreimagesFromSemanticGenerationInSession(ledger, session)
	if err != nil {
		return nil, err
	}
	if c != nil {
		c.purposes = append(c.purposes, runtimeOwnerHistoricalPurpose{raw: cloneRuntimeSemanticPlanningLedgerRaw(ledger), entries: entries})
	}
	return entries, nil
}

type runtimeOwnerHistoricalScope struct {
	workspace common.WorkspaceID
	run       common.RunID
}

func (s runtimeOwnerHistoricalScope) matches(image RuntimeOwnerStateSourceImage) bool {
	return image.Sources.Facts.WorkspaceID == s.workspace && image.Sources.Facts.RunID == s.run &&
		(image.Original == nil || image.Original.Facts.WorkspaceID == s.workspace && image.Original.Facts.RunID == s.run)
}

// Compile still requires every full image and every ordered gate on each
// invocation. Contention (including recursive use) takes a fresh cold path,
// without borrowing another call's mutable compilation or delaying owner I/O.
func (c *RuntimeOwnerHistoricalCompiler) Compile(workspace common.WorkspaceID, run common.RunID, body func(func(RuntimeOwnerStateSourceImage) error) error) (err error) {
	if c == nil || !c.mu.TryLock() {
		return CompileRuntimeOwnerStateTransaction(body)
	}
	defer c.mu.Unlock()
	if c.closed {
		return CompileRuntimeOwnerStateTransaction(body)
	}
	if workspace == "" || run == "" {
		c.semantic = nil
		c.workspace, c.run = "", ""
		return CompileRuntimeOwnerStateTransaction(body)
	}
	semantic := c.semantic
	if semantic == nil || c.workspace != workspace || c.run != run {
		semantic = newRuntimeSemanticValidationSession()
		semantic.ownerHistory = &runtimeOwnerHistoricalSemanticCompiler{}
	}
	// Remove the retained reference while the owner call runs. Failure or panic
	// discards even its successful subresults, preserving cold retry semantics.
	c.semantic = nil
	succeeded := false
	defer func() {
		if succeeded {
			c.workspace, c.run = workspace, run
			c.semantic = retainRuntimeOwnerHistoricalSubtrees(semantic)
		}
	}()
	err = compileRuntimeOwnerStateTransaction(body, semantic, &runtimeOwnerHistoricalScope{workspace: workspace, run: run})
	succeeded = err == nil
	return err
}

// Close releases all retained inputs after the owner's actual drain. Subsequent
// calls stay correct through the cold gate; they cannot resurrect the cache.
func (c *RuntimeOwnerHistoricalCompiler) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed, c.semantic = true, nil
	c.workspace, c.run = "", ""
}

// v2 retains only complete entries from the existing whitelist under the same
// global 128-entry bound. Overflow evicts optional pure products, never owner
// facts or history. No retained product is permission to skip a current gate.
func retainRuntimeOwnerHistoricalSubtrees(s *runtimeSemanticValidationSession) *runtimeSemanticValidationSession {
	remaining := runtimeOwnerHistoricalEntryLimit
	n := newRuntimeSemanticValidationSession()
	n.seeds = retainRuntimeOwnerHistoricalSuffix(s.seeds, &remaining)
	n.workDefinitions = retainRuntimeOwnerHistoricalSuffix(s.workDefinitions, &remaining)
	n.workScopes = retainRuntimeOwnerHistoricalSuffix(s.workScopes, &remaining)
	n.workRounds = retainRuntimeOwnerHistoricalSuffix(s.workRounds, &remaining)
	n.definitionSources = retainRuntimeOwnerHistoricalSuffix(s.definitionSources, &remaining)
	if s.ownerHistory != nil {
		n.ownerHistory = &runtimeOwnerHistoricalSemanticCompiler{
			generations: retainRuntimeOwnerHistoricalSuffix(s.ownerHistory.generations, &remaining),
			purposes:    retainRuntimeOwnerHistoricalSuffix(s.ownerHistory.purposes, &remaining),
		}
		if old := s.ownerHistory.lineage; old != nil {
			n.ownerHistory.lineage = &runtimeOwnerLineageIndex{
				dispatches:    retainRuntimeOwnerHistoricalMap(old.dispatches, &remaining),
				catalogs:      retainRuntimeOwnerHistoricalMap(old.catalogs, &remaining),
				receipts:      retainRuntimeOwnerHistoricalMap(old.receipts, &remaining),
				confirmations: retainRuntimeOwnerHistoricalMap(old.confirmations, &remaining),
			}
		}
	}
	n.catalogs = retainRuntimeOwnerHistoricalMap(s.catalogs, &remaining)
	n.manifests = retainRuntimeOwnerHistoricalMap(s.manifests, &remaining)
	// Manifest validation and document validation are independent complete pure
	// entries. Keep the complete manifest first; an evicted document is validated
	// afresh against it, with the original error order. Copy the mutable cache
	// container rather than modifying the completed transaction's session.
	for _, key := range slices.Sorted(maps.Keys(n.manifests)) {
		for i := range n.manifests[key] {
			entry := &n.manifests[key][i]
			if old := entry.validated.documents; old != nil {
				entry.validated.documents = &runtimeSemanticDocumentValidationSession{
					manifest: old.manifest,
					values:   retainRuntimeOwnerHistoricalSuffix(old.values, &remaining),
				}
			}
		}
	}
	// Explicit whitelist only. Never retain workStates/current Facts/budget,
	// whole-source/owner-invariant/aggregate success or transaction products.
	return n
}

func retainRuntimeOwnerHistoricalSuffix[T any](entries []T, remaining *int) []T {
	count := min(len(entries), *remaining)
	*remaining -= count
	if count == 0 {
		return nil
	}
	// Detach the container so evicted entries cannot remain reachable through
	// its backing array; the selected raw entries already own detached inputs.
	return slices.Clone(entries[len(entries)-count:])
}

func retainRuntimeOwnerHistoricalMap[T any](entries map[string][]T, remaining *int) map[string][]T {
	result := make(map[string][]T)
	// Deterministic eviction: keys order candidates only, never certify content.
	// Every hit still requires complete exact raw equality in the original gate.
	for _, key := range slices.Sorted(maps.Keys(entries)) {
		if kept := retainRuntimeOwnerHistoricalSuffix(entries[key], remaining); len(kept) > 0 {
			result[key] = kept
		}
	}
	return result
}
