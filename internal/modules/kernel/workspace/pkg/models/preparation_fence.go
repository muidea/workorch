package workspace

import (
	"encoding/hex"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const WorkspacePreparationFenceVersion = "workorch-preparation-fence/v1"
const WorkspaceFreshIntentHistoryLimit = 128

// WorkspacePreparationFence preserves creation identity and unresolved fresh
// intent identity. Accepted originals are retained, never cleared or evicted;
// a missing Run or a lost response cannot prove a durable rejection.
type WorkspacePreparationFence struct {
	Version          string
	WorkspaceID      common.WorkspaceID
	CreationEpoch    string
	PreparationEpoch uint64
	Pending          *WorkspaceFreshIntentReservation
	Settled          []WorkspaceFreshIntentSettlement
}

// Settlement preserves both owner identities. The Workspace payload hash and
// the Run receipt payload hash have different domains and are not interchangeable.
type WorkspaceFreshIntentSettlement struct {
	Reservation WorkspaceFreshIntentReservation
	Receipt     runmodel.WorkspaceExecutionIntentReceipt
}

// WorkspaceFreshIntentReservation stores only the identity of an already
// validated, credential-free typed payload, never live configuration or keys.
type WorkspaceFreshIntentReservation struct {
	RequestID               string
	Kind                    runtimecontract.RuntimeExecutionIntentKind
	StartupInputHash        string
	PayloadHash             string
	RunPayloadHash          string
	PreparationCompletionID string    `json:",omitempty"`
	PreparationRevision     uint64    `json:",omitempty"`
	PreparationConsumedAt   time.Time `json:",omitempty"`
}

func (f *WorkspacePreparationFence) Clone() *WorkspacePreparationFence {
	if f == nil {
		return nil
	}
	next := *f
	next.Settled = slices.Clone(f.Settled)
	if f.Pending != nil {
		pending := *f.Pending
		next.Pending = &pending
	}
	return &next
}

func (r WorkspaceFreshIntentReservation) Validate() error {
	if !runmodel.ValidExecutionIntentIdentity(r.RequestID) || r.Kind != runtimecontract.RuntimeExecutionIntentRestart || r.StartupInputHash == "" || len(r.StartupInputHash) > 256 || strings.TrimSpace(r.StartupInputHash) != r.StartupInputHash || !preparationFenceHex(r.PayloadHash, 32) || !preparationFenceHex(r.RunPayloadHash, 32) {
		return fmt.Errorf("%w: invalid fresh intent reservation", common.ErrInvalidArgument)
	}
	if r.PreparationCompletionID != "" && (!strings.HasPrefix(r.PreparationCompletionID, "preparation-completion-sha256:") || !preparationFenceHex(strings.TrimPrefix(r.PreparationCompletionID, "preparation-completion-sha256:"), 32)) {
		return common.ErrInvalidArgument
	}
	if r.PreparationCompletionID == "" && (r.PreparationRevision != 0 || !r.PreparationConsumedAt.IsZero()) || r.PreparationCompletionID != "" && (r.PreparationRevision == 0 || r.PreparationConsumedAt.IsZero()) {
		return common.ErrInvalidArgument
	}
	return nil
}

func (f *WorkspacePreparationFence) Validate(workspaceID common.WorkspaceID) error {
	if f == nil {
		return nil // explicitly legacy, never evidence for first M3
	}
	if f.Version != WorkspacePreparationFenceVersion || workspaceID == "" || f.WorkspaceID != workspaceID || !preparationFenceHex(f.CreationEpoch, 16) {
		return fmt.Errorf("%w: invalid preparation creation fence", common.ErrInvalidArgument)
	}
	if len(f.Settled) > WorkspaceFreshIntentHistoryLimit {
		return fmt.Errorf("%w: fresh intent history is full", common.ErrInvalidArgument)
	}
	seen := map[string]bool{}
	for _, settled := range f.Settled {
		if settled.Reservation.Validate() != nil || settled.Receipt.Validate() != nil || settled.Receipt.PayloadHash != settled.Reservation.RunPayloadHash || settled.Receipt.Admission.WorkspaceID != workspaceID || settled.Receipt.Admission.RequestID != settled.Reservation.RequestID || settled.Receipt.Admission.Kind != settled.Reservation.Kind || seen[settled.Reservation.RequestID] {
			return fmt.Errorf("%w: invalid fresh intent settlement", common.ErrInvalidArgument)
		}
		seen[settled.Reservation.RequestID] = true
	}
	epoch := uint64(len(f.Settled))
	if f.Pending != nil {
		if f.Pending.Validate() != nil || seen[f.Pending.RequestID] || len(f.Settled) == WorkspaceFreshIntentHistoryLimit {
			return fmt.Errorf("%w: invalid unresolved fresh intent", common.ErrInvalidArgument)
		}
		epoch++
	}
	if f.PreparationEpoch != epoch {
		return fmt.Errorf("%w: preparation epoch lost its history", common.ErrInvalidArgument)
	}
	return nil
}

// ValidatePreparationFenceTransition is a persistence backstop, not a reducer.
// Only creation may introduce a creation epoch; subsequent writes can append
// a pending reservation or settle it with an original receipt. They cannot
// erase, replace, evict or upgrade history. Only the private reducer may judge
// the concrete Run-owner observation used for settlement.
func ValidatePreparationFenceTransition(current, next *Workspace) error {
	if next == nil || next.ID == "" || next.PreparationFence.Validate(next.ID) != nil {
		return common.ErrInvalidArgument
	}
	if current == nil {
		if next.PreparationFence != nil && (next.PreparationFence.Pending != nil || len(next.PreparationFence.Settled) != 0) {
			return common.ErrConflict
		}
		return nil
	}
	if current.ID != next.ID || current.PreparationFence.Validate(current.ID) != nil {
		return common.ErrConflict
	}
	before, after := current.PreparationFence, next.PreparationFence
	// Raw consumption evidence is immutable and can first appear only with
	// the matching first reservation. Complete source validation is performed
	// by preparationcompiler at every repository boundary.
	if current.FirstExecutionPreparationEvidence != nil {
		if !reflect.DeepEqual(current.FirstExecutionPreparationEvidence, next.FirstExecutionPreparationEvidence) {
			return common.ErrConflict
		}
	} else if e := next.FirstExecutionPreparationEvidence; e != nil {
		if before == nil || before.PreparationEpoch != 0 || before.Pending != nil || after == nil || after.Pending == nil ||
			after.Pending.RequestID != e.RequestID || after.Pending.PreparationCompletionID == "" {
			return common.ErrConflict
		}
	}
	if before == nil {
		if after != nil { // cannot silently migrate legacy to first-M3 eligible
			return common.ErrConflict
		}
		return nil
	}
	if after == nil || before.Version != after.Version || before.WorkspaceID != after.WorkspaceID || before.CreationEpoch != after.CreationEpoch {
		return common.ErrConflict
	}
	if reflect.DeepEqual(before, after) {
		return nil
	}
	if before.Pending == nil {
		if after.Pending != nil && before.PreparationEpoch < WorkspaceFreshIntentHistoryLimit && after.PreparationEpoch == before.PreparationEpoch+1 && slices.Equal(before.Settled, after.Settled) {
			return nil
		}
	} else if after.Pending == nil && after.PreparationEpoch == before.PreparationEpoch && len(after.Settled) == len(before.Settled)+1 && slices.Equal(before.Settled, after.Settled[:len(before.Settled)]) && after.Settled[len(before.Settled)].Reservation == *before.Pending {
		return nil
	}
	return common.ErrConflict
}

func preparationFenceHex(value string, size int) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == size && hex.EncodeToString(decoded) == value
}

// Ordinary metadata updates cannot reserve an execution intent without CAS.
func ValidatePreparationFenceUpdate(current, next *Workspace) error {
	if current == nil {
		return common.ErrInvalidArgument
	}
	if err := ValidatePreparationFenceTransition(current, next); err != nil {
		return err
	}
	if !reflect.DeepEqual(current.PreparationFence, next.PreparationFence) {
		return common.ErrConflict
	}
	return nil
}
