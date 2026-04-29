package emitter

import appprepare "github.com/sipeed/picoclaw/pkg/appfactory/prepare"

// AndroidBuildConfigEmitResult 表示 android/app/build.gradle.kts 的确定性输出。
type AndroidBuildConfigEmitResult struct {
	FilePath string
	Content  string
}

// EmitAndroidBuildConfig 为 flutter-open-lite 输出稳定的 Android build.gradle.kts 基线。
func EmitAndroidBuildConfig(dm appprepare.DomainModel) (AndroidBuildConfigEmitResult, bool) {
	_ = dm
	return AndroidBuildConfigEmitResult{
		FilePath: "android/app/build.gradle.kts",
		Content:  renderAndroidBuildGradleKTS(),
	}, true
}

func renderAndroidBuildGradleKTS() string {
	return `val defaultOpenLiteApplicationId = "com.picoclaw.appfactory.flutter_open_lite"

plugins {
	id("com.android.application")
	id("kotlin-android")
	// The Flutter Gradle Plugin must be applied after the Android and Kotlin Gradle plugins.
	id("dev.flutter.flutter-gradle-plugin")
}

android {
	namespace = defaultOpenLiteApplicationId
	compileSdk = flutter.compileSdkVersion
	ndkVersion = "27.0.12077973"

	compileOptions {
		sourceCompatibility = JavaVersion.VERSION_11
		targetCompatibility = JavaVersion.VERSION_11
	}

	kotlinOptions {
		jvmTarget = JavaVersion.VERSION_11.toString()
	}

	defaultConfig {
		applicationId = defaultOpenLiteApplicationId
		minSdk = flutter.minSdkVersion
		targetSdk = flutter.targetSdkVersion
		versionCode = flutter.versionCode
		versionName = flutter.versionName
	}

	buildTypes {
			release {
			// TODO: Add your own signing config for the release build.
			// Signing with the debug keys for now, so flutter run --release works.
			signingConfig = signingConfigs.getByName("debug")
		}
	}
}

flutter {
	source = "../.."
}
`
}
