package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

// WorkspaceDeletionPhase is Workspace-owner private lifecycle authority. No
// cross-owner command is allowed to select one of these values directly.
type WorkspaceDeletionPhase string

const (
	WorkspaceDeletionPhaseRequested             WorkspaceDeletionPhase = "requested"
	WorkspaceDeletionPhaseWaitingForIdle        WorkspaceDeletionPhase = "waiting_for_idle"
	WorkspaceDeletionPhaseRunDraining           WorkspaceDeletionPhase = "run_draining"
	WorkspaceDeletionPhaseRuntimeQuiescent      WorkspaceDeletionPhase = "runtime_quiescent"
	WorkspaceDeletionPhaseRunRetired            WorkspaceDeletionPhase = "run_retired"
	WorkspaceDeletionPhaseOwnerSealing          WorkspaceDeletionPhase = "owner_sealing"
	WorkspaceDeletionPhaseOwnerPurging          WorkspaceDeletionPhase = "owner_purging"
	WorkspaceDeletionPhaseDirectoryCleanup      WorkspaceDeletionPhase = "directory_cleanup"
	WorkspaceDeletionPhaseEventStreamFinalizing WorkspaceDeletionPhase = "eventstream_finalizing"
	WorkspaceDeletionPhaseCompleted             WorkspaceDeletionPhase = "completed"
)

var workspaceDeletionRequiredSealOwners = []string{
	"kernel/orchestrator",
	"blocks/result",
	"blocks/memory",
	"blocks/decision",
	"blocks/approval",
	"blocks/eventstream",
}

var workspaceDeletionRequiredPurgeOwners = []string{
	"kernel/orchestrator",
	"blocks/result",
	"blocks/memory",
	"blocks/decision",
	"blocks/approval",
	"blocks/run",
	"blocks/eventstream",
}

func WorkspaceDeletionRequiredSealOwners() []string {
	return append([]string(nil), workspaceDeletionRequiredSealOwners...)
}

func WorkspaceDeletionRequiredPurgeOwners() []string {
	return append([]string(nil), workspaceDeletionRequiredPurgeOwners...)
}

type WorkspaceDeletionRunDrainingAck struct {
	FenceRevision          uint64
	WorkspaceRunGeneration uint64
}

type WorkspaceDeletionRuntimeQuiescenceAck struct {
	Token deletioncontract.WorkspaceRuntimeQuiescenceToken
}

type WorkspaceDeletionRunRetiredAck struct {
	FenceRevision          uint64
	WorkspaceRunGeneration uint64
	QuiescenceID           string
	RuntimeInventoryHash   string
}

type WorkspaceDeletionOwnerSealAck struct {
	OwnerModuleID string
	FenceID       string
	Revision      uint64
	AckHash       string
	SealedAt      time.Time
}

type WorkspaceDeletionOwnerPurgeAck struct {
	OwnerModuleID  string
	PurgeID        string
	Revision       uint64
	AckHash        string
	PurgeProofHash string
	PurgedAt       time.Time
}

type WorkspaceDeletionDirectoryCleanupAck struct {
	CleanupID string
	CleanedAt time.Time
}

// WorkspaceDeletionRecord is the sole persisted retirement/purge aggregate.
// It remains embedded in the Workspace tombstone after completion so response
// loss and fresh-process retries never need an application-owned journal.
type WorkspaceDeletionRecord struct {
	DeletionID                 string
	DeletionGeneration         uint64
	RequestID                  string
	Mode                       deletioncontract.Mode
	IntentHash                 string
	ExpectedRevision           uint64
	Phase                      WorkspaceDeletionPhase
	RunDrainingAck             *WorkspaceDeletionRunDrainingAck
	RuntimeQuiescenceAck       *WorkspaceDeletionRuntimeQuiescenceAck
	RunRetiredAck              *WorkspaceDeletionRunRetiredAck
	OwnerSealAcks              []WorkspaceDeletionOwnerSealAck
	OwnerPurgeAcks             []WorkspaceDeletionOwnerPurgeAck
	DirectoryCleanupAck        *WorkspaceDeletionDirectoryCleanupAck
	SealedDeliverableDirectory string
	SealedInternalDirectory    string
	Revision                   uint64
	RequestedAt                time.Time
	UpdatedAt                  time.Time
	CompletedAt                time.Time
	Completed                  bool
}

func (r WorkspaceDeletionRecord) Clone() WorkspaceDeletionRecord {
	ret := r
	if r.RunDrainingAck != nil {
		value := *r.RunDrainingAck
		ret.RunDrainingAck = &value
	}
	if r.RuntimeQuiescenceAck != nil {
		value := *r.RuntimeQuiescenceAck
		value.Token = deletioncontract.CloneWorkspaceRuntimeQuiescenceToken(value.Token)
		ret.RuntimeQuiescenceAck = &value
	}
	if r.RunRetiredAck != nil {
		value := *r.RunRetiredAck
		ret.RunRetiredAck = &value
	}
	ret.OwnerSealAcks = append([]WorkspaceDeletionOwnerSealAck(nil), r.OwnerSealAcks...)
	ret.OwnerPurgeAcks = append([]WorkspaceDeletionOwnerPurgeAck(nil), r.OwnerPurgeAcks...)
	if r.DirectoryCleanupAck != nil {
		value := *r.DirectoryCleanupAck
		ret.DirectoryCleanupAck = &value
	}
	return ret
}

func (r WorkspaceDeletionRecord) Validate(workspaceID common.WorkspaceID) error {
	if workspaceID == "" || strings.TrimSpace(r.DeletionID) == "" || r.DeletionGeneration != 1 ||
		strings.TrimSpace(r.RequestID) == "" || strings.TrimSpace(r.IntentHash) == "" || r.Revision == 0 ||
		r.RequestedAt.IsZero() || r.UpdatedAt.IsZero() || r.UpdatedAt.Before(r.RequestedAt) {
		return common.ErrInvalidArgument
	}
	switch r.Mode {
	case deletioncontract.ModeRetain, deletioncontract.ModePurge:
	default:
		return common.ErrInvalidArgument
	}
	switch r.Phase {
	case WorkspaceDeletionPhaseRequested, WorkspaceDeletionPhaseWaitingForIdle:
		if r.RunDrainingAck != nil || r.RuntimeQuiescenceAck != nil || r.RunRetiredAck != nil || len(r.OwnerSealAcks) != 0 || len(r.OwnerPurgeAcks) != 0 || r.DirectoryCleanupAck != nil || r.Completed || !r.CompletedAt.IsZero() {
			return common.ErrConflict
		}
	case WorkspaceDeletionPhaseRunDraining:
		if validateWorkspaceDeletionRunDraining(r.RunDrainingAck) != nil || r.RuntimeQuiescenceAck != nil || r.RunRetiredAck != nil || len(r.OwnerSealAcks) != 0 || len(r.OwnerPurgeAcks) != 0 || r.DirectoryCleanupAck != nil || r.Completed || !r.CompletedAt.IsZero() {
			return common.ErrConflict
		}
	case WorkspaceDeletionPhaseRuntimeQuiescent:
		if validateWorkspaceDeletionQuiescence(workspaceID, r, r.RunDrainingAck, r.RuntimeQuiescenceAck) != nil || r.RunRetiredAck != nil || len(r.OwnerSealAcks) != 0 || len(r.OwnerPurgeAcks) != 0 || r.DirectoryCleanupAck != nil || r.Completed || !r.CompletedAt.IsZero() {
			return common.ErrConflict
		}
	case WorkspaceDeletionPhaseRunRetired:
		if validateWorkspaceDeletionRetired(workspaceID, r, r.RunDrainingAck, r.RuntimeQuiescenceAck, r.RunRetiredAck) != nil || len(r.OwnerSealAcks) != 0 || len(r.OwnerPurgeAcks) != 0 || r.DirectoryCleanupAck != nil || r.Completed || !r.CompletedAt.IsZero() {
			return common.ErrConflict
		}
	case WorkspaceDeletionPhaseOwnerSealing, WorkspaceDeletionPhaseOwnerPurging, WorkspaceDeletionPhaseDirectoryCleanup, WorkspaceDeletionPhaseEventStreamFinalizing:
		if validateWorkspaceDeletionRetired(workspaceID, r, r.RunDrainingAck, r.RuntimeQuiescenceAck, r.RunRetiredAck) != nil || r.Completed || !r.CompletedAt.IsZero() {
			return common.ErrConflict
		}
		if r.Phase == WorkspaceDeletionPhaseOwnerSealing {
			if !workspaceDeletionAckOwnerPrefix(workspaceDeletionRequiredSealOwners[:len(workspaceDeletionRequiredSealOwners)-1], workspaceDeletionSealAckOwners(r.OwnerSealAcks)) || len(r.OwnerPurgeAcks) != 0 || r.DirectoryCleanupAck != nil {
				return common.ErrConflict
			}
		}
		if (r.Phase == WorkspaceDeletionPhaseOwnerPurging || r.Phase == WorkspaceDeletionPhaseDirectoryCleanup) && (r.Mode != deletioncontract.ModePurge || !workspaceDeletionAckOwnerSetExact(workspaceDeletionRequiredSealOwners[:len(workspaceDeletionRequiredSealOwners)-1], workspaceDeletionSealAckOwners(r.OwnerSealAcks))) {
			return common.ErrConflict
		}
		if r.Phase == WorkspaceDeletionPhaseOwnerPurging {
			if !workspaceDeletionAckOwnerPrefix(workspaceDeletionRequiredPurgeOwners[:len(workspaceDeletionRequiredPurgeOwners)-1], workspaceDeletionPurgeAckOwners(r.OwnerPurgeAcks)) || r.DirectoryCleanupAck != nil {
				return common.ErrConflict
			}
		}
		if r.Phase == WorkspaceDeletionPhaseDirectoryCleanup && (!workspaceDeletionAckOwnerSetExact(workspaceDeletionRequiredPurgeOwners[:len(workspaceDeletionRequiredPurgeOwners)-1], workspaceDeletionPurgeAckOwners(r.OwnerPurgeAcks)) || r.DirectoryCleanupAck != nil) {
			return common.ErrConflict
		}
		if r.Phase == WorkspaceDeletionPhaseEventStreamFinalizing {
			sealOwners := workspaceDeletionSealAckOwners(r.OwnerSealAcks)
			if !workspaceDeletionAckOwnerPrefix(workspaceDeletionRequiredSealOwners, sealOwners) || len(sealOwners) < len(workspaceDeletionRequiredSealOwners)-1 {
				return common.ErrConflict
			}
			if r.Mode == deletioncontract.ModeRetain {
				if len(sealOwners) != len(workspaceDeletionRequiredSealOwners)-1 || len(r.OwnerPurgeAcks) != 0 || r.DirectoryCleanupAck != nil {
					return common.ErrConflict
				}
			} else if r.DirectoryCleanupAck == nil || !workspaceDeletionAckOwnerSetExact(workspaceDeletionRequiredPurgeOwners[:len(workspaceDeletionRequiredPurgeOwners)-1], workspaceDeletionPurgeAckOwners(r.OwnerPurgeAcks)) {
				return common.ErrConflict
			}
		}
	case WorkspaceDeletionPhaseCompleted:
		if validateWorkspaceDeletionRetired(workspaceID, r, r.RunDrainingAck, r.RuntimeQuiescenceAck, r.RunRetiredAck) != nil || !r.Completed || r.CompletedAt.IsZero() || r.CompletedAt.Before(r.RequestedAt) || !r.CompletedAt.Equal(r.UpdatedAt) {
			return common.ErrConflict
		}
		if r.Mode == deletioncontract.ModeRetain {
			if !workspaceDeletionAckOwnerSetExact(workspaceDeletionRequiredSealOwners, workspaceDeletionSealAckOwners(r.OwnerSealAcks)) || len(r.OwnerPurgeAcks) != 0 || r.DirectoryCleanupAck != nil {
				return common.ErrConflict
			}
		} else {
			if !workspaceDeletionAckOwnerSetExact(workspaceDeletionRequiredSealOwners, workspaceDeletionSealAckOwners(r.OwnerSealAcks)) || !workspaceDeletionAckOwnerSetExact(workspaceDeletionRequiredPurgeOwners, workspaceDeletionPurgeAckOwners(r.OwnerPurgeAcks)) || r.DirectoryCleanupAck == nil || strings.TrimSpace(r.DirectoryCleanupAck.CleanupID) == "" || r.DirectoryCleanupAck.CleanedAt.IsZero() {
				return common.ErrConflict
			}
		}
	default:
		return common.ErrInvalidArgument
	}
	if err := validateWorkspaceDeletionOwnerAcks(r.OwnerSealAcks, r.OwnerPurgeAcks); err != nil {
		return err
	}
	for _, ack := range r.OwnerSealAcks {
		if ack.SealedAt.Before(r.RequestedAt) || ack.SealedAt.After(r.UpdatedAt) {
			return common.ErrConflict
		}
	}
	for _, ack := range r.OwnerPurgeAcks {
		if ack.PurgedAt.Before(r.RequestedAt) || ack.PurgedAt.After(r.UpdatedAt) {
			return common.ErrConflict
		}
	}
	if r.DirectoryCleanupAck != nil && (r.DirectoryCleanupAck.CleanedAt.Before(r.RequestedAt) || r.DirectoryCleanupAck.CleanedAt.After(r.UpdatedAt)) {
		return common.ErrConflict
	}
	if !workspaceDeletionAckOwnerPrefix(workspaceDeletionRequiredSealOwners, workspaceDeletionSealAckOwners(r.OwnerSealAcks)) || !workspaceDeletionAckOwnerPrefix(workspaceDeletionRequiredPurgeOwners, workspaceDeletionPurgeAckOwners(r.OwnerPurgeAcks)) {
		return fmt.Errorf("%w: Workspace deletion owner acknowledgement order mismatch", common.ErrConflict)
	}
	if r.IntentHash != StableWorkspaceDeletionIntentHash(workspaceID, r) {
		return fmt.Errorf("%w: Workspace deletion intent hash mismatch", common.ErrConflict)
	}
	return nil
}

func (r WorkspaceDeletionRecord) BlocksIngress() bool {
	// A durable deletion intent is an ingress tombstone even while the Run
	// owner reports busy. Only owner-private lifecycle reconciliation and an
	// exact delete retry may continue; reopening setup/execution could race the
	// eventual authoritative retirement fence.
	return true
}

func StableWorkspaceDeletionIntentHash(workspaceID common.WorkspaceID, r WorkspaceDeletionRecord) string {
	material := fmt.Sprintf("workspace-deletion-intent\x00%s\x00%s\x00%d\x00%s\x00%s\x00%d\x00%s\x00%s",
		strings.TrimSpace(string(workspaceID)), strings.TrimSpace(r.DeletionID), r.DeletionGeneration,
		strings.TrimSpace(r.RequestID), strings.TrimSpace(string(r.Mode)), r.ExpectedRevision,
		strings.TrimSpace(r.SealedDeliverableDirectory), strings.TrimSpace(r.SealedInternalDirectory))
	digest := sha256.Sum256([]byte(material))
	return "workspace-deletion-intent-sha256:" + hex.EncodeToString(digest[:])
}

// StableWorkspaceDeletionCleanupID binds the Workspace-owned physical cleanup
// to the exact deletion lineage, Runtime quiescence fence, and every required
// pre-Eventstream purge receipt. It is an identity function only; callers may
// not use it to select or advance the Workspace deletion phase.
func StableWorkspaceDeletionCleanupID(r WorkspaceDeletionRecord) string {
	if strings.TrimSpace(r.DeletionID) == "" || r.DeletionGeneration != 1 || r.Mode != deletioncontract.ModePurge ||
		r.RuntimeQuiescenceAck == nil || r.RuntimeQuiescenceAck.Token.Validate() != nil {
		return ""
	}
	parts := []string{"workspace-deletion-cleanup", r.DeletionID, r.RuntimeQuiescenceAck.Token.QuiescenceID}
	for _, owner := range workspaceDeletionRequiredPurgeOwners[:len(workspaceDeletionRequiredPurgeOwners)-1] {
		ack, ok := workspaceDeletionPurgeAck(r.OwnerPurgeAcks, owner)
		if !ok {
			return ""
		}
		parts = append(parts, owner, ack.PurgeID, ack.AckHash, ack.PurgeProofHash)
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "workspace-deletion-cleanup-sha256:" + hex.EncodeToString(digest[:])
}

func workspaceDeletionPurgeAck(values []WorkspaceDeletionOwnerPurgeAck, owner string) (WorkspaceDeletionOwnerPurgeAck, bool) {
	for _, item := range values {
		if item.OwnerModuleID == owner {
			return item, true
		}
	}
	return WorkspaceDeletionOwnerPurgeAck{}, false
}

func validateWorkspaceDeletionRunDraining(value *WorkspaceDeletionRunDrainingAck) error {
	if value == nil || value.FenceRevision == 0 {
		return common.ErrConflict
	}
	return nil
}

func validateWorkspaceDeletionQuiescence(workspaceID common.WorkspaceID, record WorkspaceDeletionRecord, draining *WorkspaceDeletionRunDrainingAck, value *WorkspaceDeletionRuntimeQuiescenceAck) error {
	if draining == nil || value == nil || value.Token.Validate() != nil {
		return common.ErrConflict
	}
	token := value.Token.Normalized()
	if token.WorkspaceID != workspaceID || token.DeletionID != record.DeletionID || token.DeletionGeneration != record.DeletionGeneration ||
		token.DataDispositionIntent != record.Mode || token.WorkspaceIntentHash != record.IntentHash ||
		token.RunDrainingRevision != draining.FenceRevision || token.WorkspaceRunGeneration != draining.WorkspaceRunGeneration {
		return common.ErrConflict
	}
	return nil
}

func validateWorkspaceDeletionRetired(workspaceID common.WorkspaceID, record WorkspaceDeletionRecord, draining *WorkspaceDeletionRunDrainingAck, quiescence *WorkspaceDeletionRuntimeQuiescenceAck, retired *WorkspaceDeletionRunRetiredAck) error {
	if validateWorkspaceDeletionQuiescence(workspaceID, record, draining, quiescence) != nil || retired == nil {
		return common.ErrConflict
	}
	token := quiescence.Token.Normalized()
	if retired.FenceRevision != draining.FenceRevision+1 || retired.WorkspaceRunGeneration != draining.WorkspaceRunGeneration ||
		retired.QuiescenceID != token.QuiescenceID || retired.RuntimeInventoryHash != token.RuntimeInventoryHash {
		return common.ErrConflict
	}
	return nil
}

func validateWorkspaceDeletionOwnerAcks(seals []WorkspaceDeletionOwnerSealAck, purges []WorkspaceDeletionOwnerPurgeAck) error {
	sealOwners := map[string]struct{}{}
	for _, item := range seals {
		owner := strings.TrimSpace(item.OwnerModuleID)
		if owner == "" || strings.TrimSpace(item.FenceID) == "" || item.Revision == 0 || strings.TrimSpace(item.AckHash) == "" || item.SealedAt.IsZero() {
			return common.ErrConflict
		}
		if _, duplicate := sealOwners[owner]; duplicate {
			return fmt.Errorf("%w: duplicate Workspace deletion seal owner %s", common.ErrConflict, owner)
		}
		sealOwners[owner] = struct{}{}
	}
	purgeOwners := map[string]struct{}{}
	for _, item := range purges {
		owner := strings.TrimSpace(item.OwnerModuleID)
		if owner == "" || strings.TrimSpace(item.PurgeID) == "" || item.Revision == 0 || strings.TrimSpace(item.AckHash) == "" || strings.TrimSpace(item.PurgeProofHash) == "" || item.PurgedAt.IsZero() {
			return common.ErrConflict
		}
		if _, duplicate := purgeOwners[owner]; duplicate {
			return fmt.Errorf("%w: duplicate Workspace deletion purge owner %s", common.ErrConflict, owner)
		}
		purgeOwners[owner] = struct{}{}
	}
	return nil
}

func workspaceDeletionSealAckOwners(values []WorkspaceDeletionOwnerSealAck) []string {
	ret := make([]string, 0, len(values))
	for _, item := range values {
		ret = append(ret, item.OwnerModuleID)
	}
	return ret
}

func workspaceDeletionPurgeAckOwners(values []WorkspaceDeletionOwnerPurgeAck) []string {
	ret := make([]string, 0, len(values))
	for _, item := range values {
		ret = append(ret, item.OwnerModuleID)
	}
	return ret
}

func workspaceDeletionAckOwnerSetExact(required []string, observed []string) bool {
	if len(required) != len(observed) {
		return false
	}
	set := make(map[string]struct{}, len(observed))
	for _, item := range observed {
		if _, duplicate := set[item]; duplicate {
			return false
		}
		set[item] = struct{}{}
	}
	for _, item := range required {
		if _, ok := set[item]; !ok {
			return false
		}
	}
	return true
}

func workspaceDeletionAckOwnerPrefix(required []string, observed []string) bool {
	if len(observed) > len(required) {
		return false
	}
	for idx := range observed {
		if observed[idx] != required[idx] {
			return false
		}
	}
	return true
}
