package governancecore

import "github.com/muidea/workorch/internal/pkg/workorcheventpayload"

func RuntimeRepairAllowedRecoveryActions(input workorcheventpayload.Payload) []string {
	values, ok := input.StringSlice("allowedRecoveryActions")
	if !ok {
		return nil
	}
	ret := make([]string, 0, len(values))
	for _, item := range values {
		action := NormalizeRuntimeRepairRecoveryAction(item)
		if action != "" {
			ret = append(ret, action)
		}
	}
	return ret
}

func ContainsRuntimeRepairRecoveryAction(values []string, want string) bool {
	want = NormalizeRuntimeRepairRecoveryAction(want)
	if want == "" {
		return false
	}
	for _, value := range values {
		if NormalizeRuntimeRepairRecoveryAction(value) == want {
			return true
		}
	}
	return false
}
