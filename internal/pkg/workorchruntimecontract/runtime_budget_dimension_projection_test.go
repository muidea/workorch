package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func referenceRuntimeBudgetDimensionPolicyFor(policy RuntimeExecutionBudgetPolicy, dimension RuntimeBudgetDimension) (RuntimeBudgetDimensionPolicy, bool) {
	for _, value := range policy.Normalized().Dimensions {
		if value.Dimension == dimension {
			return value, true
		}
	}
	return RuntimeBudgetDimensionPolicy{}, false
}

func assertBudgetDimensionProjectionReference(t testing.TB, policy RuntimeExecutionBudgetPolicy, dimension RuntimeBudgetDimension) {
	t.Helper()
	before := policy
	if policy.Dimensions != nil {
		before.Dimensions = append(make([]RuntimeBudgetDimensionPolicy, 0, len(policy.Dimensions)), policy.Dimensions...)
	}
	want, wantFound := referenceRuntimeBudgetDimensionPolicyFor(policy, dimension)
	for range 2 {
		got, found := RuntimeBudgetDimensionPolicyFor(policy, dimension)
		if got != want || found != wantFound {
			t.Fatalf("预算 dimension 首个匹配／完整标量投影漂移：got=%+v/%v want=%+v/%v", got, found, want, wantFound)
		}
	}
	if !reflect.DeepEqual(policy, before) {
		t.Fatal("只读 dimension 查询改写原 policy")
	}
	body, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded RuntimeExecutionBudgetPolicy
	if err := json.Unmarshal(body, &reloaded); err != nil {
		t.Fatal(err)
	}
	got, found := RuntimeBudgetDimensionPolicyFor(reloaded, dimension)
	want, wantFound = referenceRuntimeBudgetDimensionPolicyFor(reloaded, dimension)
	if got != want || found != wantFound {
		t.Fatal("预算 dimension 重载后投影漂移")
	}
}

func TestBudgetDimensionProjectionKeepsOriginalStableFirstMatch(t *testing.T) {
	original := DefaultRuntimeExecutionBudgetPolicy()
	for _, scenario := range []string{"valid", "reverse", "duplicate", "trim", "nil", "empty", "unknown-policy", "invalid-values"} {
		t.Run(scenario, func(t *testing.T) {
			policy := original.Clone()
			switch scenario {
			case "reverse":
				for left, right := 0, len(policy.Dimensions)-1; left < right; left, right = left+1, right-1 {
					policy.Dimensions[left], policy.Dimensions[right] = policy.Dimensions[right], policy.Dimensions[left]
				}
			case "duplicate":
				second := policy.Dimensions[0]
				second.Limit++
				policy.Dimensions = append([]RuntimeBudgetDimensionPolicy{second}, policy.Dimensions...)
			case "trim":
				for index := range policy.Dimensions {
					policy.Dimensions[index].Dimension = " " + policy.Dimensions[index].Dimension + " "
					policy.Dimensions[index].Unit = " " + policy.Dimensions[index].Unit + " "
					policy.Dimensions[index].ExhaustionDisposition = " " + policy.Dimensions[index].ExhaustionDisposition + " "
				}
			case "nil":
				policy.Dimensions = nil
			case "empty":
				policy.Dimensions = []RuntimeBudgetDimensionPolicy{}
			case "unknown-policy":
				policy.SchemaVersion, policy.PolicyHash = "unknown", "different"
			case "invalid-values":
				policy.Dimensions[0].Limit, policy.Dimensions[0].MaxIncrement = -1, -2
				policy.Dimensions[0].ExhaustionDisposition = "unknown"
			}
			for _, dimension := range append(append([]RuntimeBudgetDimensionPolicy(nil), original.Dimensions...), RuntimeBudgetDimensionPolicy{}, RuntimeBudgetDimensionPolicy{Dimension: "unknown"}, RuntimeBudgetDimensionPolicy{Dimension: " provider_call "}) {
				assertBudgetDimensionProjectionReference(t, policy, dimension.Dimension)
			}
			assertRuntimeExecutionBudgetPolicyReference(t, policy)
		})
	}
	policy := original.Clone()
	wanted := policy.Dimensions[0].Dimension
	got, found := RuntimeBudgetDimensionPolicyFor(policy, wanted)
	if !found {
		t.Fatal("有效 dimension 未命中")
	}
	got.Limit++
	if got.Limit == policy.Dimensions[0].Limit {
		t.Fatal("标量投影泄漏原值")
	}
	policy.Dimensions[0].Limit++
	after, found := RuntimeBudgetDimensionPolicyFor(policy, wanted)
	if !found || after.Limit != policy.Dimensions[0].Limit {
		t.Fatal("后续查询未读取原新事实")
	}
	if policy.Validate() == nil {
		t.Fatal("只读投影成功被错误当作完整 policy 校验凭据")
	}
}

func referenceRuntimeBudgetAllocationProjectionValidate(a RuntimeBudgetAllocation, policy RuntimeExecutionBudgetPolicy) error {
	a = a.Normalized()
	dimension, found := referenceRuntimeBudgetDimensionPolicyFor(policy, a.Dimension)
	if !found || a.AllocationID == "" || a.Unit != dimension.Unit || a.LogicalConsumptionID == "" || a.ScopeRef == "" ||
		a.PolicyRevision != policy.Normalized().PolicyRevision || a.LimitAtAllocation != dimension.Limit || a.MaxIncrement != dimension.MaxIncrement ||
		a.ReservedAmount <= 0 || a.ReservedAmount > a.MaxIncrement || a.AllocationHash == "" {
		return fmt.Errorf("runtime budget allocation is incomplete or does not match policy")
	}
	switch a.State {
	case RuntimeBudgetAllocationReserved:
		if a.OwnerFactRef != "" {
			return fmt.Errorf("reserved runtime budget allocation cannot carry owner fact")
		}
	case RuntimeBudgetAllocationConfirmed, RuntimeBudgetAllocationReleased, RuntimeBudgetAllocationUnknown, RuntimeBudgetAllocationExhausted:
		if a.OwnerFactRef == "" {
			return fmt.Errorf("terminal runtime budget allocation requires owner fact")
		}
	default:
		return fmt.Errorf("runtime budget allocation has invalid state %q", a.State)
	}
	if a.AllocationID != StableRuntimeBudgetAllocationID(a) || a.AllocationHash != StableRuntimeBudgetAllocationHash(a) {
		return fmt.Errorf("runtime budget allocation identity mismatch")
	}
	return nil
}

func budgetAllocationProjectionFixture(policy RuntimeExecutionBudgetPolicy) RuntimeBudgetAllocation {
	dimension, _ := referenceRuntimeBudgetDimensionPolicyFor(policy, RuntimeBudgetDimensionProviderCall)
	value := RuntimeBudgetAllocation{Dimension: dimension.Dimension, Unit: dimension.Unit, LogicalConsumptionID: "logical", ScopeRef: "scope", PolicyRevision: policy.PolicyRevision, LimitAtAllocation: dimension.Limit, MaxIncrement: dimension.MaxIncrement, ReservedAmount: 1, State: RuntimeBudgetAllocationReserved}
	value.AllocationID = StableRuntimeBudgetAllocationID(value)
	value.AllocationHash = StableRuntimeBudgetAllocationHash(value)
	return value
}

func TestBudgetDimensionProjectionKeepsAllocationCompleteGate(t *testing.T) {
	policy := DefaultRuntimeExecutionBudgetPolicy()
	dimension, _ := referenceRuntimeBudgetDimensionPolicyFor(policy, RuntimeBudgetDimensionProviderCall)
	original := budgetAllocationProjectionFixture(policy)
	for _, scenario := range []string{"valid", "trim-revision", "revision-drift", "limit", "increment", "unit", "state", "owner-fact", "hash", "id", "logical", "scope", "zero", "excess", "confirmed", "released", "unknown", "exhausted", "terminal-no-fact", "duplicate-policy", "duplicate-later", "policy-hash", "missing-dimension"} {
		t.Run(scenario, func(t *testing.T) {
			value, candidatePolicy := original, policy.Clone()
			switch scenario {
			case "trim-revision":
				candidatePolicy.PolicyRevision = " " + candidatePolicy.PolicyRevision + " "
			case "revision-drift":
				candidatePolicy.PolicyRevision = "different"
			case "limit":
				value.LimitAtAllocation++
			case "increment":
				value.MaxIncrement++
			case "unit":
				value.Unit = "different"
			case "state":
				value.State = "unknown-state"
			case "owner-fact":
				value.OwnerFactRef = "unexpected"
			case "hash":
				value.AllocationHash = "different"
			case "id":
				value.AllocationID = "different"
			case "logical":
				value.LogicalConsumptionID = ""
			case "scope":
				value.ScopeRef = ""
			case "zero":
				value.ReservedAmount = 0
			case "excess":
				value.ReservedAmount = value.MaxIncrement + 1
			case "confirmed", "released", "unknown", "exhausted":
				value.State = RuntimeBudgetAllocationState(scenario)
				value.OwnerFactRef = "owner-fact"
				value.AllocationHash = StableRuntimeBudgetAllocationHash(value)
			case "terminal-no-fact":
				value.State = RuntimeBudgetAllocationConfirmed
			case "duplicate-later":
				other := dimension
				other.Limit++
				candidatePolicy.Dimensions = append(candidatePolicy.Dimensions, other)
			case "policy-hash":
				candidatePolicy.PolicyHash = "different"
			case "duplicate-policy":
				other := dimension
				other.Limit++
				candidatePolicy.Dimensions = append([]RuntimeBudgetDimensionPolicy{other}, candidatePolicy.Dimensions...)
			case "missing-dimension":
				candidatePolicy.Dimensions = nil
			}
			beforeValue, beforePolicy := value, candidatePolicy
			beforePolicy.Dimensions = append([]RuntimeBudgetDimensionPolicy(nil), candidatePolicy.Dimensions...)
			want := referenceRuntimeBudgetAllocationProjectionValidate(value, candidatePolicy)
			expectValid := scenario == "valid" || scenario == "trim-revision" || scenario == "duplicate-later" || scenario == "policy-hash" || scenario == "confirmed" || scenario == "released" || scenario == "unknown" || scenario == "exhausted"
			if (want == nil) != expectValid {
				t.Fatalf("allocation 反例未命中：%s: %v", scenario, want)
			}
			assertProofCollectionIdentityErrors(t, value.Validate(candidatePolicy), want)
			if value != beforeValue || !reflect.DeepEqual(candidatePolicy, beforePolicy) {
				t.Fatal("allocation 校验改写原输入")
			}
			if (scenario == "duplicate-later" || scenario == "policy-hash") && candidatePolicy.Validate() == nil {
				t.Fatal("allocation 查询成功替代完整 policy 门禁")
			}
			body, err := json.Marshal(struct {
				Allocation RuntimeBudgetAllocation
				Policy     RuntimeExecutionBudgetPolicy
			}{value, candidatePolicy})
			if err != nil {
				t.Fatal(err)
			}
			var loaded struct {
				Allocation RuntimeBudgetAllocation
				Policy     RuntimeExecutionBudgetPolicy
			}
			if err := json.Unmarshal(body, &loaded); err != nil {
				t.Fatal(err)
			}
			assertProofCollectionIdentityErrors(t, loaded.Allocation.Validate(loaded.Policy), referenceRuntimeBudgetAllocationProjectionValidate(loaded.Allocation, loaded.Policy))
		})
	}
	before := original
	if err := original.Validate(policy); err != nil {
		t.Fatal(err)
	}
	if original != before {
		t.Fatal("allocation 查询改写原记录")
	}
	original.ReservedAmount++
	if original.Validate(policy) == nil {
		t.Fatal("后续 allocation 超增量借用先前成功")
	}
}

func TestBudgetDimensionProjectionRemovesDiscardedPolicyCopies(t *testing.T) {
	policy := DefaultRuntimeExecutionBudgetPolicy()
	original := testing.AllocsPerRun(20, func() { _, _ = referenceRuntimeBudgetDimensionPolicyFor(policy, RuntimeBudgetDimensionProviderCall) })
	current := testing.AllocsPerRun(20, func() { _, _ = RuntimeBudgetDimensionPolicyFor(policy, RuntimeBudgetDimensionProviderCall) })
	if current != 0 || current >= original {
		t.Fatalf("dimension 查询仍复制 policy：current=%v original=%v", current, original)
	}
	allocation := budgetAllocationProjectionFixture(policy)
	original = testing.AllocsPerRun(20, func() { _ = referenceRuntimeBudgetAllocationProjectionValidate(allocation, policy) })
	current = testing.AllocsPerRun(20, func() { _ = allocation.Validate(policy) })
	if current >= original {
		t.Fatalf("allocation 校验仍复制无关 policy：current=%v original=%v", current, original)
	}
}

func FuzzBudgetAllocationProjectionReference(f *testing.F) {
	f.Add(" revision ", "reserved", "", int64(1), uint8(0))
	f.Add(string([]byte{0xff, 0x80}), "unknown", "fact", int64(-1), uint8(1))
	f.Fuzz(func(t *testing.T, revision, state, ownerFact string, amount int64, mode uint8) {
		policy := DefaultRuntimeExecutionBudgetPolicy()
		value := budgetAllocationProjectionFixture(policy)
		policy.PolicyRevision, value.PolicyRevision = revision, revision
		value.State, value.OwnerFactRef, value.ReservedAmount = RuntimeBudgetAllocationState(state), ownerFact, amount
		value.AllocationID = StableRuntimeBudgetAllocationID(value)
		value.AllocationHash = StableRuntimeBudgetAllocationHash(value)
		if mode%3 == 1 {
			policy.Dimensions = nil
		}
		if mode%3 == 2 {
			policy.PolicyRevision += "different"
		}
		assertProofCollectionIdentityErrors(t, value.Validate(policy), referenceRuntimeBudgetAllocationProjectionValidate(value, policy))
	})
}

func FuzzBudgetDimensionProjectionReference(f *testing.F) {
	f.Add(" provider_call ", " call ", " terminal ", int64(1), int64(2), uint8(0))
	f.Add("", string([]byte{0xff, 0x80}), "\x00", int64(-1), int64(0), uint8(1))
	f.Fuzz(func(t *testing.T, dimension, unit, exhaustion string, limit, increment int64, mode uint8) {
		policy := DefaultRuntimeExecutionBudgetPolicy()
		value := RuntimeBudgetDimensionPolicy{Dimension: RuntimeBudgetDimension(dimension), Unit: unit, Limit: limit, MaxIncrement: increment, ExhaustionDisposition: RuntimeBudgetExhaustionDisposition(exhaustion)}
		if mode%2 == 0 {
			policy.Dimensions = append([]RuntimeBudgetDimensionPolicy{value}, policy.Dimensions...)
		} else {
			policy.Dimensions = append(policy.Dimensions, value, value)
		}
		for _, wanted := range []RuntimeBudgetDimension{RuntimeBudgetDimension(dimension), RuntimeBudgetDimension(strings.TrimSpace(dimension)), RuntimeBudgetDimensionProviderCall} {
			assertBudgetDimensionProjectionReference(t, policy, wanted)
		}
	})
}

func TestBudgetAllocationProjectionKeepsRejectionOrder(t *testing.T) {
	policy := DefaultRuntimeExecutionBudgetPolicy()
	for _, scenario := range []string{"policy-before-state", "state-before-identity", "owner-fact-before-identity", "terminal-fact-before-identity"} {
		t.Run(scenario, func(t *testing.T) {
			value := budgetAllocationProjectionFixture(policy)
			value.AllocationHash = "different"
			var expected string
			switch scenario {
			case "policy-before-state":
				value.PolicyRevision, value.State = "different", "invalid"
				expected = "runtime budget allocation is incomplete or does not match policy"
			case "state-before-identity":
				value.State = "invalid"
				expected = "runtime budget allocation has invalid state \"invalid\""
			case "owner-fact-before-identity":
				value.OwnerFactRef = "unexpected"
				expected = "reserved runtime budget allocation cannot carry owner fact"
			case "terminal-fact-before-identity":
				value.State = RuntimeBudgetAllocationUnknown
				expected = "terminal runtime budget allocation requires owner fact"
			}
			want := referenceRuntimeBudgetAllocationProjectionValidate(value, policy)
			if want == nil || want.Error() != expected {
				t.Fatalf("旧门禁未命中预期错误：%v", want)
			}
			assertProofCollectionIdentityErrors(t, value.Validate(policy), want)
		})
	}
}

func BenchmarkBudgetAllocationProjection(b *testing.B) {
	policy := DefaultRuntimeExecutionBudgetPolicy()
	value := budgetAllocationProjectionFixture(policy)
	for _, mode := range []string{"reference", "current"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				var err error
				if mode == "reference" {
					err = referenceRuntimeBudgetAllocationProjectionValidate(value, policy)
				} else {
					err = value.Validate(policy)
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkBudgetDimensionProjection(b *testing.B) {
	policy := DefaultRuntimeExecutionBudgetPolicy()
	for _, mode := range []string{"reference", "current"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if mode == "reference" {
					_, _ = referenceRuntimeBudgetDimensionPolicyFor(policy, RuntimeBudgetDimensionProviderCall)
				} else {
					_, _ = RuntimeBudgetDimensionPolicyFor(policy, RuntimeBudgetDimensionProviderCall)
				}
			}
		})
	}
}
