package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	contract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"reflect"
	"strings"
	"testing"
)

func TestRuntimeSemanticWorkContentStaysInVolatileTailAndCannotBeDropped(t *testing.T) {
	for _, renderer := range []string{contract.RuntimeSemanticContextEncoding} {
		t.Run(renderer, func(t *testing.T) {
			request := runtimeSemanticWorkProviderRequestForTest(t, true)
			request.ExpectedDocument = &contract.RuntimeExpectedDocumentContract{SemanticContextEncoding: renderer}
			baseline, err := prepareRuntimeSemanticPlanRequest(request)
			if err != nil {
				t.Fatal(err)
			}
			old, err := buildRuntimeSemanticPrompt(request, baseline)
			if err != nil {
				t.Fatal(err)
			}
			w := request.SemanticManifest.ProofObligations[0].ImplementationWork
			body := "exact current content\n"
			hash := sha256.Sum256([]byte(body))
			w.Scope.Content = &contract.RuntimeImplementationWorkContent{ProtocolRef: contract.RuntimeImplementationWorkContentV1, ManifestRevision: w.Scope.Selection.ObservedRevision, Snapshots: []contract.TargetSnapshot{{TargetRef: w.Scope.Selection.TargetRefs[0], Content: body, ContentHash: hex.EncodeToString(hash[:]), ContentState: contract.SnapshotContentStateComplete, Size: int64(len(body))}}}
			request.SemanticManifest.Hash = contract.StableRuntimeSemanticPlanManifestHash(request.SemanticManifest)
			request.SemanticContext.WorkContext = &contract.RuntimeSemanticWorkContext{Content: w.Scope.Content.Clone()}
			request.SemanticContext.CoverageProgress.ManifestHash = request.SemanticManifest.Hash
			request.SemanticContext.CoverageProgress.Hash = contract.StableRuntimeSemanticCoverageProgressHash(request.SemanticContext.CoverageProgress)
			request.SemanticContext.ExecutionHistory.ManifestHash = request.SemanticManifest.Hash
			request.SemanticContext.ExecutionHistory.Hash = contract.StableRuntimeSemanticExecutionHistoryHash(request.SemanticContext.ExecutionHistory)
			request.SemanticContext.ExecutionBudget.ManifestHash = request.SemanticManifest.Hash
			request.SemanticContext.ExecutionBudget.Hash = contract.StableRuntimeSemanticExecutionBudgetHash(request.SemanticContext.ExecutionBudget)
			prepared, err := prepareRuntimeSemanticPlanRequest(request)
			if err != nil {
				t.Fatal(err)
			}
			prompt, err := buildRuntimeSemanticPrompt(request, prepared)
			if err != nil {
				t.Fatal(err)
			}
			if prompt.definition != old.definition || prompt.capability != old.capability || prepared.stablePrefix != baseline.stablePrefix {
				t.Fatal("volatile content changed stable C1/C2/prefix")
			}
			if !strings.Contains(prompt.authority, `exact current content\n`) || strings.Contains(prompt.tail, `exact current content\n`) {
				t.Fatal("exact file body not solely in C4")
			}
			if !strings.Contains(runtimeSemanticOutputInstruction(prepared.context, prepared.manifest), "implementationIntent") {
				t.Fatal("prompt did not describe typed intent")
			}
			provider := newRuntimeSemanticProviderRequest(request, prepared, prompt)
			// Use the normal owner budget assembly and then independently remove one
			// protected source field. Exact integrity must reject the changed projection.
			budget := provider.ContextBudget
			if err := validateRuntimeSemanticContextIntegrity(prepared.context, prepared.manifest, 0, nil, nil, budget, provider.PromptContext); err != nil {
				t.Fatal(err)
			}
			for i := range provider.PromptContext.Sections {
				if strings.HasPrefix(provider.PromptContext.Sections[i].ID, "C3.") {
					var authority runtimeSemanticAuthorityProjection
					json.Unmarshal([]byte(provider.PromptContext.Sections[i].Content), &authority)
					if !reflect.DeepEqual(authority.WorkContext.Content, prepared.context.WorkContext.Content) {
						t.Fatal("projection changed original content")
					}
					authority.WorkContext.Content = nil
					raw, _ := json.Marshal(authority)
					provider.PromptContext.Sections[i].Content = string(raw)
				}
			}
			if validateRuntimeSemanticContextIntegrity(prepared.context, prepared.manifest, 0, nil, nil, budget, provider.PromptContext) == nil {
				t.Fatal("dropped current body passed integrity")
			}
		})
	}
}
