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
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/testkit"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestAuthoringHTTPCurrentProposalRoundTripAndSemanticRejects(t *testing.T) {
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "交付目标", TemplateID: "t", AcceptanceRequirements: []string{"用户验收"}}
	template := workspace.WorkspaceTemplate{ID: "t", DoDItems: []string{"模板验收"}}
	record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
	_, request, _, err := record.ReserveDerivation("derive", "provider", nil)
	if err != nil {
		t.Fatal(err)
	}
	derived := workspace.DefinitionDerivedAcceptance{AuthoringInputHash: input.InputHash(), TemplateSnapshotHash: record.TemplateSnapshotHash, AdditionalItems: []string{"派生验收"}}
	request.Proposal = &derived
	command, err := AuthoringProposalCommand(request, runtimecontract.ProviderModelTarget{ProviderID: "mock", ModelID: "model"})
	if err != nil {
		t.Fatal(err)
	}
	mapping, _, spec, err := testkit.Proposal(request, workspace.DefinitionAuthoringVerificationMapping{Status: workspace.GoalVerificationUnverified, Declaration: "待评估"})
	if err != nil {
		t.Fatal(err)
	}
	for _, native := range []bool{false, true} {
		for _, scenario := range []string{"valid", "resource_chapter_as_source", "unverified_claims_binding", "wrong_source_kind", "wrong_field_case"} {
			t.Run(scenario+map[bool]string{false: "/prompt", true: "/native"}[native], func(t *testing.T) {
				proposal := mapping.Clone()
				switch scenario {
				case "resource_chapter_as_source":
					proposal.Requirements[0].SourceItemRefs = append(proposal.Requirements[0].SourceItemRefs, "prd.md#2.5")
				case "unverified_claims_binding":
					proposal.Obligations[0].Verification.ProfileID, proposal.Obligations[0].Verification.EvidenceRequirementID, proposal.Obligations[0].Verification.Scope = "invented", "*", "dodItem"
				}
				payload, err := json.Marshal(struct {
					Mapping workspace.DefinitionAuthoringMapping `json:"mapping"`
				}{proposal})
				if err != nil {
					t.Fatal(err)
				}
				if scenario == "wrong_source_kind" {
					// 来源由 owner 编译；provider 连自报同名字段也不能混入当前 wire。
					payload = []byte(strings.Replace(string(payload), `"SourceItemRefs":`, `"SourceKind":"DERIVED","SourceItemRefs":`, 1))
				}
				if scenario == "wrong_field_case" {
					payload = []byte(strings.Replace(string(payload), `"GoalRequirement"`, `"goalrequirement"`, 1))
				}
				calls := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					var body map[string]json.RawMessage
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if _, has := body["text"]; has != native {
						t.Errorf("native schema mapping=%v want=%v", has, native)
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]any{"status": "completed", "output_text": string(payload)})
				}))
				defer server.Close()
				config := llmmodels.ProviderConfig{ID: "mock", BaseURL: server.URL, Model: "model", Timeout: time.Second, Capabilities: llmmodels.ProviderCapabilities{Responses: true, JSONSchemaResponse: native}, InferenceProfile: llmmodels.ProviderInferenceProfile{MaxOutputTokens: 1024}, ContextProfile: llmmodels.ProviderContextProfile{ContextWindowTokens: 16384, MaxInputTokens: 15360, ReservedOutputTokens: 1024, MaxRequestBytes: 61440, EstimatedTokensPerByte: 0.25}}
				result, err := llmbiz.NewHTTPProvider(config).Complete(context.Background(), mustProviderModel(t, command.Request))
				if err != nil {
					t.Fatal(err)
				}
				parsed, parseErr := ParseAuthoringProposal(result.Payload, request)
				if scenario == "valid" {
					if parseErr != nil || parsed.ContentHash() != spec.ContentHash() {
						t.Fatalf("合法来源图未贯通 owner: %v", parseErr)
					}
				} else if parseErr == nil {
					t.Fatal("语法或业务反例通过 owner 合同")
				}
				if calls != 1 {
					t.Fatalf("physical calls=%d，非法输出不得自动 fallback", calls)
				}
			})
		}
	}
}
