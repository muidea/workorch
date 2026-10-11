package events

import (
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/eventcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func rawOutputCommandFixture() QueryRuntimeValidationRawOutputCommand {
	return QueryRuntimeValidationRawOutputCommand{WorkspaceID: "ws", RunID: "run", DispatchID: "dispatch", OperationID: "op", ExecutionKey: runtimecontract.StableRuntimeOperationExecutionKey("ws", "run", "dispatch", "op"), Fence: 1, SourceReceiptHash: "original-receipt", StageID: "project-test", Stream: "stdout"}
}

func TestValidationRawOutputContractExactBytesBoundsAndIsolation(t *testing.T) {
	found := false
	for _, contract := range Contracts() {
		if contract.Topic == TopicQueryRuntimeValidationRawOutput {
			found = true
			if contract.CommandClass != eventcontract.CommandClassRead || contract.LanePolicy != eventcontract.LanePolicyQuery ||
				contract.Metadata.StateMachine != eventcontract.StateMachineNone || contract.Metadata.ReducerOwner != "" ||
				contract.Metadata.EffectClass != eventcontract.EffectClassReadOnly || contract.Metadata.TransitionTarget != eventcontract.TransitionTargetNone ||
				contract.Metadata.ScopePolicy != eventcontract.ScopePolicyWorkspaceRunStep || contract.Metadata.IdentityPolicy != eventcontract.IdentityPolicyRuntimeOperation {
				t.Fatal("raw read 被提升为 Runtime 转换或执行入口")
			}
		}
	}
	if !found {
		t.Fatal("raw read 未注册 owner 合同")
	}
	command := rawOutputCommandFixture()
	for _, raw := range [][]byte{nil, {}, []byte("原始\r\n\x00\xff TOKEN=fixture原样"), make([]byte, RuntimeValidationRawOutputMaxBytes)} {
		digest := sha256.Sum256(raw)
		identity := runtimecontract.RuntimeValidationOutputIdentity{Bytes: int64(len(raw)), SavedBytes: int64(len(raw)), SHA256: hex.EncodeToString(digest[:]), Complete: true, Storage: &runtimecontract.RuntimeValidationOutputStorage{Reference: "owner/ref"}}
		response := QueryRuntimeValidationRawOutputResult{SourceReceiptHash: command.SourceReceiptHash, StageID: command.StageID, Stream: command.Stream, Identity: identity, SavedSHA256: identity.SHA256, Raw: raw}
		if response.ValidateFor(command) != nil {
			t.Fatal("合法原 bytes 被拒绝")
		}
		copy := response.Clone()
		if !reflect.DeepEqual(copy, response) {
			t.Fatal("clone 改写 nil/empty 或原 bytes")
		}
		copy.Identity.Storage.Reference = "caller"
		if len(copy.Raw) > 0 {
			copy.Raw[0]++
		}
		if response.Identity.Storage.Reference != "owner/ref" || response.ValidateFor(command) != nil {
			t.Fatal("返回副本共享原文")
		}
		for _, field := range []string{"receipt", "stage", "stream", "hash", "bytes", "omission", "ref", "bound"} {
			changed := response.Clone()
			switch field {
			case "receipt":
				changed.SourceReceiptHash = "other"
			case "stage":
				changed.StageID = "other"
			case "stream":
				changed.Stream = "stderr"
			case "hash":
				changed.SavedSHA256 = "other"
			case "bytes":
				changed.Identity.SavedBytes++
			case "omission":
				changed.Identity.Storage.Reference, changed.Identity.Storage.OmittedReason = "", "not_saved"
			case "ref":
				changed.Identity.Storage = nil
			case "bound":
				changed.Raw = make([]byte, RuntimeValidationRawOutputMaxBytes+1)
			}
			if changed.ValidateFor(command) == nil {
				t.Fatalf("非法 response 未拒绝：%s", field)
			}
		}
		response.Identity.Complete, response.Identity.OmittedReason, response.Identity.Bytes = false, "capacity", int64(len(raw))+1
		emptyDigest := sha256.Sum256(nil)
		response.Identity.SHA256 = hex.EncodeToString(emptyDigest[:])
		if response.ValidateFor(command) != nil {
			t.Fatal("已保存前缀不能显式返回 incomplete")
		}
	}
	for _, field := range []string{"workspace", "run", "dispatch", "operation", "key", "fence", "receipt", "stage", "stream"} {
		changed := command
		switch field {
		case "workspace":
			changed.WorkspaceID = "other"
		case "run":
			changed.RunID = "other"
		case "dispatch":
			changed.DispatchID = "other"
		case "operation":
			changed.OperationID = "other"
		case "key":
			changed.ExecutionKey = "other"
		case "fence":
			changed.Fence = 0
		case "receipt":
			changed.SourceReceiptHash = " raw "
		case "stage":
			changed.StageID = "stage\x00"
		case "stream":
			changed.Stream = "other"
		}
		if changed.Validate() == nil {
			t.Fatalf("非法 command 未拒绝：%s", field)
		}
	}
}
