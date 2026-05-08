package bootstrap

import (
	"context"
	"strings"
	"testing"

	soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
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
	assertSoulContains(t, planning.SoulMD, "workspace 文件清单为空不等于缺少源码")
	assertSoulContains(t, planning.SoulMD, "新项目初始化场景")

	execution := findEmbeddedSoulByID(souls, "soul-software-dev-execution")
	if execution == nil {
		t.Fatal("soul-software-dev-execution missing")
	}
	assertSoulContains(t, execution.SoulMD, "最小代码或配置变更")
	assertSoulContains(t, execution.SoulMD, "workspace 为空且 Goal/DoD 未要求既有源码")
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

	decision := findEmbeddedSoulByID(souls, "soul-software-dev-decision")
	if decision == nil {
		t.Fatal("soul-software-dev-decision missing")
	}
	assertSoulContains(t, decision.SoulMD, "不要因为 workspace 为空就直接请求源码")
}

func TestEnsureBuiltinDefaultsUpsertsEmbeddedSoulsAndTemplates(t *testing.T) {
	ctx := context.Background()
	soulRepo := memory.NewSoulRepository()
	templateRepo := memory.NewWorkspaceTemplateRepository()

	if err := soulRepo.Create(ctx, &soulmodel.Soul{
		ID:     common.SoulID("soul-software-dev-planning"),
		Name:   "stale planning soul",
		SoulMD: "# stale\n",
	}); err != nil {
		t.Fatalf("seed stale soul: %v", err)
	}
	if err := templateRepo.Create(ctx, &repository.WorkspaceTemplate{
		ID:       "software-dev",
		Name:     "stale template",
		DoDItems: []string{"stale dod"},
		SoulBindings: map[string]string{
			"planning": "stale-soul",
		},
	}); err != nil {
		t.Fatalf("seed stale template: %v", err)
	}

	if err := EnsureBuiltinDefaults(ctx, soulRepo, templateRepo); err != nil {
		t.Fatalf("EnsureBuiltinDefaults: %v", err)
	}

	planning, err := soulRepo.Get(ctx, common.SoulID("soul-software-dev-planning"))
	if err != nil {
		t.Fatalf("get planning soul: %v", err)
	}
	assertSoulContains(t, planning.SoulMD, "workspace 文件清单为空不等于缺少源码")
	if planning.Name == "stale planning soul" || strings.Contains(planning.SoulMD, "# stale") {
		t.Fatalf("planning soul was not refreshed: %+v", planning)
	}

	template, err := templateRepo.Get(ctx, "software-dev")
	if err != nil {
		t.Fatalf("get software-dev template: %v", err)
	}
	if template.Name == "stale template" || template.SoulBindings["planning"] != "soul-software-dev-planning" {
		t.Fatalf("software-dev template was not refreshed: %+v", template)
	}
	assertContainsAnyDoDItem(t, template.DoDItems, "evidenceRefs")
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
