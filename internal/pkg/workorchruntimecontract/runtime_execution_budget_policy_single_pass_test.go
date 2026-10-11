package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// 修改前完整 validator/hash 路径，保留第二次规范化与原字节编码作为独立对照。
func referenceRuntimeBudgetHashString(h interface{ Write([]byte) (int, error) }, value string) {
	encoded := make([]byte, 8+len(value))
	binary.BigEndian.PutUint64(encoded[:8], uint64(len(value)))
	copy(encoded[8:], value)
	_, _ = h.Write(encoded)
}

func referenceRuntimeExecutionBudgetPolicyHash(policy RuntimeExecutionBudgetPolicy) string {
	p := policy.Normalized()
	h := sha256.New()
	referenceRuntimeBudgetHashString(h, "runtime-execution-budget-policy")
	referenceRuntimeBudgetHashString(h, p.SchemaVersion)
	referenceRuntimeBudgetHashString(h, p.PolicyRevision)
	referenceRuntimeBudgetHashString(h, string(p.Scope))
	referenceRuntimeBudgetHashString(h, string(p.RestartDisposition))
	for _, dimension := range p.Dimensions {
		referenceRuntimeBudgetHashString(h, string(dimension.Dimension))
		referenceRuntimeBudgetHashString(h, dimension.Unit)
		referenceRuntimeBudgetHashString(h, fmt.Sprintf("%d", dimension.Limit))
		referenceRuntimeBudgetHashString(h, fmt.Sprintf("%d", dimension.MaxIncrement))
		referenceRuntimeBudgetHashString(h, string(dimension.ExhaustionDisposition))
	}
	return "runtime-budget-policy-" + hex.EncodeToString(h.Sum(nil))[:20]
}

func referenceRuntimeExecutionBudgetPolicyValidate(p RuntimeExecutionBudgetPolicy) error {
	p = p.Normalized()
	if p.SchemaVersion != RuntimeExecutionBudgetSchemaCurrent || p.PolicyRevision == "" || len(p.Dimensions) == 0 || p.PolicyHash == "" {
		return fmt.Errorf("runtime execution budget policy is incomplete")
	}
	if p.Scope != RuntimeBudgetScopeWorkspace && p.Scope != RuntimeBudgetScopeRun {
		return fmt.Errorf("runtime execution budget policy has invalid scope")
	}
	switch p.RestartDisposition {
	case RuntimeBudgetRestartFreshFromSuccessorPolicy, RuntimeBudgetRestartZero, RuntimeBudgetRestartExplicitLimit:
	default:
		return fmt.Errorf("runtime execution budget policy has invalid restart disposition")
	}
	seen := map[RuntimeBudgetDimension]struct{}{}
	for _, dimension := range p.Dimensions {
		if err := dimension.Validate(); err != nil {
			return err
		}
		if _, duplicate := seen[dimension.Dimension]; duplicate {
			return fmt.Errorf("runtime execution budget dimension %q is duplicated", dimension.Dimension)
		}
		seen[dimension.Dimension] = struct{}{}
	}
	if p.PolicyHash != referenceRuntimeExecutionBudgetPolicyHash(p) {
		return fmt.Errorf("runtime execution budget policy hash mismatch")
	}
	return nil
}

func assertRuntimeExecutionBudgetPolicyReference(t testing.TB, policy RuntimeExecutionBudgetPolicy) {
	t.Helper()
	before := policy
	if policy.Dimensions != nil {
		before.Dimensions = append(make([]RuntimeBudgetDimensionPolicy, 0, len(policy.Dimensions)), policy.Dimensions...)
	}
	for range 2 {
		if got, want := StableRuntimeExecutionBudgetPolicyHash(policy), referenceRuntimeExecutionBudgetPolicyHash(policy); got != want {
			t.Fatalf("完整预算 policy hash 漂移: got=%s want=%s", got, want)
		}
		got, want := policy.Validate(), referenceRuntimeExecutionBudgetPolicyValidate(policy)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("预算接受／拒绝原因漂移: got=%v want=%v", got, want)
		}
		if !reflect.DeepEqual(policy, before) {
			t.Fatal("hash／校验改写原始预算输入")
		}
	}
}

func TestRuntimeExecutionBudgetPolicySinglePassMatchesReference(t *testing.T) {
	for _, scenario := range []string{"default", "raw", "long", "nil", "empty", "scope", "restart", "schema", "revision", "duplicate", "dimension", "unit", "limit", "increment", "exhaustion", "hash"} {
		t.Run(scenario, func(t *testing.T) {
			policy := DefaultRuntimeExecutionBudgetPolicy()
			switch scenario {
			case "raw":
				policy.PolicyRevision = " 原文\r\n "
				policy.Dimensions[0].Unit = " token "
				for left, right := 0, len(policy.Dimensions)-1; left < right; left, right = left+1, right-1 {
					policy.Dimensions[left], policy.Dimensions[right] = policy.Dimensions[right], policy.Dimensions[left]
				}
			case "long":
				policy.PolicyRevision = strings.Repeat("原文\x00", 4096)
				policy.Dimensions[0].Unit = string([]byte{0xff, 0x80})
			case "nil":
				policy.Dimensions = nil
			case "empty":
				policy.Dimensions = []RuntimeBudgetDimensionPolicy{}
			case "scope":
				policy.Scope = "unsupported"
			case "restart":
				policy.RestartDisposition = "unsupported"
			case "schema":
				policy.SchemaVersion = "unsupported"
			case "revision":
				policy.PolicyRevision = " "
			case "duplicate":
				policy.Dimensions = append(policy.Dimensions, policy.Dimensions[0])
			case "dimension":
				policy.Dimensions[0].Dimension = "unsupported"
			case "unit":
				policy.Dimensions[0].Unit = " "
			case "limit":
				policy.Dimensions[0].Limit = RuntimeBudgetAmountLimit + 1
			case "increment":
				policy.Dimensions[0].MaxIncrement = policy.Dimensions[0].Limit + 1
			case "exhaustion":
				policy.Dimensions[0].ExhaustionDisposition = "unsupported"
			}
			policy.PolicyHash = referenceRuntimeExecutionBudgetPolicyHash(policy)
			if scenario == "hash" {
				policy.PolicyHash = "wrong"
			}
			assertRuntimeExecutionBudgetPolicyReference(t, policy)
			encoded, err := json.Marshal(policy)
			if err != nil {
				t.Fatal(err)
			}
			var loaded RuntimeExecutionBudgetPolicy
			if err := json.Unmarshal(encoded, &loaded); err != nil {
				t.Fatal(err)
			}
			assertRuntimeExecutionBudgetPolicyReference(t, loaded)
		})
	}
	policy := DefaultRuntimeExecutionBudgetPolicy()
	if err := policy.Validate(); err != nil {
		t.Fatal(err)
	}
	policy.Dimensions[0].Limit++
	if err := policy.Validate(); err == nil {
		t.Fatal("后续调用借用先前成功，未拒绝完整正文漂移")
	}
	policy = DefaultRuntimeExecutionBudgetPolicy()
	clone := policy.Clone()
	clone.Dimensions[0].Limit++
	if policy.Dimensions[0].Limit == clone.Dimensions[0].Limit || policy.Validate() != nil {
		t.Fatal("公开 detached 副本污染原 policy")
	}
}

func FuzzRuntimeExecutionBudgetPolicySinglePass(f *testing.F) {
	for _, revision := range []string{"", " 原文\r\n ", "A\x00B", string([]byte{0xff, 0x80}), strings.Repeat("x", 121)} {
		f.Add(revision, "token", int64(512), int64(1), uint8(0))
	}
	f.Fuzz(func(t *testing.T, revision, unit string, limit, increment int64, mode uint8) {
		policy := DefaultRuntimeExecutionBudgetPolicy()
		policy.PolicyRevision = revision
		policy.Dimensions[0].Unit, policy.Dimensions[0].Limit, policy.Dimensions[0].MaxIncrement = unit, limit, increment
		switch mode % 4 {
		case 1:
			policy.Dimensions = append(policy.Dimensions, policy.Dimensions[0])
		case 2:
			policy.Scope = RuntimeBudgetScope(revision)
		case 3:
			policy.Dimensions[0].ExhaustionDisposition = RuntimeBudgetExhaustionDisposition(unit)
		}
		policy.PolicyHash = referenceRuntimeExecutionBudgetPolicyHash(policy)
		assertRuntimeExecutionBudgetPolicyReference(t, policy)
	})
}

func BenchmarkRuntimeExecutionBudgetPolicyValidationSinglePass(b *testing.B) {
	policy := DefaultRuntimeExecutionBudgetPolicy()
	for _, reference := range []bool{true, false} {
		b.Run(fmt.Sprintf("reference=%t", reference), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				var err error
				if reference {
					err = referenceRuntimeExecutionBudgetPolicyValidate(policy)
				} else {
					err = policy.Validate()
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
