package eventmapping

import (
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func TestCLIMCPMappingPreservesFullContractAndDetachesCollections(t *testing.T) {
	server := workorchmanagementcontract.MCPServer{
		ID: "mcp-a", Name: "MCP A", Transport: "stdio", Command: "mcp-a", Args: []string{"serve"},
		Env: map[string]string{"Z_KEY": "z", "A_KEY": "a"}, Enabled: true,
		AllowedTools: []string{"tool-a"}, DefaultApprovalPolicy: "required", TimeoutMillis: 1000,
	}
	dto := MCPServerDTO(server)
	if len(dto.Env) != 2 || dto.Env[0].Key != "A_KEY" {
		t.Fatalf("ordered environment = %+v", dto.Env)
	}
	dto.Args[0] = "mutated"
	dto.Env[0].Value = "mutated"
	if server.Args[0] != "serve" || server.Env["A_KEY"] != "a" {
		t.Fatal("MCP server DTO retained canonical collection storage")
	}
	dto = MCPServerDTO(server)
	roundTrip, err := MCPServerManagement(dto)
	if err != nil {
		t.Fatalf("server round trip: %v", err)
	}
	if !reflect.DeepEqual(roundTrip, server) {
		t.Fatalf("server round trip = %#v, want %#v", roundTrip, server)
	}

	capability := workorchmanagementcontract.MCPCapability{
		ID: "cap-a", ServerID: "mcp-a", Kind: "tool", Name: "tool-a", DisplayName: "Tool A",
		Tags: []string{"safe"}, Schema: map[string]any{"type": "object", "required": []any{"input"}},
		Enabled: true, ApprovalPolicy: "required", Status: "ready",
	}
	capDTO, err := MCPCapabilityDTO(capability)
	if err != nil {
		t.Fatalf("capability DTO: %v", err)
	}
	capDTO.Tags[0] = "mutated"
	if capability.Tags[0] != "safe" {
		t.Fatal("MCP capability DTO retained canonical tag storage")
	}
	capDTO, _ = MCPCapabilityDTO(capability)
	if got := MCPCapabilityManagement(capDTO); !reflect.DeepEqual(got, capability) {
		t.Fatalf("capability round trip = %#v, want %#v", got, capability)
	}
}
