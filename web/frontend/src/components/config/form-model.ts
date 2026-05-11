export type JsonRecord = Record<string, unknown>

export interface CoreConfigForm {
  workspace: string
  builderRuntimeEnabled: boolean
  defaultModelPrimary: string
  defaultModelFallbacksText: string
  upgradeModelPrimary: string
  upgradeModelFallbacksText: string
  maxAttemptsBeforeUpgrade: string
  maxFilesBeforeUpgrade: string
  maxSchemaDriftBeforeUpgrade: string
  maxUnrelatedOperationRate: string
  upgradeOnValidationFail: boolean
  upgradeOnPatchParseFail: boolean
  upgradeOnScopeViolation: boolean
  upgradeOnSemanticConflict: boolean
}

export interface LauncherForm {
  port: string
  publicAccess: boolean
  allowedCIDRsText: string
}

export const EMPTY_FORM: CoreConfigForm = {
  workspace: "",
  builderRuntimeEnabled: false,
  defaultModelPrimary: "",
  defaultModelFallbacksText: "",
  upgradeModelPrimary: "",
  upgradeModelFallbacksText: "",
  maxAttemptsBeforeUpgrade: "2",
  maxFilesBeforeUpgrade: "2",
  maxSchemaDriftBeforeUpgrade: "0",
  maxUnrelatedOperationRate: "0",
  upgradeOnValidationFail: true,
  upgradeOnPatchParseFail: true,
  upgradeOnScopeViolation: true,
  upgradeOnSemanticConflict: false,
}

export const EMPTY_LAUNCHER_FORM: LauncherForm = {
  port: "18800",
  publicAccess: false,
  allowedCIDRsText: "",
}

function asRecord(value: unknown): JsonRecord {
  if (value && typeof value === "object" && !Array.isArray(value)) {
    return value as JsonRecord
  }
  return {}
}

function asString(value: unknown): string {
  return typeof value === "string" ? value : ""
}

function asBool(value: unknown): boolean {
  return value === true
}

function asNumberString(value: unknown, fallback: string): string {
  if (typeof value === "number" && Number.isFinite(value)) {
    return String(value)
  }
  if (typeof value === "string" && value.trim() !== "") {
    return value
  }
  return fallback
}

function modelPrimary(value: unknown): string {
  if (typeof value === "string") {
    return value
  }
  return asString(asRecord(value).primary)
}

function modelFallbacksText(value: unknown): string {
  if (typeof value === "string") {
    return ""
  }
  const fallbacks = asRecord(value).fallbacks
  return Array.isArray(fallbacks)
    ? fallbacks.filter((item): item is string => typeof item === "string").join("\n")
    : ""
}

export function buildFormFromConfig(config: unknown): CoreConfigForm {
  const root = asRecord(config)
  const appfactory = asRecord(root.appfactory)
  const builderRuntime = asRecord(appfactory.builder_runtime)
  const threshold = asRecord(builderRuntime.upgrade_threshold)

  return {
    workspace: asString(root.workspace) || EMPTY_FORM.workspace,
    builderRuntimeEnabled:
      builderRuntime.enabled === undefined
        ? EMPTY_FORM.builderRuntimeEnabled
        : asBool(builderRuntime.enabled),
    defaultModelPrimary: modelPrimary(builderRuntime.default_model),
    defaultModelFallbacksText: modelFallbacksText(builderRuntime.default_model),
    upgradeModelPrimary: modelPrimary(builderRuntime.upgrade_model),
    upgradeModelFallbacksText: modelFallbacksText(builderRuntime.upgrade_model),
    maxAttemptsBeforeUpgrade: asNumberString(
      threshold.max_attempts_before_upgrade,
      EMPTY_FORM.maxAttemptsBeforeUpgrade,
    ),
    maxFilesBeforeUpgrade: asNumberString(
      threshold.max_files_before_upgrade,
      EMPTY_FORM.maxFilesBeforeUpgrade,
    ),
    maxSchemaDriftBeforeUpgrade: asNumberString(
      threshold.max_schema_drift_before_upgrade,
      EMPTY_FORM.maxSchemaDriftBeforeUpgrade,
    ),
    maxUnrelatedOperationRate: asNumberString(
      threshold.max_unrelated_operation_rate,
      EMPTY_FORM.maxUnrelatedOperationRate,
    ),
    upgradeOnValidationFail:
      threshold.upgrade_on_validation_fail === undefined
        ? EMPTY_FORM.upgradeOnValidationFail
        : asBool(threshold.upgrade_on_validation_fail),
    upgradeOnPatchParseFail:
      threshold.upgrade_on_patch_parse_fail === undefined
        ? EMPTY_FORM.upgradeOnPatchParseFail
        : asBool(threshold.upgrade_on_patch_parse_fail),
    upgradeOnScopeViolation:
      threshold.upgrade_on_scope_violation === undefined
        ? EMPTY_FORM.upgradeOnScopeViolation
        : asBool(threshold.upgrade_on_scope_violation),
    upgradeOnSemanticConflict:
      threshold.upgrade_on_semantic_conflict === undefined
        ? EMPTY_FORM.upgradeOnSemanticConflict
        : asBool(threshold.upgrade_on_semantic_conflict),
  }
}

export function parseIntField(
  rawValue: string,
  label: string,
  options: { min?: number; max?: number } = {},
): number {
  const value = Number(rawValue)
  if (!Number.isInteger(value)) {
    throw new Error(`${label} must be an integer.`)
  }
  if (options.min !== undefined && value < options.min) {
    throw new Error(`${label} must be >= ${options.min}.`)
  }
  if (options.max !== undefined && value > options.max) {
    throw new Error(`${label} must be <= ${options.max}.`)
  }
  return value
}

export function parseFloatField(
  rawValue: string,
  label: string,
  options: { min?: number; max?: number } = {},
): number {
  const value = Number(rawValue)
  if (!Number.isFinite(value)) {
    throw new Error(`${label} must be a number.`)
  }
  if (options.min !== undefined && value < options.min) {
    throw new Error(`${label} must be >= ${options.min}.`)
  }
  if (options.max !== undefined && value > options.max) {
    throw new Error(`${label} must be <= ${options.max}.`)
  }
  return value
}

export function parseCIDRText(raw: string): string[] {
  if (!raw.trim()) {
    return []
  }
  return raw
    .split(/[\n,]/)
    .map((v) => v.trim())
    .filter((v) => v.length > 0)
}

export function parseMultilineList(raw: string): string[] {
  if (!raw.trim()) {
    return []
  }
  return raw
    .split("\n")
    .map((value) => value.trim())
    .filter((value) => value.length > 0)
}