package workorchruntimecontract

// RuntimeReceiptDeliveryError describes transport only. It never authorizes an
// operation, changes a receipt, or bypasses the consumer's original admission.
type RuntimeReceiptDeliveryError struct {
	Retryable    bool
	Cause        error
	ExecutionKey string
	Fence        uint64
	Diagnostic   *RuntimeOriginalDiagnostic
}

func (e *RuntimeReceiptDeliveryError) Error() string {
	if e.Diagnostic != nil {
		return e.Diagnostic.OriginalMessage
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return "owner result delivery failed"
}
func (e *RuntimeReceiptDeliveryError) Unwrap() error { return e.Cause }
