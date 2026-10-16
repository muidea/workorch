package workorchruntimecontract

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func sourceInventoryHashMutation(i *RuntimeSourceObligationInventory, arm uint8, text string) {
	switch arm % 12 {
	case 0:
		i.CandidateHash = text
	case 1:
		i.SpecificationHash = text
	case 2:
		i.DefinitionRevision = text
	case 3:
		i.StartupInputHash = text
	case 4:
		i.Sources[0].Text = text
	case 5:
		i.Requirements[0].Text = text
	case 6:
		i.Obligations[0].Text = text
	case 7:
		i.Verifications[0].Declaration = text
	case 8:
		i.ExecutionProofMappings = []RuntimeExecutionProofMapping{{ContractRef: text, ConsumerItemRef: "consumer", RuleRef: "rule", SourceItemRef: "source", ConsumerGoalText: text}}
	case 9:
		i.EvidencePurposeMappings = []RuntimeEvidencePurposeMapping{{ContractRef: text, HistoricalItemRef: "source", AcceptanceItemRef: "acceptance", HistoricalProfileID: "profile", HistoricalDeliverableID: "deliverable", HistoricalProfileHash: text, HistoricalText: text}}
	case 10:
		i.Requirements[0].ResourceIDs = []string{text, text}
	case 11:
		i.SchemaVersion = text
	}
}

func compareSourceInventoryBufferedHash(t *testing.T, i RuntimeSourceObligationInventory) {
	t.Helper()
	before, _ := json.Marshal(i)
	if StableRuntimeSourceObligationInventoryHash(i) != referenceSourceInventoryUnbufferedHash(i) {
		t.Fatal("inventory length/field/cardinality hash changed")
	}
	after, _ := json.Marshal(i)
	if !bytes.Equal(before, after) {
		t.Fatal("hash changed source bytes")
	}
	body, _ := json.Marshal(i)
	var loaded RuntimeSourceObligationInventory
	if err := json.Unmarshal(body, &loaded); err != nil {
		t.Fatal(err)
	}
	if StableRuntimeSourceObligationInventoryHash(loaded) != referenceSourceInventoryUnbufferedHash(loaded) {
		t.Fatal("reload hash differs from original encoder")
	}
}

func TestSourceInventoryBufferedHashRetainsOriginalEncoding(t *testing.T) {
	compareSourceInventoryBufferedHash(t, RuntimeSourceObligationInventory{})
	for _, text := range []string{"", "原文\x00\xff", strings.Repeat("s", 120), strings.Repeat("s", 121), strings.Repeat("long original 文本", 512)} {
		for arm := uint8(0); arm < 12; arm++ {
			i := sourceObligationInventoryFixture()
			sourceInventoryHashMutation(&i, arm, text)
			compareSourceInventoryBufferedHash(t, i)
		}
	}
	i := sourceObligationInventoryFixture()
	i.Hash = StableRuntimeSourceObligationInventoryHash(i)
	if err := i.Validate(); err != nil {
		t.Fatal(err)
	}
	i.Sources[0].Text += " changed"
	if i.Validate() == nil {
		t.Fatal("hash reuse accepted subsequent source drift")
	}
}

func FuzzSourceInventoryBufferedHashMatchesOriginal(f *testing.F) {
	for arm := uint8(0); arm < 12; arm++ {
		f.Add(arm, "原文\x00\xff")
	}
	f.Fuzz(func(t *testing.T, arm uint8, text string) {
		if len(text) > 4096 {
			t.Skip()
		}
		i := sourceObligationInventoryFixture()
		sourceInventoryHashMutation(&i, arm, text)
		compareSourceInventoryBufferedHash(t, i)
	})
}

func BenchmarkSourceInventoryBufferedHash(b *testing.B) {
	i := sourceObligationInventoryFixture()
	for _, reference := range []bool{true, false} {
		name := "buffered"
		if reference {
			name = "original"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				if reference {
					_ = referenceSourceInventoryUnbufferedHash(i)
				} else {
					_ = StableRuntimeSourceObligationInventoryHash(i)
				}
			}
		})
	}
}
