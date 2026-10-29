package events

import (
	"encoding/json"
	"github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/protocol"
	"github.com/muidea/workorch/internal/pkg/common"
	"reflect"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

const TopicObservePreparation = "/mcp/preparation/observe"

// V1 retains its original exact-version evaluation for immutable old attempts.
const PreparationObservationVersionV1 = "workorch-mcp-preparation-observation/v1"
const PreparationObservationVersion = "workorch-mcp-preparation-observation/v2"
const PreparationMaxServers = 8
const PreparationRawLimit = 512 << 10

// An accepted Workspace operation permits protocol initialize/list/close only.
// It contains no Run, dispatch, tool arguments or execution authorization.
type ObservePreparationCommand struct {
	Version                                 string
	WorkspaceID                             common.WorkspaceID
	RequestID, PayloadHash, SnapshotHash    string
	WorkspaceRevision                       uint64
	InternalDirectory, DeliverableDirectory string
	RegistryRevision                        string
	ServerIDs                               []string
	Capabilities                            []PreparationCapability
}
type PreparationCapability struct{ ID, ServerID, Kind, Name, URI string }
type PreparationProtocolFrame struct{ Request, Response []byte }
type PreparationServerObservation struct {
	ServerID               string
	SessionID              string
	ProcessID              int
	StartedAt, CompletedAt time.Time
	Frames                 []PreparationProtocolFrame
	Stderr                 []byte
	Closed, Complete       bool
	FailureCode            string
}
type ObservePreparationResult struct {
	OwnerEpoch             string
	Command                ObservePreparationCommand
	StartedAt, CompletedAt time.Time
	Observations           []PreparationServerObservation
	RegistryStable         bool
}

func (c ObservePreparationCommand) Validate() error {
	if (c.Version != PreparationObservationVersion && c.Version != PreparationObservationVersionV1) || c.WorkspaceID == "" || c.WorkspaceRevision == 0 || c.RequestID == "" || c.PayloadHash == "" || c.SnapshotHash == "" || c.InternalDirectory == "" || c.DeliverableDirectory == "" || !utf8.ValidString(c.InternalDirectory) || !utf8.ValidString(c.DeliverableDirectory) || strings.ContainsRune(c.InternalDirectory, 0) || strings.ContainsRune(c.DeliverableDirectory, 0) || len(c.ServerIDs) == 0 || len(c.ServerIDs) > PreparationMaxServers || c.RegistryRevision == "" || len(c.Capabilities) > 4096 {
		return common.ErrInvalidArgument
	}
	seen := map[string]bool{}
	for _, id := range c.ServerIDs {
		if id == "" || seen[id] {
			return common.ErrInvalidArgument
		}
		seen[id] = true
	}
	caps := map[string]bool{}
	for _, cap := range c.Capabilities {
		if cap.ID == "" || cap.Name == "" || !seen[cap.ServerID] || caps[cap.ID] || (cap.Kind != "tool" && cap.Kind != "resource" && cap.Kind != "prompt") {
			return common.ErrInvalidArgument
		}
		caps[cap.ID] = true
	}
	return nil
}
func (r ObservePreparationResult) Clone() ObservePreparationResult {
	r.Command.ServerIDs = slices.Clone(r.Command.ServerIDs)
	r.Command.Capabilities = slices.Clone(r.Command.Capabilities)
	r.Observations = slices.Clone(r.Observations)
	for i := range r.Observations {
		r.Observations[i].Stderr = slices.Clone(r.Observations[i].Stderr)
		r.Observations[i].Frames = slices.Clone(r.Observations[i].Frames)
		for j := range r.Observations[i].Frames {
			f := &r.Observations[i].Frames[j]
			f.Request = slices.Clone(f.Request)
			f.Response = slices.Clone(f.Response)
		}
	}
	return r
}
func (r ObservePreparationResult) ValidateFor(c ObservePreparationCommand) error {
	if len(r.OwnerEpoch) != len("mcp-preparation-epoch:")+32 || !strings.HasPrefix(r.OwnerEpoch, "mcp-preparation-epoch:") || c.Validate() != nil || !reflect.DeepEqual(r.Command, c) || r.StartedAt.IsZero() || r.CompletedAt.Before(r.StartedAt) || len(r.Observations) > len(c.ServerIDs) {
		return common.ErrConflict
	}
	seen := map[string]bool{}
	for _, o := range r.Observations {
		found := false
		for _, id := range c.ServerIDs {
			if id == o.ServerID {
				found = true
			}
		}
		if !found || seen[o.ServerID] || o.StartedAt.Before(r.StartedAt) || o.CompletedAt.Before(o.StartedAt) || o.CompletedAt.After(r.CompletedAt) || len(o.Frames) > 26 || len(o.Stderr) > PreparationRawLimit {
			return common.ErrConflict
		}
		seen[o.ServerID] = true
		size := len(o.Stderr)
		for _, f := range o.Frames {
			size += len(f.Request) + len(f.Response)
		}
		if size > PreparationRawLimit {
			return common.ErrConflict
		}
		switch o.FailureCode {
		case "", "transport_unsupported", "launch_failed", "protocol_failed", "output_limit", "close_failed", "registry_changed", "protocol_version_unsupported":
		default:
			return common.ErrConflict
		}
	}
	return nil
}

// Reproduce capability presence from original protocol replies, never a PASS.
func (r ObservePreparationResult) Satisfied() bool {
	if r.ValidateFor(r.Command) != nil || !r.RegistryStable || len(r.Observations) != len(r.Command.ServerIDs) {
		return false
	}
	for _, o := range r.Observations {
		if o.ProcessID <= 0 || len(o.SessionID) != 32 || !o.Closed || !o.Complete || o.FailureCode != "" || len(o.Frames) < 1 {
			return false
		}
		found := map[string][]string{}
		for i, f := range o.Frames {
			var q struct {
				JSONRPC string          `json:"jsonrpc"`
				ID      int             `json:"id"`
				Method  string          `json:"method"`
				Params  json.RawMessage `json:"params"`
			}
			var p struct {
				JSONRPC string          `json:"jsonrpc"`
				ID      int             `json:"id"`
				Error   json.RawMessage `json:"error"`
				Result  struct {
					ProtocolVersion string `json:"protocolVersion"`
					Tools           []struct {
						Name string `json:"name"`
					} `json:"tools"`
					Resources []struct {
						URI string `json:"uri"`
					} `json:"resources"`
					Prompts []struct {
						Name string `json:"name"`
					} `json:"prompts"`
					NextCursor string `json:"nextCursor"`
				} `json:"result"`
			}
			if json.Unmarshal(f.Request, &q) != nil || json.Unmarshal(f.Response, &p) != nil || q.JSONRPC != "2.0" || p.JSONRPC != "2.0" || q.ID != i+1 || p.ID != q.ID || (len(p.Error) > 0 && string(p.Error) != "null") {
				return false
			}
			if i == 0 {
				if q.Method != "initialize" {
					return false
				}
				if r.Command.Version == PreparationObservationVersionV1 {
					if p.Result.ProtocolVersion != "2025-11-25" {
						return false
					}
				} else {
					var request struct {
						ProtocolVersion string `json:"protocolVersion"`
					}
					var response struct {
						Result json.RawMessage `json:"result"`
						Method string          `json:"method"`
					}
					if json.Unmarshal(q.Params, &request) != nil || request.ProtocolVersion != protocol.PreferredVersion || json.Unmarshal(f.Response, &response) != nil || response.Method != "" {
						return false
					}
					if _, err := protocol.NegotiatedVersion(response.Result); err != nil {
						return false
					}
				}
				continue
			}
			switch q.Method {
			case "tools/list":
				for _, v := range p.Result.Tools {
					found["tool"] = append(found["tool"], v.Name)
				}
			case "resources/list":
				for _, v := range p.Result.Resources {
					found["resource"] = append(found["resource"], v.URI)
				}
			case "prompts/list":
				for _, v := range p.Result.Prompts {
					found["prompt"] = append(found["prompt"], v.Name)
				}
			default:
				return false
			}
			// Both versions require complete inventories; pagination stays unsupported.
			if strings.TrimSpace(p.Result.NextCursor) != "" {
				return false
			}
		}
		for _, cap := range r.Command.Capabilities {
			if cap.ServerID == o.ServerID {
				name := cap.Name
				if cap.Kind == "resource" {
					name = cap.URI
					if name == "" {
						name = cap.Name
					}
				}
				if !slices.Contains(found[cap.Kind], name) {
					return false
				}
			}
		}
	}
	return true
}

const TopicQueryPreparation = "/mcp/preparation/query-original"

type QueryPreparationCommand struct {
	WorkspaceID            common.WorkspaceID
	RequestID, PayloadHash string
}
type QueryPreparationResult struct {
	State       string
	Observation *ObservePreparationResult
}
