package server

import (
	"fmt"
	"reflect"
	"testing"
)

func TestApplicationPortsValidateCoversEveryTypedPort(t *testing.T) {
	complete := completeApplicationPorts(t)
	if err := complete.Validate(); err != nil {
		t.Fatalf("complete application ports rejected: %v", err)
	}

	typeOfPorts := reflect.TypeOf(complete)
	for groupIndex := 0; groupIndex < typeOfPorts.NumField(); groupIndex++ {
		groupField := typeOfPorts.Field(groupIndex)
		groupType := groupField.Type
		for portIndex := 0; portIndex < groupType.NumField(); portIndex++ {
			portField := groupType.Field(portIndex)
			t.Run(groupField.Name+"/"+portField.Name, func(t *testing.T) {
				candidate := complete
				candidateValue := reflect.ValueOf(&candidate).Elem()
				missing := candidateValue.Field(groupIndex).Field(portIndex)
				missing.Set(reflect.Zero(missing.Type()))

				want := fmt.Sprintf("missing %s.%s application port", groupField.Name, portField.Name)
				if err := candidate.Validate(); err == nil || err.Error() != want {
					t.Fatalf("Validate error = %v, want %q", err, want)
				}
			})
		}
	}
}

func completeApplicationPorts(t *testing.T) ApplicationPorts {
	t.Helper()
	ports := ApplicationPorts{}
	value := reflect.ValueOf(&ports).Elem()
	for groupIndex := 0; groupIndex < value.NumField(); groupIndex++ {
		group := value.Field(groupIndex)
		for portIndex := 0; portIndex < group.NumField(); portIndex++ {
			port := group.Field(portIndex)
			if port.Kind() != reflect.Func {
				t.Fatalf("ApplicationPorts.%s.%s kind = %s, want func", value.Type().Field(groupIndex).Name, group.Type().Field(portIndex).Name, port.Kind())
			}
			port.Set(reflect.MakeFunc(port.Type(), func([]reflect.Value) []reflect.Value {
				results := make([]reflect.Value, port.Type().NumOut())
				for index := range results {
					results[index] = reflect.Zero(port.Type().Out(index))
				}
				return results
			}))
		}
	}
	return ports
}
