package preparationcompiler

import (
	"fmt"
	"reflect"
	"time"

	filepreparation "github.com/muidea/workorch/internal/modules/blocks/file/pkg/preparation"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/startuppreparation"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const FirstExecutionPreparationVersion = "workorch-first-execution-preparation/v1"

// FirstExecutionPreparation is a detached compilation of complete owner inputs,
// not a transferable validation credential or a Run activation permission. The
// owner must recompile against its actual reservation CAS preimage. In
// particular this product cannot be reused for Restart after a fresh intent.
type FirstExecutionPreparation struct {
	Version                   string
	CompletionID              string
	RequestID                 string
	PayloadHash               string
	CreationEpoch             string
	ExpectedWorkspaceRevision uint64
	Compilation               PreparedBootstrapCompilation
}

// CompileObservedFirstExecutionPreparation adds an actual File-owner reread to
// the historical completion gate. An unchanged owner epoch alone says nothing
// about file contents. The observation is not a lease and never extends the
// original completion window. All scope and source bytes are compared.
func CompileObservedFirstExecutionPreparation(w *workspace.Workspace, goal *workspace.Goal, dod *workspace.DoD, bootstrap runtimecontract.RuntimeBootstrap, now time.Time, files filepreparation.Observation) (FirstExecutionPreparation, error) {
	compiled, err := CompileFirstExecutionPreparation(w, goal, dod, bootstrap, now, files.OwnerEpoch)
	if err != nil {
		return FirstExecutionPreparation{}, err
	}
	a := w.StartupPreparation.Attempts[len(w.StartupPreparation.Attempts)-1]
	for _, completion := range w.StartupPreparation.Completions {
		if completion.CompletionID == compiled.CompletionID {
			if err := validateFirstExecutionFiles(w, a, completion.CompletedAt, now, files); err != nil {
				return FirstExecutionPreparation{}, err
			}
			return compiled, nil
		}
	}
	return FirstExecutionPreparation{}, common.ErrConflict
}

func validateFirstExecutionFiles(w *workspace.Workspace, a workspace.WorkspacePreparationAttempt, completed, consumed time.Time, files filepreparation.Observation) error {
	if files.Validate() != nil || files.StartedAt.Before(completed) || consumed.Before(files.ObservedAt) || !consumed.Before(files.ExpiresAt) {
		return common.ErrConflict
	}
	var previous *filepreparation.Observation
	for _, d := range w.PreparationDirectoryEvidence {
		if d.ContentHash == a.DirectoryEvidenceHash {
			if previous != nil || d.Revalidation == nil {
				return common.ErrConflict
			}
			previous = d.Revalidation
		}
	}
	if previous == nil || !reflect.DeepEqual(previous.Scope, files.Scope) || previous.OwnerEpoch != files.OwnerEpoch ||
		!sameFirstExecutionInternalDirectory(previous.Internal, files.Internal) || !reflect.DeepEqual(previous.Deliverable, files.Deliverable) ||
		!reflect.DeepEqual(previous.Resources, files.Resources) || !reflect.DeepEqual(previous.Configs, files.Configs) {
		return common.ErrConflict
	}
	return nil
}

// The Workspace owner stores its aggregate below the internal directory. Its
// completion CAS necessarily changes that directory's entry timestamps. A
// directory observation proves identity and access, not a recursive content
// snapshot. Preserve both original observations; compare every boundary field
// and independently compare all selected resource/config observations in full.
// The deliverable directory above retains the stricter complete comparison.
func sameFirstExecutionInternalDirectory(before, after fileobservation.Directory) bool {
	if before.Validate() != nil || after.Validate() != nil || !before.Complete || !after.Complete {
		return false
	}
	before.ModifiedSeconds, before.ModifiedNanos = after.ModifiedSeconds, after.ModifiedNanos
	before.ChangedSeconds, before.ChangedNanos = after.ChangedSeconds, after.ChangedNanos
	return reflect.DeepEqual(before, after)
}

// CompileFirstExecutionPreparation does not perform I/O, renew observations or
// select a completion by hash. Only the latest complete original operation can
// supply the first-execution preimage; a newer blocked/unknown attempt prevents
// falling back to an older successful check. FileEpoch must come from the File
// owner, not the archived observation. Actual formal rows are supplied by the
// Workspace owner separately from the journal's expected after-image.
func CompileFirstExecutionPreparation(w *workspace.Workspace, goal *workspace.Goal, dod *workspace.DoD, bootstrap runtimecontract.RuntimeBootstrap, now time.Time, fileEpoch string) (FirstExecutionPreparation, error) {
	fail := func(stage string) (FirstExecutionPreparation, error) {
		return FirstExecutionPreparation{}, fmt.Errorf("%w: first execution preparation %s", common.ErrConflict, stage)
	}
	if w == nil || now.IsZero() || fileEpoch == "" || w.Deletion != nil || w.ActiveRunID != "" || w.PreparationFence == nil || w.PreparationFence.Validate(w.ID) != nil || w.PreparationFence.PreparationEpoch != 0 || w.PreparationFence.Pending != nil || len(w.PreparationFence.Settled) != 0 || w.StartupPreparation == nil || len(w.StartupPreparation.Attempts) == 0 {
		return fail("scope")
	}
	if w.StartupPreparation.Validate(w.ID) != nil || ValidatePreparedPersistence(w) != nil {
		return fail("history")
	}
	a := w.StartupPreparation.Attempts[len(w.StartupPreparation.Attempts)-1]
	var completion *workspace.WorkspacePreparationCompletion
	for i := range w.StartupPreparation.Completions {
		c := &w.StartupPreparation.Completions[i]
		if c.RequestID == a.RequestID {
			if completion != nil {
				return fail("duplicate_completion")
			}
			completion = c
		}
	}
	if completion == nil || completion.ValidateFor(a) != nil || completion.CommittedWorkspaceRevision != w.Revision || completion.CreationEpoch != w.PreparationFence.CreationEpoch || now.Before(completion.CompletedAt) || !now.Before(completion.ExpiresAt) || a.Binding.CheckPolicyVersion != startuppreparation.CheckPolicyVersion || a.Binding.CompilerVersion != startuppreparation.BootstrapCompilerVersion {
		return fail("completion")
	}
	if stale, err := workspace.PreparationOwnerRevalidationStaleForAttempt(w, a, now); err != nil || stale {
		return fail("owners")
	}
	if stale, err := workspace.PreparationFileRevalidationStaleForAttempt(w, a, now, fileEpoch); err != nil || stale {
		return fail("file")
	}
	if stale, err := workspace.PreparationProviderCacheExpired(w, now); err != nil || stale {
		return fail("provider")
	}
	current, err := startuppreparation.FromOwnerPersistence(w, goal, dod)
	if err != nil || current.Candidate.CandidateID != a.Binding.CandidateID || current.Candidate.CandidateRevision != a.Binding.CandidateRevision || current.Candidate.CandidateHash != a.Binding.CandidateHash || current.StartupInput.DefinitionRevision != a.Binding.DefinitionRevision || current.StartupInput.StartupInputHash != a.Binding.StartupInputHash || current.AuthoringRef != a.Binding.AuthoringRef || current.AuthoringConfigurationHash != a.Binding.AuthoringConfigurationHash {
		return fail("seal")
	}
	original, err := CompileOriginalPreparedBootstrap(w, a)
	if err != nil || original.FailureCode != "" || original.Compilation == nil || original.Compilation.BootstrapHash != completion.BootstrapHash || runtimecontract.ValidateRuntimeBootstrapRaw(bootstrap) != nil || !reflect.DeepEqual(original.Compilation.Bootstrap, bootstrap) {
		return fail("bootstrap")
	}
	compiled := *original.Compilation
	compiled.Bootstrap = compiled.Bootstrap.Clone()
	return FirstExecutionPreparation{Version: FirstExecutionPreparationVersion, CompletionID: completion.CompletionID, RequestID: completion.RequestID, PayloadHash: completion.PayloadHash, CreationEpoch: completion.CreationEpoch, ExpectedWorkspaceRevision: w.Revision, Compilation: compiled}, nil
}
