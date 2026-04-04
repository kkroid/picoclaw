const APPFACTORY_AUTO_START_BUILDER_PREFIX = "jobs-ui-builder"
export const APPFACTORY_AUTO_START_BUILDER_IMAGE = "picoclaw/appfactory-builder:local"
const APPFACTORY_AUTO_START_CAPABILITY_TAGS = ["flutter"]

export type BuilderRuntimeMode = {
  enabled: boolean
  defaultModel: string
  upgradeModel: string
}

export function getBuilderRuntimeMode(config: Record<string, unknown> | undefined): BuilderRuntimeMode {
  const appfactory = asRecord(config?.appfactory)
  const builderRuntime = asRecord(appfactory?.builder_runtime)
  const defaultModel = readPrimaryModel(builderRuntime?.default_model)
  const upgradeModel = readPrimaryModel(builderRuntime?.upgrade_model)
  return {
    enabled: builderRuntime?.enabled === true && defaultModel !== "",
    defaultModel,
    upgradeModel,
  }
}

export function buildAutoStartBuilderRequest(jobId: string) {
  const builderId = `${APPFACTORY_AUTO_START_BUILDER_PREFIX}-${jobId}`
  return {
    builder_id: builderId,
    display_name: `Jobs UI Builder ${jobId}`,
    capability_tags: APPFACTORY_AUTO_START_CAPABILITY_TAGS,
    max_parallel_runs: 1,
    worker_profile: {
      image: APPFACTORY_AUTO_START_BUILDER_IMAGE,
    },
  }
}

export function toOptionalString(value: string | undefined) {
  if (!value) {
    return undefined
  }
  const trimmed = value.trim()
  return trimmed === "" ? undefined : trimmed
}

export function formatDate(value?: string) {
  if (!value) {
    return "-"
  }
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return value
  }
  return new Intl.DateTimeFormat(undefined, {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  }).format(date)
}

export function getErrorMessage(error: unknown, fallback: string) {
  return error instanceof Error ? error.message : fallback
}

export function isAlreadyRunningStartConflict(error: unknown) {
  if (!(error instanceof Error)) {
    return false
  }
  const message = error.message.toLowerCase()
  return message.includes("already has active execution") && message.includes("status running")
}

function asRecord(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined
}

function readPrimaryModel(value: unknown): string {
  if (typeof value === "string") {
    return value.trim()
  }
  const record = asRecord(value)
  return typeof record?.primary === "string" ? record.primary.trim() : ""
}