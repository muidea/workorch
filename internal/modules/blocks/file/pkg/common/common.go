package common

import runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

const ModuleID = runtimemodel.RuntimeModuleFile

const StepKindList = runtimemodel.StepKindFileList
const StepKindRead = runtimemodel.StepKindFileRead
const StepKindEdit = runtimemodel.StepKindFileEdit
const StepKindWrite = runtimemodel.StepKindFileWrite
const StepKindPatchApply = runtimemodel.RuntimeBlockKindPatchApply
const StepKindPatchPreview = runtimemodel.RuntimeBlockKindPatchPreview
const StepKindPatchRevert = runtimemodel.RuntimeBlockKindPatchRevert

// RuntimeCapabilityKinds is the File owner's complete Runtime V2 capability
// inventory. Callers receive a fresh slice so a consumer cannot mutate owner
// authority in process.
func RuntimeCapabilityKinds() []string {
	return []string{StepKindList, StepKindRead, StepKindEdit, StepKindWrite, StepKindPatchApply, StepKindPatchPreview}
}
