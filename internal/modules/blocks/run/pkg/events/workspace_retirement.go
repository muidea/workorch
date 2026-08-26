package events

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

// ErrWorkspaceRetirementBusy is the only retryable retirement-admission
// outcome. Invariant corruption, scope drift, and a different durable
// DeletionID remain ordinary conflicts and must not be treated as waiting.
var ErrWorkspaceRetirementBusy = runmodel.ErrWorkspaceRetirementBusy

// RunWorkspaceRetirementFence is the immutable cross-owner projection of the
// Run owner's permanent Workspace retirement head. It carries no repository
// mutation surface and cannot be used to select an SM-1 transition.
type RunWorkspaceRetirementFence struct {
	WorkspaceID            common.WorkspaceID
	DeletionID             string
	DeletionGeneration     uint64
	DataDispositionIntent  deletioncontract.Mode
	WorkspaceIntentHash    string
	Phase                  runmodel.WorkspaceRetirementPhase
	FenceRevision          uint64
	WorkspaceRunGeneration uint64
	QuiescenceID           string
	RuntimeInventoryHash   string
}

func (f RunWorkspaceRetirementFence) Normalized() RunWorkspaceRetirementFence {
	f.WorkspaceID = common.WorkspaceID(strings.TrimSpace(string(f.WorkspaceID)))
	f.DeletionID = strings.TrimSpace(f.DeletionID)
	f.DataDispositionIntent = deletioncontract.Mode(strings.TrimSpace(string(f.DataDispositionIntent)))
	f.WorkspaceIntentHash = strings.TrimSpace(f.WorkspaceIntentHash)
	f.Phase = runmodel.WorkspaceRetirementPhase(strings.TrimSpace(string(f.Phase)))
	f.QuiescenceID = strings.TrimSpace(f.QuiescenceID)
	f.RuntimeInventoryHash = strings.TrimSpace(f.RuntimeInventoryHash)
	return f
}

func (f RunWorkspaceRetirementFence) Validate() error {
	f = f.Normalized()
	return runWorkspaceRetirementFenceToModel(f).Validate()
}

func SameRunWorkspaceRetirementFence(left, right RunWorkspaceRetirementFence) bool {
	left = left.Normalized()
	right = right.Normalized()
	return left == right
}

type PrepareWorkspaceRetirementCommand struct {
	WorkspaceID           common.WorkspaceID
	DeletionID            string
	DeletionGeneration    uint64
	DataDispositionIntent deletioncontract.Mode
	WorkspaceIntentHash   string
}

func (c PrepareWorkspaceRetirementCommand) Normalized() PrepareWorkspaceRetirementCommand {
	c.WorkspaceID = common.WorkspaceID(strings.TrimSpace(string(c.WorkspaceID)))
	c.DeletionID = strings.TrimSpace(c.DeletionID)
	c.DataDispositionIntent = deletioncontract.Mode(strings.TrimSpace(string(c.DataDispositionIntent)))
	c.WorkspaceIntentHash = strings.TrimSpace(c.WorkspaceIntentHash)
	return c
}

func (c PrepareWorkspaceRetirementCommand) Validate() error {
	c = c.Normalized()
	if c.WorkspaceID == "" || c.DeletionID == "" || c.DeletionGeneration != 1 {
		return common.ErrInvalidArgument
	}
	if c.WorkspaceIntentHash == "" {
		return common.ErrInvalidArgument
	}
	switch c.DataDispositionIntent {
	case deletioncontract.ModeRetain, deletioncontract.ModePurge:
		return nil
	default:
		return common.ErrInvalidArgument
	}
}

type PrepareWorkspaceRetirementResult struct {
	Fence RunWorkspaceRetirementFence
}

func (r PrepareWorkspaceRetirementResult) ValidateFor(command PrepareWorkspaceRetirementCommand) error {
	command = command.Normalized()
	if err := command.Validate(); err != nil {
		return err
	}
	if err := r.Fence.Validate(); err != nil {
		return err
	}
	if r.Fence.WorkspaceID != command.WorkspaceID || r.Fence.DeletionID != command.DeletionID ||
		r.Fence.DeletionGeneration != command.DeletionGeneration || r.Fence.DataDispositionIntent != command.DataDispositionIntent ||
		r.Fence.WorkspaceIntentHash != command.WorkspaceIntentHash {
		return common.ErrConflict
	}
	return nil
}

type QueryWorkspaceRetirementFactCommand struct {
	WorkspaceID common.WorkspaceID
	DeletionID  string
}

func (c QueryWorkspaceRetirementFactCommand) Normalized() QueryWorkspaceRetirementFactCommand {
	c.WorkspaceID = common.WorkspaceID(strings.TrimSpace(string(c.WorkspaceID)))
	c.DeletionID = strings.TrimSpace(c.DeletionID)
	return c
}

func (c QueryWorkspaceRetirementFactCommand) Validate() error {
	c = c.Normalized()
	if c.WorkspaceID == "" {
		return common.ErrInvalidArgument
	}
	return nil
}

type RunWorkspaceRetirementRuntimeFact struct {
	RunID                  common.RunID
	WorkspaceRunGeneration uint64
	RunRevision            uint64
	RunState               common.RunState
	RuntimeStatePresent    bool
	RuntimeStateRef        string
	RuntimeStateRevision   uint64
	RuntimeStateHash       string
	TerminalDeliveryID     string
}

func (f RunWorkspaceRetirementRuntimeFact) Validate(workspaceID common.WorkspaceID) error {
	f.RuntimeStateRef = strings.TrimSpace(f.RuntimeStateRef)
	f.RuntimeStateHash = strings.TrimSpace(f.RuntimeStateHash)
	f.TerminalDeliveryID = strings.TrimSpace(f.TerminalDeliveryID)
	if workspaceID == "" || f.RunID == "" || f.WorkspaceRunGeneration == 0 || f.RunRevision == 0 || !runWorkspaceRetirementTerminalState(f.RunState) {
		return common.ErrInvalidArgument
	}
	if !f.RuntimeStatePresent {
		if f.RuntimeStateRef != "" || f.RuntimeStateRevision != 0 || f.RuntimeStateHash != "" || f.TerminalDeliveryID != "" {
			return common.ErrInvalidArgument
		}
		return nil
	}
	if f.RuntimeStateRevision == 0 || f.RuntimeStateRef != RuntimeStateRef(workspaceID, f.RunID, f.RuntimeStateRevision) || f.RuntimeStateHash == "" || f.TerminalDeliveryID == "" {
		return common.ErrInvalidArgument
	}
	return nil
}

func runWorkspaceRetirementTerminalState(state common.RunState) bool {
	switch state {
	case common.RunStateCompleted, common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		return true
	default:
		return false
	}
}

type QueryWorkspaceRetirementFactResult struct {
	Fence RunWorkspaceRetirementFence
	Runs  []RunWorkspaceRetirementRuntimeFact
}

func (r QueryWorkspaceRetirementFactResult) Clone() QueryWorkspaceRetirementFactResult {
	r.Runs = append([]RunWorkspaceRetirementRuntimeFact(nil), r.Runs...)
	return r
}

func (r QueryWorkspaceRetirementFactResult) ValidateFor(command QueryWorkspaceRetirementFactCommand) error {
	command = command.Normalized()
	if err := command.Validate(); err != nil {
		return err
	}
	if err := r.Fence.Validate(); err != nil || r.Fence.WorkspaceID != command.WorkspaceID || (command.DeletionID != "" && r.Fence.DeletionID != command.DeletionID) {
		return common.ErrConflict
	}
	previousGeneration := uint64(0)
	for _, item := range r.Runs {
		if err := item.Validate(r.Fence.WorkspaceID); err != nil || item.WorkspaceRunGeneration <= previousGeneration || item.WorkspaceRunGeneration > r.Fence.WorkspaceRunGeneration {
			return common.ErrConflict
		}
		previousGeneration = item.WorkspaceRunGeneration
	}
	return nil
}

type QueryWorkspaceRetirementInventoryCommand struct {
	Cursor string
	Limit  int
}

type QueryWorkspaceRetirementInventoryResult struct {
	Items      []RunWorkspaceRetirementFence
	NextCursor string
}

func (r QueryWorkspaceRetirementInventoryResult) Clone() QueryWorkspaceRetirementInventoryResult {
	r.Items = append([]RunWorkspaceRetirementFence(nil), r.Items...)
	return r
}

func (r QueryWorkspaceRetirementInventoryResult) Validate() error {
	if len(r.Items) > 500 {
		return common.ErrInvalidArgument
	}
	previous := ""
	for _, item := range r.Items {
		if err := item.Validate(); err != nil || (previous != "" && string(item.WorkspaceID) <= previous) {
			return common.ErrConflict
		}
		previous = string(item.WorkspaceID)
	}
	if r.NextCursor != "" && (len(r.Items) == 0 || r.NextCursor != previous) {
		return common.ErrConflict
	}
	return nil
}

type PurgeWorkspaceRunsCommand struct {
	WorkspaceID            common.WorkspaceID
	DeletionID             string
	DeletionGeneration     uint64
	DataDispositionIntent  deletioncontract.Mode
	WorkspaceIntentHash    string
	FenceRevision          uint64
	WorkspaceRunGeneration uint64
	RuntimeInventoryHash   string
	QuiescenceID           string
}

func (c PurgeWorkspaceRunsCommand) RetirementFence() RunWorkspaceRetirementFence {
	return RunWorkspaceRetirementFence{
		WorkspaceID:            c.WorkspaceID,
		DeletionID:             c.DeletionID,
		DeletionGeneration:     c.DeletionGeneration,
		DataDispositionIntent:  c.DataDispositionIntent,
		WorkspaceIntentHash:    c.WorkspaceIntentHash,
		Phase:                  runmodel.WorkspaceRetirementPhaseRetired,
		FenceRevision:          c.FenceRevision,
		WorkspaceRunGeneration: c.WorkspaceRunGeneration,
		QuiescenceID:           c.QuiescenceID,
		RuntimeInventoryHash:   c.RuntimeInventoryHash,
	}.Normalized()
}

func (c PurgeWorkspaceRunsCommand) Normalized() PurgeWorkspaceRunsCommand {
	fence := c.RetirementFence()
	c.WorkspaceID = fence.WorkspaceID
	c.DeletionID = fence.DeletionID
	c.DataDispositionIntent = fence.DataDispositionIntent
	c.WorkspaceIntentHash = fence.WorkspaceIntentHash
	c.RuntimeInventoryHash = fence.RuntimeInventoryHash
	c.QuiescenceID = strings.TrimSpace(c.QuiescenceID)
	return c
}

func (c PurgeWorkspaceRunsCommand) Validate() error {
	c = c.Normalized()
	if err := c.RetirementFence().Validate(); err != nil || c.QuiescenceID == "" || c.DataDispositionIntent != deletioncontract.ModePurge {
		return common.ErrInvalidArgument
	}
	return nil
}

type PurgeWorkspaceRunsResult struct {
	WorkspaceID            common.WorkspaceID
	DeletionID             string
	DeletionGeneration     uint64
	FenceRevision          uint64
	WorkspaceRunGeneration uint64
	QuiescenceID           string
	PurgeID                string
	AckHash                string
	PurgeProofHash         string
	Observations           []runmodel.WorkspaceRetirementPurgeObservation
}

func (r PurgeWorkspaceRunsResult) Clone() PurgeWorkspaceRunsResult {
	r.Observations = append([]runmodel.WorkspaceRetirementPurgeObservation(nil), r.Observations...)
	return r
}

func (r PurgeWorkspaceRunsResult) ValidateFor(command PurgeWorkspaceRunsCommand) error {
	command = command.Normalized()
	if err := command.Validate(); err != nil {
		return err
	}
	if r.WorkspaceID != command.WorkspaceID || r.DeletionID != command.DeletionID || r.DeletionGeneration != command.DeletionGeneration ||
		r.FenceRevision != command.FenceRevision || r.WorkspaceRunGeneration != command.WorkspaceRunGeneration ||
		r.QuiescenceID != command.QuiescenceID || r.PurgeID != StableRunWorkspacePurgeID(command) ||
		runmodel.ValidateEmptyWorkspaceRetirementPurgeObservations(r.Observations) != nil ||
		r.PurgeProofHash != StableRunWorkspacePurgeProofHashForObservations(command, r.Observations) ||
		r.AckHash != StableRunWorkspacePurgeAckHashForObservations(command, r.Observations) {
		return common.ErrConflict
	}
	return nil
}

func StableRunWorkspacePurgeID(command PurgeWorkspaceRunsCommand) string {
	return runWorkspaceRetirementHash("purge", command, "")
}

func StableRunWorkspacePurgeProofHashForObservations(command PurgeWorkspaceRunsCommand, observations []runmodel.WorkspaceRetirementPurgeObservation) string {
	return runmodel.StableWorkspaceRetirementPurgeProofHash(runWorkspaceRetirementFenceToModel(command.RetirementFence()), observations)
}

func StableRunWorkspacePurgeAckHashForObservations(command PurgeWorkspaceRunsCommand, observations []runmodel.WorkspaceRetirementPurgeObservation) string {
	return runWorkspaceRetirementHash("purge-ack", command, StableRunWorkspacePurgeProofHashForObservations(command, observations))
}

func runWorkspaceRetirementHash(kind string, command PurgeWorkspaceRunsCommand, extra string) string {
	command = command.Normalized()
	material := fmt.Sprintf("run-workspace-retirement\x00%s\x00%s\x00%s\x00%d\x00%s\x00%s\x00%d\x00%d\x00%s\x00%s\x00%s", kind, command.WorkspaceID, command.DeletionID, command.DeletionGeneration, command.DataDispositionIntent, command.WorkspaceIntentHash, command.FenceRevision, command.WorkspaceRunGeneration, command.RuntimeInventoryHash, command.QuiescenceID, extra)
	digest := sha256.Sum256([]byte(material))
	return "run-workspace-retirement-sha256:" + hex.EncodeToString(digest[:])
}

// FinalizeWorkspaceRetirementCommand carries only Orchestrator's immutable
// quiescence fact. Run compares it with its own exact draining head and Run
// records before its private reducer may choose retired.
type FinalizeWorkspaceRetirementCommand struct {
	QuiescenceToken deletioncontract.WorkspaceRuntimeQuiescenceToken
}

func (c FinalizeWorkspaceRetirementCommand) Normalized() FinalizeWorkspaceRetirementCommand {
	c.QuiescenceToken = deletioncontract.CloneWorkspaceRuntimeQuiescenceToken(c.QuiescenceToken)
	return c
}

func (c FinalizeWorkspaceRetirementCommand) Validate() error {
	return c.Normalized().QuiescenceToken.Validate()
}

type FinalizeWorkspaceRetirementResult struct {
	Fence RunWorkspaceRetirementFence
}

func (r FinalizeWorkspaceRetirementResult) ValidateFor(command FinalizeWorkspaceRetirementCommand) error {
	command = command.Normalized()
	if err := command.Validate(); err != nil || r.Fence.Validate() != nil {
		return common.ErrInvalidArgument
	}
	token := command.QuiescenceToken
	if r.Fence.WorkspaceID != token.WorkspaceID || r.Fence.DeletionID != token.DeletionID ||
		r.Fence.DeletionGeneration != token.DeletionGeneration || r.Fence.DataDispositionIntent != token.DataDispositionIntent ||
		r.Fence.WorkspaceIntentHash != token.WorkspaceIntentHash || r.Fence.Phase != runmodel.WorkspaceRetirementPhaseRetired ||
		r.Fence.FenceRevision != token.RunDrainingRevision+1 || r.Fence.WorkspaceRunGeneration != token.WorkspaceRunGeneration ||
		r.Fence.QuiescenceID != token.QuiescenceID || r.Fence.RuntimeInventoryHash != token.RuntimeInventoryHash {
		return common.ErrConflict
	}
	return nil
}

func runWorkspaceRetirementFenceToModel(f RunWorkspaceRetirementFence) runmodel.WorkspaceRetirementFence {
	f = f.Normalized()
	return runmodel.WorkspaceRetirementFence{
		WorkspaceID: f.WorkspaceID, DeletionID: f.DeletionID, DeletionGeneration: f.DeletionGeneration,
		DataDispositionIntent: f.DataDispositionIntent, WorkspaceIntentHash: f.WorkspaceIntentHash, Phase: f.Phase,
		FenceRevision: f.FenceRevision, WorkspaceRunGeneration: f.WorkspaceRunGeneration,
		QuiescenceID: f.QuiescenceID, RuntimeInventoryHash: f.RuntimeInventoryHash,
	}
}
