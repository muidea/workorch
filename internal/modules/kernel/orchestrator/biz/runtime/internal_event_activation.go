package runtime

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

// RuntimeInternalActivationPayload is the one-time Run-owner bootstrap fact.
// It uses the same private dispatcher as later mutations, so a Run
// notification cannot call initialization or a reducer directly.
type RuntimeInternalActivationPayload struct {
	ActivationFacts        *runtimecontract.RuntimeFacts
	ActivationDefinition   *runtimecontract.RuntimeDefinitionContext
	ActivationRequirements []runtimecontract.RuntimeRequirement
}

// Activation facts create the one current-schema aggregate from the validated
// Run-owner bootstrap. They never choose a convergence action.
const RuntimeInternalEventActivationObserved RuntimeInternalEventKind = "activation_observed"
