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
		SourceReceiptHash: command.SourceReceiptHash, StageID: command.StageID, Stream: command.Stream, SavedSHA256: hash, ChunkSHA256: hash, Raw: raw,
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

func TestRuntimeValidationOutputReaderAssemblesChunksAndRejectsMidstreamDrift(t *testing.T) {
	for _, mode := range []string{"complete", "identity", "saved-hash", "short-page", "wrong-offset", "tampered-page"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(16)
			defer hub.Terminate(ctx)
			command := orchestratorevents.QueryRuntimeValidationOutputCommand{WorkspaceID: "ws", RunID: "run", DispatchID: "dispatch", OperationID: "op", Fence: 1, SourceReceiptHash: "original-receipt", StageID: "project-test", Stream: "stdout"}
			command.ExecutionKey = runtimecontract.StableRuntimeOperationExecutionKey(command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID)
			ref, err := command.ContentRef()
			if err != nil {
				t.Fatal(err)
			}
			raw := []byte(strings.Repeat("native\x00\xff\r\n", 20000) + "late failure")
			hash := func(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }
			identity := runtimecontract.RuntimeValidationOutputIdentity{Bytes: int64(len(raw)), SavedBytes: int64(len(raw)), SHA256: hash(raw), Complete: true, Storage: &runtimecontract.RuntimeValidationOutputStorage{Reference: "owner-ref"}}
			observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
			defer observer.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeValidationOutput)
			calls := 0
			if subscribeErr := observer.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeValidationOutput, func(ev magicevent.Event, result magicevent.Result) {
				query := ev.Data().(orchestratorevents.QueryRuntimeValidationOutputCommand)
				calls++
				page := append([]byte(nil), raw[query.Offset:min(int64(len(raw)), query.Offset+runtimecontract.RuntimeValidationOutputChunkMaxBytes)]...)
				value := orchestratorevents.QueryRuntimeValidationOutputResult{SourceReceiptHash: query.SourceReceiptHash, StageID: query.StageID, Stream: query.Stream, Offset: query.Offset, Identity: identity.Clone(), SavedSHA256: hash(raw), ChunkSHA256: hash(page), Raw: page}
				if calls == 2 {
					switch mode {
					case "identity":
						value.Identity.Storage.Reference = "other"
					case "saved-hash":
						value.SavedSHA256 = hash([]byte("other"))
					case "short-page":
						value.Raw = page[:len(page)-1]
						value.ChunkSHA256 = hash(value.Raw)
					case "wrong-offset":
						value.Offset++
					case "tampered-page":
						value.Raw[0] ^= 1
						value.ChunkSHA256 = hash(value.Raw)
					}
				}
				result.Set(value, nil)
			}); subscribeErr != nil {
				t.Fatal(subscribeErr)
			}

			value, err := NewReader(hub).ReadRuntimeRaw(ctx, ref)
			if mode == "complete" {
				if err != nil || !reflect.DeepEqual(value.Content, raw) || value.ContentSHA256 != hash(raw) || calls < 3 {
					t.Fatalf("chunk assembly: calls=%d err=%v", calls, err)
				}
			} else if err == nil || len(value.Content) != 0 {
				t.Fatal("drift returned partial/invalid output")
			}
		})
	}
}
