package common

const ModuleID = "blocks/decision"

const (
	ResolutionTypeReplan             = "replan"
	ResolutionTypeRejectCurrentPath  = "reject_current_path"
	ResolutionTypeProvideMissingInfo = "provide_missing_info"
)

func IsSupportedResolutionType(value string) bool {
	switch value {
	case ResolutionTypeReplan,
		ResolutionTypeRejectCurrentPath,
		ResolutionTypeProvideMissingInfo:
		return true
	default:
		return false
	}
}
