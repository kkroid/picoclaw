package prepare

import "testing"

func TestLoadGenericDomainProfileCatalogKeepsKnownProfiles(t *testing.T) {
	catalog, err := loadGenericDomainProfileCatalog()
	if err != nil {
		t.Fatalf("load generic domain profile catalog: %v", err)
	}
	if catalog.SchemaVersion != "v0.1.0" {
		t.Fatalf("unexpected schema version: %s", catalog.SchemaVersion)
	}
	if len(catalog.Profiles) != 3 {
		t.Fatalf("expected 3 profiles, got %d", len(catalog.Profiles))
	}
	profileIDs := map[string]bool{}
	for _, profile := range catalog.Profiles {
		profileIDs[profile.ProfileID] = true
	}
	for _, expected := range []string{"weight-tracker", "todo-lite", "habit-checkin"} {
		if !profileIDs[expected] {
			t.Fatalf("missing generic domain profile %s", expected)
		}
	}
}

func TestDetectGenericDomainSignalsUsesConfiguredProfiles(t *testing.T) {
	weight := detectGenericDomainSignals("做一个体重记录 Android App，首页需要看趋势")
	if weight.AppTitle != "体重记录 App" {
		t.Fatalf("unexpected weight app title: %s", weight.AppTitle)
	}
	if weight.DomainCheckPattern != "体重|weight|recorded_at|latest_weight|trend" {
		t.Fatalf("unexpected weight domain check pattern: %s", weight.DomainCheckPattern)
	}

	todo := detectGenericDomainSignals("Build a todo app with inbox and done status")
	if todo.AppTitle != "待办事项 App" {
		t.Fatalf("unexpected todo app title: %s", todo.AppTitle)
	}
	if !todo.HasFilter {
		t.Fatalf("expected todo profile to keep filter support")
	}

	habit := detectGenericDomainSignals("我想做一个 habit 打卡工具")
	if habit.AppTitle != "习惯打卡 App" {
		t.Fatalf("unexpected habit app title: %s", habit.AppTitle)
	}
	if habit.Entity.EntityID != "entity-habit-record" {
		t.Fatalf("unexpected habit entity id: %s", habit.Entity.EntityID)
	}
}

func TestDetectGenericDomainSignalsFallsBackToDefaultProfile(t *testing.T) {
	signals := detectGenericDomainSignals("做一个离线记录工具")
	if signals.AppTitle != "通用工具 App" {
		t.Fatalf("unexpected default app title: %s", signals.AppTitle)
	}
	if signals.DomainCheckPattern != "record|dashboard_summary|detail|filter" {
		t.Fatalf("unexpected default domain check pattern: %s", signals.DomainCheckPattern)
	}
}
