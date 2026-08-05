package workorchruntimecontract

// RuntimeActivationDisposition is the owner-neutral immutable value carried
// when Run reports why persisted Runtime state cannot be resumed. It grants no
// lifecycle or activation authority to consumers.
type RuntimeActivationDisposition string

const RuntimeActivationDispositionFreshActivationRequired RuntimeActivationDisposition = "fresh_activation_required"
