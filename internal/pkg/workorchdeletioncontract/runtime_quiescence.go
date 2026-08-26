package workorchdeletioncontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// WorkspaceRuntimeQuiescenceRun is an immutable Run-owner snapshot observed
// after Orchestrator's durable quiescing gate has stopped new Runtime
// authority. Run remains the owner of every lifecycle and opaque persistence
// revision carried here.
type WorkspaceRuntimeQuiescenceRun struct {
	RunID                  common.RunID
	WorkspaceRunGeneration uint64
	RunRevision            uint64
	RunState               common.RunState
	RuntimeStatePresent    bool
	RuntimeStateRevision   uint64
	RuntimeStateRef        string
	RuntimeStateHash       string
	TerminalDeliveryID     string
	TerminalDeliveryAcked  bool
}

func (r WorkspaceRuntimeQuiescenceRun) Normalized() WorkspaceRuntimeQuiescenceRun {
	r.RunID = common.RunID(strings.TrimSpace(string(r.RunID)))
	r.RuntimeStateRef = strings.TrimSpace(r.RuntimeStateRef)
	r.RuntimeStateHash = strings.TrimSpace(r.RuntimeStateHash)
	r.TerminalDeliveryID = strings.TrimSpace(r.TerminalDeliveryID)
	return r
}

func (r WorkspaceRuntimeQuiescenceRun) Validate(workspaceID common.WorkspaceID) error {
	r = r.Normalized()
	if workspaceID == "" || r.RunID == "" || r.WorkspaceRunGeneration == 0 || r.RunRevision == 0 || !runtimeQuiescenceTerminalRunState(r.RunState) {
		return common.ErrInvalidArgument
	}
	if !r.RuntimeStatePresent {
		if r.RuntimeStateRef != "" || r.RuntimeStateHash != "" || r.TerminalDeliveryID != "" || r.TerminalDeliveryAcked {
			return common.ErrInvalidArgument
		}
		if r.RuntimeStateRevision != 0 {
			return common.ErrInvalidArgument
		}
		return nil
	}
	if r.RuntimeStateRevision == 0 {
		return common.ErrInvalidArgument
	}
	expectedRef := fmt.Sprintf("runtime-state/%s/%s/%d", workspaceID, r.RunID, r.RuntimeStateRevision)
	if r.RuntimeStateRef != expectedRef || r.RuntimeStateHash == "" || r.TerminalDeliveryID == "" || !r.TerminalDeliveryAcked {
		return common.ErrInvalidArgument
	}
	return nil
}

// WorkspaceRuntimeQuiescenceToken is a stateless cross-owner fact. It cannot
// select either Run's draining/retired phase or Orchestrator's private phase;
// owners compare it with their own exact persisted snapshot before reducing.
type WorkspaceRuntimeQuiescenceToken struct {
	WorkspaceID                    common.WorkspaceID
	DeletionID                     string
	DeletionGeneration             uint64
	DataDispositionIntent          Mode
	WorkspaceIntentHash            string
	RunDrainingRevision            uint64
	WorkspaceRunGeneration         uint64
	OrchestratorQuiescenceRevision uint64
	Runs                           []WorkspaceRuntimeQuiescenceRun
	RuntimeInventoryHash           string
	QuiescenceID                   string
}

func (t WorkspaceRuntimeQuiescenceToken) Normalized() WorkspaceRuntimeQuiescenceToken {
	t.WorkspaceID = common.WorkspaceID(strings.TrimSpace(string(t.WorkspaceID)))
	t.DeletionID = strings.TrimSpace(t.DeletionID)
	t.DataDispositionIntent = Mode(strings.TrimSpace(string(t.DataDispositionIntent)))
	t.WorkspaceIntentHash = strings.TrimSpace(t.WorkspaceIntentHash)
	t.RuntimeInventoryHash = strings.TrimSpace(t.RuntimeInventoryHash)
	t.QuiescenceID = strings.TrimSpace(t.QuiescenceID)
	t.Runs = append([]WorkspaceRuntimeQuiescenceRun(nil), t.Runs...)
	for idx := range t.Runs {
		t.Runs[idx] = t.Runs[idx].Normalized()
	}
	sort.Slice(t.Runs, func(i, j int) bool {
		if t.Runs[i].WorkspaceRunGeneration != t.Runs[j].WorkspaceRunGeneration {
			return t.Runs[i].WorkspaceRunGeneration < t.Runs[j].WorkspaceRunGeneration
		}
		return t.Runs[i].RunID < t.Runs[j].RunID
	})
	return t
}

func (t WorkspaceRuntimeQuiescenceToken) Validate() error {
	t = t.Normalized()
	if t.WorkspaceID == "" || t.DeletionID == "" || t.DeletionGeneration != 1 || t.RunDrainingRevision == 0 || t.WorkspaceIntentHash == "" || t.OrchestratorQuiescenceRevision != 2 {
		return common.ErrInvalidArgument
	}
	switch t.DataDispositionIntent {
	case ModeRetain, ModePurge:
	default:
		return common.ErrInvalidArgument
	}
	previousGeneration := uint64(0)
	seen := map[common.RunID]struct{}{}
	for _, item := range t.Runs {
		if err := item.Validate(t.WorkspaceID); err != nil || item.WorkspaceRunGeneration <= previousGeneration || item.WorkspaceRunGeneration > t.WorkspaceRunGeneration {
			return common.ErrConflict
		}
		if _, exists := seen[item.RunID]; exists {
			return common.ErrConflict
		}
		seen[item.RunID] = struct{}{}
		previousGeneration = item.WorkspaceRunGeneration
	}
	if t.RuntimeInventoryHash != StableWorkspaceRuntimeInventoryHash(t) || t.QuiescenceID != StableWorkspaceRuntimeQuiescenceID(t) {
		return common.ErrConflict
	}
	return nil
}

type WorkspaceRuntimeQuiescenceIntent struct {
	WorkspaceID            common.WorkspaceID
	DeletionID             string
	DeletionGeneration     uint64
	DataDispositionIntent  Mode
	WorkspaceIntentHash    string
	RunDrainingRevision    uint64
	WorkspaceRunGeneration uint64
}

func (i WorkspaceRuntimeQuiescenceIntent) Normalized() WorkspaceRuntimeQuiescenceIntent {
	i.WorkspaceID = common.WorkspaceID(strings.TrimSpace(string(i.WorkspaceID)))
	i.DeletionID = strings.TrimSpace(i.DeletionID)
	i.DataDispositionIntent = Mode(strings.TrimSpace(string(i.DataDispositionIntent)))
	i.WorkspaceIntentHash = strings.TrimSpace(i.WorkspaceIntentHash)
	return i
}

func (i WorkspaceRuntimeQuiescenceIntent) Validate() error {
	i = i.Normalized()
	if i.WorkspaceID == "" || i.DeletionID == "" || i.DeletionGeneration != 1 || i.RunDrainingRevision == 0 || i.WorkspaceIntentHash == "" {
		return common.ErrInvalidArgument
	}
	switch i.DataDispositionIntent {
	case ModeRetain, ModePurge:
		return nil
	default:
		return common.ErrInvalidArgument
	}
}

func (t WorkspaceRuntimeQuiescenceToken) ValidateFor(intent WorkspaceRuntimeQuiescenceIntent) error {
	intent = intent.Normalized()
	if err := intent.Validate(); err != nil || t.Validate() != nil {
		return common.ErrInvalidArgument
	}
	t = t.Normalized()
	if t.WorkspaceID != intent.WorkspaceID || t.DeletionID != intent.DeletionID || t.DeletionGeneration != intent.DeletionGeneration ||
		t.DataDispositionIntent != intent.DataDispositionIntent || t.WorkspaceIntentHash != intent.WorkspaceIntentHash ||
		t.RunDrainingRevision != intent.RunDrainingRevision || t.WorkspaceRunGeneration != intent.WorkspaceRunGeneration {
		return common.ErrConflict
	}
	return nil
}

func CloneWorkspaceRuntimeQuiescenceToken(t WorkspaceRuntimeQuiescenceToken) WorkspaceRuntimeQuiescenceToken {
	return t.Normalized()
}

func StableWorkspaceRuntimeInventoryHash(t WorkspaceRuntimeQuiescenceToken) string {
	t = t.Normalized()
	material := fmt.Sprintf("workspace-runtime-inventory\x00%s\x00%d", t.WorkspaceID, t.WorkspaceRunGeneration)
	for _, item := range t.Runs {
		material += fmt.Sprintf("\x00%s\x00%d\x00%d\x00%s\x00%t\x00%d\x00%s\x00%s\x00%s\x00%t", item.RunID, item.WorkspaceRunGeneration, item.RunRevision, item.RunState, item.RuntimeStatePresent, item.RuntimeStateRevision, item.RuntimeStateRef, item.RuntimeStateHash, item.TerminalDeliveryID, item.TerminalDeliveryAcked)
	}
	digest := sha256.Sum256([]byte(material))
	return "workspace-runtime-inventory-sha256:" + hex.EncodeToString(digest[:])
}

func StableWorkspaceRuntimeQuiescenceID(t WorkspaceRuntimeQuiescenceToken) string {
	t = t.Normalized()
	material := fmt.Sprintf("workspace-runtime-quiescence\x00%s\x00%s\x00%d\x00%s\x00%s\x00%d\x00%d\x00%d\x00%s", t.WorkspaceID, t.DeletionID, t.DeletionGeneration, t.DataDispositionIntent, t.WorkspaceIntentHash, t.RunDrainingRevision, t.WorkspaceRunGeneration, t.OrchestratorQuiescenceRevision, t.RuntimeInventoryHash)
	digest := sha256.Sum256([]byte(material))
	return "workspace-runtime-quiescence-sha256:" + hex.EncodeToString(digest[:])
}

func runtimeQuiescenceTerminalRunState(state common.RunState) bool {
	switch state {
	case common.RunStateCompleted, common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		return true
	default:
		return false
	}
}
