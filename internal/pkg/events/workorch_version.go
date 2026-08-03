package events

// WorkorchEventVersion is the wire version recorded by audit/eventstream
// projections. Topic ownership and message support remain in each owner's
// pkg/events contract; this neutral package intentionally keeps no topic
// registry.
const WorkorchEventVersion = "workorch.event.v1"
