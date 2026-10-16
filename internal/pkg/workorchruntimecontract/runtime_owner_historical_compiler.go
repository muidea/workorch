package workorchruntimecontract

import (
	"reflect"
	"sync"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeOwnerHistoricalCompiler is local to one Orchestrator controller. It
// exposes a complete lexical gate, never a session, success credential or raw
// product. It must not be passed through an owner port or serialized.
// runtime-owner-historical-subtree-compiler/v1 retains only detached pure
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
			if reflect.DeepEqual(entry.raw, ledger) {
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

func retainRuntimeOwnerHistoricalSubtrees(s *runtimeSemanticValidationSession) *runtimeSemanticValidationSession {
	n := len(s.workScopes) + len(s.workDefinitions) + len(s.seeds) + len(s.workRounds) + len(s.definitionSources)
	if s.ownerHistory != nil {
		n += len(s.ownerHistory.generations) + len(s.ownerHistory.purposes) + s.ownerHistory.lineage.count()
	}
	for _, entries := range s.catalogs {
		n += len(entries)
	}
	for _, entries := range s.manifests {
		n += len(entries)
		for _, entry := range entries {
			if entry.validated.documents != nil {
				n += len(entry.validated.documents.values)
			}
		}
	}
	if n > runtimeOwnerHistoricalEntryLimit {
		return nil
	}
	// Explicit allowlist: never retain workStates (which include current Facts
	// and budget), the historical full-source compiler, owner continuations or
	// transaction products. Each retained entry already owns exact raw copies.
	return &runtimeSemanticValidationSession{
		manifests: s.manifests, catalogs: s.catalogs,
		workScopes: s.workScopes, workDefinitions: s.workDefinitions,
		seeds: s.seeds, workRounds: s.workRounds, definitionSources: s.definitionSources,
		ownerHistory: s.ownerHistory,
	}
}
