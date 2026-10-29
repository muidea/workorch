package mcp

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	workspacecommon "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/common"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/events"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type preparationObservationCache struct {
	sync.Mutex
	epoch   string
	results map[string]mcpevents.ObservePreparationResult
}

func (m *Biz) handlePreparationObservation(ev magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	if ev == nil || ev.Source() != workspacecommon.ModuleID {
		result.Set(nil, cd.WrapError(cd.IllegalParam, common.ErrInvalidArgument, "invalid preparation observation"))
		return
	}
	c, ok := ev.Data().(mcpevents.ObservePreparationCommand)
	if !ok || c.Validate() != nil || !filepath.IsAbs(c.InternalDirectory) || !filepath.IsAbs(c.DeliverableDirectory) {
		result.Set(nil, cd.WrapError(cd.IllegalParam, common.ErrInvalidArgument, "invalid preparation observation"))
		return
	}
	// Bounded owner-local duplicate suppression. Workspace persists acceptance
	// before this ingress and never replays it during recovery or after restart.
	key := string(c.WorkspaceID) + "\x00" + c.RequestID
	m.preparation.Lock()
	defer m.preparation.Unlock()
	if old, ok := m.preparation.results[key]; ok {
		if !reflect.DeepEqual(old.Command, c) {
			result.Set(nil, cd.WrapError(cd.Unexpected, common.ErrConflict, "preparation observation unavailable"))
		} else {
			result.Set(old.Clone(), nil)
		}
		return
	}
	if len(m.preparation.results) >= 128 {
		result.Set(nil, cd.WrapError(cd.Unexpected, common.ErrConflict, "preparation observation unavailable"))
		return
	}
	ctx, cancel := context.WithTimeout(ev.Context(), 10*time.Second)
	defer cancel()
	if m.preparation.epoch == "" {
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			result.Set(nil, cd.WrapError(cd.Unexpected, err, "preparation epoch unavailable"))
			return
		}
		m.preparation.epoch = "mcp-preparation-epoch:" + hex.EncodeToString(random[:])
	}
	r := mcpevents.ObservePreparationResult{OwnerEpoch: m.preparation.epoch, Command: c, StartedAt: time.Now().UTC()}
	getter, ok := m.runtimeAdapter.service.(registrySnapshotGetter)
	if !ok {
		result.Set(nil, cd.WrapError(cd.Unexpected, common.ErrConflict, "preparation observation unavailable"))
		return
	}
	snapshot, rev, err := getter.RegistrySnapshot(ctx)
	if err != nil || !preparationRegistryMatches(snapshot, rev, c) {
		result.Set(nil, cd.WrapError(cd.Unexpected, common.ErrConflict, "preparation observation unavailable"))
		return
	}
	for _, id := range c.ServerIDs {
		for _, server := range snapshot.Servers {
			if string(server.ID) == id {
				r.Observations = append(r.Observations, observePreparationStdio(ctx, c, server))
			}
		}
	}
	snapshot, rev, err = getter.RegistrySnapshot(ctx)
	r.RegistryStable = err == nil && preparationRegistryMatches(snapshot, rev, c)
	r.CompletedAt = time.Now().UTC()
	if r.ValidateFor(c) != nil {
		result.Set(nil, cd.WrapError(cd.Unexpected, common.ErrConflict, "preparation observation result invalid"))
		return
	}
	if m.preparation.results == nil {
		m.preparation.results = map[string]mcpevents.ObservePreparationResult{}
	}
	m.preparation.results[key] = r.Clone()
	result.Set(r.Clone(), nil)
}
func preparationRegistryMatches(s *mcpmodel.RegistrySnapshot, rev string, c mcpevents.ObservePreparationCommand) bool {
	if s == nil || rev != c.RegistryRevision {
		return false
	}
	for _, id := range c.ServerIDs {
		found := false
		for _, a := range s.Servers {
			if a != nil && string(a.ID) == id && a.Enabled {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	for _, v := range c.Capabilities {
		found := false
		for _, a := range s.Capabilities {
			if a != nil && a.Enabled && string(a.ID) == v.ID && string(a.ServerID) == v.ServerID && a.Kind == v.Kind && a.Name == v.Name && a.URI == v.URI {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// Exact bounded stderr. Oversize is explicit and cannot become success.
type preparationBuffer struct {
	sync.Mutex
	data     []byte
	overflow bool
}

func (b *preparationBuffer) Write(p []byte) (int, error) {
	b.Lock()
	defer b.Unlock()
	n := len(p)
	remaining := mcpevents.PreparationRawLimit - len(b.data)
	if len(p) > remaining {
		b.overflow = true
		p = p[:remaining]
	}
	b.data = append(b.data, p...)
	return n, nil
}

func observePreparationStdio(ctx context.Context, c mcpevents.ObservePreparationCommand, s *mcpmodel.Server) (o mcpevents.PreparationServerObservation) {
	o.ServerID = string(s.ID)
	o.StartedAt = time.Now().UTC()
	defer func() { o.CompletedAt = time.Now().UTC() }()
	if s.Transport != "stdio" {
		o.FailureCode = "transport_unsupported"
		return
	}
	command, args, env, err := stdioLaunchConfig(s, RuntimeOperationRequest{WorkspaceID: c.WorkspaceID, WorkspaceInternalDirectory: c.InternalDirectory, WorkspaceDeliverableDirectory: c.DeliverableDirectory})
	if err != nil {
		o.FailureCode = "launch_failed"
		return
	}
	bounded, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(bounded, command, args...)
	// A configured launcher may leave inherited pipes open after its exit.
	// Keep both protocol reads and Wait's stderr copy within this operation.
	cmd.WaitDelay = 500 * time.Millisecond
	cmd.Dir = c.DeliverableDirectory
	if len(env) > 0 {
		cmd.Env = mergeEnv(os.Environ(), env)
	}
	input, err := cmd.StdinPipe()
	if err != nil {
		o.FailureCode = "launch_failed"
		return
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		o.FailureCode = "launch_failed"
		return
	}
	stopPipeCancellation := context.AfterFunc(bounded, func() { _ = input.Close(); _ = output.Close() })
	defer stopPipeCancellation()
	defer input.Close()
	defer output.Close()
	diagnostics := &preparationBuffer{}
	cmd.Stderr = diagnostics
	if err = cmd.Start(); err != nil {
		o.FailureCode = "launch_failed"
		return
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		o.Closed = true
		o.FailureCode = "launch_failed"
		return
	}
	o.SessionID = hex.EncodeToString(random[:])
	o.ProcessID = cmd.Process.Pid
	// Read one bounded raw line, including its original newline; no trimming,
	// redaction or summary replaces the original protocol reply.
	reader := bufio.NewReader(io.LimitReader(output, mcpevents.PreparationRawLimit+1))
	used := 0
	exchange := func(method string, params json.RawMessage) bool {
		id := len(o.Frames) + 1
		request, _ := json.Marshal(jsonRPCRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params})
		request = append(request, '\n')
		frame := mcpevents.PreparationProtocolFrame{Request: request}
		if _, err := input.Write(request); err != nil {
			o.FailureCode = "protocol_failed"
			o.Frames = append(o.Frames, frame)
			return false
		}
		raw, err := reader.ReadBytes('\n')
		used += len(raw) + len(request)
		if used > mcpevents.PreparationRawLimit/2 {
			o.FailureCode = "output_limit"
			return false
		}
		frame.Response = raw
		o.Frames = append(o.Frames, frame)
		var reply struct {
			ID    int             `json:"id"`
			Error json.RawMessage `json:"error"`
		}
		if err != nil || json.Unmarshal(raw, &reply) != nil || reply.ID != id || (len(reply.Error) > 0 && string(reply.Error) != "null") {
			o.FailureCode = "protocol_failed"
			return false
		}
		return true
	}
	init := json.RawMessage(`{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"workorch-preparation","version":"1"}}`)
	good := exchange("initialize", init)
	if good {
		_, err = input.Write([]byte("{\"jsonrpc\":\"2.0\",\"method\":\"notifications/initialized\",\"params\":{}}\n"))
		good = err == nil
		if !good {
			o.FailureCode = "protocol_failed"
		}
	}
	for _, kind := range []string{"tool", "resource", "prompt"} {
		required := false
		for _, v := range c.Capabilities {
			if v.ServerID == string(s.ID) && v.Kind == kind {
				required = true
			}
		}
		if good && required {
			method := map[string]string{"tool": "tools/list", "resource": "resources/list", "prompt": "prompts/list"}[kind]
			good = exchange(method, json.RawMessage(`{}`))
		}
	}
	_ = input.Close()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err = <-done:
	case <-time.After(500 * time.Millisecond):
		_ = cmd.Process.Kill()
		err = <-done
	}
	o.Closed = true
	diagnostics.Lock()
	o.Stderr = bytes.Clone(diagnostics.data)
	o.Complete = !diagnostics.overflow && o.FailureCode != "output_limit"
	diagnostics.Unlock()
	if !o.Complete {
		o.FailureCode = "output_limit"
	} else if err != nil && o.FailureCode == "" {
		o.FailureCode = "close_failed"
	}
	return
}

// A process-local original receipt query never launches a process. After owner
// recreation an absent original is unknown, not a failed/executed/empty result.
func (m *Biz) handlePreparationQuery(ev magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	if ev == nil || ev.Source() != workspacecommon.ModuleID {
		result.Set(nil, cd.WrapError(cd.IllegalParam, common.ErrInvalidArgument, "invalid preparation query source"))
		return
	}
	c, ok := ev.Data().(mcpevents.QueryPreparationCommand)
	if !ok || c.WorkspaceID == "" || c.RequestID == "" || c.PayloadHash == "" {
		result.Set(nil, cd.WrapError(cd.IllegalParam, common.ErrInvalidArgument, "invalid preparation query"))
		return
	}
	m.preparation.Lock()
	defer m.preparation.Unlock()
	r, ok := m.preparation.results[string(c.WorkspaceID)+"\x00"+c.RequestID]
	if !ok {
		result.Set(mcpevents.QueryPreparationResult{State: "unknown"}, nil)
		return
	}
	if r.Command.PayloadHash != c.PayloadHash {
		result.Set(nil, cd.WrapError(cd.IllegalParam, common.ErrConflict, "preparation identity conflict"))
		return
	}
	copy := r.Clone()
	result.Set(mcpevents.QueryPreparationResult{State: "recorded", Observation: &copy}, nil)
}
