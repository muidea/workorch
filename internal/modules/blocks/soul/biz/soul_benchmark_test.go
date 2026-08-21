package soul

import (
	"context"
	"fmt"
	"strings"
	"testing"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/common"
	soulevents "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/events"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

var soulBenchmarkSink *soulmodel.Soul

func TestServiceAcceptsNearLimitSoulAndRejectsAggregateOverflow(t *testing.T) {
	service := NewService(memory.NewSoulRepository())
	command := nearLimitSoulCommand()
	created, err := service.CreateSoul(context.Background(), command)
	if err != nil {
		t.Fatalf("create near-limit Soul: %v", err)
	}
	contentBytes := soulContentBytes(created)
	if contentBytes > modulecommon.SoulContentMaxBytes || contentBytes < modulecommon.SoulContentMaxBytes-512 {
		t.Fatalf("near-limit Soul bytes=%d, max=%d", contentBytes, modulecommon.SoulContentMaxBytes)
	}
	overflow := command
	overflow.SoulID = "soul-overflow"
	overflow.Name = strings.Repeat("n", modulecommon.SoulNameMaxBytes)
	if _, err := service.CreateSoul(context.Background(), overflow); err == nil {
		t.Fatal("aggregate Soul content overflow was accepted")
	}
}

func BenchmarkNormalizeNearLimitSoul(b *testing.B) {
	command := nearLimitSoulCommand()
	b.ReportAllocs()
	b.SetBytes(modulecommon.SoulContentMaxBytes)
	b.ResetTimer()
	for range b.N {
		var err error
		soulBenchmarkSink, err = normalizedSoulFromCommand(command)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGetNearLimitSoul(b *testing.B) {
	service := NewService(memory.NewSoulRepository())
	if _, err := service.CreateSoul(context.Background(), nearLimitSoulCommand()); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.SetBytes(modulecommon.SoulContentMaxBytes)
	b.ResetTimer()
	for range b.N {
		var err error
		soulBenchmarkSink, err = service.GetSoul(context.Background(), common.SoulID("soul-near-limit"))
		if err != nil {
			b.Fatal(err)
		}
	}
}

func nearLimitSoulCommand() soulevents.CreateSoulCommand {
	values := make([]string, modulecommon.SoulListValueMaxCount)
	for idx := range values {
		values[idx] = fmt.Sprintf("value-%02d-%s", idx, strings.Repeat("x", 65))
	}
	clone := func() []string { return append([]string(nil), values...) }
	return soulevents.CreateSoulCommand{
		SoulID: "soul-near-limit", Name: "Near Limit Soul",
		Description:     strings.Repeat("d", modulecommon.SoulDescriptionMaxBytes),
		SoulMD:          strings.Repeat("s", modulecommon.SoulInstructionMaxBytes-1),
		BusinessDomains: clone(), AllowedSkillRefs: clone(), AllowedMCPServerRefs: clone(),
		AllowedCapabilityRefs: clone(), AllowedCapabilityCategories: clone(), AllowedToolKinds: clone(),
	}
}
