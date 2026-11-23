package workorchdefaults

import "testing"

func TestDefaultProjectValidationHasBoundedConjunctiveDeadline(t *testing.T) {
	templates, err := loadEmbeddedTemplates()
	if err != nil {
		t.Fatal(err)
	}
	behaviorDeclarations := 0
	artifactDeclarations := 0
	for _, template := range templates {
		for _, policy := range template.EvidencePolicies {
			adapter, _ := policy["recipeAdapterRef"].(string)
			switch adapter {
			case "shell/project-multi-failure-validation":
				behaviorDeclarations++
				if template.ID != "software-dev" || policy["recipeTimeoutMillis"] != float64(300000) {
					t.Fatal("conjunctive proof method has an insufficient or unscoped default deadline")
				}
			case "shell/project-multi-failure-artifact-validation":
				artifactDeclarations++
				if template.ID != "software-dev" || policy["recipeTimeoutMillis"] != float64(300000) {
					t.Fatal("artifact conjunctive proof method has an insufficient or unscoped default deadline")
				}
				if _, hasCoverageReqs := policy["coverageRequirements"]; hasCoverageReqs {
					t.Fatal("artifact validation policy must not declare wildcard coverageRequirements")
				}
			}
		}
	}
	if behaviorDeclarations != 1 || artifactDeclarations != 1 {
		t.Fatalf("expected 1 behavior and 1 artifact software proof method, got %d / %d", behaviorDeclarations, artifactDeclarations)
	}
}
