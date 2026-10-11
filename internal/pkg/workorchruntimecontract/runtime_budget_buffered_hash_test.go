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

	"github.com/muidea/workorch/internal/pkg/common"
)

// 独立旧编码保留每字段长度前缀、正文 bytes 和非缓冲 SHA256 路径。
func referenceBudgetWriteHashString(w hashStringWriter, field string) {
	encoded := make([]byte, 8+len(field))
	binary.BigEndian.PutUint64(encoded[:8], uint64(len(field)))
	copy(encoded[8:], field)
	_, _ = w.Write(encoded)
}

func referenceBudgetHashEncoding(prefix string, fields ...string) string {
	h := sha256.New()
	for _, field := range fields {
		referenceBudgetWriteHashString(h, field)
	}
	return prefix + hex.EncodeToString(h.Sum(nil))[:20]
}

func referenceBudgetAllocationID(value RuntimeBudgetAllocation) string {
	a := value.Normalized()
	return referenceBudgetHashEncoding("runtime-budget-allocation-", "runtime-budget-allocation", a.PolicyRevision, string(a.Dimension), a.LogicalConsumptionID, a.ScopeRef)
}

func referenceBudgetAllocationHash(value RuntimeBudgetAllocation) string {
	a := value.Normalized()
	return referenceBudgetHashEncoding("runtime-budget-allocation-value-", "runtime-budget-allocation-value", a.AllocationID, string(a.Dimension), a.Unit, a.LogicalConsumptionID, a.ScopeRef, a.PolicyRevision,
		fmt.Sprintf("%d", a.LimitAtAllocation), fmt.Sprintf("%d", a.ReservedAmount), fmt.Sprintf("%d", a.MaxIncrement), string(a.State), a.OwnerFactRef)
}

func referenceBudgetConsumptionID(value RuntimeBudgetConsumptionFact) string {
	f := value.Normalized()
	return referenceBudgetHashEncoding("runtime-budget-consumption-", "runtime-budget-consumption", f.AllocationID, f.Owner, f.PhysicalConsumptionID)
}

func referenceBudgetConsumptionHash(value RuntimeBudgetConsumptionFact) string {
	f := value.Normalized()
	return referenceBudgetHashEncoding("runtime-budget-consumption-value-", "runtime-budget-consumption-value", f.ConsumptionID, f.AllocationID, f.Owner, f.LogicalConsumptionID, f.PhysicalConsumptionID, f.Unit,
		fmt.Sprintf("%d", f.ConfirmedAmount), fmt.Sprintf("%d", f.UnknownAmount), f.Currency, fmt.Sprintf("%d", f.MonetaryAmount), f.OwnerResultRef, f.OwnerResultHash)
}

func referenceBudgetReconciliationID(value RuntimeBudgetReconciliationRecord) string {
	r := value.Normalized()
	return referenceBudgetHashEncoding("runtime-budget-reconciliation-", "runtime-budget-reconciliation", r.AllocationID, r.ProbeKind, r.ProbeOwner, r.ProbeRequestHash, r.ExpectedOwnerFence)
}

func referenceBudgetReconciliationHash(value RuntimeBudgetReconciliationRecord) string {
	r := value.Normalized()
	return referenceBudgetHashEncoding("runtime-budget-reconciliation-value-", "runtime-budget-reconciliation-value", r.ReconciliationID, r.AllocationID, r.ProbeKind, r.ProbeOwner, r.ProbeRequestHash, r.ExpectedOwnerFence, string(r.Status), r.OwnerResultHash)
}

func referenceBudgetViewHash(view RuntimeExecutionBudgetView) string {
	h := sha256.New()
	referenceBudgetWriteHashString(h, "runtime-execution-budget-view")
	referenceBudgetWriteHashString(h, string(view.WorkspaceID))
	referenceBudgetWriteHashString(h, string(view.RunID))
	referenceBudgetWriteHashString(h, view.PolicyRevision)
	referenceBudgetWriteHashString(h, fmt.Sprintf("%d", view.StateRevision))
	for _, dimension := range view.Dimensions {
		referenceBudgetWriteHashString(h, string(dimension.Dimension))
		referenceBudgetWriteHashString(h, dimension.Unit)
		referenceBudgetWriteHashString(h, fmt.Sprintf("%d", dimension.Limit))
		referenceBudgetWriteHashString(h, fmt.Sprintf("%d", dimension.Confirmed))
		referenceBudgetWriteHashString(h, fmt.Sprintf("%d", dimension.Reserved))
		referenceBudgetWriteHashString(h, fmt.Sprintf("%d", dimension.Unknown))
		referenceBudgetWriteHashString(h, fmt.Sprintf("%d", dimension.Remaining))
		referenceBudgetWriteHashString(h, fmt.Sprintf("%t", dimension.Exhausted))
	}
	referenceBudgetWriteHashString(h, strings.TrimSpace(view.ActiveWaitID))
	referenceBudgetWriteHashString(h, strings.TrimSpace(view.TerminalReasonCode))
	return "runtime-budget-view-" + hex.EncodeToString(h.Sum(nil))[:20]
}

type budgetBufferedHashInputs struct {
	Allocation     RuntimeBudgetAllocation
	Consumption    RuntimeBudgetConsumptionFact
	Reconciliation RuntimeBudgetReconciliationRecord
	View           RuntimeExecutionBudgetView
}

func budgetBufferedHashFixture(text string, amount int64) budgetBufferedHashInputs {
	return budgetBufferedHashInputs{
		Allocation:     RuntimeBudgetAllocation{AllocationID: text, Dimension: RuntimeBudgetDimensionProviderCall, Unit: " call ", LogicalConsumptionID: "logical", ScopeRef: text, PolicyRevision: " revision ", LimitAtAllocation: amount, ReservedAmount: amount, MaxIncrement: amount, State: RuntimeBudgetAllocationUnknown, OwnerFactRef: text, AllocationHash: "unchanged"},
		Consumption:    RuntimeBudgetConsumptionFact{ConsumptionID: text, AllocationID: text, Owner: " owner ", LogicalConsumptionID: "logical", PhysicalConsumptionID: text, Unit: "call", ConfirmedAmount: amount, UnknownAmount: amount, Currency: " cny ", MonetaryAmount: amount, OwnerResultRef: text, OwnerResultHash: "result", ConsumptionHash: "unchanged"},
		Reconciliation: RuntimeBudgetReconciliationRecord{ReconciliationID: text, AllocationID: text, ProbeKind: " query ", ProbeOwner: " owner ", ProbeRequestHash: text, ExpectedOwnerFence: text, Status: RuntimeBudgetReconciliationStillUnknown, OwnerResultHash: text, ReconciliationHash: "unchanged"},
		View: RuntimeExecutionBudgetView{WorkspaceID: common.WorkspaceID(text), RunID: " run ", PolicyRevision: text, StateRevision: uint64(amount), ActiveWaitID: text, TerminalReasonCode: text, ViewHash: "unchanged", Dimensions: []RuntimeExecutionBudgetDimensionView{
			{Dimension: RuntimeBudgetDimensionProviderCall, Unit: text, Limit: amount, Confirmed: amount, Reserved: amount, Unknown: amount, Remaining: amount, Exhausted: true},
			{Dimension: RuntimeBudgetDimensionProviderInputToken, Unit: "token", Limit: 100, Confirmed: 10, Reserved: 20, Unknown: 30, Remaining: 40},
		}},
	}
}

func assertBudgetBufferedHashReference(t testing.TB, value budgetBufferedHashInputs) {
	t.Helper()
	before := value
	if value.View.Dimensions != nil {
		before.View.Dimensions = append(make([]RuntimeExecutionBudgetDimensionView, 0, len(value.View.Dimensions)), value.View.Dimensions...)
	}
	for range 2 {
		for _, pair := range []struct{ name, got, want string }{
			{"allocation ID", StableRuntimeBudgetAllocationID(value.Allocation), referenceBudgetAllocationID(value.Allocation)},
			{"allocation hash", StableRuntimeBudgetAllocationHash(value.Allocation), referenceBudgetAllocationHash(value.Allocation)},
			{"consumption ID", StableRuntimeBudgetConsumptionID(value.Consumption), referenceBudgetConsumptionID(value.Consumption)},
			{"consumption hash", StableRuntimeBudgetConsumptionHash(value.Consumption), referenceBudgetConsumptionHash(value.Consumption)},
			{"reconciliation ID", StableRuntimeBudgetReconciliationID(value.Reconciliation), referenceBudgetReconciliationID(value.Reconciliation)},
			{"reconciliation hash", StableRuntimeBudgetReconciliationHash(value.Reconciliation), referenceBudgetReconciliationHash(value.Reconciliation)},
			{"view hash", StableRuntimeExecutionBudgetViewHash(value.View), referenceBudgetViewHash(value.View)},
		} {
			if pair.got != pair.want {
				t.Fatalf("预算 %s 原编码漂移：got=%s want=%s", pair.name, pair.got, pair.want)
			}
		}
	}
	if !reflect.DeepEqual(value, before) {
		t.Fatal("预算 hash 编码改写原输入")
	}
}

func TestBudgetBufferedHashKeepsOriginalEncoding(t *testing.T) {
	for _, text := range []string{"", " 原事实\r\n\x00 ", string([]byte{0xff, 0x80}), strings.Repeat("x", 120), strings.Repeat("x", 121), strings.Repeat("原", 32768)} {
		for _, amount := range []int64{0, 1, -1, RuntimeBudgetAmountLimit, -1 << 63, 1<<63 - 1} {
			value := budgetBufferedHashFixture(text, amount)
			assertBudgetBufferedHashReference(t, value)
			body, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			var loaded budgetBufferedHashInputs
			if err := json.Unmarshal(body, &loaded); err != nil {
				t.Fatal(err)
			}
			assertBudgetBufferedHashReference(t, loaded)
		}
	}
	for _, dimensions := range [][]RuntimeExecutionBudgetDimensionView{nil, {}, {{Dimension: " b "}, {Dimension: "a"}, {Dimension: "a"}}} {
		value := budgetBufferedHashFixture("fact", 1)
		value.View.Dimensions = dimensions
		assertBudgetBufferedHashReference(t, value)
	}
}

func TestBudgetBufferedHashKeepsEveryFieldAndOrderedView(t *testing.T) {
	value := budgetBufferedHashFixture("fact", 1)
	// 反射仅遍历测试值的标量字段，不进入生产合同或跨 owner 传输。
	for _, owner := range []string{"Allocation", "Consumption", "Reconciliation", "View"} {
		original := reflect.ValueOf(value).FieldByName(owner)
		for index := 0; index < original.NumField(); index++ {
			if original.Field(index).Kind() == reflect.Slice {
				continue
			}
			t.Run(owner+"/"+original.Type().Field(index).Name, func(t *testing.T) {
				candidate := value
				field := reflect.ValueOf(&candidate).Elem().FieldByName(owner).Field(index)
				switch field.Kind() {
				case reflect.String:
					field.SetString(field.String() + "different")
				case reflect.Int64:
					field.SetInt(field.Int() + 1)
				case reflect.Uint64:
					field.SetUint(field.Uint() + 1)
				default:
					t.Fatalf("未覆盖标量：%s", field.Kind())
				}
				assertBudgetBufferedHashReference(t, candidate)
				fieldName := original.Type().Field(index).Name
				switch owner {
				case "Allocation":
					if fieldName != "AllocationHash" && StableRuntimeBudgetAllocationHash(candidate.Allocation) == StableRuntimeBudgetAllocationHash(value.Allocation) {
						t.Fatal("allocation 字段未进入原 hash")
					}
				case "Consumption":
					if fieldName != "ConsumptionHash" && StableRuntimeBudgetConsumptionHash(candidate.Consumption) == StableRuntimeBudgetConsumptionHash(value.Consumption) {
						t.Fatal("consumption 字段未进入原 hash")
					}
				case "Reconciliation":
					if fieldName != "ReconciliationHash" && StableRuntimeBudgetReconciliationHash(candidate.Reconciliation) == StableRuntimeBudgetReconciliationHash(value.Reconciliation) {
						t.Fatal("reconciliation 字段未进入原 hash")
					}
				case "View":
					if fieldName != "ViewHash" && StableRuntimeExecutionBudgetViewHash(candidate.View) == StableRuntimeExecutionBudgetViewHash(value.View) {
						t.Fatal("view 字段未进入原 hash")
					}
				}
			})
		}
	}
	for index := 0; index < reflect.TypeFor[RuntimeExecutionBudgetDimensionView]().NumField(); index++ {
		candidate := value
		candidate.View.Dimensions = append([]RuntimeExecutionBudgetDimensionView(nil), value.View.Dimensions...)
		field := reflect.ValueOf(&candidate.View.Dimensions[0]).Elem().Field(index)
		switch field.Kind() {
		case reflect.String:
			field.SetString(field.String() + "different")
		case reflect.Int64:
			field.SetInt(field.Int() + 1)
		case reflect.Bool:
			field.SetBool(!field.Bool())
		default:
			t.Fatalf("未覆盖 dimension 标量：%s", field.Kind())
		}
		assertBudgetBufferedHashReference(t, candidate)
		if StableRuntimeExecutionBudgetViewHash(candidate.View) == StableRuntimeExecutionBudgetViewHash(value.View) {
			t.Fatal("dimension 字段未进入原 hash")
		}
	}
	candidate := value
	candidate.View.Dimensions = append([]RuntimeExecutionBudgetDimensionView(nil), value.View.Dimensions...)
	candidate.View.Dimensions[0], candidate.View.Dimensions[1] = candidate.View.Dimensions[1], candidate.View.Dimensions[0]
	assertBudgetBufferedHashReference(t, candidate)
	if StableRuntimeExecutionBudgetViewHash(candidate.View) == StableRuntimeExecutionBudgetViewHash(value.View) {
		t.Fatal("view 编码排序或丢弃原 dimension 顺序")
	}
}

func TestBudgetBufferedHashKeepsCompleteLedgerAndReload(t *testing.T) {
	ledger, err := NewRuntimeExecutionBudgetLedger(DefaultRuntimeExecutionBudgetPolicy())
	if err != nil {
		t.Fatal(err)
	}
	specs, err := RuntimeProviderBudgetAllocationSpecs(ledger, RuntimeProviderDocumentOperations, 0, 1, "logical", "physical")
	if err != nil {
		t.Fatal(err)
	}
	ledger, allocations, duplicate, err := ReserveRuntimeBudgetAllocations(ledger, specs)
	if err != nil || duplicate {
		t.Fatalf("reserve: %v/%v", duplicate, err)
	}
	for _, allocation := range allocations {
		ledger, _, _, err = JoinRuntimeBudgetConsumption(ledger, RuntimeBudgetConsumptionJoin{AllocationID: allocation.AllocationID, Owner: RuntimeModuleLLM, PhysicalConsumptionID: "physical", ConfirmedAmount: 1, OwnerResultRef: "original-result", OwnerResultHash: "original-hash"})
		if err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if err := ledger.Validate(); err != nil {
			t.Fatal(err)
		}
		for _, value := range ledger.Allocations {
			if value.AllocationID != referenceBudgetAllocationID(value) || value.AllocationHash != referenceBudgetAllocationHash(value) {
				t.Fatal("原 allocation identity 漂移")
			}
		}
		for _, value := range ledger.Consumptions {
			if value.ConsumptionID != referenceBudgetConsumptionID(value) || value.ConsumptionHash != referenceBudgetConsumptionHash(value) {
				t.Fatal("原 consumption identity 漂移")
			}
		}
		view, err := BuildRuntimeExecutionBudgetView("workspace", "run", 7, ledger, "", "")
		if err != nil {
			t.Fatal(err)
		}
		if view.ViewHash != referenceBudgetViewHash(view) {
			t.Fatal("重载投影 hash 漂移")
		}
		body, err := json.Marshal(ledger)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(body, &ledger); err != nil {
			t.Fatal(err)
		}
	}
	ledger.Allocations[0].ReservedAmount++
	if ledger.Validate() == nil {
		t.Fatal("后续事实变化借用先前 hash 或成功")
	}
}

func TestBudgetBufferedHashReducesOnlyEncodingAllocations(t *testing.T) {
	value := budgetBufferedHashFixture("fact", 1)
	for _, pair := range []struct {
		name              string
		original, current func() string
	}{
		{"allocation", func() string { return referenceBudgetAllocationHash(value.Allocation) }, func() string { return StableRuntimeBudgetAllocationHash(value.Allocation) }},
		{"consumption", func() string { return referenceBudgetConsumptionHash(value.Consumption) }, func() string { return StableRuntimeBudgetConsumptionHash(value.Consumption) }},
		{"reconciliation", func() string { return referenceBudgetReconciliationHash(value.Reconciliation) }, func() string { return StableRuntimeBudgetReconciliationHash(value.Reconciliation) }},
		{"view", func() string { return referenceBudgetViewHash(value.View) }, func() string { return StableRuntimeExecutionBudgetViewHash(value.View) }},
	} {
		original := testing.AllocsPerRun(20, func() { _ = pair.original() })
		current := testing.AllocsPerRun(20, func() { _ = pair.current() })
		if current >= original {
			t.Fatalf("%s 编码分配未减少：current=%v original=%v", pair.name, current, original)
		}
	}
}

func FuzzBudgetBufferedHashReference(f *testing.F) {
	f.Add(" 原事实\r\n\x00 ", int64(1), uint8(0))
	f.Add(string([]byte{0xff, 0x80}), int64(-1), uint8(1))
	f.Fuzz(func(t *testing.T, text string, amount int64, mode uint8) {
		value := budgetBufferedHashFixture(text, amount)
		if mode%3 == 1 {
			value.View.Dimensions = nil
		}
		if mode%3 == 2 {
			value.View.Dimensions = append(value.View.Dimensions, value.View.Dimensions[0])
		}
		assertBudgetBufferedHashReference(t, value)
	})
}

func BenchmarkBudgetBufferedHash(b *testing.B) {
	for _, size := range []int{16, 4096} {
		value := budgetBufferedHashFixture(strings.Repeat("x", size), 1)
		for _, mode := range []string{"reference", "current"} {
			b.Run(fmt.Sprintf("%d/%s", size, mode), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if mode == "reference" {
						_ = referenceBudgetAllocationHash(value.Allocation)
						_ = referenceBudgetConsumptionHash(value.Consumption)
						_ = referenceBudgetReconciliationHash(value.Reconciliation)
						_ = referenceBudgetViewHash(value.View)
					} else {
						_ = StableRuntimeBudgetAllocationHash(value.Allocation)
						_ = StableRuntimeBudgetConsumptionHash(value.Consumption)
						_ = StableRuntimeBudgetReconciliationHash(value.Reconciliation)
						_ = StableRuntimeExecutionBudgetViewHash(value.View)
					}
				}
			})
		}
	}
}
