package orchestrator

import (
	"reflect"
	"testing"

	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	gitevents "github.com/muidea/workorch/internal/modules/blocks/git/pkg/events"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/events"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
)

func TestRuntimeCWIReconciliationOwnerResultsExposeOnlyCanonicalObservation(t *testing.T) {
	for name, result := range map[string]any{
		"file":  fileevents.QueryRuntimeCWIReconciliationResult{},
		"git":   gitevents.QueryRuntimeCWIReconciliationResult{},
		"mcp":   mcpevents.QueryRuntimeCWIReconciliationResult{},
		"shell": shellevents.QueryRuntimeCWIReconciliationResult{},
	} {
		t.Run(name, func(t *testing.T) {
			typeOf := reflect.TypeOf(result)
			if typeOf.NumField() != 1 || typeOf.Field(0).Name != "Observation" {
				t.Fatalf("%s reconciliation Result fields = %v, want only Observation", name, typeOf)
			}
		})
	}
}
