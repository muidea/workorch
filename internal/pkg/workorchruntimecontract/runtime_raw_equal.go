package workorchruntimecontract

import (
	"reflect"
	"sync"
)

// Compare complete raw values with DeepEqual semantics. The plan contains type
// instructions only: no business values, hashes, validity or owner scope survive
// a comparison. Unlike DeepEqual's general graph walk, ordinary finite DTO trees
// need no per-node visited map. Recursive graphs and opaque values still use the
// standard comparator. This changes comparison cost, never a validation gate.
type runtimeRawEqualPlan struct {
	equal func(reflect.Value, reflect.Value, int) bool
}

var runtimeRawEqualPlans sync.Map // reflect.Type -> *runtimeRawEqualPlan

func runtimeRawEqual(left, right any) bool {
	a, b := reflect.ValueOf(left), reflect.ValueOf(right)
	if !a.IsValid() || !b.IsValid() {
		return a.IsValid() == b.IsValid()
	}
	if a.Type() != b.Type() {
		return false
	}
	t := a.Type()
	value, ok := runtimeRawEqualPlans.Load(t)
	if !ok {
		// Build privately, including recursive edges, before publication. Losing
		// a concurrent construction only discards identical type instructions.
		plan := buildRuntimeRawEqualPlan(t, map[reflect.Type]*runtimeRawEqualPlan{})
		value, _ = runtimeRawEqualPlans.LoadOrStore(t, plan)
	}
	return value.(*runtimeRawEqualPlan).equal(a, b, 0)
}

func buildRuntimeRawEqualPlan(t reflect.Type, plans map[reflect.Type]*runtimeRawEqualPlan) *runtimeRawEqualPlan {
	if plan := plans[t]; plan != nil {
		return plan
	}
	p := &runtimeRawEqualPlan{}
	plans[t] = p
	standard := func(a, b reflect.Value, _ int) bool { return reflect.DeepEqual(a.Interface(), b.Interface()) }
	switch t.Kind() {
	case reflect.Bool:
		p.equal = func(a, b reflect.Value, _ int) bool { return a.Bool() == b.Bool() }
	case reflect.String:
		p.equal = func(a, b reflect.Value, _ int) bool { return a.String() == b.String() }
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		p.equal = func(a, b reflect.Value, _ int) bool { return a.Int() == b.Int() }
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		p.equal = func(a, b reflect.Value, _ int) bool { return a.Uint() == b.Uint() }
	case reflect.Float32, reflect.Float64:
		p.equal = func(a, b reflect.Value, _ int) bool { return a.Float() == b.Float() }
	case reflect.Complex64, reflect.Complex128:
		p.equal = func(a, b reflect.Value, _ int) bool { return a.Complex() == b.Complex() }
	case reflect.Struct:
		fields := make([]*runtimeRawEqualPlan, t.NumField())
		for i := range fields {
			if t.Field(i).PkgPath != "" {
				p.equal = standard // preserve opaque/unexported-field semantics
				return p
			}
			fields[i] = buildRuntimeRawEqualPlan(t.Field(i).Type, plans)
		}
		p.equal = func(a, b reflect.Value, depth int) bool {
			if depth >= 64 {
				return standard(a, b, depth)
			}
			for i, field := range fields {
				if !field.equal(a.Field(i), b.Field(i), depth+1) {
					return false
				}
			}
			return true
		}
	case reflect.Pointer:
		element := buildRuntimeRawEqualPlan(t.Elem(), plans)
		p.equal = func(a, b reflect.Value, depth int) bool {
			if a.IsNil() || b.IsNil() || a.Pointer() == b.Pointer() {
				return a.IsNil() == b.IsNil() && a.Pointer() == b.Pointer()
			}
			if depth >= 64 {
				return standard(a, b, depth)
			}
			return element.equal(a.Elem(), b.Elem(), depth+1)
		}
	case reflect.Slice, reflect.Array:
		element := buildRuntimeRawEqualPlan(t.Elem(), plans)
		slice := t.Kind() == reflect.Slice
		p.equal = func(a, b reflect.Value, depth int) bool {
			if a.Len() != b.Len() || slice && a.IsNil() != b.IsNil() {
				return false
			}
			if slice && a.Pointer() == b.Pointer() {
				return true
			}
			if depth >= 64 {
				return standard(a, b, depth)
			}
			for i := 0; i < a.Len(); i++ {
				if !element.equal(a.Index(i), b.Index(i), depth+1) {
					return false
				}
			}
			return true
		}
	default:
		p.equal = standard // maps, dynamic interfaces, funcs and opaque handles
	}
	return p
}
