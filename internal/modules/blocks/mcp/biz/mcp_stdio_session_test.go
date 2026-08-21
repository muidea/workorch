package mcp

import (
	"errors"
	"os"
	"testing"

	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestResolveEnvironmentReferencesResolvesOnlyOwnerApprovedReferences(t *testing.T) {
	t.Setenv("WORKORCH_TEST_MCP_TOKEN", "resolved-secret")
	request := RuntimeOperationRequest{
		WorkspaceID:                   "ws-reference",
		RunID:                         "run-reference",
		StepID:                        "step-reference",
		WorkspaceDeliverableDirectory: "/workspace/deliverable",
		WorkspaceInternalDirectory:    "/workspace/internal",
	}
	resolved, err := resolveEnvironmentReferences(map[string]string{
		"TOKEN":     "env://WORKORCH_TEST_MCP_TOKEN",
		"WORKSPACE": "{workspaceId}",
		"RUN":       "{runId}",
		"ROOT":      "{workspaceDeliverableDirectory}",
	}, request)
	if err != nil {
		t.Fatalf("resolveEnvironmentReferences: %v", err)
	}
	if resolved["TOKEN"] != "resolved-secret" || resolved["WORKSPACE"] != "ws-reference" || resolved["RUN"] != "run-reference" || resolved["ROOT"] != "/workspace/deliverable" {
		t.Fatalf("resolved environment = %#v", resolved)
	}

	if _, err := resolveEnvironmentReferences(map[string]string{"TOKEN": "plaintext-secret"}, request); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("plaintext reference err = %v, want invalid argument", err)
	}

	const missing = "WORKORCH_TEST_MCP_DEFINITELY_MISSING"
	previous, existed := os.LookupEnv(missing)
	if err := os.Unsetenv(missing); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(missing, previous)
		} else {
			_ = os.Unsetenv(missing)
		}
	})
	if _, err := resolveEnvironmentReferences(map[string]string{"TOKEN": "env://" + missing}, request); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing environment reference err = %v, want not found", err)
	}
}

func TestProbeStdioLaunchCommandDoesNotResolveSecretReference(t *testing.T) {
	const missing = "WORKORCH_TEST_MCP_PROBE_SECRET_MISSING"
	previous, existed := os.LookupEnv(missing)
	if err := os.Unsetenv(missing); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(missing, previous)
		} else {
			_ = os.Unsetenv(missing)
		}
	})
	server := &mcpmodel.Server{Command: os.Args[0], Env: map[string]string{"TOKEN": "env://" + missing}}
	if command, err := probeStdioLaunchCommand(server); err != nil || command != os.Args[0] {
		t.Fatalf("probeStdioLaunchCommand() = %q, %v; want static command without secret resolution", command, err)
	}
	if _, _, _, err := stdioLaunchConfig(server, RuntimeOperationRequest{}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("stdioLaunchConfig missing secret err = %v, want not found at execution resolution", err)
	}
}
