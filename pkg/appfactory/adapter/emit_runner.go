package adapter

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sipeed/oneappfactory/pkg/appfactory/emitter"
	appprepare "github.com/sipeed/oneappfactory/pkg/appfactory/prepare"
	appruns "github.com/sipeed/oneappfactory/pkg/appfactory/runs"
)

// emitRunnerResult 是 tryDeterministicEmit 的返回结果。
type emitRunnerResult struct {
	Patch       *appruns.WorkspacePatch
	ApplyResult appruns.WorkspacePatchApplyResult
	Handled     bool
}

// tryDeterministicEmit 尝试用确定性 Emitter 生成 workspace patch。
// 当选中的 task route_hint 为 deterministic 时调用。
// 返回 Handled=true 表示成功完成（调用方应跳过 LLM），
// 返回 Handled=false 表示 emitter 不适用或失败（调用方应 fallback 到 LLM）。
func tryDeterministicEmit(run runRecord, roundInput appruns.RoundInput, patchID string) (emitRunnerResult, error) {
	selectedTask := preferredBuilderRuntimeTask(run.TaskBundle, run.RoundState)
	if appruns.NormalizeTaskRouteHint(string(selectedTask.RouteHint)) != appruns.TaskRouteHintDeterministic {
		return emitRunnerResult{}, nil
	}

	dm, err := loadPrepareDomainModel(run.WorkspacePath)
	if err != nil {
		return emitRunnerResult{}, fmt.Errorf("load domain model for deterministic emit: %w", err)
	}

	targetSet := normalizeTargetPaths(selectedTask.TargetPaths)
	slotMap, err := loadPrepareTemplateSlotMap(run.WorkspacePath)
	if err != nil {
		return emitRunnerResult{}, fmt.Errorf("load template slot map for deterministic emit: %w", err)
	}
	matchedSlots := resolveDeterministicTemplateSlots(selectedTask, slotMap, targetSet)
	relationRichProfile := deterministicRelationRichModelProfile(dm)

	var planningContext appprepare.PlanningContext
	if deterministicSlotsRequirePlanningContext(matchedSlots) {
		planningContext, err = loadPreparePlanningContext(run.WorkspacePath)
		if err != nil {
			return emitRunnerResult{}, fmt.Errorf("load planning context for deterministic emit: %w", err)
		}
	}
	var ops []appruns.WorkspacePatchOperation
	ops = append(ops, deterministicProtocolClientOperations(run.WorkspacePath, dm)...)
	ops = append(ops, deterministicRelationModelOperations(dm, selectedTask.TargetPaths)...)
	// generic model 回退：domain-model.json 读取实体字段，确定性生成 record.dart / dashboard_summary.dart。
	if len(ops) == 0 {
		ops = append(ops, deterministicGenericModelOperations(dm, selectedTask.TargetPaths)...)
	}
	ops = append(ops, deterministicTemplateSlotOperations(matchedSlots, deterministicTemplateSlotContext{
		run:                 run,
		dm:                  dm,
		planningContext:     planningContext,
		relationRichProfile: relationRichProfile,
		targetSet:           targetSet,
	})...)

	// generic mutation 是 repository-driven，不需要 form controller。
	// 但 fallback normalize 可能在 form page 中添加 controller import。
	// 无条件写入空壳 stub 避免 Dart 编译失败。
	if strings.TrimSpace(dm.TemplateID) == "flutter-open-lite" && containsMutationSlot(matchedSlots) {
		ops = append(ops, appruns.WorkspacePatchOperation{
			Type:    "write_file",
			Path:    "lib/controllers/record_form_controller.dart",
			Content: "// Generic mutation page is repository-driven; no controller needed.\n",
		})
	}

	if len(ops) == 0 {
		return emitRunnerResult{}, nil
	}

	patch := &appruns.WorkspacePatch{
		PatchID:    patchID,
		Status:     "emitted",
		Operations: ops,
	}

	applyResult, applyErr := appruns.ApplyWorkspacePatch(run.WorkspacePath, run.AllowedPaths, run.ProtectedPaths, *patch)
	if applyErr != nil {
		return emitRunnerResult{}, fmt.Errorf("apply deterministic emit patch: %w", applyErr)
	}
	if applyResult.Status != "" {
		patch.Status = applyResult.Status
	}
	patch.ModifiedFiles = modifiedFilePaths(applyResult.ModifiedFiles)

	return emitRunnerResult{
		Patch:       patch,
		ApplyResult: applyResult,
		Handled:     true,
	}, nil
}

// loadPrepareDomainModel 从 job 根目录的 prepare/domain-model.json 加载 DomainModel。
func loadPrepareDomainModel(workspacePath string) (appprepare.DomainModel, error) {
	jobRoot := filepath.Dir(workspacePath)
	dmPath := filepath.Join(jobRoot, "prepare", "domain-model.json")
	content, err := os.ReadFile(dmPath)
	if err != nil {
		return appprepare.DomainModel{}, fmt.Errorf("read %s: %w", dmPath, err)
	}
	var dm appprepare.DomainModel
	if err := json.Unmarshal(content, &dm); err != nil {
		return appprepare.DomainModel{}, fmt.Errorf("parse domain-model.json: %w", err)
	}
	return dm, nil
}

func loadPreparePlanningContext(workspacePath string) (appprepare.PlanningContext, error) {
	jobRoot := filepath.Dir(workspacePath)
	planningContextPath := filepath.Join(jobRoot, "prepare", "planning-context.json")
	content, err := os.ReadFile(planningContextPath)
	if err != nil {
		return appprepare.PlanningContext{}, fmt.Errorf("read %s: %w", planningContextPath, err)
	}
	var planningContext appprepare.PlanningContext
	if err := json.Unmarshal(content, &planningContext); err != nil {
		return appprepare.PlanningContext{}, fmt.Errorf("parse planning-context.json: %w", err)
	}
	return planningContext, nil
}

func loadPrepareTemplateSlotMap(workspacePath string) (appprepare.TemplateSlotMap, error) {
	jobRoot := filepath.Dir(workspacePath)
	templateSlotMapPath := filepath.Join(jobRoot, "prepare", "template-slot-map.json")
	content, err := os.ReadFile(templateSlotMapPath)
	if err != nil {
		return appprepare.TemplateSlotMap{}, fmt.Errorf("read %s: %w", templateSlotMapPath, err)
	}
	var slotMap appprepare.TemplateSlotMap
	if err := json.Unmarshal(content, &slotMap); err != nil {
		return appprepare.TemplateSlotMap{}, fmt.Errorf("parse template-slot-map.json: %w", err)
	}
	return slotMap, nil
}

type deterministicTemplateSlotContext struct {
	run                 runRecord
	dm                  appprepare.DomainModel
	planningContext     appprepare.PlanningContext
	relationRichProfile builderRuntimeRelationRichModelProfile
	targetSet           map[string]struct{}
}

func deterministicRelationModelOperations(dm appprepare.DomainModel, targetPaths []string) []appruns.WorkspacePatchOperation {
	result, emitted := emitter.EmitRelationModels(dm)
	if !emitted {
		return nil
	}
	ops := make([]appruns.WorkspacePatchOperation, 0, len(targetPaths))
	for _, targetPath := range targetPaths {
		normalizedTargetPath := filepath.ToSlash(strings.TrimSpace(targetPath))
		if content, ok := result.Files[normalizedTargetPath]; ok {
			ops = append(ops, appruns.WorkspacePatchOperation{
				Type:    "write_file",
				Path:    normalizedTargetPath,
				Content: content,
			})
		}
	}
	return ops
}

// deterministicGenericModelOperations 从 domain-model.json 当 relation-rich emitter 不支持时确定性生成模型文件。
func deterministicGenericModelOperations(dm appprepare.DomainModel, targetPaths []string) []appruns.WorkspacePatchOperation {
	if len(dm.Entities) == 0 {
		return nil
	}
	var primaryEnt *appprepare.DataEntity
	for i := range dm.Entities {
		e := &dm.Entities[i]
		id := strings.TrimSpace(e.EntityID)
		if id == "entity-record" || id == "record" || (!strings.Contains(id, "dashboard") && !strings.Contains(id, "summary")) {
			primaryEnt = e
			break
		}
	}
	ops := make([]appruns.WorkspacePatchOperation, 0, 2)
	targetSet := make(map[string]bool, len(targetPaths))
	for _, p := range targetPaths {
		targetSet[filepath.ToSlash(strings.TrimSpace(p))] = true
	}
	if primaryEnt != nil && targetSet["lib/models/record.dart"] {
		if c := emitGenericRecordModel(dm, primaryEnt); c != "" {
			ops = append(ops, appruns.WorkspacePatchOperation{Type: "write_file", Path: "lib/models/record.dart", Content: c})
		}
	}
	for i := range dm.Entities {
		e := &dm.Entities[i]
		if strings.Contains(strings.TrimSpace(e.EntityID), "dashboard") || strings.Contains(strings.TrimSpace(e.EntityID), "summary") {
			if targetSet["lib/models/dashboard_summary.dart"] {
				if c := emitGenericSummaryModel(e); c != "" {
					ops = append(ops, appruns.WorkspacePatchOperation{Type: "write_file", Path: "lib/models/dashboard_summary.dart", Content: c})
				}
			}
			break
		}
	}
	return ops
}

func emitGenericRecordModel(dm appprepare.DomainModel, ent *appprepare.DataEntity) string {
	className := emitEntityClassName(ent)
	if className == "" {
		className = "Record"
	}
	enumDefs := make([]string, 0)
	enumTypes := map[string]string{}
	for _, field := range ent.Fields {
		if m := emitEnumMembers(field.Type); len(m) > 0 {
			dartName := emitSnakeToCamel(strings.TrimSpace(field.Name))
			enumName := className + emitSnakeToPascal(dartName)
			if dartName == "status" {
				enumName = className + "Status"
			}
			enumTypes[dartName] = enumName
			members := make([]string, 0, len(m))
			for _, member := range m {
				members = append(members, emitSnakeToCamel(member))
			}
			enumDefs = append(enumDefs, "enum "+enumName+" {\n  "+strings.Join(members, ",\n  ")+
				",\n}\n")
		}
	}
	prefix := ""
	if len(enumDefs) > 0 {
		prefix = strings.Join(enumDefs, "\n") + "\n"
	}
	identifierField := emitGenericIdentifierDartField(ent)
	cp := make([]string, 0)
	ci := make([]string, 0)
	fd := make([]string, 0)
	cwp := make([]string, 0)
	cwr := make([]string, 0)
	for _, field := range ent.Fields {
		dartName := emitSnakeToCamel(strings.TrimSpace(field.Name))
		if dartName == "" {
			continue
		}
		dt := emitFieldToDartType(field.Type, dartName)
		if enumType := strings.TrimSpace(enumTypes[dartName]); enumType != "" {
			dt = enumType
		}
		switch {
		case dartName == identifierField:
			cp = append(cp, "    String? "+dartName+",")
			ci = append(ci, "        "+dartName+" = "+dartName+" ?? DateTime.now().microsecondsSinceEpoch.toString()")
		case dt == "DateTime":
			cp = append(cp, "    DateTime? "+dartName+",")
			ci = append(ci, "        "+dartName+" = "+dartName+" ?? DateTime.now()")
		case strings.HasPrefix(dt, className) && len(emitEnumMembers(field.Type)) > 0:
			members := emitEnumMembers(field.Type)
			defaultMember := "inbox"
			if len(members) > 0 {
				defaultMember = emitSnakeToCamel(members[0])
			}
			cp = append(cp, "    this."+dartName+" = "+dt+"."+defaultMember+",")
		case dt == "int":
			cp = append(cp, "    this."+dartName+" = 0,")
		case dt == "double":
			cp = append(cp, "    this."+dartName+" = 0,")
		case dt == "bool":
			cp = append(cp, "    this."+dartName+" = false,")
		default:
			cp = append(cp, "    this."+dartName+" = '',")
		}
		fd = append(fd, "  final "+dt+" "+dartName+";")
		cwp = append(cwp, "    "+dt+"? "+dartName+",")
		cwr = append(cwr, "      "+dartName+": "+dartName+" ?? this."+dartName+",")
	}
	constructorClose := "  });"
	if len(ci) > 0 {
		constructorClose = "  }) :\n" + strings.Join(ci, ",\n") + ";"
	}
	lines := []string{
		"class " + className + " {",
		"  " + className + "({",
		strings.Join(cp, "\n"),
		constructorClose,
		"",
		strings.Join(fd, "\n"),
	}
	if len(cwp) > 0 {
		lines = append(lines,
			"",
			"  "+className+" copyWith({",
			strings.Join(cwp, "\n"),
			"  }) {",
			"    return "+className+"(",
			strings.Join(cwr, "\n"),
			"    );",
			"  }",
		)
	}
	lines = append(lines, "}")
	return prefix + strings.Join(lines, "\n") + "\n"
}

func emitGenericSummaryModel(ent *appprepare.DataEntity) string {
	className := emitEntityClassName(ent)
	if className == "" {
		className = "DashboardSummary"
	}
	cp := make([]string, 0)
	fd := make([]string, 0)
	for _, field := range ent.Fields {
		dartName := emitSnakeToCamel(strings.TrimSpace(field.Name))
		if dartName == "" {
			continue
		}
		dt := emitFieldToDartType(field.Type, dartName)
		defaultValue := "''"
		switch dt {
		case "int":
			defaultValue = "0"
		case "double":
			defaultValue = "0"
		case "bool":
			defaultValue = "false"
		}
		cp = append(cp, "    this."+dartName+" = "+defaultValue+",")
		fd = append(fd, "  final "+dt+" "+dartName+";")
	}
	return strings.Join([]string{
		"class " + className + " {",
		"  const " + className + "({",
		strings.Join(cp, "\n"),
		"  });",
		"",
		strings.Join(fd, "\n"),
		"}",
	}, "\n") + "\n"
}

func emitSnakeToCamel(s string) string {
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		if i == 0 {
			parts[i] = strings.ToLower(p[:1]) + p[1:]
		} else if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "")
}

func emitSnakeToPascal(s string) string {
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "")
}

// emitEntityClassName 从 DomainModel entity 推导稳定的 ASCII Dart 类名。
// 若 entity name 无法转为有效 PascalCase（如中文），则回退到 entity_id。
func emitEntityClassName(ent *appprepare.DataEntity) string {
	name := strings.TrimSpace(ent.Name)
	nameLower := strings.ToLower(name)
	// 检查是否包含 ASCII 字母（至少一个 [a-z]）
	hasASCII := false
	for _, r := range nameLower {
		if r >= 'a' && r <= 'z' {
			hasASCII = true
			break
		}
	}
	if hasASCII {
		candidate := emitSnakeToPascal(name)
		if candidate != "" {
			return candidate
		}
	}
	// 从 entity_id 推导：entity-record → Record, entity-dashboard-summary → DashboardSummary
	id := strings.TrimSpace(ent.EntityID)
	if strings.HasPrefix(id, "entity-") {
		id = strings.TrimPrefix(id, "entity-")
	}
	id = strings.ReplaceAll(id, "-", "_")
	return emitSnakeToPascal(id)
}

func emitFieldToDartType(typeStr, dartName string) string {
	if strings.HasPrefix(typeStr, "enum[") {
		return emitSnakeToPascal(dartName) + "Status"
	}
	switch typeStr {
	case "int", "integer":
		return "int"
	case "number":
		return "double"
	case "bool", "boolean":
		return "bool"
	case "date", "datetime":
		return "DateTime"
	case "double", "float":
		return "double"
	default:
		return "String"
	}
}

func emitGenericIdentifierDartField(ent *appprepare.DataEntity) string {
	if ent == nil {
		return ""
	}
	for _, field := range ent.Fields {
		dartName := emitSnakeToCamel(strings.TrimSpace(field.Name))
		if dartName == "" {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(field.Role), "identifier") {
			return dartName
		}
	}
	for _, field := range ent.Fields {
		dartName := emitSnakeToCamel(strings.TrimSpace(field.Name))
		if dartName == "" {
			continue
		}
		lowerName := strings.ToLower(strings.TrimSpace(field.Name))
		if dartName == "recordId" || strings.HasSuffix(lowerName, "_id") || strings.HasSuffix(strings.ToLower(dartName), "id") {
			return dartName
		}
	}
	return ""
}

func emitEnumMembers(raw string) []string {
	start := strings.Index(raw, "[")
	end := strings.Index(raw, "]")
	if start < 0 || end <= start {
		return nil
	}
	parts := strings.Split(raw[start+1:end], ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// emitGenericRepositoryContent 从 domain-model.json 生成泛型 Hive 仓储。
func emitGenericRepositoryContent(workspacePath string, dm appprepare.DomainModel) string {
	if len(dm.Entities) == 0 {
		return ""
	}
	repoClass := "HiveRecordRepository"
	repoInterface := "RecordRepository"
	var primaryEnt *appprepare.DataEntity
	for i := range dm.Entities {
		e := &dm.Entities[i]
		id := strings.TrimSpace(e.EntityID)
		if !strings.Contains(id, "dashboard") && !strings.Contains(id, "summary") {
			primaryEnt = e
			break
		}
	}
	if primaryEnt == nil {
		return ""
	}
	recordClass := emitEntityClassName(primaryEnt)
	if recordClass == "" {
		recordClass = "Record"
	}
	countClass := "DashboardSummary"
	summaryImport := ""
	identifierField := emitGenericIdentifierDartField(primaryEnt)
	for _, e := range dm.Entities {
		if strings.Contains(strings.TrimSpace(e.EntityID), "dashboard") || strings.Contains(strings.TrimSpace(e.EntityID), "summary") {
			summaryImport = "import '../models/dashboard_summary.dart';"
			countClass = emitEntityClassName(&e)
			if countClass == "" {
				countClass = "DashboardSummary"
			}
			break
		}
	}
	if identifierField == "" {
		return "" // 需要可识别的 identifier 字段才能生成正确仓储
	}
	methods := []string{
		"  @override",
		"  Future<List<" + recordClass + ">> loadRecords() async {",
		"    _recordBox ??= await Hive.openBox<" + recordClass + ">('records');",
		"    return _recordBox!.values.toList();",
		"  }",
		"",
		"  @override",
		"  Future<void> addRecord(" + recordClass + " record) async {",
		"    _recordBox ??= await Hive.openBox<" + recordClass + ">('records');",
		"    await _recordBox!.put(record." + identifierField + ", record);",
		"  }",
		"",
		"  @override",
		"  Future<void> updateRecord(" + recordClass + " record) async {",
		"    _recordBox ??= await Hive.openBox<" + recordClass + ">('records');",
		"    await _recordBox!.put(record." + identifierField + ", record);",
		"  }",
		"",
		"  @override",
		"  Future<void> deleteRecord(String recordId) async {",
		"    _recordBox ??= await Hive.openBox<" + recordClass + ">('records');",
		"    await _recordBox!.delete(recordId);",
		"  }",
	}
	loadSummaryMethod := ""
	loadSummaryInterfaceMethod := ""
	inMemoryLoadSummaryMethod := ""
	if summaryImport != "" {
		summaryArgs := emitGenericSummaryConstructorArgs(dm.Entities, countClass, primaryEnt)
		summaryArgBlock := strings.Join(summaryArgs, "\n")
		summarySetup := ""
		inMemorySummarySetup := ""
		if strings.Contains(summaryArgBlock, "records") {
			summarySetup = strings.Join([]string{
				"    _recordBox ??= await Hive.openBox<" + recordClass + ">('records');",
				"    final records = _recordBox!.values.toList();",
			}, "\n")
			inMemorySummarySetup = "    final records = List<" + recordClass + ">.unmodifiable(_records);"
		}
		loadSummaryInterfaceMethod = "  Future<" + countClass + "> loadSummary();"
		loadSummaryMethod = strings.Join([]string{
			"",
			"  @override",
			"  Future<" + countClass + "> loadSummary() async {",
			summarySetup,
			"    return " + countClass + "(",
			summaryArgBlock,
			"    );",
			"  }",
		}, "\n")
		inMemoryLoadSummaryMethod = strings.Join([]string{
			"",
			"  @override",
			"  Future<" + countClass + "> loadSummary() async {",
			inMemorySummarySetup,
			"    return " + countClass + "(",
			summaryArgBlock,
			"    );",
			"  }",
		}, "\n")
	}
	return strings.Join([]string{
		"import 'package:hive/hive.dart';",
		"",
		"import '../models/record.dart';",
		summaryImport,
		"",
		"abstract class " + repoInterface + " {",
		"  Future<void> init();",
		"  Future<List<" + recordClass + ">> loadRecords();",
		"  Future<void> addRecord(" + recordClass + " record);",
		"  Future<void> updateRecord(" + recordClass + " record);",
		"  Future<void> deleteRecord(String recordId);",
		loadSummaryInterfaceMethod,
		"}",
		"",
		"class " + repoClass + " implements " + repoInterface + " {",
		"  Box<" + recordClass + ">? _recordBox;",
		"",
		"  @override",
		"  Future<void> init() async {",
		"    _recordBox = await Hive.openBox<" + recordClass + ">('records');",
		"  }",
		"",
		strings.Join(methods, "\n"),
		loadSummaryMethod,
		"",
		"}",
		"",
		"class InMemoryRecordRepository implements " + repoInterface + " {",
		"  InMemoryRecordRepository({List<" + recordClass + ">? seedRecords})",
		"      : _records = List<" + recordClass + ">.of(seedRecords ?? const <" + recordClass + ">[]);",
		"",
		"  final List<" + recordClass + "> _records;",
		"",
		"  @override",
		"  Future<void> init() async {}",
		"",
		"  @override",
		"  Future<List<" + recordClass + ">> loadRecords() async => List<" + recordClass + ">.unmodifiable(_records);",
		"",
		"  @override",
		"  Future<void> addRecord(" + recordClass + " record) async {",
		"    _records.add(record);",
		"  }",
		"",
		"  @override",
		"  Future<void> updateRecord(" + recordClass + " record) async {",
		"    final index = _records.indexWhere((item) => item." + identifierField + " == record." + identifierField + ");",
		"    if (index >= 0) {",
		"      _records[index] = record;",
		"    } else {",
		"      _records.add(record);",
		"    }",
		"  }",
		"",
		"  @override",
		"  Future<void> deleteRecord(String recordId) async {",
		"    _records.removeWhere((record) => record." + identifierField + " == recordId);",
		"  }",
		inMemoryLoadSummaryMethod,
		"}",
	}, "\n") + "\n"
}

func emitGenericSummaryConstructorArgs(entities []appprepare.DataEntity, className string, primaryEnt *appprepare.DataEntity) []string {
	for i := range entities {
		entity := &entities[i]
		if emitEntityClassName(entity) != className {
			continue
		}
		args := make([]string, 0, len(entity.Fields))
		for _, field := range entity.Fields {
			dartName := emitSnakeToCamel(strings.TrimSpace(field.Name))
			if dartName == "" {
				continue
			}
			args = append(args, "      "+dartName+": "+emitGenericSummaryDefaultValue(field, primaryEnt)+",")
		}
		return args
	}
	return nil
}

func emitGenericSummaryDefaultValue(field appprepare.DataField, primaryEnt *appprepare.DataEntity) string {
	dartType := emitFieldToDartType(field.Type, emitSnakeToCamel(strings.TrimSpace(field.Name)))
	fieldName := strings.ToLower(strings.TrimSpace(field.Name))
	if expr := emitGenericSummaryNumericExpression(fieldName, primaryEnt, dartType); expr != "" {
		return expr
	}
	if dartType == "int" && (fieldName == "total_count" || fieldName == "record_count" || strings.HasPrefix(fieldName, "total_")) {
		return "records.length"
	}
	if dartType == "int" {
		if expr := emitGenericSummaryCountExpression(fieldName, primaryEnt); expr != "" {
			return expr
		}
	}
	switch dartType {
	case "int":
		return "0"
	case "double":
		return "0"
	case "bool":
		return "false"
	case "DateTime":
		return "DateTime.now()"
	default:
		return "''"
	}
}

func emitGenericSummaryNumericExpression(summaryFieldName string, primaryEnt *appprepare.DataEntity, dartType string) string {
	if primaryEnt == nil {
		return ""
	}
	if strings.Contains(summaryFieldName, "average") || strings.Contains(summaryFieldName, "avg") {
		field := emitGenericNumericSourceField(primaryEnt, summaryFieldName, "rating", "amount", "number", "duration")
		if field == nil {
			return ""
		}
		accessor := "record." + emitSnakeToCamel(field.Name)
		zero := "0"
		if dartType == "double" {
			zero = "0.0"
		}
		return "records.isEmpty ? " + zero + " : records.fold<double>(0.0, (sum, record) => sum + " + accessor + ") / records.length"
	}
	if strings.Contains(summaryFieldName, "duration") || strings.Contains(summaryFieldName, "minutes") || strings.Contains(summaryFieldName, "hours") {
		field := emitGenericNumericSourceField(primaryEnt, summaryFieldName, "duration")
		if field == nil {
			return ""
		}
		accessor := "record." + emitSnakeToCamel(field.Name)
		if dartType == "double" {
			return "records.fold<double>(0.0, (sum, record) => sum + " + accessor + ")"
		}
		return "records.fold<int>(0, (sum, record) => sum + " + accessor + ")"
	}
	return ""
}

func emitGenericNumericSourceField(ent *appprepare.DataEntity, summaryFieldName string, roles ...string) *appprepare.DataField {
	for _, role := range roles {
		if strings.Contains(summaryFieldName, strings.ReplaceAll(role, "_", "")) || strings.Contains(summaryFieldName, role) {
			if field := emitGenericRoleField(ent, role); field != nil && emitGenericIsNumericField(field) {
				return field
			}
		}
	}
	for _, role := range roles {
		if field := emitGenericRoleField(ent, role); field != nil && emitGenericIsNumericField(field) {
			return field
		}
	}
	return nil
}

func emitGenericIsNumericField(field *appprepare.DataField) bool {
	if field == nil {
		return false
	}
	switch emitFieldToDartType(field.Type, emitSnakeToCamel(strings.TrimSpace(field.Name))) {
	case "int", "double":
		return true
	default:
		return false
	}
}

func emitGenericIsBooleanField(field *appprepare.DataField) bool {
	if field == nil {
		return false
	}
	return emitFieldToDartType(field.Type, emitSnakeToCamel(strings.TrimSpace(field.Name))) == "bool"
}

func emitGenericSummaryCountExpression(summaryFieldName string, primaryEnt *appprepare.DataEntity) string {
	if primaryEnt == nil {
		return ""
	}
	statusField := emitGenericRoleField(primaryEnt, "status")
	timeField := emitGenericRoleField(primaryEnt, "due_date", "date", "time")
	if timeField == nil {
		for index := range primaryEnt.Fields {
			if emitFieldToDartType(primaryEnt.Fields[index].Type, emitSnakeToCamel(primaryEnt.Fields[index].Name)) == "DateTime" {
				timeField = &primaryEnt.Fields[index]
				break
			}
		}
	}
	if strings.Contains(summaryFieldName, "overdue") && timeField != nil {
		dateAccess := "record." + emitSnakeToCamel(timeField.Name)
		if doneCheck := emitGenericStatusComparison(primaryEnt, statusField, "done", "completed", "watched", "watered"); doneCheck != "" {
			return "records.where((record) => " + dateAccess + ".isBefore(DateTime.now()) && !(" + doneCheck + ")).length"
		}
		return "records.where((record) => " + dateAccess + ".isBefore(DateTime.now())).length"
	}
	if (strings.Contains(summaryFieldName, "due_soon") || strings.Contains(summaryFieldName, "upcoming") || strings.Contains(summaryFieldName, "next_due")) && timeField != nil {
		dateAccess := "record." + emitSnakeToCamel(timeField.Name)
		return "records.where((record) => !" + dateAccess + ".isBefore(DateTime.now()) && " + dateAccess + ".difference(DateTime.now()).inDays <= 7).length"
	}
	if strings.Contains(summaryFieldName, "done") || strings.Contains(summaryFieldName, "completed") || strings.Contains(summaryFieldName, "watched") || strings.Contains(summaryFieldName, "watered") {
		if expr := emitGenericStatusCountExpression(primaryEnt, statusField, "done", "completed", "watched", "watered"); expr != "" {
			return expr
		}
		return emitGenericBooleanCountExpression(primaryEnt, summaryFieldName, "done", "completed", "watched", "watered")
	}
	if strings.Contains(summaryFieldName, "pending") || strings.Contains(summaryFieldName, "todo") || strings.Contains(summaryFieldName, "planned") || strings.Contains(summaryFieldName, "needs") {
		if expr := emitGenericStatusCountExpression(primaryEnt, statusField, "todo", "pending", "planned", "needsWater"); expr != "" {
			return expr
		}
		return emitGenericBooleanCountExpression(primaryEnt, summaryFieldName, "pending", "todo", "planned", "needs")
	}
	if strings.Contains(summaryFieldName, "progress") || strings.Contains(summaryFieldName, "active") {
		if expr := emitGenericStatusCountExpression(primaryEnt, statusField, "inProgress", "doing", "watching", "active"); expr != "" {
			return expr
		}
		return emitGenericBooleanCountExpression(primaryEnt, summaryFieldName, "progress", "active")
	}
	if strings.Contains(summaryFieldName, "checked") || strings.Contains(summaryFieldName, "enabled") || strings.Contains(summaryFieldName, "flag") || strings.Contains(summaryFieldName, "archived") || strings.Contains(summaryFieldName, "selected") || strings.Contains(summaryFieldName, "packed") {
		return emitGenericBooleanCountExpression(primaryEnt, summaryFieldName, "checked", "enabled", "flag", "archived", "selected", "packed")
	}
	return ""
}

func emitGenericBooleanCountExpression(ent *appprepare.DataEntity, summaryFieldName string, keywords ...string) string {
	field := emitGenericBooleanSourceField(ent, summaryFieldName, keywords...)
	if field == nil {
		return ""
	}
	return "records.where((record) => record." + emitSnakeToCamel(field.Name) + ").length"
}

func emitGenericBooleanSourceField(ent *appprepare.DataEntity, summaryFieldName string, keywords ...string) *appprepare.DataField {
	if ent == nil {
		return nil
	}
	compactSummary := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(summaryFieldName)), "_", "")
	for index := range ent.Fields {
		field := &ent.Fields[index]
		if !emitGenericIsBooleanField(field) {
			continue
		}
		compactName := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(field.Name)), "_", "")
		logicalName := strings.TrimPrefix(strings.TrimPrefix(compactName, "is"), "has")
		if logicalName != "" && strings.Contains(compactSummary, logicalName) {
			return field
		}
		for _, keyword := range keywords {
			compactKeyword := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(keyword)), "_", "")
			if compactKeyword == "" {
				continue
			}
			if strings.Contains(compactSummary, compactKeyword) && (strings.Contains(compactName, compactKeyword) || strings.Contains(logicalName, compactKeyword)) {
				return field
			}
		}
	}
	for index := range ent.Fields {
		field := &ent.Fields[index]
		if emitGenericIsBooleanField(field) && (strings.EqualFold(strings.TrimSpace(field.Role), "flag") || strings.EqualFold(strings.TrimSpace(field.Role), "boolean")) {
			return field
		}
	}
	return nil
}

func emitGenericRoleField(ent *appprepare.DataEntity, roles ...string) *appprepare.DataField {
	if ent == nil {
		return nil
	}
	for _, role := range roles {
		for index := range ent.Fields {
			if strings.EqualFold(strings.TrimSpace(ent.Fields[index].Role), strings.TrimSpace(role)) {
				return &ent.Fields[index]
			}
		}
	}
	for _, role := range roles {
		role = strings.ToLower(strings.TrimSpace(role))
		for index := range ent.Fields {
			name := strings.ToLower(strings.TrimSpace(ent.Fields[index].Name))
			if name == role || strings.Contains(name, role) {
				return &ent.Fields[index]
			}
		}
	}
	return nil
}

func emitGenericStatusCountExpression(ent *appprepare.DataEntity, statusField *appprepare.DataField, members ...string) string {
	comparison := emitGenericStatusComparison(ent, statusField, members...)
	if comparison == "" {
		return ""
	}
	return "records.where((record) => " + comparison + ").length"
}

func emitGenericStatusComparison(ent *appprepare.DataEntity, statusField *appprepare.DataField, members ...string) string {
	if ent == nil || statusField == nil {
		return ""
	}
	fieldName := emitSnakeToCamel(statusField.Name)
	if fieldName == "" {
		return ""
	}
	enumType := emitGenericEnumType(emitEntityClassName(ent), *statusField)
	if enumType == "" {
		return ""
	}
	available := map[string]struct{}{}
	for _, member := range emitEnumMembers(statusField.Type) {
		available[emitSnakeToCamel(member)] = struct{}{}
	}
	comparisons := make([]string, 0, len(members))
	for _, member := range members {
		member = emitSnakeToCamel(member)
		if _, ok := available[member]; !ok {
			continue
		}
		comparisons = append(comparisons, "record."+fieldName+" == "+enumType+"."+member)
	}
	return strings.Join(comparisons, " || ")
}

func emitGenericEnumType(recordClass string, field appprepare.DataField) string {
	if len(emitEnumMembers(field.Type)) == 0 {
		return ""
	}
	dartName := emitSnakeToCamel(strings.TrimSpace(field.Name))
	if dartName == "" {
		return ""
	}
	if strings.TrimSpace(recordClass) == "" {
		recordClass = "Record"
	}
	if dartName == "status" {
		return recordClass + "Status"
	}
	return recordClass + emitSnakeToPascal(dartName)
}

func resolveDeterministicTemplateSlots(task appruns.TaskBundleItem, slotMap appprepare.TemplateSlotMap, targetSet map[string]struct{}) []appprepare.TemplateSlot {
	if len(slotMap.Slots) == 0 {
		return nil
	}
	bindingRefs := deterministicTaskBindingRefSet(task)
	matched := make([]appprepare.TemplateSlot, 0, len(slotMap.Slots))
	seen := make(map[string]struct{}, len(slotMap.Slots))
	appendSlot := func(slot appprepare.TemplateSlot) {
		key := strings.TrimSpace(slot.SlotID)
		if key == "" {
			key = strings.TrimSpace(slot.BindingID) + ":" + strings.TrimSpace(slot.SlotKind)
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		matched = append(matched, slot)
	}
	if len(bindingRefs) > 0 {
		for _, slot := range slotMap.Slots {
			if _, ok := bindingRefs[strings.TrimSpace(slot.BindingID)]; ok {
				appendSlot(slot)
			}
		}
	}
	if len(matched) == 0 {
		for _, slot := range slotMap.Slots {
			if deterministicTemplateSlotMatchesTargets(slot, targetSet) {
				appendSlot(slot)
			}
		}
	}
	return matched
}

func deterministicTaskBindingRefSet(task appruns.TaskBundleItem) map[string]struct{} {
	if task.AllocationTransition == nil || len(task.AllocationTransition.BindingRefs) == 0 {
		return nil
	}
	refs := make(map[string]struct{}, len(task.AllocationTransition.BindingRefs))
	for _, bindingRef := range task.AllocationTransition.BindingRefs {
		trimmed := strings.TrimSpace(bindingRef)
		if trimmed == "" {
			continue
		}
		refs[trimmed] = struct{}{}
	}
	if len(refs) == 0 {
		return nil
	}
	return refs
}

func deterministicTemplateSlotMatchesTargets(slot appprepare.TemplateSlot, targetSet map[string]struct{}) bool {
	if len(targetSet) == 0 || len(slot.TargetPaths) == 0 {
		return false
	}
	for _, targetPath := range slot.TargetPaths {
		normalizedTargetPath := filepath.ToSlash(strings.TrimSpace(targetPath))
		if _, ok := targetSet[normalizedTargetPath]; ok {
			return true
		}
	}
	return false
}

func deterministicSlotsRequirePlanningContext(slots []appprepare.TemplateSlot) bool {
	for _, slot := range slots {
		if normalizeDeterministicSlotKind(slot.SlotKind) == "app_entry" {
			return true
		}
	}
	return false
}

func deterministicTemplateSlotOperations(slots []appprepare.TemplateSlot, ctx deterministicTemplateSlotContext) []appruns.WorkspacePatchOperation {
	if len(slots) == 0 {
		return nil
	}
	ops := make([]appruns.WorkspacePatchOperation, 0, len(slots))
	seenPaths := make(map[string]struct{}, len(slots))
	allowDeleteFlow := builderRuntimeTaskBundleHasSemanticIntentRef(ctx.run.TaskBundle, "ac-delete")
	allowMutationFlow := builderRuntimeTaskBundleAllowsMutationFlow(ctx.run.TaskBundle)
	appendIfTargeted := func(path, content string) {
		normalizedPath := filepath.ToSlash(strings.TrimSpace(path))
		if normalizedPath == "" || strings.TrimSpace(content) == "" {
			return
		}
		if _, ok := ctx.targetSet[normalizedPath]; !ok {
			return
		}
		if _, ok := seenPaths[normalizedPath]; ok {
			return
		}
		seenPaths[normalizedPath] = struct{}{}
		ops = append(ops, appruns.WorkspacePatchOperation{
			Type:    "write_file",
			Path:    normalizedPath,
			Content: content,
		})
	}
	for _, slot := range slots {
		if !deterministicTemplateSlotCanEmit(slot) {
			continue
		}
		switch normalizeDeterministicSlotKind(slot.SlotKind) {
		case "summary":
			if result, emitted := emitter.EmitOverview(ctx.dm); emitted {
				appendIfTargeted(result.HomePagePath, result.HomePageContent)
				appendIfTargeted(result.HomeControllerPath, result.HomeControllerContent)
			} else {
				reg := builderRuntimeOpenLiteOverviewSurfaceRegistry(ctx.run.WorkspacePath)
				if c := builderRuntimeOpenLiteCanonicalGenericOverviewPage(ctx.run.WorkspacePath); c != "" {
					p := strings.TrimSpace(reg.view.resolvedPath)
					if p == "" {
						p = "lib/views/home_page.dart"
					}
					appendIfTargeted(p, c)
				}
				if c := builderRuntimeOpenLiteCanonicalGenericOverviewControllerWithDelete(ctx.run.WorkspacePath, allowDeleteFlow); c != "" {
					p := strings.TrimSpace(reg.controller.resolvedPath)
					if p == "" {
						p = "lib/controllers/home_controller.dart"
					}
					appendIfTargeted(p, c)
				}
			}
		case "list":
			if result, emitted := emitter.EmitCollection(ctx.dm); emitted {
				appendIfTargeted(result.ListPagePath, result.ListPageContent)
				appendIfTargeted(result.ListControllerPath, result.ListControllerContent)
			} else {
				pageC := builderRuntimeOpenLiteCanonicalNoFilterListPage(ctx.run.WorkspacePath)
				ctrlC := builderRuntimeOpenLiteCanonicalNoFilterListControllerWithDelete(ctx.run.WorkspacePath, allowDeleteFlow)
				if pageC != "" {
					appendIfTargeted("lib/views/record_list_page.dart", pageC)
				}
				if ctrlC != "" {
					appendIfTargeted("lib/controllers/record_list_controller.dart", ctrlC)
				}
			}
		case "form":
			if result, emitted := emitter.EmitMutation(ctx.dm); emitted {
				appendIfTargeted(result.FormPagePath, result.FormPageContent)
				appendIfTargeted(result.FormControllerPath, result.FormControllerContent)
			} else {
				if c := builderRuntimeOpenLiteCanonicalGenericMutationPage(ctx.run.WorkspacePath); c != "" {
					appendIfTargeted("lib/views/record_form_page.dart", c)
				}
			}
		case "detail":
			if result, emitted := emitter.EmitInspection(ctx.dm); emitted {
				appendIfTargeted(result.DetailPagePath, result.DetailPageContent)
			} else {
				if c := builderRuntimeOpenLiteCanonicalGenericInspectionPageWithDelete(ctx.run.WorkspacePath, allowDeleteFlow); c != "" {
					dp := strings.TrimSpace(builderRuntimeOpenLiteDetailSurfaceRegistry(ctx.run.WorkspacePath).view.resolvedPath)
					if dp == "" {
						dp = "lib/views/record_detail_page.dart"
					}
					appendIfTargeted(dp, c)
				}
			}
		case "app_entry":
			if result, emitted := emitter.EmitAppEntry(ctx.dm, ctx.planningContext); emitted {
				appendIfTargeted(result.FilePath, result.Content)
			} else {
				if c := builderRuntimeOpenLiteCanonicalGenericAppEntryForTopology(ctx.run.WorkspacePath, allowDeleteFlow, allowMutationFlow); c != "" {
					appendIfTargeted("lib/main.dart", c)
				}
			}
		case "copy":
			if result, emitted := emitter.EmitCopy(ctx.dm); emitted {
				appendIfTargeted(result.FilePath, result.Content)
			}
		case "branding":
			if result, emitted := emitter.EmitBrand(ctx.dm); emitted {
				appendIfTargeted(result.StringsXMLPath, result.StringsXML)
			}
			if result, emitted := emitter.EmitAndroidBuildConfig(ctx.dm); emitted {
				appendIfTargeted(result.FilePath, result.Content)
			}
		case "storage":
			if content := builderRuntimeCanonicalRelationRichRecordRepository(ctx.relationRichProfile); strings.TrimSpace(content) != "" {
				appendIfTargeted("lib/repositories/record_repository.dart", content)
			} else if c := emitGenericRepositoryContent(ctx.run.WorkspacePath, ctx.dm); c != "" {
				appendIfTargeted("lib/repositories/record_repository.dart", c)
			}
		case "test":
			cfg := buildTestEmitConfig(ctx.run)
			if result, emitted := emitter.EmitTest(ctx.dm, cfg); emitted {
				appendIfTargeted(result.FilePath, result.Content)
			}
		}
	}
	return ops
}

func containsMutationSlot(slots []appprepare.TemplateSlot) bool {
	for _, s := range slots {
		if normalizeDeterministicSlotKind(s.SlotKind) == "form" {
			return true
		}
	}
	return false
}

func builderRuntimeTaskBundleAllowsMutationFlow(tasks []appruns.TaskBundleItem) bool {
	return builderRuntimeTaskBundleHasSurfaceRef(tasks, builderRuntimeMutationSurfaceRef) ||
		builderRuntimeTaskBundleHasSemanticIntentRef(tasks, "ac-form") ||
		containsTargetPath(tasks, "lib/views/record_form_page.dart") ||
		containsTargetPath(tasks, "lib/controllers/record_form_controller.dart")
}

func deterministicTemplateSlotCanEmit(slot appprepare.TemplateSlot) bool {
	if !slot.EmitEligible {
		return false
	}
	slotKind := normalizeDeterministicSlotKind(slot.SlotKind)
	overridePolicy := strings.ToLower(strings.TrimSpace(slot.OverridePolicy))
	switch slotKind {
	case "summary", "list", "form", "detail", "app_entry":
		return overridePolicy == "replace" || overridePolicy == "synchronize"
	case "copy", "branding", "test":
		return overridePolicy == "synchronize" || overridePolicy == "replace"
	case "storage":
		return overridePolicy == "extend" || overridePolicy == "replace" || overridePolicy == "synchronize"
	default:
		return false
	}
}

func normalizeDeterministicSlotKind(slotKind string) string {
	normalized := strings.ToLower(strings.TrimSpace(slotKind))
	return strings.ReplaceAll(normalized, "-", "_")
}

// normalizeTargetPaths 将 TargetPaths 转为 set（统一正斜杠并去除空白）。
func normalizeTargetPaths(paths []string) map[string]struct{} {
	set := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		normalized := filepath.ToSlash(strings.TrimSpace(p))
		if normalized != "" {
			set[normalized] = struct{}{}
		}
	}
	return set
}

func deterministicRelationRichModelProfile(dm appprepare.DomainModel) builderRuntimeRelationRichModelProfile {
	entityIDs := make(map[string]struct{}, len(dm.Entities))
	for _, entity := range dm.Entities {
		entityIDs[strings.TrimSpace(entity.EntityID)] = struct{}{}
	}
	if _, ok := entityIDs["entity-project"]; ok {
		if _, ok := entityIDs["entity-task"]; ok {
			if _, ok := entityIDs["entity-tag"]; ok {
				return builderRuntimeRelationRichProjectTaskTagProfile
			}
		}
	}
	if _, ok := entityIDs["entity-inventory-sheet"]; ok {
		if _, ok := entityIDs["entity-line-item"]; ok {
			if _, ok := entityIDs["entity-sku"]; ok {
				return builderRuntimeRelationRichInventorySheetLineProfile
			}
		}
	}
	return ""
}

// buildTestEmitConfig 从 runRecord 构建 TestEmitConfig，使用默认值。
func buildTestEmitConfig(run runRecord) emitter.TestEmitConfig {
	return emitter.TestEmitConfig{
		AppClassName:        "AppFactoryApp",
		RepositoryType:      "InMemoryRecordRepository",
		DisableMutationFlow: !builderRuntimeTaskBundleAllowsMutationFlow(run.TaskBundle),
	}
}
