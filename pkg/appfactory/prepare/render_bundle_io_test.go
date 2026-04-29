package prepare

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRenderRequirementLayerIncludesHighlightsAndAssumptions(t *testing.T) {
	spec, _ := newLayerTestFixture()
	requirement := renderRequirement(spec)

	for _, expected := range []string{"# 原始需求", "## 需求摘录", "## 当前假设", "首页需要看到待整理、进行中、已完成数量。", "默认离线单机运行。"} {
		if !strings.Contains(requirement, expected) {
			t.Fatalf("renderRequirement() missing %q:\n%s", expected, requirement)
		}
	}
}

func TestRenderPRDMarkdownLayerIncludesUsersFlowsAndEntities(t *testing.T) {
	_, prd := newLayerTestFixture()
	markdown := renderPRDMarkdown(prd)

	for _, expected := range []string{"# 待办事项 App", "## 目标用户", "## 用户流程", "### 创建待办", "## 数据实体", "待办概览摘要"} {
		if !strings.Contains(markdown, expected) {
			t.Fatalf("renderPRDMarkdown() missing %q:\n%s", expected, markdown)
		}
	}
}

func TestRenderTemplateFitReportLayerIncludesTemplateMetadata(t *testing.T) {
	spec, _ := newLayerTestFixture()
	report := renderTemplateFitReport(spec)

	for _, expected := range []string{"# 模板适配报告", "- Template ID: flutter-open-lite", "- Template Name: Flutter Open Lite", "- Pinned Ref: v0.2.0", "- Health Status: healthy", "## 选择理由", "## 当前差距"} {
		if !strings.Contains(report, expected) {
			t.Fatalf("renderTemplateFitReport() missing %q:\n%s", expected, report)
		}
	}
}

func TestRenderManualConstraintsLayerIncludesHumanNotes(t *testing.T) {
	spec, _ := newLayerTestFixture()
	constraints := renderManualConstraints(spec)

	for _, expected := range []string{"# 人工约束", "## 人工备注", "当前待办模板默认本地优先，不接入推送、协作、日历同步。", "Builder 后续必须把中性骨架收口为待办事项 app", "需求拆解后二开应优先复用概览、集合浏览、实体变更、结果检查四类承载单元"} {
		if !strings.Contains(constraints, expected) {
			t.Fatalf("renderManualConstraints() missing %q:\n%s", expected, constraints)
		}
	}
}

func TestBundleFileNamesLayerReturnsSortedNames(t *testing.T) {
	bundle := Bundle{Files: map[string][]byte{
		"zeta.txt":  []byte("z"),
		"alpha.txt": []byte("a"),
		"mid.txt":   []byte("m"),
	}}

	names := bundle.FileNames()
	joined := strings.Join(names, ",")
	if joined != "alpha.txt,mid.txt,zeta.txt" {
		t.Fatalf("FileNames() = %q, want alpha.txt,mid.txt,zeta.txt", joined)
	}
}

func TestWriteBundleLayerRejectsEmptyOutputDir(t *testing.T) {
	err := WriteBundle("", Bundle{Files: map[string][]byte{"a.txt": []byte("a")}})
	if err == nil || !strings.Contains(err.Error(), "output dir is required") {
		t.Fatalf("WriteBundle() error = %v, want output dir is required", err)
	}
}

func TestWriteBundleLayerWritesFilesAtomically(t *testing.T) {
	spec, prd := newLayerTestFixture()
	bundle, err := buildBundle(spec, prd, time.Date(2026, 4, 6, 11, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("buildBundle() error = %v", err)
	}

	outputDir := filepath.Join(t.TempDir(), "prepare-output")
	if err := WriteBundle(outputDir, bundle); err != nil {
		t.Fatalf("WriteBundle() error = %v", err)
	}

	for _, name := range []string{builderInputFileName, prdMarkdownFileName, planningContextFileName} {
		content, readErr := os.ReadFile(filepath.Join(outputDir, name))
		if readErr != nil {
			t.Fatalf("ReadFile(%s) error = %v", name, readErr)
		}
		if string(content) != string(bundle.Files[name]) {
			t.Fatalf("written content mismatch for %s", name)
		}
	}
}
