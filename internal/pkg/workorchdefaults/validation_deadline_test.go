package workorchdefaults

import "testing"

func TestDefaultProjectValidationHasBoundedConjunctiveDeadline(t *testing.T) {
	templates, err := loadEmbeddedTemplates()
	if err != nil {
		t.Fatal(err)
	}
	declarations := 0
	for _, template := range templates {
		for _, policy := range template.EvidencePolicies {
			if policy["recipeAdapterRef"] != "shell/project-multi-failure-validation" {
				continue
			}
			declarations++
			if template.ID != "software-dev" || policy["recipeTimeoutMillis"] != float64(300000) {
				t.Fatal("conjunctive proof method has an insufficient or unscoped default deadline")
			}
		}
	}
	if declarations != 1 {
		t.Fatal("expected exactly one software proof method")
	}
}
