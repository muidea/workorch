package configcore

func boolPtr(value bool) *bool {
	return &value
}

func isEnabled(value *bool) bool {
	return value != nil && *value
}

func cloneFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneBoolPtr(source *bool) *bool {
	if source == nil {
		return nil
	}
	value := *source
	return &value
}
