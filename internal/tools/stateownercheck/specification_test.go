package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSpecificationReviewObservationDoesNotAuthorizeSiblingState(t *testing.T) {
	for _, model := range []string{"GoalSpecificationCompletenessReview", "GoalSpecificationVerification", "DefinitionAuthoringVerificationMapping"} {
		for _, forbidden := range []bool{false, true} {
			t.Run(model+map[bool]string{false: "/observation", true: "/target"}[forbidden], func(t *testing.T) {
				root := newEmptyVerificationRepository(t)
				fields := "Status string"
				if forbidden {
					fields += "; TargetState string"
				}
				files := map[string]string{
					"internal/modules/blocks/workspace/pkg/models/specification.go": "package models\ntype " + model + " struct { " + fields + " }\n",
					"internal/modules/blocks/workspace/pkg/events/messages.go":      "package events\nimport models \"github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models\"\ntype SubmitCommand struct { Fact models." + model + " }\n",
				}
				for name, content := range files {
					path := filepath.Join(root, name)
					if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				err := verifyCrossOwnerCommandContracts(root)
				if forbidden {
					if err == nil || !strings.Contains(err.Error(), "TargetState") {
						t.Fatalf("target-state smuggling: %v", err)
					}
				} else if err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
