package runs

import "testing"

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
	if len(profile.AllowedPaths) == 0 || profile.AllowedPaths[0] != "lib/**" {
		t.Fatalf("AllowedPaths = %v, want lib/** first", profile.AllowedPaths)
	}
	if profile.CommandProfile.ProfileName != "flutter-builder-p0" {
		t.Fatalf("CommandProfile.ProfileName = %q, want flutter-builder-p0", profile.CommandProfile.ProfileName)
	}
}
