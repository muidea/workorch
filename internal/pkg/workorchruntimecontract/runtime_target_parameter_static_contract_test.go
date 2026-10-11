package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func referenceRuntimeTargetParameterBindingsValidate(values []RuntimeTargetParameterBinding, owner string, kinds, targets []string) error {
	seen := make(map[string]bool, len(values))
	for _, binding := range values {
		key := binding.Kind + "\x00" + binding.Name
		contract, ok := RuntimeOperationContractForKind(binding.Kind)
		descriptor, found := RuntimeOperationDescriptorForKind(binding.Kind)
		parameter, declared := runtimeCandidateWireParameter(contract.Parameters, binding.Name)
		if seen[key] || !ok || !found || descriptor.OwnerModuleID != owner ||
			!runtimeOperationKindsContain(kinds, binding.Kind) || !declared || parameter.Required ||
			parameter.Type != RuntimeOperationParameterTypeString || !contract.RequireTargetRefs ||
			descriptor.TargetCardinality != RuntimeOperationTargetCardinalityOne ||
			containsString(contract.ForbiddenParameters, binding.Name) || containsString(contract.RequireOneOfParameters, binding.Name) {
			return fmt.Errorf("invalid owner target parameter declaration %q/%q", binding.Kind, binding.Name)
		}
		seen[key] = true
		for _, variant := range contract.Variants {
			if containsString(variant.ForbiddenParameters, binding.Name) || containsString(variant.RequiredParameters, binding.Name) || containsString(variant.RequiredNonEmpty, binding.Name) {
				return fmt.Errorf("target parameter %q conflicts with variant %q", binding.Name, variant.ID)
			}
			for _, discriminator := range variant.Discriminators {
				if discriminator.Name == binding.Name {
					return fmt.Errorf("target parameter %q cannot select a variant", binding.Name)
				}
			}
		}
		if len(binding.Values) > RuntimeOwnerInventoryItemsMaxCount {
			return fmt.Errorf("target parameter %q exceeds owner inventory bound", binding.Name)
		}
		seenTargets := make(map[string]bool, len(binding.Values))
		for _, value := range binding.Values {
			if value.TargetRef == "" || seenTargets[value.TargetRef] || !containsString(targets, value.TargetRef) || value.Value == "" || len(value.Value) > 1024 {
				return fmt.Errorf("invalid or duplicate target parameter preimage for %q/%q", binding.Kind, binding.Name)
			}
			seenTargets[value.TargetRef] = true
		}
	}
	return nil
}

func assertTargetParameterStaticReference(t testing.TB, values []RuntimeTargetParameterBinding, owner string, kinds, targets []string) {
	t.Helper()
	before := cloneRuntimeTargetParameterBindings(values)
	want := referenceRuntimeTargetParameterBindingsValidate(values, owner, kinds, targets)
	for range 2 {
		assertProofCollectionIdentityErrors(t, validateRuntimeTargetParameterBindings(values, owner, kinds, targets), want)
	}
	if !reflect.DeepEqual(values, before) {
		t.Fatal("静态 target parameter 查询改写原 preimage")
	}
	body, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	var loaded []RuntimeTargetParameterBinding
	if err := json.Unmarshal(body, &loaded); err != nil {
		t.Fatal(err)
	}
	assertProofCollectionIdentityErrors(t, validateRuntimeTargetParameterBindings(loaded, owner, kinds, targets), referenceRuntimeTargetParameterBindingsValidate(loaded, owner, kinds, targets))
}

func TestTargetParameterStaticContractKeepsCompleteInventoryAndRejectOrder(t *testing.T) {
	for _, descriptor := range RuntimeOperationDescriptors() {
		contract, _ := RuntimeOperationContractForKind(descriptor.Kind)
		for _, name := range append([]string{"", "unknown", " name "}, runtimeTargetParameterNamesForTest(contract)...) {
			for _, raw := range []bool{false, true} {
				kind := descriptor.Kind
				if raw {
					kind = " " + kind + " "
				}
				values := []RuntimeTargetParameterBinding{{Kind: kind, Name: name, Values: []RuntimeTargetParameterValue{{TargetRef: "source.txt", Value: "preimage"}}}}
				assertTargetParameterStaticReference(t, values, descriptor.OwnerModuleID, []string{descriptor.Kind}, []string{"source.txt"})
				assertTargetParameterStaticReference(t, values, "foreign-owner", []string{descriptor.Kind}, []string{"source.txt"})
			}
		}
	}
	valid := []RuntimeTargetParameterBinding{{Kind: RuntimeBlockKindPatchApply, Name: "baseHash", Values: []RuntimeTargetParameterValue{{TargetRef: "source.txt", Value: "preimage"}}}}
	if err := referenceRuntimeTargetParameterBindingsValidate(valid, RuntimeModuleFile, []string{RuntimeBlockKindPatchApply}, []string{"source.txt"}); err != nil {
		t.Fatalf("缺少有效 target parameter 前像：%v", err)
	}
	for _, scenario := range []string{"valid", "duplicate", "foreign-target", "duplicate-target", "empty-value", "long-value", "capacity", "wrong-kind", "wrong-name", "later-drift", "nil", "empty"} {
		t.Run(scenario, func(t *testing.T) {
			values := cloneRuntimeTargetParameterBindings(valid)
			switch scenario {
			case "duplicate":
				values = append(values, values[0])
			case "foreign-target":
				values[0].Values[0].TargetRef = "foreign"
			case "duplicate-target":
				values[0].Values = append(values[0].Values, values[0].Values[0])
			case "empty-value":
				values[0].Values[0].Value = ""
			case "long-value":
				values[0].Values[0].Value = strings.Repeat("x", 1025)
			case "capacity":
				values[0].Values = make([]RuntimeTargetParameterValue, RuntimeOwnerInventoryItemsMaxCount+1)
			case "wrong-kind":
				values[0].Kind = "unknown"
			case "wrong-name":
				values[0].Name = "oldText"
			case "later-drift":
				if err := validateRuntimeTargetParameterBindings(values, RuntimeModuleFile, []string{RuntimeBlockKindPatchApply}, []string{"source.txt"}); err != nil {
					t.Fatal(err)
				}
				values[0].Name = "unknown"
			case "nil":
				values = nil
			case "empty":
				values = []RuntimeTargetParameterBinding{}
			}
			want := referenceRuntimeTargetParameterBindingsValidate(values, RuntimeModuleFile, []string{RuntimeBlockKindPatchApply}, []string{"source.txt"})
			if (want == nil) != (scenario == "valid" || scenario == "nil" || scenario == "empty") {
				t.Fatalf("反例未命中：%s/%v", scenario, want)
			}
			assertTargetParameterStaticReference(t, values, RuntimeModuleFile, []string{RuntimeBlockKindPatchApply}, []string{"source.txt"})
		})
	}
}

func runtimeTargetParameterNamesForTest(contract RuntimeOperationContract) []string {
	var names []string
	for _, parameter := range contract.Parameters {
		names = append(names, parameter.Name)
	}
	return names
}

func TestTargetParameterStaticContractIsPrivateAndConcurrent(t *testing.T) {
	values := []RuntimeTargetParameterBinding{{Kind: RuntimeBlockKindPatchApply, Name: "baseHash", Values: []RuntimeTargetParameterValue{{TargetRef: "source.txt", Value: "preimage"}}}}
	if err := validateRuntimeTargetParameterBindings(values, RuntimeModuleFile, []string{RuntimeBlockKindPatchApply}, []string{"source.txt"}); err != nil {
		t.Fatal(err)
	}
	contract, _ := RuntimeOperationContractForKind(RuntimeBlockKindPatchApply)
	contract.Parameters[0].Name = "mutated"
	contract.RequireTargetRefs = false
	contract.ForbiddenParameters = append(contract.ForbiddenParameters, "baseHash")
	contract.Variants[0].ID = "mutated"
	contract.Variants[0].ForbiddenParameters = append(contract.Variants[0].ForbiddenParameters, "baseHash")
	if contract.RequireTargetRefs || contract.Parameters[0].Name != "mutated" || !containsString(contract.ForbiddenParameters, "baseHash") {
		t.Fatal("公开合同副本污染反例未命中")
	}
	var workers sync.WaitGroup
	for range 16 {
		workers.Go(func() {
			for range 16 {
				assertTargetParameterStaticReference(t, values, RuntimeModuleFile, []string{RuntimeBlockKindPatchApply}, []string{"source.txt"})
			}
		})
	}
	workers.Wait()
	original := testing.AllocsPerRun(20, func() {
		_ = referenceRuntimeTargetParameterBindingsValidate(values, RuntimeModuleFile, []string{RuntimeBlockKindPatchApply}, []string{"source.txt"})
	})
	current := testing.AllocsPerRun(20, func() {
		_ = validateRuntimeTargetParameterBindings(values, RuntimeModuleFile, []string{RuntimeBlockKindPatchApply}, []string{"source.txt"})
	})
	if current >= original {
		t.Fatalf("target parameter 校验仍构造完整静态合同：current=%v original=%v", current, original)
	}
}

func FuzzTargetParameterStaticContractReference(f *testing.F) {
	f.Add(RuntimeBlockKindPatchApply, "baseHash", "source.txt", "preimage", uint8(0))
	f.Add(" unknown ", string([]byte{0xff, 0x80}), "\x00", "", uint8(1))
	f.Fuzz(func(t *testing.T, kind, name, target, value string, mode uint8) {
		values := []RuntimeTargetParameterBinding{{Kind: kind, Name: name, Values: []RuntimeTargetParameterValue{{TargetRef: target, Value: value}}}}
		if mode%3 == 1 {
			values = append(values, values[0])
		}
		if mode%3 == 2 {
			values[0].Values = nil
		}
		assertTargetParameterStaticReference(t, values, RuntimeModuleFile, []string{kind}, []string{target})
	})
}

func BenchmarkTargetParameterStaticContract(b *testing.B) {
	values := []RuntimeTargetParameterBinding{{Kind: RuntimeBlockKindPatchApply, Name: "baseHash", Values: []RuntimeTargetParameterValue{{TargetRef: "source.txt", Value: "preimage"}}}}
	for _, mode := range []string{"reference", "current"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				var err error
				if mode == "reference" {
					err = referenceRuntimeTargetParameterBindingsValidate(values, RuntimeModuleFile, []string{RuntimeBlockKindPatchApply}, []string{"source.txt"})
				} else {
					err = validateRuntimeTargetParameterBindings(values, RuntimeModuleFile, []string{RuntimeBlockKindPatchApply}, []string{"source.txt"})
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
