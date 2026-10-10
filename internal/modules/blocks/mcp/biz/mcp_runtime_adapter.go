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

	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/events"
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

	sessionMu     sync.Mutex
	sessions      map[string]*stdioSession
	idleTimers    map[*idleEvictionTimer]struct{}
	idleLifecycle basebiz.Lifecycle
	closing       bool
	closeOnce     sync.Once
	closeDone     chan struct{}
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

type authorizedRuntimeOperation struct {
	spec             operationSpec
	server           *mcpmodel.Server
	capability       *mcpmodel.Capability
	registryRevision string
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

// authorizeRuntimeOperation resolves one immutable Registry aggregate and
// verifies the exact capability selected by the sealed Runtime catalog. It is
// the last owner-local preflight before any endpoint/process action.
func (a *RuntimeAdapter) authorizeRuntimeOperation(ctx context.Context, request RuntimeOperationRequest, authorization mcpevents.RuntimeExecutionAuthorization) (authorizedRuntimeOperation, error) {
	spec, err := parseOperationSpec(request)
	if err != nil {
		return authorizedRuntimeOperation{}, err
	}
	if a == nil || a.service == nil {
		return authorizedRuntimeOperation{}, common.ErrInvalidArgument
	}
	reader, ok := a.service.(registrySnapshotGetter)
	if !ok {
		return authorizedRuntimeOperation{}, &runtimecontract.RuntimeCapabilityCatalogUnavailableError{OwnerModuleID: ModuleID, SubjectRef: "mcp_capability_catalog", Reason: "owner_unavailable"}
	}
	snapshot, revision, err := reader.RegistrySnapshot(ctx)
	if err != nil {
		return authorizedRuntimeOperation{}, err
	}
	if snapshot == nil || strings.TrimSpace(revision) == "" || strings.TrimSpace(revision) != strings.TrimSpace(authorization.RegistryRevision) {
		return authorizedRuntimeOperation{}, fmt.Errorf("%w: sealed MCP registry revision is stale", common.ErrConflict)
	}
	var server *mcpmodel.Server
	for _, item := range snapshot.Servers {
		if item != nil && item.ID == authorization.Capability.ServerID {
			server = mcpmodel.CloneServer(item)
			break
		}
	}
	var capability *mcpmodel.Capability
	for _, item := range snapshot.Capabilities {
		if item != nil && item.ID == authorization.Capability.ID {
			capability = mcpmodel.CloneCapability(item)
			break
		}
	}
	if server == nil || capability == nil || !capability.Enabled {
		return authorizedRuntimeOperation{}, fmt.Errorf("%w: sealed MCP capability is unavailable", common.ErrNotFound)
	}
	if err := validateServerPolicy(server, spec); err != nil {
		return authorizedRuntimeOperation{}, err
	}
	current := runtimeMCPCapabilityContexts([]*mcpmodel.Capability{capability})
	if len(current) != 1 || !sameRuntimeMCPCapabilityAuthorization(current[0], authorization.Capability) {
		return authorizedRuntimeOperation{}, fmt.Errorf("%w: sealed MCP capability metadata is stale", common.ErrConflict)
	}
	if authorization.Policy.SubjectRef != string(capability.ID) || authorization.Policy.EffectScope != runtimecontract.RuntimeEffectScope(capability.EffectScope) ||
		authorization.Policy.IdempotencyClass != runtimecontract.RuntimeOperationIdempotencyClass(capability.IdempotencyClass) {
		return authorizedRuntimeOperation{}, fmt.Errorf("%w: sealed MCP operation policy is stale", common.ErrConflict)
	}
	if err := validateMCPOperationInputSchema(capability, spec); err != nil {
		return authorizedRuntimeOperation{}, err
	}
	return authorizedRuntimeOperation{spec: spec, server: server, capability: capability, registryRevision: revision}, nil
}

func sameRuntimeMCPCapabilityAuthorization(left, right runtimecontract.RuntimeMCPCapabilityContext) bool {
	left = runtimecontract.SealMCPCapabilityEffect(left)
	right = runtimecontract.SealMCPCapabilityEffect(right)
	return left.ID == right.ID && left.ServerID == right.ServerID && left.Kind == right.Kind && left.OperationKind == right.OperationKind &&
		left.Name == right.Name && left.URI == right.URI && left.Enabled == right.Enabled && left.Status == right.Status &&
		left.EffectScope == right.EffectScope && left.IdempotencyClass == right.IdempotencyClass &&
		left.ApprovalPolicy == right.ApprovalPolicy && left.RiskLevel == right.RiskLevel
}

func (a *RuntimeAdapter) executeAuthorizedOperation(ctx context.Context, request RuntimeOperationRequest, authorized authorizedRuntimeOperation) (RuntimeOperationResult, error) {
	if a == nil || authorized.server == nil || authorized.capability == nil || authorized.registryRevision == "" {
		return failedResult(authorized.spec, unavailableStage(authorized.spec), common.ErrInvalidArgument), common.ErrInvalidArgument
	}
	var result RuntimeOperationResult
	var err error
	switch strings.ToLower(strings.TrimSpace(authorized.server.Transport)) {
	case "http", "https":
		result, err = a.executeHTTPOperation(ctx, request, authorized.server, authorized.spec)
	case "stdio":
		result, err = a.executeStdioOperation(ctx, request, authorized.server, authorized.spec)
	default:
		err = fmt.Errorf("%w: unsupported mcp transport %q", common.ErrInvalidArgument, authorized.server.Transport)
		result = failedResult(authorized.spec, unsupportedTransportStage(authorized.spec), err)
	}
	result.OwnerRevision = authorized.registryRevision
	return result, err
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
	return probeMCPServerTransport(server, request)
}

func probeMCPServerTransport(server *mcpmodel.Server, request RuntimeOperationRequest) error {
	if server == nil {
		return common.ErrInvalidArgument
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
		command, err := probeStdioLaunchCommand(server)
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

func (a *RuntimeAdapter) probeAuthorizedOperation(request RuntimeOperationRequest, authorized authorizedRuntimeOperation) error {
	if a == nil || authorized.server == nil || authorized.capability == nil {
		return common.ErrInvalidArgument
	}
	return probeMCPServerTransport(authorized.server, request)
}
