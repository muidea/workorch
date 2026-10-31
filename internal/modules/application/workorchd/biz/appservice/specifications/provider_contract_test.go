package specifications

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	llmbiz "github.com/muidea/workorch/internal/modules/blocks/llm/biz"
	events "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	models "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestAuthoringBuilderMappingHTTPOutputContract(t *testing.T) {
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "交付目标", TemplateID: "t", AcceptanceRequirements: []string{"验收"}}
	template := workspace.WorkspaceTemplate{ID: "t", DoDItems: []string{"模板验收"}}
	record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
	record, derive, _, err := record.ReserveDerivation("derive", "provider", nil)
	if err != nil {
		t.Fatal(err)
	}
	derived := workspace.DefinitionDerivedAcceptance{AuthoringInputHash: input.InputHash(), TemplateSnapshotHash: record.TemplateSnapshotHash, AdditionalItems: []string{"派生验收"}}
	record, err = completeTestDerivation(record, derive.PermitID, &derived, "")
	if err != nil {
		t.Fatal(err)
	}
	target := runtimecontract.ProviderModelTarget{ProviderID: "mock", ModelID: "model"}
	derivation, err := DerivationCommand(derive, target)
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := AuthoringProposalCommand(record.DerivationRequests[0], target)
	if err != nil {
		t.Fatal(err)
	}
	spec := authoringCommandFixture(t, input, template, derived)
	review, err := AuthoringReviewCommand(record.DerivationRequests[0], spec, workspace.GoalReviewTraceability, target)
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range []struct {
		name    string
		request models.ProviderRequest
	}{{"derivation", mustProviderModel(t, derivation.Request)}, {"proposal", mustProviderModel(t, proposal.Request)}, {"review", mustProviderModel(t, review.Request)}} {
		for _, native := range []bool{false, true} {
			t.Run(command.name+map[bool]string{true: "/native", false: "/prompt"}[native], func(t *testing.T) {
				calls := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					text, has := body["text"]
					if has != native {
						t.Errorf("native format = %v, wanted %v", has, native)
					}
					if native {
						format := text.(map[string]any)["format"].(map[string]any)
						var expected any
						_ = json.Unmarshal([]byte(command.request.Schema), &expected)
						got, _ := json.Marshal(format["schema"])
						want, _ := json.Marshal(expected)
						if string(got) != string(want) || format["strict"] != true {
							t.Error("native schema drift")
						}
					}
					encoded, _ := json.Marshal(body["input"])
					requiredField := "SourceItemRefs"
					if command.name == "derivation" {
						requiredField = "AdditionalItems"
					}
					if !strings.Contains(string(encoded), requiredField) || !strings.Contains(string(encoded), "additionalProperties") {
						t.Error("full stable output contract missing")
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"status":"completed","output_text":"{\"unknown\":true}"}`))
				}))
				defer server.Close()
				config := models.ProviderConfig{ID: "mock", BaseURL: server.URL, Model: "model", Timeout: time.Second, Capabilities: models.ProviderCapabilities{Responses: true, JSONSchemaResponse: native}, InferenceProfile: models.ProviderInferenceProfile{MaxOutputTokens: 1024}, ContextProfile: models.ProviderContextProfile{ContextWindowTokens: 8192, MaxInputTokens: 7168, ReservedOutputTokens: 1024, MaxRequestBytes: 28672, EstimatedTokensPerByte: 0.25}}
				result, err := llmbiz.NewHTTPProvider(config).Complete(context.Background(), command.request)
				if err != nil {
					t.Fatal(err)
				}
				if calls != 1 {
					t.Fatalf("physical calls %d", calls)
				}
				if command.name == "derivation" {
					if _, err := ParseDerivedAcceptance(result.Payload, input, &template); err == nil {
						t.Error("local strict parser accepted unknown derivation field")
					}
				} else if command.name == "proposal" {
					if _, err := ParseProposal(result.Payload, workspace.DefinitionCandidateItem{CandidateItemRef: spec.GoalItemRef, Text: input.GoalText}, nil); err == nil {
						t.Error("local strict parser accepted unknown wrapper")
					}
				} else {
					if _, err := ParseReview(result.Payload, workspace.GoalReviewTraceability, spec); err == nil {
						t.Error("local strict parser accepted unknown wrapper")
					}
				}
			})
		}
	}
}

func mustProviderModel(t *testing.T, request events.ProviderRequest) models.ProviderRequest {
	t.Helper()
	result, err := request.ToModel()
	if err != nil {
		t.Fatal(err)
	}
	return result
}
