package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	blockcommon "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/common"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/events"
	"github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/protocol"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
)

// A real child process records every received request, including anything sent
// after an invalid negotiation. It never runs a real MCP server or provider.
func TestMCPNegotiationProcess(t *testing.T) {
	if os.Getenv("WORKORCH_TEST_MCP_NEGOTIATION") != "1" {
		return
	}
	f, err := os.OpenFile(os.Getenv("WORKORCH_TEST_MCP_NEGOTIATION_LOG"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		os.Exit(2)
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		_, _ = fmt.Fprintln(f, scanner.Text())
		var q struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if json.Unmarshal(scanner.Bytes(), &q) != nil {
			os.Exit(3)
		}
		switch q.Method {
		case "initialize":
			fmt.Print(os.Getenv("WORKORCH_TEST_MCP_NEGOTIATION_RESPONSE"))
		case "notifications/initialized":
		case "tools/list":
			fmt.Printf("{\"jsonrpc\":\"2.0\",\"id\":%d,\"result\":{\"tools\":[{\"name\":\"inspect\"},{\"name\":\"unselected-tool\"}]}}\n", q.ID)
		case "resources/list":
			fmt.Printf("{\"jsonrpc\":\"2.0\",\"id\":%d,\"result\":{\"resources\":[{\"uri\":\"file:///fixture\"}]}}\n", q.ID)
		case "prompts/list":
			fmt.Printf("{\"jsonrpc\":\"2.0\",\"id\":%d,\"result\":{\"prompts\":[{\"name\":\"inspect\"}]}}\n", q.ID)
		case "tools/call":
			fmt.Printf("{\"jsonrpc\":\"2.0\",\"id\":%d,\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"fixture\"}]}}\n", q.ID)
		case "resources/read":
			fmt.Printf("{\"jsonrpc\":\"2.0\",\"id\":%d,\"result\":{\"contents\":[{\"uri\":\"file:///fixture\",\"text\":\"fixture\"}]}}\n", q.ID)
		case "prompts/get":
			fmt.Printf("{\"jsonrpc\":\"2.0\",\"id\":%d,\"result\":{\"messages\":[{\"role\":\"user\",\"content\":{\"type\":\"text\",\"text\":\"fixture\"}}]}}\n", q.ID)
		default:
			os.Exit(4)
		}
	}
	_, _ = fmt.Fprintln(f, `{"method":"stdin_closed"}`)
	_ = f.Close()
	if os.Getenv("WORKORCH_TEST_MCP_NEGOTIATION_EXIT_FAILURE") == "1" {
		os.Exit(5)
	}
	os.Exit(0)
}

func negotiationFixture(t *testing.T, response string) (*mcpmodel.Server, string) {
	t.Helper()
	t.Setenv("WORKORCH_TEST_MCP_NEGOTIATION", "1")
	t.Setenv("WORKORCH_TEST_MCP_NEGOTIATION_RESPONSE", response)
	log := filepath.Join(t.TempDir(), "requests.jsonl")
	t.Setenv("WORKORCH_TEST_MCP_NEGOTIATION_LOG", log)
	t.Setenv("GORACE", os.Getenv("GORACE")+" atexit_sleep_ms=0")
	return &mcpmodel.Server{ID: "fixture", Transport: "stdio", Command: os.Args[0], Args: []string{"-test.run=^TestMCPNegotiationProcess$"}, TimeoutMillis: 1000}, log
}

func assertNegotiationRequests(t *testing.T, log string, methods ...string) {
	t.Helper()
	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var q struct {
			Method string `json:"method"`
			Params struct {
				ProtocolVersion string `json:"protocolVersion"`
			} `json:"params"`
		}
		if json.Unmarshal([]byte(line), &q) != nil {
			t.Fatalf("bad fixture request: %s", line)
		}
		if q.Method == "initialize" && q.Params.ProtocolVersion != protocol.PreferredVersion {
			t.Fatal("preferred version drifted")
		}
		got = append(got, q.Method)
	}
	if !reflect.DeepEqual(got, methods) {
		t.Fatalf("received methods %v, want %v", got, methods)
	}
}

func TestStdioNegotiationRetainsServerVersionBeforeBusinessCall(t *testing.T) {
	for _, version := range []string{"2024-11-05", "2025-11-25"} {
		t.Run(version, func(t *testing.T) {
			result := fmt.Sprintf(`{ "protocolVersion":%q, "capabilities":{"tools":{},"resources":{},"prompts":{}}, "serverInfo":{"name":"fixture","version":"test"}, "instructions":" unchanged " }`, version)
			s, log := negotiationFixture(t, `{"jsonrpc":"2.0","id":1,"result":`+result+"}\n")
			session, err := newInitializedStdioSession(context.Background(), RuntimeOperationRequest{}, s, "fixture")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { session.Close() })
			if session.protocolVersion != version || string(session.initializeResult) != result {
				t.Fatal("negotiation result lost or rewritten")
			}
			for i, spec := range []operationSpec{
				{Operation: blockcommon.StepKindToolCall, Tool: "inspect"},
				{Operation: blockcommon.StepKindResourceRead, URI: "file:///fixture"},
				{Operation: blockcommon.StepKindPromptGet, Prompt: "inspect"},
			} {
				if _, _, err = session.Execute(context.Background(), spec, i+2); err != nil {
					t.Fatal(err)
				}
			}
			session.Close()
			assertNegotiationRequests(t, log, "initialize", "notifications/initialized", "tools/call", "resources/read", "prompts/get", "stdin_closed")
		})
	}
}

func TestStdioAndPreparationRejectBadNegotiationBeforeInitialized(t *testing.T) {
	for _, tc := range []struct{ name, response, code string }{
		{"unsupported", `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2099-01-01"}}`, "protocol_version_unsupported"},
		{"missing", `{"jsonrpc":"2.0","id":1,"result":{}}`, "protocol_failed"},
		{"null", `{"jsonrpc":"2.0","id":1,"result":null}`, "protocol_failed"},
		{"type", `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":20241105}}`, "protocol_failed"},
		{"rpc_version", `{"jsonrpc":"1.0","id":1,"result":{"protocolVersion":"2024-11-05"}}`, "protocol_failed"},
		{"fractional_id", `{"jsonrpc":"2.0","id":1.2,"result":{"protocolVersion":"2024-11-05"}}`, "protocol_failed"},
		{"rpc_error", `{"jsonrpc":"2.0","id":1,"error":{"code":-32603,"message":"fixture"}}`, "protocol_failed"},
		{"malformed", `{"jsonrpc":"2.0","id":1,"result":`, "protocol_failed"},
	} {
		for _, path := range []string{"runtime", "preparation"} {
			t.Run(tc.name+"/"+path, func(t *testing.T) {
				s, log := negotiationFixture(t, tc.response+"\n")
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				if path == "runtime" {
					session, err := newInitializedStdioSession(ctx, RuntimeOperationRequest{}, s, "fixture")
					if err == nil || session != nil {
						if session != nil {
							session.Close()
						}
						t.Fatal("invalid negotiation accepted")
					}
					if tc.code == "protocol_version_unsupported" && !errors.Is(err, protocol.ErrUnsupportedVersion) {
						t.Fatal("unsupported version diagnostic lost", err)
					}
				} else {
					o := observePreparationStdio(ctx, mcpevents.ObservePreparationCommand{DeliverableDirectory: t.TempDir()}, s)
					if o.FailureCode != tc.code || !o.Closed || !o.Complete || len(o.Frames) != 1 || string(o.Frames[0].Response) != tc.response+"\n" {
						t.Fatalf("original failure not retained: %+v", o)
					}
				}
				assertNegotiationRequests(t, log, "initialize", "stdin_closed")
			})
		}
	}
}

func TestPreparationNegotiatesBothSupportedVersionsAndRequiresCleanClose(t *testing.T) {
	for _, version := range []string{"2024-11-05", "2025-11-25"} {
		for _, badClose := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/close-failure=%v", version, badClose), func(t *testing.T) {
				response := fmt.Sprintf("{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"protocolVersion\":%q}}\n", version)
				s, log := negotiationFixture(t, response)
				if badClose {
					t.Setenv("WORKORCH_TEST_MCP_NEGOTIATION_EXIT_FAILURE", "1")
				}
				c := mcpevents.ObservePreparationCommand{Version: mcpevents.PreparationObservationVersion, WorkspaceID: "ws", WorkspaceRevision: 1, RequestID: "req", PayloadHash: "payload", SnapshotHash: "snapshot", InternalDirectory: t.TempDir(), DeliverableDirectory: t.TempDir(), RegistryRevision: "registry", ServerIDs: []string{"fixture"}, Capabilities: []mcpevents.PreparationCapability{
					{ID: "inspect-tool", ServerID: "fixture", Kind: "tool", Name: "inspect"},
					{ID: "inspect-resource", ServerID: "fixture", Kind: "resource", Name: "fixture", URI: "file:///fixture"},
					{ID: "inspect-prompt", ServerID: "fixture", Kind: "prompt", Name: "inspect"},
				}}
				r := mcpevents.ObservePreparationResult{OwnerEpoch: "mcp-preparation-epoch:" + strings.Repeat("a", 32), Command: c, RegistryStable: true, StartedAt: time.Now()}
				r.Observations = []mcpevents.PreparationServerObservation{observePreparationStdio(context.Background(), c, s)}
				r.CompletedAt = time.Now()
				if r.ValidateFor(c) != nil || r.Satisfied() == badClose {
					t.Fatalf("incorrect negotiation/close judgment: %+v", r)
				}
				if badClose && r.Observations[0].FailureCode != "close_failed" {
					t.Fatal("close failure lost")
				}
				assertNegotiationRequests(t, log, "initialize", "notifications/initialized", "tools/list", "resources/list", "prompts/list", "stdin_closed")
				// Old immutable attempts keep their original evaluation across reload.
				old := r.Clone()
				old.Command.Version = mcpevents.PreparationObservationVersionV1
				want := version == "2025-11-25" && !badClose
				if old.Satisfied() != want {
					t.Fatal("v1 historical judgment changed")
				}
				data, _ := json.Marshal(old)
				var reloaded mcpevents.ObservePreparationResult
				if json.Unmarshal(data, &reloaded) != nil || reloaded.Satisfied() != want {
					t.Fatal("historical reload changed judgment")
				}
				// The consumer rechecks the requested version, not just returned text.
				tampered := r.Clone()
				tampered.Observations[0].Frames[0].Request = []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"unknown"}}`)
				if tampered.Satisfied() {
					t.Fatal("request drift accepted")
				}
			})
		}
	}
}
