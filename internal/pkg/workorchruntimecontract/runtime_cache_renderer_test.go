package workorchruntimecontract

import "testing"

func TestCurrentCacheEncodingIsSealedAndClosed(t *testing.T) {
	for _, kind := range []RuntimeProviderDocumentKind{RuntimeProviderDocumentOperations, RuntimeProviderDocumentSemanticPlan} {
		c := RuntimeExpectedDocumentContract{SchemaVersion: RuntimeExpectedDocumentSchemaCurrent, DocumentKind: kind, ManifestHash: "manifest", LogicalRequestID: "logical", StablePrefixHash: "prefix", MaxOutputTokens: 100, MaxRecoveryAttempts: 1}
		if kind == RuntimeProviderDocumentOperations {
			c.WireContractHash = "wire"
			c.OperationsContextEncoding = RuntimeOperationsContextEncoding
		} else {
			c.PlanStep = RuntimeSemanticPlanStepCoverageOrder
			c.DefinitionSeedHash = "seed"
			c.SemanticContextEncoding = RuntimeSemanticContextEncoding
		}
		c.Hash = StableRuntimeExpectedDocumentHash(c)
		if err := c.Validate(); err != nil {
			t.Fatal(err)
		}
		for _, invalid := range []string{"", "unknown", "semantic-render/work-content/v2", "operations-render/compact-contract/v2"} {
			b := c
			if kind == RuntimeProviderDocumentOperations {
				b.OperationsContextEncoding = invalid
			} else {
				b.SemanticContextEncoding = invalid
			}
			b.Hash = StableRuntimeExpectedDocumentHash(b)
			if b.Validate() == nil {
				t.Fatal("obsolete or unknown encoding accepted", invalid)
			}
		}
	}
}
