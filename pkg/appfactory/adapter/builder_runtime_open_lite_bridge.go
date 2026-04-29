package adapter

import (
	"path/filepath"
	"strings"

	"github.com/sipeed/picoclaw/pkg/appfactory/emitter"
	appprepare "github.com/sipeed/picoclaw/pkg/appfactory/prepare"
)

func builderRuntimeNormalizeSourceForComparison(content string) string {
	return strings.TrimSpace(strings.ReplaceAll(content, "\r\n", "\n"))
}

func builderRuntimeOpenLiteRelationRichWorkspace(workspacePath string) bool {
	return builderRuntimeRelationRichModelProfileForWorkspace(workspacePath) != ""
}

func builderRuntimeOpenLiteCanonicalRelationRichCopyContent(workspacePath, content string) string {
	dm, ok := builderRuntimeOpenLiteRelationRichDomainModel(workspacePath, content)
	if !ok {
		return ""
	}
	result, ok := emitter.EmitCopy(dm)
	if !ok {
		return ""
	}
	return result.Content
}

func builderRuntimeOpenLiteCanonicalRelationRichFormControllerContent(workspacePath, content string) string {
	dm, ok := builderRuntimeOpenLiteRelationRichDomainModel(workspacePath, content)
	if !ok {
		return ""
	}
	result, ok := emitter.EmitMutation(dm)
	if !ok {
		return ""
	}
	return result.FormControllerContent
}

func normalizeBuilderRuntimeOpenLiteEscapedNewlineGetterNames(content string) string {
	return content
}

func normalizeBuilderRuntimeOpenLiteCopyHelperReferences(content string) string {
	updated := strings.ReplaceAll(content, "openCCopy.", "openLiteCopy.")
	updated = strings.ReplaceAll(updated, "openLiteKey.", "openLiteCopy.")
	return updated
}

func normalizeBuilderRuntimeOpenLiteDropdownButtonInitialValue(content string) string {
	return strings.ReplaceAll(content, "initialValue:", "value:")
}

func normalizeBuilderRuntimeOpenLiteHomeControllerContent(workspacePath, content string) string {
	className, ok := builderRuntimeOpenLiteOverviewControllerClassName(content)
	if !ok {
		return content
	}
	canonical := builderRuntimeOpenLiteCanonicalRelationRichHomeControllerContent(workspacePath, content)
	if strings.TrimSpace(canonical) == "" {
		return content
	}
	repository := builderRuntimePrimaryRepositoryCandidate(workspacePath, content, className)
	if className != "HomeController" {
		canonical = strings.ReplaceAll(canonical, "HomeController", className)
	}
	canonical = builderRuntimeOpenLiteRewriteHomeControllerCanonicalToRepository(canonical, repository, className, content)
	if builderRuntimeNormalizeSourceForComparison(content) == builderRuntimeNormalizeSourceForComparison(canonical) {
		return content
	}
	return canonical
}

func normalizeBuilderRuntimeOpenLiteHomePageContent(workspacePath, path, content string) string {
	surface := builderRuntimePrimaryOverviewSurfaceCandidate(workspacePath, path, content)
	viewPath := strings.TrimSpace(surface.view.path)
	if viewPath == "" && builderRuntimeLikelyOverviewSurfaceViewPath(path) {
		surface.view.path = filepath.ToSlash(strings.TrimSpace(path))
	}
	if currentClass, ok := builderRuntimeOpenLiteOverviewViewClassName(content); ok {
		if surface.view.className == "" || (currentClass != "HomePage" && surface.view.className == "HomePage") {
			surface.view.className = currentClass
		}
		accepted, _ := builderRuntimeDartConstructorNamedParameters(content, currentClass)
		if controllerParam := builderRuntimeOverviewViewControllerParam(accepted); controllerParam != "" {
			surface.view.controllerParam = controllerParam
		}
		if createCallbackName := builderRuntimeOverviewViewCreateCallbackName(accepted); createCallbackName != "" {
			surface.view.createCallbackName = createCallbackName
		}
		if viewAllCallbackName := builderRuntimeOverviewViewViewAllCallbackName(accepted); viewAllCallbackName != "" {
			surface.view.viewAllCallbackName = viewAllCallbackName
		}
		if surface.view.controllerParam != "" {
			if controllerType := builderRuntimeDartFieldTypeByName(content, surface.view.controllerParam); controllerType != "" {
				surface.view.controllerType = controllerType
			}
		}
	}
	if strings.TrimSpace(surface.view.controllerType) != "" {
		if surface.controller.className == "" || (strings.TrimSpace(surface.view.controllerType) != "RecordFormController" && surface.controller.className == "RecordFormController") {
			surface.controller.className = strings.TrimSpace(surface.view.controllerType)
		}
	}
	if strings.TrimSpace(surface.view.path) != "" {
		if surface.controller.path == "" || (strings.TrimSpace(surface.view.controllerType) != "" && strings.TrimSpace(surface.view.controllerType) != "RecordFormController" && surface.controller.path == "lib/controllers/record_form_controller.dart") {
			surface.controller.path = builderRuntimeControllerPathHintForViewPath(surface.view.path)
		}
	}
	if !builderRuntimeOpenLiteMatchesOverviewViewCandidate(content, surface.view) {
		return content
	}
	canonical := builderRuntimeOpenLiteCanonicalRelationRichHomePageContent(workspacePath, content)
	if strings.TrimSpace(canonical) == "" {
		return content
	}
	canonical = builderRuntimeOpenLiteRewriteHomePageCanonicalToCandidate(canonical, surface.view, surface.controller)
	if builderRuntimeNormalizeSourceForComparison(content) == builderRuntimeNormalizeSourceForComparison(canonical) {
		return content
	}
	return canonical
}

func builderRuntimeOpenLiteMatchesOverviewViewCandidate(content string, candidate builderRuntimeOverviewViewCandidate) bool {
	if strings.Contains(content, "class HomePage") {
		return true
	}
	className := strings.TrimSpace(candidate.className)
	if className != "" && strings.Contains(content, "class "+className) {
		return true
	}
	_, ok := builderRuntimeOpenLiteOverviewViewClassName(content)
	return ok
}

func builderRuntimeOpenLiteOverviewViewClassName(content string) (string, bool) {
	if strings.Contains(content, "class HomePage") {
		return "HomePage", true
	}
	matches := builderRuntimeNamedDartTypeDeclarationPattern.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) < 3 || strings.TrimSpace(match[1]) != "class" {
			continue
		}
		className := strings.TrimSpace(match[2])
		if className == "" {
			continue
		}
		lowerName := strings.ToLower(className)
		if (strings.Contains(lowerName, "home") || strings.Contains(lowerName, "overview")) && (strings.Contains(lowerName, "page") || strings.Contains(lowerName, "view")) {
			return className, true
		}
	}
	return "", false
}

func builderRuntimeOpenLiteRewriteHomePageCanonicalToCandidate(canonical string, view builderRuntimeOverviewViewCandidate, controller builderRuntimeOverviewControllerCandidate) string {
	updated := canonical
	className := strings.TrimSpace(view.className)
	if className == "" {
		className = "HomePage"
	}
	if className != "HomePage" {
		updated = strings.ReplaceAll(updated, "HomePage", className)
	}
	controllerType := strings.TrimSpace(controller.className)
	if controllerType == "" {
		controllerType = strings.TrimSpace(view.controllerType)
	}
	if controllerType == "" {
		controllerType = "HomeController"
	}
	if controllerType != "HomeController" {
		updated = strings.ReplaceAll(updated, "HomeController", controllerType)
	}
	if controllerPath := strings.TrimSpace(controller.path); controllerPath != "" && controllerPath != "lib/controllers/home_controller.dart" {
		updated = strings.Replace(updated, "import '../controllers/home_controller.dart';", "import '../controllers/"+filepath.Base(controllerPath)+"';", 1)
	}
	controllerParam := strings.TrimSpace(view.controllerParam)
	if controllerParam == "" {
		controllerParam = "controller"
	}
	if controllerParam != "controller" {
		updated = strings.ReplaceAll(updated, "required this.controller,", "required this."+controllerParam+",")
		updated = strings.ReplaceAll(updated, "final "+controllerType+" controller;", "final "+controllerType+" "+controllerParam+";")
		updated = strings.ReplaceAll(updated, "animation: controller,", "animation: "+controllerParam+",")
		updated = strings.ReplaceAll(updated, "controller.isLoading", controllerParam+".isLoading")
		updated = strings.ReplaceAll(updated, "controller.projects", controllerParam+".projects")
		updated = strings.ReplaceAll(updated, "controller.warehouses", controllerParam+".warehouses")
		updated = strings.ReplaceAll(updated, "controller.getSummaryForProject", controllerParam+".getSummaryForProject")
		updated = strings.ReplaceAll(updated, "controller.getSummaryForWarehouse", controllerParam+".getSummaryForWarehouse")
	}
	createCallbackName := strings.TrimSpace(view.createCallbackName)
	defaultCreateCallbackName := builderRuntimeOpenLiteOverviewDefaultCreateCallbackName(updated)
	if createCallbackName == "" {
		createCallbackName = defaultCreateCallbackName
	}
	if createCallbackName != defaultCreateCallbackName {
		updated = strings.ReplaceAll(updated, defaultCreateCallbackName, createCallbackName)
	}
	viewAllCallbackName := strings.TrimSpace(view.viewAllCallbackName)
	defaultViewAllCallbackName := builderRuntimeOpenLiteOverviewDefaultViewAllCallbackName(updated)
	if viewAllCallbackName == "" {
		viewAllCallbackName = defaultViewAllCallbackName
	}
	if viewAllCallbackName != defaultViewAllCallbackName {
		updated = strings.ReplaceAll(updated, defaultViewAllCallbackName, viewAllCallbackName)
	}
	return updated
}

func builderRuntimeOpenLiteOverviewDefaultCreateCallbackName(content string) string {
	if strings.Contains(content, "onCreateRecord") {
		return "onCreateRecord"
	}
	return "onCreateTask"
}

func builderRuntimeOpenLiteOverviewDefaultViewAllCallbackName(content string) string {
	if strings.Contains(content, "onViewAllRecords") {
		return "onViewAllRecords"
	}
	return "onViewAllTasks"
}

func normalizeBuilderRuntimeOpenLiteRecordDetailPageContent(workspacePath, content string) string {
	surface := builderRuntimePrimaryDetailSurfaceCandidate(workspacePath, content)
	if currentClass, ok := builderRuntimeOpenLiteDetailViewClassName(content); ok {
		if surface.view.className == "" || (currentClass != "RecordDetailPage" && surface.view.className == "RecordDetailPage") {
			surface.view.className = currentClass
		}
		accepted, required := builderRuntimeDartConstructorNamedParameters(content, currentClass)
		if recordParam := builderRuntimeDetailViewRecordParam(required); recordParam != "" {
			surface.view.recordParam = recordParam
		}
		if editCallbackName := builderRuntimeDetailViewEditCallbackName(accepted); editCallbackName != "" {
			surface.view.editCallbackName = editCallbackName
		}
		if deleteCallbackName := builderRuntimeDetailViewDeleteCallbackName(accepted); deleteCallbackName != "" {
			surface.view.deleteCallbackName = deleteCallbackName
		}
		if len(required) > 0 {
			surface.view.requiredParams = builderRuntimeSortedAcceptedNames(required)
		}
	}
	if !builderRuntimeOpenLiteMatchesDetailViewCandidate(content, surface.view) {
		return content
	}
	canonical := builderRuntimeOpenLiteCanonicalRelationRichDetailPageContent(workspacePath, content)
	if strings.TrimSpace(canonical) == "" {
		return content
	}
	canonical = builderRuntimeOpenLiteRewriteDetailPageCanonicalToCandidate(canonical, content, surface.view)
	if builderRuntimeNormalizeSourceForComparison(content) == builderRuntimeNormalizeSourceForComparison(canonical) {
		return content
	}
	return canonical
}

func builderRuntimeOpenLiteMatchesDetailViewCandidate(content string, candidate builderRuntimeDetailViewCandidate) bool {
	if strings.Contains(content, "class RecordDetailPage") {
		return true
	}
	className := strings.TrimSpace(candidate.className)
	if className != "" && strings.Contains(content, "class "+className) {
		return true
	}
	_, ok := builderRuntimeOpenLiteDetailViewClassName(content)
	return ok
}

func builderRuntimeOpenLiteDetailViewClassName(content string) (string, bool) {
	if strings.Contains(content, "class RecordDetailPage") {
		return "RecordDetailPage", true
	}
	matches := builderRuntimeNamedDartTypeDeclarationPattern.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) < 3 || strings.TrimSpace(match[1]) != "class" {
			continue
		}
		className := strings.TrimSpace(match[2])
		if className == "" {
			continue
		}
		lowerName := strings.ToLower(className)
		if strings.Contains(lowerName, "detail") || strings.Contains(lowerName, "inspection") {
			return className, true
		}
	}
	return "", false
}

func builderRuntimeOpenLiteRewriteDetailPageCanonicalToCandidate(canonical, originalContent string, candidate builderRuntimeDetailViewCandidate) string {
	updated := canonical
	className := strings.TrimSpace(candidate.className)
	if className == "" {
		className = "RecordDetailPage"
	}
	if className != "RecordDetailPage" {
		updated = strings.ReplaceAll(updated, "RecordDetailPage", className)
	}
	defaultRecordParam := builderRuntimeOpenLiteDetailViewRecordParam(updated)
	recordParam := strings.TrimSpace(candidate.recordParam)
	if recordParam == "" {
		recordParam = defaultRecordParam
	}
	if defaultRecordParam != "" && recordParam != "" && recordParam != defaultRecordParam {
		recordType := builderRuntimeDartFieldTypeByName(updated, defaultRecordParam)
		updated = strings.ReplaceAll(updated, "required this."+defaultRecordParam+",", "required this."+recordParam+",")
		updated = strings.ReplaceAll(updated, "widget."+defaultRecordParam, "widget."+recordParam)
		if recordType != "" {
			updated = strings.ReplaceAll(updated, "final "+recordType+" "+defaultRecordParam+";", "final "+recordType+" "+recordParam+";")
		}
	}
	defaultEditCallbackName := builderRuntimeOpenLiteDetailViewEditCallbackName(updated)
	editCallbackName := strings.TrimSpace(candidate.editCallbackName)
	if editCallbackName == "" {
		editCallbackName = defaultEditCallbackName
	}
	if defaultEditCallbackName != "" && editCallbackName != "" && editCallbackName != defaultEditCallbackName {
		recordType := builderRuntimeDartFieldTypeByName(updated, recordParam)
		updated = strings.ReplaceAll(updated, "required this."+defaultEditCallbackName+",", "required this."+editCallbackName+",")
		if recordType != "" && recordParam != "" {
			updated = strings.ReplaceAll(updated, "final Future<void> Function("+recordType+" "+recordParam+") "+defaultEditCallbackName+";", "final Future<void> Function("+recordType+" "+recordParam+") "+editCallbackName+";")
		}
		updated = strings.ReplaceAll(updated, "widget."+defaultEditCallbackName+"(", "widget."+editCallbackName+"(")
	}
	updated = builderRuntimeOpenLiteRewriteDetailPageAliasParamsByType(updated, originalContent, candidate)
	return updated
}

func builderRuntimeOpenLiteRewriteDetailPageAliasParamsByType(canonical, originalContent string, candidate builderRuntimeDetailViewCandidate) string {
	canonicalClassName, ok := builderRuntimeOpenLiteDetailViewClassName(canonical)
	if !ok {
		return canonical
	}
	originalClassName, ok := builderRuntimeOpenLiteDetailViewClassName(originalContent)
	if !ok {
		return canonical
	}
	_, canonicalRequired := builderRuntimeDartConstructorNamedParameters(canonical, canonicalClassName)
	_, originalRequired := builderRuntimeDartConstructorNamedParameters(originalContent, originalClassName)
	usedOriginal := map[string]struct{}{}
	for _, reserved := range []string{candidate.recordParam, candidate.editCallbackName, candidate.deleteCallbackName} {
		trimmed := strings.TrimSpace(reserved)
		if trimmed != "" {
			usedOriginal[trimmed] = struct{}{}
		}
	}
	updated := canonical
	for _, canonicalName := range builderRuntimeSortedAcceptedNames(canonicalRequired) {
		trimmedCanonicalName := strings.TrimSpace(canonicalName)
		if trimmedCanonicalName == "" || trimmedCanonicalName == "key" || strings.HasPrefix(trimmedCanonicalName, "on") {
			continue
		}
		if trimmedCanonicalName == strings.TrimSpace(candidate.recordParam) {
			continue
		}
		canonicalFieldType := builderRuntimeDartFieldTypeByName(updated, trimmedCanonicalName)
		if canonicalFieldType == "" {
			continue
		}
		aliasName := builderRuntimeOpenLiteFindDetailAliasParamByType(originalContent, originalRequired, canonicalFieldType, usedOriginal)
		if aliasName == "" || aliasName == trimmedCanonicalName {
			continue
		}
		usedOriginal[aliasName] = struct{}{}
		updated = strings.ReplaceAll(updated, "required this."+trimmedCanonicalName+",", "required this."+aliasName+",")
		updated = strings.ReplaceAll(updated, "widget."+trimmedCanonicalName, "widget."+aliasName)
		updated = strings.ReplaceAll(updated, "final "+canonicalFieldType+" "+trimmedCanonicalName+";", "final "+canonicalFieldType+" "+aliasName+";")
	}
	return updated
}

func builderRuntimeOpenLiteFindDetailAliasParamByType(originalContent string, originalRequired map[string]struct{}, wantType string, usedOriginal map[string]struct{}) string {
	trimmedWantType := strings.TrimSpace(wantType)
	if trimmedWantType == "" {
		return ""
	}
	for _, name := range builderRuntimeSortedAcceptedNames(originalRequired) {
		trimmedName := strings.TrimSpace(name)
		if trimmedName == "" || trimmedName == "key" || strings.HasPrefix(trimmedName, "on") {
			continue
		}
		if _, used := usedOriginal[trimmedName]; used {
			continue
		}
		if builderRuntimeDartFieldTypeByName(originalContent, trimmedName) == trimmedWantType {
			return trimmedName
		}
	}
	return ""
}

func builderRuntimeOpenLiteDetailViewRecordParam(content string) string {
	className, ok := builderRuntimeOpenLiteDetailViewClassName(content)
	if !ok {
		return ""
	}
	_, required := builderRuntimeDartConstructorNamedParameters(content, className)
	return builderRuntimeDetailViewRecordParam(required)
}

func builderRuntimeOpenLiteDetailViewEditCallbackName(content string) string {
	className, ok := builderRuntimeOpenLiteDetailViewClassName(content)
	if !ok {
		return ""
	}
	accepted, _ := builderRuntimeDartConstructorNamedParameters(content, className)
	return builderRuntimeDetailViewEditCallbackName(accepted)
}

func builderRuntimeOpenLiteOverviewControllerClassName(content string) (string, bool) {
	if strings.Contains(content, "class HomeController") {
		return "HomeController", true
	}
	matches := builderRuntimeNamedDartTypeDeclarationPattern.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) < 3 || strings.TrimSpace(match[1]) != "class" {
			continue
		}
		className := strings.TrimSpace(match[2])
		if className == "" {
			continue
		}
		lowerName := strings.ToLower(className)
		if strings.Contains(lowerName, "controller") && (strings.Contains(lowerName, "home") || strings.Contains(lowerName, "overview")) {
			return className, true
		}
	}
	return "", false
}

func builderRuntimeOpenLiteRewriteHomeControllerCanonicalToRepository(canonical string, repository builderRuntimeRepositoryCandidate, className string, content string) string {
	updated := canonical
	repositoryImportPath := strings.TrimSpace(repository.path)
	if repositoryImportPath != "" && repositoryImportPath != "lib/repositories/record_repository.dart" {
		updated = strings.Replace(updated, "import '../repositories/record_repository.dart';", "import '../repositories/"+filepath.Base(repositoryImportPath)+"';", 1)
	}
	repositoryType := strings.TrimSpace(repository.repositoryType)
	if repositoryType != "" && repositoryType != "RecordRepository" {
		updated = strings.ReplaceAll(updated, "RecordRepository", repositoryType)
	}
	repositoryParam := builderRuntimeOpenLiteOverviewControllerRepositoryParam(content)
	shouldRewriteRepositoryParam := className != "HomeController" || repositoryImportPath != "" && repositoryImportPath != "lib/repositories/record_repository.dart" || repositoryType != "" && repositoryType != "RecordRepository"
	if shouldRewriteRepositoryParam && repositoryParam != "" && repositoryParam != "repository" {
		requiredRepositoryParam := "required " + builderRuntimeOpenLiteCollectionControllerRepositoryTypeName(repositoryType) + " repository"
		updated = strings.Replace(updated, requiredRepositoryParam, "required "+builderRuntimeOpenLiteCollectionControllerRepositoryTypeName(repositoryType)+" "+repositoryParam, 1)
		updated = strings.Replace(updated, "_repository = repository;", "_repository = "+repositoryParam+";", 1)
	}
	return updated
}

func builderRuntimeOpenLiteOverviewControllerRepositoryParam(content string) string {
	accepted, _ := builderRuntimeDartConstructorNamedParameters(content, builderRuntimeOpenLiteOverviewControllerClassNameOrEmpty(content))
	if len(accepted) == 0 {
		return ""
	}
	for _, candidate := range []string{"repository", "recordRepository", "taskRepository", "dashboardRepository"} {
		if _, ok := accepted[candidate]; ok {
			return candidate
		}
	}
	for name := range accepted {
		if strings.Contains(strings.ToLower(name), "repository") {
			return name
		}
	}
	return ""
}

func builderRuntimeOpenLiteOverviewControllerClassNameOrEmpty(content string) string {
	className, ok := builderRuntimeOpenLiteOverviewControllerClassName(content)
	if !ok {
		return ""
	}
	return className
}

func normalizeBuilderRuntimeOpenLiteFormControllerContent(workspacePath, content string) string {
	className, ok := builderRuntimeOpenLiteMutationControllerClassName(content)
	if !ok {
		return content
	}
	canonical := builderRuntimeOpenLiteCanonicalRelationRichFormControllerContent(workspacePath, content)
	if strings.TrimSpace(canonical) == "" {
		return content
	}
	repository := builderRuntimePrimaryRepositoryCandidate(workspacePath, content, className)
	if className != "RecordFormController" {
		canonical = strings.ReplaceAll(canonical, "RecordFormController", className)
	}
	canonical = builderRuntimeOpenLiteRewriteFormControllerCanonicalToRepository(canonical, repository, className, content)
	if builderRuntimeNormalizeSourceForComparison(content) == builderRuntimeNormalizeSourceForComparison(canonical) {
		return content
	}
	return canonical
}

func builderRuntimeOpenLiteMutationControllerClassName(content string) (string, bool) {
	if strings.Contains(content, "class RecordFormController") {
		return "RecordFormController", true
	}
	matches := builderRuntimeNamedDartTypeDeclarationPattern.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) < 3 || strings.TrimSpace(match[1]) != "class" {
			continue
		}
		className := strings.TrimSpace(match[2])
		if className == "" {
			continue
		}
		lowerName := strings.ToLower(className)
		if strings.Contains(lowerName, "form") || strings.Contains(lowerName, "edit") || strings.Contains(lowerName, "mutation") {
			return className, true
		}
	}
	return "", false
}

func builderRuntimeOpenLiteRewriteFormControllerCanonicalToRepository(canonical string, repository builderRuntimeRepositoryCandidate, className string, content string) string {
	updated := canonical
	repositoryImportPath := strings.TrimSpace(repository.path)
	if repositoryImportPath != "" && repositoryImportPath != "lib/repositories/record_repository.dart" {
		updated = strings.Replace(updated, "import '../repositories/record_repository.dart';", "import '../repositories/"+filepath.Base(repositoryImportPath)+"';", 1)
	}
	repositoryType := strings.TrimSpace(repository.repositoryType)
	if repositoryType != "" && repositoryType != "RecordRepository" {
		updated = strings.ReplaceAll(updated, "RecordRepository", repositoryType)
	}
	repositoryParam := builderRuntimeOpenLiteMutationControllerRepositoryParam(content)
	shouldRewriteRepositoryParam := className != "RecordFormController" || repositoryImportPath != "" && repositoryImportPath != "lib/repositories/record_repository.dart" || repositoryType != "" && repositoryType != "RecordRepository"
	if shouldRewriteRepositoryParam && repositoryParam != "" && repositoryParam != "repository" {
		requiredRepositoryParam := "required " + builderRuntimeOpenLiteCollectionControllerRepositoryTypeName(repositoryType) + " repository"
		updated = strings.Replace(updated, requiredRepositoryParam, "required "+builderRuntimeOpenLiteCollectionControllerRepositoryTypeName(repositoryType)+" "+repositoryParam, 1)
		updated = strings.ReplaceAll(updated, "this.repository", "this."+repositoryParam)
		updated = strings.Replace(updated, "final "+builderRuntimeOpenLiteCollectionControllerRepositoryTypeName(repositoryType)+" repository;", "final "+builderRuntimeOpenLiteCollectionControllerRepositoryTypeName(repositoryType)+" "+repositoryParam+";", 1)
		updated = strings.ReplaceAll(updated, "repository.load", repositoryParam+".load")
		updated = strings.ReplaceAll(updated, "repository.save", repositoryParam+".save")
		updated = strings.ReplaceAll(updated, "repository.update", repositoryParam+".update")
		updated = strings.ReplaceAll(updated, "repository.add", repositoryParam+".add")
	}
	return updated
}

func builderRuntimeOpenLiteMutationControllerRepositoryParam(content string) string {
	accepted, _ := builderRuntimeDartConstructorNamedParameters(content, builderRuntimeOpenLiteMutationControllerClassNameOrEmpty(content))
	if len(accepted) == 0 {
		return ""
	}
	return builderRuntimeMutationViewRepositoryParam(accepted)
}

func builderRuntimeOpenLiteMutationControllerClassNameOrEmpty(content string) string {
	className, ok := builderRuntimeOpenLiteMutationControllerClassName(content)
	if !ok {
		return ""
	}
	return className
}

func normalizeBuilderRuntimeOpenLiteListControllerContent(workspacePath string, allowFilterFlow bool, needsCollectionCreateEntry bool, content string) string {
	surface := builderRuntimePrimaryCollectionSurfaceCandidate(workspacePath, content)
	candidate := surface.controller
	if currentClass := builderRuntimeOpenLiteFirstNamedDartClass(content); currentClass != "" && currentClass != "RecordListController" && currentClass != candidate.className {
		if explicitCandidate := builderRuntimePrimaryCollectionControllerCandidate(workspacePath, currentClass, content); explicitCandidate.className == currentClass {
			candidate = explicitCandidate
		}
	}
	repository := surface.repository
	if candidate.path != surface.controller.path || candidate.className != surface.controller.className || candidate.repositoryParam != surface.controller.repositoryParam {
		repository = builderRuntimePrimaryRepositoryCandidate(workspacePath, content, candidate.path, candidate.className, candidate.repositoryParam)
	}
	if !builderRuntimeOpenLiteMatchesCollectionControllerCandidate(content, candidate) {
		return content
	}
	if !allowFilterFlow && builderRuntimeTaskOutputLooksLikeUnexpectedListFilter(content) {
		canonical := builderRuntimeOpenLiteCanonicalNoFilterListController(workspacePath)
		if strings.TrimSpace(canonical) == "" {
			return content
		}
		canonical = builderRuntimeOpenLiteRewriteListControllerCanonicalToCandidate(canonical, candidate, repository)
		if builderRuntimeNormalizeSourceForComparison(content) == builderRuntimeNormalizeSourceForComparison(canonical) {
			return content
		}
		return canonical
	}
	canonical := builderRuntimeOpenLiteCanonicalRelationRichListControllerContent(workspacePath, content)
	if strings.TrimSpace(canonical) == "" {
		return content
	}
	canonical = builderRuntimeOpenLiteRewriteListControllerCanonicalToCandidate(canonical, candidate, repository)
	if builderRuntimeNormalizeSourceForComparison(content) == builderRuntimeNormalizeSourceForComparison(canonical) {
		return content
	}
	return canonical
}

func builderRuntimeOpenLiteMatchesCollectionControllerCandidate(content string, candidate builderRuntimeCollectionControllerCandidate) bool {
	if strings.Contains(content, "class RecordListController") {
		return true
	}
	className := strings.TrimSpace(candidate.className)
	if className == "" {
		return false
	}
	return strings.Contains(content, "class "+className)
}

func builderRuntimeOpenLiteRewriteListControllerCanonicalToCandidate(canonical string, candidate builderRuntimeCollectionControllerCandidate, repository builderRuntimeRepositoryCandidate) string {
	updated := canonical
	className := strings.TrimSpace(candidate.className)
	if className == "" {
		className = "RecordListController"
	}
	if className != "RecordListController" {
		updated = strings.ReplaceAll(updated, "RecordListController", className)
	}
	repositoryImportPath := strings.TrimSpace(repository.path)
	if repositoryImportPath != "" && repositoryImportPath != "lib/repositories/record_repository.dart" {
		updated = strings.Replace(updated, "import '../repositories/record_repository.dart';", "import '../repositories/"+filepath.Base(repositoryImportPath)+"';", 1)
	}
	repositoryType := strings.TrimSpace(repository.repositoryType)
	if repositoryType != "" && repositoryType != "RecordRepository" {
		updated = strings.ReplaceAll(updated, "RecordRepository", repositoryType)
	}
	repositoryParam := strings.TrimSpace(candidate.repositoryParam)
	if repositoryParam == "" {
		repositoryParam = "repository"
	}
	if repositoryParam != "repository" {
		requiredRepositoryParam := "required " + builderRuntimeOpenLiteCollectionControllerRepositoryTypeName(repositoryType) + " repository"
		updated = strings.Replace(updated, requiredRepositoryParam, "required "+builderRuntimeOpenLiteCollectionControllerRepositoryTypeName(repositoryType)+" "+repositoryParam, 1)
		updated = strings.Replace(updated, "_repository = repository {", "_repository = "+repositoryParam+" {", 1)
	}
	return updated
}

func builderRuntimeOpenLiteCollectionControllerRepositoryTypeName(repositoryType string) string {
	if strings.TrimSpace(repositoryType) == "" {
		return "RecordRepository"
	}
	return strings.TrimSpace(repositoryType)
}

func builderRuntimeOpenLiteMutationViewClassName(content string) (string, bool) {
	if strings.Contains(content, "class RecordFormPage") {
		return "RecordFormPage", true
	}
	matches := builderRuntimeNamedDartTypeDeclarationPattern.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) < 3 || strings.TrimSpace(match[1]) != "class" {
			continue
		}
		className := strings.TrimSpace(match[2])
		if className == "" {
			continue
		}
		lowerName := strings.ToLower(className)
		if (strings.Contains(lowerName, "form") || strings.Contains(lowerName, "edit") || strings.Contains(lowerName, "mutation")) && strings.Contains(lowerName, "page") {
			return className, true
		}
	}
	return "", false
}

func builderRuntimeOpenLiteMatchesMutationViewCandidate(content string, candidate builderRuntimeMutationViewCandidate) bool {
	if strings.Contains(content, "class RecordFormPage") {
		return true
	}
	className := strings.TrimSpace(candidate.className)
	if className != "" && strings.Contains(content, "class "+className) {
		return true
	}
	_, ok := builderRuntimeOpenLiteMutationViewClassName(content)
	return ok
}

func builderRuntimeOpenLiteRewriteFormPageCanonicalToCandidate(canonical string, candidate builderRuntimeMutationViewCandidate, controller builderRuntimeMutationControllerCandidate) string {
	updated := canonical
	className := strings.TrimSpace(candidate.className)
	if className == "" {
		className = "RecordFormPage"
	}
	if className != "RecordFormPage" {
		updated = strings.ReplaceAll(updated, "RecordFormPage", className)
	}
	controllerType := strings.TrimSpace(candidate.controllerType)
	if controllerType == "" {
		controllerType = strings.TrimSpace(controller.className)
	}
	if controllerType == "" {
		controllerType = "RecordFormController"
	}
	if controllerType != "RecordFormController" {
		updated = strings.ReplaceAll(updated, "RecordFormController", controllerType)
	}
	controllerPath := strings.TrimSpace(controller.path)
	if controllerType != "RecordFormController" && (controllerPath == "" || controllerPath == "lib/controllers/record_form_controller.dart") {
		if controllerFile := builderRuntimeOpenLiteIdentifierSnakeCase(controllerType); controllerFile != "" {
			controllerPath = "lib/controllers/" + controllerFile + ".dart"
		}
	}
	if controllerPath != "" && controllerPath != "lib/controllers/record_form_controller.dart" {
		updated = strings.Replace(updated, "import '../controllers/record_form_controller.dart';", "import '../controllers/"+filepath.Base(controllerPath)+"';", 1)
	}
	controllerParam := strings.TrimSpace(candidate.controllerParam)
	if controllerParam == "" {
		controllerParam = "controller"
	}
	if controllerParam != "controller" {
		updated = strings.ReplaceAll(updated, "required this.controller,", "required this."+controllerParam+",")
		updated = strings.ReplaceAll(updated, "final "+controllerType+" controller;", "final "+controllerType+" "+controllerParam+";")
		updated = strings.ReplaceAll(updated, "widget.controller", "widget."+controllerParam)
	}
	return updated
}

func normalizeBuilderRuntimeOpenLiteFormPageContent(workspacePath, content string) string {
	surface := builderRuntimePrimaryMutationSurfaceCandidate(workspacePath, content)
	if currentClass, ok := builderRuntimeOpenLiteMutationViewClassName(content); ok {
		if surface.view.className == "" || (currentClass != "RecordFormPage" && surface.view.className == "RecordFormPage") {
			surface.view.className = currentClass
		}
		if surface.view.path == "" && surface.view.className != "" {
			if viewFile := builderRuntimeOpenLiteIdentifierSnakeCase(surface.view.className); viewFile != "" {
				surface.view.path = "lib/views/" + viewFile + ".dart"
			}
		}
		accepted, _ := builderRuntimeDartConstructorNamedParameters(content, currentClass)
		if controllerParam := builderRuntimeMutationViewControllerParam(accepted); controllerParam != "" {
			surface.view.controllerParam = controllerParam
			if controllerType := builderRuntimeDartFieldTypeByName(content, controllerParam); controllerType != "" {
				surface.view.controllerType = controllerType
			}
		}
		if repositoryParam := builderRuntimeMutationViewRepositoryParam(accepted); repositoryParam != "" {
			surface.view.repositoryParam = repositoryParam
		}
		if initialParam := builderRuntimeMutationViewInitialParam(accepted); initialParam != "" {
			surface.view.initialParam = initialParam
		}
	}
	if surface.controller.className == "" && strings.TrimSpace(surface.view.controllerType) != "" {
		surface.controller.className = strings.TrimSpace(surface.view.controllerType)
	}
	if surface.controller.path == "" && strings.TrimSpace(surface.view.path) != "" {
		surface.controller.path = builderRuntimeControllerPathHintForViewPath(surface.view.path)
	}
	if surface.controller.path == "" && strings.TrimSpace(surface.controller.className) != "" {
		if controllerFile := builderRuntimeOpenLiteIdentifierSnakeCase(surface.controller.className); controllerFile != "" {
			surface.controller.path = "lib/controllers/" + controllerFile + ".dart"
		}
	}
	if !builderRuntimeOpenLiteMatchesMutationViewCandidate(content, surface.view) {
		return content
	}
	canonical := ""
	if strings.Contains(content, "class RecordFormPage") || strings.TrimSpace(surface.view.controllerParam) != "" {
		canonical = builderRuntimeOpenLiteCanonicalRelationRichFormPageContent(workspacePath, content)
		if strings.TrimSpace(canonical) != "" {
			canonical = builderRuntimeOpenLiteRewriteFormPageCanonicalToCandidate(canonical, surface.view, surface.controller)
		}
	}
	if strings.TrimSpace(canonical) == "" {
		updated := normalizeBuilderRuntimeOpenLiteCopyHelperReferences(content)
		updated = normalizeBuilderRuntimeOpenLiteDropdownButtonInitialValue(updated)
		if strings.Contains(updated, "HomeController") || strings.Contains(updated, "homeController") {
			updated = normalizeBuilderRuntimeOpenLiteRewriteFormPageRepositoryDependency(workspacePath, updated)
		}
		if !builderRuntimeOpenLiteFormControllerSupportsDateAPI(workspacePath) {
			updated = builderRuntimeRemoveDartMethodBlock(updated, builderRuntimeOpenLiteFormPagePickDateSignaturePattern)
			updated = builderRuntimeOpenLiteFormPageDateTilePattern.ReplaceAllString(updated, "\n        const SizedBox(height: 16),")
			updated = strings.ReplaceAll(updated, "onPressed: _pickDate,", "onPressed: null,")
		}
		return updated
	}
	if builderRuntimeNormalizeSourceForComparison(content) == builderRuntimeNormalizeSourceForComparison(canonical) {
		return content
	}
	return canonical
}

func normalizeBuilderRuntimeOpenLiteListPageContent(workspacePath string, allowFilterFlow bool, needsCollectionCreateEntry bool, content string) string {
	if !builderRuntimeOpenLiteLooksLikeCollectionPageContent(workspacePath, content) {
		return content
	}
	surface := builderRuntimePrimaryCollectionSurfaceCandidate(workspacePath, content)
	candidate := surface.view
	if currentClass, ok := builderRuntimeOpenLiteCollectionViewClassName(content); ok {
		if candidate.className == "" || (currentClass != "RecordListPage" && candidate.className == "RecordListPage") {
			candidate.className = currentClass
		}
		if candidate.path == "" && candidate.className != "" {
			if viewFile := builderRuntimeOpenLiteIdentifierSnakeCase(candidate.className); viewFile != "" {
				candidate.path = "lib/views/" + viewFile + ".dart"
			}
		}
		accepted, _ := builderRuntimeDartConstructorNamedParameters(content, currentClass)
		if controllerParam := builderRuntimeCollectionViewControllerParam(accepted); controllerParam != "" {
			candidate.controllerParam = controllerParam
			if controllerType := builderRuntimeDartFieldTypeByName(content, controllerParam); controllerType != "" {
				candidate.controllerType = controllerType
			}
		}
		if detailCallbackName := builderRuntimeCollectionViewDetailCallbackName(accepted); detailCallbackName != "" {
			candidate.detailCallbackName = detailCallbackName
			if detailCallbackType := builderRuntimeDartFieldTypeByName(content, detailCallbackName); detailCallbackType != "" {
				candidate.detailCallbackType = detailCallbackType
			}
		}
		if createCallbackName := builderRuntimeCollectionViewCreateCallbackName(accepted); createCallbackName != "" {
			candidate.createCallbackName = createCallbackName
		}
	}
	if strings.TrimSpace(surface.controller.className) == "" && strings.TrimSpace(candidate.controllerType) != "" {
		surface.controller.className = strings.TrimSpace(candidate.controllerType)
	}
	if strings.TrimSpace(surface.controller.path) == "" && strings.TrimSpace(candidate.path) != "" {
		surface.controller.path = builderRuntimeControllerPathHintForViewPath(candidate.path)
	}
	if !allowFilterFlow && builderRuntimeTaskOutputLooksLikeUnexpectedListFilter(content) {
		if builderRuntimeOpenLiteMatchesCollectionViewCandidate(content, candidate) {
			canonical := builderRuntimeOpenLiteCanonicalNoFilterListPage(workspacePath)
			if strings.TrimSpace(canonical) != "" {
				canonical = builderRuntimeOpenLiteRewriteListPageCanonicalToCandidate(workspacePath, content, canonical, candidate, surface.controller)
				if strings.TrimSpace(canonical) != "" && builderRuntimeNormalizeSourceForComparison(content) != builderRuntimeNormalizeSourceForComparison(canonical) {
					return canonical
				}
			}
		}
	}
	if builderRuntimeOpenLiteMatchesCollectionViewCandidate(content, candidate) {
		canonical := builderRuntimeOpenLiteCanonicalRelationRichListPageContent(workspacePath, content)
		if strings.TrimSpace(canonical) != "" {
			canonical = builderRuntimeOpenLiteRewriteListPageCanonicalToCandidate(workspacePath, content, canonical, candidate, surface.controller)
			if strings.TrimSpace(canonical) != "" && builderRuntimeNormalizeSourceForComparison(content) != builderRuntimeNormalizeSourceForComparison(canonical) {
				return canonical
			}
		}
	}
	updated := normalizeBuilderRuntimeOpenLiteCopyHelperReferences(content)
	updated = normalizeBuilderRuntimeOpenLiteDropdownButtonInitialValue(updated)
	if builderRuntimeOpenLiteShouldDropTotalCount(workspacePath) {
		updated = strings.ReplaceAll(updated, "openLiteCopy.listCountLabel(visibleRecords.length, records.length)", "openLiteCopy.listCountLabel(visibleRecords.length)")
	}
	updated = normalizeBuilderRuntimeOpenLiteRewriteListPageFilterReferences(workspacePath, updated)
	updated = builderRuntimeOpenLiteDropImportIfUnused(updated, "import '../repositories/record_repository.dart';", "RecordRepository", "recordRepository")
	updated = builderRuntimeOpenLiteDropImportIfUnused(updated, "import '../models/tag.dart';", "Tag ", "Tag)", "List<Tag>", "tag.")
	updated = normalizeBuilderRuntimeOpenLiteListPageCreateCallback(updated)
	return normalizeBuilderRuntimeOpenLiteListPageCreateEntry(needsCollectionCreateEntry, updated)
}

func builderRuntimeOpenLiteCollectionViewClassName(content string) (string, bool) {
	if strings.Contains(content, "class RecordListPage") {
		return "RecordListPage", true
	}
	for _, match := range builderRuntimeNamedDartTypeDeclarationPattern.FindAllStringSubmatch(content, -1) {
		if len(match) < 3 || strings.TrimSpace(match[1]) != "class" {
			continue
		}
		className := strings.TrimSpace(match[2])
		if className == "" {
			continue
		}
		lowerName := strings.ToLower(className)
		if strings.Contains(lowerName, "listpage") || strings.Contains(lowerName, "collectionpage") {
			return className, true
		}
	}
	return "", false
}

func builderRuntimeOpenLiteLooksLikeCollectionPageContent(workspacePath, content string) bool {
	if strings.Contains(content, "listCountLabel(") ||
		strings.Contains(content, "RecordListFilter") ||
		strings.Contains(content, "visibleRecords") ||
		strings.Contains(content, "onOpenRecordDetail") ||
		strings.Contains(content, "onOpenTaskDetail") ||
		strings.Contains(content, "class RecordListPage") {
		return true
	}
	for _, match := range builderRuntimeNamedDartTypeDeclarationPattern.FindAllStringSubmatch(content, -1) {
		if len(match) < 3 || strings.TrimSpace(match[1]) != "class" {
			continue
		}
		className := strings.ToLower(strings.TrimSpace(match[2]))
		if strings.Contains(className, "listpage") || strings.Contains(className, "collectionpage") {
			return true
		}
	}
	if strings.TrimSpace(workspacePath) == "" {
		return false
	}
	view := builderRuntimeOpenLiteCollectionSurfaceRegistry(workspacePath, content).view
	if view.resolutionSource == builderRuntimeOpenLiteSurfaceRegistryResolutionLegacyTemplate {
		return false
	}
	className := strings.TrimSpace(view.resolvedClassName)
	return className != "" && strings.Contains(content, "class "+className)
}

func builderRuntimeOpenLiteMatchesCollectionViewCandidate(content string, candidate builderRuntimeCollectionViewCandidate) bool {
	if strings.Contains(content, "class RecordListPage") {
		return true
	}
	className := strings.TrimSpace(candidate.className)
	if className == "" {
		return false
	}
	return strings.Contains(content, "class "+className)
}

func builderRuntimeOpenLiteRewriteListPageCanonicalToCandidate(workspacePath, content, canonical string, candidate builderRuntimeCollectionViewCandidate, controller builderRuntimeCollectionControllerCandidate) string {
	updated := canonical
	resolvedItemType, ok := builderRuntimeCollectionViewResolvedItemType(workspacePath, candidate)
	if !ok {
		return ""
	}
	className := strings.TrimSpace(candidate.className)
	if className == "" {
		className = "RecordListPage"
	}
	if className != "RecordListPage" {
		updated = strings.ReplaceAll(updated, "RecordListPage", className)
	}
	controllerType := strings.TrimSpace(controller.className)
	if controllerType == "" {
		controllerType = strings.TrimSpace(candidate.controllerType)
	}
	if controllerType == "" {
		controllerType = "RecordListController"
	}
	if controllerType != "RecordListController" {
		updated = strings.ReplaceAll(updated, "RecordListController", controllerType)
	}
	if controllerPath := strings.TrimSpace(controller.path); controllerPath != "" && controllerPath != "lib/controllers/record_list_controller.dart" {
		updated = strings.Replace(updated, "import '../controllers/record_list_controller.dart';", "import '../controllers/"+filepath.Base(controllerPath)+"';", 1)
	}
	controllerParam := strings.TrimSpace(candidate.controllerParam)
	if controllerParam == "" {
		controllerParam = "controller"
	}
	if controllerParam != "controller" {
		updated = strings.ReplaceAll(updated, "required this.controller,", "required this."+controllerParam+",")
		updated = strings.ReplaceAll(updated, "final "+controllerType+" controller;", "final "+controllerType+" "+controllerParam+";")
		updated = strings.ReplaceAll(updated, "animation: controller,", "animation: "+controllerParam+",")
		updated = strings.ReplaceAll(updated, "final records = controller.records;", "final records = "+controllerParam+".records;")
	}
	canonicalRecordType := builderRuntimeOpenLitePrimaryRecordTypeName(workspacePath)
	if canonicalRecordType == "" {
		canonicalRecordType = "TodoItem"
	}
	detailCallbackName := strings.TrimSpace(candidate.detailCallbackName)
	if detailCallbackName == "" {
		detailCallbackName = "onOpenRecordDetail"
	}
	detailCallbackType := strings.TrimSpace(candidate.detailCallbackType)
	if detailCallbackType == "" {
		detailCallbackType = "Future<void> Function(" + resolvedItemType + " record)"
	}
	updated = strings.Replace(updated, "  final Future<void> Function("+canonicalRecordType+" record) onOpenRecordDetail;", "  final "+detailCallbackType+" "+detailCallbackName+";", 1)
	if detailCallbackName != "onOpenRecordDetail" {
		updated = strings.ReplaceAll(updated, "onOpenRecordDetail", detailCallbackName)
	}
	if resolvedItemType != canonicalRecordType {
		updated = strings.Replace(updated, "  final "+canonicalRecordType+" record;", "  final "+resolvedItemType+" record;", 1)
		if modelPath := builderRuntimeModelImportPathForType(workspacePath, resolvedItemType); modelPath != "" && modelPath != "lib/models/record.dart" {
			updated = strings.Replace(updated, "import '../models/record.dart';", "import '../models/"+filepath.Base(modelPath)+"';", 1)
		}
	}
	createCallbackName := strings.TrimSpace(candidate.createCallbackName)
	if createCallbackName == "" {
		createCallbackName = builderRuntimeOpenLiteListPageCreateCallbackName(content)
	}
	if createCallbackName == "" {
		createCallbackName = "onCreateRecord"
	}
	if createCallbackName != "onCreateRecord" {
		updated = strings.ReplaceAll(updated, "onCreateRecord", createCallbackName)
	}
	return updated
}

func builderRuntimeCollectionViewResolvedItemType(workspacePath string, candidate builderRuntimeCollectionViewCandidate) (string, bool) {
	itemType := builderRuntimeCollectionViewItemType(candidate.detailCallbackType)
	switch itemType {
	case "", "Object", "dynamic", "AppRecord":
		itemType = builderRuntimeOpenLitePrimaryRecordTypeName(workspacePath)
		if itemType == "" {
			itemType = "TodoItem"
		}
		return itemType, true
	case "String", "int", "double", "num", "bool", "DateTime", "Duration":
		return "", false
	default:
		return itemType, true
	}
}

func builderRuntimeCollectionViewItemType(detailCallbackType string) string {
	trimmedType := strings.TrimSpace(detailCallbackType)
	if trimmedType == "" {
		return ""
	}
	functionIndex := strings.Index(trimmedType, "Function(")
	if functionIndex < 0 {
		return ""
	}
	arguments := trimmedType[functionIndex+len("Function("):]
	closeIndex := strings.Index(arguments, ")")
	if closeIndex < 0 {
		return ""
	}
	parameter := strings.TrimSpace(arguments[:closeIndex])
	if parameter == "" {
		return ""
	}
	lastSpace := strings.LastIndex(parameter, " ")
	if lastSpace < 0 {
		return ""
	}
	return strings.TrimSpace(strings.TrimSuffix(parameter[:lastSpace], "?"))
}

func normalizeBuilderRuntimeOpenLiteListPageCreateEntry(needsCollectionCreateEntry bool, content string) string {
	if !needsCollectionCreateEntry || strings.TrimSpace(content) == "" || strings.Contains(content, "FloatingActionButton.extended(") {
		return content
	}
	const marker = "      body:"
	createCallbackName := builderRuntimeOpenLiteListPageCreateCallbackName(content)
	labelExpression := "const Text('Create')"
	if strings.Contains(content, "openLiteCopy") || strings.Contains(content, "open_lite_copy.dart") {
		labelExpression = "Text(openLiteCopy.createPrimaryActionLabel)"
	}
	createEntry := "      floatingActionButton: FloatingActionButton.extended(\n        onPressed: " + createCallbackName + ",\n        label: " + labelExpression + ",\n      ),"
	if strings.Contains(content, marker) {
		return strings.Replace(content, marker, createEntry+"\n"+marker, 1)
	}
	return content
}

func normalizeBuilderRuntimeOpenLiteListPageCreateCallback(content string) string {
	updated := content
	createCallbackName := builderRuntimeOpenLiteListPageCreateCallbackName(updated)
	detailCallbackName := builderRuntimeOpenLiteListPageDetailCallbackName(updated)
	if !strings.Contains(updated, "required this."+createCallbackName+",") {
		updated = strings.Replace(updated, "    required this."+detailCallbackName+",", "    required this."+createCallbackName+",\n    required this."+detailCallbackName+",", 1)
		updated = strings.Replace(updated, "required this."+detailCallbackName+"});", "required this."+createCallbackName+", required this."+detailCallbackName+"});", 1)
	}
	if !strings.Contains(updated, "final Future<void> Function() "+createCallbackName+";") {
		updated = strings.Replace(updated, "  final Future<void> Function(AppRecord record) "+detailCallbackName+";", "  final Future<void> Function() "+createCallbackName+";\n  final Future<void> Function(AppRecord record) "+detailCallbackName+";", 1)
		updated = strings.Replace(updated, "  final Future<void> Function(Object record) "+detailCallbackName+";", "  final Future<void> Function() "+createCallbackName+";\n  final Future<void> Function(Object record) "+detailCallbackName+";", 1)
		updated = strings.Replace(updated, "  final Future<void> Function(String taskId) "+detailCallbackName+";", "  final Future<void> Function() "+createCallbackName+";\n  final Future<void> Function(String taskId) "+detailCallbackName+";", 1)
		updated = strings.Replace(updated, "  final Future<void> Function(Task task) "+detailCallbackName+";", "  final Future<void> Function() "+createCallbackName+";\n  final Future<void> Function(Task task) "+detailCallbackName+";", 1)
	}
	return updated
}

func builderRuntimeOpenLiteListPageCreateCallbackName(content string) string {
	if strings.Contains(content, "onCreateTask") {
		return "onCreateTask"
	}
	if strings.Contains(content, "onCreateRecord") {
		return "onCreateRecord"
	}
	if strings.Contains(content, "onOpenTaskDetail") || strings.Contains(content, "TaskCollectionPage") {
		return "onCreateTask"
	}
	return "onCreateRecord"
}

func builderRuntimeOpenLiteListPageDetailCallbackName(content string) string {
	if strings.Contains(content, "onOpenTaskDetail") {
		return "onOpenTaskDetail"
	}
	return "onOpenRecordDetail"
}

func normalizeBuilderRuntimeOpenLiteRewriteListPageFilterReferences(workspacePath, content string) string {
	filterType := builderRuntimeOpenLiteSelectedFilterTypeName(workspacePath)
	if filterType == "" || !strings.Contains(content, "RecordListFilter") {
		return content
	}
	updated := strings.ReplaceAll(content, "final RecordListFilter filter;", "final "+filterType+"? filter;")
	updated = strings.ReplaceAll(updated, "RecordListFilter.inbox", filterType+".inbox")
	updated = strings.ReplaceAll(updated, "RecordListFilter.inProgress", filterType+".inProgress")
	updated = strings.ReplaceAll(updated, "RecordListFilter.done", filterType+".done")
	updated = strings.ReplaceAll(updated, "RecordListFilter.all", "null")
	updated = strings.ReplaceAll(updated, "Key('record-filter-${filter.name}')", "const Key('record-filter-all')")
	return updated
}

func normalizeBuilderRuntimeOpenLiteRewriteFormPageRepositoryDependency(workspacePath, content string) string {
	surface := builderRuntimePrimaryMutationSurfaceCandidate(workspacePath, content)
	repositoryPath := strings.TrimSpace(surface.repository.path)
	if repositoryPath == "" {
		repositoryPath = "lib/repositories/record_repository.dart"
	}
	repositoryType := strings.TrimSpace(surface.repository.repositoryType)
	if repositoryType == "" {
		repositoryType = "RecordRepository"
	}
	repositoryParam := strings.TrimSpace(surface.view.repositoryParam)
	if repositoryParam == "" {
		repositoryParam = builderRuntimeOpenLiteLowerCamelIdentifier(repositoryType)
	}
	if repositoryParam == "" {
		repositoryParam = "recordRepository"
	}
	repositoryImportLine := "import '../repositories/" + filepath.Base(repositoryPath) + "';"
	updated := content
	if strings.Contains(updated, "import '../controllers/home_controller.dart';") {
		if strings.Contains(updated, repositoryImportLine) {
			updated = strings.Replace(updated, "import '../controllers/home_controller.dart';\n", "", 1)
			updated = strings.Replace(updated, "import '../controllers/home_controller.dart';", "", 1)
		} else {
			updated = strings.Replace(updated, "import '../controllers/home_controller.dart';", repositoryImportLine, 1)
		}
	}
	if !strings.Contains(updated, repositoryImportLine) {
		updated = normalizeBuilderRuntimeOpenLiteEnsureRepositoryImport(updated, repositoryPath, strings.TrimSpace(surface.controller.path))
	}
	updated = strings.ReplaceAll(updated, "HomeController", repositoryType)
	updated = strings.ReplaceAll(updated, "homeController", repositoryParam)
	return updated
}

func normalizeBuilderRuntimeOpenLiteEnsureRepositoryImport(content, repositoryPath, controllerPath string) string {
	trimmedRepositoryPath := strings.TrimSpace(repositoryPath)
	if trimmedRepositoryPath == "" {
		trimmedRepositoryPath = "lib/repositories/record_repository.dart"
	}
	importLine := "import '../repositories/" + filepath.Base(trimmedRepositoryPath) + "';\n"
	if strings.Contains(content, strings.TrimSpace(importLine)) {
		return content
	}
	controllerImportLine := ""
	if strings.TrimSpace(controllerPath) != "" {
		controllerImportLine = "import '../controllers/" + filepath.Base(strings.TrimSpace(controllerPath)) + "';\n"
	}
	switch {
	case controllerImportLine != "" && strings.Contains(content, controllerImportLine):
		return strings.Replace(content, controllerImportLine, controllerImportLine+importLine, 1)
	case strings.Contains(content, "import '../template/open_lite_copy.dart';\n"):
		return strings.Replace(content, "import '../template/open_lite_copy.dart';\n", importLine+"import '../template/open_lite_copy.dart';\n", 1)
	default:
		return importLine + content
	}
}

func builderRuntimeOpenLiteFormControllerSupportsDateAPI(workspacePath string) bool {
	controllerContent := builderRuntimeOpenLiteMutationControllerContent(workspacePath)
	if strings.TrimSpace(controllerContent) == "" {
		return false
	}
	for _, markers := range [][]string{
		{"selectedDate", "setDate("},
		{"selectedDueOn", "setDueOn("},
		{"selectedCountedOn", "setCountedOn("},
	} {
		matched := true
		for _, marker := range markers {
			if !strings.Contains(controllerContent, marker) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func builderRuntimeOpenLiteCanonicalRelationRichHomeControllerContent(workspacePath, content string) string {
	dm, ok := builderRuntimeOpenLiteRelationRichDomainModel(workspacePath, content)
	if !ok {
		return ""
	}
	result, ok := emitter.EmitOverview(dm)
	if !ok {
		return ""
	}
	return result.HomeControllerContent
}

func builderRuntimeOpenLiteCanonicalRelationRichHomePageContent(workspacePath, content string) string {
	dm, ok := builderRuntimeOpenLiteRelationRichDomainModel(workspacePath, content)
	if !ok {
		return ""
	}
	result, ok := emitter.EmitOverview(dm)
	if !ok {
		return ""
	}
	return result.HomePageContent
}

func builderRuntimeOpenLiteCanonicalRelationRichDetailPageContent(workspacePath, content string) string {
	dm, ok := builderRuntimeOpenLiteRelationRichDomainModel(workspacePath, content)
	if !ok {
		return ""
	}
	result, ok := emitter.EmitInspection(dm)
	if !ok {
		return ""
	}
	return result.DetailPageContent
}

func builderRuntimeOpenLiteCanonicalRelationRichListControllerContent(workspacePath, content string) string {
	dm, ok := builderRuntimeOpenLiteRelationRichDomainModel(workspacePath, content)
	if !ok {
		return ""
	}
	result, ok := emitter.EmitCollection(dm)
	if !ok {
		return ""
	}
	return result.ListControllerContent
}

func builderRuntimeOpenLiteCanonicalRelationRichFormPageContent(workspacePath, content string) string {
	dm, ok := builderRuntimeOpenLiteRelationRichDomainModel(workspacePath, content)
	if !ok {
		return ""
	}
	result, ok := emitter.EmitMutation(dm)
	if !ok {
		return ""
	}
	return result.FormPageContent
}

func builderRuntimeOpenLiteCanonicalRelationRichListPageContent(workspacePath, content string) string {
	dm, ok := builderRuntimeOpenLiteRelationRichDomainModel(workspacePath, content)
	if !ok {
		return ""
	}
	result, ok := emitter.EmitCollection(dm)
	if !ok {
		return ""
	}
	return result.ListPageContent
}

func builderRuntimeOpenLiteRelationRichDomainModel(workspacePath, content string) (appprepare.DomainModel, bool) {
	profile := builderRuntimeRelationRichModelProfileForWorkspace(workspacePath)
	if profile == "" {
		return appprepare.DomainModel{}, false
	}
	title := builderRuntimeOpenLiteResolveAppTitle(workspacePath, content)
	switch profile {
	case builderRuntimeRelationRichProjectTaskTagProfile:
		return appprepare.DomainModel{
			TemplateID: "flutter-open-lite",
			DomainCopy: appprepare.DomainCopy{Title: title},
			Entities: []appprepare.DataEntity{
				{EntityID: "entity-project", Name: "Project"},
				{EntityID: "entity-task", Name: "Task"},
				{EntityID: "entity-tag", Name: "Tag"},
			},
		}, true
	case builderRuntimeRelationRichInventorySheetLineProfile:
		return appprepare.DomainModel{
			TemplateID: "flutter-open-lite",
			DomainCopy: appprepare.DomainCopy{Title: title},
			Entities: []appprepare.DataEntity{
				{EntityID: "entity-inventory-sheet", Name: "InventorySheet"},
				{EntityID: "entity-line-item", Name: "LineItem"},
				{EntityID: "entity-sku", Name: "Sku"},
			},
		}, true
	default:
		return appprepare.DomainModel{}, false
	}
}

func builderRuntimeOpenLiteResolveAppTitle(workspacePath, content string) string {
	for _, candidate := range []string{content, builderRuntimeOpenLiteReadCopyContent(workspacePath)} {
		if title := builderRuntimeOpenLiteExtractAppTitle(candidate); title != "" {
			return title
		}
	}
	return "App"
}

func builderRuntimeOpenLiteReadCopyContent(workspacePath string) string {
	content, err := builderRuntimeSemanticFileContent(workspacePath, nil, "lib/template/open_lite_copy.dart")
	if err != nil {
		return ""
	}
	return content
}

func builderRuntimeOpenLiteExtractAppTitle(content string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	match := builderRuntimeOpenLiteAppTitleGetterPattern.FindStringSubmatch(content)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}
