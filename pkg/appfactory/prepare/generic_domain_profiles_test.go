package prepare

import (
	"reflect"
	"testing"
)

func TestLoadGenericDomainProfileCatalogKeepsKnownProfiles(t *testing.T) {
	catalog, err := loadGenericDomainProfileCatalog()
	if err != nil {
		t.Fatalf("load generic domain profile catalog: %v", err)
	}
	if catalog.SchemaVersion != "v0.1.0" {
		t.Fatalf("unexpected schema version: %s", catalog.SchemaVersion)
	}
	if len(catalog.Profiles) != 9 {
		t.Fatalf("expected 9 profiles, got %d", len(catalog.Profiles))
	}
	profileIDs := map[string]bool{}
	for _, profile := range catalog.Profiles {
		profileIDs[profile.ProfileID] = true
	}
	for _, expected := range []string{"weight-tracker", "todo-lite", "habit-checkin", "coursework-tracker", "pet-vaccine-record", "plant-watering-record", "movie-watchlist", "workout-log", "packing-list"} {
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

	movie := detectGenericDomainSignals("做一个观影清单，记录电影评分")
	if movie.AppTitle != "观影清单 App" {
		t.Fatalf("unexpected movie app title: %s", movie.AppTitle)
	}
	if movie.Entity.EntityID != "entity-movie-record" {
		t.Fatalf("unexpected movie entity id: %s", movie.Entity.EntityID)
	}

	packing := detectGenericDomainSignals("做一个行李打包清单，记录物品是否已打包")
	if packing.AppTitle != "行李打包清单 App" {
		t.Fatalf("unexpected packing app title: %s", packing.AppTitle)
	}
	if packing.Entity.EntityID != "entity-packing-item" {
		t.Fatalf("unexpected packing entity id: %s", packing.Entity.EntityID)
	}
}

func TestCompileGenericL1FieldProfilesExposeCoreFields(t *testing.T) {
	testCases := []struct {
		name              string
		requirement       string
		wantTitle         string
		wantEntityID      string
		wantFields        []genericFieldExpectation
		wantSummaryFields []string
	}{
		{
			name:         "coursework-tracker",
			requirement:  "做一个课程作业追踪 App，记录课程名、作业标题、截止日期、状态和备注。",
			wantTitle:    "课程作业追踪 App",
			wantEntityID: "entity-course-assignment",
			wantFields: []genericFieldExpectation{
				{name: "assignment_id", role: "identifier"},
				{name: "course_name", role: "secondary_text"},
				{name: "assignment_title", role: "primary_text"},
				{name: "due_date", role: "due_date"},
				{name: "status", role: "status"},
				{name: "note", role: "note"},
			},
			wantSummaryFields: []string{"due_soon_count", "overdue_count", "done_count"},
		},
		{
			name:         "pet-vaccine-record",
			requirement:  "做一个宠物疫苗记录 App，记录宠物名、疫苗名称、接种日期、下次提醒日期和备注。",
			wantTitle:    "宠物疫苗记录 App",
			wantEntityID: "entity-pet-vaccine-record",
			wantFields: []genericFieldExpectation{
				{name: "vaccine_record_id", role: "identifier"},
				{name: "pet_name", role: "primary_text"},
				{name: "vaccine_name", role: "secondary_text"},
				{name: "vaccinated_at", role: "date"},
				{name: "next_due_at", role: "due_date"},
				{name: "note", role: "note"},
			},
			wantSummaryFields: []string{"upcoming_count", "overdue_count", "total_count"},
		},
		{
			name:         "plant-watering-record",
			requirement:  "做一个植物浇水记录 App，记录植物名、位置、上次浇水时间、浇水状态和备注。",
			wantTitle:    "植物浇水记录 App",
			wantEntityID: "entity-plant-watering-record",
			wantFields: []genericFieldExpectation{
				{name: "watering_record_id", role: "identifier"},
				{name: "plant_name", role: "primary_text"},
				{name: "location", role: "secondary_text"},
				{name: "last_watered_at", role: "date"},
				{name: "water_status", role: "status"},
				{name: "note", role: "note"},
			},
			wantSummaryFields: []string{"needs_water_count", "watered_count", "total_count"},
		},
		{
			name:         "movie-watchlist",
			requirement:  "做一个观影清单 App，记录电影名、类型、观看状态、评分和短评。",
			wantTitle:    "观影清单 App",
			wantEntityID: "entity-movie-record",
			wantFields: []genericFieldExpectation{
				{name: "movie_record_id", role: "identifier"},
				{name: "movie_title", role: "primary_text"},
				{name: "genre", role: "secondary_text"},
				{name: "watch_status", role: "status"},
				{name: "rating", role: "rating"},
				{name: "review", role: "note"},
			},
			wantSummaryFields: []string{"watched_count", "planned_count", "average_rating"},
		},
		{
			name:         "workout-log",
			requirement:  "做一个运动训练日志 App，记录训练名称、训练日期、训练时长、完成状态和备注。",
			wantTitle:    "运动训练日志 App",
			wantEntityID: "entity-workout-session",
			wantFields: []genericFieldExpectation{
				{name: "workout_record_id", role: "identifier"},
				{name: "workout_name", role: "primary_text"},
				{name: "workout_date", role: "date"},
				{name: "duration_minutes", role: "duration"},
				{name: "completion_status", role: "status"},
				{name: "note", role: "note"},
			},
			wantSummaryFields: []string{"total_sessions", "completed_count", "total_duration_minutes"},
		},
		{
			name:         "packing-list",
			requirement:  "做一个行李打包清单 App，记录物品名称、分类、是否已打包和备注。",
			wantTitle:    "行李打包清单 App",
			wantEntityID: "entity-packing-item",
			wantFields: []genericFieldExpectation{
				{name: "packing_item_id", role: "identifier"},
				{name: "item_name", role: "primary_text"},
				{name: "category", role: "secondary_text"},
				{name: "is_packed", role: "flag"},
				{name: "note", role: "note"},
			},
			wantSummaryFields: []string{"total_count", "packed_count"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			spec := compileGenericSpec(Request{}, testCase.requirement)
			if spec.Title != testCase.wantTitle {
				t.Fatalf("Title = %q, want %q", spec.Title, testCase.wantTitle)
			}

			domainModel := buildDomainModel(spec)
			if len(domainModel.Entities) != 2 {
				t.Fatalf("Entities len = %d, want 2", len(domainModel.Entities))
			}
			if domainModel.Entities[0].EntityID != testCase.wantEntityID {
				t.Fatalf("primary entity id = %q, want %q", domainModel.Entities[0].EntityID, testCase.wantEntityID)
			}
			if !reflect.DeepEqual(fieldNames(domainModel.Entities[0].Fields), expectedGenericFieldNames(testCase.wantFields)) {
				t.Fatalf("primary fields = %v, want %v", fieldNames(domainModel.Entities[0].Fields), expectedGenericFieldNames(testCase.wantFields))
			}
			if !reflect.DeepEqual(fieldNames(domainModel.Entities[1].Fields), testCase.wantSummaryFields) {
				t.Fatalf("summary fields = %v, want %v", fieldNames(domainModel.Entities[1].Fields), testCase.wantSummaryFields)
			}

			for _, want := range testCase.wantFields {
				field := findDataField(t, domainModel.Entities[0].Fields, want.name)
				if field.Role != want.role {
					t.Fatalf("field %q role = %q, want %q", want.name, field.Role, want.role)
				}
			}

			coveredRefs := semanticFieldRefSet(domainModel.SemanticAcceptanceRules)
			for _, want := range testCase.wantFields {
				if !coveredRefs[want.name] {
					t.Fatalf("semantic acceptance rules missing field ref %q: %+v", want.name, domainModel.SemanticAcceptanceRules)
				}
			}
		})
	}
}

type genericFieldExpectation struct {
	name string
	role string
}

func expectedGenericFieldNames(fields []genericFieldExpectation) []string {
	names := make([]string, 0, len(fields))
	for _, field := range fields {
		names = append(names, field.name)
	}
	return names
}

func findDataField(t *testing.T, fields []DataField, name string) DataField {
	t.Helper()
	for _, field := range fields {
		if field.Name == name {
			return field
		}
	}
	t.Fatalf("field %q not found in %+v", name, fields)
	return DataField{}
}

func semanticFieldRefSet(rules []SemanticAcceptanceRule) map[string]bool {
	refs := map[string]bool{}
	for _, rule := range rules {
		for _, fieldRef := range rule.FieldRefs {
			refs[fieldRef] = true
		}
	}
	return refs
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

func TestCompileGenericDomainModelCarriesComplexityContracts(t *testing.T) {
	spec := compileGenericSpec(Request{}, "做一个课程作业追踪 App，记录课程名、作业标题、截止日期、状态和备注。")
	domainModel := buildDomainModel(spec)

	if domainModel.ComplexityLevel != "L2-behavior-contract" {
		t.Fatalf("ComplexityLevel = %q, want L2-behavior-contract", domainModel.ComplexityLevel)
	}
	for _, capability := range []string{"due-date", "status-field", "local-storage", "status-filter"} {
		if !containsString(domainModel.CapabilityFlags, capability) {
			t.Fatalf("CapabilityFlags = %v, want %q", domainModel.CapabilityFlags, capability)
		}
	}
	if domainModel.PersistenceContract == nil || domainModel.PersistenceContract.Mode != "local-hive" {
		t.Fatalf("PersistenceContract = %+v, want local-hive", domainModel.PersistenceContract)
	}
	if !containsString(domainModel.PersistenceContract.EntityRefs, "entity-course-assignment") {
		t.Fatalf("PersistenceContract.EntityRefs = %v, want primary entity", domainModel.PersistenceContract.EntityRefs)
	}

	filterRule := findBehaviorRule(t, domainModel.BehaviorRules, "behavior-filter-records")
	if !containsString(filterRule.FieldRefs, "status") || !containsString(filterRule.CapabilityRefs, "status-filter") {
		t.Fatalf("filter behavior rule = %+v, want status field and status-filter capability", filterRule)
	}
	persistRule := findBehaviorRule(t, domainModel.BehaviorRules, "behavior-persist-records")
	if !containsString(persistRule.CapabilityRefs, "local-storage") || !containsString(persistRule.AcceptanceRefs, "ac-persistence") {
		t.Fatalf("persist behavior rule = %+v, want persistence refs", persistRule)
	}

	allocation := buildTaskAllocation(spec.TaskBundle, spec)
	repositoryUnit := findAllocationUnit(t, allocation.Units, "task-create-repository")
	if !containsString(repositoryUnit.CapabilityFlags, "local-storage") || !containsString(repositoryUnit.BehaviorRefs, "behavior-persist-records") {
		t.Fatalf("repository allocation = %+v, want persistence capability and behavior refs", repositoryUnit)
	}

	acceptancePlan := buildAcceptancePlan(spec)
	persistenceCheck := mustFindAcceptancePlanItem(t, acceptancePlan.SemanticChecks, "ac-persistence")
	if !containsString(persistenceCheck.CapabilityRefs, "local-storage") || !containsString(persistenceCheck.BehaviorRefs, "behavior-persist-records") {
		t.Fatalf("persistence acceptance check = %+v, want persistence contract refs", persistenceCheck)
	}

	executionContract := buildExecutionContract(spec, PRD{Title: spec.Title, UserFlows: spec.UserFlows, AcceptanceCriteria: spec.AcceptanceCriteria}, spec.TaskBundle)
	if executionContract.DomainModel.ComplexityLevel != domainModel.ComplexityLevel {
		t.Fatalf("ExecutionContract.DomainModel.ComplexityLevel = %q, want %q", executionContract.DomainModel.ComplexityLevel, domainModel.ComplexityLevel)
	}
}

func findBehaviorRule(t *testing.T, rules []DomainBehaviorRule, ruleID string) DomainBehaviorRule {
	t.Helper()
	for _, rule := range rules {
		if rule.RuleID == ruleID {
			return rule
		}
	}
	t.Fatalf("behavior rule %q not found in %+v", ruleID, rules)
	return DomainBehaviorRule{}
}
