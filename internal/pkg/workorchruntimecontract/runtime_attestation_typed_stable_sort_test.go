package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"testing"
)

func referenceAttestationStableSort(values []RuntimeValidationCheckAttestationBinding) []RuntimeValidationCheckAttestationBinding {
	ret := make([]RuntimeValidationCheckAttestationBinding, 0, len(values))
	for _, raw := range values {
		value := raw.Normalized()
		if value.RequiredCheckRef != "" {
			ret = append(ret, value)
		}
	}
	sort.SliceStable(ret, func(i, j int) bool {
		if ret[i].RequiredCheckRef != ret[j].RequiredCheckRef {
			return ret[i].RequiredCheckRef < ret[j].RequiredCheckRef
		}
		if ret[i].ObservationProtocolRef != ret[j].ObservationProtocolRef {
			return ret[i].ObservationProtocolRef < ret[j].ObservationProtocolRef
		}
		return ret[i].NativeCheckRef < ret[j].NativeCheckRef
	})
	return nilIfEmpty(ret)
}

func assertAttestationStableSortReference(t testing.TB, values []RuntimeValidationCheckAttestationBinding) {
	t.Helper()
	before := cloneRuntimeStateRawSlice(values, func(value RuntimeValidationCheckAttestationBinding) RuntimeValidationCheckAttestationBinding {
		return value
	})
	want, got := referenceAttestationStableSort(values), NormalizeRuntimeValidationCheckAttestationBindings(values)
	if !reflect.DeepEqual(got, want) {
		t.Fatal("attestation typed sort 改变原三字段排序或同 key 顺序")
	}
	if !reflect.DeepEqual(CloneRuntimeValidationCheckAttestationBindings(values), want) {
		t.Fatal("公开 Clone 改变原排序")
	}
	if len(got) != 0 {
		got[0].SupplementalNativeCheckRef = "changed-return"
	}
	if !reflect.DeepEqual(values, before) {
		t.Fatal("返回集合污染原 attestation")
	}
	if !reflect.DeepEqual(NormalizeRuntimeValidationCheckAttestationBindings(values), want) {
		t.Fatal("返回副本污染后续调用")
	}
	body, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	var loaded []RuntimeValidationCheckAttestationBinding
	if err := json.Unmarshal(body, &loaded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(NormalizeRuntimeValidationCheckAttestationBindings(loaded), referenceAttestationStableSort(loaded)) {
		t.Fatal("重载后排序漂移")
	}
}

func attestationStableSortFixture(count int) []RuntimeValidationCheckAttestationBinding {
	var values []RuntimeValidationCheckAttestationBinding
	for index := range count {
		values = append(values, RuntimeValidationCheckAttestationBinding{
			RequiredCheckRef: fmt.Sprintf(" check-%d ", (count-index)%3), ObservationProtocolRef: fmt.Sprintf("protocol-%d", index%2),
			NativeCheckRef: fmt.Sprintf("native-%d", index%2), SupplementalNativeCheckRef: fmt.Sprintf("ordinal-%03d", index), ExecutionStagesHash: " raw-stage-hash ",
		})
	}
	return values
}

func TestAttestationTypedStableSortKeepsOriginalKeysAndIsolation(t *testing.T) {
	for _, count := range []int{0, 1, 2, 32, 128} {
		values := attestationStableSortFixture(count)
		assertAttestationStableSortReference(t, values)
		if len(values) != 0 {
			values[0].RequiredCheckRef = "later"
			assertAttestationStableSortReference(t, values)
		}
	}
	assertAttestationStableSortReference(t, []RuntimeValidationCheckAttestationBinding{})
	assertAttestationStableSortReference(t, []RuntimeValidationCheckAttestationBinding{
		{RequiredCheckRef: "b", ObservationProtocolRef: "a", NativeCheckRef: "a"},
		{RequiredCheckRef: "a", ObservationProtocolRef: "b", NativeCheckRef: "a"},
		{RequiredCheckRef: "a", ObservationProtocolRef: "a", NativeCheckRef: "b"},
		{RequiredCheckRef: "a", ObservationProtocolRef: "a", NativeCheckRef: "a", ExecutionStagesHash: " raw-first "},
		{RequiredCheckRef: "a", ObservationProtocolRef: "a", NativeCheckRef: "a", ExecutionStagesHash: " raw-second "},
	})
	assertAttestationStableSortReference(t, []RuntimeValidationCheckAttestationBinding{
		{RequiredCheckRef: " ", NativeCheckRef: "native"},
		{RequiredCheckRef: "\x00", ObservationProtocolRef: string([]byte{0xff, 0x80}), NativeCheckRef: "中文"},
		{RequiredCheckRef: "check", ObservationProtocolRef: "protocol", NativeCheckRef: "native", SupplementalNativeCheckRef: "first"},
		{RequiredCheckRef: " check ", ObservationProtocolRef: " protocol ", NativeCheckRef: " native ", SupplementalNativeCheckRef: "second"},
	})
}

func TestAttestationTypedStableSortDoesNotLaunderDuplicateAuthority(t *testing.T) {
	values := runtimeValidationCheckAttestationsForTest([]string{"check"})
	if err := ValidateRuntimeValidationCheckAttestationBindings(values, []string{"check"}); err != nil {
		t.Fatal(err)
	}
	values = append(values, values[0])
	assertAttestationStableSortReference(t, values)
	if len(NormalizeRuntimeValidationCheckAttestationBindings(values)) != 2 {
		t.Fatal("排序去除了重复 attestation")
	}
	if err := ValidateRuntimeValidationCheckAttestationBindings(values, []string{"check"}); err == nil {
		t.Fatal("重复原 attestation 未拒绝")
	}
}

func TestAttestationTypedStableSortRemovesOnlyReflectionAllocations(t *testing.T) {
	values := attestationStableSortFixture(32)
	original := testing.AllocsPerRun(20, func() { _ = referenceAttestationStableSort(values) })
	current := testing.AllocsPerRun(20, func() { _ = NormalizeRuntimeValidationCheckAttestationBindings(values) })
	if current >= original {
		t.Fatalf("attestation 反射分配未减少：current=%v original=%v", current, original)
	}
}

func FuzzAttestationTypedStableSortReference(f *testing.F) {
	f.Add(" check ", "protocol", "native", "supplement", uint8(32))
	f.Add(" ", string([]byte{0xff, 0x80}), "\x00", "中文", uint8(64))
	f.Fuzz(func(t *testing.T, check, protocol, native, supplement string, count uint8) {
		values := attestationStableSortFixture(int(count))
		for index := range values {
			if index%3 == 0 {
				values[index].RequiredCheckRef = check
			}
			if index%2 == 0 {
				values[index].ObservationProtocolRef, values[index].NativeCheckRef = protocol, native
			}
			values[index].SupplementalNativeCheckRef = supplement
		}
		assertAttestationStableSortReference(t, values)
	})
}

func BenchmarkAttestationTypedStableSort(b *testing.B) {
	for _, count := range []int{1, 2, 32} {
		values := attestationStableSortFixture(count)
		for _, mode := range []string{"reference", "current"} {
			b.Run(fmt.Sprintf("%s/%d", mode, count), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if mode == "reference" {
						_ = referenceAttestationStableSort(values)
					} else {
						_ = NormalizeRuntimeValidationCheckAttestationBindings(values)
					}
				}
			})
		}
	}
}
