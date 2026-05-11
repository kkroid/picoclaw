package adapter

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode"

	appprepare "github.com/sipeed/oneappfactory/pkg/appfactory/prepare"
)

var builderRuntimeOpenLiteTotalCountArgPattern = regexp.MustCompile(`,\s*int\s+totalCount\b`)
var builderRuntimeOpenLiteTotalCountPlaceholderPattern = regexp.MustCompile(`\s*/\s*\$(?:\{)?totalCount(?:\})?`)
var builderRuntimeOpenLiteParameterizedGetterPattern = regexp.MustCompile(`\bget\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
var builderRuntimeOpenLiteSlashSeparatedGetterPattern = regexp.MustCompile(`(?m)^(\s*[A-Za-z0-9_<>,?]+\s+get\s+)[A-Za-z_][A-Za-z0-9_]*\s*/\s*([A-Za-z_][A-Za-z0-9_]*)(\s*(?:=>|\{))`)
var builderRuntimeOpenLiteTruncatedViewAllActionGetterPattern = regexp.MustCompile(`(?m)^(\s*)String\s+get\s+viewAllAction\s*$`)
var builderRuntimeOpenLiteMisspelledViewAllActionGetterPattern = regexp.MustCompile(`(?m)^(\s*)String\s+get\s+viewAllActionlar\s*=>.*$`)
var builderRuntimeOpenLiteAppTitleGetterPattern = regexp.MustCompile(`(?m)^\s*String\s+get\s+appTitle\s*=>\s*'([^']*)';\s*$`)
var builderRuntimeRecordDetailPageUpdatedAtTilePattern = regexp.MustCompile(`(?s)_InfoTile\(\s*label:\s*openLiteCopy\.detailDateLabel,\s*value:\s*'\$\{[A-Za-z_][A-Za-z0-9_]*\.updatedAt\.year\}-\$\{[A-Za-z_][A-Za-z0-9_]*\.updatedAt\.month\.toString\(\)\.padLeft\(2, '0'\)\}-\$\{[A-Za-z_][A-Za-z0-9_]*\.updatedAt\.day\.toString\(\)\.padLeft\(2, '0'\)\}',\s*\),`)
var builderRuntimeOpenLiteFormPagePickDateSignaturePattern = regexp.MustCompile(`(?m)^[ \t]*Future<void>\s+_pickDate\(\)\s+async\s*\{`)
var builderRuntimeOpenLiteFormPageDateTilePattern = regexp.MustCompile(`(?s)\n\s*const SizedBox\(height:\s*16\),\n\s*ListTile\(\s*contentPadding:.*?onPressed:\s*_pickDate,\s*icon:\s*const Icon\(Icons\.calendar_today\),\s*\),\s*\),\n\s*const SizedBox\(height:\s*16\),`)
var builderRuntimeOpenLiteSelectedFilterFieldPattern = regexp.MustCompile(`(?m)^\s*([A-Z][A-Za-z0-9_]*)\?\s+_selectedFilter\b`)
var builderRuntimeOpenLiteSelectedFilterGetterPattern = regexp.MustCompile(`(?m)^\s*([A-Z][A-Za-z0-9_]*)\?\s+get\s+selectedFilter\b`)
var builderRuntimeOpenLiteRecordFormControllerClassPattern = regexp.MustCompile(`(?m)^\s*class\s+RecordFormController\b`)
var builderRuntimeOpenLiteMainSeedColorPattern = regexp.MustCompile(`seedColor:\s*(?:const\s+)?Color\(([^)]+)\)`)
var builderRuntimeOpenLiteDartHexColorLiteralPattern = regexp.MustCompile(`(?i)^0x[0-9a-f]{8}$`)
var builderRuntimeOpenLiteMainListControllerDeclarationPattern = regexp.MustCompile(`(?m)^[ \t]*final\s+listController\s*=\s*RecordList\s*Controller\(\s*repository:\s*repository\s*\);[ \t]*(?:\n|$)`)
var builderRuntimeOpenLiteMainGenericListControllerDeclarationPattern = regexp.MustCompile(`(?m)^[ \t]*final\s+listController\s*=\s*[A-Z][A-Za-z0-9_]*(?:List|Collection)Controller\(\s*[A-Za-z_][A-Za-z0-9_]*\s*:\s*repository\s*\);[ \t]*(?:\n|$)`)
var builderRuntimeOpenLiteMainOnCreateRecordCallbackPattern = regexp.MustCompile(`(?s)onCreateRecord:\s*\(\)\s*async\s*\{.*?\n\s*\},`)
var builderRuntimeOpenLiteMainOnOpenRecordDetailCallbackPattern = regexp.MustCompile(`(?s)onOpenRecordDetail:\s*\(record\)\s*async\s*\{.*?\n\s*\},`)
var builderRuntimeOpenLiteMainNavigatorPushDoubleOpenParenPattern = regexp.MustCompile(`Navigator\.of\(context\)\.push(<[^>\n]+>)?\s*\(\(`)
var builderRuntimeOpenLiteMainNavigatorPushPattern = regexp.MustCompile(`Navigator\.of\(context\)\.push(<[^>\n]+>)?\(`)
var builderRuntimeOpenLiteCreateStatePrivateReturnPattern = regexp.MustCompile(`(?m)^([ \t]*)_([A-Z][A-Za-z0-9_]*)State\s+createState\(\)\s*=>\s*_[A-Z][A-Za-z0-9_]*State\(\);\s*$`)
var builderRuntimeOpenLiteMainOnDeleteNullReturnPattern = regexp.MustCompile(`(?s)(onDelete:\s*\([^)]*\)\s*async\s*\{.*?)(\n[ \t]*)return null;(\n[ \t]*\},)`)
var builderRuntimeOpenLiteMainFormControllerFieldPattern = regexp.MustCompile(`(?m)^[ \t]*late\s+final\s+([A-Z][A-Za-z0-9_]*FormController)\s+formController;\s*$`)
var builderRuntimeOpenLiteOnOpenRecordDetailParamPattern = regexp.MustCompile(`(?m)^(\s*required this\.onOpenRecordDetail,\s*)$`)
var builderRuntimeOpenLiteOnOpenRecordDetailFieldPattern = regexp.MustCompile(`(?m)^(\s*final\s+Future<void>\s+Function\([A-Z][A-Za-z0-9_]*\s+record\)\s+onOpenRecordDetail;\s*)$`)
var builderRuntimeOpenLiteStatusLabelMethodPattern = regexp.MustCompile(`(?m)^\s*String\s+statusLabel\([^)]*\)\s*\{`)
var builderRuntimeOpenLiteStatuslessCopyGetterPattern = regexp.MustCompile(`(?m)^\s*String\s+get\s+(?:detailStatusLabel|inboxFilterLabel|inProgressFilterLabel|doneFilterLabel)\s*=>.*(?:\n|$)`)
var builderRuntimeOpenLiteTitlelessCopyGetterPattern = regexp.MustCompile(`(?m)^\s*String\s+get\s+(?:titleFieldLabel|titleFieldRequiredError)\s*=>.*(?:\n|$)`)
var builderRuntimeOpenLiteCategorylessCopyGetterPattern = regexp.MustCompile(`(?m)^\s*String\s+get\s+(?:categoryFieldLabel|detailCategoryLabel)\s*=>.*(?:\n|$)`)
var builderRuntimeOpenLiteRecordImportPattern = regexp.MustCompile(`(?m)^import '../models/record\.dart';\n?`)

type builderRuntimeOpenLiteSurfaceRegistryConstructorContract struct {
	controllerParam     string
	controllerType      string
	repositoryParam     string
	recordParam         string
	initialParam        string
	detailCallbackName  string
	detailCallbackType  string
	createCallbackName  string
	viewAllCallbackName string
	editCallbackName    string
	deleteCallbackName  string
	requiredParams      []string
}

type builderRuntimeOpenLiteSurfaceRegistryCapabilityFlags struct {
	supportsCreate  bool
	supportsDetail  bool
	supportsInit    bool
	supportsRefresh bool
	supportsUpdate  bool
	supportsEdit    bool
	hasCategory     bool
	hasStatus       bool
	hasTitleField   bool
	hasNoteField    bool
}

type builderRuntimeOpenLiteSurfaceRegistryFieldSemantics struct {
	identifierField           string
	primaryTextField          string
	primaryTextFieldType      string
	secondaryTextField        string
	statusField               string
	statusEnumType            string
	statusCopyLabelMethodName string
	timeField                 string
	numericFields             []builderRuntimeOpenLiteNumericFieldSemantics
	booleanFields             []builderRuntimeOpenLiteBooleanFieldSemantics
	noteField                 string
}

type builderRuntimeOpenLiteNumericFieldSemantics struct {
	name      string
	fieldType string
	role      string
}

type builderRuntimeOpenLiteBooleanFieldSemantics struct {
	name string
	role string
}

type builderRuntimeOpenLiteSummaryModelSnapshot struct {
	className string
	fields    []builderRuntimeOpenLiteDeclaredField
}

type builderRuntimeOpenLiteSurfaceRegistryFallbackMode string

const (
	builderRuntimeOpenLiteSurfaceRegistryFallbackExplicitSurface    builderRuntimeOpenLiteSurfaceRegistryFallbackMode = "explicit_surface"
	builderRuntimeOpenLiteSurfaceRegistryFallbackWorkspaceCandidate builderRuntimeOpenLiteSurfaceRegistryFallbackMode = "workspace_candidate"
	builderRuntimeOpenLiteSurfaceRegistryFallbackLikelyPath         builderRuntimeOpenLiteSurfaceRegistryFallbackMode = "likely_path"
	builderRuntimeOpenLiteSurfaceRegistryFallbackLegacyTemplate     builderRuntimeOpenLiteSurfaceRegistryFallbackMode = "legacy_template"
	builderRuntimeOpenLiteSurfaceRegistryFallbackUnresolved         builderRuntimeOpenLiteSurfaceRegistryFallbackMode = "unresolved"
)

type builderRuntimeOpenLiteSurfaceRegistryResolutionSource string

const (
	builderRuntimeOpenLiteSurfaceRegistryResolutionExplicitSurface    builderRuntimeOpenLiteSurfaceRegistryResolutionSource = "explicit_surface"
	builderRuntimeOpenLiteSurfaceRegistryResolutionWorkspaceCandidate builderRuntimeOpenLiteSurfaceRegistryResolutionSource = "workspace_candidate"
	builderRuntimeOpenLiteSurfaceRegistryResolutionLikelyPath         builderRuntimeOpenLiteSurfaceRegistryResolutionSource = "likely_path"
	builderRuntimeOpenLiteSurfaceRegistryResolutionLegacyTemplate     builderRuntimeOpenLiteSurfaceRegistryResolutionSource = "legacy_template"
	builderRuntimeOpenLiteSurfaceRegistryResolutionUnresolved         builderRuntimeOpenLiteSurfaceRegistryResolutionSource = "unresolved"
)

type builderRuntimeOpenLiteSurfaceRegistryEntry struct {
	registryKey          string
	bindingRef           string
	surfaceRef           string
	pathClass            string
	templateRole         string
	resolvedPath         string
	resolvedClassName    string
	modelImportPath      string
	modelType            string
	repositoryImportPath string
	repositoryType       string
	controllerImportPath string
	constructorContract  builderRuntimeOpenLiteSurfaceRegistryConstructorContract
	capabilityFlags      builderRuntimeOpenLiteSurfaceRegistryCapabilityFlags
	fieldSemantics       builderRuntimeOpenLiteSurfaceRegistryFieldSemantics
	fallbackMode         builderRuntimeOpenLiteSurfaceRegistryFallbackMode
	resolutionSource     builderRuntimeOpenLiteSurfaceRegistryResolutionSource
}

type builderRuntimeOpenLiteCollectionSurfaceRegistrySnapshot struct {
	controller builderRuntimeOpenLiteSurfaceRegistryEntry
	view       builderRuntimeOpenLiteSurfaceRegistryEntry
}

type builderRuntimeOpenLiteOverviewSurfaceRegistrySnapshot struct {
	controller builderRuntimeOpenLiteSurfaceRegistryEntry
	view       builderRuntimeOpenLiteSurfaceRegistryEntry
}

type builderRuntimeOpenLiteDetailSurfaceRegistrySnapshot struct {
	view                builderRuntimeOpenLiteSurfaceRegistryEntry
	mutation            builderRuntimeOpenLiteSurfaceRegistryEntry
	controller          builderRuntimeOpenLiteSurfaceRegistryEntry
	detailCandidate     builderRuntimeDetailViewCandidate
	mutationCandidate   builderRuntimeMutationViewCandidate
	controllerCandidate builderRuntimeCollectionControllerCandidate
}

type builderRuntimeOpenLiteMutationSurfaceRegistrySnapshot struct {
	view       builderRuntimeOpenLiteSurfaceRegistryEntry
	controller builderRuntimeOpenLiteSurfaceRegistryEntry
}

func builderRuntimeOpenLiteRegistryKey(bindingRef, surfaceRef, pathClass, templateRole string) string {
	parts := []string{strings.TrimSpace(bindingRef), strings.TrimSpace(surfaceRef), strings.TrimSpace(pathClass), strings.TrimSpace(templateRole)}
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			filtered = append(filtered, part)
		}
	}
	return strings.Join(filtered, ":")
}

func builderRuntimeOpenLiteSurfaceRegistryResolution(workspacePath, path string) (builderRuntimeOpenLiteSurfaceRegistryFallbackMode, builderRuntimeOpenLiteSurfaceRegistryResolutionSource) {
	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == "" {
		return builderRuntimeOpenLiteSurfaceRegistryFallbackUnresolved, builderRuntimeOpenLiteSurfaceRegistryResolutionUnresolved
	}
	if builderRuntimeWorkspaceHasSemanticFile(workspacePath, trimmedPath) {
		return builderRuntimeOpenLiteSurfaceRegistryFallbackWorkspaceCandidate, builderRuntimeOpenLiteSurfaceRegistryResolutionWorkspaceCandidate
	}
	return builderRuntimeOpenLiteSurfaceRegistryFallbackLegacyTemplate, builderRuntimeOpenLiteSurfaceRegistryResolutionLegacyTemplate
}

func builderRuntimeOpenLiteSurfaceRegistryResolutionWithTemplateFallback(workspacePath, path string) (builderRuntimeOpenLiteSurfaceRegistryFallbackMode, builderRuntimeOpenLiteSurfaceRegistryResolutionSource) {
	fallbackMode, resolutionSource := builderRuntimeOpenLiteSurfaceRegistryResolution(workspacePath, path)
	if fallbackMode == builderRuntimeOpenLiteSurfaceRegistryFallbackUnresolved {
		return builderRuntimeOpenLiteSurfaceRegistryFallbackLegacyTemplate, builderRuntimeOpenLiteSurfaceRegistryResolutionLegacyTemplate
	}
	return fallbackMode, resolutionSource
}

func builderRuntimeOpenLiteRelativeImport(fromPath, toPath, fallback string) string {
	trimmedFrom := strings.TrimSpace(fromPath)
	trimmedTo := strings.TrimSpace(toPath)
	if trimmedFrom == "" || trimmedTo == "" {
		return fallback
	}
	relPath, err := filepath.Rel(filepath.Dir(filepath.FromSlash(trimmedFrom)), filepath.FromSlash(trimmedTo))
	if err != nil {
		return fallback
	}
	relPath = filepath.ToSlash(relPath)
	if strings.TrimSpace(relPath) == "" {
		return fallback
	}
	return relPath
}

func builderRuntimeOpenLiteCollectionRegistryModel(workspacePath string, candidate builderRuntimeCollectionViewCandidate) (string, string) {
	modelType := builderRuntimeOpenLitePrimaryRecordTypeName(workspacePath)
	modelPath := builderRuntimeOpenLitePrimaryRecordModelPath(workspacePath)
	if resolvedItemType, ok := builderRuntimeCollectionViewResolvedItemType(workspacePath, candidate); ok && strings.TrimSpace(resolvedItemType) != "" {
		modelType = strings.TrimSpace(resolvedItemType)
		if itemModelPath := builderRuntimeModelImportPathForType(workspacePath, modelType); itemModelPath != "" {
			modelPath = itemModelPath
		}
	}
	if strings.TrimSpace(modelType) == "" {
		modelType = "TodoItem"
	}
	if strings.TrimSpace(modelPath) == "" {
		modelPath = "lib/models/record.dart"
	}
	return modelType, modelPath
}

func builderRuntimeOpenLiteCollectionSurfaceRegistry(workspacePath string, hints ...string) builderRuntimeOpenLiteCollectionSurfaceRegistrySnapshot {
	surface := builderRuntimePrimaryCollectionSurfaceCandidate(workspacePath, hints...)
	modelType, modelPath := builderRuntimeOpenLiteCollectionRegistryModel(workspacePath, surface.view)
	fieldSemantics := builderRuntimeOpenLiteCollectionFieldSemantics(workspacePath)
	repositoryPath := strings.TrimSpace(surface.repository.path)
	if repositoryPath == "" {
		repositoryPath = "lib/repositories/record_repository.dart"
	}
	repositoryType := strings.TrimSpace(surface.repository.repositoryType)
	if repositoryType == "" {
		repositoryType = "RecordRepository"
	}
	controllerPath := strings.TrimSpace(surface.controller.path)
	if controllerPath == "" {
		controllerPath = "lib/controllers/record_list_controller.dart"
	}
	controllerClass := strings.TrimSpace(surface.controller.className)
	if controllerClass == "" {
		controllerClass = "RecordListController"
	}
	controllerFallbackMode, controllerResolutionSource := builderRuntimeOpenLiteSurfaceRegistryResolutionWithTemplateFallback(workspacePath, controllerPath)
	controllerRepositoryParam := strings.TrimSpace(surface.controller.repositoryParam)
	if controllerRepositoryParam == "" {
		controllerRepositoryParam = "repository"
	}
	viewPath := strings.TrimSpace(surface.view.path)
	if viewPath == "" {
		viewPath = "lib/views/record_list_page.dart"
	}
	viewClass := strings.TrimSpace(surface.view.className)
	if viewClass == "" {
		viewClass = "RecordListPage"
	}
	viewFallbackMode, viewResolutionSource := builderRuntimeOpenLiteSurfaceRegistryResolutionWithTemplateFallback(workspacePath, viewPath)
	controllerParam := strings.TrimSpace(surface.view.controllerParam)
	if controllerParam == "" {
		controllerParam = "controller"
	}
	controllerType := strings.TrimSpace(surface.view.controllerType)
	if controllerType == "" {
		controllerType = controllerClass
	}
	detailCallbackName := strings.TrimSpace(surface.view.detailCallbackName)
	if detailCallbackName == "" {
		detailCallbackName = "onOpenRecordDetail"
	}
	detailCallbackType := strings.TrimSpace(surface.view.detailCallbackType)
	if detailCallbackType == "" {
		detailCallbackType = "Future<void> Function(" + modelType + " record)"
	}
	createCallbackName := strings.TrimSpace(surface.view.createCallbackName)
	if createCallbackName == "" {
		createCallbackName = "onCreateRecord"
	}
	bindingRef := builderRuntimeCollectionSurfaceRef
	surfaceRef := builderRuntimeCollectionSurfaceRef
	return builderRuntimeOpenLiteCollectionSurfaceRegistrySnapshot{
		controller: builderRuntimeOpenLiteSurfaceRegistryEntry{
			registryKey:          builderRuntimeOpenLiteRegistryKey(bindingRef, surfaceRef, "controller", "collection_controller"),
			bindingRef:           bindingRef,
			surfaceRef:           surfaceRef,
			pathClass:            "controller",
			templateRole:         "collection_controller",
			resolvedPath:         controllerPath,
			resolvedClassName:    controllerClass,
			modelImportPath:      builderRuntimeOpenLiteRelativeImport(controllerPath, modelPath, "../models/record.dart"),
			modelType:            modelType,
			repositoryImportPath: builderRuntimeOpenLiteRelativeImport(controllerPath, repositoryPath, "../repositories/record_repository.dart"),
			repositoryType:       repositoryType,
			constructorContract: builderRuntimeOpenLiteSurfaceRegistryConstructorContract{
				repositoryParam: controllerRepositoryParam,
			},
			capabilityFlags: builderRuntimeOpenLiteSurfaceRegistryCapabilityFlags{
				supportsInit:    surface.controller.supportsInit,
				supportsRefresh: surface.controller.supportsRefresh,
				supportsUpdate:  surface.controller.supportsUpdate,
			},
			fieldSemantics:   fieldSemantics,
			fallbackMode:     controllerFallbackMode,
			resolutionSource: controllerResolutionSource,
		},
		view: builderRuntimeOpenLiteSurfaceRegistryEntry{
			registryKey:          builderRuntimeOpenLiteRegistryKey(bindingRef, surfaceRef, "view", "collection_view"),
			bindingRef:           bindingRef,
			surfaceRef:           surfaceRef,
			pathClass:            "view",
			templateRole:         "collection_view",
			resolvedPath:         viewPath,
			resolvedClassName:    viewClass,
			controllerImportPath: builderRuntimeOpenLiteRelativeImport(viewPath, controllerPath, "../controllers/record_list_controller.dart"),
			modelImportPath:      builderRuntimeOpenLiteRelativeImport(viewPath, modelPath, "../models/record.dart"),
			modelType:            modelType,
			constructorContract: builderRuntimeOpenLiteSurfaceRegistryConstructorContract{
				controllerParam:    controllerParam,
				controllerType:     controllerType,
				detailCallbackName: detailCallbackName,
				detailCallbackType: detailCallbackType,
				createCallbackName: createCallbackName,
			},
			capabilityFlags: builderRuntimeOpenLiteSurfaceRegistryCapabilityFlags{
				supportsCreate: createCallbackName != "",
				supportsDetail: detailCallbackName != "",
				hasCategory:    !builderRuntimeOpenLiteShouldDropCategoryField(workspacePath),
				hasStatus:      builderRuntimeOpenLiteRecordModelHasStatus(workspacePath),
			},
			fieldSemantics:   fieldSemantics,
			fallbackMode:     viewFallbackMode,
			resolutionSource: viewResolutionSource,
		},
	}
}

func builderRuntimeOpenLiteOverviewSurfaceRegistry(workspacePath string, hints ...string) builderRuntimeOpenLiteOverviewSurfaceRegistrySnapshot {
	surface := builderRuntimePrimaryOverviewSurfaceCandidate(workspacePath, hints...)
	repositoryPath := strings.TrimSpace(surface.repository.path)
	if repositoryPath == "" {
		repositoryPath = "lib/repositories/record_repository.dart"
	}
	repositoryType := strings.TrimSpace(surface.repository.repositoryType)
	if repositoryType == "" {
		repositoryType = "RecordRepository"
	}
	controllerPath := strings.TrimSpace(surface.controller.path)
	if controllerPath == "" {
		controllerPath = "lib/controllers/home_controller.dart"
	}
	controllerClass := strings.TrimSpace(surface.controller.className)
	if controllerClass == "" {
		controllerClass = "HomeController"
	}
	controllerFallbackMode, controllerResolutionSource := builderRuntimeOpenLiteSurfaceRegistryResolutionWithTemplateFallback(workspacePath, controllerPath)
	controllerRepositoryParam := strings.TrimSpace(surface.controller.repositoryParam)
	if controllerRepositoryParam == "" {
		controllerRepositoryParam = "repository"
	}
	viewPath := strings.TrimSpace(surface.view.path)
	if viewPath == "" {
		viewPath = "lib/views/home_page.dart"
	}
	viewClass := strings.TrimSpace(surface.view.className)
	if viewClass == "" {
		viewClass = "HomePage"
	}
	viewFallbackMode, viewResolutionSource := builderRuntimeOpenLiteSurfaceRegistryResolutionWithTemplateFallback(workspacePath, viewPath)
	controllerParam := strings.TrimSpace(surface.view.controllerParam)
	if controllerParam == "" {
		controllerParam = "controller"
	}
	controllerType := strings.TrimSpace(surface.view.controllerType)
	if controllerType == "" {
		controllerType = controllerClass
	}
	createCallbackName := strings.TrimSpace(surface.view.createCallbackName)
	if createCallbackName == "" {
		createCallbackName = "onCreateRecord"
	}
	viewAllCallbackName := strings.TrimSpace(surface.view.viewAllCallbackName)
	if viewAllCallbackName == "" {
		viewAllCallbackName = "onViewAllRecords"
	}
	bindingRef := builderRuntimeOverviewSurfaceRef
	surfaceRef := builderRuntimeOverviewSurfaceRef
	return builderRuntimeOpenLiteOverviewSurfaceRegistrySnapshot{
		controller: builderRuntimeOpenLiteSurfaceRegistryEntry{
			registryKey:          builderRuntimeOpenLiteRegistryKey(bindingRef, surfaceRef, "controller", "overview_controller"),
			bindingRef:           bindingRef,
			surfaceRef:           surfaceRef,
			pathClass:            "controller",
			templateRole:         "overview_controller",
			resolvedPath:         controllerPath,
			resolvedClassName:    controllerClass,
			repositoryImportPath: builderRuntimeOpenLiteRelativeImport(controllerPath, repositoryPath, "../repositories/record_repository.dart"),
			repositoryType:       repositoryType,
			constructorContract: builderRuntimeOpenLiteSurfaceRegistryConstructorContract{
				repositoryParam: controllerRepositoryParam,
			},
			fallbackMode:     controllerFallbackMode,
			resolutionSource: controllerResolutionSource,
		},
		view: builderRuntimeOpenLiteSurfaceRegistryEntry{
			registryKey:          builderRuntimeOpenLiteRegistryKey(bindingRef, surfaceRef, "view", "overview_view"),
			bindingRef:           bindingRef,
			surfaceRef:           surfaceRef,
			pathClass:            "view",
			templateRole:         "overview_view",
			resolvedPath:         viewPath,
			resolvedClassName:    viewClass,
			controllerImportPath: builderRuntimeOpenLiteRelativeImport(viewPath, controllerPath, "../controllers/home_controller.dart"),
			constructorContract: builderRuntimeOpenLiteSurfaceRegistryConstructorContract{
				controllerParam:     controllerParam,
				controllerType:      controllerType,
				createCallbackName:  createCallbackName,
				viewAllCallbackName: viewAllCallbackName,
			},
			fallbackMode:     viewFallbackMode,
			resolutionSource: viewResolutionSource,
		},
	}
}

func builderRuntimeOpenLiteDetailSurfaceRegistry(workspacePath string, hints ...string) builderRuntimeOpenLiteDetailSurfaceRegistrySnapshot {
	detailSurface := builderRuntimePrimaryDetailSurfaceCandidate(workspacePath, hints...)
	collectionSurface := builderRuntimePrimaryCollectionSurfaceCandidate(workspacePath, hints...)
	repositoryPath := strings.TrimSpace(collectionSurface.repository.path)
	if repositoryPath == "" {
		repositoryPath = "lib/repositories/record_repository.dart"
	}
	repositoryType := strings.TrimSpace(collectionSurface.repository.repositoryType)
	if repositoryType == "" {
		repositoryType = "RecordRepository"
	}
	viewPath := strings.TrimSpace(detailSurface.view.path)
	viewClass := strings.TrimSpace(detailSurface.view.className)
	mutationPath := strings.TrimSpace(detailSurface.mutation.view.path)
	mutationClass := strings.TrimSpace(detailSurface.mutation.view.className)
	controllerPath := strings.TrimSpace(detailSurface.controller.path)
	controllerClass := strings.TrimSpace(detailSurface.controller.className)
	viewFallbackMode, viewResolutionSource := builderRuntimeOpenLiteSurfaceRegistryResolution(workspacePath, viewPath)
	mutationFallbackMode, mutationResolutionSource := builderRuntimeOpenLiteSurfaceRegistryResolution(workspacePath, mutationPath)
	controllerFallbackMode, controllerResolutionSource := builderRuntimeOpenLiteSurfaceRegistryResolution(workspacePath, controllerPath)
	bindingRef := builderRuntimeInspectionSurfaceRef
	surfaceRef := builderRuntimeInspectionSurfaceRef
	return builderRuntimeOpenLiteDetailSurfaceRegistrySnapshot{
		view: builderRuntimeOpenLiteSurfaceRegistryEntry{
			registryKey:       builderRuntimeOpenLiteRegistryKey(bindingRef, surfaceRef, "view", "inspection_view"),
			bindingRef:        bindingRef,
			surfaceRef:        surfaceRef,
			pathClass:         "view",
			templateRole:      "inspection_view",
			resolvedPath:      viewPath,
			resolvedClassName: viewClass,
			constructorContract: builderRuntimeOpenLiteSurfaceRegistryConstructorContract{
				recordParam:        detailSurface.view.recordParam,
				editCallbackName:   detailSurface.view.editCallbackName,
				deleteCallbackName: detailSurface.view.deleteCallbackName,
				requiredParams:     append([]string(nil), detailSurface.view.requiredParams...),
			},
			fallbackMode:     viewFallbackMode,
			resolutionSource: viewResolutionSource,
		},
		mutation: builderRuntimeOpenLiteSurfaceRegistryEntry{
			registryKey:       builderRuntimeOpenLiteRegistryKey(bindingRef, surfaceRef, "view", "mutation_view"),
			bindingRef:        bindingRef,
			surfaceRef:        surfaceRef,
			pathClass:         "view",
			templateRole:      "mutation_view",
			resolvedPath:      mutationPath,
			resolvedClassName: mutationClass,
			constructorContract: builderRuntimeOpenLiteSurfaceRegistryConstructorContract{
				repositoryParam: detailSurface.mutation.view.repositoryParam,
				initialParam:    detailSurface.mutation.view.initialParam,
			},
			fallbackMode:     mutationFallbackMode,
			resolutionSource: mutationResolutionSource,
		},
		controller: builderRuntimeOpenLiteSurfaceRegistryEntry{
			registryKey:          builderRuntimeOpenLiteRegistryKey(bindingRef, surfaceRef, "controller", "collection_controller"),
			bindingRef:           bindingRef,
			surfaceRef:           surfaceRef,
			pathClass:            "controller",
			templateRole:         "collection_controller",
			resolvedPath:         controllerPath,
			resolvedClassName:    controllerClass,
			repositoryImportPath: builderRuntimeOpenLiteRelativeImport(controllerPath, repositoryPath, "../repositories/record_repository.dart"),
			repositoryType:       repositoryType,
			constructorContract: builderRuntimeOpenLiteSurfaceRegistryConstructorContract{
				repositoryParam: detailSurface.controller.repositoryParam,
			},
			fallbackMode:     controllerFallbackMode,
			resolutionSource: controllerResolutionSource,
		},
		detailCandidate:     detailSurface.view,
		mutationCandidate:   detailSurface.mutation.view,
		controllerCandidate: detailSurface.controller,
	}
}

func builderRuntimeOpenLiteMutationSurfaceRegistry(workspacePath string, hints ...string) builderRuntimeOpenLiteMutationSurfaceRegistrySnapshot {
	surface := builderRuntimePrimaryMutationSurfaceCandidate(workspacePath, hints...)
	repository := builderRuntimePrimaryRepositoryCandidate(workspacePath, hints...)
	repositoryPath := strings.TrimSpace(repository.path)
	if repositoryPath == "" {
		repositoryPath = "lib/repositories/record_repository.dart"
	}
	repositoryType := strings.TrimSpace(repository.repositoryType)
	if repositoryType == "" {
		repositoryType = "RecordRepository"
	}
	viewPath := strings.TrimSpace(surface.view.path)
	if viewPath == "" {
		viewPath = "lib/views/record_form_page.dart"
	}
	viewClass := strings.TrimSpace(surface.view.className)
	if viewClass == "" {
		viewClass = "RecordFormPage"
	}
	viewFallbackMode, viewResolutionSource := builderRuntimeOpenLiteSurfaceRegistryResolutionWithTemplateFallback(workspacePath, viewPath)
	controllerPath := strings.TrimSpace(surface.controller.path)
	if controllerPath == "" {
		controllerPath = builderRuntimeOpenLiteMutationControllerPath(workspacePath)
	}
	controllerClass := strings.TrimSpace(surface.controller.className)
	controllerContent := builderRuntimeOpenLiteMutationControllerContent(workspacePath)
	if controllerClass == "" {
		controllerClass = builderRuntimeOpenLiteFirstNamedDartClass(controllerContent)
	}
	if controllerClass == "" {
		controllerClass = "RecordFormController"
	}
	controllerFallbackMode, controllerResolutionSource := builderRuntimeOpenLiteSurfaceRegistryResolutionWithTemplateFallback(workspacePath, controllerPath)
	hasTitleField := strings.Contains(controllerContent, "titleController")
	hasNoteField := strings.Contains(controllerContent, "noteController")
	hasStatusFlow := builderRuntimeOpenLiteRecordModelHasStatus(workspacePath)
	hasExplicitMutationView := strings.TrimSpace(surface.view.path) != ""
	bindingRef := builderRuntimeMutationSurfaceRef
	surfaceRef := builderRuntimeMutationSurfaceRef
	return builderRuntimeOpenLiteMutationSurfaceRegistrySnapshot{
		view: builderRuntimeOpenLiteSurfaceRegistryEntry{
			registryKey:       builderRuntimeOpenLiteRegistryKey(bindingRef, surfaceRef, "view", "mutation_view"),
			bindingRef:        bindingRef,
			surfaceRef:        surfaceRef,
			pathClass:         "view",
			templateRole:      "mutation_view",
			resolvedPath:      viewPath,
			resolvedClassName: viewClass,
			constructorContract: builderRuntimeOpenLiteSurfaceRegistryConstructorContract{
				controllerParam: surface.view.controllerParam,
				controllerType:  surface.view.controllerType,
				repositoryParam: surface.view.repositoryParam,
				initialParam:    surface.view.initialParam,
			},
			capabilityFlags: builderRuntimeOpenLiteSurfaceRegistryCapabilityFlags{
				hasStatus:    hasStatusFlow,
				supportsEdit: hasTitleField && (!hasExplicitMutationView || surface.view.initialParam != ""),
			},
			fallbackMode:     viewFallbackMode,
			resolutionSource: viewResolutionSource,
		},
		controller: builderRuntimeOpenLiteSurfaceRegistryEntry{
			registryKey:          builderRuntimeOpenLiteRegistryKey(bindingRef, surfaceRef, "controller", "mutation_controller"),
			bindingRef:           bindingRef,
			surfaceRef:           surfaceRef,
			pathClass:            "controller",
			templateRole:         "mutation_controller",
			resolvedPath:         controllerPath,
			resolvedClassName:    controllerClass,
			repositoryImportPath: builderRuntimeOpenLiteRelativeImport(controllerPath, repositoryPath, "../repositories/record_repository.dart"),
			repositoryType:       repositoryType,
			constructorContract: builderRuntimeOpenLiteSurfaceRegistryConstructorContract{
				repositoryParam: surface.controller.repositoryParam,
			},
			capabilityFlags: builderRuntimeOpenLiteSurfaceRegistryCapabilityFlags{
				hasStatus:     hasStatusFlow,
				hasTitleField: hasTitleField,
				hasNoteField:  hasNoteField,
			},
			fallbackMode:     controllerFallbackMode,
			resolutionSource: controllerResolutionSource,
		},
	}
}

func builderRuntimeOpenLiteFirstNamedDartClass(content string) string {
	matches := builderRuntimeNamedDartTypeDeclarationPattern.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) < 3 || strings.TrimSpace(match[1]) != "class" {
			continue
		}
		className := strings.TrimSpace(match[2])
		if className != "" {
			return className
		}
	}
	return ""
}

func appendBuilderRuntimeOpenLiteTemplateCopyPromptGuidance(builder *strings.Builder, forbiddenSchemaTokens []string) {
	builder.WriteString("For current template copy files under lib/template/*.dart, expose only copy helpers justified by the current record.dart, dashboard_summary.dart, and screen flows in context. Remove default seed helper methods whose required fields or workflow concepts are absent from the current models.\n")
	if slices.Contains(forbiddenSchemaTokens, "totalCount") {
		builder.WriteString("Current summary model does not define totalCount. Do not emit summaryCountLabel, recentRecordsCountLabel, listCountLabel, or any current template copy helper signature that requires totalCount. Never emit the identifier totalCount anywhere in the current template copy file. If count copy is still needed, keep count-only helpers such as summaryCountLabel(int count), recentRecordsCountLabel(int count), and listCountLabel(int visibleCount).\n")
	}
	if slices.Contains(forbiddenSchemaTokens, "RecordStatus") || slices.Contains(forbiddenSchemaTokens, ".status") {
		builder.WriteString("Current record model does not define a status workflow. Do not emit RecordStatus-based labels, filter wording, or statusLabel helpers in the current template copy file.\n")
	}
}

func appendBuilderRuntimeOpenLiteSchemaRemapGuidance(builder *strings.Builder) {
	builder.WriteString("When remapping flutter-open-lite away from the default generic record schema, do not leave stale references to title, category, status, updatedAt, totalCount, inboxCount, inProgressCount, or doneCount unless the final record or summary model still defines them. If record.dart or dashboard_summary.dart changes, update every dependent controller, repository, widget, and test in the current file context to the final schema before finishing the patch.\n")
	builder.WriteString("If the final schema no longer defines status- or updatedAt-style fields, remove or rewrite every selectedStatus state, RecordStatus enum branch, status filter chip, status badge, and repository sort/order call that still depends on those generic fields. Replace them with domain-appropriate behavior derived from the final schema such as recordedAt ordering, weight history display, note text, or other actual domain fields.\n")
	builder.WriteString("Unless the final schema explicitly keeps status-based workflow buckets, do not reference RecordStatus, RecordListFilter, openLiteCopy.statusLabel, openLiteCopy.inboxFilterLabel, openLiteCopy.inProgressFilterLabel, openLiteCopy.doneFilterLabel, summary.inboxCount, summary.inProgressCount, or summary.doneCount in the final patch.\n")
}

func appendBuilderRuntimeOpenLiteWeightRecordTemplateCopyFailureGuidance(builder *strings.Builder) {
	builder.WriteString("For current template copy files, keep domain copy focused on weight records and remove any status label helpers or workflow-bucket wording that depends on RecordStatus.\n")
}

func normalizeBuilderRuntimeTemplateCopyContent(workspacePath, content string) string {
	return normalizeBuilderRuntimeOpenLiteCopyContent(workspacePath, content)
}

func normalizeBuilderRuntimeOpenLiteCopyContent(workspacePath, content string) string {
	if strings.TrimSpace(content) == "" {
		return content
	}
	if canonical := builderRuntimeOpenLiteCanonicalRelationRichCopyContent(workspacePath, content); strings.TrimSpace(canonical) != "" {
		if builderRuntimeNormalizeSourceForComparison(content) != builderRuntimeNormalizeSourceForComparison(canonical) {
			return canonical
		}
		return content
	}
	updated := normalizeBuilderRuntimeOpenLiteEscapedNewlineGetterNames(content)
	updated = builderRuntimeOpenLiteMisspelledViewAllActionGetterPattern.ReplaceAllString(updated, "${1}String get viewAllActionLabel => '查看任务列表';")
	updated = builderRuntimeOpenLiteSlashSeparatedGetterPattern.ReplaceAllString(updated, "$1$2$3")
	updated = builderRuntimeOpenLiteTruncatedViewAllActionGetterPattern.ReplaceAllString(updated, "${1}String get viewAllActionLabel => '查看全部';")
	updated = builderRuntimeOpenLiteParameterizedGetterPattern.ReplaceAllString(updated, "$1(")
	if strings.Contains(updated, "totalCount") && builderRuntimeOpenLiteShouldDropTotalCount(workspacePath) {
		lines := strings.Split(updated, "\n")
		activeReplacement := ""
		braceDepth := 0
		for index, line := range lines {
			switch {
			case strings.Contains(line, "summaryCountLabel(") && strings.Contains(line, "totalCount"):
				lines[index] = normalizeBuilderRuntimeOpenLiteCopyLine(line, "count")
				activeReplacement, braceDepth = builderRuntimeOpenLiteCopyMethodNormalizationState(lines[index], "count")
				continue
			case strings.Contains(line, "recentRecordsCountLabel(") && strings.Contains(line, "totalCount"):
				lines[index] = normalizeBuilderRuntimeOpenLiteCopyLine(line, "count")
				activeReplacement, braceDepth = builderRuntimeOpenLiteCopyMethodNormalizationState(lines[index], "count")
				continue
			case strings.Contains(line, "listCountLabel(") && strings.Contains(line, "totalCount"):
				lines[index] = normalizeBuilderRuntimeOpenLiteCopyLine(line, "visibleCount")
				activeReplacement, braceDepth = builderRuntimeOpenLiteCopyMethodNormalizationState(lines[index], "visibleCount")
				continue
			}
			if activeReplacement == "" {
				continue
			}
			lines[index] = normalizeBuilderRuntimeOpenLiteCopyLine(line, activeReplacement)
			braceDepth += strings.Count(lines[index], "{")
			braceDepth -= strings.Count(lines[index], "}")
			if braceDepth <= 0 {
				activeReplacement = ""
				braceDepth = 0
			}
		}
		updated = strings.Join(lines, "\n")
		updated = builderRuntimeOpenLiteTotalCountArgPattern.ReplaceAllString(updated, "")
		updated = builderRuntimeOpenLiteTotalCountPlaceholderPattern.ReplaceAllString(updated, "")
		updated = strings.ReplaceAll(updated, "totalCount", "count")
	}
	if builderRuntimeOpenLiteShouldDropStatusWorkflow(workspacePath) {
		updated = builderRuntimeRemoveDartMethodBlock(updated, builderRuntimeOpenLiteStatusLabelMethodPattern)
		updated = builderRuntimeOpenLiteStatuslessCopyGetterPattern.ReplaceAllString(updated, "")
		if !strings.Contains(updated, "RecordStatus") {
			updated = builderRuntimeOpenLiteRecordImportPattern.ReplaceAllString(updated, "")
		}
	}
	if builderRuntimeOpenLiteShouldDropTitleField(workspacePath) {
		updated = builderRuntimeOpenLiteTitlelessCopyGetterPattern.ReplaceAllString(updated, "")
	}
	if builderRuntimeOpenLiteShouldDropCategoryField(workspacePath) {
		updated = builderRuntimeOpenLiteCategorylessCopyGetterPattern.ReplaceAllString(updated, "")
	}
	updated = builderRuntimeOpenLiteRetargetCopyStatusTypeToCurrentModel(workspacePath, updated)
	return updated
}

func builderRuntimeOpenLiteRetargetCopyStatusTypeToCurrentModel(workspacePath, content string) string {
	if strings.TrimSpace(workspacePath) == "" || strings.TrimSpace(content) == "" {
		return content
	}
	fieldSemantics := builderRuntimeOpenLiteCollectionSurfaceRegistry(workspacePath).view.fieldSemantics
	statusEnumType := strings.TrimSpace(fieldSemantics.statusEnumType)
	if statusEnumType == "" {
		return content
	}
	updated := strings.ReplaceAll(content, "RecordStatus", statusEnumType)
	statusCopyLabelMethodName := strings.TrimSpace(fieldSemantics.statusCopyLabelMethodName)
	if statusCopyLabelMethodName != "" && statusCopyLabelMethodName != "statusLabel" {
		updated = regexp.MustCompile(`\bstatusLabel\s*\(`).ReplaceAllString(updated, statusCopyLabelMethodName+"(")
	}
	updated = builderRuntimeOpenLiteRetargetCopyStatusEnumMembers(workspacePath, statusEnumType, updated)
	modelPath := builderRuntimeOpenLitePrimaryRecordModelPath(workspacePath)
	if strings.TrimSpace(modelPath) == "" {
		return updated
	}
	modelImportPath := builderRuntimeOpenLiteRelativeImport("lib/template/open_lite_copy.dart", modelPath, "../models/record.dart")
	if strings.TrimSpace(modelImportPath) == "" {
		return updated
	}
	targetImport := "import '" + modelImportPath + "';"
	if strings.Contains(updated, "import '../models/record.dart';") {
		updated = strings.ReplaceAll(updated, "import '../models/record.dart';", targetImport)
	} else if strings.Contains(updated, "statusLabel(") && !strings.Contains(updated, targetImport) {
		updated = targetImport + "\n\n" + strings.TrimLeft(updated, "\n")
	}
	return updated
}

func builderRuntimeOpenLiteRetargetCopyStatusEnumMembers(workspacePath, statusEnumType, content string) string {
	trimmedWorkspacePath := strings.TrimSpace(workspacePath)
	trimmedStatusEnumType := strings.TrimSpace(statusEnumType)
	if trimmedWorkspacePath == "" || trimmedStatusEnumType == "" || strings.TrimSpace(content) == "" {
		return content
	}
	enumMembers := builderRuntimeOpenLiteStatusEnumMembers(trimmedWorkspacePath, trimmedStatusEnumType)
	if len(enumMembers) == 0 {
		return content
	}
	updated := content
	updated = strings.ReplaceAll(updated, trimmedStatusEnumType+".inbox", trimmedStatusEnumType+"."+enumMembers[0])
	updated = strings.ReplaceAll(updated, trimmedStatusEnumType+".todo", trimmedStatusEnumType+"."+enumMembers[0])
	if len(enumMembers) >= 2 {
		updated = strings.ReplaceAll(updated, trimmedStatusEnumType+".inProgress", trimmedStatusEnumType+"."+enumMembers[1])
		updated = strings.ReplaceAll(updated, trimmedStatusEnumType+".doing", trimmedStatusEnumType+"."+enumMembers[1])
	}
	if len(enumMembers) >= 3 {
		updated = strings.ReplaceAll(updated, trimmedStatusEnumType+".done", trimmedStatusEnumType+"."+enumMembers[2])
	}
	return updated
}

func builderRuntimeOpenLiteStatusEnumMembers(workspacePath, statusEnumType string) []string {
	recordContent := builderRuntimeOpenLitePrimaryRecordModelContent(workspacePath)
	trimmedStatusEnumType := strings.TrimSpace(statusEnumType)
	if strings.TrimSpace(recordContent) == "" || trimmedStatusEnumType == "" {
		return nil
	}
	pattern := regexp.MustCompile(`(?s)enum\s+` + regexp.QuoteMeta(trimmedStatusEnumType) + `\s*\{([^}]*)\}`)
	match := pattern.FindStringSubmatch(recordContent)
	if len(match) < 2 {
		return nil
	}
	parts := strings.Split(match[1], ",")
	if len(parts) == 0 {
		return nil
	}
	members := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	identifierPattern := regexp.MustCompile(`^[a-z][A-Za-z0-9_]*$`)
	for _, part := range parts {
		candidate := strings.TrimSpace(part)
		if candidate == "" || !identifierPattern.MatchString(candidate) {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		members = append(members, candidate)
	}
	return members
}

func normalizeBuilderRuntimeOpenLiteMainContent(workspacePath string, needsCollectionCreateEntry bool, content string) string {
	if strings.TrimSpace(content) == "" {
		return content
	}
	updated := strings.ReplaceAll(content, "runrunApp(", "runApp(")
	updated = normalizeBuilderRuntimeOpenLiteMainHomePageConstructorCalls(workspacePath, updated)
	appClassName := builderRuntimeRunAppWidgetClassName(updated)
	if appClassName == "" {
		return updated
	}
	repositoryType := builderRuntimeOpenLiteRecordRepositoryTypeName(workspacePath)
	if repositoryType == "" {
		repositoryType = "RecordRepository"
	}
	concreteRepositoryType := builderRuntimeOpenLiteConcreteRecordRepositoryTypeName(workspacePath)
	if concreteRepositoryType == "" {
		concreteRepositoryType = "HiveRecordRepository"
	}
	updated = normalizeBuilderRuntimeOpenLiteMainLocalImports(updated)
	updated = normalizeBuilderRuntimeOpenLiteMainRepositoryImports(workspacePath, updated)
	updated = normalizeBuilderRuntimeOpenLiteMainUnusedRecordImport(workspacePath, updated)
	updated = normalizeBuilderRuntimeOpenLiteMainCollectionControllerVariable(workspacePath, updated)
	updated = normalizeBuilderRuntimeOpenLiteMainCollectionControllerImports(workspacePath, updated)
	updated = normalizeBuilderRuntimeOpenLiteMainCollectionViewImports(workspacePath, updated)
	updated = normalizeBuilderRuntimeOpenLiteMainOverviewControllerImports(workspacePath, updated)
	updated = normalizeBuilderRuntimeOpenLiteMainOverviewViewImports(workspacePath, updated)
	runAppCall := "runApp(const " + appClassName + "());"
	if strings.Contains(updated, runAppCall) {
		updated = strings.Replace(updated, runAppCall, "runApp("+appClassName+"(repository: repository));", 1)
	}
	if strings.Contains(updated, "await Hive.initFlutter();") && !strings.Contains(updated, "await repository.init();") {
		repositoryInit := "  final repository = " + concreteRepositoryType + "();\n  await repository.init();"
		updated = strings.Replace(updated, "  await Hive.initFlutter();", repositoryInit, 1)
	}
	oldConstructor := "  const " + appClassName + "({super.key});"
	newConstructor := "  const " + appClassName + "({super.key, required this.repository});"
	if !builderRuntimeDartConstructorAcceptsNamedParameter(updated, appClassName, "repository") && strings.Contains(updated, oldConstructor) {
		updated = strings.Replace(updated, oldConstructor, newConstructor, 1)
	}
	fieldDeclaration := "final " + repositoryType + " repository;"
	if !strings.Contains(updated, fieldDeclaration) {
		updated = builderRuntimeInsertFieldIntoStatelessWidget(updated, appClassName, fieldDeclaration)
	}
	updated = strings.ReplaceAll(updated, "repository: "+concreteRepositoryType+"()", "repository: repository")
	updated = strings.ReplaceAll(updated, "recordRepository: "+concreteRepositoryType+"()", "recordRepository: repository")
	updated = normalizeBuilderRuntimeOpenLiteMainDetailCallback(workspacePath, updated)
	updated = normalizeBuilderRuntimeOpenLiteMainCollectionCreateFlow(workspacePath, needsCollectionCreateEntry, updated, appClassName)
	updated = normalizeBuilderRuntimeOpenLiteMainRecordFormCallbacks(workspacePath, needsCollectionCreateEntry, updated, appClassName)
	updated = normalizeBuilderRuntimeOpenLiteMainDetailViewImports(workspacePath, updated)
	updated = normalizeBuilderRuntimeOpenLiteMainCollectionControllerImports(workspacePath, updated)
	updated = normalizeBuilderRuntimeOpenLiteMainCollectionViewImports(workspacePath, updated)
	updated = normalizeBuilderRuntimeOpenLiteMainMutationViewImports(workspacePath, updated)
	updated = normalizeBuilderRuntimeOpenLiteMainNavigatorPushCalls(updated, appClassName)
	updated = normalizeBuilderRuntimeOpenLiteMainTodoStatefulDrift(updated)
	updated = normalizeBuilderRuntimeOpenLiteMainThemeTokens(updated)
	updated = normalizeBuilderRuntimeOpenLiteMainSeedColorTokens(updated)
	if !strings.Contains(updated, "Hive.") {
		updated = strings.ReplaceAll(updated, "import 'package:hive_flutter/hive_flutter.dart';\n", "")
	}
	return updated
}

func normalizeBuilderRuntimeOpenLiteMainTodoStatefulDrift(content string) string {
	if strings.TrimSpace(content) == "" {
		return content
	}
	updated := strings.ReplaceAll(content, "    super.disposed();", "    super.dispose();")
	updated = builderRuntimeOpenLiteCreateStatePrivateReturnPattern.ReplaceAllString(updated, "${1}State<${2}> createState() => _${2}State();")
	updated = builderRuntimeOpenLiteMainOnDeleteNullReturnPattern.ReplaceAllString(updated, "$1$3")
	if strings.Count(updated, "listController") == 1 {
		listControllerClass := ""
		if matches := regexp.MustCompile(`(?m)^[ \t]*final\s+listController\s*=\s*([A-Z][A-Za-z0-9_]*(?:List|Collection)Controller)\(\s*[A-Za-z_][A-Za-z0-9_]*\s*:\s*repository\s*\);[ \t]*(?:\n|$)`).FindStringSubmatch(updated); len(matches) >= 2 {
			listControllerClass = strings.TrimSpace(matches[1])
		}
		updated = builderRuntimeOpenLiteMainListControllerDeclarationPattern.ReplaceAllString(updated, "")
		updated = builderRuntimeOpenLiteMainGenericListControllerDeclarationPattern.ReplaceAllString(updated, "")
		if listControllerClass != "" {
			updated = builderRuntimeOpenLiteDropControllerImportIfUnused(updated, listControllerClass)
		}
	}
	if matches := builderRuntimeOpenLiteMainFormControllerFieldPattern.FindStringSubmatch(updated); len(matches) >= 2 && strings.Count(updated, "formController") == 3 {
		controllerClass := strings.TrimSpace(matches[1])
		if controllerClass != "" {
			updated = builderRuntimeOpenLiteMainFormControllerFieldPattern.ReplaceAllString(updated, "")
			assignmentPattern := regexp.MustCompile(`(?m)^[ \t]*formController\s*=\s*` + regexp.QuoteMeta(controllerClass) + `\(\s*[A-Za-z_][A-Za-z0-9_]*\s*:\s*widget\.[A-Za-z_][A-Za-z0-9_]*\s*\);\s*$`)
			updated = assignmentPattern.ReplaceAllString(updated, "")
			disposePattern := regexp.MustCompile(`(?m)^[ \t]*formController\.dispose\(\);\s*$`)
			updated = disposePattern.ReplaceAllString(updated, "")
			updated = builderRuntimeOpenLiteDropControllerImportIfUnused(updated, controllerClass)
		}
	}
	if !strings.Contains(updated, "RecordFormController") {
		updated = strings.ReplaceAll(updated, "import 'controllers/record_form_controller.dart';\n", "")
	}
	return updated
}

func normalizeBuilderRuntimeOpenLiteMainHomePageConstructorCalls(workspacePath, content string) string {
	if strings.TrimSpace(content) == "" {
		return content
	}
	candidates, err := builderRuntimeMainViewConstructorCandidates(workspacePath, content)
	if err != nil || len(candidates) == 0 {
		return content
	}
	updated := content
	usedNoop := false
	usedNoopRecord := false
	for _, candidate := range candidates {
		viewContent, readErr := builderRuntimeSemanticFileContent(workspacePath, nil, candidate.path)
		if readErr != nil || strings.TrimSpace(viewContent) == "" {
			continue
		}
		_, required := builderRuntimeDartConstructorNamedParameters(viewContent, candidate.className)
		if len(required) == 0 {
			continue
		}
		argsList := builderRuntimeConstructedClassArgs(updated, candidate.className)
		for _, args := range argsList {
			provided := builderRuntimeTopLevelNamedArgumentSet(args)
			missing := make([]string, 0, len(required))
			for name := range required {
				if _, ok := provided[name]; !ok {
					missing = append(missing, name)
				}
			}
			if len(missing) == 0 {
				continue
			}
			sort.Strings(missing)
			injectedArgs := strings.TrimRight(args, " \t\n")
			if strings.TrimSpace(injectedArgs) != "" && !strings.HasSuffix(strings.TrimSpace(injectedArgs), ",") {
				injectedArgs += ","
			}
			for _, name := range missing {
				expression, requiresNoop, requiresNoopRecord := builderRuntimeOpenLiteMainHomePageFallbackArgument(workspacePath, candidate, name, updated)
				if strings.TrimSpace(expression) == "" {
					continue
				}
				usedNoop = usedNoop || requiresNoop
				usedNoopRecord = usedNoopRecord || requiresNoopRecord
				indent := builderRuntimeOpenLiteMainArgumentIndent(args)
				injectedArgs += "\n" + indent + name + ": " + expression + ","
			}
			if strings.Contains(args, "\n") {
				injectedArgs += "\n"
			}
			updated = strings.Replace(updated, candidate.className+"("+args+")", candidate.className+"("+injectedArgs+")", 1)
		}
	}
	if usedNoop {
		updated = builderRuntimeOpenLiteEnsureTopLevelHelper(updated, "Future<void> _noop() async {}")
	}
	if usedNoopRecord {
		updated = builderRuntimeOpenLiteEnsureTopLevelHelper(updated, "Future<void> _noopRecord(Object record) async {}")
	}
	return updated
}

func builderRuntimeOpenLiteMainHomePageFallbackArgument(workspacePath string, candidate builderRuntimeViewConstructorCandidate, name, content string) (expression string, requiresNoop bool, requiresNoopRecord bool) {
	trimmedName := strings.TrimSpace(name)
	overviewRegistry := builderRuntimeOpenLiteOverviewSurfaceRegistry(workspacePath, candidate.path, candidate.className, trimmedName)
	controllerParam := strings.TrimSpace(overviewRegistry.view.constructorContract.controllerParam)
	if controllerParam == "" {
		controllerParam = "controller"
	}
	createCallbackName := strings.TrimSpace(overviewRegistry.view.constructorContract.createCallbackName)
	if createCallbackName == "" {
		createCallbackName = "onCreateRecord"
	}
	viewAllCallbackName := strings.TrimSpace(overviewRegistry.view.constructorContract.viewAllCallbackName)
	if viewAllCallbackName == "" {
		viewAllCallbackName = "onViewAllRecords"
	}
	switch trimmedName {
	case controllerParam:
		if strings.Contains(content, "_homeController") {
			return "_homeController", false, false
		}
		repositoryExpr := builderRuntimeOpenLiteMainRepositoryValueExpression(content, overviewRegistry.controller.repositoryType)
		if overviewRegistry.controller.resolvedClassName != "" && overviewRegistry.controller.constructorContract.repositoryParam != "" && repositoryExpr != "" {
			return overviewRegistry.controller.resolvedClassName + "(" + overviewRegistry.controller.constructorContract.repositoryParam + ": " + repositoryExpr + ")", false, false
		}
		if strings.Contains(content, "HomeController(repository: repository)") {
			return "HomeController(repository: repository)", false, false
		}
		return "Object()", false, false
	case createCallbackName:
		if strings.Contains(content, "_navigateToCreateRecord") {
			return "_navigateToCreateRecord", false, false
		}
		if strings.Contains(content, "_openCreateRecord") {
			return "_openCreateRecord", false, false
		}
		if suffix := strings.TrimSpace(strings.TrimPrefix(trimmedName, "on")); suffix != "" {
			if helperRef := "_open" + suffix; strings.Contains(content, helperRef) {
				return helperRef, false, false
			}
			if helperRef := "_navigateTo" + suffix; strings.Contains(content, helperRef) {
				return helperRef, false, false
			}
		}
		return "_noop", true, false
	case viewAllCallbackName:
		if strings.Contains(content, "_navigateToViewAllRecords") {
			return "_navigateToViewAllRecords", false, false
		}
		if strings.Contains(content, "_openViewAllRecords") {
			return "_openViewAllRecords", false, false
		}
		if strings.Contains(content, "_openTaskList") {
			return "_openTaskList", false, false
		}
		if suffix := strings.TrimSpace(strings.TrimPrefix(trimmedName, "on")); suffix != "" {
			if helperRef := "_open" + suffix; strings.Contains(content, helperRef) {
				return helperRef, false, false
			}
			if helperRef := "_navigateTo" + suffix; strings.Contains(content, helperRef) {
				return helperRef, false, false
			}
		}
		return "_noop", true, false
	case "onOpenRecordDetail", "onOpenTaskDetail":
		if strings.Contains(content, "_navigateToRecordDetail") {
			return "_navigateToRecordDetail", false, false
		}
		if strings.Contains(content, "_openTaskDetail") {
			return "_openTaskDetail", false, false
		}
		if navigateSuffix := strings.TrimSpace(strings.TrimPrefix(trimmedName, "onOpen")); navigateSuffix != "" {
			if helperRef := "_navigateTo" + navigateSuffix; strings.Contains(content, helperRef) {
				return helperRef, false, false
			}
		}
		return "_noopRecord", false, true
	default:
		if strings.HasPrefix(trimmedName, "onOpen") && strings.Contains(strings.ToLower(trimmedName), "detail") {
			if helperName := builderRuntimeOpenLiteLowerCamelIdentifier(strings.TrimPrefix(trimmedName, "on")); helperName != "" {
				helperRef := "_" + helperName
				if strings.Contains(content, helperRef) {
					return helperRef, false, false
				}
			}
			if navigateSuffix := strings.TrimSpace(strings.TrimPrefix(trimmedName, "onOpen")); navigateSuffix != "" {
				if helperRef := "_navigateTo" + navigateSuffix; strings.Contains(content, helperRef) {
					return helperRef, false, false
				}
			}
			return "_noopRecord", false, true
		}
		if strings.HasPrefix(strings.TrimSpace(name), "on") {
			return "_noop", true, false
		}
		return "", false, false
	}
}

func builderRuntimeOpenLiteMainRepositoryValueExpression(content, repositoryType string) string {
	trimmedContent := strings.TrimSpace(content)
	if trimmedContent == "" {
		return ""
	}
	if strings.Contains(trimmedContent, "widget.repository") {
		return "widget.repository"
	}
	if strings.Contains(trimmedContent, "required this.repository") || strings.Contains(trimmedContent, "final repository =") || strings.Contains(trimmedContent, " repository =") {
		return "repository"
	}
	trimmedRepositoryType := strings.TrimSpace(repositoryType)
	if trimmedRepositoryType != "" && strings.Contains(trimmedContent, "final "+trimmedRepositoryType+" repository;") {
		return "repository"
	}
	return ""
}

func builderRuntimeOpenLiteMainArgumentIndent(args string) string {
	if strings.Contains(args, "\n") {
		lines := strings.Split(args, "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		}
	}
	return "        "
}

func builderRuntimeOpenLiteEnsureTopLevelHelper(content, helper string) string {
	trimmedHelper := strings.TrimSpace(helper)
	if trimmedHelper == "" || strings.Contains(content, trimmedHelper) {
		return content
	}
	for _, marker := range []string{"\nFuture<void> main", "\nvoid main", "\nclass "} {
		if strings.Contains(content, marker) {
			return strings.Replace(content, marker, "\n"+trimmedHelper+"\n"+marker, 1)
		}
	}
	return content + "\n" + trimmedHelper + "\n"
}

func normalizeBuilderRuntimeOpenLiteMainThemeTokens(content string) string {
	if strings.TrimSpace(content) == "" {
		return content
	}
	lines := strings.Split(content, "\n")
	seenUseMaterial3 := make(map[string]struct{}, 2)
	normalized := make([]string, 0, len(lines))
	for _, line := range lines {
		updatedLine := line
		trimmed := strings.TrimSpace(updatedLine)
		if strings.Contains(trimmed, "useMaterial async:") {
			trimmed = strings.Replace(trimmed, "useMaterial async:", "useMaterial3:", 1)
			updatedLine = strings.Replace(updatedLine, strings.TrimSpace(updatedLine), trimmed, 1)
		}
		trimmed = strings.TrimSpace(updatedLine)
		if strings.HasPrefix(trimmed, "useMaterial3:") {
			if _, exists := seenUseMaterial3[trimmed]; exists {
				continue
			}
			seenUseMaterial3[trimmed] = struct{}{}
		}
		normalized = append(normalized, updatedLine)
	}
	return strings.Join(normalized, "\n")
}

func normalizeBuilderRuntimeOpenLiteMainSeedColorTokens(content string) string {
	if strings.TrimSpace(content) == "" {
		return content
	}
	return builderRuntimeOpenLiteMainSeedColorPattern.ReplaceAllStringFunc(content, func(match string) string {
		submatches := builderRuntimeOpenLiteMainSeedColorPattern.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		if builderRuntimeOpenLiteDartHexColorLiteralPattern.MatchString(strings.TrimSpace(submatches[1])) {
			return match
		}
		return "seedColor: const Color(0xFF1565C0)"
	})
}

func normalizeBuilderRuntimeOpenLiteMainLocalImports(content string) string {
	if strings.TrimSpace(content) == "" {
		return content
	}
	lines := strings.Split(content, "\n")
	seenImports := make(map[string]struct{}, len(lines))
	normalized := make([]string, 0, len(lines))
	for _, line := range lines {
		updatedLine := line
		match := dartLocalImportPattern.FindStringSubmatch(line)
		if len(match) >= 2 {
			ref := strings.TrimSpace(match[1])
			if strings.HasPrefix(ref, ".") {
				normalizedRef := ref
				for strings.HasPrefix(normalizedRef, "../") {
					normalizedRef = strings.TrimPrefix(normalizedRef, "../")
				}
				for strings.HasPrefix(normalizedRef, "./") {
					normalizedRef = strings.TrimPrefix(normalizedRef, "./")
				}
				if normalizedRef != "" && normalizedRef != ref {
					updatedLine = strings.Replace(updatedLine, ref, normalizedRef, 1)
				}
			}
		}
		trimmed := strings.TrimSpace(updatedLine)
		if strings.HasPrefix(trimmed, "import ") || strings.HasPrefix(trimmed, "export ") || strings.HasPrefix(trimmed, "part ") {
			if _, exists := seenImports[trimmed]; exists {
				continue
			}
			seenImports[trimmed] = struct{}{}
		}
		normalized = append(normalized, updatedLine)
	}
	return strings.Join(normalized, "\n")
}

func normalizeBuilderRuntimeOpenLiteMainRepositoryImports(workspacePath, content string) string {
	if strings.TrimSpace(content) == "" {
		return content
	}
	repository := builderRuntimePrimaryRepositoryCandidate(workspacePath, content)
	if repository.path == "" {
		return content
	}
	if !strings.Contains(content, repository.repositoryType) && !strings.Contains(content, repository.concreteType) && !strings.Contains(content, repository.inMemoryType) {
		return content
	}
	expectedImport := "import '" + strings.TrimPrefix(repository.path, "lib/") + "';"
	lines := strings.Split(content, "\n")
	normalized := make([]string, 0, len(lines))
	seenExpected := false
	replacedUnexpected := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == expectedImport:
			if seenExpected {
				continue
			}
			seenExpected = true
			normalized = append(normalized, expectedImport)
		case strings.HasPrefix(trimmed, "import 'repositories/") && strings.HasSuffix(trimmed, ".dart';"):
			if !seenExpected {
				normalized = append(normalized, expectedImport)
				seenExpected = true
			}
			replacedUnexpected = true
		default:
			normalized = append(normalized, line)
		}
	}
	updated := strings.Join(normalized, "\n")
	if seenExpected || replacedUnexpected {
		return updated
	}
	insertionIndex := -1
	updatedLines := strings.Split(updated, "\n")
	for index, line := range updatedLines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "import ") {
			insertionIndex = index + 1
			continue
		}
		if insertionIndex >= 0 && trimmed != "" {
			break
		}
	}
	if insertionIndex < 0 {
		return expectedImport + "\n" + updated
	}
	updatedLines = append(updatedLines[:insertionIndex], append([]string{expectedImport}, updatedLines[insertionIndex:]...)...)
	return strings.Join(updatedLines, "\n")
}

func normalizeBuilderRuntimeOpenLiteMainUnusedRecordImport(workspacePath, content string) string {
	const recordImport = "import 'models/record.dart';\n"
	if strings.TrimSpace(content) == "" || !strings.Contains(content, recordImport) {
		return content
	}
	recordType := builderRuntimeOpenLitePrimaryRecordTypeName(workspacePath)
	if recordType == "" {
		recordType = "AppRecord"
	}
	statusType := "RecordStatus"
	if recordContent := builderRuntimeOpenLitePrimaryRecordModelContent(workspacePath); strings.TrimSpace(recordContent) != "" {
		if resolvedStatusType := builderRuntimeDeclaredFieldTypes(recordContent)["status"]; resolvedStatusType != "" {
			statusType = resolvedStatusType
		}
	}
	withoutImport := strings.Replace(content, recordImport, "", 1)
	for _, symbol := range []string{recordType, statusType} {
		if builderRuntimeContentContainsDartIdentifier(withoutImport, symbol) {
			return content
		}
	}
	return withoutImport
}

func builderRuntimeContentContainsDartIdentifier(content, identifier string) bool {
	trimmedIdentifier := strings.TrimSpace(identifier)
	if strings.TrimSpace(content) == "" || trimmedIdentifier == "" {
		return false
	}
	pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(trimmedIdentifier) + `\b`)
	return pattern.MatchString(content)
}

type builderRuntimeOpenLiteResolvedArgument struct {
	expression string
	prelude    []string
}

func normalizeBuilderRuntimeOpenLiteMainDetailCallback(workspacePath, content string) string {
	if strings.TrimSpace(content) == "" {
		return content
	}
	collectionRegistry := builderRuntimeOpenLiteCollectionSurfaceRegistry(workspacePath, content)
	detailCallbackName := strings.TrimSpace(collectionRegistry.view.constructorContract.detailCallbackName)
	if detailCallbackName == "" {
		detailCallbackName = "onOpenRecordDetail"
	}
	detailCallbackPattern := regexp.MustCompile(`(?s)` + regexp.QuoteMeta(detailCallbackName) + `\s*:\s*\([^\)]*\)\s*(?:async\s*)?\{\s*(?:(?://[^\n]*\n)|/\*.*?\*/|\s)*\}`)
	if detailCallbackPattern.FindStringIndex(content) == nil {
		return content
	}
	detailRegistry := builderRuntimeOpenLiteDetailSurfaceRegistry(workspacePath, content)
	detailView := detailRegistry.detailCandidate
	mutationView := detailRegistry.mutationCandidate
	collectionController := detailRegistry.controllerCandidate
	resolvedDetailArgs := builderRuntimeOpenLiteMainDetailResolvedArgumentExpressions(workspacePath, detailView, collectionController)
	if detailRegistry.view.resolvedPath != "" && !builderRuntimeDetailViewSupportsMainCallbackWiring(detailView, resolvedDetailArgs) {
		return content
	}
	if detailRegistry.view.resolvedPath != "" {
		if detailView.editCallbackName != "" && (mutationView.path == "" || mutationView.initialParam == "") {
			return content
		}
		updated := normalizeBuilderRuntimeOpenLiteEnsureDetailPageImport(content, detailRegistry.view.resolvedPath)
		if detailRegistry.mutation.resolvedPath != "" {
			updated = normalizeBuilderRuntimeOpenLiteEnsureFormPageImport(updated, detailRegistry.mutation.resolvedPath)
		}
		updated = normalizeBuilderRuntimeOpenLiteEnsureListControllerVariable(updated, collectionController)
		replacement := builderRuntimeOpenLiteMainDetailSurfaceCallbackReplacementWithResolvedArgs(detailCallbackName, updated, detailView, mutationView, collectionController, resolvedDetailArgs)
		updated = detailCallbackPattern.ReplaceAllString(updated, replacement)
		return normalizeBuilderRuntimeOpenLiteMainDetailViewImports(workspacePath, updated)
	}
	if mutationView.path == "" || mutationView.initialParam == "" || collectionController.path == "" {
		return content
	}
	updated := normalizeBuilderRuntimeOpenLiteEnsureFormPageImport(content, detailRegistry.mutation.resolvedPath)
	updated = normalizeBuilderRuntimeOpenLiteEnsureListControllerVariable(updated, collectionController)
	replacement := builderRuntimeOpenLiteMainDetailCallbackReplacement(detailCallbackName, updated, mutationView, collectionController)
	return detailCallbackPattern.ReplaceAllString(updated, replacement)
}

func normalizeBuilderRuntimeOpenLiteMainCollectionCreateFlow(workspacePath string, needsCollectionCreateEntry bool, content, appClassName string) string {
	if strings.TrimSpace(content) == "" || !needsCollectionCreateEntry {
		return content
	}
	collectionRegistry := builderRuntimeOpenLiteCollectionSurfaceRegistry(workspacePath, content)
	mutationRegistry := builderRuntimeOpenLiteMutationSurfaceRegistry(workspacePath, content)
	createCallbackName := strings.TrimSpace(collectionRegistry.view.constructorContract.createCallbackName)
	if createCallbackName == "" {
		createCallbackName = "onCreateRecord"
	}
	mutationView := builderRuntimeMutationViewCandidate{
		path:            strings.TrimSpace(mutationRegistry.view.resolvedPath),
		className:       strings.TrimSpace(mutationRegistry.view.resolvedClassName),
		repositoryParam: strings.TrimSpace(mutationRegistry.view.constructorContract.repositoryParam),
		initialParam:    strings.TrimSpace(mutationRegistry.view.constructorContract.initialParam),
	}
	if !builderRuntimeWorkspaceHasSemanticFile(workspacePath, mutationView.path) {
		mutationView.path = ""
	}
	collectionController := builderRuntimeCollectionControllerCandidate{
		path:            strings.TrimSpace(collectionRegistry.controller.resolvedPath),
		className:       strings.TrimSpace(collectionRegistry.controller.resolvedClassName),
		repositoryParam: strings.TrimSpace(collectionRegistry.controller.constructorContract.repositoryParam),
		supportsInit:    collectionRegistry.controller.capabilityFlags.supportsInit,
		supportsRefresh: collectionRegistry.controller.capabilityFlags.supportsRefresh,
		supportsUpdate:  collectionRegistry.controller.capabilityFlags.supportsUpdate,
	}
	if !builderRuntimeWorkspaceHasSemanticFile(workspacePath, collectionController.path) {
		collectionController.path = ""
	}
	if mutationView.path == "" || collectionController.path == "" || !collectionController.supportsRefresh {
		return content
	}
	updated := normalizeBuilderRuntimeOpenLiteEnsureFormPageImport(content, mutationView.path)
	updated = normalizeBuilderRuntimeOpenLiteEnsureListControllerVariable(updated, collectionController)
	updated = strings.ReplaceAll(updated, "            listController.updateRecord(updatedRecord);", "            await listController.refresh();")
	if !strings.Contains(updated, createCallbackName+":") && strings.Contains(updated, "        controller: listController,\n") {
		updated = strings.Replace(updated, "        controller: listController,\n", strings.Join([]string{
			"        controller: listController,",
			"        " + createCallbackName + ": () async {",
			"          final createdRecord = await _navigatorKey.currentState!.push<dynamic>(",
			"            MaterialPageRoute(",
			"              builder: (context) => " + mutationView.className + "(",
			"                " + mutationView.repositoryParam + ": repository,",
			"              ),",
			"            ),",
			"          );",
			"          if (createdRecord != null) {",
			"            await listController.refresh();",
			"          }",
			"        },",
		}, "\n")+"\n", 1)
	}
	return updated
}

func normalizeBuilderRuntimeOpenLiteMainNavigatorPushCalls(content, appClassName string) string {
	if strings.TrimSpace(content) == "" || strings.TrimSpace(appClassName) == "" || !strings.Contains(content, "Navigator.of(context).push") {
		return content
	}
	updated := normalizeBuilderRuntimeOpenLiteEnsureNavigatorKey(content, appClassName)
	updated = builderRuntimeOpenLiteMainNavigatorPushDoubleOpenParenPattern.ReplaceAllString(updated, "Navigator.of(context).push$1(")
	updated = builderRuntimeOpenLiteMainNavigatorPushPattern.ReplaceAllString(updated, "_navigatorKey.currentState!.push$1(")
	return updated
}

func normalizeBuilderRuntimeOpenLiteMainRecordFormCallbacks(workspacePath string, needsCollectionCreateEntry bool, content, appClassName string) string {
	if strings.TrimSpace(content) == "" || strings.TrimSpace(appClassName) == "" {
		return content
	}
	collectionRegistry := builderRuntimeOpenLiteCollectionSurfaceRegistry(workspacePath, content)
	mutationRegistry := builderRuntimeOpenLiteMutationSurfaceRegistry(workspacePath, content)
	createCallbackName := strings.TrimSpace(collectionRegistry.view.constructorContract.createCallbackName)
	if createCallbackName == "" {
		createCallbackName = "onCreateRecord"
	}
	detailCallbackName := strings.TrimSpace(collectionRegistry.view.constructorContract.detailCallbackName)
	if detailCallbackName == "" {
		detailCallbackName = "onOpenRecordDetail"
	}
	mutationView := builderRuntimeMutationViewCandidate{
		path:            strings.TrimSpace(mutationRegistry.view.resolvedPath),
		className:       strings.TrimSpace(mutationRegistry.view.resolvedClassName),
		repositoryParam: strings.TrimSpace(mutationRegistry.view.constructorContract.repositoryParam),
		initialParam:    strings.TrimSpace(mutationRegistry.view.constructorContract.initialParam),
	}
	if !builderRuntimeWorkspaceHasSemanticFile(workspacePath, mutationView.path) {
		mutationView.path = ""
	}
	collectionController := builderRuntimeCollectionControllerCandidate{
		path:            strings.TrimSpace(collectionRegistry.controller.resolvedPath),
		className:       strings.TrimSpace(collectionRegistry.controller.resolvedClassName),
		repositoryParam: strings.TrimSpace(collectionRegistry.controller.constructorContract.repositoryParam),
		supportsInit:    collectionRegistry.controller.capabilityFlags.supportsInit,
		supportsRefresh: collectionRegistry.controller.capabilityFlags.supportsRefresh,
		supportsUpdate:  collectionRegistry.controller.capabilityFlags.supportsUpdate,
	}
	if !builderRuntimeWorkspaceHasSemanticFile(workspacePath, collectionController.path) {
		collectionController.path = ""
	}
	if mutationView.path == "" || collectionController.path == "" || !collectionController.supportsRefresh {
		return content
	}
	updated := normalizeBuilderRuntimeOpenLiteEnsureNavigatorKey(content, appClassName)
	updated = normalizeBuilderRuntimeOpenLiteEnsureListControllerVariable(updated, collectionController)
	if needsCollectionCreateEntry && strings.Contains(updated, createCallbackName+":") {
		createCallbackPattern := regexp.MustCompile(`(?s)` + regexp.QuoteMeta(createCallbackName) + `:\s*\(\)\s*async\s*\{.*?\n\s*\},`)
		updated = createCallbackPattern.ReplaceAllString(updated, strings.Join([]string{
			createCallbackName + ": () async {",
			"          final createdRecord = await _navigatorKey.currentState!.push<dynamic>(",
			"            MaterialPageRoute(",
			"              builder: (context) => " + mutationView.className + "(",
			"                " + mutationView.repositoryParam + ": repository,",
			"              ),",
			"            ),",
			"          );",
			"          if (createdRecord != null) {",
			"            await listController.refresh();",
			"          }",
			"        },",
		}, "\n"))
	}
	if strings.Contains(updated, detailCallbackName+":") && mutationView.initialParam != "" && strings.Contains(updated, mutationView.initialParam+": record") {
		detailCallbackPattern := regexp.MustCompile(`(?s)` + regexp.QuoteMeta(detailCallbackName) + `:\s*\(record\)\s*async\s*\{.*?\n\s*\},`)
		updated = detailCallbackPattern.ReplaceAllString(updated, strings.Join([]string{
			detailCallbackName + ": (record) async {",
			"          final updatedRecord = await _navigatorKey.currentState!.push<dynamic>(",
			"            MaterialPageRoute(",
			"              builder: (context) => " + mutationView.className + "(",
			"                " + mutationView.repositoryParam + ": repository,",
			"                " + mutationView.initialParam + ": record,",
			"              ),",
			"            ),",
			"          );",
			"          if (updatedRecord != null) {",
			"            await listController.refresh();",
			"          }",
			"        },",
		}, "\n"))
	}
	return updated
}

func normalizeBuilderRuntimeOpenLiteEnsureFormPageImport(content, formViewPath string) string {
	updated := content
	trimmedPath := strings.TrimPrefix(strings.TrimSpace(formViewPath), "lib/")
	if trimmedPath == "" {
		trimmedPath = "views/record_form_page.dart"
	}
	importLine := "import '" + trimmedPath + "';"
	if strings.Contains(updated, importLine) {
		return updated
	}
	lines := strings.Split(updated, "\n")
	insertIndex := -1
	for index, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "import ") {
			insertIndex = index + 1
		}
	}
	if insertIndex < 0 {
		return importLine + "\n" + updated
	}
	lines = append(lines[:insertIndex], append([]string{importLine}, lines[insertIndex:]...)...)
	return strings.Join(lines, "\n")
}

func normalizeBuilderRuntimeOpenLiteEnsureDetailPageImport(content, detailViewPath string) string {
	updated := content
	trimmedPath := strings.TrimPrefix(strings.TrimSpace(detailViewPath), "lib/")
	if trimmedPath == "" {
		trimmedPath = "views/record_detail_page.dart"
	}
	importLine := "import '" + trimmedPath + "';"
	if strings.Contains(updated, importLine) {
		return updated
	}
	lines := strings.Split(updated, "\n")
	insertIndex := -1
	for index, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "import ") {
			insertIndex = index + 1
		}
	}
	if insertIndex < 0 {
		return importLine + "\n" + updated
	}
	lines = append(lines[:insertIndex], append([]string{importLine}, lines[insertIndex:]...)...)
	return strings.Join(lines, "\n")
}

func normalizeBuilderRuntimeOpenLiteEnsureOverviewPageImport(content, overviewViewPath string) string {
	updated := content
	trimmedPath := strings.TrimPrefix(strings.TrimSpace(overviewViewPath), "lib/")
	if trimmedPath == "" {
		trimmedPath = "views/home_page.dart"
	}
	importLine := "import '" + trimmedPath + "';"
	if strings.Contains(updated, importLine) {
		return updated
	}
	lines := strings.Split(updated, "\n")
	insertIndex := -1
	for index, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "import ") {
			insertIndex = index + 1
		}
	}
	if insertIndex < 0 {
		return importLine + "\n" + updated
	}
	lines = append(lines[:insertIndex], append([]string{importLine}, lines[insertIndex:]...)...)
	return strings.Join(lines, "\n")
}

func normalizeBuilderRuntimeOpenLiteEnsureOverviewControllerImport(content, overviewControllerPath string) string {
	updated := content
	trimmedPath := strings.TrimPrefix(strings.TrimSpace(overviewControllerPath), "lib/")
	if trimmedPath == "" {
		trimmedPath = "controllers/home_controller.dart"
	}
	importLine := "import '" + trimmedPath + "';"
	if strings.Contains(updated, importLine) {
		return updated
	}
	lines := strings.Split(updated, "\n")
	insertIndex := -1
	for index, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "import ") {
			insertIndex = index + 1
		}
	}
	if insertIndex < 0 {
		return importLine + "\n" + updated
	}
	lines = append(lines[:insertIndex], append([]string{importLine}, lines[insertIndex:]...)...)
	return strings.Join(lines, "\n")
}

func normalizeBuilderRuntimeOpenLiteEnsureCollectionControllerImport(content, collectionControllerPath string) string {
	updated := content
	trimmedPath := strings.TrimPrefix(strings.TrimSpace(collectionControllerPath), "lib/")
	if trimmedPath == "" {
		trimmedPath = "controllers/record_list_controller.dart"
	}
	importLine := "import '" + trimmedPath + "';"
	if strings.Contains(updated, importLine) {
		return updated
	}
	lines := strings.Split(updated, "\n")
	insertIndex := -1
	for index, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "import ") {
			insertIndex = index + 1
		}
	}
	if insertIndex < 0 {
		return importLine + "\n" + updated
	}
	lines = append(lines[:insertIndex], append([]string{importLine}, lines[insertIndex:]...)...)
	return strings.Join(lines, "\n")
}

func normalizeBuilderRuntimeOpenLiteEnsureCollectionPageImport(content, collectionViewPath string) string {
	updated := content
	trimmedPath := strings.TrimPrefix(strings.TrimSpace(collectionViewPath), "lib/")
	if trimmedPath == "" {
		trimmedPath = "views/record_list_page.dart"
	}
	importLine := "import '" + trimmedPath + "';"
	if strings.Contains(updated, importLine) {
		return updated
	}
	lines := strings.Split(updated, "\n")
	insertIndex := -1
	for index, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "import ") {
			insertIndex = index + 1
		}
	}
	if insertIndex < 0 {
		return importLine + "\n" + updated
	}
	lines = append(lines[:insertIndex], append([]string{importLine}, lines[insertIndex:]...)...)
	return strings.Join(lines, "\n")
}

func normalizeBuilderRuntimeOpenLiteMainCollectionControllerImports(workspacePath, content string) string {
	if strings.TrimSpace(content) == "" {
		return content
	}
	updated := content
	defaultImport := "import 'controllers/record_list_controller.dart';"
	defaultClassName := "RecordListController"
	collectionRegistry := builderRuntimeOpenLiteCollectionSurfaceRegistry(workspacePath, updated)
	collectionControllerPath := strings.TrimSpace(collectionRegistry.controller.resolvedPath)
	collectionControllerClass := strings.TrimSpace(collectionRegistry.controller.resolvedClassName)
	if !builderRuntimeWorkspaceHasSemanticFile(workspacePath, collectionControllerPath) || collectionControllerClass == "" {
		return builderRuntimeOpenLiteDropImportIfUnused(updated, defaultImport, defaultClassName)
	}
	trimmedPath := strings.TrimPrefix(collectionControllerPath, "lib/")
	if trimmedPath != "" && builderRuntimeContentContainsDartIdentifier(updated, collectionControllerClass) {
		updated = normalizeBuilderRuntimeOpenLiteEnsureCollectionControllerImport(updated, collectionControllerPath)
	}
	currentImport := ""
	if trimmedPath != "" {
		currentImport = "import '" + trimmedPath + "';"
	}
	if trimmedPath != "controllers/record_list_controller.dart" {
		updated = builderRuntimeOpenLiteDropImportIfUnused(updated, defaultImport, defaultClassName)
	}
	if currentImport != "" {
		updated = builderRuntimeOpenLiteDropImportIfUnused(updated, currentImport, collectionControllerClass)
	}
	return updated
}

func normalizeBuilderRuntimeOpenLiteMainCollectionControllerVariable(workspacePath, content string) string {
	if strings.TrimSpace(content) == "" {
		return content
	}
	view := builderRuntimePrimaryCollectionViewCandidate(workspacePath, content)
	collectionController := builderRuntimePrimaryCollectionControllerCandidate(workspacePath, view.path, view.className, view.controllerParam, view.controllerType, content)
	if !builderRuntimeWorkspaceHasSemanticFile(workspacePath, collectionController.path) || strings.TrimSpace(collectionController.className) == "" || strings.TrimSpace(collectionController.repositoryParam) == "" {
		return content
	}
	canonicalDeclaration := "    final listController = " + collectionController.className + "(" + collectionController.repositoryParam + ": repository);\n"
	updated := builderRuntimeOpenLiteMainListControllerDeclarationPattern.ReplaceAllString(content, canonicalDeclaration)
	return normalizeBuilderRuntimeOpenLiteEnsureListControllerVariable(updated, collectionController)
}

func normalizeBuilderRuntimeOpenLiteMainCollectionViewImports(workspacePath, content string) string {
	if strings.TrimSpace(content) == "" {
		return content
	}
	updated := content
	defaultImport := "import 'views/record_list_page.dart';"
	defaultClassName := "RecordListPage"
	collectionRegistry := builderRuntimeOpenLiteCollectionSurfaceRegistry(workspacePath, updated)
	collectionViewPath := strings.TrimSpace(collectionRegistry.view.resolvedPath)
	collectionViewClass := strings.TrimSpace(collectionRegistry.view.resolvedClassName)
	if !builderRuntimeWorkspaceHasSemanticFile(workspacePath, collectionViewPath) || collectionViewClass == "" {
		return builderRuntimeOpenLiteDropImportIfUnused(updated, defaultImport, defaultClassName)
	}
	trimmedPath := strings.TrimPrefix(collectionViewPath, "lib/")
	if trimmedPath != "" && builderRuntimeContentContainsDartIdentifier(updated, collectionViewClass) {
		updated = normalizeBuilderRuntimeOpenLiteEnsureCollectionPageImport(updated, collectionViewPath)
	}
	currentImport := ""
	if trimmedPath != "" {
		currentImport = "import '" + trimmedPath + "';"
	}
	if trimmedPath != "views/record_list_page.dart" {
		updated = builderRuntimeOpenLiteDropImportIfUnused(updated, defaultImport, defaultClassName)
	}
	if currentImport != "" {
		updated = builderRuntimeOpenLiteDropImportIfUnused(updated, currentImport, collectionViewClass)
	}
	return updated
}

func normalizeBuilderRuntimeOpenLiteMainOverviewControllerImports(workspacePath, content string) string {
	if strings.TrimSpace(content) == "" {
		return content
	}
	updated := content
	defaultImport := "import 'controllers/home_controller.dart';"
	defaultClassName := "HomeController"
	overviewRegistry := builderRuntimeOpenLiteOverviewSurfaceRegistry(workspacePath, updated)
	overviewControllerPath := strings.TrimSpace(overviewRegistry.controller.resolvedPath)
	overviewControllerClass := strings.TrimSpace(overviewRegistry.controller.resolvedClassName)
	if !builderRuntimeWorkspaceHasSemanticFile(workspacePath, overviewControllerPath) || overviewControllerClass == "" {
		return builderRuntimeOpenLiteDropImportIfUnused(updated, defaultImport, defaultClassName)
	}
	trimmedPath := strings.TrimPrefix(overviewControllerPath, "lib/")
	if trimmedPath != "" && builderRuntimeContentContainsDartIdentifier(updated, overviewControllerClass) {
		updated = normalizeBuilderRuntimeOpenLiteEnsureOverviewControllerImport(updated, overviewControllerPath)
	}
	currentImport := ""
	if trimmedPath != "" {
		currentImport = "import '" + trimmedPath + "';"
	}
	if trimmedPath != "controllers/home_controller.dart" {
		updated = builderRuntimeOpenLiteDropImportIfUnused(updated, defaultImport, defaultClassName)
	}
	if currentImport != "" {
		updated = builderRuntimeOpenLiteDropImportIfUnused(updated, currentImport, overviewControllerClass)
	}
	return updated
}

func normalizeBuilderRuntimeOpenLiteMainOverviewViewImports(workspacePath, content string) string {
	if strings.TrimSpace(content) == "" {
		return content
	}
	updated := content
	defaultImport := "import 'views/home_page.dart';"
	defaultClassName := "HomePage"
	overviewRegistry := builderRuntimeOpenLiteOverviewSurfaceRegistry(workspacePath, updated)
	overviewViewPath := strings.TrimSpace(overviewRegistry.view.resolvedPath)
	overviewViewClass := strings.TrimSpace(overviewRegistry.view.resolvedClassName)
	if !builderRuntimeWorkspaceHasSemanticFile(workspacePath, overviewViewPath) || overviewViewClass == "" {
		return builderRuntimeOpenLiteDropImportIfUnused(updated, defaultImport, defaultClassName)
	}
	trimmedPath := strings.TrimPrefix(overviewViewPath, "lib/")
	if trimmedPath != "" && builderRuntimeContentContainsDartIdentifier(updated, overviewViewClass) {
		updated = normalizeBuilderRuntimeOpenLiteEnsureOverviewPageImport(updated, overviewViewPath)
	}
	currentImport := ""
	if trimmedPath != "" {
		currentImport = "import '" + trimmedPath + "';"
	}
	if trimmedPath != "views/home_page.dart" {
		updated = builderRuntimeOpenLiteDropImportIfUnused(updated, defaultImport, defaultClassName)
	}
	if currentImport != "" {
		updated = builderRuntimeOpenLiteDropImportIfUnused(updated, currentImport, overviewViewClass)
	}
	return updated
}

func normalizeBuilderRuntimeOpenLiteMainDetailViewImports(workspacePath, content string) string {
	if strings.TrimSpace(content) == "" {
		return content
	}
	updated := content
	defaultImport := "import 'views/record_detail_page.dart';"
	defaultClassName := "RecordDetailPage"
	detailRegistry := builderRuntimeOpenLiteDetailSurfaceRegistry(workspacePath, updated)
	detailViewPath := strings.TrimSpace(detailRegistry.view.resolvedPath)
	detailViewClass := strings.TrimSpace(detailRegistry.view.resolvedClassName)
	if !builderRuntimeWorkspaceHasSemanticFile(workspacePath, detailViewPath) || detailViewClass == "" {
		return builderRuntimeOpenLiteDropImportIfUnused(updated, defaultImport, defaultClassName)
	}
	trimmedPath := strings.TrimPrefix(detailViewPath, "lib/")
	if trimmedPath != "" && builderRuntimeContentContainsDartIdentifier(updated, detailViewClass) {
		updated = normalizeBuilderRuntimeOpenLiteEnsureDetailPageImport(updated, detailViewPath)
	}
	currentImport := ""
	if trimmedPath != "" {
		currentImport = "import '" + trimmedPath + "';"
	}
	if trimmedPath != "views/record_detail_page.dart" {
		updated = builderRuntimeOpenLiteDropImportIfUnused(updated, defaultImport, defaultClassName)
	}
	if currentImport != "" {
		updated = builderRuntimeOpenLiteDropImportIfUnused(updated, currentImport, detailViewClass)
	}
	return updated
}

func normalizeBuilderRuntimeOpenLiteMainMutationViewImports(workspacePath, content string) string {
	if strings.TrimSpace(content) == "" {
		return content
	}
	updated := content
	defaultImport := "import 'views/record_form_page.dart';"
	defaultClassName := "RecordFormPage"
	mutationRegistry := builderRuntimeOpenLiteMutationSurfaceRegistry(workspacePath, updated)
	mutationViewPath := strings.TrimSpace(mutationRegistry.view.resolvedPath)
	mutationViewClass := strings.TrimSpace(mutationRegistry.view.resolvedClassName)
	if !builderRuntimeWorkspaceHasSemanticFile(workspacePath, mutationViewPath) || mutationViewClass == "" {
		return builderRuntimeOpenLiteDropImportIfUnused(updated, defaultImport, defaultClassName)
	}
	trimmedPath := strings.TrimPrefix(mutationViewPath, "lib/")
	if trimmedPath != "" && builderRuntimeContentContainsDartIdentifier(updated, mutationViewClass) {
		updated = normalizeBuilderRuntimeOpenLiteEnsureFormPageImport(updated, mutationViewPath)
	}
	currentImport := ""
	if trimmedPath != "" {
		currentImport = "import '" + trimmedPath + "';"
	}
	if trimmedPath != "views/record_form_page.dart" {
		updated = builderRuntimeOpenLiteDropImportIfUnused(updated, defaultImport, defaultClassName)
	}
	if currentImport != "" {
		updated = builderRuntimeOpenLiteDropImportIfUnused(updated, currentImport, mutationViewClass)
	}
	return updated
}

func normalizeBuilderRuntimeOpenLiteEnsureListControllerVariable(content string, controller builderRuntimeCollectionControllerCandidate) string {
	if controller.className == "" || controller.repositoryParam == "" {
		return content
	}
	inlineController := "        controller: " + controller.className + "(" + controller.repositoryParam + ": repository),"
	canonicalDeclaration := "    final listController = " + controller.className + "(" + controller.repositoryParam + ": repository);\n"
	updated := content
	if controller.className == "RecordListController" && controller.repositoryParam == "repository" {
		updated = builderRuntimeOpenLiteMainListControllerDeclarationPattern.ReplaceAllString(updated, canonicalDeclaration)
	} else {
		updated = builderRuntimeOpenLiteMainListControllerDeclarationPattern.ReplaceAllString(updated, canonicalDeclaration)
	}
	declarationPattern := regexp.MustCompile(`(?m)^[ \t]*final\s+listController\s*=\s*` + regexp.QuoteMeta(controller.className) + `\s*\(\s*` + regexp.QuoteMeta(controller.repositoryParam) + `\s*:\s*repository\s*\);[ \t]*(?:\n|$)`)
	updated = declarationPattern.ReplaceAllString(updated, canonicalDeclaration)
	if strings.Contains(updated, inlineController) {
		updated = strings.ReplaceAll(updated, inlineController, "        controller: listController,")
	}
	if strings.Contains(updated, "controller: listController,") && !strings.Contains(updated, strings.TrimSpace(canonicalDeclaration)) {
		updated = strings.Replace(updated, "    return MaterialApp(", canonicalDeclaration+"    return MaterialApp(", 1)
	}
	return updated
}

func normalizeBuilderRuntimeOpenLiteEnsureNavigatorKey(content, appClassName string) string {
	updated := content
	constConstructor := "  const " + appClassName + "({super.key, required this.repository});"
	if strings.Contains(updated, constConstructor) {
		updated = strings.Replace(updated, constConstructor, "  "+appClassName+"({super.key, required this.repository});", 1)
	}
	if !strings.Contains(updated, "final GlobalKey<NavigatorState> _navigatorKey = GlobalKey<NavigatorState>();") {
		updated = builderRuntimeInsertFieldIntoStatelessWidget(updated, appClassName, "final GlobalKey<NavigatorState> _navigatorKey = GlobalKey<NavigatorState>();")
	}
	if !strings.Contains(updated, "navigatorKey: _navigatorKey,") {
		updated = strings.Replace(updated, "    return MaterialApp(\n", "    return MaterialApp(\n      navigatorKey: _navigatorKey,\n", 1)
	}
	return updated
}

func builderRuntimeOpenLiteMainDetailCallbackReplacement(detailCallbackName, content string, mutationView builderRuntimeMutationViewCandidate, controller builderRuntimeCollectionControllerCandidate) string {
	if strings.TrimSpace(detailCallbackName) == "" {
		detailCallbackName = "onOpenRecordDetail"
	}
	if controller.supportsUpdate && strings.Contains(content, "listController") {
		return strings.Join([]string{
			detailCallbackName + ": (record) async {",
			"          final updatedRecord = await Navigator.of(context).push<dynamic>(",
			"            MaterialPageRoute(",
			"              builder: (context) => " + mutationView.className + "(",
			"                " + mutationView.repositoryParam + ": repository,",
			"                " + mutationView.initialParam + ": record,",
			"              ),",
			"            ),",
			"          );",
			"          if (updatedRecord != null) {",
			"            listController.updateRecord(updatedRecord);",
			"          }",
			"        }",
		}, "\n")
	}
	return strings.Join([]string{
		detailCallbackName + ": (record) async {",
		"          await Navigator.of(context).push<void>(",
		"            MaterialPageRoute(",
		"              builder: (context) => " + mutationView.className + "(",
		"                " + mutationView.repositoryParam + ": repository,",
		"                " + mutationView.initialParam + ": record,",
		"              ),",
		"            ),",
		"          );",
		"        }",
	}, "\n")
}

func builderRuntimeOpenLiteMainDetailSurfaceCallbackReplacement(detailCallbackName, content string, detailView builderRuntimeDetailViewCandidate, mutationView builderRuntimeMutationViewCandidate, controller builderRuntimeCollectionControllerCandidate) string {
	return builderRuntimeOpenLiteMainDetailSurfaceCallbackReplacementWithResolvedArgs(detailCallbackName, content, detailView, mutationView, controller, nil)
}

func builderRuntimeOpenLiteMainDetailSurfaceCallbackReplacementWithResolvedArgs(detailCallbackName, content string, detailView builderRuntimeDetailViewCandidate, mutationView builderRuntimeMutationViewCandidate, controller builderRuntimeCollectionControllerCandidate, resolvedArgs map[string]builderRuntimeOpenLiteResolvedArgument) string {
	if strings.TrimSpace(detailCallbackName) == "" {
		detailCallbackName = "onOpenRecordDetail"
	}
	lines := []string{
		detailCallbackName + ": (record) async {",
	}
	lines = append(lines, builderRuntimeOpenLiteMainDetailResolvedArgumentPreludeLines(detailView, resolvedArgs)...)
	lines = append(lines,
		"          await Navigator.of(context).push<void>(",
		"            MaterialPageRoute(",
		"              builder: (context) => "+detailView.className+"(",
		"                "+detailView.recordParam+": record,",
	)
	for _, name := range detailView.requiredParams {
		trimmedName := strings.TrimSpace(name)
		if trimmedName == "" || trimmedName == detailView.recordParam || trimmedName == detailView.editCallbackName || trimmedName == detailView.deleteCallbackName {
			continue
		}
		if expression := strings.TrimSpace(resolvedArgs[trimmedName].expression); expression != "" {
			lines = append(lines, "                "+trimmedName+": "+expression+",")
		}
	}
	if detailView.editCallbackName != "" && mutationView.path != "" && mutationView.initialParam != "" {
		lines = append(lines,
			"                "+detailView.editCallbackName+": (updatedRecord) async {",
			"                  final editedRecord = await Navigator.of(context).push<dynamic>(",
			"                    MaterialPageRoute(",
			"                      builder: (context) => "+mutationView.className+"(",
			"                        "+mutationView.repositoryParam+": repository,",
			"                        "+mutationView.initialParam+": updatedRecord,",
			"                      ),",
			"                    ),",
			"                  );",
		)
		if strings.Contains(content, "listController") && controller.supportsUpdate {
			lines = append(lines,
				"                  if (editedRecord != null) {",
				"                    listController.updateRecord(editedRecord);",
				"                  }",
			)
		} else if strings.Contains(content, "listController") && controller.supportsRefresh {
			lines = append(lines,
				"                  if (editedRecord != null) {",
				"                    await listController.refresh();",
				"                  }",
			)
		}
		lines = append(lines, "                },")
	}
	lines = append(lines,
		"              ),",
		"            ),",
		"          );",
		"        }",
	)
	return strings.Join(lines, "\n")
}

func builderRuntimeOpenLiteMainDetailResolvedArgumentPreludeLines(detailView builderRuntimeDetailViewCandidate, resolvedArgs map[string]builderRuntimeOpenLiteResolvedArgument) []string {
	if len(resolvedArgs) == 0 {
		return nil
	}
	lines := make([]string, 0)
	seen := make(map[string]struct{})
	for _, name := range detailView.requiredParams {
		trimmedName := strings.TrimSpace(name)
		if trimmedName == "" {
			continue
		}
		for _, line := range resolvedArgs[trimmedName].prelude {
			if _, ok := seen[line]; ok {
				continue
			}
			seen[line] = struct{}{}
			lines = append(lines, line)
		}
	}
	return lines
}

func builderRuntimeOpenLiteMainDetailResolvedArgumentExpressions(workspacePath string, detailView builderRuntimeDetailViewCandidate, controller builderRuntimeCollectionControllerCandidate) map[string]builderRuntimeOpenLiteResolvedArgument {
	if detailView.path == "" || controller.path == "" {
		return nil
	}
	detailContent, _ := builderRuntimeSemanticFileContent(workspacePath, nil, detailView.path)
	controllerContent, err := builderRuntimeSemanticFileContent(workspacePath, nil, controller.path)
	if err != nil || strings.TrimSpace(controllerContent) == "" {
		return nil
	}
	resolved := make(map[string]builderRuntimeOpenLiteResolvedArgument)
	for _, name := range detailView.requiredParams {
		trimmedName := strings.TrimSpace(name)
		if trimmedName == "" || trimmedName == detailView.recordParam || trimmedName == detailView.editCallbackName || trimmedName == detailView.deleteCallbackName {
			continue
		}
		if builderRuntimeOpenLiteControllerHasReadableMember(controllerContent, trimmedName) {
			resolved[trimmedName] = builderRuntimeOpenLiteResolvedArgument{expression: "listController." + trimmedName}
			continue
		}
		if derived, ok := builderRuntimeOpenLiteControllerDerivedDetailArgument(workspacePath, detailContent, controllerContent, trimmedName); ok {
			resolved[trimmedName] = derived
		}
	}
	repository := builderRuntimePrimaryRepositoryCandidate(workspacePath, detailView.path, detailView.className, controller.path, controller.className, controller.repositoryParam)
	if repository.path != "" {
		repositoryContent, err := builderRuntimeSemanticFileContent(workspacePath, nil, repository.path)
		if err == nil && strings.TrimSpace(repositoryContent) != "" {
			for _, name := range detailView.requiredParams {
				trimmedName := strings.TrimSpace(name)
				if trimmedName == "" || trimmedName == detailView.recordParam || trimmedName == detailView.editCallbackName || trimmedName == detailView.deleteCallbackName {
					continue
				}
				if _, ok := resolved[trimmedName]; ok {
					continue
				}
				if derived, ok := builderRuntimeOpenLiteRepositoryDerivedDetailArgument(workspacePath, detailContent, repositoryContent, trimmedName); ok {
					resolved[trimmedName] = derived
				}
			}
		}
	}
	return resolved
}

func builderRuntimeOpenLiteControllerHasReadableMember(content, name string) bool {
	trimmedName := strings.TrimSpace(name)
	if strings.TrimSpace(content) == "" || trimmedName == "" {
		return false
	}
	fieldPattern := regexp.MustCompile(`(?m)^\s*(?:final|late\s+final|late)\s+[^\n;=]+\s+` + regexp.QuoteMeta(trimmedName) + `\s*(?:=|;)`)
	if fieldPattern.FindStringIndex(content) != nil {
		return true
	}
	getterPattern := regexp.MustCompile(`(?m)^\s*[^\n]+\s+get\s+` + regexp.QuoteMeta(trimmedName) + `\s*(?:=>|\{)`)
	return getterPattern.FindStringIndex(content) != nil
}

func builderRuntimeOpenLiteControllerDerivedDetailArgument(workspacePath, detailContent, controllerContent, name string) (builderRuntimeOpenLiteResolvedArgument, bool) {
	trimmedName := strings.TrimSpace(name)
	if strings.TrimSpace(workspacePath) == "" || strings.TrimSpace(controllerContent) == "" || trimmedName == "" {
		return builderRuntimeOpenLiteResolvedArgument{}, false
	}
	if derived, ok := builderRuntimeOpenLiteControllerDerivedDetailArgumentForRelation(workspacePath, controllerContent, trimmedName); ok {
		return derived, true
	}
	relationName, isCollection, ok := builderRuntimeOpenLiteDetailArgumentRelationHint(workspacePath, detailContent, trimmedName)
	if !ok || isCollection {
		return builderRuntimeOpenLiteResolvedArgument{}, false
	}
	return builderRuntimeOpenLiteControllerDerivedDetailArgumentForRelation(workspacePath, controllerContent, relationName)
}

func builderRuntimeOpenLiteControllerDerivedDetailArgumentForRelation(workspacePath, controllerContent, relationName string) (builderRuntimeOpenLiteResolvedArgument, bool) {
	trimmedName := strings.TrimSpace(relationName)
	if strings.TrimSpace(workspacePath) == "" || strings.TrimSpace(controllerContent) == "" || trimmedName == "" {
		return builderRuntimeOpenLiteResolvedArgument{}, false
	}
	recordModelContent := builderRuntimeOpenLitePrimaryRecordModelContent(workspacePath)
	if strings.TrimSpace(recordModelContent) == "" {
		return builderRuntimeOpenLiteResolvedArgument{}, false
	}
	recordField := trimmedName + "Id"
	fields := builderRuntimeDeclaredFieldNames(recordModelContent)
	if _, ok := fields[recordField]; !ok {
		return builderRuntimeOpenLiteResolvedArgument{}, false
	}
	lookupMethod := trimmedName + "For"
	if !builderRuntimeOpenLiteContentHasMethod(controllerContent, lookupMethod) {
		return builderRuntimeOpenLiteResolvedArgument{}, false
	}
	return builderRuntimeOpenLiteResolvedArgument{
		expression: "listController." + lookupMethod + "(record." + recordField + ")",
	}, true
}

func builderRuntimeOpenLiteContentHasMethod(content, name string) bool {
	trimmedName := strings.TrimSpace(name)
	if strings.TrimSpace(content) == "" || trimmedName == "" {
		return false
	}
	methodPattern := regexp.MustCompile(`(?m)^\s*[^\n;=]+\s+` + regexp.QuoteMeta(trimmedName) + `\s*\(`)
	return methodPattern.FindStringIndex(content) != nil
}

func builderRuntimeOpenLiteRepositoryHasMethod(content, name string) bool {
	return builderRuntimeOpenLiteContentHasMethod(content, name)
}

func builderRuntimeOpenLiteRepositoryDerivedDetailArgument(workspacePath, detailContent, repositoryContent, name string) (builderRuntimeOpenLiteResolvedArgument, bool) {
	if derived, ok := builderRuntimeOpenLiteRepositoryDerivedCollectionArgument(repositoryContent, name); ok {
		return derived, true
	}
	if derived, ok := builderRuntimeOpenLiteRepositoryDerivedSingularRelationArgument(workspacePath, repositoryContent, name); ok {
		return derived, true
	}
	if relationName, isCollection, ok := builderRuntimeOpenLiteDetailArgumentRelationHint(workspacePath, detailContent, name); ok {
		if isCollection {
			if derived, ok := builderRuntimeOpenLiteRepositoryDerivedCollectionArgumentWithAlias(repositoryContent, builderRuntimeOpenLitePluralizeIdentifier(relationName), name); ok {
				return derived, true
			}
		} else if derived, ok := builderRuntimeOpenLiteRepositoryDerivedSingularRelationArgumentWithAlias(workspacePath, repositoryContent, relationName, name); ok {
			return derived, true
		}
	}
	switch strings.TrimSpace(name) {
	case "taskTags":
		if !builderRuntimeOpenLiteRepositoryHasMethod(repositoryContent, "loadTaskTagLinks") {
			return builderRuntimeOpenLiteResolvedArgument{}, false
		}
		return builderRuntimeOpenLiteResolvedArgument{
			expression: "taskTags",
			prelude: []string{
				"          final taskTags = (await repository.loadTaskTagLinks())",
				"              .where((link) => link.taskId == record.taskId)",
				"              .map((link) => link.tagId as String)",
				"              .toList();",
			},
		}, true
	case "warehouse":
		if !builderRuntimeOpenLiteRepositoryHasMethod(repositoryContent, "loadWarehouses") {
			return builderRuntimeOpenLiteResolvedArgument{}, false
		}
		return builderRuntimeOpenLiteResolvedArgument{
			expression: "warehouse",
			prelude: []string{
				"          dynamic warehouse;",
				"          for (final candidate in await repository.loadWarehouses()) {",
				"            if (candidate.warehouseId == record.warehouseId) {",
				"              warehouse = candidate;",
				"              break;",
				"            }",
				"          }",
			},
		}, true
	case "items":
		if !builderRuntimeOpenLiteRepositoryHasMethod(repositoryContent, "loadLineItems") {
			return builderRuntimeOpenLiteResolvedArgument{}, false
		}
		return builderRuntimeOpenLiteResolvedArgument{
			expression: "items",
			prelude: []string{
				"          final items = (await repository.loadLineItems())",
				"              .where((candidate) => candidate.sheetId == record.sheetId)",
				"              .toList();",
			},
		}, true
	case "skus":
		if !builderRuntimeOpenLiteRepositoryHasMethod(repositoryContent, "loadSkus") {
			return builderRuntimeOpenLiteResolvedArgument{}, false
		}
		return builderRuntimeOpenLiteResolvedArgument{
			expression: "skus",
			prelude: []string{
				"          final skus = await repository.loadSkus();",
			},
		}, true
	default:
		return builderRuntimeOpenLiteResolvedArgument{}, false
	}
}

func builderRuntimeOpenLiteRepositoryDerivedCollectionArgument(repositoryContent, name string) (builderRuntimeOpenLiteResolvedArgument, bool) {
	return builderRuntimeOpenLiteRepositoryDerivedCollectionArgumentWithAlias(repositoryContent, name, name)
}

func builderRuntimeOpenLiteRepositoryDerivedCollectionArgumentWithAlias(repositoryContent, collectionName, aliasName string) (builderRuntimeOpenLiteResolvedArgument, bool) {
	trimmedCollectionName := strings.TrimSpace(collectionName)
	trimmedAliasName := strings.TrimSpace(aliasName)
	if strings.TrimSpace(repositoryContent) == "" || trimmedCollectionName == "" || trimmedAliasName == "" || !strings.HasSuffix(trimmedCollectionName, "s") {
		return builderRuntimeOpenLiteResolvedArgument{}, false
	}
	loaderName := "load" + strings.ToUpper(trimmedCollectionName[:1]) + trimmedCollectionName[1:]
	if !builderRuntimeOpenLiteRepositoryHasMethod(repositoryContent, loaderName) {
		return builderRuntimeOpenLiteResolvedArgument{}, false
	}
	return builderRuntimeOpenLiteResolvedArgument{
		expression: trimmedAliasName,
		prelude: []string{
			"          final " + trimmedAliasName + " = await repository." + loaderName + "();",
		},
	}, true
}

func builderRuntimeOpenLiteRepositoryDerivedSingularRelationArgument(workspacePath, repositoryContent, name string) (builderRuntimeOpenLiteResolvedArgument, bool) {
	return builderRuntimeOpenLiteRepositoryDerivedSingularRelationArgumentWithAlias(workspacePath, repositoryContent, name, name)
}

func builderRuntimeOpenLiteRepositoryDerivedSingularRelationArgumentWithAlias(workspacePath, repositoryContent, relationName, aliasName string) (builderRuntimeOpenLiteResolvedArgument, bool) {
	trimmedName := strings.TrimSpace(relationName)
	trimmedAliasName := strings.TrimSpace(aliasName)
	if strings.TrimSpace(workspacePath) == "" || strings.TrimSpace(repositoryContent) == "" || trimmedName == "" || strings.HasSuffix(trimmedName, "s") {
		return builderRuntimeOpenLiteResolvedArgument{}, false
	}
	if trimmedAliasName == "" {
		return builderRuntimeOpenLiteResolvedArgument{}, false
	}
	recordModelContent := builderRuntimeOpenLitePrimaryRecordModelContent(workspacePath)
	if strings.TrimSpace(recordModelContent) == "" {
		return builderRuntimeOpenLiteResolvedArgument{}, false
	}
	recordField := trimmedName + "Id"
	fields := builderRuntimeDeclaredFieldNames(recordModelContent)
	if _, ok := fields[recordField]; !ok {
		return builderRuntimeOpenLiteResolvedArgument{}, false
	}
	loaderTarget := builderRuntimeOpenLitePluralizeIdentifier(trimmedName)
	loaderName := "load" + strings.ToUpper(loaderTarget[:1]) + loaderTarget[1:]
	if !builderRuntimeOpenLiteRepositoryHasMethod(repositoryContent, loaderName) {
		return builderRuntimeOpenLiteResolvedArgument{}, false
	}
	return builderRuntimeOpenLiteResolvedArgument{
		expression: trimmedAliasName,
		prelude: []string{
			"          dynamic " + trimmedAliasName + ";",
			"          for (final candidate in await repository." + loaderName + "()) {",
			"            if (candidate." + recordField + " == record." + recordField + ") {",
			"              " + trimmedAliasName + " = candidate;",
			"              break;",
			"            }",
			"          }",
		},
	}, true
}

func builderRuntimeOpenLiteDetailArgumentRelationHint(workspacePath, detailContent, name string) (string, bool, bool) {
	trimmedName := strings.TrimSpace(name)
	if strings.TrimSpace(workspacePath) == "" || strings.TrimSpace(detailContent) == "" || trimmedName == "" {
		return "", false, false
	}
	fieldType := builderRuntimeDartFieldTypeByName(detailContent, trimmedName)
	modelType, isCollection := builderRuntimeOpenLiteDetailArgumentModelType(fieldType)
	if modelType == "" {
		return "", false, false
	}
	if modelType == builderRuntimeOpenLitePrimaryRecordTypeName(workspacePath) {
		return "", false, false
	}
	if builderRuntimeModelImportPathForType(workspacePath, modelType) == "" {
		return "", false, false
	}
	relationName := builderRuntimeOpenLiteLowerCamelIdentifier(modelType)
	if relationName == "" {
		return "", false, false
	}
	return relationName, isCollection, true
}

func builderRuntimeOpenLiteDetailArgumentModelType(fieldType string) (string, bool) {
	trimmedType := strings.TrimSpace(strings.TrimSuffix(fieldType, "?"))
	if trimmedType == "" {
		return "", false
	}
	isCollection := false
	if strings.HasPrefix(trimmedType, "List<") && strings.HasSuffix(trimmedType, ">") {
		isCollection = true
		trimmedType = strings.TrimSpace(strings.TrimSuffix(trimmedType[len("List<"):len(trimmedType)-1], "?"))
	}
	if separator := strings.LastIndex(trimmedType, "."); separator >= 0 {
		trimmedType = strings.TrimSpace(trimmedType[separator+1:])
	}
	return trimmedType, isCollection
}

func builderRuntimeOpenLiteLowerCamelIdentifier(name string) string {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return ""
	}
	return strings.ToLower(trimmedName[:1]) + trimmedName[1:]
}

func builderRuntimeOpenLitePluralizeIdentifier(name string) string {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return ""
	}
	switch {
	case strings.HasSuffix(trimmedName, "ch"), strings.HasSuffix(trimmedName, "sh"), strings.HasSuffix(trimmedName, "s"), strings.HasSuffix(trimmedName, "x"), strings.HasSuffix(trimmedName, "z"):
		return trimmedName + "es"
	case strings.HasSuffix(trimmedName, "y") && len(trimmedName) > 1:
		return trimmedName[:len(trimmedName)-1] + "ies"
	default:
		return trimmedName + "s"
	}
}

func builderRuntimeOpenLiteListControllerSupportsUpdate(workspacePath string) bool {
	return builderRuntimeOpenLiteCollectionSurfaceRegistry(workspacePath).controller.capabilityFlags.supportsUpdate
}

func builderRuntimeOpenLiteListControllerSupportsInit(workspacePath string) bool {
	return builderRuntimeOpenLiteCollectionSurfaceRegistry(workspacePath).controller.capabilityFlags.supportsInit
}

func builderRuntimeOpenLiteListControllerSupportsRefresh(workspacePath string) bool {
	return builderRuntimeOpenLiteCollectionSurfaceRegistry(workspacePath).controller.capabilityFlags.supportsRefresh
}

func builderRuntimeOpenLiteCanonicalNoFilterListController(workspacePath string) string {
	return builderRuntimeOpenLiteCanonicalNoFilterListControllerWithDelete(workspacePath, false)
}

func builderRuntimeOpenLiteCanonicalNoFilterListControllerWithDelete(workspacePath string, allowDeleteFlow bool) string {
	entry := builderRuntimeOpenLiteCollectionSurfaceRegistry(workspacePath).controller
	repositoryType := strings.TrimSpace(entry.repositoryType)
	if repositoryType == "" {
		repositoryType = "RecordRepository"
	}
	recordType := strings.TrimSpace(entry.modelType)
	if recordType == "" {
		recordType = "TodoItem"
	}
	className := strings.TrimSpace(entry.resolvedClassName)
	if className == "" || className == "HomeController" {
		className = "RecordListController"
	}
	repositoryParam := strings.TrimSpace(entry.constructorContract.repositoryParam)
	if repositoryParam == "" {
		repositoryParam = "repository"
	}
	modelImportPath := strings.TrimSpace(entry.modelImportPath)
	if modelImportPath == "" {
		modelImportPath = "../models/record.dart"
	}
	repositoryImportPath := strings.TrimSpace(entry.repositoryImportPath)
	if repositoryImportPath == "" {
		repositoryImportPath = "../repositories/record_repository.dart"
	}
	loadMethodName, addMethodName, updateMethodName, deleteMethodName := builderRuntimeOpenLiteCollectionRepositoryMethodNames(workspacePath, recordType)
	lines := []string{
		"import 'package:flutter/foundation.dart' show ChangeNotifier;",
		"",
		"import '" + modelImportPath + "';",
		"import '" + repositoryImportPath + "';",
		"",
		"class " + className + " extends ChangeNotifier {",
		"  " + className + "({required " + repositoryType + " " + repositoryParam + "})",
		"      : _repository = " + repositoryParam + " {",
		"    _init();",
		"  }",
		"",
		"  final " + repositoryType + " _repository;",
		"  List<" + recordType + "> _records = [];",
		"  bool _isLoading = false;",
		"",
		"  List<" + recordType + "> get records => _records;",
		"  bool get isLoading => _isLoading;",
		"",
		"  Future<void> _init() async {",
		"    await refresh();",
		"  }",
		"",
		"  Future<void> refresh() async {",
		"    _isLoading = true;",
		"    notifyListeners();",
		"    try {",
		"      _records = await _repository." + loadMethodName + "();",
		"    } finally {",
		"      _isLoading = false;",
		"      notifyListeners();",
		"    }",
		"  }",
		"",
		"  Future<void> addRecord(" + recordType + " record) async {",
		"    await _repository." + addMethodName + "(record);",
		"    await refresh();",
		"  }",
		"",
		"  Future<void> updateRecord(" + recordType + " record) async {",
		"    await _repository." + updateMethodName + "(record);",
		"    await refresh();",
		"  }",
	}
	if allowDeleteFlow {
		lines = append(lines,
			"",
			"  Future<void> deleteRecord(String recordId) async {",
			"    await _repository."+deleteMethodName+"(recordId);",
			"    await refresh();",
			"  }",
		)
	}
	lines = append(lines, "}")
	return strings.Join(lines, "\n") + "\n"
}

func builderRuntimeOpenLiteCollectionRepositoryMethodNames(workspacePath, recordType string) (string, string, string, string) {
	loadMethodName := "loadRecords"
	addMethodName := "addRecord"
	updateMethodName := "updateRecord"
	deleteMethodName := "deleteRecord"
	trimmedWorkspacePath := strings.TrimSpace(workspacePath)
	trimmedRecordType := strings.TrimSpace(recordType)
	if trimmedWorkspacePath == "" || trimmedRecordType == "" {
		return loadMethodName, addMethodName, updateMethodName, deleteMethodName
	}
	surface := builderRuntimePrimaryCollectionSurfaceCandidate(trimmedWorkspacePath)
	repositoryPath := strings.TrimSpace(surface.repository.path)
	if repositoryPath == "" {
		repositoryPath = "lib/repositories/record_repository.dart"
	}
	repositoryContent, err := builderRuntimeSemanticFileContent(trimmedWorkspacePath, nil, repositoryPath)
	if err != nil || strings.TrimSpace(repositoryContent) == "" {
		return loadMethodName, addMethodName, updateMethodName, deleteMethodName
	}
	loadPattern := regexp.MustCompile(`Future<List<` + regexp.QuoteMeta(trimmedRecordType) + `>>\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	if match := loadPattern.FindStringSubmatch(repositoryContent); len(match) >= 2 {
		if candidate := strings.TrimSpace(match[1]); candidate != "" {
			loadMethodName = candidate
		}
	}
	addPattern := regexp.MustCompile(`Future<void>\s+(add[A-Z][A-Za-z0-9_]*)\s*\(\s*` + regexp.QuoteMeta(trimmedRecordType) + `\s+[A-Za-z_][A-Za-z0-9_]*\s*\)`)
	if match := addPattern.FindStringSubmatch(repositoryContent); len(match) >= 2 {
		if candidate := strings.TrimSpace(match[1]); candidate != "" {
			addMethodName = candidate
		}
	}
	updatePattern := regexp.MustCompile(`Future<void>\s+(update[A-Z][A-Za-z0-9_]*)\s*\(\s*` + regexp.QuoteMeta(trimmedRecordType) + `\s+[A-Za-z_][A-Za-z0-9_]*\s*\)`)
	if match := updatePattern.FindStringSubmatch(repositoryContent); len(match) >= 2 {
		if candidate := strings.TrimSpace(match[1]); candidate != "" {
			updateMethodName = candidate
		}
	}
	deletePattern := regexp.MustCompile(`Future<void>\s+(delete[A-Z][A-Za-z0-9_]*)\s*\(\s*String\s+[A-Za-z_][A-Za-z0-9_]*\s*\)`)
	if match := deletePattern.FindStringSubmatch(repositoryContent); len(match) >= 2 {
		if candidate := strings.TrimSpace(match[1]); candidate != "" {
			deleteMethodName = candidate
		}
	}
	return loadMethodName, addMethodName, updateMethodName, deleteMethodName
}

func builderRuntimeOpenLiteCanonicalNoFilterListPage(workspacePath string) string {
	entry := builderRuntimeOpenLiteCollectionSurfaceRegistry(workspacePath).view
	recordType := strings.TrimSpace(entry.modelType)
	if recordType == "" {
		recordType = "TodoItem"
	}
	fieldSemantics := entry.fieldSemantics
	primaryTextField := strings.TrimSpace(fieldSemantics.primaryTextField)
	if primaryTextField == "" {
		return ""
	}
	primaryTextFieldType := strings.TrimSpace(fieldSemantics.primaryTextFieldType)
	secondaryTextField := strings.TrimSpace(fieldSemantics.secondaryTextField)
	statusField := strings.TrimSpace(fieldSemantics.statusField)
	statusCopyLabelMethodName := strings.TrimSpace(fieldSemantics.statusCopyLabelMethodName)
	if statusField != "" && statusCopyLabelMethodName == "" {
		statusCopyLabelMethodName = "statusLabel"
	}
	timeField := strings.TrimSpace(fieldSemantics.timeField)
	className := strings.TrimSpace(entry.resolvedClassName)
	if className == "" || className == "HomePage" {
		className = "RecordListPage"
	}
	controllerType := strings.TrimSpace(entry.constructorContract.controllerType)
	if controllerType == "" || controllerType == "HomeController" {
		controllerType = "RecordListController"
	}
	controllerParam := strings.TrimSpace(entry.constructorContract.controllerParam)
	if controllerParam == "" {
		controllerParam = "controller"
	}
	detailCallbackName := strings.TrimSpace(entry.constructorContract.detailCallbackName)
	if detailCallbackName == "" {
		detailCallbackName = "onOpenRecordDetail"
	}
	detailCallbackType := strings.TrimSpace(entry.constructorContract.detailCallbackType)
	if detailCallbackType == "" {
		detailCallbackType = "Future<void> Function(" + recordType + " record)"
	}
	createCallbackName := strings.TrimSpace(entry.constructorContract.createCallbackName)
	if createCallbackName == "" {
		createCallbackName = "onCreateRecord"
	}
	controllerImportPath := strings.TrimSpace(entry.controllerImportPath)
	if controllerImportPath == "" || !strings.Contains(controllerImportPath, "record_list_controller.dart") {
		controllerImportPath = "../controllers/record_list_controller.dart"
	}
	modelImportPath := strings.TrimSpace(entry.modelImportPath)
	if modelImportPath == "" {
		modelImportPath = "../models/record.dart"
	}
	copyImportPath := builderRuntimeOpenLiteRelativeImport(strings.TrimSpace(entry.resolvedPath), "lib/template/open_lite_copy.dart", "../template/open_lite_copy.dart")
	rowChildren := []string{
		"                    Text(",
		"                      " + builderRuntimeOpenLiteValueDisplayExpression("record."+primaryTextField, primaryTextFieldType) + ",",
		"                      style: const TextStyle(fontWeight: FontWeight.w700),",
		"                    ),",
	}
	subtitleParts := make([]string, 0, 2)
	if secondaryTextField != "" {
		subtitleParts = append(subtitleParts, "record."+secondaryTextField)
	}
	if timeField != "" {
		subtitleParts = append(subtitleParts, "_formatDate(record."+timeField+")")
	}
	for _, field := range fieldSemantics.numericFields {
		subtitleParts = append(subtitleParts, builderRuntimeOpenLiteNumericDisplayExpression("record."+field.name, field))
	}
	for _, field := range fieldSemantics.booleanFields {
		subtitleParts = append(subtitleParts, builderRuntimeOpenLiteBooleanFieldValueExpression("record."+field.name, field))
	}
	if statusField != "" {
		subtitleParts = append(subtitleParts, "openLiteCopy."+statusCopyLabelMethodName+"(record."+statusField+")")
	}
	subtitle := builderRuntimeOpenLiteInterpolatedExpression(subtitleParts)
	if subtitle != "" {
		rowChildren = append(rowChildren,
			"                    const SizedBox(height: 4),",
			"                    Text(",
			"                      "+subtitle+",",
			"                    ),",
		)
	}
	lines := []string{
		"import 'package:flutter/material.dart';",
		"",
		"import '" + controllerImportPath + "';",
		"import '" + modelImportPath + "';",
		"import '" + copyImportPath + "';",
		"",
		"class " + className + " extends StatelessWidget {",
		"  const " + className + "({",
		"    super.key,",
		"    required this." + controllerParam + ",",
		"    required this." + detailCallbackName + ",",
		"    required this." + createCallbackName + ",",
		"  });",
		"",
		"  final " + controllerType + " " + controllerParam + ";",
		"  final " + detailCallbackType + " " + detailCallbackName + ";",
		"  final Future<void> Function() " + createCallbackName + ";",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return AnimatedBuilder(",
		"      animation: " + controllerParam + ",",
		"      builder: (context, _) {",
		"        final records = " + controllerParam + ".records;",
		"        return Scaffold(",
		"          appBar: AppBar(title: Text(openLiteCopy.listPageTitle)),",
		"          body: records.isEmpty",
		"              ? Center(child: Text(openLiteCopy.listEmptyLabel))",
		"              : ListView(",
		"                  padding: const EdgeInsets.all(20),",
		"                  children: [",
		"                    ...List<Widget>.generate(records.length, (index) {",
		"                      final record = records[index];",
		"                      return Padding(",
		"                        padding: EdgeInsets.only(bottom: index == records.length - 1 ? 0 : 12),",
		"                        child: _RecordRow(",
		"                          record: record,",
		"                          onTap: () => " + detailCallbackName + "(record),",
		"                        ),",
		"                      );",
		"                    }),",
		"                  ],",
		"                ),",
		"          floatingActionButton: FloatingActionButton.extended(",
		"            onPressed: () => " + createCallbackName + "(),",
		"            icon: const Icon(Icons.add),",
		"            label: Text(openLiteCopy.createPrimaryActionLabel),",
		"          ),",
		"        );",
		"      },",
		"    );",
		"  }",
		"}",
		"",
		"class _RecordRow extends StatelessWidget {",
		"  const _RecordRow({required this.record, required this.onTap});",
		"",
		"  final " + recordType + " record;",
		"  final VoidCallback onTap;",
		"",
		builderRuntimeOpenLiteFormatDateMethod(timeField != ""),
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return Material(",
		"      color: Theme.of(context).colorScheme.surface,",
		"      borderRadius: BorderRadius.circular(18),",
		"      child: InkWell(",
		"        onTap: onTap,",
		"        borderRadius: BorderRadius.circular(18),",
		"        child: Padding(",
		"          padding: const EdgeInsets.all(16),",
		"          child: Row(",
		"            children: [",
		"              Expanded(",
		"                child: Column(",
		"                  crossAxisAlignment: CrossAxisAlignment.start,",
		"                  children: [",
	}
	lines = append(lines, rowChildren...)
	lines = append(lines,
		"                  ],",
		"                ),",
		"              ),",
		"            ],",
		"          ),",
		"        ),",
		"      ),",
		"    );",
		"  }",
		"}",
	)
	return strings.Join(lines, "\n") + "\n"
}

func builderRuntimeOpenLiteInterpolatedExpression(parts []string) string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			filtered = append(filtered, trimmed)
		}
	}
	if len(filtered) == 0 {
		return ""
	}
	if len(filtered) == 1 {
		return filtered[0]
	}
	var builder strings.Builder
	builder.WriteString("'")
	for index, part := range filtered {
		if index > 0 {
			builder.WriteString(" · ")
		}
		builder.WriteString("${")
		builder.WriteString(part)
		builder.WriteString("}")
	}
	builder.WriteString("'")
	return builder.String()
}

func builderRuntimeOpenLiteFormatDateMethod(enabled bool) string {
	if !enabled {
		return ""
	}
	return "  String _formatDate(DateTime value) => '${value.year}-${value.month.toString().padLeft(2, '0')}-${value.day.toString().padLeft(2, '0')}';"
}

// ── Generic deterministic generators（from-scratch、registry-driven）───────────────────

// builderRuntimeOpenLiteCanonicalGenericInspectionPage 从 registry 生成 generic 详情页。
func builderRuntimeOpenLiteCanonicalGenericInspectionPage(workspacePath string) string {
	return builderRuntimeOpenLiteCanonicalGenericInspectionPageWithDelete(workspacePath, false)
}

func builderRuntimeOpenLiteCanonicalGenericInspectionPageWithDelete(workspacePath string, allowDeleteFlow bool) string {
	detailRegistry := builderRuntimeOpenLiteDetailSurfaceRegistry(workspacePath)
	collectionRegistry := builderRuntimeOpenLiteCollectionSurfaceRegistry(workspacePath)
	modelType := strings.TrimSpace(collectionRegistry.view.modelType)
	if modelType == "" {
		modelType = "TodoItem"
	}
	className := strings.TrimSpace(detailRegistry.view.resolvedClassName)
	if className == "" {
		className = "RecordDetailPage"
	}
	recordParam := strings.TrimSpace(detailRegistry.view.constructorContract.recordParam)
	if recordParam == "" {
		recordParam = "record"
	}
	editCallbackName := strings.TrimSpace(detailRegistry.view.constructorContract.editCallbackName)
	hasEdit := editCallbackName != ""
	deleteCallbackName := "onDeleteRecord"
	modelImportPath := strings.TrimSpace(collectionRegistry.view.modelImportPath)
	if modelImportPath == "" {
		modelImportPath = "../models/record.dart"
	}
	copyImportPath := builderRuntimeOpenLiteRelativeImport(strings.TrimSpace(detailRegistry.view.resolvedPath), "lib/template/open_lite_copy.dart", "../template/open_lite_copy.dart")
	fs := collectionRegistry.view.fieldSemantics
	primaryTextField := strings.TrimSpace(fs.primaryTextField)
	primaryTextFieldType := strings.TrimSpace(fs.primaryTextFieldType)
	secondaryTextField := strings.TrimSpace(fs.secondaryTextField)
	statusField := strings.TrimSpace(fs.statusField)
	statusMethod := strings.TrimSpace(fs.statusCopyLabelMethodName)
	if statusField != "" && statusMethod == "" {
		statusMethod = "statusLabel"
	}
	timeField := strings.TrimSpace(fs.timeField)
	numericFields := fs.numericFields
	booleanFields := fs.booleanFields
	noteField := strings.TrimSpace(fs.noteField)
	constructorParams := []string{"super.key", "required this." + recordParam}
	if hasEdit {
		constructorParams = append(constructorParams, "required this."+editCallbackName)
	}
	if allowDeleteFlow {
		constructorParams = append(constructorParams, "required this."+deleteCallbackName)
	}
	fields := []string{"  final " + modelType + " " + recordParam + ";"}
	if hasEdit {
		fields = append(fields, "  final Future<void> Function("+modelType+" "+recordParam+") "+editCallbackName+";")
	}
	if allowDeleteFlow {
		fields = append(fields, "  final Future<void> Function("+modelType+" "+recordParam+") "+deleteCallbackName+";")
	}
	bodyChildren := []string{
		"          _DetailCard(",
		"            title: " + builderRuntimeOpenLiteValueDisplayExpression(recordParam+"."+primaryTextField, primaryTextFieldType) + ",",
	}
	if statusField != "" {
		bodyChildren = append(bodyChildren, "            subtitle: openLiteCopy."+statusMethod+"("+recordParam+"."+statusField+"),")
	} else {
		bodyChildren = append(bodyChildren, "            subtitle: '',")
	}
	bodyChildren = append(bodyChildren, "          ),", "          const SizedBox(height: 16),")
	if statusField != "" {
		bodyChildren = append(bodyChildren, "          _InfoTile(label: openLiteCopy.detailStatusLabel, value: openLiteCopy."+statusMethod+"("+recordParam+"."+statusField+")),")
	}
	if secondaryTextField != "" {
		bodyChildren = append(bodyChildren, "          _InfoTile(label: openLiteCopy.detailCategoryLabel, value: "+recordParam+"."+secondaryTextField+"),")
	}
	if timeField != "" {
		bodyChildren = append(bodyChildren, "          _InfoTile(label: openLiteCopy.detailDateLabel, value: _formatDate("+recordParam+"."+timeField+")),")
	}
	for _, field := range numericFields {
		bodyChildren = append(bodyChildren, "          _InfoTile(label: openLiteCopy."+builderRuntimeOpenLiteNumericDetailLabelGetter(field)+", value: "+builderRuntimeOpenLiteNumericDisplayExpression(recordParam+"."+field.name, field)+"),")
	}
	for _, field := range booleanFields {
		bodyChildren = append(bodyChildren, "          _InfoTile(label: openLiteCopy."+builderRuntimeOpenLiteBooleanDetailLabelGetter(field)+", value: "+builderRuntimeOpenLiteBooleanValueExpression(recordParam+"."+field.name)+"),")
	}
	if noteField != "" {
		bodyChildren = append(bodyChildren, "          _InfoTile(label: openLiteCopy.detailNoteLabel, value: "+recordParam+"."+noteField+".trim().isEmpty ? openLiteCopy.emptyNoteLabel : "+recordParam+"."+noteField+".trim(), multiline: true),")
	}
	bodyChildren = append(bodyChildren, "        ],")
	appBarActions := ""
	if hasEdit || allowDeleteFlow {
		actions := make([]string, 0, 2)
		if hasEdit {
			actions = append(actions, "TextButton(onPressed: () => "+editCallbackName+"("+recordParam+"), child: Text(openLiteCopy.editActionLabel))")
		}
		if allowDeleteFlow {
			actions = append(actions, "IconButton(onPressed: () => "+deleteCallbackName+"("+recordParam+"), icon: const Icon(Icons.delete_outline))")
		}
		appBarActions = "\n      actions: [" + strings.Join(actions, ", ") + "],"
	}
	return strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"", "import '" + modelImportPath + "';", "import '" + copyImportPath + "';", "",
		"class " + className + " extends StatelessWidget {",
		"  const " + className + "({" + strings.Join(constructorParams, ", ") + "});",
		"", strings.Join(fields, "\n"), "",
		builderRuntimeOpenLiteFormatDateMethod(timeField != ""),
		"  @override", "  Widget build(BuildContext context) {",
		"    return Scaffold(",
		"      appBar: AppBar(title: Text(openLiteCopy.detailPageTitle)," + appBarActions,
		"      ),",
		"      body: ListView(padding: const EdgeInsets.all(20), children: [",
		strings.Join(bodyChildren, "\n"),
		"      ),",
		"    );",
		"  }", "}", "",
		"class _DetailCard extends StatelessWidget {",
		"  const _DetailCard({required this.title, required this.subtitle});",
		"  final String title; final String subtitle;",
		"  @override Widget build(BuildContext context) {",
		"    final colors = Theme.of(context).colorScheme;",
		"    return Container(padding: const EdgeInsets.all(20), decoration: BoxDecoration(borderRadius: BorderRadius.circular(24), color: colors.primary),",
		"      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [",
		"        Text(title, style: TextStyle(color: colors.onPrimary, fontSize: 24, fontWeight: FontWeight.w700)),",
		"        const SizedBox(height: 8),",
		"        Text(subtitle, style: TextStyle(color: colors.onPrimary.withValues(alpha: 0.7))),",
		"      ]),", "    );", "  }", "}", "",
		"class _InfoTile extends StatelessWidget {",
		builderRuntimeOpenLiteInfoTileConstructor(noteField != ""),
		builderRuntimeOpenLiteInfoTileFields(noteField != ""),
		"  @override Widget build(BuildContext context) {",
		"    final colors = Theme.of(context).colorScheme;",
		"    return Container(margin: const EdgeInsets.only(bottom: 12), padding: const EdgeInsets.all(16), decoration: BoxDecoration(color: colors.surface, borderRadius: BorderRadius.circular(18)),",
		"      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [",
		"        Text(label, style: Theme.of(context).textTheme.labelMedium),",
		"        const SizedBox(height: 6),",
		builderRuntimeOpenLiteInfoTileValueLine(noteField != ""),
		"      ]),", "    );", "  }", "}",
	}, "\n") + "\n"
}

func builderRuntimeOpenLiteInfoTileConstructor(hasMultiline bool) string {
	if hasMultiline {
		return "  const _InfoTile({required this.label, required this.value, this.multiline = false});"
	}
	return "  const _InfoTile({required this.label, required this.value});"
}

func builderRuntimeOpenLiteInfoTileFields(hasMultiline bool) string {
	if hasMultiline {
		return "  final String label; final String value; final bool multiline;"
	}
	return "  final String label; final String value;"
}

func builderRuntimeOpenLiteInfoTileValueLine(hasMultiline bool) string {
	if hasMultiline {
		return "        Text(value, style: TextStyle(height: multiline ? 1.4 : 1.2)),"
	}
	return "        Text(value, style: const TextStyle(height: 1.2)),"
}

type builderRuntimeOpenLiteNumericMutationParts struct {
	decl    string
	init    string
	dispose string
	body    string
	submit  string
	args    string
}

func builderRuntimeOpenLiteBuildNumericMutationParts(fields []builderRuntimeOpenLiteNumericFieldSemantics, hasEdit bool, initParam string) builderRuntimeOpenLiteNumericMutationParts {
	var parts builderRuntimeOpenLiteNumericMutationParts
	for _, field := range fields {
		if strings.TrimSpace(field.name) == "" || !builderRuntimeOpenLiteIsNumericField(field.fieldType) {
			continue
		}
		controllerName := "_" + field.name + "Controller"
		parts.decl += "\n  late final TextEditingController " + controllerName + ";"
		if hasEdit && strings.TrimSpace(initParam) != "" {
			parts.init += "\n    " + controllerName + " = TextEditingController(text: widget." + initParam + "?." + field.name + ".toString() ?? '');"
		} else {
			parts.init += "\n    " + controllerName + " = TextEditingController();"
		}
		parts.dispose += "\n    " + controllerName + ".dispose();"
		parts.body += "\n              const SizedBox(height: 16),\n              TextField(controller: " + controllerName + ", key: const Key('" + builderRuntimeOpenLiteNumericFieldKey(field) + "'), keyboardType: " + builderRuntimeOpenLiteNumericKeyboardType(field) + ", decoration: InputDecoration(labelText: openLiteCopy." + builderRuntimeOpenLiteNumericFieldLabelGetter(field) + ")),"
		textVar := field.name + "Text"
		parser := builderRuntimeOpenLiteNumericParser(field)
		parts.submit += "\n    final " + textVar + " = " + controllerName + ".text.trim();"
		parts.submit += "\n    final " + field.name + " = " + parser + "(" + textVar + ".isEmpty ? '0' : " + textVar + ");"
		parts.submit += "\n    if (" + field.name + " == null) return;"
		parts.args += ", " + field.name + ": " + field.name
	}
	return parts
}

type builderRuntimeOpenLiteBooleanMutationParts struct {
	decl string
	init string
	body string
	args string
}

func builderRuntimeOpenLiteBuildBooleanMutationParts(fields []builderRuntimeOpenLiteBooleanFieldSemantics, hasEdit bool, initParam string) builderRuntimeOpenLiteBooleanMutationParts {
	var parts builderRuntimeOpenLiteBooleanMutationParts
	for _, field := range fields {
		if strings.TrimSpace(field.name) == "" {
			continue
		}
		stateName := "_" + field.name + "Value"
		parts.decl += "\n  bool " + stateName + " = false;"
		if hasEdit && strings.TrimSpace(initParam) != "" {
			parts.init += "\n    " + stateName + " = widget." + initParam + "?." + field.name + " ?? false;"
		}
		parts.body += "\n              const SizedBox(height: 16),\n              SwitchListTile(key: const Key('" + builderRuntimeOpenLiteBooleanFieldKey(field) + "'), contentPadding: EdgeInsets.zero, title: Text(openLiteCopy." + builderRuntimeOpenLiteBooleanFieldLabelGetter(field) + "), value: " + stateName + ", onChanged: (value) => setState(() => " + stateName + " = value)),"
		parts.args += ", " + field.name + ": " + stateName
	}
	return parts
}

func builderRuntimeOpenLitePrimarySubmitPrefix(primaryField, primaryFieldType string) (string, string) {
	if builderRuntimeOpenLiteIsNumericField(primaryFieldType) {
		field := builderRuntimeOpenLiteNumericFieldSemantics{name: primaryField, fieldType: primaryFieldType, role: builderRuntimeOpenLiteNumericFieldRole(primaryField)}
		textVar := primaryField + "Text"
		prefix := "    final " + textVar + " = _titleController.text.trim();"
		prefix += "\n    final " + primaryField + " = " + builderRuntimeOpenLiteNumericParser(field) + "(" + textVar + ");"
		prefix += "\n    if (" + primaryField + " == null) return;"
		return prefix, primaryField
	}
	return "    final title = _titleController.text.trim(); if (title.isEmpty) return;", "title"
}

func builderRuntimeOpenLitePrimaryKeyboardArgument(primaryFieldType string) string {
	if !builderRuntimeOpenLiteIsNumericField(primaryFieldType) {
		return ""
	}
	field := builderRuntimeOpenLiteNumericFieldSemantics{fieldType: primaryFieldType}
	return " keyboardType: " + builderRuntimeOpenLiteNumericKeyboardType(field) + ","
}

// builderRuntimeOpenLiteCanonicalGenericMutationPage 从 registry 生成 generic 表单页（支持 create + edit）。
func builderRuntimeOpenLiteCanonicalGenericMutationPage(workspacePath string) string {
	mutReg := builderRuntimeOpenLiteMutationSurfaceRegistry(workspacePath)
	colReg := builderRuntimeOpenLiteCollectionSurfaceRegistry(workspacePath)
	modelType := strings.TrimSpace(colReg.view.modelType)
	if modelType == "" || modelType == "TodoItem" {
		modelType = "Record"
	}
	className := strings.TrimSpace(mutReg.view.resolvedClassName)
	if className == "" || className == "HomePage" {
		className = "RecordFormPage"
	}
	repoParam := strings.TrimSpace(mutReg.view.constructorContract.repositoryParam)
	if repoParam == "" {
		repoParam = "repository"
	}
	repoType := strings.TrimSpace(mutReg.controller.repositoryType)
	if repoType == "" {
		repoType = "RecordRepository"
	}
	modelImportPath := strings.TrimSpace(colReg.view.modelImportPath)
	if modelImportPath == "" {
		modelImportPath = "../models/record.dart"
	}
	copyImportPath := "../template/open_lite_copy.dart"
	repoImportPath := "../repositories/record_repository.dart"
	fs := colReg.view.fieldSemantics
	primaryField := strings.TrimSpace(fs.primaryTextField)
	if primaryField == "" {
		primaryField = "title"
	}
	primaryFieldType := strings.TrimSpace(fs.primaryTextFieldType)
	secondaryField := strings.TrimSpace(fs.secondaryTextField)
	if secondaryField == primaryField {
		secondaryField = ""
	}
	statusField := strings.TrimSpace(fs.statusField)
	statusEnumType := strings.TrimSpace(fs.statusEnumType)
	statusEnumMembers := builderRuntimeOpenLiteEnumMembers(workspacePath, statusEnumType)
	if statusField == "" || statusEnumType == "" || len(statusEnumMembers) == 0 {
		statusField = ""
		statusEnumType = ""
	}
	timeField := strings.TrimSpace(fs.timeField)
	noteField := strings.TrimSpace(fs.noteField)
	hasNote := noteField != ""
	secondaryDecl := ""
	secondaryInit := ""
	secondaryDispose := ""
	secondaryArg := ""
	secondaryBody := ""
	if secondaryField != "" {
		secondaryDecl = "\n  late final TextEditingController _secondaryController;"
		secondaryInit = "\n    _secondaryController = TextEditingController();"
		secondaryDispose = "\n    _secondaryController.dispose();"
		secondaryArg = ", " + secondaryField + ": _secondaryController.text.trim()"
		secondaryBody = "\n              const SizedBox(height: 16),\n              TextField(controller: _secondaryController, key: const Key('secondary-field'), decoration: InputDecoration(labelText: openLiteCopy.categoryFieldLabel)),"
	}
	statusDecl := ""
	statusInit := ""
	statusArg := ""
	statusBody := ""
	if statusField != "" {
		defaultStatusMember := ""
		if len(statusEnumMembers) > 0 {
			defaultStatusMember = statusEnumMembers[0]
		}
		defaultStatusExpr := statusEnumType + "." + defaultStatusMember
		if defaultStatusMember == "" {
			defaultStatusExpr = statusEnumType + ".values.first"
		}
		statusDecl = "\n  late " + statusEnumType + " _selectedStatus;"
		statusInit = "\n    _selectedStatus = " + defaultStatusExpr + ";"
		statusArg = ", " + statusField + ": _selectedStatus"
		statusBody = "\n              const SizedBox(height: 16),\n              DropdownButtonFormField<" + statusEnumType + ">(key: const Key('status-field'), value: _selectedStatus, decoration: InputDecoration(labelText: openLiteCopy.detailStatusLabel), items: " + statusEnumType + ".values.map((value) => DropdownMenuItem(value: value, child: Text(openLiteCopy.statusLabel(value)))).toList(), onChanged: (value) { if (value != null) { setState(() => _selectedStatus = value); } }),"
	}
	timeDecl := ""
	timeInit := ""
	timeArg := ""
	timeBody := ""
	pickDateMethod := ""
	if timeField != "" {
		timeDecl = "\n  DateTime _selectedDate = DateTime.now();"
		timeInit = "\n    _selectedDate = DateTime.now();"
		timeArg = ", " + timeField + ": _selectedDate"
		timeBody = "\n              const SizedBox(height: 16),\n              ListTile(key: const Key('date-field'), contentPadding: EdgeInsets.zero, title: Text(openLiteCopy.dateFieldLabel), subtitle: Text(_formatDate(_selectedDate)), trailing: const Icon(Icons.calendar_today), onTap: _pickDate),"
		pickDateMethod = "\n  String _formatDate(DateTime value) => '${value.year}-${value.month.toString().padLeft(2, '0')}-${value.day.toString().padLeft(2, '0')}';\n  Future<void> _pickDate() async {\n    final picked = await showDatePicker(context: context, initialDate: _selectedDate, firstDate: DateTime(2000), lastDate: DateTime(2100));\n    if (picked != null) { setState(() => _selectedDate = picked); }\n  }"
	}
	nodeInit := ""
	if hasNote {
		nodeInit = "\n    _noteController = TextEditingController();"
	}
	notebody := ""
	if hasNote {
		notebody = "\n              const SizedBox(height: 16),\n              TextField(controller: _noteController, key: const Key('note-field'), decoration: InputDecoration(labelText: openLiteCopy.noteFieldLabel), maxLines: 3),"
	}
	nodedisp := ""
	if hasNote {
		nodedisp = "\n    _noteController.dispose();"
	}
	noteDecl := ""
	if hasNote {
		noteDecl = "\n  late final TextEditingController _noteController;"
	}
	noteArg := ""
	if hasNote {
		noteArg = ", " + noteField + ": _noteController.text.trim()"
	}
	initParam := strings.TrimSpace(mutReg.view.constructorContract.initialParam)
	hasEdit := initParam != "" && mutReg.view.capabilityFlags.supportsEdit
	numericParts := builderRuntimeOpenLiteBuildNumericMutationParts(fs.numericFields, hasEdit, initParam)
	booleanParts := builderRuntimeOpenLiteBuildBooleanMutationParts(fs.booleanFields, hasEdit, initParam)
	schemaArgs := secondaryArg + statusArg + timeArg + numericParts.args + booleanParts.args + noteArg
	_, _, updateMethod, _ := builderRuntimeOpenLiteCollectionRepositoryMethodNames(workspacePath, modelType)
	extraParam := ""
	extraField := ""
	if hasEdit {
		extraParam = ", this." + initParam
		extraField = "\n  final " + modelType + "? " + initParam + ";"
	}
	titleInit := "    _titleController = TextEditingController();"
	if hasEdit {
		titleInit = "    _titleController = TextEditingController(text: widget." + initParam + "?." + primaryField + " ?? '');"
		if secondaryField != "" {
			secondaryInit = "\n    _secondaryController = TextEditingController(text: widget." + initParam + "?." + secondaryField + " ?? '');"
		}
		if statusField != "" {
			defaultStatusExpr := statusEnumType + ".values.first"
			if defaultStatusMember := builderRuntimeOpenLiteFirstEnumMember(workspacePath, statusEnumType); defaultStatusMember != "" {
				defaultStatusExpr = statusEnumType + "." + defaultStatusMember
			}
			statusInit = "\n    _selectedStatus = widget." + initParam + "?." + statusField + " ?? " + defaultStatusExpr + ";"
		}
		if timeField != "" {
			timeInit = "\n    _selectedDate = widget." + initParam + "?." + timeField + " ?? DateTime.now();"
		}
	}
	pageTitle := "openLiteCopy.createPageTitle"
	submitLabel := "openLiteCopy.createSubmitLabel"
	if hasEdit {
		pageTitle = "openLiteCopy.editPageTitle"
		submitLabel = "openLiteCopy.editSubmitLabel"
	}
	submitBody := ""
	primarySubmitPrefix, primaryArgValue := builderRuntimeOpenLitePrimarySubmitPrefix(primaryField, primaryFieldType)
	submitPrefix := primarySubmitPrefix + numericParts.submit
	if hasEdit {
		submitBody = submitPrefix + "\n    final record = widget." + initParam + "!.copyWith(" + primaryField + ": " + primaryArgValue + schemaArgs + ");\n    await widget." + repoParam + "." + updateMethod + "(record);\n    if (!mounted) return;\n    Navigator.of(context).pop(record);"
	} else {
		submitBody = submitPrefix + "\n    final record = " + modelType + "(" + primaryField + ": " + primaryArgValue + schemaArgs + ");\n    await widget." + repoParam + ".addRecord(record);\n    if (!mounted) return;\n    Navigator.of(context).pop(record);"
	}
	return strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"", "import '" + modelImportPath + "';", "import '" + repoImportPath + "';", "import '" + copyImportPath + "';", "",
		"class " + className + " extends StatefulWidget {",
		"  const " + className + "({super.key, required this." + repoParam + extraParam + "});",
		"  final " + repoType + " " + repoParam + ";" + extraField,
		"  @override State<" + className + "> createState() => _" + className + "State();",
		"}", "",
		"class _" + className + "State extends State<" + className + "> {",
		"  late final TextEditingController _titleController;" + secondaryDecl + numericParts.decl + booleanParts.decl + noteDecl + statusDecl + timeDecl,
		"  @override void initState() { super.initState();", titleInit + secondaryInit + statusInit + timeInit + numericParts.init + booleanParts.init + nodeInit, "  }",
		"  @override void dispose() { _titleController.dispose();" + secondaryDispose + numericParts.dispose + nodedisp + " super.dispose(); }",
		pickDateMethod,
		"  Future<void> _submit() async {", submitBody, "  }",
		"  @override Widget build(BuildContext context) {",
		"    return Scaffold(appBar: AppBar(title: Text(" + pageTitle + ")),",
		"      body: ListView(padding: const EdgeInsets.all(20), children: [",
		"        TextField(controller: _titleController, key: const Key('title-field')," + builderRuntimeOpenLitePrimaryKeyboardArgument(primaryFieldType) + " decoration: InputDecoration(labelText: openLiteCopy.titleFieldLabel))," + secondaryBody + statusBody + timeBody + numericParts.body + booleanParts.body + notebody,
		"        const SizedBox(height: 24),",
		"        ElevatedButton(onPressed: _submit, child: Text(" + submitLabel + ")),",
		"      ]),",
		"    );",
		"  }", "}",
	}, "\n") + "\n"
}

// builderRuntimeOpenLiteCanonicalGenericOverviewPage 从 registry 生成 generic 主页（含摘要卡片）。
func builderRuntimeOpenLiteCanonicalGenericOverviewPage(workspacePath string) string {
	ovReg := builderRuntimeOpenLiteOverviewSurfaceRegistry(workspacePath)
	colReg := builderRuntimeOpenLiteCollectionSurfaceRegistry(workspacePath)
	className := strings.TrimSpace(ovReg.view.resolvedClassName)
	if className == "" {
		className = "HomePage"
	}
	ctrlType := strings.TrimSpace(ovReg.view.constructorContract.controllerType)
	if ctrlType == "" {
		ctrlType = "HomeController"
	}
	ctrlParam := strings.TrimSpace(ovReg.view.constructorContract.controllerParam)
	if ctrlParam == "" {
		ctrlParam = "controller"
	}
	createCB := strings.TrimSpace(ovReg.view.constructorContract.createCallbackName)
	if createCB == "" {
		createCB = "onCreateRecord"
	}
	viewAllCB := strings.TrimSpace(ovReg.view.constructorContract.viewAllCallbackName)
	if viewAllCB == "" {
		viewAllCB = "onViewAllRecords"
	}
	fs := colReg.view.fieldSemantics
	primaryField := strings.TrimSpace(fs.primaryTextField)
	if primaryField == "" {
		primaryField = "title"
	}
	primaryFieldType := strings.TrimSpace(fs.primaryTextFieldType)
	statusField := strings.TrimSpace(fs.statusField)
	statusEnumType := strings.TrimSpace(fs.statusEnumType)
	completedStatusMember := builderRuntimeOpenLiteCompletedStatusMember(workspacePath, statusEnumType)
	hasCompletedStatus := statusField != "" && statusEnumType != "" && completedStatusMember != ""
	summaryModel := builderRuntimeOpenLiteSummaryModelSemantics(workspacePath)
	hasSummaryMetrics := len(summaryModel.fields) > 0
	hasDoneCount := hasCompletedStatus && !hasSummaryMetrics
	ctrlImportPath := strings.TrimSpace(ovReg.view.controllerImportPath)
	if ctrlImportPath == "" {
		ctrlImportPath = "../controllers/home_controller.dart"
	}
	copyImportPath := builderRuntimeOpenLiteRelativeImport(strings.TrimSpace(ovReg.view.resolvedPath), "lib/template/open_lite_copy.dart", "../template/open_lite_copy.dart")
	modelImport := ""
	if hasDoneCount {
		modelImportPath := strings.TrimSpace(colReg.view.modelImportPath)
		if modelImportPath == "" {
			modelImportPath = "../models/record.dart"
		}
		modelImport = "import '" + modelImportPath + "';"
	}
	totalCountExpression := ctrlParam + ".records.length"
	for _, field := range summaryModel.fields {
		if field.name == "totalCount" {
			totalCountExpression = ctrlParam + ".summary.totalCount"
			break
		}
	}
	summaryCard := "                    _SummaryCard(totalCount: " + totalCountExpression + ","
	if hasDoneCount {
		summaryCard += "\n                      doneCount: " + ctrlParam + ".records.where((r) => r." + statusField + " == " + statusEnumType + "." + completedStatusMember + ").length,"
	}
	if hasSummaryMetrics {
		summaryCard += "\n                      metrics: [\n" + builderRuntimeOpenLiteSummaryMetricItems(ctrlParam+".summary", summaryModel.fields) + "\n                      ],"
	}
	summaryCard += "\n                    ),\n                    const SizedBox(height: 16),"
	recentPreview := "                    if (" + ctrlParam + ".records.isNotEmpty)\n                      Padding(padding: const EdgeInsets.only(bottom: 16),\n                        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [\n                          Text(openLiteCopy.recentRecordsTitle, style: Theme.of(context).textTheme.titleMedium),\n                          const SizedBox(height: 8),\n                          ...List<Widget>.generate(" + ctrlParam + ".records.length.clamp(0, 3), (index) { final r = " + ctrlParam + ".records[index];\n                            return ListTile(dense: true, title: Text(" + builderRuntimeOpenLiteValueDisplayExpression("r."+primaryField, primaryFieldType) + "), contentPadding: EdgeInsets.zero);\n                          }),\n                        ],),\n                      ),"
	return strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"", "import '" + ctrlImportPath + "';", modelImport, "import '" + copyImportPath + "';", "",
		"class " + className + " extends StatelessWidget {",
		"  const " + className + "({super.key, required this." + ctrlParam + ", required this." + createCB + ", required this." + viewAllCB + "});",
		"  final " + ctrlType + " " + ctrlParam + ";",
		"  final Future<void> Function() " + createCB + ";",
		"  final Future<void> Function() " + viewAllCB + ";",
		"  @override Widget build(BuildContext context) {",
		"    return AnimatedBuilder(animation: " + ctrlParam + ", builder: (context, _) {",
		"      return Scaffold(appBar: AppBar(title: Text(openLiteCopy.appTitle)),",
		"        body: " + ctrlParam + ".isLoading ? const Center(child: CircularProgressIndicator()) : ListView(padding: const EdgeInsets.all(20), children: [",
		"          _OverviewActions(" + createCB + ": () => " + createCB + "(), " + viewAllCB + ": () => " + viewAllCB + "()),",
		"          const SizedBox(height: 20),",
		summaryCard,
		recentPreview,
		"        ],),",
		"      );",
		"    },);",
		"  }", "}", "",
		"class _OverviewActions extends StatelessWidget {",
		"  const _OverviewActions({required this." + createCB + ", required this." + viewAllCB + "});",
		"  final Future<void> Function() " + createCB + ";",
		"  final Future<void> Function() " + viewAllCB + ";",
		"  @override Widget build(BuildContext context) {",
		"    return Row(children: [",
		"      Expanded(child: ElevatedButton.icon(onPressed: () => " + createCB + "(), icon: const Icon(Icons.add), label: Text(openLiteCopy.createPrimaryActionLabel))),",
		"      const SizedBox(width: 12),",
		"      Expanded(child: OutlinedButton.icon(onPressed: () => " + viewAllCB + "(), icon: const Icon(Icons.list), label: Text(openLiteCopy.viewAllActionLabel))),",
		"    ]);",
		"  }", "}", "",
		"class _SummaryCard extends StatelessWidget {",
		builderRuntimeOpenLiteSummaryCardConstructor(hasDoneCount, hasSummaryMetrics),
		builderRuntimeOpenLiteSummaryCardFields(hasDoneCount, hasSummaryMetrics),
		"  @override Widget build(BuildContext context) {",
		"    final colors = Theme.of(context).colorScheme;",
		"    return Container(padding: const EdgeInsets.all(20), decoration: BoxDecoration(borderRadius: BorderRadius.circular(20), color: colors.primaryContainer),",
		"      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [",
		"        Row(children: [",
		"          Text('$totalCount', style: TextStyle(fontSize: 36, fontWeight: FontWeight.w700, color: colors.onPrimaryContainer)),",
		"          const SizedBox(width: 12),",
		"          Text(openLiteCopy.summaryCountLabel(totalCount), style: TextStyle(color: colors.onPrimaryContainer.withValues(alpha: 0.8))),",
		builderRuntimeOpenLiteSummaryCardDoneChildren(hasDoneCount),
		"        ]),",
		builderRuntimeOpenLiteSummaryMetricChildren(hasSummaryMetrics),
		"      ]),",
		"    );",
		"  }", "}",
		builderRuntimeOpenLiteSummaryMetricClass(hasSummaryMetrics),
	}, "\n") + "\n"
}

func builderRuntimeOpenLiteSummaryCardConstructor(hasDoneCount, hasSummaryMetrics bool) string {
	params := []string{"required this.totalCount"}
	if hasDoneCount {
		params = append(params, "required this.doneCount")
	}
	if hasSummaryMetrics {
		params = append(params, "required this.metrics")
	}
	return "  const _SummaryCard({" + strings.Join(params, ", ") + "});"
}

func builderRuntimeOpenLiteSummaryCardFields(hasDoneCount, hasSummaryMetrics bool) string {
	fields := []string{"  final int totalCount;"}
	if hasDoneCount {
		fields = append(fields, "  final int doneCount;")
	}
	if hasSummaryMetrics {
		fields = append(fields, "  final List<_SummaryMetric> metrics;")
	}
	return strings.Join(fields, "\n")
}

func builderRuntimeOpenLiteSummaryCardDoneChildren(hasCompletedStatus bool) string {
	if !hasCompletedStatus {
		return ""
	}
	return "          const Spacer(), Text('$doneCount ${openLiteCopy.doneFilterLabel}', style: TextStyle(color: colors.onPrimaryContainer.withValues(alpha: 0.8))),"
}

func builderRuntimeOpenLiteSummaryMetricItems(summaryAccessor string, fields []builderRuntimeOpenLiteDeclaredField) string {
	items := make([]string, 0, len(fields))
	for _, field := range fields {
		getter := builderRuntimeOpenLiteSummaryMetricLabelGetter(field.name)
		if getter == "" {
			continue
		}
		items = append(items, "                        _SummaryMetric(label: openLiteCopy."+getter+", value: "+builderRuntimeOpenLiteSummaryMetricValueExpression(summaryAccessor, field)+"),")
	}
	return strings.Join(items, "\n")
}

func builderRuntimeOpenLiteSummaryMetricChildren(hasSummaryMetrics bool) string {
	if !hasSummaryMetrics {
		return ""
	}
	return "        const SizedBox(height: 12),\n        Wrap(spacing: 8, runSpacing: 8, children: metrics.map((metric) => Chip(label: Text('${metric.label}: ${metric.value}'))).toList()),"
}

func builderRuntimeOpenLiteSummaryMetricClass(hasSummaryMetrics bool) string {
	if !hasSummaryMetrics {
		return ""
	}
	return strings.Join([]string{
		"",
		"class _SummaryMetric {",
		"  const _SummaryMetric({required this.label, required this.value});",
		"  final String label;",
		"  final String value;",
		"}",
	}, "\n")
}

// builderRuntimeOpenLiteCanonicalGenericOverviewController 从 registry 生成 generic 主页控制器。
func builderRuntimeOpenLiteCanonicalGenericOverviewController(workspacePath string) string {
	return builderRuntimeOpenLiteCanonicalGenericOverviewControllerWithDelete(workspacePath, false)
}

func builderRuntimeOpenLiteCanonicalGenericOverviewControllerWithDelete(workspacePath string, allowDeleteFlow bool) string {
	ovReg := builderRuntimeOpenLiteOverviewSurfaceRegistry(workspacePath)
	colReg := builderRuntimeOpenLiteCollectionSurfaceRegistry(workspacePath)
	className := strings.TrimSpace(ovReg.controller.resolvedClassName)
	if className == "" {
		className = "HomeController"
	}
	repoType := strings.TrimSpace(ovReg.controller.repositoryType)
	if repoType == "" {
		repoType = "RecordRepository"
	}
	repoParam := strings.TrimSpace(ovReg.controller.constructorContract.repositoryParam)
	if repoParam == "" {
		repoParam = "repository"
	}
	repoImportPath := strings.TrimSpace(ovReg.controller.repositoryImportPath)
	if repoImportPath == "" {
		repoImportPath = "../repositories/record_repository.dart"
	}
	recordType := strings.TrimSpace(colReg.view.modelType)
	if recordType == "" {
		recordType = "TodoItem"
	}
	modelImportPath := strings.TrimSpace(colReg.view.modelImportPath)
	if modelImportPath == "" {
		modelImportPath = "../models/record.dart"
	}
	summaryModel := builderRuntimeOpenLiteSummaryModelSemantics(workspacePath)
	hasSummaryModel := summaryModel.className != ""
	summaryImport := ""
	summaryField := ""
	summaryGetter := ""
	summaryRefresh := ""
	if hasSummaryModel {
		summaryImport = "import '../models/dashboard_summary.dart';"
		summaryField = "  " + summaryModel.className + " _summary = const " + summaryModel.className + "();"
		summaryGetter = "  " + summaryModel.className + " get summary => _summary;"
		summaryRefresh = " _summary = await _repository.loadSummary();"
	}
	loadMethod, _, _, deleteMethod := builderRuntimeOpenLiteCollectionRepositoryMethodNames(workspacePath, recordType)
	lines := []string{
		"import 'package:flutter/foundation.dart' show ChangeNotifier;",
		"", "import '" + modelImportPath + "';",
	}
	if summaryImport != "" {
		lines = append(lines, summaryImport)
	}
	lines = append(lines,
		"import '"+repoImportPath+"';", "",
		"class "+className+" extends ChangeNotifier {",
		"  "+className+"({required "+repoType+" "+repoParam+"}) : _repository = "+repoParam+" { _init(); }",
		"  final "+repoType+" _repository;",
		"  List<"+recordType+"> _records = [];",
	)
	if summaryField != "" {
		lines = append(lines, summaryField)
	}
	lines = append(lines,
		"  bool _isLoading = true;",
		"  List<"+recordType+"> get records => List.unmodifiable(_records);",
	)
	if summaryGetter != "" {
		lines = append(lines, summaryGetter)
	}
	lines = append(lines,
		"  bool get isLoading => _isLoading;",
		"  Future<void> _init() async { await refresh(); }",
		"  Future<void> refresh() async {",
		"    _isLoading = true; notifyListeners();",
		"    try { _records = await _repository."+loadMethod+"();"+summaryRefresh+" } finally { _isLoading = false; notifyListeners(); }",
		"  }",
	)
	if allowDeleteFlow {
		lines = append(lines,
			"  Future<void> deleteRecord(String recordId) async {",
			"    await _repository."+deleteMethod+"(recordId);",
			"    await refresh();",
			"  }",
		)
	}
	lines = append(lines, "}")
	return strings.Join(lines, "\n") + "\n"
}

// builderRuntimeOpenLiteCanonicalGenericAppEntry 从各 surface registry 生成 generic main.dart。
// 注意：该函数在运行时不依赖 registry 的 resolvedPath/ClassName（workspace 中文件可能尚未生成）。
// 使用模板默认值以确保稳定性。
func builderRuntimeOpenLiteCanonicalGenericAppEntry(workspacePath string) string {
	return builderRuntimeOpenLiteCanonicalGenericAppEntryWithDelete(workspacePath, false)
}

func builderRuntimeOpenLiteCanonicalGenericAppEntryWithDelete(workspacePath string, allowDeleteFlow bool) string {
	appClassName := "AppFactoryApp"
	concRepoType := builderRuntimeOpenLiteConcreteRecordRepositoryTypeName(workspacePath)
	if concRepoType == "" {
		concRepoType = "HiveRecordRepository"
	}
	deleteIdentifierField := strings.TrimSpace(builderRuntimeOpenLiteCollectionSurfaceRegistry(workspacePath).view.fieldSemantics.identifierField)
	if deleteIdentifierField == "" {
		deleteIdentifierField = "recordId"
	}
	detailPageLine := "      MaterialPageRoute(builder: (context) => RecordDetailPage(record: record)),"
	deleteMethodLines := []string{}
	if allowDeleteFlow {
		detailPageLine = "      MaterialPageRoute(builder: (context) => RecordDetailPage(record: record, onDeleteRecord: _deleteRecord)),"
		deleteMethodLines = []string{
			"  Future<void> _deleteRecord(dynamic record) async {",
			"    await _overviewController.deleteRecord(record." + deleteIdentifierField + ");",
			"    await _collectionController.refresh();",
			"    _navigatorKey.currentState!.pop(true);",
			"  }",
			"",
		}
	}
	lines := []string{
		"import 'package:flutter/material.dart';",
		"",
		"import 'controllers/home_controller.dart';",
		"import 'controllers/record_list_controller.dart';",
		"import 'views/home_page.dart';",
		"import 'views/record_list_page.dart';",
		"import 'views/record_form_page.dart';",
		"import 'views/record_detail_page.dart';",
		"import 'repositories/record_repository.dart';",
		"import 'template/open_lite_copy.dart';",
		"",
		"Future<void> main() async {",
		"  WidgetsFlutterBinding.ensureInitialized();",
		"  final repository = " + concRepoType + "();",
		"  await repository.init();",
		"  runApp(" + appClassName + "(repository: repository));",
		"}",
		"",
		"class " + appClassName + " extends StatefulWidget {",
		"  const " + appClassName + "({super.key, required this.repository});",
		"  final RecordRepository repository;",
		"  @override",
		"  State<" + appClassName + "> createState() => _" + appClassName + "State();",
		"}",
		"",
		"class _" + appClassName + "State extends State<" + appClassName + "> {",
		"  final GlobalKey<NavigatorState> _navigatorKey = GlobalKey<NavigatorState>();",
		"  late final HomeController _overviewController;",
		"  late final RecordListController _collectionController;",
		"",
		"  @override",
		"  void initState() {",
		"    super.initState();",
		"    _overviewController = HomeController(repository: widget.repository);",
		"    _collectionController = RecordListController(repository: widget.repository);",
		"  }",
		"",
		"  @override",
		"  void dispose() {",
		"    _collectionController.dispose();",
		"    _overviewController.dispose();",
		"    super.dispose();",
		"  }",
		"",
		"  Future<void> _openCreateRecord() async {",
		"    final created = await _navigatorKey.currentState!.push(",
		"      MaterialPageRoute(builder: (context) => RecordFormPage(repository: widget.repository)),",
		"    );",
		"    if (created != null) { await _overviewController.refresh(); await _collectionController.refresh(); }",
		"  }",
		"",
		"  Future<void> _openDetail(dynamic record) async {",
		"    final deleted = await _navigatorKey.currentState!.push(",
		detailPageLine,
		"    );",
		"    if (deleted == true) { await _overviewController.refresh(); }",
		"  }",
		"",
	}
	lines = append(lines, deleteMethodLines...)
	lines = append(lines,
		"  Future<void> _openCollection() async {",
		"    await _navigatorKey.currentState!.push(",
		"      MaterialPageRoute(builder: (context) => RecordListPage(",
		"        controller: _collectionController,",
		"        onOpenRecordDetail: _openDetail,",
		"        onCreateRecord: _openCreateRecord,",
		"      )),",
		"    );",
		"  }",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return MaterialApp(",
		"      navigatorKey: _navigatorKey,",
		"      title: openLiteCopy.appTitle,",
		"      theme: ThemeData(primarySwatch: Colors.blue, useMaterial3: true),",
		"      initialRoute: '/',",
		"      routes: {",
		"        '/': (context) => HomePage(controller: _overviewController, onCreateRecord: _openCreateRecord, onViewAllRecords: _openCollection),",
		"        '/collection': (context) => RecordListPage(controller: _collectionController, onOpenRecordDetail: _openDetail, onCreateRecord: _openCreateRecord),",
		"      },",
		"    );",
		"  }",
		"}",
	)
	return strings.Join(lines, "\n") + "\n"
}
func normalizeBuilderRuntimeOpenLiteWidgetTestContent(workspacePath string, needsCollectionCreateEntry bool, content string) string {
	if strings.TrimSpace(content) == "" {
		return content
	}
	updated := content
	mutationRegistry := builderRuntimeOpenLiteMutationSurfaceRegistry(workspacePath)
	if !needsCollectionCreateEntry || strings.TrimSpace(mutationRegistry.controller.resolvedPath) == "" {
		return updated
	}
	hasTitleField := mutationRegistry.controller.capabilityFlags.hasTitleField
	hasNoteField := mutationRegistry.controller.capabilityFlags.hasNoteField
	hasStatusFlow := mutationRegistry.controller.capabilityFlags.hasStatus
	hasCreateSubmitFlow := hasTitleField
	hasEditMutationFlow := mutationRegistry.view.capabilityFlags.supportsEdit
	packageName := workspaceDartPackageName(workspacePath)
	if packageName == "" {
		packageName = "flutter_open_lite"
	}
	appClassName := "MyApp"
	if mainContent, err := builderRuntimeSemanticFileContent(workspacePath, nil, "lib/main.dart"); err == nil {
		if currentAppClassName := builderRuntimeRunAppWidgetClassName(mainContent); currentAppClassName != "" {
			appClassName = currentAppClassName
		}
	}
	if appClassName == "MyApp" {
		if match := builderRuntimeWidgetTestPumpWidgetRepositoryPattern.FindStringSubmatch(updated); len(match) >= 2 {
			if currentAppClassName := strings.TrimSpace(match[1]); currentAppClassName != "" {
				appClassName = currentAppClassName
			}
		}
	}
	repositoryImportPath := builderRuntimeOpenLiteWidgetTestRepositoryImportPath(workspacePath)
	repositoryType := builderRuntimeOpenLitePreferredWidgetTestRepositoryTypeName(workspacePath)
	lines := []string{
		"import 'package:flutter/material.dart';",
		"import 'package:flutter_test/flutter_test.dart';",
		"",
		"import 'package:" + packageName + "/main.dart';",
		"import 'package:" + packageName + "/" + repositoryImportPath + "';",
		"import 'package:" + packageName + "/template/open_lite_copy.dart';",
		"",
		"void main() {",
		"  testWidgets('open lite flow supports create, read, update', (WidgetTester tester) async {",
		"    final repository = " + repositoryType + "();",
		"    await repository.init();",
		"",
		"    await tester.pumpWidget(" + appClassName + "(repository: repository));",
		"    await tester.pumpAndSettle();",
		"",
		"    expect(find.text(openLiteCopy.listPageTitle), findsOneWidget);",
		"    expect(find.text(openLiteCopy.createPrimaryActionLabel), findsOneWidget);",
		"",
		"    await tester.tap(find.text(openLiteCopy.createPrimaryActionLabel));",
		"    await tester.pumpAndSettle();",
	}
	if hasCreateSubmitFlow {
		lines = append(lines,
			"",
			"    await tester.enterText(find.byKey(const Key('title-field')), '测试任务');",
		)
		if hasNoteField {
			lines = append(lines, "    await tester.enterText(find.byKey(const Key('note-field')), '这是一个测试备注');")
		}
		lines = append(lines,
			"    await tester.tap(find.text(openLiteCopy.createSubmitLabel));",
			"    await tester.pumpAndSettle();",
			"",
			"    expect(find.text('测试任务'), findsOneWidget);",
		)
	} else {
		lines = append(lines,
			"",
			"    expect(find.text(openLiteCopy.createPageTitle), findsOneWidget);",
			"    expect(find.text(openLiteCopy.createSubmitLabel), findsOneWidget);",
		)
	}
	if hasEditMutationFlow {
		lines = append(lines,
			"",
			"    await tester.tap(find.text('测试任务').first);",
			"    await tester.pumpAndSettle();",
			"",
			"    expect(find.text(openLiteCopy.editPageTitle), findsOneWidget);",
			"    await tester.enterText(find.byKey(const Key('title-field')), '更新后的测试任务');",
		)
		if hasNoteField {
			lines = append(lines, "    await tester.enterText(find.byKey(const Key('note-field')), '这是一个更新后的测试备注');")
		}
	}
	if hasStatusFlow && hasEditMutationFlow {
		lines = append(lines,
			"    await tester.tap(find.text(openLiteCopy.doneFilterLabel));",
			"    await tester.pumpAndSettle();",
		)
	}
	if hasEditMutationFlow {
		lines = append(lines,
			"    await tester.tap(find.text(openLiteCopy.editSubmitLabel));",
			"    await tester.pumpAndSettle();",
			"",
			"    expect(find.text('更新后的测试任务'), findsOneWidget);",
		)
	}
	if hasStatusFlow && hasEditMutationFlow {
		lines = append(lines, "    expect(find.textContaining(openLiteCopy.doneFilterLabel), findsWidgets);")
	}
	lines = append(lines,
		"  });",
		"}",
	)
	return strings.Join(lines, "\n") + "\n"
}

func builderRuntimeOpenLiteMutationControllerHasField(workspacePath, marker string) bool {
	if strings.TrimSpace(marker) == "" {
		return false
	}
	controllerContent := builderRuntimeOpenLiteMutationControllerContent(workspacePath)
	if strings.TrimSpace(controllerContent) == "" {
		return false
	}
	return strings.Contains(controllerContent, marker)
}

func builderRuntimeOpenLiteEnsureDebugPrintImport(content string) string {
	if strings.TrimSpace(content) == "" || !strings.Contains(content, "debugPrint(") {
		return content
	}
	if strings.Contains(content, "import 'package:flutter/material.dart';") || strings.Contains(content, "import 'package:flutter/foundation.dart';") || strings.Contains(content, "import 'package:flutter/foundation.dart' show ChangeNotifier, debugPrint;") {
		return content
	}
	if strings.Contains(content, "import 'package:flutter/foundation.dart' show ChangeNotifier;") {
		return strings.Replace(content, "import 'package:flutter/foundation.dart' show ChangeNotifier;", "import 'package:flutter/foundation.dart' show ChangeNotifier, debugPrint;", 1)
	}
	if strings.Contains(content, "import 'package:flutter/foundation.dart' show debugPrint;") {
		return strings.Replace(content, "import 'package:flutter/foundation.dart' show debugPrint;", "import 'package:flutter/foundation.dart' show ChangeNotifier, debugPrint;", 1)
	}
	return "import 'package:flutter/foundation.dart' show ChangeNotifier, debugPrint;\n" + content
}

func builderRuntimeOpenLiteDropImportIfUnused(content, importLine string, usedMarkers ...string) string {
	if strings.TrimSpace(content) == "" || strings.TrimSpace(importLine) == "" || !strings.Contains(content, importLine) {
		return content
	}
	withoutImport := strings.Replace(content, importLine+"\n", "", 1)
	if withoutImport == content {
		withoutImport = strings.Replace(content, importLine, "", 1)
	}
	for _, marker := range usedMarkers {
		if marker != "" && strings.Contains(withoutImport, marker) {
			return content
		}
	}
	return withoutImport
}

func builderRuntimeOpenLiteDropControllerImportIfUnused(content, controllerClass string) string {
	trimmedClass := strings.TrimSpace(controllerClass)
	if trimmedClass == "" || strings.Contains(content, trimmedClass) {
		return content
	}
	controllerFile := builderRuntimeOpenLiteIdentifierSnakeCase(trimmedClass)
	if controllerFile == "" {
		return content
	}
	updated := builderRuntimeOpenLiteDropImportIfUnused(content, "import 'controllers/"+controllerFile+".dart';", trimmedClass)
	updated = builderRuntimeOpenLiteDropImportIfUnused(updated, "import '../controllers/"+controllerFile+".dart';", trimmedClass)
	return updated
}

func builderRuntimeOpenLiteIdentifierSnakeCase(identifier string) string {
	trimmedIdentifier := strings.TrimSpace(identifier)
	if trimmedIdentifier == "" {
		return ""
	}
	var builder strings.Builder
	for index, current := range trimmedIdentifier {
		if unicode.IsUpper(current) {
			if index > 0 {
				previous, _ := utf8DecodeLastRuneInString(builder.String())
				if previous != '_' {
					builder.WriteByte('_')
				}
			}
			builder.WriteRune(unicode.ToLower(current))
			continue
		}
		if current == '-' || current == ' ' {
			if builder.Len() > 0 {
				previous, _ := utf8DecodeLastRuneInString(builder.String())
				if previous != '_' {
					builder.WriteByte('_')
				}
			}
			continue
		}
		builder.WriteRune(unicode.ToLower(current))
	}
	return strings.Trim(builder.String(), "_")
}

func utf8DecodeLastRuneInString(value string) (rune, int) {
	if value == "" {
		return rune(0), 0
	}
	runes := []rune(value)
	if len(runes) == 0 {
		return rune(0), 0
	}
	return runes[len(runes)-1], len(string(runes[len(runes)-1]))
}

func builderRuntimeOpenLiteNeedsCollectionCreateEntryFromWorkspace(workspacePath string) bool {
	if strings.TrimSpace(workspacePath) == "" {
		return false
	}
	if builderRuntimeOpenLiteWorkspaceHasOverviewSurface(workspacePath) {
		return false
	}
	collectionRegistry := builderRuntimeOpenLiteCollectionSurfaceRegistry(workspacePath)
	collectionControllerPath := strings.TrimSpace(collectionRegistry.controller.resolvedPath)
	collectionViewPath := strings.TrimSpace(collectionRegistry.view.resolvedPath)
	hasCollectionSurface := builderRuntimeWorkspaceHasSemanticFile(workspacePath, collectionViewPath) || builderRuntimeWorkspaceHasSemanticFile(workspacePath, collectionControllerPath)
	if !hasCollectionSurface {
		collectionControllerPath = builderRuntimeOpenLiteFirstLikelyControllerPath(workspacePath, builderRuntimeOpenLiteLikelyCollectionControllerPath)
		hasCollectionSurface = strings.TrimSpace(collectionControllerPath) != ""
	}
	if !hasCollectionSurface {
		return false
	}
	mutationRegistry := builderRuntimeOpenLiteMutationSurfaceRegistry(workspacePath)
	if builderRuntimeWorkspaceHasSemanticFile(workspacePath, strings.TrimSpace(mutationRegistry.controller.resolvedPath)) {
		return true
	}
	if builderRuntimeWorkspaceHasSemanticFile(workspacePath, strings.TrimSpace(mutationRegistry.view.resolvedPath)) {
		return true
	}
	return false
}

func builderRuntimeOpenLiteWorkspaceHasOverviewSurface(workspacePath string) bool {
	if strings.TrimSpace(workspacePath) == "" {
		return false
	}
	overviewRegistry := builderRuntimeOpenLiteOverviewSurfaceRegistry(workspacePath)
	hasOverviewSurface := builderRuntimeWorkspaceHasSemanticFile(workspacePath, strings.TrimSpace(overviewRegistry.view.resolvedPath)) || builderRuntimeWorkspaceHasSemanticFile(workspacePath, strings.TrimSpace(overviewRegistry.controller.resolvedPath))
	if !hasOverviewSurface {
		hasOverviewSurface = builderRuntimeOpenLiteFirstLikelyViewPath(workspacePath, builderRuntimeLikelyOverviewSurfaceViewPath) != ""
	}
	if !hasOverviewSurface {
		hasOverviewSurface = builderRuntimeOpenLiteWorkspaceHasLikelyControllerPath(workspacePath, builderRuntimeLikelyOverviewControllerPath)
	}
	if hasOverviewSurface {
		return true
	}
	mainContent, err := builderRuntimeSemanticFileContent(workspacePath, nil, "lib/main.dart")
	if err == nil && strings.TrimSpace(mainContent) != "" {
		if candidates, candidateErr := builderRuntimeMainViewConstructorCandidates(workspacePath, mainContent); candidateErr == nil && len(candidates) > 0 {
			return true
		}
	}
	return false
}

func builderRuntimeOpenLiteFirstLikelyViewPath(workspacePath string, predicate func(string) bool) string {
	if strings.TrimSpace(workspacePath) == "" || predicate == nil {
		return ""
	}
	viewDir := filepath.Join(workspacePath, "lib", "views")
	entries, err := os.ReadDir(viewDir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".dart") {
			continue
		}
		relPath := filepath.ToSlash(filepath.Join("lib", "views", entry.Name()))
		if predicate(relPath) && builderRuntimeWorkspaceHasSemanticFile(workspacePath, relPath) {
			return relPath
		}
	}
	return ""
}

func builderRuntimeOpenLiteWorkspaceHasLikelyControllerPath(workspacePath string, predicate func(string) bool) bool {
	return builderRuntimeOpenLiteFirstLikelyControllerPath(workspacePath, predicate) != ""
}

func builderRuntimeOpenLiteMutationControllerPath(workspacePath string) string {
	if strings.TrimSpace(workspacePath) == "" {
		return ""
	}
	controllerPath := "lib/controllers/record_form_controller.dart"
	mutationSurface := builderRuntimePrimaryMutationSurfaceCandidate(workspacePath)
	if candidatePath := strings.TrimSpace(mutationSurface.controller.path); candidatePath != "" {
		return candidatePath
	}
	if builderRuntimeWorkspaceHasSemanticFile(workspacePath, controllerPath) {
		return controllerPath
	}
	if hintedPath := builderRuntimePrimaryMutationControllerPathHint(workspacePath); hintedPath != "" && builderRuntimeWorkspaceHasSemanticFile(workspacePath, hintedPath) {
		return hintedPath
	}
	if likelyPath := builderRuntimeOpenLiteFirstLikelyControllerPath(workspacePath, builderRuntimeLikelyMutationControllerPath); likelyPath != "" {
		return likelyPath
	}
	return controllerPath
}

func builderRuntimeOpenLiteFirstLikelyControllerPath(workspacePath string, predicate func(string) bool) string {
	if strings.TrimSpace(workspacePath) == "" || predicate == nil {
		return ""
	}
	controllerDir := filepath.Join(workspacePath, "lib", "controllers")
	entries, err := os.ReadDir(controllerDir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".dart") {
			continue
		}
		relPath := filepath.ToSlash(filepath.Join("lib", "controllers", entry.Name()))
		if predicate(relPath) && builderRuntimeWorkspaceHasSemanticFile(workspacePath, relPath) {
			return relPath
		}
	}
	return ""
}

func builderRuntimeOpenLiteLikelyCollectionControllerPath(path string) bool {
	normalizedPath := filepath.ToSlash(strings.TrimSpace(path))
	if !strings.HasPrefix(normalizedPath, "lib/controllers/") || !strings.HasSuffix(normalizedPath, ".dart") {
		return false
	}
	baseName := strings.TrimSuffix(filepath.Base(normalizedPath), ".dart")
	if baseName == "" {
		return false
	}
	tokens := strings.Split(strings.ToLower(baseName), "_")
	hasCollectionToken := false
	hasControllerToken := false
	for _, token := range tokens {
		switch token {
		case "list", "collection":
			hasCollectionToken = true
		case "controller":
			hasControllerToken = true
		case "home", "overview", "form", "edit", "mutation":
			return false
		}
	}
	return hasCollectionToken && hasControllerToken
}

func builderRuntimeOpenLiteSupportsTodoMutationFlow(workspacePath string) bool {
	controllerContent := builderRuntimeOpenLiteMutationControllerContent(workspacePath)
	if strings.TrimSpace(controllerContent) == "" {
		return false
	}
	if !strings.Contains(controllerContent, "titleController") {
		return false
	}
	recordContent := builderRuntimeOpenLitePrimaryRecordModelContent(workspacePath)
	if strings.TrimSpace(recordContent) == "" {
		return true
	}
	if !builderRuntimeOpenLiteRecordModelHasStatus(workspacePath) {
		return true
	}
	statusType := builderRuntimeDeclaredFieldTypes(recordContent)["status"]
	if strings.TrimSpace(statusType) == "" {
		for _, candidate := range []string{"RecordStatus", "TodoItemStatus"} {
			if strings.Contains(controllerContent, candidate) {
				return true
			}
		}
		return false
	}
	return strings.Contains(controllerContent, statusType)
}

func builderRuntimeOpenLitePrimaryRecordModelPath(workspacePath string) string {
	if strings.TrimSpace(workspacePath) == "" {
		return ""
	}
	itemType := builderRuntimeCollectionViewItemType(builderRuntimePrimaryCollectionSurfaceCandidate(workspacePath).view.detailCallbackType)
	switch itemType {
	case "", "Object", "dynamic", "AppRecord", "String", "int", "double", "num", "bool", "DateTime", "Duration":
		// Fall through to the default record path when the collection surface does
		// not expose a concrete model type.
	default:
		if modelPath := builderRuntimeModelImportPathForType(workspacePath, itemType); modelPath != "" {
			return modelPath
		}
	}
	if builderRuntimeWorkspaceHasSemanticFile(workspacePath, "lib/models/record.dart") {
		return "lib/models/record.dart"
	}
	return ""
}

func builderRuntimeOpenLitePrimaryRecordModelContent(workspacePath string) string {
	modelPath := builderRuntimeOpenLitePrimaryRecordModelPath(workspacePath)
	if modelPath == "" {
		return ""
	}
	content, err := builderRuntimeSemanticFileContent(workspacePath, nil, modelPath)
	if err != nil {
		return ""
	}
	return content
}

func builderRuntimeOpenLiteMutationControllerContent(workspacePath string) string {
	controllerPath := builderRuntimeOpenLiteMutationControllerPath(workspacePath)
	if strings.TrimSpace(controllerPath) == "" {
		return ""
	}
	controllerContent, err := builderRuntimeSemanticFileContent(workspacePath, nil, controllerPath)
	if err != nil {
		return ""
	}
	return controllerContent
}

func builderRuntimeOpenLiteRecordRepositoryTypeName(workspacePath string) string {
	return builderRuntimePrimaryRepositoryCandidate(workspacePath).repositoryType
}

func builderRuntimeOpenLiteConcreteRecordRepositoryTypeName(workspacePath string) string {
	return builderRuntimePrimaryRepositoryCandidate(workspacePath).concreteType
}

func builderRuntimeOpenLitePreferredWidgetTestRepositoryTypeName(workspacePath string) string {
	repository := builderRuntimePrimaryRepositoryCandidate(workspacePath)
	if repository.inMemoryType != "" {
		return repository.inMemoryType
	}
	if repository.concreteType != "" {
		return repository.concreteType
	}
	return "HiveRecordRepository"
}

func builderRuntimeOpenLiteWidgetTestRepositoryImportPath(workspacePath string) string {
	repository := builderRuntimePrimaryRepositoryCandidate(workspacePath)
	if repository.path != "" {
		return strings.TrimPrefix(filepath.ToSlash(repository.path), "lib/")
	}
	return "repositories/record_repository.dart"
}

func builderRuntimeOpenLitePrimaryRecordTypeName(workspacePath string) string {
	recordContent := builderRuntimeOpenLitePrimaryRecordModelContent(workspacePath)
	if strings.TrimSpace(recordContent) == "" {
		return ""
	}
	matches := builderRuntimeExportedDartTypePattern.FindAllStringSubmatch(recordContent, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		name := strings.TrimSpace(match[1])
		if name == "" || strings.HasSuffix(name, "Status") {
			continue
		}
		return name
	}
	return ""
}

type builderRuntimeOpenLiteDeclaredField struct {
	name      string
	fieldType string
}

func builderRuntimeOpenLiteCollectionFieldSemantics(workspacePath string) builderRuntimeOpenLiteSurfaceRegistryFieldSemantics {
	recordContent := builderRuntimeOpenLitePrimaryRecordModelContent(workspacePath)
	if strings.TrimSpace(recordContent) == "" {
		return builderRuntimeOpenLiteSurfaceRegistryFieldSemantics{}
	}
	fields := builderRuntimeOpenLiteDeclaredFields(recordContent)
	primaryTextField := builderRuntimeOpenLitePrimaryTextField(fields)
	semantics := builderRuntimeOpenLiteSurfaceRegistryFieldSemantics{
		identifierField:      builderRuntimeOpenLiteIdentifierField(fields),
		primaryTextField:     primaryTextField,
		primaryTextFieldType: builderRuntimeOpenLiteFieldTypeByName(fields, primaryTextField),
		secondaryTextField:   builderRuntimeOpenLiteSecondaryTextField(fields),
		timeField:            builderRuntimeOpenLiteTimeField(fields),
		numericFields:        builderRuntimeOpenLiteNumericFields(fields),
		booleanFields:        builderRuntimeOpenLiteBooleanFields(fields),
		noteField:            builderRuntimeOpenLiteNoteField(fields),
	}
	semantics.statusField, semantics.statusEnumType = builderRuntimeOpenLiteStatusField(fields)
	semantics = builderRuntimeOpenLiteMergeDomainFieldSemantics(workspacePath, fields, semantics)
	if semantics.statusEnumType != "" {
		semantics.statusCopyLabelMethodName = builderRuntimeOpenLiteStatusCopyLabelMethodName(workspacePath, semantics.statusEnumType)
	}
	return semantics
}

func builderRuntimeOpenLiteMergeDomainFieldSemantics(workspacePath string, fields []builderRuntimeOpenLiteDeclaredField, semantics builderRuntimeOpenLiteSurfaceRegistryFieldSemantics) builderRuntimeOpenLiteSurfaceRegistryFieldSemantics {
	if strings.TrimSpace(workspacePath) == "" || len(fields) == 0 {
		return semantics
	}
	dm, err := loadPrepareDomainModel(workspacePath)
	if err != nil {
		return semantics
	}
	primaryEntity := builderRuntimeOpenLitePrimaryDomainEntity(dm)
	if primaryEntity == nil {
		return semantics
	}
	declaredByName := make(map[string]builderRuntimeOpenLiteDeclaredField, len(fields))
	for _, field := range fields {
		declaredByName[field.name] = field
	}
	domainFieldByRole := func(roles ...string) (string, builderRuntimeOpenLiteDeclaredField, bool) {
		field := emitGenericRoleField(primaryEntity, roles...)
		if field == nil {
			return "", builderRuntimeOpenLiteDeclaredField{}, false
		}
		dartName := emitSnakeToCamel(strings.TrimSpace(field.Name))
		declared, ok := declaredByName[dartName]
		if !ok {
			return "", builderRuntimeOpenLiteDeclaredField{}, false
		}
		return dartName, declared, true
	}
	if name, _, ok := domainFieldByRole("identifier"); ok {
		semantics.identifierField = name
	}
	if name, _, ok := domainFieldByRole("primary_text"); ok {
		semantics.primaryTextField = name
		semantics.primaryTextFieldType = builderRuntimeOpenLiteNormalizedFieldType(declaredByName[name].fieldType)
	}
	if name, _, ok := domainFieldByRole("secondary_text"); ok {
		semantics.secondaryTextField = name
	}
	if name, declared, ok := domainFieldByRole("status"); ok {
		semantics.statusField = name
		semantics.statusEnumType = builderRuntimeOpenLiteNormalizedFieldType(declared.fieldType)
	}
	if name, _, ok := domainFieldByRole("due_date", "date", "time"); ok {
		semantics.timeField = name
	}
	if name, _, ok := domainFieldByRole("note"); ok {
		semantics.noteField = name
	}
	semantics.numericFields = builderRuntimeOpenLiteMergeDomainNumericFieldSemantics(primaryEntity, declaredByName, semantics.numericFields)
	semantics.numericFields = builderRuntimeOpenLiteFilterNumericFields(semantics.numericFields, semantics.primaryTextField)
	semantics.booleanFields = builderRuntimeOpenLiteMergeDomainBooleanFieldSemantics(primaryEntity, declaredByName, semantics.booleanFields)
	semantics.booleanFields = builderRuntimeOpenLiteFilterBooleanFields(semantics.booleanFields, semantics.primaryTextField)
	return semantics
}

func builderRuntimeOpenLiteFilterNumericFields(fields []builderRuntimeOpenLiteNumericFieldSemantics, excludedNames ...string) []builderRuntimeOpenLiteNumericFieldSemantics {
	excluded := make(map[string]struct{}, len(excludedNames))
	for _, name := range excludedNames {
		trimmedName := strings.TrimSpace(name)
		if trimmedName != "" {
			excluded[trimmedName] = struct{}{}
		}
	}
	filtered := make([]builderRuntimeOpenLiteNumericFieldSemantics, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if _, ok := excluded[field.name]; ok {
			continue
		}
		if _, ok := seen[field.name]; ok {
			continue
		}
		seen[field.name] = struct{}{}
		filtered = append(filtered, field)
	}
	return filtered
}

func builderRuntimeOpenLiteMergeDomainNumericFieldSemantics(primaryEntity *appprepare.DataEntity, declaredByName map[string]builderRuntimeOpenLiteDeclaredField, current []builderRuntimeOpenLiteNumericFieldSemantics) []builderRuntimeOpenLiteNumericFieldSemantics {
	if primaryEntity == nil || len(declaredByName) == 0 {
		return current
	}
	merged := append([]builderRuntimeOpenLiteNumericFieldSemantics(nil), current...)
	seen := make(map[string]struct{}, len(merged))
	for _, field := range merged {
		seen[field.name] = struct{}{}
	}
	for index := range primaryEntity.Fields {
		field := primaryEntity.Fields[index]
		role := strings.ToLower(strings.TrimSpace(field.Role))
		if !builderRuntimeOpenLiteIsNumericRole(role) {
			continue
		}
		dartName := emitSnakeToCamel(strings.TrimSpace(field.Name))
		declared, ok := declaredByName[dartName]
		if !ok || !builderRuntimeOpenLiteIsNumericField(declared.fieldType) {
			continue
		}
		if _, ok := seen[dartName]; ok {
			continue
		}
		seen[dartName] = struct{}{}
		merged = append(merged, builderRuntimeOpenLiteNumericFieldSemantics{name: dartName, fieldType: builderRuntimeOpenLiteNormalizedFieldType(declared.fieldType), role: role})
	}
	return merged
}

func builderRuntimeOpenLiteFilterBooleanFields(fields []builderRuntimeOpenLiteBooleanFieldSemantics, excludedNames ...string) []builderRuntimeOpenLiteBooleanFieldSemantics {
	excluded := make(map[string]struct{}, len(excludedNames))
	for _, name := range excludedNames {
		trimmedName := strings.TrimSpace(name)
		if trimmedName != "" {
			excluded[trimmedName] = struct{}{}
		}
	}
	filtered := make([]builderRuntimeOpenLiteBooleanFieldSemantics, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if _, ok := excluded[field.name]; ok {
			continue
		}
		if _, ok := seen[field.name]; ok {
			continue
		}
		seen[field.name] = struct{}{}
		filtered = append(filtered, field)
	}
	return filtered
}

func builderRuntimeOpenLiteMergeDomainBooleanFieldSemantics(primaryEntity *appprepare.DataEntity, declaredByName map[string]builderRuntimeOpenLiteDeclaredField, current []builderRuntimeOpenLiteBooleanFieldSemantics) []builderRuntimeOpenLiteBooleanFieldSemantics {
	if primaryEntity == nil || len(declaredByName) == 0 {
		return current
	}
	merged := append([]builderRuntimeOpenLiteBooleanFieldSemantics(nil), current...)
	seen := make(map[string]struct{}, len(merged))
	for _, field := range merged {
		seen[field.name] = struct{}{}
	}
	for index := range primaryEntity.Fields {
		field := primaryEntity.Fields[index]
		role := strings.ToLower(strings.TrimSpace(field.Role))
		if role != "" && !builderRuntimeOpenLiteIsBooleanRole(role) {
			continue
		}
		dartName := emitSnakeToCamel(strings.TrimSpace(field.Name))
		declared, ok := declaredByName[dartName]
		if !ok || !builderRuntimeOpenLiteIsBooleanField(declared.fieldType) {
			continue
		}
		if _, ok := seen[dartName]; ok {
			continue
		}
		seen[dartName] = struct{}{}
		merged = append(merged, builderRuntimeOpenLiteBooleanFieldSemantics{name: dartName, role: role})
	}
	return merged
}

func builderRuntimeOpenLitePrimaryDomainEntity(dm appprepare.DomainModel) *appprepare.DataEntity {
	for index := range dm.Entities {
		if strings.TrimSpace(dm.Entities[index].Source) != "derived" {
			return &dm.Entities[index]
		}
	}
	if len(dm.Entities) == 0 {
		return nil
	}
	return &dm.Entities[0]
}

func builderRuntimeOpenLiteSummaryModelSemantics(workspacePath string) builderRuntimeOpenLiteSummaryModelSnapshot {
	if strings.TrimSpace(workspacePath) == "" {
		return builderRuntimeOpenLiteSummaryModelSnapshot{}
	}
	content, err := builderRuntimeSemanticFileContent(workspacePath, nil, "lib/models/dashboard_summary.dart")
	if err != nil || strings.TrimSpace(content) == "" {
		return builderRuntimeOpenLiteSummaryModelSnapshot{}
	}
	className := builderRuntimeOpenLiteFirstNamedDartClass(content)
	if className == "" {
		className = "DashboardSummary"
	}
	fields := builderRuntimeOpenLiteDeclaredFields(content)
	if len(fields) == 0 {
		return builderRuntimeOpenLiteSummaryModelSnapshot{className: className}
	}
	return builderRuntimeOpenLiteSummaryModelSnapshot{className: className, fields: fields}
}

func builderRuntimeOpenLiteIdentifierField(fields []builderRuntimeOpenLiteDeclaredField) string {
	for _, field := range fields {
		if builderRuntimeOpenLiteIsIdentifierField(field.name) {
			return field.name
		}
	}
	return ""
}

func builderRuntimeOpenLiteDeclaredFields(content string) []builderRuntimeOpenLiteDeclaredField {
	matches := builderRuntimeDartTypedFieldPattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}
	fields := make([]builderRuntimeOpenLiteDeclaredField, 0, len(matches))
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		fieldType := strings.TrimSpace(match[1])
		fieldName := strings.TrimSpace(match[2])
		if fieldType == "" || fieldName == "" {
			continue
		}
		fields = append(fields, builderRuntimeOpenLiteDeclaredField{name: fieldName, fieldType: fieldType})
	}
	return fields
}

func builderRuntimeOpenLitePrimaryTextField(fields []builderRuntimeOpenLiteDeclaredField) string {
	for _, preferred := range []string{"title", "name", "headline", "subject", "summary", "label"} {
		if field := builderRuntimeOpenLiteFieldByName(fields, preferred); field != nil && builderRuntimeOpenLiteIsStringField(field.fieldType) {
			return field.name
		}
	}
	for _, field := range fields {
		if !builderRuntimeOpenLiteIsStringField(field.fieldType) || builderRuntimeOpenLiteIsIdentifierField(field.name) || builderRuntimeOpenLiteIsNoteLikeField(field.name) || builderRuntimeOpenLiteIsStatusLikeField(field.name) || builderRuntimeOpenLiteIsTimeLikeField(field.name) {
			continue
		}
		fieldName := strings.ToLower(field.name)
		if strings.Contains(fieldName, "title") || strings.Contains(fieldName, "headline") || strings.Contains(fieldName, "subject") || strings.Contains(fieldName, "label") {
			return field.name
		}
	}
	for _, field := range fields {
		if !builderRuntimeOpenLiteIsStringField(field.fieldType) || builderRuntimeOpenLiteIsIdentifierField(field.name) || builderRuntimeOpenLiteIsNoteLikeField(field.name) || builderRuntimeOpenLiteIsStatusLikeField(field.name) || builderRuntimeOpenLiteIsTimeLikeField(field.name) {
			continue
		}
		if strings.Contains(strings.ToLower(field.name), "name") {
			return field.name
		}
	}
	for _, field := range fields {
		if !builderRuntimeOpenLiteIsStringField(field.fieldType) {
			continue
		}
		if builderRuntimeOpenLiteIsIdentifierField(field.name) || builderRuntimeOpenLiteIsNoteLikeField(field.name) || builderRuntimeOpenLiteIsStatusLikeField(field.name) || builderRuntimeOpenLiteIsTimeLikeField(field.name) {
			continue
		}
		return field.name
	}
	return ""
}

func builderRuntimeOpenLiteSecondaryTextField(fields []builderRuntimeOpenLiteDeclaredField) string {
	primaryTextField := builderRuntimeOpenLitePrimaryTextField(fields)
	for _, preferred := range []string{"category", "group", "project", "projectName", "type", "kind", "section", "bucket"} {
		if field := builderRuntimeOpenLiteFieldByName(fields, preferred); field != nil && builderRuntimeOpenLiteIsStringField(field.fieldType) && field.name != primaryTextField {
			return field.name
		}
	}
	for _, field := range fields {
		if field.name == primaryTextField || !builderRuntimeOpenLiteIsStringField(field.fieldType) {
			continue
		}
		if builderRuntimeOpenLiteIsIdentifierField(field.name) || builderRuntimeOpenLiteIsNoteLikeField(field.name) || builderRuntimeOpenLiteIsStatusLikeField(field.name) || builderRuntimeOpenLiteIsTimeLikeField(field.name) {
			continue
		}
		return field.name
	}
	return ""
}

func builderRuntimeOpenLiteStatusField(fields []builderRuntimeOpenLiteDeclaredField) (string, string) {
	for _, preferred := range []string{"status", "taskStatus", "state", "phase", "stage"} {
		if field := builderRuntimeOpenLiteFieldByName(fields, preferred); field != nil {
			return field.name, builderRuntimeOpenLiteNormalizedFieldType(field.fieldType)
		}
	}
	for _, field := range fields {
		fieldType := builderRuntimeOpenLiteNormalizedFieldType(field.fieldType)
		if fieldType == "" || builderRuntimeOpenLiteIsStringField(fieldType) {
			continue
		}
		if strings.HasSuffix(fieldType, "Status") || strings.HasSuffix(fieldType, "State") || strings.HasSuffix(fieldType, "Phase") || strings.HasSuffix(fieldType, "Stage") {
			return field.name, fieldType
		}
		if builderRuntimeOpenLiteIsStatusLikeField(field.name) {
			return field.name, fieldType
		}
	}
	return "", ""
}

func builderRuntimeOpenLiteTimeField(fields []builderRuntimeOpenLiteDeclaredField) string {
	for _, preferred := range []string{"updatedAt", "dueOn", "date", "occurredOn", "createdAt", "deadline"} {
		if field := builderRuntimeOpenLiteFieldByName(fields, preferred); field != nil {
			return field.name
		}
	}
	for _, field := range fields {
		if builderRuntimeOpenLiteIsTimeField(field.fieldType) {
			return field.name
		}
	}
	return ""
}

func builderRuntimeOpenLiteNoteField(fields []builderRuntimeOpenLiteDeclaredField) string {
	for _, preferred := range []string{"note", "description", "remark", "details", "comment"} {
		if field := builderRuntimeOpenLiteFieldByName(fields, preferred); field != nil && builderRuntimeOpenLiteIsStringField(field.fieldType) {
			return field.name
		}
	}
	return ""
}

func builderRuntimeOpenLiteNumericFields(fields []builderRuntimeOpenLiteDeclaredField) []builderRuntimeOpenLiteNumericFieldSemantics {
	result := make([]builderRuntimeOpenLiteNumericFieldSemantics, 0)
	for _, field := range fields {
		if !builderRuntimeOpenLiteIsNumericField(field.fieldType) || builderRuntimeOpenLiteIsIdentifierField(field.name) {
			continue
		}
		role := builderRuntimeOpenLiteNumericFieldRole(field.name)
		if role == "" {
			continue
		}
		result = append(result, builderRuntimeOpenLiteNumericFieldSemantics{name: field.name, fieldType: builderRuntimeOpenLiteNormalizedFieldType(field.fieldType), role: role})
	}
	return result
}

func builderRuntimeOpenLiteBooleanFields(fields []builderRuntimeOpenLiteDeclaredField) []builderRuntimeOpenLiteBooleanFieldSemantics {
	result := make([]builderRuntimeOpenLiteBooleanFieldSemantics, 0)
	for _, field := range fields {
		if !builderRuntimeOpenLiteIsBooleanField(field.fieldType) || builderRuntimeOpenLiteIsIdentifierField(field.name) {
			continue
		}
		result = append(result, builderRuntimeOpenLiteBooleanFieldSemantics{name: field.name, role: builderRuntimeOpenLiteBooleanFieldRole(field.name)})
	}
	return result
}

func builderRuntimeOpenLiteNumericFieldRole(name string) string {
	lowerName := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.Contains(lowerName, "rating") || strings.Contains(lowerName, "score"):
		return "rating"
	case strings.Contains(lowerName, "duration") || strings.Contains(lowerName, "minutes") || strings.Contains(lowerName, "hours"):
		return "duration"
	case strings.Contains(lowerName, "amount") || strings.Contains(lowerName, "price") || strings.Contains(lowerName, "cost") || strings.Contains(lowerName, "weight"):
		return "amount"
	default:
		return ""
	}
}

func builderRuntimeOpenLiteBooleanFieldRole(name string) string {
	lowerName := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.HasPrefix(lowerName, "is") || strings.HasPrefix(lowerName, "has") || strings.Contains(lowerName, "enabled") || strings.Contains(lowerName, "checked"):
		return "flag"
	default:
		return "boolean"
	}
}

func builderRuntimeOpenLiteIsNumericField(fieldType string) bool {
	switch builderRuntimeOpenLiteNormalizedFieldType(fieldType) {
	case "int", "double", "num":
		return true
	default:
		return false
	}
}

func builderRuntimeOpenLiteIsBooleanField(fieldType string) bool {
	return builderRuntimeOpenLiteNormalizedFieldType(fieldType) == "bool"
}

func builderRuntimeOpenLiteIsNumericRole(role string) bool {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "rating", "duration", "amount", "number", "quantity":
		return true
	default:
		return false
	}
}

func builderRuntimeOpenLiteIsBooleanRole(role string) bool {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "boolean", "bool", "flag", "toggle":
		return true
	default:
		return false
	}
}

func builderRuntimeOpenLiteNumericFieldLabelGetter(field builderRuntimeOpenLiteNumericFieldSemantics) string {
	switch strings.ToLower(strings.TrimSpace(field.role)) {
	case "rating":
		return "ratingFieldLabel"
	case "duration":
		return "durationFieldLabel"
	default:
		return "amountFieldLabel"
	}
}

func builderRuntimeOpenLiteBooleanFieldLabelGetter(field builderRuntimeOpenLiteBooleanFieldSemantics) string {
	trimmedName := strings.TrimSpace(field.name)
	if trimmedName == "" {
		return "booleanFieldLabel"
	}
	return trimmedName + "FieldLabel"
}

func builderRuntimeOpenLiteBooleanDetailLabelGetter(field builderRuntimeOpenLiteBooleanFieldSemantics) string {
	trimmedName := strings.TrimSpace(field.name)
	if trimmedName == "" {
		return "detailBooleanLabel"
	}
	return "detail" + strings.ToUpper(trimmedName[:1]) + trimmedName[1:] + "Label"
}

func builderRuntimeOpenLiteBooleanFieldKey(field builderRuntimeOpenLiteBooleanFieldSemantics) string {
	name := builderRuntimeOpenLiteIdentifierSnakeCase(field.name)
	if name == "" {
		return "boolean-field"
	}
	return strings.ReplaceAll(name, "_", "-") + "-field"
}

func builderRuntimeOpenLiteBooleanValueExpression(accessor string) string {
	trimmedAccessor := strings.TrimSpace(accessor)
	if trimmedAccessor == "" {
		return "''"
	}
	return "openLiteCopy.booleanValueLabel(" + trimmedAccessor + ")"
}

func builderRuntimeOpenLiteBooleanFieldValueExpression(accessor string, field builderRuntimeOpenLiteBooleanFieldSemantics) string {
	trimmedAccessor := strings.TrimSpace(accessor)
	if trimmedAccessor == "" {
		return "''"
	}
	return "openLiteCopy.booleanFieldValueLabel(openLiteCopy." + builderRuntimeOpenLiteBooleanFieldLabelGetter(field) + ", " + trimmedAccessor + ")"
}

func builderRuntimeOpenLiteNumericDetailLabelGetter(field builderRuntimeOpenLiteNumericFieldSemantics) string {
	switch strings.ToLower(strings.TrimSpace(field.role)) {
	case "rating":
		return "detailRatingLabel"
	case "duration":
		return "detailDurationLabel"
	default:
		return "detailAmountLabel"
	}
}

func builderRuntimeOpenLiteNumericFieldKey(field builderRuntimeOpenLiteNumericFieldSemantics) string {
	role := strings.ToLower(strings.TrimSpace(field.role))
	if role != "" {
		return role + "-field"
	}
	name := builderRuntimeOpenLiteIdentifierSnakeCase(field.name)
	if name == "" {
		return "number-field"
	}
	return strings.ReplaceAll(name, "_", "-") + "-field"
}

func builderRuntimeOpenLiteNumericKeyboardType(field builderRuntimeOpenLiteNumericFieldSemantics) string {
	switch builderRuntimeOpenLiteNormalizedFieldType(field.fieldType) {
	case "double", "num":
		return "const TextInputType.numberWithOptions(decimal: true)"
	default:
		return "TextInputType.number"
	}
}

func builderRuntimeOpenLiteNumericParser(field builderRuntimeOpenLiteNumericFieldSemantics) string {
	switch builderRuntimeOpenLiteNormalizedFieldType(field.fieldType) {
	case "int":
		return "int.tryParse"
	case "num":
		return "num.tryParse"
	default:
		return "double.tryParse"
	}
}

func builderRuntimeOpenLiteNumericDisplayExpression(accessor string, field builderRuntimeOpenLiteNumericFieldSemantics) string {
	trimmedAccessor := strings.TrimSpace(accessor)
	if trimmedAccessor == "" {
		return "''"
	}
	if builderRuntimeOpenLiteNormalizedFieldType(field.fieldType) == "double" {
		return trimmedAccessor + ".toStringAsFixed(1)"
	}
	return trimmedAccessor + ".toString()"
}

func builderRuntimeOpenLiteValueDisplayExpression(accessor, fieldType string) string {
	trimmedAccessor := strings.TrimSpace(accessor)
	if trimmedAccessor == "" {
		return "''"
	}
	switch builderRuntimeOpenLiteNormalizedFieldType(fieldType) {
	case "", "String":
		return trimmedAccessor
	case "double":
		return trimmedAccessor + ".toStringAsFixed(1)"
	default:
		return trimmedAccessor + ".toString()"
	}
}

func builderRuntimeOpenLiteSummaryMetricLabelGetter(fieldName string) string {
	trimmedName := strings.TrimSpace(fieldName)
	if trimmedName == "" {
		return ""
	}
	return "summary" + strings.ToUpper(trimmedName[:1]) + trimmedName[1:] + "Label"
}

func builderRuntimeOpenLiteSummaryMetricValueExpression(summaryAccessor string, field builderRuntimeOpenLiteDeclaredField) string {
	return builderRuntimeOpenLiteValueDisplayExpression(strings.TrimSpace(summaryAccessor)+"."+field.name, field.fieldType)
}

func builderRuntimeOpenLiteStatusCopyLabelMethodName(workspacePath, statusType string) string {
	trimmedWorkspacePath := strings.TrimSpace(workspacePath)
	trimmedStatusType := strings.TrimSpace(statusType)
	if trimmedWorkspacePath == "" || trimmedStatusType == "" {
		return ""
	}
	copyContent, err := builderRuntimeSemanticFileContent(trimmedWorkspacePath, nil, "lib/template/open_lite_copy.dart")
	if err != nil || strings.TrimSpace(copyContent) == "" {
		return "statusLabel"
	}
	pattern := regexp.MustCompile(`(?m)^\s*String\s+([A-Za-z_][A-Za-z0-9_]*)\(\s*` + regexp.QuoteMeta(trimmedStatusType) + `\s+[A-Za-z_][A-Za-z0-9_]*\s*\)\s*(?:=>|\{)`)
	if match := pattern.FindStringSubmatch(copyContent); len(match) >= 2 {
		return strings.TrimSpace(match[1])
	}
	return "statusLabel"
}

func builderRuntimeOpenLiteCompletedStatusMember(workspacePath, statusType string) string {
	for _, member := range builderRuntimeOpenLiteEnumMembers(workspacePath, statusType) {
		for _, preferred := range []string{"done", "completed", "watched", "watered", "finished"} {
			if member == preferred {
				return preferred
			}
		}
	}
	return ""
}

func builderRuntimeOpenLiteFirstEnumMember(workspacePath, statusType string) string {
	members := builderRuntimeOpenLiteEnumMembers(workspacePath, statusType)
	if len(members) == 0 {
		return ""
	}
	return members[0]
}

func builderRuntimeOpenLiteEnumMembers(workspacePath, statusType string) []string {
	trimmedStatusType := strings.TrimSpace(statusType)
	if trimmedStatusType == "" {
		return nil
	}
	recordContent := builderRuntimeOpenLitePrimaryRecordModelContent(workspacePath)
	if strings.TrimSpace(recordContent) == "" {
		return nil
	}
	pattern := regexp.MustCompile(`(?s)enum\s+` + regexp.QuoteMeta(trimmedStatusType) + `\s*\{([^}]*)\}`)
	match := pattern.FindStringSubmatch(recordContent)
	if len(match) < 2 {
		return nil
	}
	rawMembers := strings.Split(match[1], ",")
	members := make([]string, 0, len(rawMembers))
	for _, member := range rawMembers {
		trimmedMember := strings.TrimSpace(strings.TrimSuffix(member, ";"))
		if trimmedMember == "" {
			continue
		}
		members = append(members, trimmedMember)
	}
	return members
}

func builderRuntimeOpenLiteFormFieldLabel(fieldName string) string {
	trimmedName := strings.TrimSpace(fieldName)
	if trimmedName == "" {
		return "字段"
	}
	return trimmedName
}

func builderRuntimeOpenLiteFieldByName(fields []builderRuntimeOpenLiteDeclaredField, name string) *builderRuntimeOpenLiteDeclaredField {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return nil
	}
	for index := range fields {
		if fields[index].name == trimmedName {
			return &fields[index]
		}
	}
	return nil
}

func builderRuntimeOpenLiteFieldTypeByName(fields []builderRuntimeOpenLiteDeclaredField, name string) string {
	field := builderRuntimeOpenLiteFieldByName(fields, name)
	if field == nil {
		return ""
	}
	return builderRuntimeOpenLiteNormalizedFieldType(field.fieldType)
}

func builderRuntimeOpenLiteNormalizedFieldType(fieldType string) string {
	return strings.TrimSuffix(strings.TrimSpace(fieldType), "?")
}

func builderRuntimeOpenLiteIsStringField(fieldType string) bool {
	return builderRuntimeOpenLiteNormalizedFieldType(fieldType) == "String"
}

func builderRuntimeOpenLiteIsTimeField(fieldType string) bool {
	return strings.Contains(builderRuntimeOpenLiteNormalizedFieldType(fieldType), "DateTime")
}

func builderRuntimeOpenLiteIsIdentifierField(name string) bool {
	trimmedName := strings.TrimSpace(name)
	return trimmedName == "id" || strings.HasSuffix(trimmedName, "Id") || strings.HasSuffix(trimmedName, "ID")
}

func builderRuntimeOpenLiteIsNoteLikeField(name string) bool {
	trimmedName := strings.TrimSpace(name)
	return trimmedName == "note" || trimmedName == "description" || trimmedName == "remark" || trimmedName == "details" || trimmedName == "comment"
}

func builderRuntimeOpenLiteIsStatusLikeField(name string) bool {
	trimmedName := strings.TrimSpace(name)
	return trimmedName == "status" || trimmedName == "taskStatus" || trimmedName == "state" || trimmedName == "phase" || trimmedName == "stage"
}

func builderRuntimeOpenLiteIsTimeLikeField(name string) bool {
	trimmedName := strings.TrimSpace(name)
	return trimmedName == "updatedAt" || trimmedName == "dueOn" || trimmedName == "date" || trimmedName == "occurredOn" || trimmedName == "createdAt" || trimmedName == "deadline"
}

func builderRuntimeOpenLiteRecordModelHasStatus(workspacePath string) bool {
	recordContent := builderRuntimeOpenLitePrimaryRecordModelContent(workspacePath)
	if strings.TrimSpace(recordContent) == "" {
		return false
	}
	return strings.Contains(recordContent, " status;") || strings.Contains(recordContent, " status ")
}

func builderRuntimeInsertFieldIntoStatelessWidget(content, className, fieldDeclaration string) string {
	trimmedClassName := strings.TrimSpace(className)
	trimmedFieldDeclaration := strings.TrimSpace(fieldDeclaration)
	if strings.TrimSpace(content) == "" || trimmedClassName == "" || trimmedFieldDeclaration == "" {
		return content
	}
	classIndex := strings.Index(content, "class "+trimmedClassName+" extends StatelessWidget {")
	if classIndex < 0 {
		return content
	}
	markerIndex := strings.Index(content[classIndex:], "\n\n  @override")
	if markerIndex < 0 {
		return content
	}
	markerIndex += classIndex
	return content[:markerIndex] + "\n\n  " + trimmedFieldDeclaration + content[markerIndex:]
}

func builderRuntimeOpenLiteSelectedFilterTypeName(workspacePath string) string {
	controllerPath := strings.TrimSpace(builderRuntimeOpenLiteCollectionSurfaceRegistry(workspacePath).controller.resolvedPath)
	if !builderRuntimeWorkspaceHasSemanticFile(workspacePath, controllerPath) {
		if likelyPath := builderRuntimeOpenLiteFirstLikelyControllerPath(workspacePath, builderRuntimeOpenLiteLikelyCollectionControllerPath); likelyPath != "" {
			controllerPath = likelyPath
		} else {
			controllerPath = "lib/controllers/record_list_controller.dart"
		}
	}
	controllerContent, err := builderRuntimeSemanticFileContent(workspacePath, nil, controllerPath)
	if err != nil || strings.TrimSpace(controllerContent) == "" {
		return ""
	}
	if match := builderRuntimeOpenLiteSelectedFilterFieldPattern.FindStringSubmatch(controllerContent); len(match) >= 2 {
		return strings.TrimSpace(match[1])
	}
	if match := builderRuntimeOpenLiteSelectedFilterGetterPattern.FindStringSubmatch(controllerContent); len(match) >= 2 {
		return strings.TrimSpace(match[1])
	}
	return ""
}

func builderRuntimeOpenLiteShouldDropTotalCount(workspacePath string) bool {
	if strings.TrimSpace(workspacePath) == "" {
		return false
	}
	if forbiddenTokens, err := builderRuntimeForbiddenSchemaTokens(workspacePath); err == nil && len(forbiddenTokens) > 0 {
		return slices.Contains(forbiddenTokens, "totalCount")
	}
	summaryModelContent, err := builderRuntimeSemanticFileContent(workspacePath, nil, "lib/models/dashboard_summary.dart")
	if err != nil || strings.TrimSpace(summaryModelContent) == "" {
		return false
	}
	return !strings.Contains(summaryModelContent, "totalCount")
}

func builderRuntimeOpenLiteShouldDropStatusWorkflow(workspacePath string) bool {
	if strings.TrimSpace(workspacePath) == "" {
		return false
	}
	if forbiddenTokens, err := builderRuntimeForbiddenSchemaTokens(workspacePath); err == nil && len(forbiddenTokens) > 0 {
		return slices.Contains(forbiddenTokens, "RecordStatus") || slices.Contains(forbiddenTokens, ".status")
	}
	return !builderRuntimeOpenLiteRecordModelHasStatus(workspacePath)
}

func builderRuntimeOpenLiteShouldDropTitleField(workspacePath string) bool {
	if strings.TrimSpace(workspacePath) == "" {
		return false
	}
	if forbiddenTokens, err := builderRuntimeForbiddenSchemaTokens(workspacePath); err == nil && len(forbiddenTokens) > 0 {
		return slices.Contains(forbiddenTokens, "titleFieldLabel") || slices.Contains(forbiddenTokens, "titleFieldRequiredError")
	}
	recordModelContent := builderRuntimeOpenLitePrimaryRecordModelContent(workspacePath)
	if strings.TrimSpace(recordModelContent) == "" {
		return false
	}
	return !strings.Contains(recordModelContent, "title")
}

func builderRuntimeOpenLiteShouldDropCategoryField(workspacePath string) bool {
	if strings.TrimSpace(workspacePath) == "" {
		return false
	}
	if forbiddenTokens, err := builderRuntimeForbiddenSchemaTokens(workspacePath); err == nil && len(forbiddenTokens) > 0 {
		return slices.Contains(forbiddenTokens, "categoryFieldLabel") || slices.Contains(forbiddenTokens, "detailCategoryLabel")
	}
	recordModelContent := builderRuntimeOpenLitePrimaryRecordModelContent(workspacePath)
	if strings.TrimSpace(recordModelContent) == "" {
		return false
	}
	return !strings.Contains(recordModelContent, "category")
}

func builderRuntimeRemoveDartMethodBlock(content string, signaturePattern *regexp.Regexp) string {
	if signaturePattern == nil || strings.TrimSpace(content) == "" {
		return content
	}
	loc := signaturePattern.FindStringIndex(content)
	if len(loc) != 2 {
		return content
	}
	openBrace := strings.LastIndex(content[loc[0]:loc[1]], "{")
	if openBrace < 0 {
		return content
	}
	openBrace += loc[0]
	closeBrace := builderRuntimeMatchingBraceIndex(content, openBrace)
	if closeBrace < 0 {
		return content
	}
	start := loc[0]
	if start > 0 && content[start-1] == '\n' {
		start--
	}
	end := closeBrace + 1
	if end < len(content) && content[end] == '\n' {
		end++
	}
	return content[:start] + content[end:]
}

func builderRuntimeMatchingBraceIndex(content string, openBrace int) int {
	if openBrace < 0 || openBrace >= len(content) || content[openBrace] != '{' {
		return -1
	}
	depth := 0
	inSingleQuoted := false
	inDoubleQuoted := false
	escaped := false
	for index := openBrace; index < len(content); index++ {
		ch := content[index]
		if inSingleQuoted || inDoubleQuoted {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if inSingleQuoted && ch == '\'' {
				inSingleQuoted = false
				continue
			}
			if inDoubleQuoted && ch == '"' {
				inDoubleQuoted = false
			}
			continue
		}
		switch ch {
		case '\'':
			inSingleQuoted = true
		case '"':
			inDoubleQuoted = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

func builderRuntimeMatchingParenIndex(content string, openParen int) int {
	if openParen < 0 || openParen >= len(content) || content[openParen] != '(' {
		return -1
	}
	depth := 0
	for index := openParen; index < len(content); index++ {
		switch content[index] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

func builderRuntimeFirstTopLevelArgument(args string) string {
	depth := 0
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				return args[:index]
			}
		}
	}
	return args
}

func normalizeBuilderRuntimeOpenLiteCopyLine(line, replacement string) string {
	updated := line
	if replacement == "visibleCount" {
		updated = builderRuntimeOpenLiteTotalCountArgPattern.ReplaceAllString(updated, "")
		updated = builderRuntimeOpenLiteTotalCountPlaceholderPattern.ReplaceAllString(updated, "")
	}
	updated = strings.ReplaceAll(updated, "${totalCount}", "${"+replacement+"}")
	updated = strings.ReplaceAll(updated, "$totalCount", "$"+replacement)
	return strings.ReplaceAll(updated, "totalCount", replacement)
}

func builderRuntimeOpenLiteCopyMethodNormalizationState(line, replacement string) (string, int) {
	if !strings.Contains(line, "{") {
		return "", 0
	}
	depth := strings.Count(line, "{") - strings.Count(line, "}")
	if depth <= 0 {
		return "", 0
	}
	return replacement, depth
}
