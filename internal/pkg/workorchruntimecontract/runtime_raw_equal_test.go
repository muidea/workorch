package workorchruntimecontract

import (
	"math"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestRuntimeRawEqualStandardOracle(t *testing.T) {
	type cycle struct {
		Text string
		Next *cycle
	}
	a, b := &cycle{Text: "cycle"}, &cycle{Text: "cycle"}
	a.Next, b.Next = a, b
	unequal := &cycle{Text: "different"}
	unequal.Next = unequal
	nan := math.NaN()
	nans := []float64{nan}
	values := []any{nil, true, false, "", "\x00原文 \n", 1, int64(1), uint64(1),
		float32(1), float64(1), nan, complex(nan, 1), []string(nil), []string{},
		[]string{"a", "b"}, [2]string{"a", "b"}, (*cycle)(nil), a, b, unequal,
		nans, []float64{nan}, map[string]any(nil), map[string]any{},
		map[string]any{"nested": nans}, time.Now(), time.Time{},
		(func())(nil), func() {}, make(chan int)}
	// Same-pointer NaNs, cycles, nil/empty, dynamic/unexported fields and
	// differing types must agree with the standard comparator in both orders.
	for _, left := range values {
		for _, right := range values {
			if got, want := runtimeRawEqual(left, right), reflect.DeepEqual(left, right); got != want {
				t.Fatalf("raw comparison differs for %T / %T: %v != %v", left, right, got, want)
			}
		}
	}
}

func TestRuntimeRawEqualCompleteDTOMutations(t *testing.T) {
	base := terminalHistoricalAcceptanceFixture(t)
	compare := func(changed RuntimeStateRecord) {
		t.Helper()
		if got, want := runtimeRawEqual(base, changed), reflect.DeepEqual(base, changed); got != want {
			t.Fatal("complete DTO raw equality differs from standard comparison")
		}
	}
	compare(base)
	copy := base.Clone()
	compare(copy)
	ownerLineageFieldMutations(reflect.ValueOf(&copy).Elem(), func() { compare(copy) })
	if !runtimeRawEqual(base, base.Clone()) {
		t.Fatal("later comparison retained a previous business value")
	}
}

func TestRuntimeRawEqualParallelTypePlans(t *testing.T) {
	var wg sync.WaitGroup
	for index := 0; index < 16; index++ {
		wg.Go(func() {
			for i := 0; i < 20; i++ {
				left := struct {
					Workspace string
					Values    []int
				}{"scope", []int{i, i + 1}}
				right := struct {
					Workspace string
					Values    []int
				}{"other", []int{i, i + 1}}
				if !runtimeRawEqual(left, left) || runtimeRawEqual(left, right) {
					t.Error("type plan mixed independent values")
				}
			}
		})
	}
	wg.Wait()
}

func FuzzRuntimeRawEqualStandardOracle(f *testing.F) {
	f.Add([]byte("original\x00 bytes"), []byte("different"), true)
	f.Fuzz(func(t *testing.T, left, right []byte, present bool) {
		type value struct {
			Data   []byte
			Nested *struct{ Text string }
		}
		a, b := value{Data: left}, value{Data: right}
		if present {
			a.Nested, b.Nested = &struct{ Text string }{string(left)}, &struct{ Text string }{string(right)}
		}
		if runtimeRawEqual(a, b) != reflect.DeepEqual(a, b) {
			t.Fatal("raw comparator differs from standard oracle")
		}
	})
}
