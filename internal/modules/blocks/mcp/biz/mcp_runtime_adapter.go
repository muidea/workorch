package mcp

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type RuntimeAdapter struct {
	service serverGetter
	client  *http.Client
	// idleSessionTimeout belongs to this adapter instance. It must not be a
	// package-global test knob because delayed eviction callbacks can outlive a
	// test and race with another adapter's configuration.
	idleSessionTimeout time.Duration

	sessionMu sync.Mutex
	sessions  map[string]*stdioSession
}

type serverGetter interface {
	GetServer(context.Context, common.MCPServerID) (*mcpmodel.Server, error)
}

type registrySnapshotGetter interface {
	RegistrySnapshot(context.Context) (*mcpmodel.RegistrySnapshot, string, error)
}

const (
	mcpProtocolVersion = "2025-11-25"
	mcpClientName      = "workorch"
	mcpClientVersion   = "mvp"

	sessionPolicyLongLived = "long_lived"

	defaultLongLivedStdioSessionIdleTimeout = 5 * time.Minute

	// maxMCPObservedFailureTargets bounds a capability owner's structured
	// failure observation before it crosses the typed runtime receipt boundary.
	maxMCPObservedFailureTargets = 16
)

type operationSpec struct {
	Operation  string
	ServerID   string
	Tool       string
	URI        string
	Prompt     string
	Input      []runtimecontract.OperationParameter
	Arguments  []runtimecontract.OperationParameter
	Summary    string
	Stream     bool
	TargetRefs []string
	TargetRole string
}

func NewRuntimeAdapter(service serverGetter) *RuntimeAdapter {
	return NewRuntimeAdapterWithHTTPClient(service, http.DefaultClient)
}

func NewRuntimeAdapterWithHTTPClient(service serverGetter, client *http.Client) *RuntimeAdapter {
	return newRuntimeAdapterWithIdleSessionTimeout(service, client, defaultLongLivedStdioSessionIdleTimeout)
}

func newRuntimeAdapterWithIdleSessionTimeout(service serverGetter, client *http.Client, idleSessionTimeout time.Duration) *RuntimeAdapter {
	if client == nil {
		client = http.DefaultClient
	}
	return &RuntimeAdapter{service: service, client: client, idleSessionTimeout: idleSessionTimeout, sessions: map[string]*stdioSession{}}
}

func (a *RuntimeAdapter) ExecuteOperation(ctx context.Context, request RuntimeOperationRequest) (RuntimeOperationResult, error) {
	spec, err := parseOperationSpec(request)
	if err != nil {
		return failedResult(spec, invalidStage(spec), err), err
	}
	if a == nil || a.service == nil {
		err := common.ErrInvalidArgument
		return failedResult(spec, unavailableStage(spec), err), err
	}
	server, err := a.service.GetServer(ctx, common.MCPServerID(spec.ServerID))
	if err != nil {
		return failedResult(spec, serverUnavailableStage(spec), err), err
	}
	if err := validateServerPolicy(server, spec); err != nil {
		return failedResult(spec, rejectedStage(spec), err), err
	}
	switch strings.ToLower(strings.TrimSpace(server.Transport)) {
	case "http", "https":
		return a.executeHTTPOperation(ctx, request, server, spec)
	case "stdio":
		return a.executeStdioOperation(ctx, request, server, spec)
	default:
		err := fmt.Errorf("%w: unsupported mcp transport %q", common.ErrInvalidArgument, server.Transport)
		return failedResult(spec, unsupportedTransportStage(spec), err), err
	}
}

// ProbeOperation validates the selected MCP server and its local transport
// launch prerequisites without creating a stdio session or sending an HTTP
// request. Runtime uses it before dispatch seal; execution remains the only
// path that can call a tool or create a receipt.
func (a *RuntimeAdapter) ProbeOperation(ctx context.Context, request RuntimeOperationRequest) error {
	spec, err := parseOperationSpec(request)
	if err != nil {
		return err
	}
	if a == nil || a.service == nil {
		return common.ErrInvalidArgument
	}
	server, err := a.service.GetServer(ctx, common.MCPServerID(spec.ServerID))
	if err != nil {
		return err
	}
	if err := validateServerPolicy(server, spec); err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(server.Transport)) {
	case "http", "https":
		endpoint := strings.TrimSpace(server.Endpoint)
		if endpoint == "" {
			return fmt.Errorf("%w: mcp endpoint is required", common.ErrInvalidArgument)
		}
		if _, err := url.ParseRequestURI(endpoint); err != nil {
			return fmt.Errorf("%w: invalid mcp endpoint: %v", common.ErrInvalidArgument, err)
		}
		return nil
	case "stdio":
		command, _, _, err := stdioLaunchConfig(server, request)
		if err != nil {
			return err
		}
		if strings.ContainsAny(command, `/\\`) {
			info, statErr := os.Stat(command)
			if statErr != nil {
				return statErr
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("%w: mcp stdio command is not a regular file", common.ErrInvalidArgument)
			}
			return nil
		}
		_, err = exec.LookPath(command)
		return err
	default:
		return fmt.Errorf("%w: unsupported mcp transport %q", common.ErrInvalidArgument, server.Transport)
	}
}
