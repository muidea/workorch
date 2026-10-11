package workorchruntimecontract

import (
	"errors"
	"sync"
	"testing"
)

func TestRuntimeBinaryOperationContractRevisionMatchesFullReference(t *testing.T) {
	want := referenceRuntimeOperationContractRevision()
	if want == "" || compileRuntimeOperationContractRevision() != want {
		t.Fatal("binary 静态库存编码改变原 contract identity")
	}
	var workers sync.WaitGroup
	for range 32 {
		workers.Go(func() {
			for range 32 {
				if RuntimeOperationContractRevision() != want {
					t.Error("并发读取静态 contract identity 不一致")
				}
			}
		})
	}
	workers.Wait()
	// 公开库存仍返回隔离值；caller 不能改变本 binary 的静态 revision。
	for _, descriptor := range RuntimeOperationDescriptors() {
		kind := descriptor.Kind
		if len(descriptor.AllowedStates) != 0 {
			descriptor.AllowedStates[0] = "caller-change"
		}
		contract, found := RuntimeOperationContractForKind(kind)
		// descriptor 还包含非 executable 的 Approval/Governance；原 revision 只编码有 contract 的项。
		if found && len(contract.Parameters) != 0 {
			contract.Parameters[0].Name = "caller-change"
		}
		if RuntimeOperationContractRevision() != want || referenceRuntimeOperationContractRevision() != want {
			t.Fatal("公开库存副本变更污染静态 revision")
		}
	}
}

func TestRuntimeBinaryOperationContractRevisionAlwaysChecksPersistedFence(t *testing.T) {
	current := RuntimeOperationContractRevision()
	for _, value := range []string{current, " " + current + " ", "", "operation-contract-stale", current + "changed", "\x00"} {
		err := ValidateRuntimeOperationContractRevision(value)
		if value == current || value == " "+current+" " {
			if err != nil {
				t.Fatal(err)
			}
			continue
		}
		var mismatch *RuntimeOperationContractRevisionMismatchError
		if !errors.Is(err, ErrRuntimeOperationContractRevisionMismatch) || !errors.As(err, &mismatch) || mismatch.PersistedRevision != value || mismatch.CurrentRevision != current {
			t.Fatalf("静态值复用跳过 persisted revision fence: input=%q err=%v", value, err)
		}
	}
	if err := ValidateRuntimeOperationContractRevision(current); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeOperationContractRevision("operation-contract-later-drift"); err == nil {
		t.Fatal("前次通过掩盖后续 persisted revision 漂移")
	}
}

func BenchmarkRuntimeBinaryOperationContractRevision(b *testing.B) {
	for _, reference := range []bool{true, false} {
		name := "binary_static"
		if reference {
			name = "reference"
		}
		b.Run(name, func(b *testing.B) {
			want := RuntimeOperationContractRevision()
			b.ReportAllocs()
			for b.Loop() {
				value := RuntimeOperationContractRevision()
				if reference {
					value = referenceRuntimeOperationContractRevision()
				}
				if value != want {
					b.Fatal("contract identity 漂移")
				}
			}
		})
	}
}
