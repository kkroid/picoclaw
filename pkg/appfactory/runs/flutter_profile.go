package runs

import (
	"os"
	"strings"
)

func NewFlutterAndroidProfile() StackProfile {
	structuralChecks := []AcceptanceCheck{
		{
			CheckID:         "check-counter-demo-removed",
			Label:           "阻断默认 counter 模板",
			Stage:           StageCheap,
			Required:        true,
			Commands:        []string{"grep -q . lib/main.dart && grep -q . lib/models/entry.dart && grep -q . lib/models/summary.dart && grep -q . lib/views/home_page.dart && grep -q . lib/views/entry_form_page.dart && grep -q . lib/views/entry_list_page.dart && grep -q . lib/controllers/home_controller.dart && grep -q . lib/controllers/entry_form_controller.dart && grep -q . lib/controllers/entry_list_controller.dart && grep -q . lib/repositories/entry_repository.dart && ! grep -E 'Flutter Demo Home Page|You have pushed the button this many times|Counter increments smoke test|_counter|_incrementCounter|MyHomePage' lib/main.dart test/widget_test.dart >/dev/null 2>&1"},
			SuccessCriteria: "工作区中不再保留 Flutter 默认 counter demo 文案、状态字段、页面类名或 smoke test。",
			TimeoutSeconds:  30,
		},
		{
			CheckID:         "check-entry-form-wiring",
			Label:           "确认记一笔表单已落地",
			Stage:           StageCheap,
			Required:        true,
			Commands:        []string{"grep -E 'TextEditingController|TextFormField|DropdownButtonFormField|showDatePicker|categoryController|Category' lib/views/entry_form_page.dart lib/controllers/entry_form_controller.dart"},
			SuccessCriteria: "代码中存在真实录入表单或日期选择逻辑，而不是只有静态操作按钮。",
			TimeoutSeconds:  30,
		},
		{
			CheckID:         "check-local-persistence-wiring",
			Label:           "确认本地持久化已接线",
			Stage:           StageCheap,
			Required:        true,
			Commands:        []string{"grep -E 'hive_flutter|Hive' pubspec.yaml lib/repositories/entry_repository.dart lib/main.dart >/dev/null 2>&1 && ! grep -E 'shared_preferences|SharedPreferences' pubspec.yaml lib/repositories/entry_repository.dart lib/main.dart >/dev/null 2>&1"},
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
			Commands:        []string{"flutter build apk --debug --no-pub"},
			SuccessCriteria: "产出 app-debug.apk。",
			TimeoutSeconds:  1800,
		},
	}
	if deviceVerificationEnabled() {
		commandChecks = append(commandChecks,
			AcceptanceCheck{
				CheckID:         "check-adb-device-ready",
				Label:           "确认 adb 设备在线",
				Stage:           StageDevice,
				Required:        true,
				Commands:        []string{adbDeviceReadyCommand()},
				SuccessCriteria: "adb 已连接到目标设备，可继续执行安装与启动验证。",
				TimeoutSeconds:  120,
			},
			AcceptanceCheck{
				CheckID:         "check-install-debug-apk",
				Label:           "安装 Debug APK 到设备",
				Stage:           StageDevice,
				Required:        true,
				Commands:        []string{adbInstallDebugAPKCommand()},
				SuccessCriteria: "构建产出的 app-debug.apk 已成功安装到目标设备。",
				TimeoutSeconds:  300,
			},
			AcceptanceCheck{
				CheckID:         "check-launch-app-and-capture-logcat",
				Label:           "启动应用并采集 logcat",
				Stage:           StageSmoke,
				Required:        true,
				Commands:        []string{adbLaunchAndLogcatCommand()},
				SuccessCriteria: "应用已拉起，并生成最小 logcat 证据用于后续交付与排障。",
				TimeoutSeconds:  300,
			},
		)
	}
	allowedStages := []string{"baseline", "cheap", "milestone"}
	allowedCommands := []string{"flutter", "grep"}
	if deviceVerificationEnabled() {
		allowedStages = append(allowedStages, "device", "smoke")
		allowedCommands = append(allowedCommands, "adb", "sed", "head", "mkdir")
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
			AllowedStages:           allowedStages,
			AllowedCommands:         allowedCommands,
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

func deviceVerificationEnabled() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("APPFACTORY_DEVICE_VERIFICATION_ENABLED")))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func adbDeviceReadyCommand() string {
	return strings.TrimSpace(`
command -v adb >/dev/null 2>&1 || {
	echo "__picoclaw_failure_signature__:environment_check_failed:adb_binary_unavailable" >&2
	exit 1
}
if [ -n "${APPFACTORY_DEVICE_SERIAL:-}" ]; then
	adb -s "$APPFACTORY_DEVICE_SERIAL" wait-for-device || {
		echo "__picoclaw_failure_signature__:device_check_failed:adb_device_unavailable" >&2
		exit 1
	}
	adb -s "$APPFACTORY_DEVICE_SERIAL" get-state | grep -qx 'device' || {
		echo "__picoclaw_failure_signature__:device_check_failed:adb_device_unavailable" >&2
		exit 1
	}
else
	adb wait-for-device || {
		echo "__picoclaw_failure_signature__:device_check_failed:adb_device_unavailable" >&2
		exit 1
	}
	adb get-state | grep -qx 'device' || {
		echo "__picoclaw_failure_signature__:device_check_failed:adb_device_unavailable" >&2
		exit 1
	}
fi
`)
}

func adbInstallDebugAPKCommand() string {
	return strings.TrimSpace(`
APK_PATH="build/app/outputs/flutter-apk/app-debug.apk"
test -f "$APK_PATH" || {
	echo "__picoclaw_failure_signature__:environment_check_failed:debug_apk_missing" >&2
	exit 1
}
APP_ID="${APPFACTORY_ANDROID_APP_ID:-}"
if [ -z "$APP_ID" ] && [ -f android/app/build.gradle.kts ]; then
	APP_ID="$(sed -n 's/.*applicationId *= *"\([^"]*\)".*/\1/p' android/app/build.gradle.kts | head -n1)"
fi
if [ -z "$APP_ID" ] && [ -f android/app/build.gradle ]; then
	APP_ID="$(sed -n 's/.*applicationId[[:space:]]*"\([^"]*\)".*/\1/p' android/app/build.gradle | head -n1)"
fi
test -n "$APP_ID" || {
	echo "__picoclaw_failure_signature__:environment_check_failed:android_app_id_missing" >&2
	exit 1
}
install_debug_apk() {
	if [ -n "${APPFACTORY_DEVICE_SERIAL:-}" ]; then
		adb -s "$APPFACTORY_DEVICE_SERIAL" install -r "$APK_PATH"
	else
		adb install -r "$APK_PATH"
	fi
}
install_exit=0
install_output="$(install_debug_apk 2>&1)" || install_exit=$?
install_exit="${install_exit:-0}"
if [ "$install_exit" -ne 0 ] && printf '%s' "$install_output" | grep -q 'INSTALL_FAILED_UPDATE_INCOMPATIBLE'; then
	if [ -n "${APPFACTORY_DEVICE_SERIAL:-}" ]; then
		adb -s "$APPFACTORY_DEVICE_SERIAL" uninstall "$APP_ID" >/dev/null 2>&1 || true
	else
		adb uninstall "$APP_ID" >/dev/null 2>&1 || true
	fi
	install_exit=0
	install_output="$(install_debug_apk 2>&1)" || install_exit=$?
	install_exit="${install_exit:-0}"
fi
if [ "$install_exit" -ne 0 ]; then
	printf '%s\n' "$install_output" >&2
	echo "__picoclaw_failure_signature__:device_check_failed:apk_install_failed" >&2
	exit 1
fi
`)
}

func adbLaunchAndLogcatCommand() string {
	return strings.TrimSpace(`
REPORTS_DIR="${PICOCLAW_REPORTS_DIR:-../reports}"
mkdir -p "$REPORTS_DIR"
APP_ID="${APPFACTORY_ANDROID_APP_ID:-}"
if [ -z "$APP_ID" ] && [ -f android/app/build.gradle.kts ]; then
	APP_ID="$(sed -n 's/.*applicationId *= *"\([^"]*\)".*/\1/p' android/app/build.gradle.kts | head -n1)"
fi
if [ -z "$APP_ID" ] && [ -f android/app/build.gradle ]; then
	APP_ID="$(sed -n 's/.*applicationId[[:space:]]*"\([^"]*\)".*/\1/p' android/app/build.gradle | head -n1)"
fi
test -n "$APP_ID" || {
	echo "__picoclaw_failure_signature__:environment_check_failed:android_app_id_missing" >&2
	exit 1
}
if [ -n "${APPFACTORY_DEVICE_SERIAL:-}" ]; then
	adb -s "$APPFACTORY_DEVICE_SERIAL" shell monkey -p "$APP_ID" -c android.intent.category.LAUNCHER 1 || {
		echo "__picoclaw_failure_signature__:device_check_failed:app_launch_failed" >&2
		exit 1
	}
	adb -s "$APPFACTORY_DEVICE_SERIAL" logcat -d > "$REPORTS_DIR/device-logcat.txt" || {
		echo "__picoclaw_failure_signature__:device_check_failed:log_capture_failed" >&2
		exit 1
	}
	if [ "${APPFACTORY_DEVICE_CAPTURE_SCREENSHOT:-0}" = "1" ]; then
		adb -s "$APPFACTORY_DEVICE_SERIAL" exec-out screencap -p > "$REPORTS_DIR/device-screenshot.png"
	fi
else
	adb shell monkey -p "$APP_ID" -c android.intent.category.LAUNCHER 1 || {
		echo "__picoclaw_failure_signature__:device_check_failed:app_launch_failed" >&2
		exit 1
	}
	adb logcat -d > "$REPORTS_DIR/device-logcat.txt" || {
		echo "__picoclaw_failure_signature__:device_check_failed:log_capture_failed" >&2
		exit 1
	}
	if [ "${APPFACTORY_DEVICE_CAPTURE_SCREENSHOT:-0}" = "1" ]; then
		adb exec-out screencap -p > "$REPORTS_DIR/device-screenshot.png"
	fi
fi
test -s "$REPORTS_DIR/device-logcat.txt" || {
	echo "__picoclaw_failure_signature__:device_check_failed:log_capture_failed" >&2
	exit 1
}
grep -Eq 'FATAL EXCEPTION|AndroidRuntime|Process[[:space:]].*[[:space:]]has died' "$REPORTS_DIR/device-logcat.txt" && {
	echo "__picoclaw_failure_signature__:device_check_failed:app_runtime_crash" >&2
	exit 1
}
`)
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
