package workspace

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationports"
	workspaceeventmapping "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/eventmapping"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
)

func TestOriginalPreparedBootstrapMissingSourceFailsClosed(t *testing.T) {
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	accepted, fact := executionInputsOriginal(t, owners)
	r, err := reduceWorkspacePreparationBlocked(accepted, fact)
	if err != nil {
		t.Fatal(err)
	}
	w := r.Next
	w.Revision++ // repository CAS applies this owner-local revision increment
	a := w.StartupPreparation.Attempts[0]
	compiled, err := activationports.CompileOriginalPreparedBootstrap(w, a)
	if err != nil || compiled.FailureCode != "soul_source_unavailable" || compiled.Compilation != nil {
		t.Fatal("missing Soul archive became an empty activation", compiled, err)
	}
	bad := a
	bad.RequestID += "other"
	if _, err := activationports.CompileOriginalPreparedBootstrap(w, bad); err == nil {
		t.Fatal("unarchived caller identity accepted")
	}
	if _, err := activationports.CompileOriginalPreparedBootstrap(nil, a); err == nil {
		t.Fatal("missing owner state accepted")
	}
	response, err := preparationCommandResult(w, fact.Command)
	if err != nil || response.CompilationSource == nil {
		t.Fatal("owner did not supply exact detached after-image", err)
	}
	detached, err := response.CompilationSource.Model()
	if err != nil || !reflect.DeepEqual(detached.PreparationSealEvidence, w.PreparationSealEvidence) || !reflect.DeepEqual(detached.DefinitionAuthoring, w.DefinitionAuthoring) {
		t.Fatal("private source lost original archive", err)
	}
	fromDTO, err := activationports.CompileOriginalPreparedBootstrap(detached, a)
	if err != nil || !reflect.DeepEqual(fromDTO, compiled) {
		t.Fatal("typed conversion changed independent compilation", err)
	}
	raw, _ := json.Marshal(response)
	if contains := string(raw); contains != "" {
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || fields["CompilationSource"] != nil {
			t.Fatal("private compiler source escaped public serialization")
		}
	}
	badResponse := response
	other := *w
	other.Revision++
	badResponse.CompilationSource = workspaceeventmapping.PreparationCompilationSource(&other)
	if badResponse.ValidateFor(fact.Command) == nil {
		t.Fatal("mixed source revision accepted")
	}
}

// Explicit opt-in read-only reproduction of the production HTTP verifier's
// restricted fixture archive. Never reads live configuration or fetches owners.
// The ordinary suite separately tests missing sources; this check requires a
// real generated archive and must not silently fall back to a synthetic one.
func TestOriginalPreparedBootstrapArchivedProductionSource(t *testing.T) {
	root := os.Getenv("WORKORCH_PREPARATION_ARCHIVE_ROOT")
	if root == "" {
		t.Skip("requires explicit isolated STEP04 production evidence data root")
	}
	repo := workorchrepo.NewWorkspaceScopedFileRepositorySet(root).Workspaces
	items, err := repo.List(context.Background())
	if err != nil || len(items) != 1 {
		t.Fatal("requires one isolated fixture Workspace", err)
	}
	w := items[0]
	if w.ActiveRunID != "" || w.StartupPreparation == nil {
		t.Fatal("not the no-Run production fixture")
	}
	originalHistory := w.StartupPreparation.Clone()
	for _, a := range w.StartupPreparation.Attempts {
		compiled, err := activationports.CompileOriginalPreparedBootstrap(w, a)
		if err != nil || compiled.FailureCode != "" || compiled.Compilation == nil || compiled.Compilation.Bootstrap.Validate() != nil {
			t.Fatal("full original bootstrap unavailable", a.RequestID, compiled.FailureCode, string(compiled.Diagnostic), err)
		}
		detached, err := workspaceeventmapping.PreparationCompilationSource(w).Model()
		if err != nil || workspace.ValidatePreparationEvidencePersistence(detached) != nil || !reflect.DeepEqual(detached.DefinitionAuthoring, w.DefinitionAuthoring) || !reflect.DeepEqual(detached.DefinitionAuthoringHistory, w.DefinitionAuthoringHistory) || !reflect.DeepEqual(detached.PreparationSealEvidence, w.PreparationSealEvidence) {
			t.Fatal("typed source lost original production lineage", err)
		}
		fromDTO, err := activationports.CompileOriginalPreparedBootstrap(detached, a)
		if err != nil || !reflect.DeepEqual(compiled, fromDTO) {
			t.Fatal("typed source changed full original bootstrap", err)
		}
		b := compiled.Compilation.Bootstrap
		if b.Definition.StartupInputHash != a.Binding.StartupInputHash || b.Definition.Revision != a.Binding.DefinitionRevision || b.Definition.SourceObligations == nil || b.Activation.DefaultSoul == nil || b.Activation.DefaultSoul.ID != "step04-soul" || b.Activation.ExecutionContract.MaxRepairAttemptsPerFocus != 10 || b.Activation.ExecutionContract.CandidateDocumentRecovery.MaxAttempts != 3 {
			t.Fatal("lost original Definition/resource/policy input")
		}
		compiled.Compilation.Bootstrap.Activation.DefaultSoul.Instructions = "caller mutation"
		reopened, err := repo.Get(context.Background(), w.ID)
		if err != nil {
			t.Fatal(err)
		}
		again, err := activationports.CompileOriginalPreparedBootstrap(reopened, a)
		if err != nil || again.Compilation == nil || reflect.DeepEqual(compiled, again) || again.Compilation.Bootstrap.Activation.DefaultSoul.Instructions == "caller mutation" {
			t.Fatal("compiled return aliases original archive", err)
		}
		for _, mutation := range []string{"seal", "soul", "skill", "memory", "mcp", "provider", "policy", "raw_skill_projection"} {
			bad, err := repo.Get(context.Background(), w.ID)
			if err != nil {
				t.Fatal(err)
			}
			switch mutation {
			case "seal":
				bad.PreparationSealEvidence = nil
			case "soul":
				bad.PreparationSoulEvidence = nil
			case "skill":
				bad.PreparationSkillRegistryEvidence = nil
			case "memory":
				bad.PreparationMemoryInventoryEvidence = nil
			case "mcp":
				bad.PreparationMCPRegistryEvidence = nil
			case "provider":
				bad.PreparationProviderCacheEvidence = nil
			case "policy":
				e := &bad.PreparationSealEvidence[0]
				e.ExecutionInputs.Process.Before.MaxRuntimeRepairAttempts++
				e.ContentHash = e.Hash()
			case "raw_skill_projection":
				e := &bad.PreparationSkillRegistryEvidence[0]
				e.Selection.Context.Projection = append(e.Selection.Context.Projection, b.Activation.Skills...)
				e.Selection.Context.FailureCode = "caller"
				e.ContentHash = e.Hash()
			}
			if _, err := activationports.CompileOriginalPreparedBootstrap(bad, a); err == nil {
				t.Fatal("missing/rehash-substituted original source accepted", mutation)
			}
		}
		if a.State != workspace.WorkspacePreparationBlocked || !reflect.DeepEqual(*w.StartupPreparation, originalHistory) {
			t.Fatal("compilation changed authority")
		}
	}
	if len(w.StartupPreparation.Attempts) != 3 {
		t.Fatal("not the three-attempt production fixture")
	}
}
