package events

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	models "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCompleteProviderDocumentContractRejectsIdentityAndOutcomeTampering(t *testing.T) {
	command := CompleteProviderDocumentCommand{RequestIdentity: "reserved-id", Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "model"}, ExpectedConfigurationRevision: "config-revision", Request: ProviderRequest{Model: "model", Kind: "opaque-kind"}}
	hash, err := command.RequestHash()
	if err != nil {
		t.Fatal(err)
	}
	content := models.NewProviderObservedContent([]byte("原始内容"), true, "")
	valid := CompleteProviderDocumentResult{RequestIdentity: command.RequestIdentity, RequestHash: hash, Target: command.Target, ConfigurationRevision: command.ExpectedConfigurationRevision, Document: &ProviderCompleteDocument{Content: content.Clone()}, ObservedDocument: content, Wire: models.ProviderObservedContent{Reason: "not_observed"}, ProviderSubmitted: true, PhysicalResponseMode: llmaudit.PhysicalResponseModeStreaming}
	if err := valid.ValidateFor(command); err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*CompleteProviderDocumentResult){
		"identity":      func(r *CompleteProviderDocumentResult) { r.RequestIdentity = "other" },
		"hash":          func(r *CompleteProviderDocumentResult) { r.RequestHash += "changed" },
		"target":        func(r *CompleteProviderDocumentResult) { r.Target.ModelID = "other" },
		"configuration": func(r *CompleteProviderDocumentResult) { r.ConfigurationRevision = "other" },
		"multiple arms": func(r *CompleteProviderDocumentResult) {
			r.Failure = &ProviderDocumentFailure{Code: "http_400", Cause: []byte("classified failure")}
		},
		"no arm":                func(r *CompleteProviderDocumentResult) { r.Document = nil },
		"mode":                  func(r *CompleteProviderDocumentResult) { r.PhysicalResponseMode = " streaming " },
		"different observation": func(r *CompleteProviderDocumentResult) { r.ObservedDocument.Reason = "different" },
		"unsubmitted failure": func(r *CompleteProviderDocumentResult) {
			r.Document = nil
			r.ProviderSubmitted = false
			r.Failure = &ProviderDocumentFailure{Code: "http_400", Cause: []byte("classified failure")}
		},
		"noncanonical code": func(r *CompleteProviderDocumentResult) {
			r.Document = nil
			r.Failure = &ProviderDocumentFailure{Code: " http_400 ", Cause: []byte("classified failure")}
		},
		"invalid incomplete": func(r *CompleteProviderDocumentResult) {
			r.Document = nil
			r.ObservedDocument = models.ProviderObservedContent{Reason: "not_observed"}
			r.Incomplete = &ProviderIncompleteDocument{Code: "completed", Cause: []byte("classified failure")}
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			altered := valid.Clone()
			mutate(&altered)
			if altered.ValidateFor(command) == nil {
				t.Fatal("tampered result accepted")
			}
		})
	}
	alteredCommand := command.Clone()
	alteredCommand.Request.WorkspaceID = "other-workspace"
	if valid.ValidateFor(alteredCommand) == nil {
		t.Fatal("result rebound to another request")
	}
	encoded, err := json.Marshal(valid)
	if err != nil || strings.Contains(string(encoded), "原始内容") {
		t.Fatal("ordinary JSON exposed raw diagnostic content")
	}
}

func TestCompleteProviderDocumentClonesMutableRequestAndRawOutcome(t *testing.T) {
	temperature := 0.7
	command := CompleteProviderDocumentCommand{Request: ProviderRequest{Config: models.RedactedProviderConfig{Capabilities: models.ProviderCapabilities{NonChatOperations: []string{"embeddings"}}, InferenceProfile: models.ProviderInferenceProfile{Temperature: &temperature}}, PromptContext: &models.PromptContext{Sections: []models.PromptSection{{SourceRefs: []string{"original"}}}}}}
	before := command.Clone()
	clone := command.Clone()
	clone.Request.Config.Capabilities.NonChatOperations[0] = "changed"
	*clone.Request.Config.InferenceProfile.Temperature = 0.1
	clone.Request.PromptContext.Sections[0].SourceRefs[0] = "changed"
	if !reflect.DeepEqual(before, command) {
		t.Fatal("request clone aliases original")
	}
	observation := models.NewProviderObservedContent([]byte("raw"), true, "")
	response := CompleteProviderDocumentResult{Wire: observation, ObservedDocument: observation.Clone(), Failure: &ProviderDocumentFailure{Code: "request_failed", Cause: []byte("safe category")}, TransportCause: &observation}
	beforeResponse := response.Clone()
	copy := response.Clone()
	copy.Wire.Content[0] = 'x'
	copy.ObservedDocument.Content[0] = 'x'
	copy.Failure.Cause[0] = 'x'
	copy.TransportCause.Content[0] = 'x'
	if !reflect.DeepEqual(beforeResponse, response) {
		t.Fatal("raw diagnostic clone aliases original")
	}
}
