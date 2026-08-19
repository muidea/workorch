package common

const (
	ModuleID = "blocks/skill"

	SkillListDefaultLimit = 100
	SkillListMaxLimit     = 200

	SkillIDMaxBytes          = 256
	SkillNameMaxBytes        = 256
	SkillVersionMaxBytes     = 128
	SkillDescriptionMaxBytes = 4 * 1024
	SkillInstructionMaxBytes = 64 * 1024
	SkillListValueMaxBytes   = 2 * 1024
	SkillListValueMaxCount   = 64
	SkillContentMaxBytes     = 96 * 1024
)
