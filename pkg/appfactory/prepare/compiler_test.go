package prepare

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appruns "github.com/sipeed/picoclaw/pkg/appfactory/runs"
)

func TestCompileBookkeepingRequirement(t *testing.T) {
	bundle, err := Compile(Request{
		RequirementText:   "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		RequirementSource: "inline:test",
		Now: func() time.Time {
			return time.Date(2026, 3, 26, 8, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if bundle.PRD.ID != "prd-bookkeeping-lite" {
		t.Fatalf("PRD.ID = %q, want prd-bookkeeping-lite", bundle.PRD.ID)
	}
	if bundle.PRD.Title != "轻量记账 App" {
		t.Fatalf("PRD.Title = %q, want 轻量记账 App", bundle.PRD.Title)
	}
	if bundle.BuilderInput.TemplateID != "flutter-finance-lite" {
		t.Fatalf("BuilderInput.TemplateID = %q, want flutter-finance-lite", bundle.BuilderInput.TemplateID)
	}
	if !strings.Contains(string(bundle.Files[fitReportFileName]), "命中模板注册表条目：flutter-finance-lite") {
		t.Fatalf("template fit report should include registry selection reason: %s", string(bundle.Files[fitReportFileName]))
	}
	if len(bundle.BuilderInput.TaskBundle) != 5 {
		t.Fatalf("TaskBundle len = %d, want 5", len(bundle.BuilderInput.TaskBundle))
	}
	if len(bundle.BuilderInput.KnowledgePack) != 4 {
		t.Fatalf("KnowledgePack len = %d, want 4", len(bundle.BuilderInput.KnowledgePack))
	}
	if bundle.BuilderInput.KnowledgePack[0].SkillID != "prd-to-task-bundle" {
		t.Fatalf("first skill = %q, want prd-to-task-bundle", bundle.BuilderInput.KnowledgePack[0].SkillID)
	}
	if bundle.BuilderInput.TaskBundle[0].TaskID != "task-domain-models" {
		t.Fatalf("first task id = %q, want task-domain-models", bundle.BuilderInput.TaskBundle[0].TaskID)
	}
	if bundle.BuilderInput.TaskBundle[0].Category != "domain" {
		t.Fatalf("first task category = %q, want domain", bundle.BuilderInput.TaskBundle[0].Category)
	}
	if bundle.BuilderInput.TaskBundle[1].Category != "storage" {
		t.Fatalf("second task category = %q, want storage", bundle.BuilderInput.TaskBundle[1].Category)
	}
	if bundle.BuilderInput.TaskBundle[2].Category != "screen" {
		t.Fatalf("third task category = %q, want screen", bundle.BuilderInput.TaskBundle[2].Category)
	}
	if bundle.BuilderInput.TaskBundle[3].Category != "flow" {
		t.Fatalf("fourth task category = %q, want flow", bundle.BuilderInput.TaskBundle[3].Category)
	}
	if bundle.BuilderInput.TaskBundle[4].Category != "validation" {
		t.Fatalf("fifth task category = %q, want validation", bundle.BuilderInput.TaskBundle[4].Category)
	}
	if len(bundle.BuilderInput.TaskBundle[4].Dependencies) != 1 || bundle.BuilderInput.TaskBundle[4].Dependencies[0] != "task-flow-wiring" {
		t.Fatalf("validation dependencies = %v, want [task-flow-wiring]", bundle.BuilderInput.TaskBundle[4].Dependencies)
	}
	for _, name := range []string{requirementFileName, prdMarkdownFileName, prdJSONFileName, prdApprovalFileName, templateApprovalFileName, fitReportFileName, planFileName, constraintsFileName, builderInputFileName} {
		if _, ok := bundle.Files[name]; !ok {
			t.Fatalf("expected generated file %s", name)
		}
	}
	var prd map[string]any
	if err := json.Unmarshal(bundle.Files[prdJSONFileName], &prd); err != nil {
		t.Fatalf("Unmarshal(PRD.json) error = %v", err)
	}
	if prd["status"] != "draft" {
		t.Fatalf("status = %v, want draft", prd["status"])
	}
	var input map[string]any
	if err := json.Unmarshal(bundle.Files[builderInputFileName], &input); err != nil {
		t.Fatalf("Unmarshal(builder-input.json) error = %v", err)
	}
	if input["job_id"] != "job-bookkeeping-lite" {
		t.Fatalf("job_id = %v, want job-bookkeeping-lite", input["job_id"])
	}
	contextFiles, ok := input["context_files"].(map[string]any)
	if !ok {
		t.Fatalf("context_files type = %T, want map[string]any", input["context_files"])
	}
	if contextFiles["prd_markdown_path"] != prdMarkdownFileName {
		t.Fatalf("prd_markdown_path = %v, want %s", contextFiles["prd_markdown_path"], prdMarkdownFileName)
	}
	supportingFiles, ok := contextFiles["supporting_files"].([]any)
	if !ok || len(supportingFiles) != 3 {
		t.Fatalf("supporting_files = %v, want [requirement.md prd-approval.json template-approval.json]", contextFiles["supporting_files"])
	}
	if supportingFiles[1] != prdApprovalFileName || supportingFiles[2] != templateApprovalFileName {
		t.Fatalf("supporting_files = %v, want approval snapshots appended", supportingFiles)
	}
	var prdApproval appruns.ApprovalRecord
	if err := json.Unmarshal(bundle.Files[prdApprovalFileName], &prdApproval); err != nil {
		t.Fatalf("Unmarshal(prd-approval.json) error = %v", err)
	}
	if prdApproval.Status != appruns.ApprovalStatusApproved || prdApproval.ApprovalType != appruns.ApprovalTypePRD {
		t.Fatalf("prd approval = %+v, want approved prd record", prdApproval)
	}
	var templateApproval appruns.ApprovalRecord
	if err := json.Unmarshal(bundle.Files[templateApprovalFileName], &templateApproval); err != nil {
		t.Fatalf("Unmarshal(template-approval.json) error = %v", err)
	}
	if templateApproval.Status != appruns.ApprovalStatusApproved || templateApproval.ApprovalType != appruns.ApprovalTypeTemplate {
		t.Fatalf("template approval = %+v, want approved template record", templateApproval)
	}
	if templateApproval.SubjectVersion != "selected-template@flutter-finance-lite@v0.1.0" {
		t.Fatalf("template approval subject_version = %q, want selected-template@flutter-finance-lite@v0.1.0", templateApproval.SubjectVersion)
	}
}

func TestWriteBundle(t *testing.T) {
	bundle, err := Compile(Request{RequirementText: "做一个简单记账 app，优先验证 Builder 技术链路。"})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	outputDir := filepath.Join(t.TempDir(), "bundle")
	if err := WriteBundle(outputDir, bundle); err != nil {
		t.Fatalf("WriteBundle() error = %v", err)
	}
	for _, name := range bundle.FileNames() {
		if _, err := os.Stat(filepath.Join(outputDir, name)); err != nil {
			t.Fatalf("expected %s to exist: %v", name, err)
		}
	}
}

func TestCompileBookkeepingRequirementWithRealBuild(t *testing.T) {
	bundle, err := Compile(Request{
		RequirementText: "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		RealBuild:       true,
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if bundle.BuilderInput.ExecutorImage != "picoclaw/appfactory-builder:local" {
		t.Fatalf("ExecutorImage = %q, want picoclaw/appfactory-builder:local", bundle.BuilderInput.ExecutorImage)
	}
	if len(bundle.BuilderInput.AcceptanceChecks) != 7 {
		t.Fatalf("AcceptanceChecks len = %d, want 7", len(bundle.BuilderInput.AcceptanceChecks))
	}
	if bundle.BuilderInput.AcceptanceChecks[0].Commands[0] != "flutter pub get" {
		t.Fatalf("first command = %q, want flutter pub get", bundle.BuilderInput.AcceptanceChecks[0].Commands[0])
	}
	if bundle.BuilderInput.AcceptanceChecks[1].CheckID != "check-counter-demo-removed" {
		t.Fatalf("second check id = %q, want check-counter-demo-removed", bundle.BuilderInput.AcceptanceChecks[1].CheckID)
	}
	if bundle.BuilderInput.GoalSummary != "在 flutter-finance-lite 模板基础上完成一个可用的离线记账 MVP，必须彻底替换默认 counter demo，至少实现首页概览、记一笔录入、账单列表和本地持久化，并通过 analyze、test 和 debug APK 构建验证。" {
		t.Fatalf("GoalSummary = %q", bundle.BuilderInput.GoalSummary)
	}
	planText := string(bundle.Files[planFileName])
	if !strings.Contains(planText, "关联需求：feature-local-data, feature-home-summary") {
		t.Fatalf("implementation plan missing related requirements section: %s", planText)
	}
	if !strings.Contains(planText, "风险提示：如果实体字段频繁变化，后续页面和测试都会跟着返工") {
		t.Fatalf("implementation plan missing risk section: %s", planText)
	}
	var profile map[string]any
	if err := json.Unmarshal(bundle.BuilderInput.CommandProfile, &profile); err != nil {
		t.Fatalf("Unmarshal(command_profile) error = %v", err)
	}
	allowedCommands, ok := profile["allowed_commands"].([]any)
	if !ok || len(allowedCommands) != 2 || allowedCommands[0] != "flutter" || allowedCommands[1] != "grep" {
		t.Fatalf("allowed_commands = %v, want [flutter grep]", profile["allowed_commands"])
	}
	if len(bundle.BuilderInput.AllowedPaths) != 4 || bundle.BuilderInput.AllowedPaths[0] != "lib/**" {
		t.Fatalf("AllowedPaths = %v, want Flutter profile allowed paths", bundle.BuilderInput.AllowedPaths)
	}
	if len(bundle.BuilderInput.ProtectedPaths) != 5 || bundle.BuilderInput.ProtectedPaths[0] != "android/**" {
		t.Fatalf("ProtectedPaths = %v, want Flutter profile protected paths", bundle.BuilderInput.ProtectedPaths)
	}
	if len(bundle.BuilderInput.KnowledgePack) != 4 || bundle.BuilderInput.KnowledgePack[3].SkillID != "flutter-build-closure" {
		t.Fatalf("KnowledgePack = %+v, want Flutter skill pack through builder input", bundle.BuilderInput.KnowledgePack)
	}
}

func TestCompileBookkeepingRequirementIncludesFlutterStructuralChecksByDefault(t *testing.T) {
	bundle, err := Compile(Request{
		RequirementText: "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if bundle.BuilderInput.ExecutorImage != "" {
		t.Fatalf("ExecutorImage = %q, want empty on default compile path", bundle.BuilderInput.ExecutorImage)
	}
	if len(bundle.BuilderInput.AcceptanceChecks) != 6 {
		t.Fatalf("AcceptanceChecks len = %d, want 6", len(bundle.BuilderInput.AcceptanceChecks))
	}
	for index, checkID := range []string{"check-context-ready", "check-bookkeeping-scope", "check-plan-ready", "check-counter-demo-removed", "check-entry-form-wiring", "check-local-persistence-wiring"} {
		if bundle.BuilderInput.AcceptanceChecks[index].CheckID != checkID {
			t.Fatalf("acceptance_check[%d] = %q, want %q", index, bundle.BuilderInput.AcceptanceChecks[index].CheckID, checkID)
		}
	}
	if bundle.BuilderInput.TaskBundle[1].TargetPaths[0] != "lib/repositories/entry_repository.dart" {
		t.Fatalf("storage target_paths = %v, want repository landing path", bundle.BuilderInput.TaskBundle[1].TargetPaths)
	}
	if bundle.BuilderInput.TaskBundle[2].TargetPaths[0] != "lib/main.dart" {
		t.Fatalf("screen target_paths = %v, want main.dart landing path", bundle.BuilderInput.TaskBundle[2].TargetPaths)
	}
	if bundle.BuilderInput.TaskBundle[3].TargetPaths[0] != "lib/controllers/home_controller.dart" {
		t.Fatalf("flow target_paths = %v, want controller landing path", bundle.BuilderInput.TaskBundle[3].TargetPaths)
	}
}

func TestCompileGenericRequirementUsesRegistrySelection(t *testing.T) {
	bundle, err := Compile(Request{RequirementText: "做一个简单 Android MVP，只要一个主页面和一次核心操作。"})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if bundle.BuilderInput.TemplateID != "flutter-template-demo" {
		t.Fatalf("TemplateID = %q, want flutter-template-demo", bundle.BuilderInput.TemplateID)
	}
	fitReport := string(bundle.Files[fitReportFileName])
	if !strings.Contains(fitReport, "命中模板注册表条目：flutter-template-demo") {
		t.Fatalf("template fit report missing registry entry reason: %s", fitReport)
	}
	if !strings.Contains(fitReport, "Pinned Ref: v0.1.0") || !strings.Contains(fitReport, "Health Status: healthy") {
		t.Fatalf("template fit report missing pinned ref or health status: %s", fitReport)
	}
	if !strings.Contains(fitReport, "single-screen") || !strings.Contains(fitReport, "local-state") {
		t.Fatalf("template fit report missing capability coverage: %s", fitReport)
	}
}

func TestCompileRejectsUnknownTemplateID(t *testing.T) {
	_, err := Compile(Request{RequirementText: "做一个简单记账 app。", TemplateID: "unknown-template"})
	if err == nil {
		t.Fatal("Compile() error = nil, want unknown template failure")
	}
	if !strings.Contains(err.Error(), "not found in registry") {
		t.Fatalf("Compile() error = %v, want registry not found", err)
	}
}
