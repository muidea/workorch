package content

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeValidationOutputReaderPreservesBytesAndRejectsIdentityDrift(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	command := orchestratorevents.QueryRuntimeValidationOutputCommand{
		WorkspaceID: "ws-output", RunID: "run-output", DispatchID: "dispatch-output", OperationID: "operation-output",
		Fence: 2, SourceReceiptHash: "receipt-hash", StageID: "test", Stream: "stdout",
	}
	command.ExecutionKey = runtimecontract.StableRuntimeOperationExecutionKey(command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID)
	ref, err := command.ContentRef()
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte{'\x00', '\xff', '\r', '\n', 'x'}
	digest := sha256.Sum256(raw)
	hash := hex.EncodeToString(digest[:])
	output := orchestratorevents.QueryRuntimeValidationOutputResult{
		SourceReceiptHash: command.SourceReceiptHash, StageID: command.StageID, Stream: command.Stream, SavedSHA256: hash, Raw: raw,
		Identity: runtimecontract.RuntimeValidationOutputIdentity{Bytes: int64(len(raw)), SavedBytes: int64(len(raw)), SHA256: hash, Complete: true, Storage: &runtimecontract.RuntimeValidationOutputStorage{Reference: "owner-only-reference"}},
	}
	observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	calls := 0
	if err := observer.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeValidationOutput, func(ev magicevent.Event, result magicevent.Result) {
		calls++
		if got := ev.Data().(orchestratorevents.QueryRuntimeValidationOutputCommand); got != command || common.RuntimeWorkspaceIDFromContext(ev.Context()) != command.WorkspaceID || ev.Source() != orchestratorevents.RuntimeValidationOutputReadSource {
			t.Error("read selector or context scope drifted")
		}
		result.Set(output, nil)
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { observer.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeValidationOutput) })
	reader := NewReader(hub)
	value, err := reader.ReadRuntimeRaw(ctx, ref)
	if err != nil || !reflect.DeepEqual(value.Content, raw) || value.ContentSHA256 != hash || value.OutputIdentity == nil || !value.OutputIdentity.Complete {
		t.Fatalf("original output lost: %+v err=%v", value, err)
	}
	value.Content[0] = 1
	value.OutputIdentity.Storage.Reference = "caller"
	if raw[0] != 0 || output.Identity.Storage.Reference != "owner-only-reference" {
		t.Fatal("application result aliases owner output")
	}
	body, err := json.Marshal(command)
	if err != nil {
		t.Fatal(err)
	}
	wireRef := func(body []byte) string {
		return "workorch://workspaces/ws-output/runs/run-output/validation-output/" + base64.RawURLEncoding.EncodeToString(body) + "/stdout.bin"
	}
	for name, changed := range map[string]string{
		"workspace":               strings.Replace(ref, "/ws-output/", "/other/", 1),
		"run":                     strings.Replace(ref, "/run-output/", "/other/", 1),
		"stream":                  strings.Replace(ref, "/stdout.bin", "/stderr.bin", 1),
		"trailing document":       "workorch://workspaces/ws-output/runs/run-output/validation-output/" + base64.RawURLEncoding.EncodeToString([]byte(`{} {}`)) + "/stdout.bin",
		"duplicate key":           wireRef([]byte(strings.Replace(string(body), `"stageId":"test"`, `"stageId":"test","stageId":"test"`, 1))),
		"unknown field":           wireRef([]byte(strings.Replace(string(body), `"stageId":"test"`, `"stageId":"test","storagePath":"private"`, 1))),
		"noncanonical whitespace": wireRef(append([]byte(" "), body...)),
		"invalid UTF8":            wireRef(append([]byte{255}, body...)),
		"oversize selector":       wireRef([]byte(strings.Repeat("x", 8193))),
	} {
		t.Run(name, func(t *testing.T) {
			before := calls
			if _, err := reader.ReadRuntimeRaw(ctx, changed); err == nil || calls != before {
				t.Fatal("invalid reference reached owner")
			}
		})
	}
	output.SavedSHA256 = strings.Repeat("0", 64)
	if _, err := reader.ReadRuntimeRaw(ctx, ref); err == nil {
		t.Fatal("tampered bytes hash accepted")
	}
	output.SavedSHA256 = hash
	output.Identity.Bytes++
	output.Identity.Complete = false
	output.Identity.OmittedReason = "output_capacity_exceeded"
	if value, err := reader.ReadRuntimeRaw(ctx, ref); err != nil || value.OutputIdentity.Complete || value.OutputIdentity.Bytes != int64(len(raw)+1) || value.OutputIdentity.OmittedReason == "" {
		t.Fatalf("partial saved bytes mislabeled: %+v err=%v", value, err)
	}
}
