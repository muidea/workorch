package llm

import (
	"strings"
	"testing"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
)

func TestBuildProviderFacingPromptKeepsC3OutOfStablePrefix(t *testing.T) {
	request := llmmodels.ProviderRequest{PromptContext: &llmmodels.PromptContext{Sections: []llmmodels.PromptSection{
		{ID: "C0.contract", Role: "system", Cacheability: "stable", Content: "contract", DisplayOrder: 0, Protected: true},
		{ID: "C1.definition", Role: "system", Cacheability: "stable", Content: "definition", DisplayOrder: 1, Protected: true},
		{ID: "C2.capabilities", Role: "system", Cacheability: "stable", Content: "capabilities", DisplayOrder: 2, Protected: true},
		{ID: "C3.authority", Role: "user", Cacheability: "volatile", Content: "continuation-signature", DisplayOrder: 3, Protected: true},
	}}}

	prompt := buildProviderFacingPrompt(request)
	if strings.Contains(prompt.StablePrefix, "continuation-signature") {
		t.Fatalf("stable prefix contains C3 continuation: %s", prompt.StablePrefix)
	}
	if len(prompt.DynamicMessages) != 1 || !strings.Contains(prompt.DynamicMessages[0].Content, "continuation-signature") {
		t.Fatalf("dynamic messages = %+v, want C3 authority", prompt.DynamicMessages)
	}
	if got := strings.Join(prompt.StablePrefixSectionIDs, ","); got != "C0.contract,C1.definition,C2.capabilities" {
		t.Fatalf("stable prefix sections = %q", got)
	}
}
