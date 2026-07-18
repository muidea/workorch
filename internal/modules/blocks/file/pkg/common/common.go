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
