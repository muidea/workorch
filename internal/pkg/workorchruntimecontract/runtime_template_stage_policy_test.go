package workorchruntimecontract

import "testing"

func TestRuntimeTemplateStagePolicyNormalizesAndValidates(t *testing.T) {
	policy := RuntimeTemplateStagePolicy{SchemaVersion: " workorch.template_stage_policy.v1 ", Stages: []RuntimeTemplateStage{
		{ID: " validation ", RequiredOutputs: []RuntimeTemplateStageRequiredOutput{{Kind: " validation ", EvidenceKinds: []string{"validation", " validation "}}}},
		{ID: " execution ", PolicyRefs: RuntimeTemplateStagePolicyRefs{EvidencePolicies: []string{"policy-b", "policy-a"}}},
	}}.Normalized()
	if err := policy.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(policy.Stages) != 2 || policy.Stages[0].ID != "execution" || policy.Stages[1].ID != "validation" ||
		len(policy.Stages[1].RequiredOutputs[0].EvidenceKinds) != 1 {
		t.Fatalf("normalized policy = %+v", policy)
	}
}
