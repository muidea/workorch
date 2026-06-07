package activationcore

import "strings"

func CompactTargetSnapshots(input map[string]any) []map[string]any {
	var ret []map[string]any
	seen := map[string]bool{}
	add := func(raw any, source string) {
		for _, item := range MapSlice(raw) {
			targetRef := strings.TrimSpace(stringFromPayload(item, "targetRef"))
			if targetRef == "" || seen[targetRef] {
				continue
			}
			seen[targetRef] = true
			snapshot := map[string]any{}
			for _, key := range []string{
				"targetRef",
				"exists",
				"contentHash",
				"contentComplete",
				"safeForFileWrite",
				"redacted",
				"truncated",
				"isBinary",
				"size",
				"source",
				"error",
			} {
				if value, ok := item[key]; ok && value != nil {
					snapshot[key] = CompactContextValue(value, 1)
				}
			}
			if source != "" {
				snapshot["contextSource"] = source
			}
			ret = append(ret, snapshot)
			if len(ret) >= 20 {
				return
			}
		}
	}
	add(input["targetSnapshots"], "lineage_target")
	if len(ret) < 20 {
		add(input["failureTargetRepairContext"], "failure_target")
	}
	if len(ret) < 20 {
		add(input["patchRepairContext"], "patch_target")
	}
	return ret
}
