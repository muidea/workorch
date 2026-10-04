package biz

import (
	"errors"
	"testing"

	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
)

func TestAuthoringParserDiagnosticRetainsWireWithoutProviderFailure(t *testing.T) {
	payload := []byte("\xef\xbb\xbf{\"unknown\":true}\r\n")
	response := llmevents.CompleteProviderDocumentResult{
		ObservedDocument: llmmodels.NewProviderObservedContent(payload, true, ""),
		Wire:             llmmodels.NewProviderObservedContent([]byte("data: 原始 wire\r\n"), true, ""),
	}
	diagnostic := authoringParserDiagnostic(errors.New("strict parser rejected unknown field"), payload, response)
	if err := diagnostic.Validate(); err != nil || diagnostic.ProviderCode != "" {
		t.Fatalf("parser 失败被归为 provider 失败: %v", err)
	}
	for _, mutate := range []func(*workspace.DefinitionAuthoringDiagnostic){
		func(d *workspace.DefinitionAuthoringDiagnostic) { d.ProviderWire = nil },
		func(d *workspace.DefinitionAuthoringDiagnostic) {
			d.ProviderDocument.Complete = false
			d.ProviderDocument.Reason = "partial"
		},
		func(d *workspace.DefinitionAuthoringDiagnostic) {
			observation := authoringProviderObservation(llmmodels.NewProviderObservedContent([]byte("different valid bytes"), true, ""))
			d.ProviderDocument = &observation
		},
	} {
		changed := diagnostic.Clone()
		mutate(&changed)
		if changed.Validate() == nil {
			t.Fatal("不完整或与 payload 不同的 parser 诊断被接受")
		}
	}
	legacy := workspace.NewDefinitionAuthoringDiagnostic("legacy parser error", payload)
	if err := legacy.Validate(); err != nil {
		t.Fatalf("旧无 provider 观察诊断失去兼容性: %v", err)
	}
}

func authoringTestDocumentResult(t *testing.T, command llmevents.CompleteProviderDocumentCommand, payload []byte) llmevents.CompleteProviderDocumentResult {
	t.Helper()
	hash, err := command.RequestHash()
	if err != nil {
		t.Fatal(err)
	}
	observation := llmmodels.NewProviderObservedContent(payload, true, "")
	result := llmevents.CompleteProviderDocumentResult{
		RequestIdentity: command.RequestIdentity, RequestHash: hash, Target: command.Target,
		ConfigurationRevision: command.ExpectedConfigurationRevision,
		Document:              &llmevents.ProviderCompleteDocument{Content: observation.Clone()}, ObservedDocument: observation,
		Wire:              llmmodels.ProviderObservedContent{Reason: "synthetic_provider_no_wire"},
		ProviderSubmitted: true, PhysicalResponseMode: llmaudit.PhysicalResponseModeNonStreaming,
	}
	if err := result.ValidateFor(command); err != nil {
		t.Fatal(err)
	}
	return result
}
