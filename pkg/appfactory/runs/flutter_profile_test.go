package runs

import (
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
	if !strings.Contains(checks[1].Commands[0], "grep -q . lib/views/home_page.dart") {
		t.Fatalf("check-counter-demo-removed command = %q, want explicit file existence guard", checks[1].Commands[0])
	}
	if len(profile.AllowedPaths) == 0 || profile.AllowedPaths[0] != "lib/**" {
		t.Fatalf("AllowedPaths = %v, want lib/** first", profile.AllowedPaths)
	}
	if profile.CommandProfile.ProfileName != "flutter-builder-p0" {
		t.Fatalf("CommandProfile.ProfileName = %q, want flutter-builder-p0", profile.CommandProfile.ProfileName)
	}
}

func TestNewFlutterAndroidProfileAddsDeviceChecksWhenEnabled(t *testing.T) {
	t.Setenv("APPFACTORY_DEVICE_VERIFICATION_ENABLED", "1")

	profile := NewFlutterAndroidProfile()
	if len(profile.CommandChecks) != 7 {
		t.Fatalf("CommandChecks len = %d, want 7 when device verification is enabled", len(profile.CommandChecks))
	}
	checks := profile.AcceptanceChecks()
	if len(checks) != 10 {
		t.Fatalf("AcceptanceChecks len = %d, want 10 when device verification is enabled", len(checks))
	}
	foundADBReady := false
	foundInstallAPK := false
	foundLaunchSmoke := false
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
		}
	}
	if !foundADBReady || !foundInstallAPK || !foundLaunchSmoke {
		t.Fatalf("device checks missing: adb_ready=%v install_apk=%v launch_smoke=%v", foundADBReady, foundInstallAPK, foundLaunchSmoke)
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
		"check-adb-device-ready":              "__picoclaw_failure_signature__:device_check_failed:adb_device_unavailable",
		"check-install-debug-apk":             "__picoclaw_failure_signature__:device_check_failed:apk_install_failed",
		"check-launch-app-and-capture-logcat": "__picoclaw_failure_signature__:device_check_failed:app_runtime_crash",
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
