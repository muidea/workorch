package workorchruntimecontract

import (
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeTerminalMaterialSealRawCloneDetachesOpaqueState(t *testing.T) {
	seal := RuntimeTerminalMaterialSeal{
		Material: RuntimeTerminalResultMaterial{
			EvidenceRefs: []string{}, ArtifactRefs: []common.ArtifactID{},
			RuntimeCloseout: &RuntimeResultCloseout{
				EvidenceRefs: []string{"evidence-original"},
				OriginalDiagnostic: &RuntimeOriginalDiagnostic{
					OriginalMessage: "  原始诊断\n不得改写  ", TargetRefs: []string{" target ", " target "},
				},
			},
		},
	}
	cloned := seal.CloneRaw()
	if !reflect.DeepEqual(cloned, seal) || cloned.Material.EvidenceRefs == nil || cloned.Material.ArtifactRefs == nil {
		t.Fatal("seal raw clone 丢失原值或 nil/empty 身份")
	}
	cloned.Material.RuntimeCloseout.EvidenceRefs[0] = "changed"
	cloned.Material.RuntimeCloseout.OriginalDiagnostic.TargetRefs[0] = "changed"
	if seal.Material.RuntimeCloseout.EvidenceRefs[0] != "evidence-original" || seal.Material.RuntimeCloseout.OriginalDiagnostic.TargetRefs[0] != " target " {
		t.Fatal("seal clone 共享嵌套原材料")
	}
	record := RuntimeStateRecord{TerminalMaterialSeal: &seal}
	copy := record.Clone()
	if !reflect.DeepEqual(copy, record) {
		t.Fatal("opaque state clone 规范化了 seal")
	}
	copy.TerminalMaterialSeal.Material.RuntimeCloseout.OriginalDiagnostic.OriginalMessage = "changed"
	if record.TerminalMaterialSeal.Material.RuntimeCloseout.OriginalDiagnostic.OriginalMessage != "  原始诊断\n不得改写  " {
		t.Fatal("opaque state clone 共享 seal 的原始诊断")
	}
}
