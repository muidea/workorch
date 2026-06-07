package governancecore

func RuntimeRepairAllowedRecoveryActions(input map[string]any) []string {
	if input == nil {
		return nil
	}
	value, ok := input["allowedRecoveryActions"]
	if !ok {
		return nil
	}
	switch typed := value.(type) {
	case []string:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			action := NormalizeRuntimeRepairRecoveryAction(item)
			if action != "" {
				ret = append(ret, action)
			}
		}
		return ret
	case []any:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			raw, _ := item.(string)
			action := NormalizeRuntimeRepairRecoveryAction(raw)
			if action != "" {
				ret = append(ret, action)
			}
		}
		return ret
	default:
		return nil
	}
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
