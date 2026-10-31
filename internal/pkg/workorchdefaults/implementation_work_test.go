package workorchdefaults

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/runtimeprofiledecode"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestDefaultImplementationWorkIsProfileScopedAndDoesNotRequireArtificialRounds(t *testing.T) {
	templates, err := loadEmbeddedTemplates()
	if err != nil {
		t.Fatal(err)
	}
	declarations := 0
	for _, template := range templates {
		profiles := runtimeprofiledecode.Decode(template.ContextHints["profileContracts"])
		if err := runtimecontract.ValidateRuntimeProfileContracts(profiles); err != nil {
			t.Fatalf("%s: %v", template.ID, err)
		}
		for _, profile := range profiles {
			for _, requirement := range profile.EvidenceRequirements {
				if requirement.ImplementationWorkPolicy == nil {
					continue
				}
				declarations++
				if requirement.ImplementationWorkPolicy.ProgressProtocolRef != runtimecontract.RuntimeImplementationWorkProgressV1 {
					t.Fatal("default work must review complete original requirements")
				}
				if template.ID != "software-dev" || requirement.ID != "*" || requirement.Scope != "dodItem" {
					t.Fatal("software implementation authority leaked to another task or proof purpose")
				}
				if err := runtimecontract.ValidateRuntimeImplementationWorkAvailability([]runtimecontract.RuntimeProfileContract{profile}); err != nil {
					t.Fatal(err)
				}
				for _, rule := range requirement.LineageAssertions {
					if rule.Kind == runtimecontract.RuntimeLineageCompletedWorkRounds || rule.Kind == runtimecontract.RuntimeLineageRepairThenFreshValidation {
						t.Fatal("default forced artificial rounds or an intentional failure on unrelated projects")
					}
				}
			}
		}
	}
	if declarations != 1 {
		t.Fatalf("expected one bounded default work declaration, got %d", declarations)
	}
}
