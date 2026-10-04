package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCompleteProviderDocumentOwnerPreservesFactsAndAcceptedCommand(t *testing.T) {
	for _, streaming := range []bool{true} {
		for _, outcome := range []string{"complete", "empty", "http_failure", "output_limit", "no_output"} {
			t.Run(outcome+map[bool]string{false: "/json", true: "/stream"}[streaming], func(t *testing.T) {
				ctx := context.Background()
				var calls atomic.Int32
				const document = "  {\"未知字段\":true}\r\n"
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/v1/models":
						_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": []any{map[string]any{"id": "chat-model", "supported_endpoints": []string{"/v1/responses"}, "contextWindowTokens": 8192, "maxOutputTokens": 1024}}})
					case "/v1/responses":
						calls.Add(1)
						if outcome == "http_failure" {
							w.Header().Set("Content-Type", "application/json")
							w.WriteHeader(http.StatusBadRequest)
							_, _ = w.Write([]byte("\xef\xbb\xbf原始失败\r\n"))
							return
						}
						content := document
						if outcome == "empty" || outcome == "no_output" {
							content = ""
						}
						status := "completed"
						if outcome == "output_limit" {
							status = "incomplete"
						}
						response := map[string]any{"status": status, "output_text": content}
						if outcome == "output_limit" {
							response["incomplete_details"] = map[string]any{"reason": "max_output_tokens"}
						}
						if streaming {
							w.Header().Set("Content-Type", "text/event-stream")
							if outcome == "output_limit" {
								delta, _ := json.Marshal(map[string]any{"type": "response.output_text.delta", "delta": content})
								_, _ = w.Write(append(append([]byte("data: "), delta...), '\n', '\n'))
							}
							if outcome == "no_output" {
								_, _ = w.Write([]byte("data: {\"type\":\"response.reasoning_summary_text.delta\",\"delta\":\"not document\"}\n\n"))
								return
							}
							encoded, _ := json.Marshal(map[string]any{"type": "response." + status, "response": response})
							_, _ = w.Write(append(append([]byte("data: "), encoded...), '\n', '\n'))
						} else {
							w.Header().Set("Content-Type", "application/json")
							_ = json.NewEncoder(w).Encode(response)
						}
					default:
						http.NotFound(w, r)
					}
				}))
				defer server.Close()
				configured := workorchconfig.DefaultLLMConfig()
				configured.AIProxy.BaseURL, configured.AIProxy.APIKey = server.URL, "synthetic-key"
				configured.DefaultChat = workorchconfig.ProviderModelTargetConfig{ProviderID: "ai-proxy", ModelID: "chat-model"}
				hub := magicevent.NewHub(16)
				defer hub.Terminate(ctx)
				owner, err := New(hub, nil, Dependencies{LoadLLMConfig: func() (workorchconfig.LLMConfig, error) { return configured, nil }})
				if err != nil {
					t.Fatal(err)
				}
				if err := owner.Start(); err != nil {
					t.Fatal(err)
				}
				defer owner.Teardown(ctx)
				command := llmevents.CompleteProviderDocumentCommand{RequestIdentity: "opaque-reserved-id", Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "chat-model"}, ExpectedConfigurationRevision: owner.activeLLMConfigurationRevision, Request: llmevents.ProviderRequest{WorkspaceID: "workspace-test", Model: "chat-model", Kind: "opaque-document", ContextBudget: &llmmodels.ProviderContextBudget{}, PromptContext: &llmmodels.PromptContext{Sections: []llmmodels.PromptSection{{ID: "C0", Content: "稳定合同"}}}}}
				before := command.Clone()
				acceptedHash, err := command.RequestHash()
				if err != nil {
					t.Fatal(err)
				}
				result := hub.Send(magicevent.NewEventWithContext(llmevents.TopicLLMCommandCompleteProviderDocument, llmApplicationOwnerSource, llmevents.CommandDestination, magicevent.NewHeader(), ctx, command))
				if result == nil {
					t.Fatal("missing document result")
				}
				value, resultErr := result.Get()
				if resultErr != nil {
					t.Fatalf("document result: %v", resultErr)
				}
				response, ok := value.(llmevents.CompleteProviderDocumentResult)
				if !ok || response.ValidateFor(before) != nil {
					t.Fatalf("invalid typed result: %T", value)
				}
				afterHash, _ := command.RequestHash()
				if !reflect.DeepEqual(command, before) || afterHash != acceptedHash || calls.Load() != 1 {
					t.Fatal("adapter changed accepted command or repeated call")
				}
				if outcome == "http_failure" {
					if response.Failure == nil || response.Failure.Code != "http_400" || string(response.Wire.Content) != "\xef\xbb\xbf原始失败\r\n" || !response.Wire.Complete {
						t.Fatal("HTTP failure lost original wire")
					}
				} else if outcome == "output_limit" {
					if response.Incomplete == nil || response.Incomplete.Code != "output_limit" || string(response.ObservedDocument.Content) != document || response.ObservedDocument.Complete {
						t.Fatal("output limit lost partial document")
					}
				} else if outcome == "no_output" && streaming {
					if response.Incomplete == nil || response.ObservedDocument.Observed {
						t.Fatal("reasoning-only stream fabricated an observed document")
					}
				} else {
					want := document
					if outcome != "complete" {
						want = ""
					}
					if response.Document == nil || string(response.Document.Content.Content) != want || !response.Document.Content.Complete {
						t.Fatal("complete/empty document was rewritten")
					}
				}
			})
		}
	}
}
