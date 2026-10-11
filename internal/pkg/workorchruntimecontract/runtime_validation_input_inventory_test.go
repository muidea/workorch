package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func validationInputTargetsPublicInventoryReference(surface RuntimeValidationSurface) error {
	if (surface.InputTargetOwnerModuleID == "") != (len(surface.InputTargetRefs) == 0) {
		return fmt.Errorf("validation input targets require resource owner and targets together")
	}
	if len(surface.InputTargetRefs) == 0 {
		return nil
	}
	knownOwner := false
	for _, descriptor := range RuntimeOperationDescriptors() {
		knownOwner = knownOwner || descriptor.OwnerModuleID == surface.InputTargetOwnerModuleID
	}
	if !knownOwner || len(surface.InputTargetRefs) > RuntimeValidationInputTargetsMaxCount {
		return fmt.Errorf("validation input targets have unknown resource owner or exceed inventory bound")
	}
	seen := make(map[string]bool, len(surface.InputTargetRefs))
	for _, target := range surface.InputTargetRefs {
		if target == "" || strings.TrimSpace(target) != target || len(target) > 2048 || seen[target] ||
			strings.IndexFunc(target, func(r rune) bool { return r < 32 || r == 127 }) >= 0 {
			return fmt.Errorf("validation input targets contain an invalid or duplicate identity")
		}
		seen[target] = true
	}
	return nil
}

func assertValidationInputTargetsInventoryReference(t testing.TB, value RuntimeValidationSurface) {
	t.Helper()
	before, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		got, want := validateRuntimeValidationInputTargets(value), validationInputTargetsPublicInventoryReference(value)
		if (got == nil) != (want == nil) || (got != nil && got.Error() != want.Error()) {
			t.Fatalf("validation input target 完整拒绝原因漂移：got=%v want=%v", got, want)
		}
	}
	after, err := json.Marshal(value)
	if err != nil || string(before) != string(after) {
		t.Fatal("inventory 只读查询改写原 surface")
	}
	var reloaded RuntimeValidationSurface
	if err := json.Unmarshal(before, &reloaded); err != nil {
		t.Fatal(err)
	}
	got, want := validateRuntimeValidationInputTargets(reloaded), validationInputTargetsPublicInventoryReference(reloaded)
	if (got == nil) != (want == nil) || (got != nil && got.Error() != want.Error()) {
		t.Fatal("JSON 重载后 inventory 校验不等价")
	}
}

func TestValidationInputTargetsReadOnlyInventoryMatchesOriginal(t *testing.T) {
	before := RuntimeOperationDescriptors()
	owners := []string{"", "unknown", " file ", string([]byte{0xff})}
	for _, descriptor := range before {
		owners = append(owners, descriptor.OwnerModuleID)
	}
	refsAtBound := make([]string, RuntimeValidationInputTargetsMaxCount)
	for index := range refsAtBound {
		refsAtBound[index] = fmt.Sprintf("input-%03d", index)
	}
	for _, owner := range owners {
		for _, refs := range [][]string{nil, {}, {"input"}, {""}, {" input "}, {"input", "input"}, {"\x00"}, {"\x7f"}, {string([]byte{0xff})}, {strings.Repeat("x", 2048)}, {strings.Repeat("x", 2049)}, refsAtBound, append(append([]string(nil), refsAtBound...), "over-bound")} {
			assertValidationInputTargetsInventoryReference(t, RuntimeValidationSurface{InputTargetOwnerModuleID: owner, InputTargetRefs: refs})
		}
	}
	if !reflect.DeepEqual(before, RuntimeOperationDescriptors()) {
		t.Fatal("validation input targets 改写静态 descriptor 库存")
	}
	copies := RuntimeOperationDescriptors()
	for index := range copies {
		copies[index].OwnerModuleID = "tampered"
		if len(copies[index].AllowedStates) != 0 {
			copies[index].AllowedStates[0] = "tampered"
		}
	}
	if !reflect.DeepEqual(before, RuntimeOperationDescriptors()) {
		t.Fatal("公开库存副本泄漏内部可变集合")
	}
	value := RuntimeValidationSurface{InputTargetOwnerModuleID: RuntimeModuleFile, InputTargetRefs: []string{"input"}}
	assertValidationInputTargetsInventoryReference(t, value)
	value.InputTargetOwnerModuleID = "unknown"
	if validateRuntimeValidationInputTargets(value) == nil {
		t.Fatal("后续 owner 变化借用先前查询成功")
	}
	assertValidationInputTargetsInventoryReference(t, value)
	value.InputTargetOwnerModuleID = RuntimeModuleFile
	value.InputTargetRefs = []string{"input", "input"}
	if validateRuntimeValidationInputTargets(value) == nil {
		t.Fatal("后续 duplicate target 未重新拒绝")
	}
}

func TestValidationInputTargetsReadOnlyInventoryRemovesDiscardedCopies(t *testing.T) {
	value := RuntimeValidationSurface{InputTargetOwnerModuleID: RuntimeModuleFile, InputTargetRefs: []string{"input"}}
	original := testing.AllocsPerRun(50, func() { _ = validationInputTargetsPublicInventoryReference(value) })
	current := testing.AllocsPerRun(50, func() { _ = validateRuntimeValidationInputTargets(value) })
	if current >= original {
		t.Fatalf("未去除丢弃的库存副本：current=%v original=%v", current, original)
	}
}

func FuzzValidationInputTargetsReadOnlyInventory(f *testing.F) {
	f.Add(RuntimeModuleFile, "input", "other")
	f.Add("unknown", "\x00", "input")
	f.Add("", "", "")
	f.Fuzz(func(t *testing.T, owner, first, second string) {
		assertValidationInputTargetsInventoryReference(t, RuntimeValidationSurface{InputTargetOwnerModuleID: owner, InputTargetRefs: []string{first, second}})
	})
}

func BenchmarkValidationInputTargetsInventory(b *testing.B) {
	value := RuntimeValidationSurface{InputTargetOwnerModuleID: RuntimeModuleFile, InputTargetRefs: []string{"input"}}
	for _, mode := range []string{"reference", "current"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if mode == "reference" {
					_ = validationInputTargetsPublicInventoryReference(value)
				} else {
					_ = validateRuntimeValidationInputTargets(value)
				}
			}
		})
	}
}
