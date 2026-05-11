package runs

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNewFlutterAndroidProfile(t *testing.T) {
	profile := NewFlutterAndroidProfile()
	if profile.ProfileID != "flutter-android-p0" {
		t.Fatalf("ProfileID = %q, want flutter-android-p0", profile.ProfileID)
	}
	if profile.Stack != "flutter" {
		t.Fatalf("Stack = %q, want flutter", profile.Stack)
	}
	if len(profile.WorkspaceDetector.RequiredFiles) != 3 {
		t.Fatalf("RequiredFiles len = %d, want 3", len(profile.WorkspaceDetector.RequiredFiles))
	}
	if len(profile.WorkspaceDetector.AndroidMarkers) != 2 || profile.WorkspaceDetector.AndroidMarkers[0] != "android/app/build.gradle.kts" {
		t.Fatalf("AndroidMarkers = %v, want Kotlin DSL app marker first", profile.WorkspaceDetector.AndroidMarkers)
	}
	if len(profile.StructuralChecks) != 3 {
		t.Fatalf("StructuralChecks len = %d, want 3", len(profile.StructuralChecks))
	}
	if len(profile.CommandChecks) != 4 {
		t.Fatalf("CommandChecks len = %d, want 4", len(profile.CommandChecks))
	}
	if len(profile.KnowledgePack) != 4 {
		t.Fatalf("KnowledgePack len = %d, want 4", len(profile.KnowledgePack))
	}
	if profile.KnowledgePack[0].SkillID != "prd-to-task-bundle" {
		t.Fatalf("first skill = %q, want prd-to-task-bundle", profile.KnowledgePack[0].SkillID)
	}
	if profile.KnowledgePack[3].SkillID != "flutter-build-closure" {
		t.Fatalf("last skill = %q, want flutter-build-closure", profile.KnowledgePack[3].SkillID)
	}
	if profile.KnowledgePack[0].Scope != "flutter-android-p0" {
		t.Fatalf("skill scope = %q, want flutter-android-p0", profile.KnowledgePack[0].Scope)
	}
	checks := profile.AcceptanceChecks()
	if len(checks) != 7 {
		t.Fatalf("AcceptanceChecks len = %d, want 7", len(checks))
	}
	if checks[0].CheckID != "check-flutter-pub-get" {
		t.Fatalf("first check = %q, want check-flutter-pub-get", checks[0].CheckID)
	}
	if checks[1].CheckID != "check-counter-demo-removed" {
		t.Fatalf("second check = %q, want check-counter-demo-removed", checks[1].CheckID)
	}
	if checks[4].CheckID != "check-flutter-analyze" {
		t.Fatalf("fifth check = %q, want check-flutter-analyze", checks[4].CheckID)
	}
	for _, expected := range []string{
		"grep -q . lib/models/*.dart",
		"grep -q . lib/views/*.dart",
		"grep -q . lib/controllers/*.dart",
		"grep -q . lib/repositories/*.dart",
	} {
		if !strings.Contains(checks[1].Commands[0], expected) {
			t.Fatalf("check-counter-demo-removed command = %q, want contain %q", checks[1].Commands[0], expected)
		}
	}
	for _, forbidden := range []string{
		"lib/models/entry.dart",
		"lib/models/summary.dart",
		"lib/views/home_page.dart",
		"lib/views/entry_form_page.dart",
		"lib/views/entry_list_page.dart",
		"lib/controllers/home_controller.dart",
		"lib/controllers/entry_form_controller.dart",
		"lib/controllers/entry_list_controller.dart",
		"lib/repositories/entry_repository.dart",
	} {
		if strings.Contains(checks[1].Commands[0], forbidden) {
			t.Fatalf("check-counter-demo-removed command = %q, want avoid fixed path %q", checks[1].Commands[0], forbidden)
		}
	}
	if !strings.Contains(checks[2].Commands[0], "lib/views/*.dart lib/controllers/*.dart") {
		t.Fatalf("check-entry-form-wiring command = %q, want generic view/controller scope", checks[2].Commands[0])
	}
	for _, forbidden := range []string{
		"lib/views/entry_form_page.dart",
		"lib/controllers/entry_form_controller.dart",
	} {
		if strings.Contains(checks[2].Commands[0], forbidden) {
			t.Fatalf("check-entry-form-wiring command = %q, want avoid fixed path %q", checks[2].Commands[0], forbidden)
		}
	}
	if checks[3].CheckID != "check-local-persistence-wiring" {
		t.Fatalf("fourth check = %q, want check-local-persistence-wiring", checks[3].CheckID)
	}
	if !strings.Contains(checks[3].Commands[0], "pubspec.yaml lib/repositories/*.dart lib/main.dart") {
		t.Fatalf("check-local-persistence-wiring command = %q, want generic repository scope", checks[3].Commands[0])
	}
	for _, expected := range []string{
		"hive_flutter|Hive",
		"shared_preferences|SharedPreferences",
	} {
		if !strings.Contains(checks[3].Commands[0], expected) {
			t.Fatalf("check-local-persistence-wiring command = %q, want contain %q", checks[3].Commands[0], expected)
		}
	}
	for _, forbidden := range []string{
		"lib/repositories/entry_repository.dart",
	} {
		if strings.Contains(checks[3].Commands[0], forbidden) {
			t.Fatalf("check-local-persistence-wiring command = %q, want avoid fixed path %q", checks[3].Commands[0], forbidden)
		}
	}
	if len(profile.AllowedPaths) != 6 || profile.AllowedPaths[0] != "lib/**" || profile.AllowedPaths[4] != "android/app/build.gradle.kts" || profile.AllowedPaths[5] != "android/app/src/main/res/values/strings.xml" {
		t.Fatalf("AllowedPaths = %v, want lib-first plus Android override points", profile.AllowedPaths)
	}
	if len(profile.ProtectedPaths) != 8 || profile.ProtectedPaths[0] != "android/app/src/main/AndroidManifest.xml" {
		t.Fatalf("ProtectedPaths = %v, want narrowed protected paths with Android manifest first", profile.ProtectedPaths)
	}
	if profile.CommandProfile.ProfileName != "flutter-builder-p0" {
		t.Fatalf("CommandProfile.ProfileName = %q, want flutter-builder-p0", profile.CommandProfile.ProfileName)
	}
}

func TestNewFlutterAndroidProfileAddsDeviceChecksWhenEnabled(t *testing.T) {
	t.Setenv("APPFACTORY_DEVICE_VERIFICATION_ENABLED", "1")

	profile := NewFlutterAndroidProfile()
	if len(profile.CommandChecks) != 8 {
		t.Fatalf("CommandChecks len = %d, want 8 when device verification is enabled", len(profile.CommandChecks))
	}
	checks := profile.AcceptanceChecks()
	if len(checks) != 11 {
		t.Fatalf("AcceptanceChecks len = %d, want 11 when device verification is enabled", len(checks))
	}
	foundADBReady := false
	foundInstallAPK := false
	foundLaunchSmoke := false
	foundUISemantic := false
	for _, check := range checks {
		switch check.CheckID {
		case "check-adb-device-ready":
			foundADBReady = true
			if check.Stage != StageDevice {
				t.Fatalf("check-adb-device-ready stage = %q, want %q", check.Stage, StageDevice)
			}
		case "check-install-debug-apk":
			foundInstallAPK = true
			if check.Stage != StageDevice {
				t.Fatalf("check-install-debug-apk stage = %q, want %q", check.Stage, StageDevice)
			}
		case "check-launch-app-and-capture-logcat":
			foundLaunchSmoke = true
			if check.Stage != StageSmoke {
				t.Fatalf("check-launch-app-and-capture-logcat stage = %q, want %q", check.Stage, StageSmoke)
			}
		case "check-device-ui-semantic":
			foundUISemantic = true
			if check.Stage != StageSmoke {
				t.Fatalf("check-device-ui-semantic stage = %q, want %q", check.Stage, StageSmoke)
			}
		}
	}
	if !foundADBReady || !foundInstallAPK || !foundLaunchSmoke || !foundUISemantic {
		t.Fatalf("device checks missing: adb_ready=%v install_apk=%v launch_smoke=%v ui_semantic=%v", foundADBReady, foundInstallAPK, foundLaunchSmoke, foundUISemantic)
	}
	if len(profile.CommandProfile.AllowedStages) != 5 {
		t.Fatalf("AllowedStages len = %d, want 5 when device verification is enabled", len(profile.CommandProfile.AllowedStages))
	}
	foundDeviceStage := false
	foundSmokeStage := false
	for _, stage := range profile.CommandProfile.AllowedStages {
		if stage == "device" {
			foundDeviceStage = true
		}
		if stage == "smoke" {
			foundSmokeStage = true
		}
	}
	if !foundDeviceStage || !foundSmokeStage {
		t.Fatalf("AllowedStages = %v, want device and smoke", profile.CommandProfile.AllowedStages)
	}
}

func TestFlutterDeviceCommandsEmitFailureSignatureMarkers(t *testing.T) {
	t.Setenv("APPFACTORY_DEVICE_VERIFICATION_ENABLED", "1")

	profile := NewFlutterAndroidProfile()
	checks := profile.AcceptanceChecks()
	markers := map[string]string{
		"check-adb-device-ready":              "__oneappfactory_failure_signature__:device_check_failed:adb_device_unavailable",
		"check-install-debug-apk":             "__oneappfactory_failure_signature__:device_check_failed:apk_install_failed",
		"check-launch-app-and-capture-logcat": "__oneappfactory_failure_signature__:device_check_failed:app_runtime_crash",
		"check-device-ui-semantic":            "__oneappfactory_failure_signature__:device_check_failed:ui_semantic_mismatch",
	}
	for _, check := range checks {
		want, ok := markers[check.CheckID]
		if !ok {
			continue
		}
		if len(check.Commands) == 0 || !strings.Contains(check.Commands[0], want) {
			t.Fatalf("check %s commands = %v, want marker %q", check.CheckID, check.Commands, want)
		}
	}
}

func TestFlutterDeviceUISemanticCommandCapturesHierarchyAndSupportsExpectedText(t *testing.T) {
	command := adbCaptureAndVerifyUISemanticsCommand()
	for _, expected := range []string{
		"uiautomator dump",
		"device-ui.xml",
		"APPFACTORY_DEVICE_REQUIRED_UI_TEXT",
		"Open Lite Seed|open_lite_seed",
		"__oneappfactory_failure_signature__:device_check_failed:ui_dump_failed",
		"__oneappfactory_failure_signature__:device_check_failed:ui_expected_text_missing",
	} {
		if !strings.Contains(command, expected) {
			t.Fatalf("command = %q, want contain %q", command, expected)
		}
	}
}

func TestBuildRunnerScriptAllowsDeviceShellSnippets(t *testing.T) {
	t.Setenv("APPFACTORY_DEVICE_VERIFICATION_ENABLED", "1")

	profile := NewFlutterAndroidProfile()
	commandProfile, err := json.Marshal(profile.CommandProfile)
	if err != nil {
		t.Fatalf("Marshal(command profile) error = %v", err)
	}
	contextFiles, err := json.Marshal(ContextFiles{})
	if err != nil {
		t.Fatalf("Marshal(context files) error = %v", err)
	}
	input := BuildInput{
		AcceptanceChecks: profile.AcceptanceChecks(),
		CommandProfile:   commandProfile,
		ContextFiles:     contextFiles,
	}
	if _, err := buildRunnerScript(input, "/tmp/workspace", "/tmp/artifacts", "/tmp/log.txt"); err != nil {
		t.Fatalf("buildRunnerScript() error = %v, want device shell snippets accepted", err)
	}
}

func TestExecutableTokensSkipsShellBuiltinsButKeepsDeniedCommands(t *testing.T) {
	command := "echo ready && rm -rf tmp/output && adb devices"
	tokens := executableTokens(command)
	if len(tokens) != 2 {
		t.Fatalf("executableTokens len = %d, want 2 for %q", len(tokens), command)
	}
	if tokens[0] != "rm" || tokens[1] != "adb" {
		t.Fatalf("executableTokens = %v, want [rm adb]", tokens)
	}
}
