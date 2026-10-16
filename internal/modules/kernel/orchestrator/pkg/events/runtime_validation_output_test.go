package events

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeValidationOutputResultRejectsAlteredOrUnboundedOwnerFacts(t *testing.T) {
	command := QueryRuntimeValidationOutputCommand{WorkspaceID: "ws-output", RunID: "run-output", DispatchID: "dispatch-output", OperationID: "operation-output", Fence: 1, SourceReceiptHash: "receipt-hash", StageID: "test", Stream: "stdout"}
	command.ExecutionKey = runtimecontract.StableRuntimeOperationExecutionKey(command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID)
	raw := []byte{0, 255, '\r', '\n'}
	digest := sha256.Sum256(raw)
	hash := hex.EncodeToString(digest[:])
	value := QueryRuntimeValidationOutputResult{
		SourceReceiptHash: command.SourceReceiptHash, StageID: command.StageID, Stream: command.Stream, Raw: raw, SavedSHA256: hash,
		Identity: runtimecontract.RuntimeValidationOutputIdentity{Bytes: 4, SavedBytes: 4, SHA256: hash, Complete: true, Storage: &runtimecontract.RuntimeValidationOutputStorage{Reference: "owner-reference"}},
	}
	if err := value.ValidateFor(command); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*QueryRuntimeValidationOutputResult)
	}{
		{"receipt", func(v *QueryRuntimeValidationOutputResult) { v.SourceReceiptHash = "other" }},
		{"stage", func(v *QueryRuntimeValidationOutputResult) { v.StageID = "other" }},
		{"stream", func(v *QueryRuntimeValidationOutputResult) { v.Stream = "stderr" }},
		{"storage missing", func(v *QueryRuntimeValidationOutputResult) { v.Identity.Storage = nil }},
		{"storage omitted", func(v *QueryRuntimeValidationOutputResult) {
			v.Identity.Storage = &runtimecontract.RuntimeValidationOutputStorage{OmittedReason: "storage_unavailable"}
		}},
		{"saved length", func(v *QueryRuntimeValidationOutputResult) { v.Raw = v.Raw[:3] }},
		{"saved hash", func(v *QueryRuntimeValidationOutputResult) { v.SavedSHA256 = "tampered" }},
		{"original hash", func(v *QueryRuntimeValidationOutputResult) { v.Identity.SHA256 = hex.EncodeToString(make([]byte, 32)) }},
		{"raw bound", func(v *QueryRuntimeValidationOutputResult) {
			v.Raw = make([]byte, 64*1024+1)
			digest := sha256.Sum256(v.Raw)
			v.SavedSHA256 = hex.EncodeToString(digest[:])
			v.Identity.SHA256 = v.SavedSHA256
			v.Identity.Bytes, v.Identity.SavedBytes = int64(len(v.Raw)), int64(len(v.Raw))
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			invalid := value
			invalid.Identity = invalid.Identity.Clone()
			test.mutate(&invalid)
			if invalid.ValidateFor(command) == nil {
				t.Fatal("altered owner result accepted")
			}
		})
	}
	partial := value
	partial.Identity = partial.Identity.Clone()
	partial.Identity.Bytes = 9
	partial.Identity.Complete = false
	partial.Identity.OmittedReason = "output_capacity_exceeded"
	partial.Identity.SHA256 = hex.EncodeToString(make([]byte, 32))
	if err := partial.ValidateFor(command); err != nil || partial.Identity.Bytes != 9 || partial.Identity.SHA256 == partial.SavedSHA256 {
		t.Fatalf("partial output substituted saved prefix for original stream: %+v err=%v", partial, err)
	}
}
