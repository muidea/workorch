package orchestrator

import (
	"fmt"
	"reflect"
)

func populateConversionRawFixture(value reflect.Value, active map[reflect.Type]int, sequence *int) {
	if !value.IsValid() || !value.CanSet() {
		return
	}
	typeOfValue := value.Type()
	switch value.Kind() {
	case reflect.Pointer:
		if active[typeOfValue] != 0 {
			return
		}
		active[typeOfValue]++
		defer func() { active[typeOfValue]-- }()
		value.Set(reflect.New(typeOfValue.Elem()))
		populateConversionRawFixture(value.Elem(), active, sequence)
	case reflect.Struct:
		if active[typeOfValue] != 0 {
			return
		}
		active[typeOfValue]++
		defer func() { active[typeOfValue]-- }()
		for idx := 0; idx < value.NumField(); idx++ {
			populateConversionRawFixture(value.Field(idx), active, sequence)
		}
	case reflect.Slice:
		if active[typeOfValue] != 0 {
			return
		}
		active[typeOfValue]++
		defer func() { active[typeOfValue]-- }()
		length := 1
		if conversionRawScalarKind(typeOfValue.Elem().Kind()) {
			length = 3
		}
		value.Set(reflect.MakeSlice(typeOfValue, length, length))
		for idx := 0; idx < length; idx++ {
			populateConversionRawFixture(value.Index(idx), active, sequence)
		}
	case reflect.Map:
		value.Set(reflect.MakeMapWithSize(typeOfValue, 1))
		key := reflect.New(typeOfValue.Key()).Elem()
		item := reflect.New(typeOfValue.Elem()).Elem()
		populateConversionRawFixture(key, active, sequence)
		populateConversionRawFixture(item, active, sequence)
		value.SetMapIndex(key, item)
	case reflect.Array:
		for idx := 0; idx < value.Len(); idx++ {
			populateConversionRawFixture(value.Index(idx), active, sequence)
		}
	case reflect.String:
		*sequence++
		value.SetString(fmt.Sprintf(" raw-%04d  ", *sequence))
	case reflect.Bool:
		value.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		*sequence++
		value.SetInt(int64(*sequence + 1))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		*sequence++
		value.SetUint(uint64(*sequence + 1))
	case reflect.Float32, reflect.Float64:
		*sequence++
		value.SetFloat(float64(*sequence) + 0.25)
	}
}

func conversionRawScalarKind(kind reflect.Kind) bool {
	switch kind {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}
