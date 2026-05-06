package bootstrap

import (
	"strings"
	"testing"
)

func TestEmbeddedSoftwareDevTemplateDoDItemsCoverScopeEvidenceAndRisk(t *testing.T) {
	templates, err := loadEmbeddedTemplates()
	if err != nil {
		t.Fatalf("loadEmbeddedTemplates: %v", err)
	}
	item := findEmbeddedTemplateByID(templates, "software-dev")
	if item == nil {
		t.Fatal("software-dev template missing")
	}
	if len(item.DoDItems) < 6 {
		t.Fatalf("software-dev DoD items = %d, want >= 6", len(item.DoDItems))
	}

	assertContainsAnyDoDItem(t, item.DoDItems, "Goal")
	assertContainsAnyDoDItem(t, item.DoDItems, "unified_diff")
	assertContainsAnyDoDItem(t, item.DoDItems, "build/test")
	assertContainsAnyDoDItem(t, item.DoDItems, "lint/静态检查")
	assertContainsAnyDoDItem(t, item.DoDItems, "残余风险")
	assertContainsAnyDoDItem(t, item.DoDItems, "evidenceRefs")
}

func TestEmbeddedSoftwareDevSoulsContainPlanningExecutionValidationArtifactGuidance(t *testing.T) {
	souls, err := loadEmbeddedSouls()
	if err != nil {
		t.Fatalf("loadEmbeddedSouls: %v", err)
	}

	planning := findEmbeddedSoulByID(souls, "soul-software-dev-planning")
	if planning == nil {
		t.Fatal("soul-software-dev-planning missing")
	}
	assertSoulContains(t, planning.SoulMD, "技术栈")
	assertSoulContains(t, planning.SoulMD, "受影响文件")
	assertSoulContains(t, planning.SoulMD, "DoD 条目")

	execution := findEmbeddedSoulByID(souls, "soul-software-dev-execution")
	if execution == nil {
		t.Fatal("soul-software-dev-execution missing")
	}
	assertSoulContains(t, execution.SoulMD, "最小代码或配置变更")
	assertSoulContains(t, execution.SoulMD, "每轮修改后先执行最小相关验证")
	assertSoulContains(t, execution.SoulMD, "失败命令")

	validation := findEmbeddedSoulByID(souls, "soul-software-dev-validation")
	if validation == nil {
		t.Fatal("soul-software-dev-validation missing")
	}
	assertSoulContains(t, validation.SoulMD, "satisfied/failed/skipped")
	assertSoulContains(t, validation.SoulMD, "逐项")
	assertSoulContains(t, validation.SoulMD, "evidenceRefs")

	artifact := findEmbeddedSoulByID(souls, "soul-software-dev-artifact")
	if artifact == nil {
		t.Fatal("soul-software-dev-artifact missing")
	}
	assertSoulContains(t, artifact.SoulMD, "DoD 条目证据引用")
	assertSoulContains(t, artifact.SoulMD, "不要只写")
}

func findEmbeddedTemplateByID(items []embeddedTemplate, id string) *embeddedTemplate {
	for i := range items {
		if items[i].ID == id {
			return &items[i]
		}
	}
	return nil
}

func findEmbeddedSoulByID(items []embeddedSoul, id string) *embeddedSoul {
	for i := range items {
		if items[i].ID == id {
			return &items[i]
		}
	}
	return nil
}

func assertContainsAnyDoDItem(t *testing.T, items []string, want string) {
	t.Helper()
	for _, item := range items {
		if strings.Contains(item, want) {
			return
		}
	}
	t.Fatalf("DoD items %q missing keyword %q", items, want)
}

func assertSoulContains(t *testing.T, text string, want string) {
	t.Helper()
	if !strings.Contains(text, want) {
		t.Fatalf("soul markdown missing %q", want)
	}
}
