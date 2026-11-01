package llm

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestOperationsIntentProjectionIntegrityAndWorkspaceIsolation(t *testing.T) {
	for n := range 4 {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			t.Parallel()
			id := fmt.Sprint("workspace-", n)
			binding := &runtimecontract.RuntimeImplementationWorkIntentBinding{Source: runtimecontract.RuntimeImplementationWorkPurposeChoice{SemanticManifestHash: id + "/manifest", LogicalRequestID: id + "/request", DocumentHash: id + "/document", ScopeRequestID: id + "/scope", CoverageWorkItemID: "original", Purpose: runtimecontract.RuntimeImplementationWorkPurposeEffect}, Intent: &runtimecontract.RuntimeImplementationIntent{DeclarationRef: "original", ContentHash: id + "/content", RemainingWork: "implement task " + id, Reason: "original unmet behavior", SelectedRequirementRefs: []string{"original"}}}
			manifest := runtimecontract.RuntimeCandidateContractManifest{ImplementationWorkingBinding: &runtimecontract.RuntimeImplementationWorkingBinding{IntentBinding: binding}}
			context := runtimecontract.RuntimeCandidateContext{WorkspaceID: common.WorkspaceID(id), RunID: "run-1"}
			projection := runtimeCandidateAuthorityContextProjection(context, manifest)
			if !reflect.DeepEqual(projection.ImplementationIntent, binding) {
				t.Fatal("operations lost original complete intent")
			}
			render := func(p runtimeCandidateAuthorityProjection) *llmmodels.PromptContext {
				raw, err := json.Marshal(struct {
					Context runtimeCandidateAuthorityProjection `json:"context"`
				}{p})
				if err != nil {
					t.Fatal(err)
				}
				return &llmmodels.PromptContext{Sections: []llmmodels.PromptSection{{ID: "C3.runtime-authority", Content: string(raw)}}}
			}
			prompt := render(projection)
			if err := validateRuntimeCandidateWorkIntentIntegrity(manifest, prompt); err != nil {
				t.Fatal(err)
			}
			for _, kind := range []string{"missing", "changed", "source", "foreign", "duplicate"} {
				changed := runtimeCandidateAuthorityContextProjection(context, manifest)
				switch kind {
				case "missing":
					changed.ImplementationIntent = nil
				case "changed":
					changed.ImplementationIntent.Intent.RemainingWork = "another task"
				case "source":
					changed.ImplementationIntent.Source.DocumentHash = "self-rehashed"
				case "foreign":
					changed.ImplementationIntent.Intent.ContentHash = "another-workspace/content"
				}
				bad := render(changed)
				if kind == "duplicate" {
					bad.Sections = append(bad.Sections, bad.Sections[0])
				}
				if err := validateRuntimeCandidateWorkIntentIntegrity(manifest, bad); err == nil {
					t.Fatal("post-render intent tampering reached provider", kind)
				}
			}
			if binding.Intent.RemainingWork != "implement task "+id {
				t.Fatal("projection mutated sealed source")
			}
			if err := validateRuntimeCandidateWorkIntentIntegrity(runtimecontract.RuntimeCandidateContractManifest{}, prompt); err == nil {
				t.Fatal("ordinary operation borrowed implementation intent")
			}
		})
	}
}
