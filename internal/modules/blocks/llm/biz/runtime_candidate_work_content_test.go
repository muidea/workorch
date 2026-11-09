package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	contract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestOperationsFrozenWorkContentRenderGateAndIsolation(t *testing.T) {
	for _, body := range []string{"original 中文\r\n", "another workspace source\n"} {
		t.Run(body, func(t *testing.T) {
			t.Parallel()
			sum := sha256.Sum256([]byte(body))
			c := contract.RuntimeCandidateContext{ImplementationWorkContent: &contract.RuntimeImplementationWorkContent{ProtocolRef: contract.RuntimeImplementationWorkContentV1, ManifestRevision: "tree", Snapshots: []contract.TargetSnapshot{{TargetRef: "src.txt", Content: body, ContentHash: hex.EncodeToString(sum[:]), ContentState: contract.SnapshotContentStateComplete, Size: int64(len(body))}}}}
			m := contract.RuntimeCandidateContractManifest{RequiredOperationSequence: []contract.RuntimeRequiredOperationVariant{{RequirementID: "edit", TargetRef: "src.txt"}}, ImplementationWorkingBinding: &contract.RuntimeImplementationWorkingBinding{IntentBinding: &contract.RuntimeImplementationWorkIntentBinding{Source: contract.RuntimeImplementationWorkPurposeChoice{ScopeRequestID: "scope"}, Intent: &contract.RuntimeImplementationIntent{ContentHash: c.ImplementationWorkContent.Hash()}}}}
			render := func() *llmmodels.PromptContext {
				projection := buildRuntimeCandidateExecutionTailProjection(c, 0, nil, m, nil)
				raw, _ := json.Marshal(projection)
				return &llmmodels.PromptContext{Sections: []llmmodels.PromptSection{{ID: "C4.execution-tail", Content: string(raw)}}}
			}
			p := render()
			if _, err := runtimeCandidateExecutionTailForIntegrity(c, m, p); err != nil {
				t.Fatal(err)
			}
			for _, which := range []string{"missing", "altered", "duplicate", "foreign"} {
				p := render()
				switch which {
				case "missing":
					p.Sections[0].Content = `{}`
				case "altered":
					var projection runtimeCandidateExecutionTailProjection
					json.Unmarshal([]byte(p.Sections[0].Content), &projection)
					projection.ImplementationWorkContent.Snapshots[0].Content += "drift"
					raw, _ := json.Marshal(projection)
					p.Sections[0].Content = string(raw)
				case "duplicate":
					p.Sections = append(p.Sections, p.Sections[0])
				case "foreign":
					var projection runtimeCandidateExecutionTailProjection
					json.Unmarshal([]byte(p.Sections[0].Content), &projection)
					projection.ImplementationWorkContent.ScopeRequestID = "foreign"
					raw, _ := json.Marshal(projection)
					p.Sections[0].Content = string(raw)
				}
				if _, err := runtimeCandidateExecutionTailForIntegrity(c, m, p); err == nil {
					t.Fatal("post-render change reached provider", which)
				}
			}
			if c.ImplementationWorkContent.Snapshots[0].Content != body {
				t.Fatal("render changed original source")
			}
		})
	}
}
