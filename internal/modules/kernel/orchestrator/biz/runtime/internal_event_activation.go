package runtime

// Activation facts create the one current-schema aggregate from the validated
// Run-owner bootstrap. They never choose a convergence action.
const RuntimeInternalEventActivationObserved RuntimeInternalEventKind = "activation_observed"
