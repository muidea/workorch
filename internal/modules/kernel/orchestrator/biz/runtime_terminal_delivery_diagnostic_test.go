package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func assertRuntimeTerminalDeliveryDiagnostic(t *testing.T, m *Biz, a runtime.RuntimeAggregate, err error) {
	t.Helper()
	var detail *runtimeDeliveryBoundaryFailure
	if !errors.As(err, &detail) || detail.Owner == "" || detail.Boundary == "" || detail.DeliveryID == "" || detail.MaterialRevision == 0 {
		t.Fatalf("delivery failure lost exact boundary: %v", err)
	}
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), a.Facts.WorkspaceID)
	contents, readErr := m.evidence.ArtifactContinuations.ListByRun(ctx, a.Facts.RunID)
	if readErr != nil {
		t.Fatal(readErr)
	}
	found := 0
	for _, content := range contents {
		meta, raw, getErr := m.evidence.ArtifactContinuations.GetAssembled(ctx, content.ID)
		if getErr != nil {
			continue
		}
		var d runtimeTerminalDeliveryDiagnostic
		if json.Unmarshal([]byte(raw), &d) != nil || d.DeliveryID != detail.DeliveryID {
			continue
		}
		found++
		if detail.responseError != nil && !bytes.Equal(d.OwnerResponseError, []byte(detail.responseError.Error())) {
			t.Fatal("Run response adapter lost original owner error")
		}
		if d.Owner != detail.Owner || d.Boundary != detail.Boundary || d.MaterialRevision != detail.MaterialRevision || !bytes.Equal(d.OriginalError, []byte(detail.cause.Error())) || meta.Bytes != int64(len(raw)) || meta.RollingHash != runtimeCandidateOutputHash(raw) {
			t.Fatal("diagnostic changed original cause, boundary or bytes")
		}
		if (d.ObservedRuntimeRevision == 0 && len(d.RuntimeObservationError) == 0) || (d.ObservedRunRevision == 0 && len(d.RunObservationError) == 0) {
			t.Fatal("diagnostic omitted actual owner observations")
		}
	}
	if found != 1 {
		t.Fatalf("want one exact restricted diagnostic, got %d", found)
	}
}

func TestRuntimeTerminalDeliveryBoundaryPreservesErrorIdentity(t *testing.T) {
	original := errors.New("原始错误\r\n\x00\xff")
	request := orchestratorevents.RuntimeTerminalDeliveryRequestData{StateRevision: 42}
	request.TerminalDeliveryID = "delivery-original"
	err := runtimeDeliveryRequestBoundaryError(original, runtimecontract.RuntimeDeliveryOwnerResult, "acknowledgement_persist", request)
	if !errors.Is(err, original) {
		t.Fatal("lost original error identity")
	}
	wrapped := runtimeDeliveryBoundaryError(err, ModuleID, "outer", runtime.RuntimeAggregate{})
	if wrapped != err {
		t.Fatal("outer boundary hid actual failure boundary")
	}
	d := runtimeTerminalDeliveryDiagnostic{OriginalError: []byte(original.Error())}
	raw, _ := json.Marshal(d)
	var copy runtimeTerminalDeliveryDiagnostic
	if json.Unmarshal(raw, &copy) != nil || !bytes.Equal(copy.OriginalError, []byte(original.Error())) {
		t.Fatal("raw diagnostic encoding changed arbitrary bytes")
	}
}

func TestRuntimeTerminalDeliveryBoundarySeparatesResponseFromACKPersistence(t *testing.T) {
	for _, validResponse := range []bool{false, true} {
		t.Run(map[bool]string{false: "wrong_response_type", true: "missing_ack"}[validResponse], func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(8)
			defer hub.Terminate(ctx)
			observer := magicevent.NewSimpleObserver(resultevents.CommandDestination, hub)
			observer.Subscribe(resultevents.TopicResultFactTerminalDelivery, func(_ magicevent.Event, result magicevent.Result) {
				if validResponse {
					result.Set(resultevents.ResultLifecycleCommandResult{}, nil)
				} else {
					result.Set("wrong type", nil)
				}
			})
			m := newRuntimeProjectionTestModule(t, hub)
			a := runtimeTerminalAggregate(t)
			before := a.CloneRaw()
			request := mustTerminalDeliveryRequest(t, a)
			err := m.deliverRuntimeResult(ctx, request, mustRuntimeResultMaterial(t, a))
			var detail *runtimeDeliveryBoundaryFailure
			want := "owner_response_decode"
			if validResponse {
				want = "acknowledgement_persist"
			}
			if !errors.As(err, &detail) || detail.Owner != string(runtimecontract.RuntimeDeliveryOwnerResult) || detail.Boundary != want || !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("wrong failure location: %v", err)
			}
			if !reflect.DeepEqual(a, before) {
				t.Fatal("diagnostic wrapper changed authority")
			}
		})
	}
}
