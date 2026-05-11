package emitter

import (
	"strings"
	"testing"

	appprepare "github.com/sipeed/oneappfactory/pkg/appfactory/prepare"
)

func TestEmitAndroidBuildConfigReturnsTemplateBaseline(t *testing.T) {
	result, ok := EmitAndroidBuildConfig(appprepare.DomainModel{})
	if !ok {
		t.Fatal("EmitAndroidBuildConfig returned false")
	}
	if result.FilePath != "android/app/build.gradle.kts" {
		t.Fatalf("FilePath = %q, want android/app/build.gradle.kts", result.FilePath)
	}
	for _, marker := range []string{
		`val defaultOpenLiteApplicationId = "com.appfactory.flutter_open_lite"`,
		`namespace = defaultOpenLiteApplicationId`,
		`applicationId = defaultOpenLiteApplicationId`,
		`id("dev.flutter.flutter-gradle-plugin")`,
		`source = "../.."`,
	} {
		if !strings.Contains(result.Content, marker) {
			t.Fatalf("Content missing %q:\n%s", marker, result.Content)
		}
	}
}
