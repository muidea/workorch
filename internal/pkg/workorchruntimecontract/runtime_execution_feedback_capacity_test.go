package workorchruntimecontract

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestExecutionFeedbackCapacityMeasuresWholeEnvelopeWithoutChangingIt(t *testing.T) {
	f := []RuntimeSemanticExecutionFeedback{{ReceiptRef: "x"}}
	bytes, _ := json.Marshal(f)
	remaining := RuntimeSemanticExecutionFeedbackMaxBytes - len(bytes)
	f[0].ReceiptRef += strings.Repeat("x", remaining)
	if err := ValidateRuntimeExecutionFeedbackCapacity(f); err != nil {
		t.Fatal("exact capacity boundary rejected", err)
	}
	f[0].ReceiptRef += "x"
	old := f[0].ReceiptRef
	err := ValidateRuntimeExecutionFeedbackCapacity(f)
	var capacity *RuntimeExecutionFeedbackCapacityError
	if !errors.As(err, &capacity) || capacity.ActualBytes != RuntimeSemanticExecutionFeedbackMaxBytes+1 || capacity.LimitBytes != RuntimeSemanticExecutionFeedbackMaxBytes || f[0].ReceiptRef != old {
		t.Fatal("overflow was clipped or lost its exact dimension", err)
	}
}
