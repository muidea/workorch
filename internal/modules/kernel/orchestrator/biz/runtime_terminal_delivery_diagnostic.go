package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Owner/boundary are diagnostic labels, not state, retry authority or evidence
// of an owner's durable write. In particular owner_command can fail after an
// owner has committed but before its response is delivered.
type runtimeDeliveryBoundaryFailure struct {
	Owner            string
	Boundary         string
	DeliveryID       string
	MaterialRevision uint64
	cause            error
	responseError    error
}

func (e *runtimeDeliveryBoundaryFailure) Error() string {
	return fmt.Sprintf("terminal delivery owner=%s boundary=%s delivery=%s materialRevision=%d: %s", e.Owner, e.Boundary, e.DeliveryID, e.MaterialRevision, e.cause.Error())
}
func (e *runtimeDeliveryBoundaryFailure) Unwrap() error { return e.cause }

func runtimeDeliveryRequestBoundaryError(err error, owner runtimecontract.RuntimeDeliveryOwner, boundary string, request orchestratorevents.RuntimeTerminalDeliveryRequestData) error {
	if err == nil {
		return nil
	}
	return &runtimeDeliveryBoundaryFailure{Owner: string(owner), Boundary: boundary, DeliveryID: request.TerminalDeliveryID, MaterialRevision: request.StateRevision, cause: err}
}

func runtimeDeliveryBoundaryError(err error, owner, boundary string, a runtime.RuntimeAggregate) error {
	if err == nil {
		return nil
	}
	var detail *runtimeDeliveryBoundaryFailure
	if errors.As(err, &detail) {
		return err
	}
	detail = &runtimeDeliveryBoundaryFailure{Owner: owner, Boundary: boundary, cause: err}
	if a.TerminalMaterialSeal != nil {
		detail.DeliveryID = a.TerminalMaterialSeal.Identity.TerminalDeliveryID
		detail.MaterialRevision = a.TerminalMaterialSeal.GenerationStateRevision
	}
	return detail
}

type runtimeTerminalDeliveryDiagnostic struct {
	WorkspaceID              common.WorkspaceID                     `json:"workspaceId"`
	RunID                    common.RunID                           `json:"runId"`
	DeliveryID               string                                 `json:"terminalDeliveryId"`
	Owner                    string                                 `json:"owner"`
	Boundary                 string                                 `json:"boundary"`
	MaterialRevision         uint64                                 `json:"materialRevision"`
	AttemptRuntimeRevision   uint64                                 `json:"attemptRuntimeRevision"`
	ObservedRuntimeRevision  uint64                                 `json:"observedRuntimeRevision,omitempty"`
	ObservedRunRevision      uint64                                 `json:"observedRunRevision,omitempty"`
	ObservedAcknowledgements []runtimecontract.RuntimeDeliveryOwner `json:"observedAcknowledgements,omitempty"`
	RuntimeObservationError  []byte                                 `json:"runtimeObservationError,omitempty"`
	RunObservationError      []byte                                 `json:"runObservationError,omitempty"`
	OwnerResponseError       []byte                                 `json:"ownerResponseErrorBytes,omitempty"`
	OriginalError            []byte                                 `json:"originalErrorBytes"`
}

// Uses the existing restricted owner byte carrier, with a separate logical-key
// namespace. No new authority/ledger, aggregate write, model call or retry.
// Error strings use bytes (base64 in JSON), preserving invalid UTF-8 too.
// Generic telemetry gets only the reference. Failure to archive is explicit and never replaces the cause.
func (m *Biz) archiveRuntimeTerminalDeliveryFailure(ctx context.Context, a runtime.RuntimeAggregate, err error) {
	if m == nil || a.Facts.WorkspaceID == "" || a.Facts.RunID == "" {
		return
	}
	diagnosticCtx, cancel := context.WithTimeout(context.WithoutCancel(common.ContextWithRuntimeWorkspaceID(ctx, a.Facts.WorkspaceID)), 5*time.Second)
	defer cancel()
	var detail *runtimeDeliveryBoundaryFailure
	if !errors.As(err, &detail) {
		return
	}
	d := runtimeTerminalDeliveryDiagnostic{WorkspaceID: a.Facts.WorkspaceID, RunID: a.Facts.RunID,
		DeliveryID: detail.DeliveryID, Owner: detail.Owner, Boundary: detail.Boundary, MaterialRevision: detail.MaterialRevision,
		AttemptRuntimeRevision: a.StateRevision, OriginalError: []byte(detail.cause.Error())}
	if detail.responseError != nil {
		d.OwnerResponseError = []byte(detail.responseError.Error())
	}
	// A diagnostic needs the Run owner's observed opaque revision, not another
	// expensive full aggregate admission. Read both revisions from one typed
	// owner snapshot; none of these observations can authorize an ACK.
	if m.EventHub() != nil {
		observed, readErr := (runSnapshotReader{hub: m.EventHub()}).Read(diagnosticCtx, d.WorkspaceID, d.RunID)
		if readErr != nil {
			d.RunObservationError = []byte(readErr.Error())
			d.RuntimeObservationError = []byte(readErr.Error())
		} else {
			d.ObservedRunRevision = observed.Revision
			if observed.RuntimeState == nil {
				d.RuntimeObservationError = []byte("Run owner snapshot has no Runtime record")
			} else {
				d.ObservedRuntimeRevision = observed.RuntimeState.StateRevision
				for _, ack := range observed.RuntimeState.DeliveryAcknowledgements {
					if ack.TerminalDeliveryID == d.DeliveryID {
						d.ObservedAcknowledgements = append(d.ObservedAcknowledgements, ack.Owner)
					}
				}
			}
		}
	} else {
		d.RunObservationError = []byte("Run owner observation unavailable")
		d.RuntimeObservationError = []byte("Run owner observation unavailable")
	}
	// Optional observations must not consume the time reserved for saving the
	// original cause. Archival never changes the original delivery outcome.
	cancel()
	archiveCtx, archiveCancel := context.WithTimeout(context.WithoutCancel(common.ContextWithRuntimeWorkspaceID(ctx, a.Facts.WorkspaceID)), 5*time.Second)
	defer archiveCancel()
	raw, marshalErr := json.Marshal(d)
	var ref common.ArtifactContinuationID
	var hash string
	archiveErr := marshalErr
	if archiveErr == nil {
		archiveErr = m.withOrchestratorEvidenceMutation(archiveCtx, d.WorkspaceID, func(guarded context.Context) error {
			var saveErr error
			ref, hash, saveErr = persistRuntimeCandidateOpaqueContent(guarded, m.evidence.ArtifactContinuations, a, "terminal-delivery-diagnostic/"+runtimeCandidateOutputHash(string(raw)), raw)
			return saveErr
		})
	}
	if archiveErr != nil {
		slog.Warn("workorch Runtime terminal diagnostic not saved", "workspaceID", d.WorkspaceID, "runID", d.RunID, "deliveryID", d.DeliveryID, "owner", d.Owner, "boundary", d.Boundary, "bytes", len(raw), "err", archiveErr)
		return
	}
	slog.Warn("workorch Runtime terminal diagnostic saved", "workspaceID", d.WorkspaceID, "runID", d.RunID, "deliveryID", d.DeliveryID, "owner", d.Owner, "boundary", d.Boundary, "runtimeRevision", d.ObservedRuntimeRevision, "runRevision", d.ObservedRunRevision, "ref", ref, "bytes", len(raw), "sha256", hash)
}
