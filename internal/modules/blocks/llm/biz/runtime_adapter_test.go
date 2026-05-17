package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	webcommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type staticProvider struct {
	payload []byte
	err     error
	request llmmodels.ProviderRequest
	usage   *llmmodels.ProviderUsage
}

func (p *staticProvider) Complete(_ context.Context, request llmmodels.ProviderRequest) (llmmodels.ProviderCompletionResult, error) {
	p.request = request
	return llmmodels.ProviderCompletionResult{
		Payload: p.payload,
		Usage:   p.usage,
	}, p.err
}

func promptContextText(context *llmmodels.PromptContext) string {
	if context == nil {
		return ""
	}
	var joined strings.Builder
	for _, turn := range context.Turns {
		joined.WriteString(turn.Content)
		joined.WriteByte('\n')
	}
	return joined.String()
}

type sequenceProvider struct {
	payloads [][]byte
	requests []llmmodels.ProviderRequest
}

func (p *sequenceProvider) Complete(_ context.Context, request llmmodels.ProviderRequest) (llmmodels.ProviderCompletionResult, error) {
	p.requests = append(p.requests, request)
	if len(p.payloads) == 0 {
		return llmmodels.ProviderCompletionResult{}, common.ErrInvalidArgument
	}
	payload := p.payloads[0]
	p.payloads = p.payloads[1:]
	return llmmodels.ProviderCompletionResult{Payload: payload}, nil
}

type failoverProvider struct {
	payload  []byte
	err      error
	requests []llmmodels.ProviderRequest
	usage    *llmmodels.ProviderUsage
}

func (p *failoverProvider) Complete(_ context.Context, request llmmodels.ProviderRequest) (llmmodels.ProviderCompletionResult, error) {
	p.requests = append(p.requests, request)
	return llmmodels.ProviderCompletionResult{
		Payload: p.payload,
		Usage:   p.usage,
	}, p.err
}

type streamingProvider struct {
	payload []byte
	events  []llmmodels.ProviderStreamEvent
	usage   *llmmodels.ProviderUsage
	err     error
}

func (p *streamingProvider) Complete(context.Context, llmmodels.ProviderRequest) (llmmodels.ProviderCompletionResult, error) {
	return llmmodels.ProviderCompletionResult{}, common.ErrInvalidArgument
}

func (p *streamingProvider) StreamComplete(_ context.Context, _ llmmodels.ProviderRequest, emit func(llmmodels.ProviderStreamEvent) error) (llmmodels.ProviderCompletionResult, error) {
	for _, event := range p.events {
		if err := emit(event); err != nil {
			return llmmodels.ProviderCompletionResult{}, err
		}
	}
	return llmmodels.ProviderCompletionResult{Payload: p.payload, Usage: p.usage}, p.err
}

type embeddingStaticProvider struct {
	request llmmodels.ProviderEmbeddingRequest
	result  llmmodels.ProviderEmbeddingResult
	err     error
}

func (p *embeddingStaticProvider) Complete(context.Context, llmmodels.ProviderRequest) (llmmodels.ProviderCompletionResult, error) {
	return llmmodels.ProviderCompletionResult{}, common.ErrInvalidArgument
}

func (p *embeddingStaticProvider) Embed(_ context.Context, request llmmodels.ProviderEmbeddingRequest) (llmmodels.ProviderEmbeddingResult, error) {
	p.request = request
	return p.result, p.err
}

func TestRuntimeAdapterFailsClearlyWhenProviderUnavailable(t *testing.T) {
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		Unavailable:       true,
		UnavailableReason: "llm provider is not configured",
	}, nil)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest())
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("execute err = %v, want invalid argument", err)
	}
	if result.Output["status"] != "failed" || result.Output["stage"] != "llm_provider_unavailable" {
		t.Fatalf("output = %+v, want provider unavailable failure", result.Output)
	}
	if result.Output["failureCategory"] != "llm_provider" || result.Output["resolutionHint"] == "" {
		t.Fatalf("output = %+v, want provider diagnostics", result.Output)
	}
}

func TestRuntimeAdapterProjectsContextBudgetToStepOutput(t *testing.T) {
	provider := &staticProvider{payload: []byte(`{
		"completionState":"continue",
		"shellCommands":[{"command":"echo","args":["ok"]}]
	}`)}
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		ID:       "reasoning",
		Provider: llmcommon.ProviderCompatible,
		APIKey:   "secret-value",
		Model:    "test-model",
		ContextProfile: llmmodels.ProviderContextProfile{
			ContextWindowTokens:    4096,
			MaxInputTokens:         512,
			ReservedOutputTokens:   512,
			SafetyRatio:            0.8,
			BytesHardLimit:         64 * 1024,
			EstimatedTokensPerByte: 0.25,
			InputTokenPrice:        0.000001,
			OutputTokenPrice:       0.000002,
			Currency:               "usd",
		},
	}, provider)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest())
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	budget, ok := result.Output["contextBudget"].(map[string]any)
	if !ok {
		t.Fatalf("contextBudget = %#v, want projected map", result.Output["contextBudget"])
	}
	if budget["providerId"] != nil || budget["model"] != nil || budget["profile"] != nil {
		t.Fatalf("contextBudget = %+v, want no persisted provider/model/profile binding", budget)
	}
	if budget["maxBytes"] == nil || budget["actualBytes"] == nil || budget["truncated"] == nil {
		t.Fatalf("contextBudget = %+v, want required budget fields", budget)
	}
	usage, ok := result.Output["llmUsage"].(map[string]any)
	if !ok {
		t.Fatalf("llmUsage = %#v, want usage metadata", result.Output["llmUsage"])
	}
	if usage["providerId"] != nil || usage["provider"] != nil || usage["model"] != nil || usage["estimated"] != true {
		t.Fatalf("llmUsage = %+v, want usage without persisted provider/model binding", usage)
	}
	if usage["usageSource"] != llmcommon.UsageSourceEstimated {
		t.Fatalf("llmUsage = %+v, want estimated usage source", usage)
	}
	outputTokens, ok := usage["outputTokens"].(int)
	if !ok {
		t.Fatalf("llmUsage = %+v, want integer outputTokens", usage)
	}
	expectedOutputTokens := estimateUsageTokensFromBytes(len(provider.payload), 0.25)
	if outputTokens != expectedOutputTokens {
		t.Fatalf("llmUsage = %+v, want outputTokens estimated from response bytes %d", usage, expectedOutputTokens)
	}
	inputTokens, ok := usage["inputTokens"].(int)
	if !ok || inputTokens <= 0 {
		t.Fatalf("llmUsage = %+v, want positive integer inputTokens", usage)
	}
	totalTokens, ok := usage["totalTokens"].(int)
	if !ok || totalTokens != inputTokens+outputTokens {
		t.Fatalf("llmUsage = %+v, want totalTokens=inputTokens+outputTokens", usage)
	}
	if usage["estimatedCost"] == nil || usage["currency"] != "USD" || usage["costEstimated"] != true || usage["costSource"] != llmcommon.CostSourceProfilePricing {
		t.Fatalf("llmUsage = %+v, want estimated cost with currency", usage)
	}
}

func TestRuntimeAdapterUsesNativeProviderUsageWhenAvailable(t *testing.T) {
	provider := &staticProvider{
		payload: []byte(`{
			"completionState":"continue",
			"shellCommands":[{"command":"echo","args":["ok"]}]
		}`),
		usage: &llmmodels.ProviderUsage{
			InputTokens:  11,
			OutputTokens: 7,
			TotalTokens:  18,
		},
	}
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		ID:       "reasoning",
		Provider: llmcommon.ProviderCompatible,
		APIKey:   "secret-value",
		Model:    "test-model",
		ContextProfile: llmmodels.ProviderContextProfile{
			EstimatedTokensPerByte: 0.25,
		},
	}, provider)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest())
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	usage, ok := result.Output["llmUsage"].(map[string]any)
	if !ok {
		t.Fatalf("llmUsage = %#v, want usage metadata", result.Output["llmUsage"])
	}
	if usage["estimated"] != false ||
		usage["inputTokens"] != 11 ||
		usage["outputTokens"] != 7 ||
		usage["totalTokens"] != 18 {
		t.Fatalf("llmUsage = %+v, want native usage with estimated=false", usage)
	}
	if usage["usageSource"] != llmcommon.UsageSourceProviderNative {
		t.Fatalf("llmUsage = %+v, want provider native usage source", usage)
	}
}

func TestRuntimeAdapterUsesNativeProviderUsageForStreamingWhenAvailable(t *testing.T) {
	provider := &streamingProvider{
		events: []llmmodels.ProviderStreamEvent{
			{Type: "delta", Delta: `{"completionState":"continue"`},
			{Type: "delta", Delta: `,"shellCommands":[{"command":"echo","args":["ok"]}]}`},
			{Type: "done"},
		},
		payload: []byte(`{"completionState":"continue","shellCommands":[{"command":"echo","args":["ok"]}]}`),
		usage: &llmmodels.ProviderUsage{
			InputTokens:  23,
			OutputTokens: 9,
			TotalTokens:  32,
		},
	}
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		ID:       "streaming-provider",
		Provider: llmcommon.ProviderCompatible,
		APIKey:   "secret-value",
		Model:    "test-model",
		Capabilities: llmmodels.ProviderCapabilities{
			ChatCompletions: true,
			Streaming:       true,
		},
	}, provider)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest())
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	usage, ok := result.Output["llmUsage"].(map[string]any)
	if !ok {
		t.Fatalf("llmUsage = %#v, want usage metadata", result.Output["llmUsage"])
	}
	if usage["estimated"] != false ||
		usage["inputTokens"] != 23 ||
		usage["outputTokens"] != 9 ||
		usage["totalTokens"] != 32 ||
		usage["streaming"] != true {
		t.Fatalf("llmUsage = %+v, want native streaming usage with estimated=false", usage)
	}
	if usage["usageSource"] != llmcommon.UsageSourceProviderNative {
		t.Fatalf("llmUsage = %+v, want provider native usage source", usage)
	}
}

func TestRuntimeAdapterRepairsInvalidDecisionOnce(t *testing.T) {
	provider := &sequenceProvider{payloads: [][]byte{
		[]byte(`{
			"workspaceId":"ws-1",
			"thoughtSummary":"need product direction",
			"completionState":"blocked_on_decision",
			"requiredDecision":"missing implementation path"
		}`),
		[]byte(`{
			"thoughtSummary":"need product direction",
			"completionState":"blocked_on_decision",
			"requiredDecision":{
				"reason":"missing implementation path",
				"decisionKind":"provide_missing_info",
				"question":"Should the run use the shell sandbox first?"
			}
		}`),
	}}
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		APIKey:   "secret-value",
		Model:    "test-model",
	}, provider)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest())
	if err != nil {
		t.Fatalf("execute runtime step: %v", err)
	}
	if len(provider.requests) != 2 || provider.requests[1].Repair == nil {
		t.Fatalf("requests = %+v, want one repair retry", provider.requests)
	}
	if provider.requests[1].Repair.Attempt != 1 ||
		provider.requests[1].Repair.ParseError == "" ||
		provider.requests[1].Repair.OriginalOutput == "" {
		t.Fatalf("repair request = %+v, want parse context", provider.requests[1].Repair)
	}
	if result.Output["stage"] != "llm_decision_required" ||
		result.Output["blockDecisionKind"] != "provide_missing_info" {
		t.Fatalf("output = %+v, want repaired decision", result.Output)
	}
	repair, ok := result.Output["llmRepair"].(map[string]any)
	if !ok || repair["attempted"] != true || repair["parseError"] == "" {
		t.Fatalf("llmRepair = %+v, want successful repair metadata", result.Output["llmRepair"])
	}
}

func TestRuntimeAdapterRepairsUnavailableNestedShellCommand(t *testing.T) {
	unavailableCommand := "sshpass"
	commandUnavailable := false
	for _, command := range unavailableRuntimeCommands() {
		if command == unavailableCommand {
			commandUnavailable = true
			break
		}
	}
	if !commandUnavailable {
		unavailableCommand = unavailableWorkorchCLICommand
	}

	provider := &sequenceProvider{payloads: [][]byte{
		[]byte(`{
			"thoughtSummary":"inspect remote host through shell",
			"completionState":"continue",
			"executionGraph":{"nodes":[
				{"id":"remote_inspect","kind":"shell/command","input":{"command":"sh","args":["-c","` + unavailableCommand + ` -p secret ssh host uname -a"]}}
			]}
		}`),
		[]byte(`{
			"thoughtSummary":"runtime command is unavailable",
			"completionState":"blocked_on_decision",
			"requiredDecision":{
				"reason":"requested remote access depends on a command that is unavailable in this runtime",
				"decisionKind":"provide_missing_info",
				"question":"Please provide a supported non-interactive access method such as key-based SSH, an installed helper, MCP, or an external runtime adapter."
			}
		}`),
	}}
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		APIKey:   "secret-value",
		Model:    "test-model",
	}, provider)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest())
	if err != nil {
		t.Fatalf("execute runtime step: %v", err)
	}
	if len(provider.requests) != 2 || provider.requests[1].Repair == nil {
		t.Fatalf("requests = %+v, want repair after unavailable nested shell command", provider.requests)
	}
	if !strings.Contains(provider.requests[1].Repair.ParseError, "unavailable runtime command") ||
		!strings.Contains(provider.requests[1].Repair.ParseError, unavailableCommand) {
		t.Fatalf("repair parse error = %q, want unavailable command context", provider.requests[1].Repair.ParseError)
	}
	if result.Output["stage"] != "llm_decision_required" ||
		result.Output["blockDecisionKind"] != "provide_missing_info" {
		t.Fatalf("output = %+v, want repaired decision ticket", result.Output)
	}
}

func TestRuntimeAdapterReportsRepairFailure(t *testing.T) {
	provider := &sequenceProvider{payloads: [][]byte{
		[]byte(`{"workspaceId":"ws-1","completionState":"continue"}`),
		[]byte(`{"completionState":"unknown"}`),
	}}
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		APIKey:   "secret-value",
		Model:    "test-model",
	}, provider)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest())
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("execute err = %v, want invalid argument", err)
	}
	if result.Output["stage"] != "llm_schema_invalid" ||
		result.Output["repairAttempted"] != true ||
		result.Output["parseError"] == "" ||
		result.Output["repairError"] == "" ||
		result.Output["rawContentPreview"] == "" ||
		result.Output["repairRawContentPreview"] == "" {
		t.Fatalf("output = %+v, want repair failure diagnostics", result.Output)
	}
	if len(provider.requests) != 2 {
		t.Fatalf("provider calls = %d, want original plus one repair", len(provider.requests))
	}
}

func TestRuntimeAdapterParsesDecisionAndReturnsNormalizedOutput(t *testing.T) {
	provider := &staticProvider{payload: []byte(`{
		"thoughtSummary":"need to validate with go test",
		"completionState":"continue",
		"shellCommands":[{"command":"go","args":["test","./..."]}]
	}`)}
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		APIKey:   "secret-value",
		Model:    "test-model",
	}, provider)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest())
	if err != nil {
		t.Fatalf("execute runtime step: %v", err)
	}
	if provider.request.Schema == "" || provider.request.Config.Ready != true {
		t.Fatalf("provider request = %+v, want schema and ready redacted config", provider.request)
	}
	if !runtimeCapabilitiesContainBlock(provider.request.RuntimeCapabilities, llmcommon.StepKindWorkspaceExecution) ||
		!runtimeCapabilitiesContainBlock(provider.request.RuntimeCapabilities, "file/list") ||
		!runtimeCapabilitiesContainBlock(provider.request.RuntimeCapabilities, "file/read") ||
		!runtimeCapabilitiesContainBlock(provider.request.RuntimeCapabilities, "file/edit") ||
		!runtimeCapabilitiesContainBlock(provider.request.RuntimeCapabilities, "shell/command") ||
		!runtimeCapabilitiesContainBlock(provider.request.RuntimeCapabilities, "ssh/command") ||
		!runtimeCapabilitiesContainBlock(provider.request.RuntimeCapabilities, webcommon.StepKindSearch) ||
		!runtimeCapabilitiesContainBlock(provider.request.RuntimeCapabilities, webcommon.StepKindFetch) ||
		!runtimeCapabilitiesContainBlock(provider.request.RuntimeCapabilities, llmcommon.RuntimeBlockKindPatchPreview) ||
		!runtimeCapabilitiesContainBlock(provider.request.RuntimeCapabilities, llmcommon.RuntimeBlockKindPatchApply) ||
		!runtimeCapabilitiesContainBlock(provider.request.RuntimeCapabilities, llmcommon.RuntimeBlockKindPatchRevert) ||
		!runtimeCapabilitiesContainServiceAPI(provider.request.RuntimeCapabilities, "GET", "/api/v1/workorch/runs/{runId}/snapshot") {
		t.Fatalf("runtime capabilities = %+v, want declared block and service API catalog", provider.request.RuntimeCapabilities)
	}
	if description := runtimeCapabilityBlockDescription(provider.request.RuntimeCapabilities, llmcommon.RuntimeBlockKindPatchPreview); !strings.Contains(description, "complete, concrete, and directly applicable") {
		t.Fatalf("patch/preview description = %q, want concrete patch guidance", description)
	}
	if description := runtimeCapabilityBlockDescription(provider.request.RuntimeCapabilities, llmcommon.RuntimeBlockKindPatchApply); !strings.Contains(description, "complete, concrete, and directly applicable") {
		t.Fatalf("patch/apply description = %q, want concrete patch guidance", description)
	}
	for _, kind := range []string{
		llmcommon.RuntimeBlockKindDecisionRequest,
		llmcommon.RuntimeBlockKindApprovalRequest,
		llmcommon.RuntimeBlockKindMemoryWrite,
		llmcommon.RuntimeBlockKindDoDAssessment,
	} {
		if runtimeCapabilitiesContainBlock(provider.request.RuntimeCapabilities, kind) {
			t.Fatalf("runtime capabilities = %+v, pseudo output kind %s must not be advertised as executable graph block", provider.request.RuntimeCapabilities, kind)
		}
	}
	if result.Output["status"] != "completed" ||
		result.Output["stage"] != "llm_shell_plan" ||
		result.Output["nextKind"] != "shell/command" {
		t.Fatalf("output = %+v, want shell plan", result.Output)
	}
}

func TestRuntimeAdapterHidesExecutableCapabilitiesForAssessmentOnlyRequest(t *testing.T) {
	provider := &staticProvider{payload: []byte(`{
		"completionState":"failed",
		"dodAssessment":{
			"satisfied":false,
			"summary":"current evidence is insufficient",
			"goalOutcome":"incomplete",
			"items":[{"item":"done","state":"failed","reason":"missing evidence"}]
		}
	}`)}
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		APIKey:   "secret-value",
		Model:    "test-model",
	}, provider)
	request := llmStepRequest()
	request.Input["assessmentOnly"] = true

	result, err := adapter.ExecuteRuntimeStep(context.Background(), request)
	if err != nil {
		t.Fatalf("execute runtime step: %v", err)
	}
	if len(provider.request.RuntimeCapabilities.BlockKinds) != 0 || len(provider.request.RuntimeCapabilities.ServiceAPIs) != 0 {
		t.Fatalf("runtime capabilities = %+v, assessment-only request must hide executable capabilities", provider.request.RuntimeCapabilities)
	}
	if !provider.request.RuntimeCapabilities.ShellPolicy.Enabled {
		t.Fatalf("runtime capabilities = %+v, want shell policy evidence retained", provider.request.RuntimeCapabilities)
	}
	if result.Output["stage"] != "llm_dod_assessment" {
		t.Fatalf("output = %+v, want DoD assessment output", result.Output)
	}
}

func TestRuntimeAdapterToleratesPatchOperationProviderDrift(t *testing.T) {
	provider := &staticProvider{payload: []byte(`{
		"completionState":"continue",
		"patchRequests":[{
			"operation":"create",
			"targetRef":"workorch-runtime-output.txt",
			"content":"run=run-patch-drift",
			"baseHash":"abc123"
		}]
	}`)}
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		APIKey:   "secret-value",
		Model:    "test-model",
	}, provider)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest())
	if err != nil {
		t.Fatalf("execute runtime step: %v", err)
	}
	patches, ok := result.Output["patches"].([]map[string]any)
	if !ok || len(patches) != 1 {
		t.Fatalf("patches = %#v, want one normalized patch request", result.Output["patches"])
	}
	if patches[0]["targetRef"] != "workorch-runtime-output.txt" || patches[0]["content"] != "run=run-patch-drift" {
		t.Fatalf("patch = %+v, want normalized target/content", patches[0])
	}
	if patches[0]["patchType"] != "file_write" {
		t.Fatalf("patch = %+v, want operation drift normalized to file_write", patches[0])
	}
	if patches[0]["baseHash"] != "abc123" {
		t.Fatalf("patch = %+v, want baseHash propagated", patches[0])
	}
	if _, exists := patches[0]["operation"]; exists {
		t.Fatalf("patch = %+v, operation is provider drift and must not leak to runtime output", patches[0])
	}
	decision, ok := result.Output["llmDecision"].(map[string]any)
	if !ok {
		t.Fatalf("llmDecision = %#v, want normalized map", result.Output["llmDecision"])
	}
	decisionPatches, ok := decision["patchRequests"].([]any)
	if !ok || len(decisionPatches) != 1 {
		t.Fatalf("llmDecision.patchRequests = %#v, want one normalized patch", decision["patchRequests"])
	}
	decisionPatch, ok := decisionPatches[0].(map[string]any)
	if !ok || decisionPatch["patchType"] != "file_write" {
		t.Fatalf("llmDecision.patchRequests[0] = %#v, want file_write", decisionPatches[0])
	}
	if _, exists := decisionPatch["operation"]; exists {
		t.Fatalf("llmDecision.patchRequests[0] = %+v, operation must not leak", decisionPatch)
	}
}

func TestRuntimeAdapterNormalizesPatchTypeCreateProviderDrift(t *testing.T) {
	provider := &staticProvider{payload: []byte(`{
		"completionState":"continue",
		"patchRequests":[{
			"patchType":"create",
			"targetRef":"workorch-runtime-output.txt",
			"content":"run=run-patch-create-drift"
		}]
	}`)}
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		APIKey:   "secret-value",
		Model:    "test-model",
	}, provider)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest())
	if err != nil {
		t.Fatalf("execute runtime step: %v", err)
	}
	patches, ok := result.Output["patches"].([]map[string]any)
	if !ok || len(patches) != 1 {
		t.Fatalf("patches = %#v, want one normalized patch request", result.Output["patches"])
	}
	if patches[0]["patchType"] != "file_write" || patches[0]["targetRef"] != "workorch-runtime-output.txt" {
		t.Fatalf("patch = %+v, want create normalized to file_write", patches[0])
	}
	decision := result.Output["llmDecision"].(map[string]any)
	decisionPatch := decision["patchRequests"].([]any)[0].(map[string]any)
	if decisionPatch["patchType"] != "file_write" {
		t.Fatalf("llmDecision.patchRequests[0] = %+v, want create normalized to file_write", decisionPatch)
	}
}

func TestRuntimeAdapterRejectsReservedGovernanceReportPatchTarget(t *testing.T) {
	provider := &staticProvider{payload: []byte(`{
		"completionState":"continue",
		"patchRequests":[{
			"patchType":"file_write",
			"targetRef":"decision_record.md",
			"content":"decision"
		}]
	}`)}
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		APIKey:   "secret-value",
		Model:    "test-model",
	}, provider)

	_, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest())
	if err == nil || !strings.Contains(err.Error(), "reserved for runtime governance/report artifacts") {
		t.Fatalf("ExecuteRuntimeStep err = %v, want reserved patch target error", err)
	}
}

func TestRuntimeAdapterUsesStreamingProviderWhenCapabilityDeclared(t *testing.T) {
	provider := &streamingProvider{
		payload: []byte(`{"completionState":"continue","shellCommands":[{"command":"echo","args":["stream"]}]}`),
		events: []llmmodels.ProviderStreamEvent{
			{Type: "delta", Delta: `{"completionState":"continue"`},
			{Type: "delta", Delta: `,"shellCommands":[{"command":"echo","args":["stream"]}]}`},
		},
	}
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		APIKey:   "secret-value",
		Model:    "test-model",
		Capabilities: llmmodels.ProviderCapabilities{
			ChatCompletions: true,
			Streaming:       true,
		},
	}, provider)

	request := llmStepRequest()
	request.WorkspaceInternalDirectory = t.TempDir()
	result, err := adapter.ExecuteRuntimeStep(context.Background(), request)
	if err != nil {
		t.Fatalf("execute runtime step: %v", err)
	}
	if result.Output["nextKind"] != "shell/command" {
		t.Fatalf("output = %+v, want shell plan from streamed decision", result.Output)
	}
	if result.Output["streamEvents"] != nil {
		t.Fatalf("streamEvents = %+v, want raw stream omitted from primary output", result.Output["streamEvents"])
	}
	diagnostics, ok := result.Output["streamDiagnostics"].(map[string]any)
	if !ok || diagnostics["receivedEventCount"] != 2 || diagnostics["receivedBytes"] == nil {
		t.Fatalf("streamDiagnostics = %+v, want bounded stream diagnostics", result.Output["streamDiagnostics"])
	}
	refs, ok := result.Output["llmCallRefs"].(map[string]any)
	if !ok || refs["requestRef"] == "" || refs["responseRef"] == "" || refs["streamRef"] == "" || refs["usageRef"] == "" {
		t.Fatalf("llmCallRefs = %+v, want persisted request/response/stream/usage refs", result.Output["llmCallRefs"])
	}
	itemDir := filepath.Join(request.WorkspaceInternalDirectory, "runs", string(request.RunID), "llm-calls", safeLLMCallEvidenceFileName(string(request.StepID)))
	for _, name := range []string{"request.json", "response.json", "response.sse", "usage.json"} {
		if _, err := os.Stat(filepath.Join(itemDir, name)); err != nil {
			t.Fatalf("llm evidence %s stat: %v", name, err)
		}
	}
}

func TestRuntimeAdapterFailsStreamingErrorClearly(t *testing.T) {
	provider := &streamingProvider{
		events: []llmmodels.ProviderStreamEvent{{Type: "delta", Delta: `{"completionState"`}},
		err:    errors.New("stream disconnected"),
	}
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		APIKey:   "secret-value",
		Model:    "test-model",
		Capabilities: llmmodels.ProviderCapabilities{
			ChatCompletions: true,
			Streaming:       true,
		},
	}, provider)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest())
	if err == nil {
		t.Fatal("expected streaming error")
	}
	if result.Output["stage"] != "llm_provider_stream_error" || result.Output["streamEvents"] != nil {
		t.Fatalf("output = %+v, want stream error without raw events", result.Output)
	}
	if diagnostics, ok := result.Output["streamDiagnostics"].(map[string]any); !ok || diagnostics["streamIncomplete"] != true {
		t.Fatalf("streamDiagnostics = %+v, want stream error diagnostics", result.Output["streamDiagnostics"])
	}
	if result.Output["failureCategory"] != "llm_provider" || result.Output["resolutionHint"] == "" {
		t.Fatalf("output = %+v, want provider diagnostics", result.Output)
	}
}

func TestRuntimeAdapterReportsStreamDiagnosticsOnProviderTimeout(t *testing.T) {
	provider := &streamingProvider{
		events: []llmmodels.ProviderStreamEvent{{Type: "delta", Delta: `{"completionState"`}},
		err:    context.DeadlineExceeded,
	}
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		APIKey:   "secret-value",
		Model:    "test-model",
		Capabilities: llmmodels.ProviderCapabilities{
			ChatCompletions: true,
			Streaming:       true,
		},
	}, provider)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest())
	if err == nil {
		t.Fatal("expected provider timeout")
	}
	if result.Output["stage"] != "llm_provider_timeout" {
		t.Fatalf("stage = %v, want llm_provider_timeout", result.Output["stage"])
	}
	diagnostics, ok := result.Output["streamDiagnostics"].(map[string]any)
	if !ok || diagnostics["doneReceived"] != false || diagnostics["streamIncomplete"] != true || diagnostics["receivedDeltaCount"] != 1 {
		t.Fatalf("streamDiagnostics = %+v, want incomplete stream diagnostics", result.Output["streamDiagnostics"])
	}
}

func TestRuntimeAdapterClassifiesProviderStreamIncomplete(t *testing.T) {
	provider := &streamingProvider{
		events: []llmmodels.ProviderStreamEvent{{Type: "delta", Delta: `{"completionState"`}},
		err:    fmt.Errorf("%w: llm provider stream incomplete: ended before [DONE]", common.ErrInvalidArgument),
	}
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		APIKey:   "secret-value",
		Model:    "test-model",
		Capabilities: llmmodels.ProviderCapabilities{
			ChatCompletions: true,
			Streaming:       true,
		},
	}, provider)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest())
	if err == nil {
		t.Fatal("expected stream incomplete error")
	}
	if result.Output["stage"] != "llm_provider_stream_incomplete" {
		t.Fatalf("stage = %v, want llm_provider_stream_incomplete", result.Output["stage"])
	}
	usage, ok := result.Output["llmUsage"].(map[string]any)
	if !ok || usage["failureClass"] != providerFailureClassStreamIncomplete {
		t.Fatalf("llmUsage = %+v, want stream incomplete failure class", result.Output["llmUsage"])
	}
}

func TestRuntimeAdapterRejectsUndeclaredNonChatOperation(t *testing.T) {
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		APIKey:   "secret-value",
		Model:    "test-model",
		Capabilities: llmmodels.ProviderCapabilities{
			ChatCompletions:   true,
			NonChatOperations: []string{"embeddings"},
		},
	}, &staticProvider{payload: []byte(`{"completionState":"continue"}`)})
	request := llmStepRequest()
	request.Input["llmOperation"] = "responses"

	result, err := adapter.ExecuteRuntimeStep(context.Background(), request)
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("execute err = %v, want invalid argument", err)
	}
	if result.Output["stage"] != "llm_provider_operation_unsupported" {
		t.Fatalf("output = %+v, want unsupported operation failure", result.Output)
	}
}

func TestRuntimeAdapterGeneratesEmbeddingForDeclaredProvider(t *testing.T) {
	provider := &embeddingStaticProvider{result: llmmodels.ProviderEmbeddingResult{
		Vector: []float64{0.1, 0.2, 0.3},
	}}
	adapter := NewRuntimeAdapterWithProviders("reasoning", map[string]ProviderRuntime{
		"embedding": {
			Config: llmmodels.ProviderConfig{
				ID:       "embedding",
				Provider: llmcommon.ProviderCompatible,
				APIKey:   "secret-value",
				Model:    "embedding-model",
				Capabilities: llmmodels.ProviderCapabilities{
					NonChatOperations: []string{llmcommon.ProviderOperationEmbeddings},
				},
			},
			Provider: provider,
		},
	})

	result, err := adapter.GenerateEmbedding(context.Background(), "embedding", "workspace goal")
	if err != nil {
		t.Fatalf("generate embedding: %v", err)
	}
	if provider.request.Input != "workspace goal" || provider.request.Model != "embedding-model" {
		t.Fatalf("embedding request = %+v, want input and model", provider.request)
	}
	if result.ProviderID != "embedding" || result.Model != "embedding-model" || len(result.Vector) != 3 {
		t.Fatalf("embedding result = %+v, want selected provider vector", result)
	}
}

func TestRuntimeAdapterRejectsEmbeddingWhenCapabilityMissing(t *testing.T) {
	adapter := NewRuntimeAdapterWithProviders("embedding", map[string]ProviderRuntime{
		"embedding": {
			Config: llmmodels.ProviderConfig{
				ID:       "embedding",
				Provider: llmcommon.ProviderCompatible,
				APIKey:   "secret-value",
				Model:    "embedding-model",
				Capabilities: llmmodels.ProviderCapabilities{
					ChatCompletions: true,
				},
			},
			Provider: &embeddingStaticProvider{result: llmmodels.ProviderEmbeddingResult{Vector: []float64{0.1}}},
		},
	})

	_, err := adapter.GenerateEmbedding(context.Background(), "embedding", "workspace goal")
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("generate embedding err = %v, want invalid argument", err)
	}
}

func TestRuntimeAdapterSelectsRequestedProvider(t *testing.T) {
	localProvider := &staticProvider{payload: []byte(`{"completionState":"continue"}`)}
	reasoningProvider := &staticProvider{payload: []byte(`{
		"completionState":"continue",
		"shellCommands":[{"command":"go","args":["test","./..."]}]
	}`)}
	adapter := NewRuntimeAdapterWithProviders("local", map[string]ProviderRuntime{
		"local": {
			Config: llmmodels.ProviderConfig{
				ID:       "local",
				Provider: llmcommon.ProviderCompatible,
				APIKey:   "local-secret",
				Model:    "local-model",
			},
			Provider: localProvider,
		},
		"reasoning": {
			Config: llmmodels.ProviderConfig{
				ID:       "reasoning",
				Provider: llmcommon.ProviderOpenAI,
				APIKey:   "reasoning-secret",
				Model:    "reasoning-model",
			},
			Provider: reasoningProvider,
		},
	})
	request := llmStepRequest()
	request.Input["executionContext"] = map[string]any{
		llmcommon.ExecutionContextLLMProviderID: "reasoning",
	}

	result, err := adapter.ExecuteRuntimeStep(context.Background(), request)
	if err != nil {
		t.Fatalf("execute runtime step: %v", err)
	}
	if localProvider.request.Model != "" {
		t.Fatalf("local provider was called: %+v", localProvider.request)
	}
	if reasoningProvider.request.Model != "reasoning-model" || result.Output["nextKind"] != "shell/command" {
		t.Fatalf("reasoning request=%+v output=%+v, want selected reasoning provider", reasoningProvider.request, result.Output)
	}
}

func TestRuntimeAdapterInjectsDialogLanguageFromSystemLocale(t *testing.T) {
	t.Setenv("LC_ALL", "zh_CN.UTF-8")
	provider := &staticProvider{payload: []byte(`{"completionState":"continue"}`)}
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		APIKey:   "secret-value",
		Model:    "test-model",
	}, provider)

	if _, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest()); err != nil {
		t.Fatalf("execute runtime step: %v", err)
	}
	if provider.request.SystemLocale != "zh_CN.UTF-8" {
		t.Fatalf("systemLocale = %q, want zh_CN.UTF-8", provider.request.SystemLocale)
	}
	if provider.request.DialogLanguage != "Chinese (Simplified)" {
		t.Fatalf("dialogLanguage = %q, want Chinese (Simplified)", provider.request.DialogLanguage)
	}
}

func TestRuntimeAdapterDialogLanguageContextOverridesSystemLocale(t *testing.T) {
	t.Setenv("LC_ALL", "en_US.UTF-8")
	provider := &staticProvider{payload: []byte(`{"completionState":"continue"}`)}
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		APIKey:   "secret-value",
		Model:    "test-model",
	}, provider)
	request := llmStepRequest()
	request.Input["executionContext"] = map[string]any{
		"dialogLanguage": "zh-CN",
	}

	if _, err := adapter.ExecuteRuntimeStep(context.Background(), request); err != nil {
		t.Fatalf("execute runtime step: %v", err)
	}
	if provider.request.DialogLanguage != "Chinese (Simplified)" {
		t.Fatalf("dialogLanguage = %q, want Chinese (Simplified)", provider.request.DialogLanguage)
	}
}

func TestRuntimeAdapterInjectsCurrentSoulFromRoutingIntoProviderRequest(t *testing.T) {
	provider := &staticProvider{payload: []byte(`{"completionState":"continue"}`)}
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		APIKey:   "secret-value",
		Model:    "test-model",
	}, provider)
	request := llmStepRequest()
	request.Input["decisionResolution"] = map[string]any{
		"resolutionType": "provide_missing_info",
	}
	request.Input["executionContext"] = map[string]any{
		"soulRouting": map[string]any{
			"bindings": map[string]any{
				"decision": map[string]any{
					"soulId": "soul-decision",
					"source": "workspace_stage_binding",
					"soul": map[string]any{
						"id":   "soul-decision",
						"name": "Decision Soul",
					},
				},
			},
		},
	}

	if _, err := adapter.ExecuteRuntimeStep(context.Background(), request); err != nil {
		t.Fatalf("execute runtime step: %v", err)
	}
	if provider.request.Input["currentSoulStage"] != "decision" {
		t.Fatalf("currentSoulStage = %v, want decision", provider.request.Input["currentSoulStage"])
	}
	binding := provider.request.Input["currentSoulBinding"].(map[string]any)
	if binding["soulId"] != "soul-decision" || binding["source"] != "workspace_stage_binding" {
		t.Fatalf("currentSoulBinding = %+v, want decision soul binding", binding)
	}
	soul := provider.request.Input["currentSoul"].(map[string]any)
	if soul["id"] != "soul-decision" || soul["name"] != "Decision Soul" {
		t.Fatalf("currentSoul = %+v, want decision soul context", soul)
	}
}

func TestRuntimeAdapterAssessmentOnlyPromptProjectsDecisionAndExecutionSoulPolicies(t *testing.T) {
	provider := &staticProvider{payload: []byte(`{
		"completionState":"failed",
		"dodAssessment":{
			"satisfied":false,
			"summary":"Current evidence is insufficient.",
			"goalOutcome":"The goal is incomplete.",
			"items":[{"item":"project exists","state":"failed","reason":"workspace scan is empty"}],
			"nextActions":["Initialize the minimal project in a repair step."]
		}
	}`)}
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		APIKey:   "secret-value",
		Model:    "test-model",
	}, provider)
	request := llmStepRequest()
	request.Input["runtimeStage"] = "verification"
	request.Input["runtimeStageMode"] = "strict_convergence"
	request.Input["soulStage"] = "validation"
	request.Input["providerRetryAssessmentOnly"] = true
	request.Input["providerRetryMode"] = "completion_assessment_only"
	request.Input["executionContext"] = map[string]any{
		"soulRouting": map[string]any{
			"bindings": map[string]any{
				"validation": map[string]any{
					"soulId": "soul-validation",
					"source": "workspace_stage_binding",
					"soul": map[string]any{
						"id":     "soul-validation",
						"name":   "Validation Soul",
						"soulMd": "# Validation\n\nAssess every DoD item before completion.",
					},
				},
				"decision": map[string]any{
					"soulId": "soul-decision",
					"source": "workspace_stage_binding",
					"soul": map[string]any{
						"id":     "soul-decision",
						"name":   "Decision Soul",
						"soulMd": "# Decision\n\nDo not ask for source code when an empty workspace can be initialized as a new project.",
					},
				},
				"execution": map[string]any{
					"soulId": "soul-execution",
					"source": "workspace_stage_binding",
					"soul": map[string]any{
						"id":     "soul-execution",
						"name":   "Execution Soul",
						"soulMd": "# Execution\n\nWhen the workspace is empty and existing source is not required, initialize a minimal verifiable project.",
					},
				},
			},
		},
	}

	if _, err := adapter.ExecuteRuntimeStep(context.Background(), request); err != nil {
		t.Fatalf("execute runtime step: %v", err)
	}
	if provider.request.Input["currentSoulStage"] != "validation" {
		t.Fatalf("currentSoulStage = %v, want validation", provider.request.Input["currentSoulStage"])
	}
	if provider.request.Input["currentRuntimeStage"] != "verification" {
		t.Fatalf("currentRuntimeStage = %v, want verification", provider.request.Input["currentRuntimeStage"])
	}
	if provider.request.Input["currentRuntimeStageMode"] != "strict_convergence" {
		t.Fatalf("currentRuntimeStageMode = %v, want strict_convergence", provider.request.Input["currentRuntimeStageMode"])
	}
	joined := promptContextText(provider.request.PromptContext)
	for _, want := range []string{
		"Governance and Soul Policy View",
		"runtimeStage",
		"runtimeStageMode",
		"Before returning blocked_on_decision",
		"later runtime repair or replan",
		"Do not ask for source code when an empty workspace can be initialized as a new project",
		"When the workspace is empty and existing source is not required",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("prompt context missing %q:\n%s", want, joined)
		}
	}
}

func TestRuntimeAdapterUsesExplicitCurrentSoulStageFromStepInput(t *testing.T) {
	provider := &staticProvider{payload: []byte(`{"completionState":"continue"}`)}
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		APIKey:   "secret-value",
		Model:    "test-model",
	}, provider)
	request := llmStepRequest()
	request.Input["currentSoulStage"] = "artifact"
	request.Input["executionContext"] = map[string]any{
		"soulRouting": map[string]any{
			"bindings": map[string]any{
				"artifact": map[string]any{
					"soulId": "soul-writer",
					"source": "workspace_stage_binding",
					"soul": map[string]any{
						"id":   "soul-writer",
						"name": "Artifact Writer",
					},
				},
			},
		},
	}

	if _, err := adapter.ExecuteRuntimeStep(context.Background(), request); err != nil {
		t.Fatalf("execute runtime step: %v", err)
	}
	if provider.request.Input["currentSoulStage"] != "artifact" {
		t.Fatalf("currentSoulStage = %v, want artifact", provider.request.Input["currentSoulStage"])
	}
	soul := provider.request.Input["currentSoul"].(map[string]any)
	if soul["id"] != "soul-writer" || soul["name"] != "Artifact Writer" {
		t.Fatalf("currentSoul = %+v, want artifact writer soul context", soul)
	}
}

func TestRuntimeAdapterBuildsPromptContextFromGovernanceAndGoalDoDHistory(t *testing.T) {
	provider := &staticProvider{payload: []byte(`{"completionState":"continue"}`)}
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		APIKey:   "secret-value",
		Model:    "test-model",
	}, provider)
	request := llmStepRequest()
	request.Input["decisionResolution"] = map[string]any{
		"resolutionType": "provide_missing_info",
		"resolutionPayload": map[string]any{
			"city": "Nanjing",
		},
	}
	request.Input["approvalResolution"] = map[string]any{
		"action": "approve",
	}
	request.Input["executionContext"] = map[string]any{
		"goals": []any{
			map[string]any{"text": "goal one"},
			map[string]any{"text": "goal two"},
		},
		"dods": []any{
			map[string]any{"items": []any{"dod one"}},
			map[string]any{"items": []any{"dod two"}},
		},
		"governanceHistory": []any{
			map[string]any{"id": "ticket-1", "state": "resolved"},
		},
	}
	if _, err := adapter.ExecuteRuntimeStep(context.Background(), request); err != nil {
		t.Fatalf("execute runtime step: %v", err)
	}
	if provider.request.PromptContext == nil || len(provider.request.PromptContext.Turns) == 0 {
		t.Fatalf("promptContext = %+v, want generated turns", provider.request.PromptContext)
	}
	joined := ""
	for _, turn := range provider.request.PromptContext.Turns {
		joined += turn.Content
	}
	for _, expected := range []string{"decisionResolution", "approvalResolution", "goal one", "goal two", "dod one", "dod two"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("prompt context missing %q, joined=%s", expected, joined)
		}
	}
	if provider.request.ContextBudget == nil {
		t.Fatalf("contextBudget = nil, want budget manifest")
	}
	if provider.request.ContextBudget.GoalDoDCompressed {
		t.Fatalf("contextBudget = %+v, want goal/dod protected", provider.request.ContextBudget)
	}
}

func TestRuntimeAdapterRedactsOutboundProviderRequest(t *testing.T) {
	provider := &staticProvider{payload: []byte(`{"completionState":"continue"}`)}
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		APIKey:   "provider-secret",
		Model:    "test-model",
	}, provider)
	request := llmStepRequest()
	request.Input["decisionResolution"] = map[string]any{
		"resolutionPayload": map[string]any{
			"apiKey": "sk-test-secret-123456",
			"notes":  "authorization: bearer abcdefghijk",
		},
	}
	request.Input["executionContext"] = map[string]any{
		"governanceHistory": []any{
			map[string]any{
				"id":      "ticket-secret",
				"payload": "token=secret-token-value",
			},
		},
	}

	if _, err := adapter.ExecuteRuntimeStep(context.Background(), request); err != nil {
		t.Fatalf("execute runtime step: %v", err)
	}

	encoded, err := json.Marshal(provider.request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	serialized := string(encoded)
	for _, leaked := range []string{"sk-test-secret-123456", "abcdefghijk", "secret-token-value"} {
		if strings.Contains(serialized, leaked) {
			t.Fatalf("provider request leaked %q: %s", leaked, serialized)
		}
	}
	if !strings.Contains(serialized, "[REDACTED]") {
		t.Fatalf("provider request = %s, want redacted markers", serialized)
	}
	if provider.request.ContextBudget == nil || !provider.request.ContextBudget.RedactionApplied || len(provider.request.ContextBudget.RedactedPaths) == 0 {
		t.Fatalf("contextBudget = %+v, want redaction summary", provider.request.ContextBudget)
	}
}

func TestRuntimeAdapterFailsUnknownRequestedProvider(t *testing.T) {
	adapter := NewRuntimeAdapterWithProviders("local", map[string]ProviderRuntime{
		"local": {
			Config: llmmodels.ProviderConfig{
				ID:       "local",
				Provider: llmcommon.ProviderCompatible,
				APIKey:   "local-secret",
				Model:    "local-model",
			},
			Provider: &staticProvider{payload: []byte(`{"completionState":"continue"}`)},
		},
	})
	request := llmStepRequest()
	request.Input[llmcommon.ExecutionContextLLMProviderID] = "missing"

	result, err := adapter.ExecuteRuntimeStep(context.Background(), request)
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("execute err = %v, want invalid argument", err)
	}
	if result.Output["stage"] != "llm_provider_unavailable" || result.Output["provider"] != nil {
		t.Fatalf("output = %+v, want missing provider failure", result.Output)
	}
}

func TestRuntimeAdapterFailoverOnRetryableProviderFailure(t *testing.T) {
	primary := &failoverProvider{err: errors.New("http status 429: rate limit reached")}
	backup := &failoverProvider{payload: []byte(`{
		"completionState":"continue",
		"shellCommands":[{"command":"echo","args":["fallback"]}]
	}`)}
	adapter := NewRuntimeAdapterWithProviders("primary", map[string]ProviderRuntime{
		"primary": {
			Config: llmmodels.ProviderConfig{
				ID:                "primary",
				Provider:          llmcommon.ProviderCompatible,
				APIKey:            "primary-secret",
				Model:             "primary-model",
				FailoverEnabled:   true,
				FailoverProviders: []string{"backup"},
			},
			Provider: primary,
		},
		"backup": {
			Config: llmmodels.ProviderConfig{
				ID:       "backup",
				Provider: llmcommon.ProviderCompatible,
				APIKey:   "backup-secret",
				Model:    "backup-model",
			},
			Provider: backup,
		},
	})

	result, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest())
	if err != nil {
		t.Fatalf("execute runtime step: %v", err)
	}
	if len(primary.requests) != 1 || len(backup.requests) != 1 {
		t.Fatalf("provider calls primary=%d backup=%d, want one call each", len(primary.requests), len(backup.requests))
	}
	if result.Output["stage"] != "llm_shell_plan" {
		t.Fatalf("output stage = %v, want llm_shell_plan from backup provider", result.Output["stage"])
	}
	budget, ok := result.Output["contextBudget"].(map[string]any)
	if !ok || budget["providerId"] != nil || budget["model"] != nil || budget["profile"] != nil {
		t.Fatalf("contextBudget = %+v, want budget without provider/model/profile binding", result.Output["contextBudget"])
	}
	failover, ok := result.Output["llmFailover"].(map[string]any)
	if !ok ||
		failover["attempted"] != true ||
		failover["success"] != true ||
		failover["failureClass"] != providerFailureClassRateLimited ||
		failover["fromProviderId"] != nil ||
		failover["toProviderId"] != nil ||
		failover["resolvedBy"] != nil {
		t.Fatalf("llmFailover = %+v, want successful failover metadata", result.Output["llmFailover"])
	}
}

func TestRuntimeAdapterLoadsProviderConfigAtExecutionTime(t *testing.T) {
	firstProviderCalls := 0
	firstProvider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		firstProviderCalls++
		writeChatCompletion(t, w, `{"completionState":"continue","shellCommands":[{"command":"echo","args":["first"]}]}`)
	}))
	defer firstProvider.Close()
	secondProviderCalls := 0
	secondProvider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		secondProviderCalls++
		writeChatCompletion(t, w, `{"completionState":"continue","shellCommands":[{"command":"echo","args":["second"]}]}`)
	}))
	defer secondProvider.Close()

	baseURL := firstProvider.URL + "/v1"
	adapter := NewRuntimeAdapterFromConfigLoader(func() (string, map[string]llmmodels.ProviderConfig, error) {
		return "local", map[string]llmmodels.ProviderConfig{
			"local": {
				ID:       "local",
				Provider: llmcommon.ProviderCompatible,
				BaseURL:  baseURL,
				APIKey:   "secret-value",
				Model:    "test-model",
				Timeout:  time.Second,
			},
		}, nil
	})

	firstResult, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest())
	if err != nil {
		t.Fatalf("first execute runtime step: %v", err)
	}
	baseURL = secondProvider.URL + "/v1"
	secondResult, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest())
	if err != nil {
		t.Fatalf("second execute runtime step: %v", err)
	}
	if firstProviderCalls != 1 || secondProviderCalls != 1 {
		t.Fatalf("provider calls first=%d second=%d, want one call each", firstProviderCalls, secondProviderCalls)
	}
	if firstResult.Output["nextInput"].(map[string]any)["args"].([]string)[0] != "first" ||
		secondResult.Output["nextInput"].(map[string]any)["args"].([]string)[0] != "second" {
		t.Fatalf("outputs first=%+v second=%+v, want runtime-loaded provider results", firstResult.Output, secondResult.Output)
	}
}

func TestRuntimeAdapterCachesProviderConfigUntilRevisionChanges(t *testing.T) {
	firstProviderCalls := 0
	firstProvider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		firstProviderCalls++
		writeChatCompletion(t, w, `{"completionState":"continue","shellCommands":[{"command":"echo","args":["first"]}]}`)
	}))
	defer firstProvider.Close()
	secondProviderCalls := 0
	secondProvider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		secondProviderCalls++
		writeChatCompletion(t, w, `{"completionState":"continue","shellCommands":[{"command":"echo","args":["second"]}]}`)
	}))
	defer secondProvider.Close()

	revision := uint64(1)
	baseURL := firstProvider.URL + "/v1"
	loaderCalls := 0
	adapter := NewRuntimeAdapterFromConfigLoaderWithRevision(func() (string, map[string]llmmodels.ProviderConfig, error) {
		loaderCalls++
		return "local", map[string]llmmodels.ProviderConfig{
			"local": {
				ID:       "local",
				Provider: llmcommon.ProviderCompatible,
				BaseURL:  baseURL,
				APIKey:   "secret-value",
				Model:    "test-model",
				Timeout:  time.Second,
			},
		}, nil
	}, func() uint64 { return revision })

	firstResult, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest())
	if err != nil {
		t.Fatalf("first execute runtime step: %v", err)
	}
	baseURL = secondProvider.URL + "/v1"
	secondResult, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest())
	if err != nil {
		t.Fatalf("second execute runtime step: %v", err)
	}
	if loaderCalls != 1 || firstProviderCalls != 2 || secondProviderCalls != 0 {
		t.Fatalf("calls loader=%d firstProvider=%d secondProvider=%d, want cached runtime before revision change", loaderCalls, firstProviderCalls, secondProviderCalls)
	}
	if firstResult.Output["nextInput"].(map[string]any)["args"].([]string)[0] != "first" ||
		secondResult.Output["nextInput"].(map[string]any)["args"].([]string)[0] != "first" {
		t.Fatalf("outputs first=%+v second=%+v, want cached provider result", firstResult.Output, secondResult.Output)
	}

	revision++
	thirdResult, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest())
	if err != nil {
		t.Fatalf("third execute runtime step: %v", err)
	}
	if loaderCalls != 2 || secondProviderCalls != 1 {
		t.Fatalf("calls loader=%d secondProvider=%d, want reload after revision change", loaderCalls, secondProviderCalls)
	}
	if thirdResult.Output["nextInput"].(map[string]any)["args"].([]string)[0] != "second" {
		t.Fatalf("third output = %+v, want refreshed provider result", thirdResult.Output)
	}
}

func TestRuntimeAdapterLazilyConstructsOnlySelectedProvider(t *testing.T) {
	revision := uint64(1)
	factoryCalls := map[string]int{}
	adapter := NewRuntimeAdapterFromConfigLoaderWithRevision(func() (string, map[string]llmmodels.ProviderConfig, error) {
		return "selected", map[string]llmmodels.ProviderConfig{
			"selected": {
				ID:       "selected",
				Provider: llmcommon.ProviderCompatible,
				BaseURL:  "http://selected.test/v1",
				APIKey:   "secret-value",
				Model:    "selected-model",
				Timeout:  time.Second,
			},
			"unused": {
				ID:       "unused",
				Provider: llmcommon.ProviderOpenAI,
				BaseURL:  "http://unused.test/v1",
				APIKey:   "secret-value",
				Model:    "unused-model",
				Timeout:  time.Second,
			},
		}, nil
	}, func() uint64 { return revision })
	adapter.providerFactory = func(config llmmodels.ProviderConfig) Provider {
		factoryCalls[config.ID]++
		return &staticProvider{payload: []byte(`{"completionState":"continue"}`)}
	}

	if _, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest()); err != nil {
		t.Fatalf("first execute runtime step: %v", err)
	}
	if _, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest()); err != nil {
		t.Fatalf("second execute runtime step: %v", err)
	}
	if factoryCalls["selected"] != 1 || factoryCalls["unused"] != 0 {
		t.Fatalf("factory calls = %+v, want only selected provider constructed once", factoryCalls)
	}

	revision++
	if _, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest()); err != nil {
		t.Fatalf("third execute runtime step after revision: %v", err)
	}
	if factoryCalls["selected"] != 2 || factoryCalls["unused"] != 0 {
		t.Fatalf("factory calls = %+v, want selected provider reconstructed after revision only", factoryCalls)
	}
}

func writeChatCompletion(t *testing.T, w http.ResponseWriter, content string) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	payload := map[string]any{
		"choices": []map[string]any{{
			"message": map[string]string{"content": content},
		}},
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("write chat completion: %v", err)
	}
}

func TestRuntimeAdapterNormalizesRequiredDecisionFields(t *testing.T) {
	provider := &staticProvider{payload: []byte(`{
		"thoughtSummary":"need product direction",
		"completionState":"blocked_on_decision",
		"requiredDecision":{
			"reason":"missing implementation path",
			"decisionKind":"provide_missing_info",
			"question":"Should the run use the shell sandbox first?",
			"context":{"topic":"execution"}
		}
	}`)}
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		APIKey:   "secret-value",
		Model:    "test-model",
	}, provider)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest())
	if err != nil {
		t.Fatalf("execute runtime step: %v", err)
	}
	if result.Output["stage"] != "llm_decision_required" ||
		result.Output["blockDecisionReason"] != "missing implementation path" ||
		result.Output["blockDecisionKind"] != "provide_missing_info" ||
		result.Output["blockDecisionQuestion"] != "Should the run use the shell sandbox first?" {
		t.Fatalf("output = %+v, want structured decision block", result.Output)
	}
	contextMap, ok := result.Output["blockDecisionContext"].(map[string]any)
	if !ok || contextMap["topic"] != "execution" {
		t.Fatalf("blockDecisionContext = %+v, want topic execution", result.Output["blockDecisionContext"])
	}
}

func TestRuntimeAdapterExposesExecutionGraph(t *testing.T) {
	provider := &staticProvider{payload: []byte(`{
		"thoughtSummary":"plan shell inspection and validation",
		"completionState":"continue",
		"executionGraph":{
			"nodes":[
				{"id":"inspect","kind":"shell/command","input":{"command":"ls","args":["-la"]}},
				{"id":"test","kind":"shell/command","dependsOn":["inspect"],"input":{"command":"go","args":["test","./..."]}}
			]
		}
	}`)}
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		APIKey:   "secret-value",
		Model:    "test-model",
	}, provider)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest())
	if err != nil {
		t.Fatalf("execute runtime step: %v", err)
	}
	graph, ok := result.Output["executionGraph"].(map[string]any)
	if result.Output["stage"] != "llm_execution_graph_plan" || !ok {
		t.Fatalf("output = %+v, want execution graph plan", result.Output)
	}
	nodes, ok := graph["nodes"].([]map[string]any)
	if !ok || len(nodes) != 2 || nodes[1]["kind"] != "shell/command" {
		t.Fatalf("nodes = %+v, want shell command graph nodes", graph["nodes"])
	}
}

func TestRuntimeAdapterRejectsReservedGovernanceReportGraphPatchTarget(t *testing.T) {
	provider := &staticProvider{payload: []byte(`{
		"completionState":"continue",
		"executionGraph":{
			"nodes":[
				{"id":"write-report","kind":"patch/apply","input":{"patchType":"file_write","targetRef":"approval_record.md","content":"approval"}}
			]
		}
	}`)}
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		APIKey:   "secret-value",
		Model:    "test-model",
	}, provider)

	_, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest())
	if err == nil || !strings.Contains(err.Error(), "reserved for runtime governance/report artifacts") {
		t.Fatalf("ExecuteRuntimeStep err = %v, want reserved graph patch target error", err)
	}
}

func TestRuntimeAdapterNormalizesPatchRequestsGraphNodeKind(t *testing.T) {
	provider := &staticProvider{payload: []byte(`{
		"completionState":"continue",
		"executionGraph":{
			"nodes":[
				{"id":"write-result","kind":"patchRequests","input":{"patchType":"file_write","targetRef":"result.txt","content":"ready"}},
				{"id":"verify","kind":"shell/command","dependsOn":["write-result"],"input":{"command":"test","args":["-f","result.txt"]}}
			]
		}
	}`)}
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		APIKey:   "secret-value",
		Model:    "test-model",
	}, provider)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest())
	if err != nil {
		t.Fatalf("execute runtime step: %v", err)
	}
	graph, ok := result.Output["executionGraph"].(map[string]any)
	if result.Output["stage"] != "llm_execution_graph_plan" || !ok {
		t.Fatalf("output = %+v, want execution graph plan", result.Output)
	}
	nodes, ok := graph["nodes"].([]map[string]any)
	if !ok || len(nodes) != 2 {
		t.Fatalf("nodes = %+v, want two graph nodes", graph["nodes"])
	}
	if nodes[0]["kind"] != llmcommon.RuntimeBlockKindPatchApply {
		t.Fatalf("first node kind = %v, want %s", nodes[0]["kind"], llmcommon.RuntimeBlockKindPatchApply)
	}
}

func TestRuntimeAdapterReturnsErrorForFailedDecision(t *testing.T) {
	provider := &staticProvider{payload: []byte(`{
		"thoughtSummary":"DoD cannot be satisfied with current context",
		"completionState":"failed"
	}`)}
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		APIKey:   "secret-value",
		Model:    "test-model",
	}, provider)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest())
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("execute err = %v, want invalid argument", err)
	}
	if result.Output["status"] != "failed" ||
		result.Output["stage"] != "llm_failed" ||
		result.Output["error"] != "DoD cannot be satisfied with current context" {
		t.Fatalf("output = %+v, want failed llm decision", result.Output)
	}
}

func TestRuntimeAdapterExposesCompletedDoDAssessment(t *testing.T) {
	provider := &staticProvider{payload: []byte(`{
		"thoughtSummary":"DoD satisfied by validation",
		"completionState":"completed",
		"dodAssessment":{
			"satisfied":true,
			"summary":"all checks passed",
			"items":[{"item":"tests pass","state":"satisfied","evidenceRefs":["run-1/validation/runtime-step"]}],
			"goalCoverage":{"satisfied":true,"items":[{"goal":"goal covered","state":"satisfied","evidenceRefs":["run-1/validation/runtime-step"]}]},
			"acceptanceDimensions":{"satisfied":true,"items":[{"id":"runtime_entrypoint","state":"satisfied","evidenceRefs":["run-1/build/runtime-step"]}]},
			"deliverableCoverage":{"satisfied":true,"items":[{"id":"user_deliverable","state":"satisfied","artifactRefs":["run-1/artifacts/manifest"]}]}
		}
	}`)}
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		APIKey:   "secret-value",
		Model:    "test-model",
	}, provider)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest())
	if err != nil {
		t.Fatalf("execute runtime step: %v", err)
	}
	assessment, ok := result.Output["dodAssessment"].(map[string]any)
	if !ok || assessment["satisfied"] != true || assessment["summary"] != "all checks passed" {
		t.Fatalf("dodAssessment = %+v, want satisfied assessment", result.Output["dodAssessment"])
	}
	items, ok := assessment["items"].([]map[string]any)
	if !ok || len(items) != 1 {
		t.Fatalf("dodAssessment items = %+v, want one item", assessment["items"])
	}
	coverage, ok := assessment["goalCoverage"].(map[string]any)
	if !ok {
		t.Fatalf("goalCoverage = %+v, want map", assessment["goalCoverage"])
	}
	coverageItems, ok := coverage["items"].([]map[string]any)
	if !ok || coverage["satisfied"] != true || len(coverageItems) != 1 || coverageItems[0]["goal"] != "goal covered" {
		t.Fatalf("goalCoverage = %+v, want preserved goal coverage", assessment["goalCoverage"])
	}
	dimensions, ok := assessment["acceptanceDimensions"].(map[string]any)
	if !ok {
		t.Fatalf("acceptanceDimensions = %+v, want map", assessment["acceptanceDimensions"])
	}
	dimensionItems, ok := dimensions["items"].([]map[string]any)
	if !ok || len(dimensionItems) != 1 || dimensionItems[0]["id"] != "runtime_entrypoint" {
		t.Fatalf("acceptanceDimensions = %+v, want preserved dimension coverage", assessment["acceptanceDimensions"])
	}
	deliverables, ok := assessment["deliverableCoverage"].(map[string]any)
	if !ok {
		t.Fatalf("deliverableCoverage = %+v, want map", assessment["deliverableCoverage"])
	}
	deliverableItems, ok := deliverables["items"].([]map[string]any)
	if !ok || len(deliverableItems) != 1 || deliverableItems[0]["id"] != "user_deliverable" {
		t.Fatalf("deliverableCoverage = %+v, want preserved deliverable coverage", assessment["deliverableCoverage"])
	}
	if refs := deliverableItems[0]["artifactRefs"]; !reflect.DeepEqual(refs, []string{"run-1/artifacts/manifest"}) {
		t.Fatalf("artifactRefs = %+v, want preserved artifact refs", refs)
	}
}

func TestParseLLMExecutionDecisionRejectsSatisfiedDoDWithoutEvidence(t *testing.T) {
	_, err := ParseLLMExecutionDecision([]byte(`{
		"completionState":"completed",
		"dodAssessment":{"satisfied":true,"summary":"done","items":[{"item":"done","state":"satisfied"}]}
	}`))
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("parse err = %v, want invalid argument", err)
	}
}

func TestParseLLMExecutionDecisionRejectsCompletedDoDWithoutSummary(t *testing.T) {
	_, err := ParseLLMExecutionDecision([]byte(`{
		"completionState":"completed",
		"dodAssessment":{"satisfied":true,"items":[{"item":"done","state":"satisfied","evidenceRefs":["run-1/step-1"]}]}
	}`))
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("parse err = %v, want invalid argument", err)
	}
}

func TestParseLLMExecutionDecisionAcceptsMarkdownFencedJSONObject(t *testing.T) {
	decision, err := ParseLLMExecutionDecision([]byte("```json\n{\"completionState\":\"continue\"}\n```"))
	if err != nil {
		t.Fatalf("parse decision: %v", err)
	}
	if decision.CompletionState != "continue" {
		t.Fatalf("completionState = %q, want continue", decision.CompletionState)
	}
}

func TestParseLLMExecutionDecisionRejectsUnknownFieldAfterNormalization(t *testing.T) {
	_, err := ParseLLMExecutionDecision([]byte("Here is the json:\n{\"workspaceId\":\"ws-1\",\"completionState\":\"continue\"}"))
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("parse err = %v, want invalid argument", err)
	}
}

func TestParseLLMExecutionDecisionAllowsNestedGovernanceExtraFields(t *testing.T) {
	decision, err := ParseLLMExecutionDecision([]byte(`{
		"completionState":"blocked_on_approval",
		"requiredApproval":{
			"reason":"operator approval is required",
			"action":"query external weather tool",
			"approver":"operator",
			"context":{"risk":"external_tool_call"}
		}
	}`))
	if err != nil {
		t.Fatalf("parse decision: %v", err)
	}
	if decision.RequiredApproval == nil || decision.RequiredApproval.Reason != "operator approval is required" {
		t.Fatalf("requiredApproval = %+v, want parsed approval", decision.RequiredApproval)
	}
	if decision.RequiredApproval.Action != "query external weather tool" || decision.RequiredApproval.Context["risk"] != "external_tool_call" {
		t.Fatalf("requiredApproval action/context = %q/%+v, want parsed risk metadata", decision.RequiredApproval.Action, decision.RequiredApproval.Context)
	}
}

func TestNormalizeDecisionOutputPreservesRequiredApprovalActionContext(t *testing.T) {
	output, err := NormalizeDecisionOutput(runtimemodel.StepExecutionRequest{
		RunID:  common.RunID("run-approval"),
		StepID: common.StepID("run-approval/llm-1"),
	}, llmmodels.LLMExecutionDecision{
		CompletionState: "blocked_on_approval",
		RequiredApproval: &llmmodels.RequiredApproval{
			Reason:  "risky operation requires approval",
			Action:  "remove public API",
			Context: map[string]any{"risk": "breaking_change"},
		},
	})
	if err != nil {
		t.Fatalf("normalize decision output: %v", err)
	}
	if output["blockApprovalReason"] != "risky operation requires approval" ||
		output["blockApprovalAction"] != "remove public API" {
		t.Fatalf("approval output = %+v, want reason/action", output)
	}
	contextMap, ok := output["blockApprovalContext"].(map[string]any)
	if !ok || contextMap["risk"] != "breaking_change" {
		t.Fatalf("blockApprovalContext = %+v, want risk metadata", output["blockApprovalContext"])
	}
}

func TestParseLLMExecutionDecisionRejectsInvalidExecutionGraph(t *testing.T) {
	_, err := ParseLLMExecutionDecision([]byte(`{
		"completionState":"continue",
		"executionGraph":{"nodes":[
			{"id":"a","kind":"shell/command","dependsOn":["b"]},
			{"id":"b","kind":"shell/command","dependsOn":["a"]}
		]}
	}`))
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("parse err = %v, want invalid argument", err)
	}
}

func TestParseLLMExecutionDecisionRejectsUnavailableWorkorchCLI(t *testing.T) {
	for _, payload := range []string{
		`{"completionState":"continue","shellCommands":[{"command":"workorch","args":["run","snapshot"]}]}`,
		`{"completionState":"continue","executionGraph":{"nodes":[{"id":"inspect","kind":"shell/command","input":{"command":"workorch","args":["run","snapshot"]}}]}}`,
		`{"completionState":"continue","executionGraph":{"nodes":[{"id":"inspect","kind":"shell/command","input":{"command":"sh","args":["-c","workorch run snapshot"]}}]}}`,
	} {
		_, err := ParseLLMExecutionDecision([]byte(payload))
		if !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("parse err = %v, want invalid argument for payload %s", err, payload)
		}
		if !strings.Contains(err.Error(), "unavailable runtime command") {
			t.Fatalf("parse err = %v, want unavailable runtime command", err)
		}
	}
}

func TestParseLLMExecutionDecisionRejectsShellSyntaxInCommandName(t *testing.T) {
	for _, payload := range []string{
		`{"completionState":"continue","shellCommands":[{"command":"mkdir -p llm-proxy && cd llm-proxy"}]}`,
		`{"completionState":"continue","executionGraph":{"nodes":[{"id":"write","kind":"shell/command","input":{"command":"cat > llm-proxy/config.yaml << 'EOF'"}}]}}`,
	} {
		_, err := ParseLLMExecutionDecision([]byte(payload))
		if !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("parse err = %v, want invalid argument for payload %s", err, payload)
		}
		if !strings.Contains(err.Error(), "command must be an executable name or path only") {
			t.Fatalf("parse err = %v, want executable name validation", err)
		}
	}
}

func TestParseLLMExecutionDecisionRejectsUndeclaredLocalWorkorchServiceAPI(t *testing.T) {
	for _, payload := range []string{
		`{"completionState":"continue","shellCommands":[{"command":"bash","args":["-c","curl -s -X POST -H 'Content-Type: application/json' -d '{\"priority\":1}' http://localhost:8080/runs"]}]}`,
		`{"completionState":"continue","executionGraph":{"nodes":[{"id":"inspect","kind":"shell/command","input":{"command":"bash","args":["-c","curl -s http://127.0.0.1:8080/observability?run=run-1"]}}]}}`,
		`{"completionState":"continue","shellCommands":[{"command":"curl","args":["-s","http://localhost:8080/metrics?run=run-1"]}]}`,
	} {
		_, err := ParseLLMExecutionDecision([]byte(payload))
		if !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("parse err = %v, want invalid argument for payload %s", err, payload)
		}
		if !strings.Contains(err.Error(), "undeclared local workorch service API") {
			t.Fatalf("parse err = %v, want undeclared local workorch service API", err)
		}
	}
}

func TestParseLLMExecutionDecisionAcceptsDeclaredLocalWorkorchServiceAPI(t *testing.T) {
	for _, payload := range []string{
		`{"completionState":"continue","shellCommands":[{"command":"curl","args":["-s","http://127.0.0.1:8080/api/v1/workorch/runs/run-1/snapshot"]}]}`,
		`{"completionState":"continue","executionGraph":{"nodes":[{"id":"metrics","kind":"shell/command","input":{"command":"curl","args":["-s","http://localhost:8080/metrics"]}}]}}`,
	} {
		decision, err := ParseLLMExecutionDecision([]byte(payload))
		if err != nil {
			t.Fatalf("parse decision err = %v for payload %s", err, payload)
		}
		if decision.CompletionState != "continue" {
			t.Fatalf("completionState = %q, want continue", decision.CompletionState)
		}
	}
}

func TestParseLLMExecutionDecisionRepairsMalformedShellCommandArgs(t *testing.T) {
	payload := `{"completionState":"continue","executionGraph":{"nodes":[{"id":"explore_workspace","kind":"shell/command","input":{"command":"ls","args":["-la"]}},{"id":"detect_structure","kind":"shell/command","dependsOn":["explore_workspace"],"input":{"command":"find",".","-maxdepth","3","-type","f","-name","*.go"]}},{"id":"detect_existing","kind":"shell/command","dependsOn":["detect_structure"],"input":{"command":"find",".","args":["-type","f","-name","*proxy*"]}}]}}`
	decision, err := ParseLLMExecutionDecision([]byte(payload))
	if err != nil {
		t.Fatalf("parse repaired decision: %v", err)
	}
	if decision.ExecutionGraph == nil || len(decision.ExecutionGraph.Nodes) != 3 {
		t.Fatalf("executionGraph = %+v, want three nodes", decision.ExecutionGraph)
	}
	structureArgs := stringSliceValue(decision.ExecutionGraph.Nodes[1].Input["args"])
	if strings.Join(structureArgs, " ") != ". -maxdepth 3 -type f -name *.go" {
		t.Fatalf("structure args = %#v, want repaired positional args", structureArgs)
	}
	existingArgs := stringSliceValue(decision.ExecutionGraph.Nodes[2].Input["args"])
	if strings.Join(existingArgs, " ") != ". -type f -name *proxy*" {
		t.Fatalf("existing args = %#v, want prepended positional arg", existingArgs)
	}
}

func TestParseLLMExecutionDecisionRejectsMemoryWriteWithoutSummary(t *testing.T) {
	_, err := ParseLLMExecutionDecision([]byte(`{
		"completionState":"continue",
		"memoryWrites":[{"memoryType":"workspace"}]
	}`))
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("parse err = %v, want invalid argument", err)
	}
}

func TestRuntimeAdapterFailsOnSchemaInvalidOutput(t *testing.T) {
	provider := &staticProvider{payload: []byte(`{"completionState":"unknown"}`)}
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		APIKey:   "secret-value",
		Model:    "test-model",
	}, provider)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest())
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("execute err = %v, want invalid argument", err)
	}
	if result.Output["status"] != "failed" || result.Output["stage"] != "llm_schema_invalid" {
		t.Fatalf("output = %+v, want schema invalid failure", result.Output)
	}
}

func runtimeCapabilitiesContainBlock(caps llmmodels.RuntimeCapabilities, kind string) bool {
	for _, block := range caps.BlockKinds {
		if block.Kind == kind {
			return true
		}
	}
	return false
}

func runtimeCapabilityBlockDescription(caps llmmodels.RuntimeCapabilities, kind string) string {
	for _, block := range caps.BlockKinds {
		if block.Kind == kind {
			return block.Description
		}
	}
	return ""
}

func runtimeCapabilitiesContainServiceAPI(caps llmmodels.RuntimeCapabilities, method string, pathTemplate string) bool {
	for _, api := range caps.ServiceAPIs {
		if api.Method == method && api.PathTemplate == pathTemplate {
			return true
		}
	}
	return false
}

func llmStepRequest() runtimemodel.StepExecutionRequest {
	return runtimemodel.StepExecutionRequest{
		WorkspaceID: common.WorkspaceID("ws-1"),
		RunID:       common.RunID("run-1"),
		StepID:      common.StepID("run-1/llm-1"),
		Kind:        llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			"goalId": "goal-1",
			"doDId":  "dod-1",
		},
	}
}
