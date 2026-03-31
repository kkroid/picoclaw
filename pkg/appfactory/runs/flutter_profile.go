package runs

func NewFlutterAndroidProfile() StackProfile {
	structuralChecks := []AcceptanceCheck{
		{
			CheckID:         "check-counter-demo-removed",
			Label:           "阻断默认 counter 模板",
			Stage:           StageCheap,
			Required:        true,
			Commands:        []string{"grep -q . lib/main.dart lib/views/home_page.dart lib/views/entry_form_page.dart lib/views/entry_list_page.dart lib/controllers/home_controller.dart lib/controllers/entry_form_controller.dart lib/controllers/entry_list_controller.dart lib/repositories/entry_repository.dart && ! grep -E 'Flutter Demo Home Page|You have pushed the button this many times|Counter increments smoke test|_counter|_incrementCounter|MyHomePage' lib/main.dart test/widget_test.dart >/dev/null 2>&1"},
			SuccessCriteria: "工作区中不再保留 Flutter 默认 counter demo 文案、状态字段、页面类名或 smoke test。",
			TimeoutSeconds:  30,
		},
		{
			CheckID:         "check-entry-form-wiring",
			Label:           "确认记一笔表单已落地",
			Stage:           StageCheap,
			Required:        true,
			Commands:        []string{"grep -E 'TextEditingController|TextFormField|DropdownButtonFormField|showDatePicker' lib/views/entry_form_page.dart lib/controllers/entry_form_controller.dart"},
			SuccessCriteria: "代码中存在真实录入表单或日期选择逻辑，而不是只有静态操作按钮。",
			TimeoutSeconds:  30,
		},
		{
			CheckID:         "check-local-persistence-wiring",
			Label:           "确认本地持久化已接线",
			Stage:           StageCheap,
			Required:        true,
			Commands:        []string{"grep -E 'shared_preferences|SharedPreferences|sqflite|hive|isar' pubspec.yaml lib/repositories/entry_repository.dart lib/main.dart"},
			SuccessCriteria: "代码或依赖中出现明确的本地持久化实现，而不是只展示静态账单。",
			TimeoutSeconds:  30,
		},
	}
	commandChecks := []AcceptanceCheck{
		{
			CheckID:         "check-flutter-pub-get",
			Label:           "安装 Flutter 依赖",
			Stage:           StageBaseline,
			Required:        true,
			Commands:        []string{"flutter pub get"},
			SuccessCriteria: "Flutter 依赖安装成功。",
			TimeoutSeconds:  300,
		},
		{
			CheckID:         "check-flutter-analyze",
			Label:           "执行静态检查",
			Stage:           StageCheap,
			Required:        true,
			Commands:        []string{"flutter analyze"},
			SuccessCriteria: "Flutter analyze 无错误。",
			TimeoutSeconds:  300,
		},
		{
			CheckID:         "check-flutter-test",
			Label:           "执行模板测试",
			Stage:           StageCheap,
			Required:        true,
			Commands:        []string{"flutter test"},
			SuccessCriteria: "Widget test 全部通过。",
			TimeoutSeconds:  300,
		},
		{
			CheckID:         "check-flutter-build-apk",
			Label:           "构建 Debug APK",
			Stage:           StageMilestone,
			Required:        true,
			Commands:        []string{"flutter build apk --debug"},
			SuccessCriteria: "产出 app-debug.apk。",
			TimeoutSeconds:  1800,
		},
	}
	return StackProfile{
		ProfileID: "flutter-android-p0",
		Stack:     "flutter",
		WorkspaceDetector: WorkspaceDetector{
			RequiredFiles:  []string{"pubspec.yaml", "lib/main.dart", "test/widget_test.dart"},
			RequiredDirs:   []string{"lib", "test", "android"},
			AndroidMarkers: []string{"android/app/build.gradle.kts", "android/app/src/main/AndroidManifest.xml"},
			ForbiddenMarker: []string{
				"Flutter Demo Home Page",
				"You have pushed the button this many times",
				"Counter increments smoke test",
				"_counter",
				"_incrementCounter",
				"MyHomePage",
			},
		},
		StructuralChecks: structuralChecks,
		CommandChecks:    commandChecks,
		CommandProfile: CommandProfile{
			ProfileName:             "flutter-builder-p0",
			AllowedStages:           []string{"baseline", "cheap", "milestone"},
			AllowedCommands:         []string{"flutter", "grep"},
			DeniedCommands:          []string{"rm", "sudo"},
			MaxSingleCommandSeconds: 1800,
			MaxParallelCommands:     1,
			NetworkPolicy:           "open",
			WritableRoots:           []string{"."},
			EnvAllowlist:            []string{"PATH", "HOME", "JAVA_HOME", "ANDROID_HOME", "ANDROID_SDK_ROOT", "PUB_CACHE", "FLUTTER_ROOT"},
		},
		AllowedPaths:   []string{"lib/**", "assets/**", "pubspec.yaml", "test/**"},
		ProtectedPaths: []string{"android/**", "ios/**", "linux/**", "macos/**", "windows/**"},
		KnowledgePack: []ProfileSkill{
			{SkillID: "prd-to-task-bundle", Role: "把 PRD 压成 Flutter profile 可执行任务包", UsageStage: "planning", Scope: "flutter-android-p0", Notes: "只服务 Flutter 首个 profile，不属于执行器内核。"},
			{SkillID: "flutter-mvc-template", Role: "固定 Flutter MVC 目录、依赖与页面落点", UsageStage: "layout", Scope: "flutter-android-p0", Notes: "负责模板和目录规则，不负责状态机与调度。"},
			{SkillID: "builder-direct-edit", Role: "约束 Builder 在 Flutter 工作区中直接改码", UsageStage: "edit", Scope: "flutter-android-p0", Notes: "负责单轮直接改码行为，不负责超时、重试和预算。"},
			{SkillID: "flutter-build-closure", Role: "在 Flutter 工作区接近完成时做低风险收口", UsageStage: "closure", Scope: "flutter-android-p0", Notes: "只处理 analyze/test/build 收口，不重做功能实现。"},
		},
		Metadata: map[string]string{
			"template_family":  "flutter-mvc-seed",
			"android_required": "true",
		},
	}
}

func (profile StackProfile) AcceptanceChecks() []AcceptanceCheck {
	checks := make([]AcceptanceCheck, 0, len(profile.StructuralChecks)+len(profile.CommandChecks))
	if len(profile.CommandChecks) > 0 {
		checks = append(checks, profile.CommandChecks[0])
	}
	checks = append(checks, profile.StructuralChecks...)
	if len(profile.CommandChecks) > 1 {
		checks = append(checks, profile.CommandChecks[1:]...)
	}
	return checks
}
