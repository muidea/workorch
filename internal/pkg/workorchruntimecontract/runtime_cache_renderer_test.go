package workorchruntimecontract

import "testing"

func TestCacheRendererVersionIsSealedAndClosed(t *testing.T) {
	for _, kind := range []RuntimeProviderDocumentKind{RuntimeProviderDocumentOperations, RuntimeProviderDocumentSemanticPlan} {
		c := RuntimeExpectedDocumentContract{SchemaVersion: RuntimeExpectedDocumentSchemaCurrent, DocumentKind: kind, ManifestHash: "manifest", LogicalRequestID: "logical", StablePrefixHash: "prefix", MaxOutputTokens: 100, MaxRecoveryAttempts: 1}
		if kind == RuntimeProviderDocumentOperations {
			c.WireContractHash = "wire"
		} else {
			c.PlanStep = RuntimeSemanticPlanStepCoverageOrder
			c.DefinitionSeedHash = "seed"
		}
		versions := []string{"", RuntimeOperationsRenderContractV2, RuntimeOperationsRenderContractV3, RuntimeOperationsRenderContractV4}
		if kind == RuntimeProviderDocumentSemanticPlan {
			versions = []string{"", RuntimeSemanticRenderContractV2, RuntimeSemanticRenderContractV3, RuntimeSemanticRenderContractV4, RuntimeSemanticRenderContractV5}
		}
		seen := map[string]bool{}
		for _, version := range versions {
			if kind == RuntimeProviderDocumentOperations {
				c.OperationsRenderContractRef = version
			} else {
				c.SemanticRenderContractRef = version
			}
			c.Hash = StableRuntimeExpectedDocumentHash(c)
			if err := c.Validate(); err != nil {
				t.Fatal("supported renderer rejected", version, err)
			}
			if seen[c.Hash] {
				t.Fatal("renderer not bound by expected-document hash")
			}
			seen[c.Hash] = true
		}
		if kind == RuntimeProviderDocumentOperations {
			c.OperationsRenderContractRef = "unknown"
		} else {
			c.SemanticRenderContractRef = "unknown"
		}
		c.Hash = StableRuntimeExpectedDocumentHash(c)
		if c.Validate() == nil {
			t.Fatal("unknown renderer accepted")
		}
	}
}
