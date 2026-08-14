package recordpayload

// WorkorchEventVersion is the Eventstream-owned audit wire version consumed by
// projections. Source notification topics remain in each publishing owner's
// pkg/events contract; this package does not register or forward those topics.
const WorkorchEventVersion = "workorch.event"
