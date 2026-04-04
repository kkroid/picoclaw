package adapter

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"

	appruns "github.com/sipeed/picoclaw/pkg/appfactory/runs"
)

type ThinExecutor interface {
	Prepare(ctx context.Context, run runRecord) (RoundPlan, error)
}

type ExecutionStep struct {
	StepID  string
	Stage   appruns.ExecutionStage
	Summary string
	Command *exec.Cmd
	Check   *CheckExecutionPreview
}

type TaskExecutionPreview struct {
	TaskID    string
	Title     string
	Category  appruns.TaskCategory
	TaskType  appruns.BuilderRuntimeTaskType
	DependsOn []string
}

type CheckExecutionPreview struct {
	CheckID      string
	Label        string
	Stage        appruns.ExecutionStage
	Required     bool
	AllowFailure bool
	Commands     []string
}

type RoundPlan struct {
	Summary          string
	RoundInput       appruns.RoundInput
	EditStep         ExecutionStep
	ValidationSteps  []ExecutionStep
	TaskBundle       []TaskExecutionPreview
	AcceptanceChecks []CheckExecutionPreview
}

type defaultEditPlan struct {
	Summary string
	Files   []defaultEditFile
}

type defaultEditFile struct {
	Path    string
	Content string
	Mode    defaultEditMode
}

type defaultEditMode string

const (
	defaultEditOverwrite    defaultEditMode = "overwrite"
	defaultEditIfMissing    defaultEditMode = "if_missing"
	defaultEditIfDemo       defaultEditMode = "if_missing_or_default_demo"
	defaultEditIfIncomplete defaultEditMode = "if_missing_or_incomplete"
)

func (plan RoundPlan) Steps() []ExecutionStep {
	steps := make([]ExecutionStep, 0, 1+len(plan.ValidationSteps))
	if plan.EditStep.StepID != "" || plan.EditStep.Command != nil {
		steps = append(steps, plan.EditStep)
	}
	steps = append(steps, plan.ValidationSteps...)
	return steps
}

type StagedThinExecutor struct{}

func NewThinExecutor() ThinExecutor {
	return StagedThinExecutor{}
}

func (StagedThinExecutor) Prepare(ctx context.Context, run runRecord) (RoundPlan, error) {
	plannedRun := plannedExecutionRun(run)
	validationSteps := make([]ExecutionStep, 0, len(plannedRun.AcceptanceChecks))
	prepareCmd, editSummary, err := buildDefaultEditCommand(ctx, plannedRun)
	if err != nil {
		return RoundPlan{}, err
	}
	editStep := ExecutionStep{
		StepID:  "thin-prepare",
		Stage:   appruns.StageThinPrepare,
		Summary: editSummary,
		Command: prepareCmd,
	}
	for _, check := range plannedRun.AcceptanceChecks {
		preview := CheckExecutionPreview{
			CheckID:      check.CheckID,
			Label:        check.Label,
			Stage:        normalizeStage(check.Stage),
			Required:     check.Required,
			AllowFailure: check.AllowFailure,
			Commands:     append([]string(nil), check.Commands...),
		}
		trimmedCommands := compactCommands(check.Commands)
		if len(trimmedCommands) == 0 {
			continue
		}
		checkCmd, err := buildExecutionCommand(ctx, run, []string{"/bin/sh", "-lc", strings.Join(trimmedCommands, "\n")}, []string{
			"PICOCLAW_EXECUTION_STEP=" + check.CheckID,
			"PICOCLAW_EXECUTION_STAGE=" + string(preview.Stage),
			"PICOCLAW_ACCEPTANCE_CHECK_ID=" + check.CheckID,
		})
		if err != nil {
			return RoundPlan{}, err
		}
		validationSteps = append(validationSteps, ExecutionStep{
			StepID:  check.CheckID,
			Stage:   preview.Stage,
			Summary: buildCheckSummary(preview),
			Command: checkCmd,
			Check:   &preview,
		})
	}
	return RoundPlan{
		Summary:          buildExecutionSummary(plannedRun),
		RoundInput:       buildRoundInput(plannedRun),
		EditStep:         editStep,
		ValidationSteps:  validationSteps,
		TaskBundle:       previewTasks(plannedRun.TaskBundle),
		AcceptanceChecks: previewChecks(plannedRun.AcceptanceChecks),
	}, nil
}

func plannedExecutionRun(run runRecord) runRecord {
	planned := run
	planned.TaskBundle = plannedTaskBundle(run)
	planned.AcceptanceChecks = plannedAcceptanceChecks(run)
	return planned
}

func buildDefaultEditCommand(ctx context.Context, run runRecord) (*exec.Cmd, string, error) {
	plan, err := buildDefaultEditPlan(run)
	if err != nil {
		return nil, "", err
	}
	cmd, err := buildExecutionCommand(ctx, run, []string{"/bin/sh", "-lc", buildDefaultEditScript(plan)}, []string{
		"PICOCLAW_EXECUTION_STEP=thin-prepare",
		"PICOCLAW_EXECUTION_STAGE=" + string(appruns.StageThinPrepare),
	})
	if err != nil {
		return nil, "", err
	}
	return cmd, plan.Summary, nil
}

func buildDefaultEditPlan(run runRecord) (defaultEditPlan, error) {
	outputPath, err := defaultExecutorOutputPath(run)
	if err != nil {
		return defaultEditPlan{}, err
	}
	summary := buildExecutionSummary(run) + " | output=" + outputPath
	if run.BuilderRuntime != nil && run.BuilderRuntime.Enabled {
		if route, upgraded := selectBuilderRuntimeRoute(run); route.Model.Primary != "" {
			summary = buildExecutionSummary(run) + " | output=builder-runtime-workspace-patch"
			if route.RouteSource != "" {
				summary += " | route=" + route.RouteSource
			}
			if upgraded {
				summary += " | upgraded=true"
			}
		}
	}
	return defaultEditPlan{
		Summary: summary,
		Files: []defaultEditFile{{
			Path:    outputPath,
			Content: renderDefaultEditContent(run, outputPath),
			Mode:    defaultEditOverwrite,
		}},
	}, nil
}

func defaultExecutorOutputPath(run runRecord) (string, error) {
	for _, task := range run.TaskBundle {
		for _, targetPath := range task.TargetPaths {
			candidate, ok := candidateProbePathFromReference(targetPath)
			if ok && isProbePathAllowed(candidate, run.AllowedPaths, run.ProtectedPaths) {
				return candidate, nil
			}
		}
	}
	for _, allowedPath := range run.AllowedPaths {
		candidate, ok := candidateProbePathFromAllowedPath(allowedPath)
		if ok && isProbePathAllowed(candidate, run.AllowedPaths, run.ProtectedPaths) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("run %s has no writable allowed path for default thin executor", run.RunID)
}

func candidateProbePathFromReference(reference string) (string, bool) {
	trimmed := filepath.ToSlash(strings.TrimSpace(reference))
	if trimmed == "" || strings.ContainsAny(trimmed, "*?[]") {
		return "", false
	}
	cleaned := path.Clean(trimmed)
	if cleaned == "." || strings.HasPrefix(cleaned, "../") || path.IsAbs(cleaned) {
		return "", false
	}
	if strings.HasPrefix(cleaned, "lib/") || cleaned == "lib" {
		return path.Join("lib", defaultProbeFilename("lib")), true
	}
	if strings.HasPrefix(cleaned, "test/") || cleaned == "test" {
		return path.Join("test", defaultProbeFilename("test")), true
	}
	return path.Join(path.Dir(cleaned), defaultProbeFilename(cleaned)), true
}

func candidateProbePathFromAllowedPath(allowedPath string) (string, bool) {
	trimmed := filepath.ToSlash(strings.TrimSpace(allowedPath))
	if trimmed == "" {
		return "", false
	}
	if strings.HasSuffix(trimmed, "/**") {
		dir := path.Clean(strings.TrimSuffix(trimmed, "/**"))
		if dir == "." || dir == "" {
			return defaultProbeFilename("."), true
		}
		return path.Join(dir, defaultProbeFilename(dir)), true
	}
	if strings.ContainsAny(trimmed, "*?[]") {
		return "", false
	}
	cleaned := path.Clean(trimmed)
	if cleaned == "." || strings.HasPrefix(cleaned, "../") || path.IsAbs(cleaned) {
		return "", false
	}
	return path.Join(path.Dir(cleaned), defaultProbeFilename(cleaned)), true
}

func isProbePathAllowed(relPath string, allowedPaths, protectedPaths []string) bool {
	if strings.TrimSpace(relPath) == "" {
		return false
	}
	if matchesWorkspacePatterns(relPath, protectedPaths) {
		return false
	}
	return matchesWorkspacePatterns(relPath, allowedPaths)
}

func defaultProbeFilename(reference string) string {
	ext := filepath.Ext(reference)
	if ext == "" {
		rel := filepath.ToSlash(reference)
		if rel == "lib" || rel == "test" || strings.HasPrefix(rel, "lib/") || strings.HasPrefix(rel, "test/") {
			ext = ".dart"
		} else {
			ext = ".txt"
		}
	}
	return "picoclaw_executor_probe" + ext
}

func buildDefaultEditScript(plan defaultEditPlan) string {
	var builder strings.Builder
	for _, file := range plan.Files {
		dir := path.Dir(file.Path)
		if dir != "." {
			builder.WriteString("mkdir -p ")
			builder.WriteString(shellQuote(dir))
			builder.WriteString("\n")
		}
		switch file.Mode {
		case defaultEditIfMissing:
			builder.WriteString("if [ ! -f ")
			builder.WriteString(shellQuote(file.Path))
			builder.WriteString(" ]; then\n")
			appendDefaultEditHereDoc(&builder, file.Path, file.Content)
			builder.WriteString("fi\n")
		case defaultEditIfIncomplete:
			builder.WriteString("if [ ! -f ")
			builder.WriteString(shellQuote(file.Path))
			builder.WriteString(" ] || ! grep -E '^[[:space:]]*hive_flutter:' ")
			builder.WriteString(shellQuote(file.Path))
			builder.WriteString(" >/dev/null 2>&1; then\n")
			appendDefaultEditHereDoc(&builder, file.Path, file.Content)
			builder.WriteString("fi\n")
		case defaultEditIfDemo:
			builder.WriteString("if [ ! -f ")
			builder.WriteString(shellQuote(file.Path))
			builder.WriteString(" ] || grep -E 'Flutter Demo Home Page|You have pushed the button this many times|Counter increments smoke test|_counter|_incrementCounter|MyHomePage' ")
			builder.WriteString(shellQuote(file.Path))
			builder.WriteString(" >/dev/null 2>&1; then\n")
			appendDefaultEditHereDoc(&builder, file.Path, file.Content)
			builder.WriteString("fi\n")
		default:
			appendDefaultEditHereDoc(&builder, file.Path, file.Content)
		}
	}
	return builder.String()
}

func appendDefaultEditHereDoc(builder *strings.Builder, outputPath, content string) {
	builder.WriteString("cat > ")
	builder.WriteString(shellQuote(outputPath))
	builder.WriteString(" <<'EOF'\n")
	builder.WriteString(content)
	if !strings.HasSuffix(content, "\n") {
		builder.WriteString("\n")
	}
	builder.WriteString("EOF\n")
}

func renderDefaultEditContent(run runRecord, outputPath string) string {
	if filepath.Ext(outputPath) == ".dart" {
		return renderDefaultDartProbe(run)
	}
	return renderDefaultTextProbe(run)
}

func renderDefaultDartProbe(run runRecord) string {
	return strings.Join([]string{
		"// Generated by PicoClaw thin executor.",
		"// This file proves the default inspect -> edit -> validate loop can write a real workspace patch.",
		"const picoclawExecutorProbe = <String, Object>{",
		"  'jobId': " + dartString(run.JobID) + ",",
		"  'goalSummary': " + dartString(run.GoalSummary) + ",",
		fmt.Sprintf("  'taskCount': %d,", len(run.TaskBundle)),
		fmt.Sprintf("  'acceptanceCheckCount': %d,", len(run.AcceptanceChecks)),
		"};",
	}, "\n") + "\n"
}

func renderDefaultTextProbe(run runRecord) string {
	return strings.Join([]string{
		"Generated by PicoClaw thin executor.",
		"This file proves the default inspect -> edit -> validate loop can write a real workspace patch.",
		"job_id=" + strings.TrimSpace(run.JobID),
		"goal_summary=" + strings.TrimSpace(run.GoalSummary),
		fmt.Sprintf("task_count=%d", len(run.TaskBundle)),
		fmt.Sprintf("acceptance_check_count=%d", len(run.AcceptanceChecks)),
	}, "\n") + "\n"
}

func buildExecutionCommand(ctx context.Context, run runRecord, argv []string, extraEnv []string) (*exec.Cmd, error) {
	if strings.TrimSpace(run.ExecutorImage) != "" {
		return dockerCommand(ctx, run, argv, extraEnv)
	}
	env, err := localExecutionEnv(run)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = run.WorkspacePath
	cmd.Env = append(env, extraEnv...)
	return cmd, nil
}

func dartString(value string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"'", "\\'",
		"\n", "\\n",
		"\r", "\\r",
		"\t", "\\t",
	)
	return "'" + replacer.Replace(strings.TrimSpace(value)) + "'"
}

func localExecutionEnv(run runRecord) ([]string, error) {
	env := append([]string(nil), baseExecutionEnv(run)...)
	root := appFactoryRootFromWorkspace(run.WorkspacePath)
	pubCacheDir := resolveBuilderPubCache(root)
	if err := os.MkdirAll(pubCacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("create builder pub cache: %w", err)
	}
	gradleUserHome := resolveBuilderGradleUserHome(root)
	if err := ensureBuilderGradleUserHome(gradleUserHome); err != nil {
		return nil, fmt.Errorf("create builder gradle user home: %w", err)
	}
	env = append(env, "PUB_CACHE="+pubCacheDir)
	env = append(env, "GRADLE_USER_HOME="+gradleUserHome)
	return env, nil
}

func DockerPrepareCommandForTest(ctx context.Context, run RunRecord) (*exec.Cmd, error) {
	return dockerPrepareCommand(ctx, run)
}

func LocalExecutionEnvForTest(run RunRecord) ([]string, error) {
	return localExecutionEnv(run)
}

func baseExecutionEnv(run runRecord) []string {
	jobRoot := filepath.Clean(filepath.Join(run.WorkspacePath, ".."))
	reportsDir := filepath.Join(jobRoot, "reports")
	return []string{
		"PICOCLAW_RUN_ID=" + run.RunID,
		"PICOCLAW_JOB_ID=" + run.JobID,
		"PICOCLAW_JOB_ROOT=" + jobRoot,
		"PICOCLAW_REPORTS_DIR=" + reportsDir,
		"PICOCLAW_WORKSPACE_PATH=" + run.WorkspacePath,
		"PICOCLAW_ARTIFACT_DIR=" + run.ArtifactDir,
	}
}

func buildExecutionSummary(run runRecord) string {
	parts := make([]string, 0, 5)
	if goal := strings.TrimSpace(run.GoalSummary); goal != "" {
		parts = append(parts, goal)
	}
	if knowledgePack := buildKnowledgePackSummary(run.KnowledgePack); knowledgePack != "" {
		parts = append(parts, knowledgePack)
	}
	if taskFocus := buildTaskFocusSummary(run); taskFocus != "" {
		parts = append(parts, taskFocus)
	}
	if len(run.TaskBundle) > 0 {
		parts = append(parts, fmt.Sprintf("tasks=%d", len(run.TaskBundle)))
	}
	if len(run.AcceptanceChecks) > 0 {
		parts = append(parts, fmt.Sprintf("checks=%d", len(run.AcceptanceChecks)))
	}
	return strings.Join(parts, " | ")
}

func hasKnowledgeSkill(run runRecord, skillID string) bool {
	trimmedID := strings.TrimSpace(skillID)
	if trimmedID == "" {
		return false
	}
	for _, skill := range run.KnowledgePack {
		if strings.TrimSpace(skill.SkillID) == trimmedID {
			return true
		}
	}
	return false
}

func buildKnowledgePackSummary(skills []appruns.ProfileSkill) string {
	ordered := orderedKnowledgePack(skills)
	if len(ordered) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ordered))
	for _, skill := range ordered {
		skillID := strings.TrimSpace(skill.SkillID)
		if skillID == "" {
			continue
		}
		usageStage := strings.TrimSpace(skill.UsageStage)
		if usageStage == "" {
			parts = append(parts, skillID)
			continue
		}
		parts = append(parts, usageStage+":"+skillID)
	}
	if len(parts) == 0 {
		return ""
	}
	return "skill_flow=" + strings.Join(parts, " -> ")
}

func buildTaskFocusSummary(run runRecord) string {
	if !hasKnowledgeSkill(run, "prd-to-task-bundle") || len(run.TaskBundle) == 0 {
		return ""
	}
	focusIDs := make([]string, 0, 2)
	for _, task := range run.TaskBundle {
		if strings.TrimSpace(task.TaskID) == "" {
			continue
		}
		focusIDs = append(focusIDs, task.TaskID)
		if len(focusIDs) == 2 {
			break
		}
	}
	if len(focusIDs) == 0 {
		return ""
	}
	return "focus_tasks=" + strings.Join(focusIDs, ",")
}

func orderedKnowledgePack(skills []appruns.ProfileSkill) []appruns.ProfileSkill {
	if len(skills) == 0 {
		return nil
	}
	ordered := make([]appruns.ProfileSkill, 0, len(skills))
	used := make([]bool, len(skills))
	for _, stage := range []string{"planning", "layout", "edit", "closure"} {
		for index, skill := range skills {
			if used[index] || strings.TrimSpace(skill.UsageStage) != stage {
				continue
			}
			ordered = append(ordered, skill)
			used[index] = true
		}
	}
	for index, skill := range skills {
		if used[index] {
			continue
		}
		ordered = append(ordered, skill)
	}
	return ordered
}

func plannedTaskBundle(run runRecord) []appruns.TaskBundleItem {
	tasks := append([]appruns.TaskBundleItem(nil), run.TaskBundle...)
	if len(tasks) == 0 || !hasKnowledgeSkill(run, "prd-to-task-bundle") {
		return tasks
	}
	return orderTaskBundle(tasks)
}

func orderTaskBundle(tasks []appruns.TaskBundleItem) []appruns.TaskBundleItem {
	if len(tasks) <= 1 {
		return tasks
	}
	type taskNode struct {
		index int
		task  appruns.TaskBundleItem
	}
	nodes := make([]taskNode, 0, len(tasks))
	known := make(map[string]struct{}, len(tasks))
	for index, task := range tasks {
		nodes = append(nodes, taskNode{index: index, task: task})
		if taskID := strings.TrimSpace(task.TaskID); taskID != "" {
			known[taskID] = struct{}{}
		}
	}
	ordered := make([]appruns.TaskBundleItem, 0, len(tasks))
	completed := make(map[string]struct{}, len(tasks))
	used := make([]bool, len(nodes))
	for len(ordered) < len(tasks) {
		ready := make([]taskNode, 0, len(nodes))
		for index, node := range nodes {
			if used[index] || !taskDependenciesSatisfied(node.task, known, completed) {
				continue
			}
			ready = append(ready, node)
		}
		if len(ready) == 0 {
			remaining := make([]taskNode, 0, len(nodes))
			for index, node := range nodes {
				if used[index] {
					continue
				}
				remaining = append(remaining, node)
			}
			sort.SliceStable(remaining, func(left, right int) bool {
				return compareTaskNodes(remaining[left], remaining[right]) < 0
			})
			for _, node := range remaining {
				ordered = append(ordered, node.task)
			}
			break
		}
		sort.SliceStable(ready, func(left, right int) bool {
			return compareTaskNodes(ready[left], ready[right]) < 0
		})
		selected := ready[0]
		ordered = append(ordered, selected.task)
		if taskID := strings.TrimSpace(selected.task.TaskID); taskID != "" {
			completed[taskID] = struct{}{}
		}
		used[selected.index] = true
	}
	return ordered
}

func taskDependenciesSatisfied(task appruns.TaskBundleItem, known, completed map[string]struct{}) bool {
	for _, dependency := range task.Dependencies {
		dependency = strings.TrimSpace(dependency)
		if dependency == "" {
			continue
		}
		if _, exists := known[dependency]; !exists {
			continue
		}
		if _, done := completed[dependency]; !done {
			return false
		}
	}
	return true
}

func compareTaskNodes(left, right struct {
	index int
	task  appruns.TaskBundleItem
}) int {
	leftRank := taskCategoryRank(left.task.Category)
	rightRank := taskCategoryRank(right.task.Category)
	if leftRank != rightRank {
		return leftRank - rightRank
	}
	if len(left.task.Dependencies) != len(right.task.Dependencies) {
		return len(left.task.Dependencies) - len(right.task.Dependencies)
	}
	return left.index - right.index
}

func taskCategoryRank(category appruns.TaskCategory) int {
	switch category {
	case appruns.TaskCategoryDomain:
		return 0
	case appruns.TaskCategoryStorage:
		return 1
	case appruns.TaskCategoryScreen:
		return 2
	case appruns.TaskCategoryFlow:
		return 3
	case appruns.TaskCategoryValidation:
		return 4
	default:
		return 5
	}
}

func plannedAcceptanceChecks(run runRecord) []appruns.AcceptanceCheck {
	checks := append([]appruns.AcceptanceCheck(nil), run.AcceptanceChecks...)
	if len(checks) == 0 || !hasKnowledgeSkill(run, "flutter-build-closure") {
		return checks
	}
	sort.SliceStable(checks, func(left, right int) bool {
		return acceptanceCheckStageRank(checks[left].Stage) < acceptanceCheckStageRank(checks[right].Stage)
	})
	return checks
}

func acceptanceCheckStageRank(stage appruns.ExecutionStage) int {
	switch normalizeStage(stage) {
	case appruns.StageBaseline:
		return 0
	case appruns.StageCheap:
		return 1
	case appruns.StageMilestone:
		return 2
	case appruns.StageHeavy:
		return 3
	case appruns.StageDevice:
		return 4
	case appruns.StageSmoke:
		return 5
	case appruns.StageManual:
		return 6
	default:
		return 7
	}
}

func buildCheckSummary(check CheckExecutionPreview) string {
	parts := []string{"acceptance check: " + check.CheckID}
	if strings.TrimSpace(check.Label) != "" {
		parts = append(parts, check.Label)
	}
	parts = append(parts, "stage="+string(check.Stage))
	return strings.Join(parts, " | ")
}

func compactCommands(commands []string) []string {
	result := make([]string, 0, len(commands))
	for _, command := range commands {
		trimmed := strings.TrimSpace(command)
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	return result
}

func normalizeStage(stage appruns.ExecutionStage) appruns.ExecutionStage {
	if stage == "" {
		return appruns.StageOther
	}
	return stage
}

func previewTasks(tasks []appruns.TaskBundleItem) []TaskExecutionPreview {
	result := make([]TaskExecutionPreview, 0, len(tasks))
	for _, task := range tasks {
		result = append(result, TaskExecutionPreview{
			TaskID:    task.TaskID,
			Title:     task.Title,
			Category:  task.Category,
			TaskType:  task.EffectiveTaskType(),
			DependsOn: append([]string(nil), task.Dependencies...),
		})
	}
	return result
}

func previewChecks(checks []appruns.AcceptanceCheck) []CheckExecutionPreview {
	result := make([]CheckExecutionPreview, 0, len(checks))
	for _, check := range checks {
		result = append(result, CheckExecutionPreview{
			CheckID:      check.CheckID,
			Label:        check.Label,
			Stage:        normalizeStage(check.Stage),
			Required:     check.Required,
			AllowFailure: check.AllowFailure,
			Commands:     append([]string(nil), check.Commands...),
		})
	}
	return result
}
