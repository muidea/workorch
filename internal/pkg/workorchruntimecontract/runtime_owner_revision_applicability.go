package workorchruntimecontract

import "fmt"

// CloneRuntimeOwnerRevisionSet 保全原值（包括非规范表示），仅隔离可变 entries。
func CloneRuntimeOwnerRevisionSet(value *RuntimeOwnerRevisionSet) *RuntimeOwnerRevisionSet {
	if value == nil {
		return nil
	}
	cloned := *value
	if value.Entries != nil {
		cloned.Entries = append(make([]RuntimeOwnerRevisionRef, 0, len(value.Entries)), value.Entries...)
	}
	return &cloned
}

func validateRuntimeOwnerRevisionSetRaw(value *RuntimeOwnerRevisionSet) error {
	if value == nil {
		return nil
	}
	if !isRuntimeOwnerRevisionSetRawCanonical(*value) {
		return fmt.Errorf("runtime owner revision set is not raw canonical")
	}
	return value.Validate()
}

// RuntimeOwnerRevisionSetMatchesCurrent 仅比较实际依赖与调用方提供的 current owner facts。
// 无关资源不参与适用性；缺失或任一版本／dependency 字段变化均不匹配。
// 此纯函数不证明观察来源、时间或执行许可，也不选择 stale／reopen transition。
func RuntimeOwnerRevisionSetMatchesCurrent(dependencies, current RuntimeOwnerRevisionSet) (bool, error) {
	if !isRuntimeOwnerRevisionSetRawCanonical(dependencies) {
		return false, fmt.Errorf("runtime owner revision dependencies is not raw canonical")
	}
	if err := dependencies.Validate(); err != nil {
		return false, fmt.Errorf("runtime owner revision dependencies: %w", err)
	}
	if equalRuntimeOwnerRevisionSet(&dependencies, &current) {
		return true, nil
	}
	if !isRuntimeOwnerRevisionSetRawCanonical(current) {
		return false, fmt.Errorf("runtime owner revision current is not raw canonical")
	}
	if err := current.Validate(); err != nil {
		return false, fmt.Errorf("runtime owner revision current: %w", err)
	}
	if dependencies.DefinitionRevision != current.DefinitionRevision {
		return false, fmt.Errorf("runtime owner revision applicability definition mismatch")
	}
	if len(dependencies.Entries) == 0 {
		return true, nil
	}
	byResource := make(map[[4]string]RuntimeOwnerRevisionRef, len(current.Entries))
	for _, entry := range current.Entries {
		byResource[ownerRevisionResourceKey(entry)] = entry
	}
	for _, dependency := range dependencies.Entries {
		if observed, exists := byResource[ownerRevisionResourceKey(dependency)]; !exists || observed != dependency {
			return false, nil
		}
	}
	return true, nil
}
