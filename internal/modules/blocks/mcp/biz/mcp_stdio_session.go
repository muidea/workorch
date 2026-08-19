package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	blockcommon "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/common"
	registryevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagedconfig"
)

type stdioSession struct {
	serverID  string
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	reader    *bufio.Reader
	stderr    *limitedBuffer
	done      chan error
	lastUsed  time.Time
	mu        sync.Mutex
	closeOnce sync.Once
}

type limitedBuffer struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	limit int
}

func newInitializedStdioSession(ctx context.Context, request RuntimeOperationRequest, server *mcpmodel.Server, serverID string) (*stdioSession, error) {
	command, args, env, err := stdioLaunchConfig(server, request)
	if err != nil {
		return nil, err
	}
	initCtx, cancel := context.WithTimeout(ctx, timeoutFromServer(server))
	defer cancel()

	launchCtx := ctx
	if strings.EqualFold(strings.TrimSpace(server.SessionPolicy), sessionPolicyLongLived) {
		launchCtx = context.WithoutCancel(ctx)
	}
	cmd := exec.CommandContext(launchCtx, command, args...)
	if dir := strings.TrimSpace(request.WorkspaceDeliverableDirectory); dir != "" {
		cmd.Dir = dir
	}
	if len(env) > 0 {
		cmd.Env = mergeEnv(os.Environ(), env)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	session := &stdioSession{
		serverID: serverID,
		cmd:      cmd,
		stdin:    stdin,
		reader:   bufio.NewReader(stdout),
		stderr:   &limitedBuffer{limit: 64 << 10},
		done:     make(chan error, 1),
		lastUsed: time.Now().UTC(),
	}
	go func() {
		_, _ = io.Copy(session.stderr, stderr)
	}()
	go func() {
		session.done <- cmd.Wait()
	}()

	initializeRaw, err := json.Marshal(struct {
		ProtocolVersion string   `json:"protocolVersion"`
		Capabilities    struct{} `json:"capabilities"`
		ClientInfo      struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"clientInfo"`
	}{
		ProtocolVersion: mcpProtocolVersion,
		ClientInfo: struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}{Name: mcpClientName, Version: mcpClientVersion},
	})
	if err != nil {
		session.Close()
		return nil, err
	}
	initializeParams := json.RawMessage(initializeRaw)
	if err := writeJSONRPCLine(stdin, jsonRPCRequest{JSONRPC: "2.0", ID: 1, Method: "initialize", Params: initializeParams}); err != nil {
		session.Close()
		return nil, err
	}
	if _, _, err := readJSONRPCResponseWithContext(initCtx, session.reader, 1, session); err != nil {
		session.Close()
		return nil, err
	}
	initializedParams := json.RawMessage("{}")
	if err := writeJSONRPCLine(stdin, jsonRPCRequest{JSONRPC: "2.0", Method: "notifications/initialized", Params: initializedParams}); err != nil {
		session.Close()
		return nil, err
	}
	return session, nil
}

func (s *stdioSession) Execute(ctx context.Context, spec operationSpec, id int) (jsonRPCResponse, []MCPStreamEvent, error) {
	if s == nil {
		return jsonRPCResponse{}, nil, common.ErrInvalidArgument
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	request := jsonRPCRequest{JSONRPC: "2.0", ID: id}
	var params json.RawMessage
	var err error
	switch spec.Operation {
	case blockcommon.StepKindToolCall:
		arguments, encodeErr := mcpArgumentsJSON(spec.Input)
		if encodeErr != nil {
			return jsonRPCResponse{}, nil, encodeErr
		}
		request.Method = "tools/call"
		var raw []byte
		raw, err = json.Marshal(struct {
			Name      string                     `json:"name"`
			Arguments map[string]json.RawMessage `json:"arguments"`
		}{Name: spec.Tool, Arguments: arguments})
		params = json.RawMessage(raw)
	case blockcommon.StepKindResourceRead:
		request.Method = "resources/read"
		var raw []byte
		raw, err = json.Marshal(struct {
			URI string `json:"uri"`
		}{URI: spec.URI})
		params = json.RawMessage(raw)
	case blockcommon.StepKindPromptGet:
		arguments, encodeErr := mcpArgumentsJSON(spec.Arguments)
		if encodeErr != nil {
			return jsonRPCResponse{}, nil, encodeErr
		}
		request.Method = "prompts/get"
		var raw []byte
		raw, err = json.Marshal(struct {
			Name      string                     `json:"name"`
			Arguments map[string]json.RawMessage `json:"arguments"`
		}{Name: spec.Prompt, Arguments: arguments})
		params = json.RawMessage(raw)
	default:
		return jsonRPCResponse{}, nil, common.ErrInvalidArgument
	}
	if err != nil {
		return jsonRPCResponse{}, nil, err
	}
	request.Params = params
	if err := writeJSONRPCLine(s.stdin, request); err != nil {
		return jsonRPCResponse{}, nil, err
	}
	response, streamEvents, err := readJSONRPCResponseWithContext(ctx, s.reader, id, s)
	s.lastUsed = time.Now().UTC()
	return response, streamEvents, err
}

func (s *stdioSession) Close() string {
	if s == nil {
		return ""
	}
	s.closeOnce.Do(func() {
		_ = s.stdin.Close()
		select {
		case <-s.done:
		case <-time.After(2 * time.Second):
			if s.cmd != nil && s.cmd.Process != nil {
				_ = s.cmd.Process.Signal(syscall.SIGTERM)
			}
			select {
			case <-s.done:
			case <-time.After(2 * time.Second):
				if s.cmd != nil && s.cmd.Process != nil {
					_ = s.cmd.Process.Kill()
				}
				<-s.done
			}
		}
	})
	return s.Stderr()
}

func (s *stdioSession) Stderr() string {
	if s == nil || s.stderr == nil {
		return ""
	}
	return s.stderr.String()
}

func parseStdioEndpoint(endpoint string) (string, []string, error) {
	fields := strings.Fields(strings.TrimSpace(endpoint))
	if len(fields) == 0 {
		return "", nil, fmt.Errorf("%w: mcp stdio endpoint command is required", common.ErrInvalidArgument)
	}
	command := fields[0]
	if strings.ContainsAny(command, `/\`) && !filepath.IsAbs(command) {
		if abs, err := filepath.Abs(command); err == nil {
			command = abs
		}
	}
	return command, fields[1:], nil
}

func stdioLaunchConfig(server *mcpmodel.Server, request RuntimeOperationRequest) (string, []string, map[string]string, error) {
	if server == nil {
		return "", nil, nil, common.ErrInvalidArgument
	}
	if strings.TrimSpace(server.Endpoint) != "" {
		command, args, err := parseStdioEndpoint(server.Endpoint)
		return command, args, nil, err
	}
	command := strings.TrimSpace(server.Command)
	if command == "" {
		return "", nil, nil, fmt.Errorf("%w: stdio MCP server requires endpoint or command", common.ErrInvalidArgument)
	}
	env, err := resolveEnvironmentReferences(server.Env, request)
	if err != nil {
		return "", nil, nil, err
	}
	return command, expandWorkspaceStringSlice(server.Args, request), env, nil
}

func stdioSessionKey(workspaceID common.WorkspaceID, serverID string) string {
	if workspaceID == "" {
		return strings.TrimSpace(serverID)
	}
	return string(workspaceID) + "\x00" + strings.TrimSpace(serverID)
}

func mergeEnv(base []string, extra map[string]string) []string {
	values := make(map[string]string, len(base)+len(extra))
	for _, item := range base {
		key, value, ok := strings.Cut(item, "=")
		if !ok || key == "" {
			continue
		}
		values[key] = value
	}
	for key, value := range extra {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		values[key] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ret := make([]string, 0, len(keys))
	for _, key := range keys {
		ret = append(ret, key+"="+values[key])
	}
	return ret
}

func cloneStringEnv(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	ret := make(map[string]string, len(input))
	for key, value := range input {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		ret[key] = value
	}
	return ret
}

func resolveEnvironmentReferences(input map[string]string, request RuntimeOperationRequest) (map[string]string, error) {
	if input == nil {
		return nil, nil
	}
	ret := make(map[string]string, len(input))
	for key, reference := range input {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		reference = strings.TrimSpace(reference)
		if !registryevents.ValidEnvironmentReference(reference) {
			return nil, fmt.Errorf("%w: invalid MCP environment reference", common.ErrInvalidArgument)
		}
		if name, ok := strings.CutPrefix(reference, "env://"); ok {
			value, exists := os.LookupEnv(name)
			if !exists {
				return nil, fmt.Errorf("%w: MCP environment reference %q is unavailable", common.ErrNotFound, reference)
			}
			ret[key] = value
			continue
		}
		ret[key] = expandWorkspaceString(reference, request)
	}
	return ret, nil
}

func expandWorkspaceStringSlice(input []string, request RuntimeOperationRequest) []string {
	if input == nil {
		return nil
	}
	ret := make([]string, 0, len(input))
	for _, value := range input {
		ret = append(ret, expandWorkspaceString(value, request))
	}
	return ret
}

func expandWorkspaceString(value string, request RuntimeOperationRequest) string {
	value = workorchmanagedconfig.ExpandPlaceholders(value, request.WorkspaceInternalDirectory)
	replacements := map[string]string{
		"{workspaceId}":                   string(request.WorkspaceID),
		"{runId}":                         string(request.RunID),
		"{stepId}":                        string(request.StepID),
		"{workspaceDeliverableDirectory}": request.WorkspaceDeliverableDirectory,
		"{workspaceInternalDirectory}":    request.WorkspaceInternalDirectory,
	}
	for old, replacement := range replacements {
		value = strings.ReplaceAll(value, old, replacement)
	}
	return value
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b == nil || b.limit <= 0 {
		return len(p), nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	remaining := b.limit - b.buf.Len()
	if remaining > 0 {
		if len(p) < remaining {
			remaining = len(p)
		}
		_, _ = b.buf.Write(p[:remaining])
	}
	return len(p), nil
}

func (b *limitedBuffer) String() string {
	if b == nil {
		return ""
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.TrimSpace(b.buf.String())
}
