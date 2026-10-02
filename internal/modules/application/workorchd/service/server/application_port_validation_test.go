package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
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

func TestApplicationPortsExcludeRetiredDefinitionWrites(t *testing.T) {
	for _, group := range []reflect.Type{reflect.TypeOf(WorkspacePorts{}), reflect.TypeOf(DefinitionPorts{})} {
		for _, name := range []string{"CreateWorkspace", "DefineGoal", "UpdateGoal", "DefineDoD", "UpdateDoD", "CheckGoalDoDQuality", "GenerateWorkspaceSetupDraft"} {
			if _, exists := group.FieldByName(name); exists {
				t.Fatalf("退休端口仍公开: %s.%s", group.Name(), name)
			}
		}
	}
	if err := completeApplicationPorts(t).Validate(); err != nil {
		t.Fatalf("现行端口必须足以完成装配: %v", err)
	}
}

func TestRunApplicationPortsRemainReadOnly(t *testing.T) {
	typeOfRunPorts := reflect.TypeOf(RunPorts{})
	for index := 0; index < typeOfRunPorts.NumField(); index++ {
		field := typeOfRunPorts.Field(index)
		if !strings.HasPrefix(field.Name, "Read") && !strings.HasPrefix(field.Name, "List") {
			t.Errorf("ApplicationPorts.Run.%s is not a read-only operation", field.Name)
		}
	}
}

func TestApplicationPortGroupsMapUnknownErrorsWithoutTransportLeak(t *testing.T) {
	tests := []struct {
		group string
		port  string
		path  string
	}{
		{group: "Workspace", port: "ReadWorkspace", path: "/api/v1/workorch/workspaces/ws-port-error"},
		{group: "Definition", port: "ListGoals", path: "/api/v1/workorch/workspaces/ws-port-error/goals"},
		{group: "Run", port: "ListRuns", path: "/api/v1/workorch/runs?workspaceId=ws-port-error"},
		{group: "Governance", port: "ReadDecision", path: "/api/v1/workorch/decisions/ticket-port-error"},
		{group: "Registry", port: "ListSouls", path: "/api/v1/workorch/souls"},
		{group: "Memory", port: "SearchMemory", path: "/api/v1/workorch/memory?workspaceId=ws-port-error"},
		{group: "Configuration", port: "ReadConfigurationWithCatalog", path: "/api/v1/workorch/configuration"},
	}
	secret := errors.New("owner transport failed: credential=must-not-leak")
	for _, test := range tests {
		t.Run(test.group, func(t *testing.T) {
			ports := completeApplicationPorts(t)
			applicationPortValue := reflect.ValueOf(&ports).Elem().FieldByName(test.group).FieldByName(test.port)
			if !applicationPortValue.IsValid() || applicationPortValue.Kind() != reflect.Func {
				t.Fatalf("ApplicationPorts.%s.%s is unavailable", test.group, test.port)
			}
			applicationPortValue.Set(reflect.MakeFunc(applicationPortValue.Type(), func([]reflect.Value) []reflect.Value {
				results := make([]reflect.Value, applicationPortValue.Type().NumOut())
				for index := range results {
					results[index] = reflect.Zero(applicationPortValue.Type().Out(index))
				}
				results[len(results)-1] = reflect.ValueOf(secret)
				return results
			}))
			state := newServerStateWithApplicationPorts(normalizeConfig(Config{Backend: "memory"}), ports, nil, nil)
			recorder := httptest.NewRecorder()
			newMuxWithStateNoStart(state).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, test.path, nil))
			if recorder.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500; body=%s", recorder.Code, recorder.Body.String())
			}
			var response errorResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if response.Error != "internal server error" || strings.Contains(recorder.Body.String(), "credential") {
				t.Fatalf("unsafe transport error = %+v body=%s", response, recorder.Body.String())
			}
		})
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
