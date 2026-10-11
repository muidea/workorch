package events

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

// QueryRuntimeFileReadDependenciesCommand 只观察原 file/read 选择器，不授执行或 credit。
type QueryRuntimeFileReadDependenciesCommand struct {
	runtimecontract.RuntimeOperationExecutionContext
	RequestID             string
	IssuedAtStateRevision uint64
	SealedOperation       SealedRuntimeOperation
	SourceReceiptHash     string
	OriginalDependencies  runtimecontract.RuntimeOwnerRevisionSet
}

type RuntimeFileReadDependencyObservationStatus string

const (
	RuntimeFileReadDependencyObserved RuntimeFileReadDependencyObservationStatus = "observed"
	RuntimeFileReadDependencyMissing  RuntimeFileReadDependencyObservationStatus = "missing"
)

type QueryRuntimeFileReadDependenciesResult struct {
	RequestID             string
	IssuedAtStateRevision uint64
	SourceReceiptHash     string
	Status                RuntimeFileReadDependencyObservationStatus
	CurrentDependencies   *runtimecontract.RuntimeOwnerRevisionSet
}
