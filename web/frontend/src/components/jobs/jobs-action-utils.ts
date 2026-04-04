import type { PublicJobDeliveryContext, PublicJobRecord, PublicJobStatusContext, PublicNotification } from "@/api/system"

type Translate = (key: string, options?: Record<string, unknown>) => string

export type ExecutableStatusAction =
  | "submit_prd_approval"
  | "submit_template_approval"
  | "compile_prepare_bundle"
  | "start"

export type ExecutableNotificationAction =
  | "inspect_execution_history"
  | "inspect_device_evidence"
  | "inspect_orchestrator_status"
  | "inspect_orchestrator_lock_owner"
  | "restart_orchestrator_watch"
  | "inspect_delivery_record"
  | "verify_staged_release"
  | "prepare_signing"
  | "stage_release"
  | "monitor_release_feedback"
  | "update_subject_and_resubmit"

const EXECUTABLE_STATUS_ACTIONS = new Set<ExecutableStatusAction>([
  "submit_prd_approval",
  "submit_template_approval",
  "compile_prepare_bundle",
  "start",
])

const EXECUTABLE_NOTIFICATION_ACTIONS = new Set<ExecutableNotificationAction>([
  "inspect_execution_history",
  "inspect_device_evidence",
  "inspect_orchestrator_status",
  "inspect_orchestrator_lock_owner",
  "restart_orchestrator_watch",
  "inspect_delivery_record",
  "verify_staged_release",
  "prepare_signing",
  "stage_release",
  "monitor_release_feedback",
  "update_subject_and_resubmit",
])

export function getPendingHumanApprovalCount(job: PublicJobRecord) {
  if (job.status === "awaiting_prd_approval" || job.status === "awaiting_template_approval") {
    return 1
  }
  if (
    job.status_context?.suggested_action === "submit_prd_approval" ||
    job.status_context?.suggested_action === "submit_template_approval"
  ) {
    return 1
  }
  return 0
}

export function renderFailureDomainLabel(domain: string | undefined, t: Translate) {
  if (!domain) {
    return "-"
  }
  const key = `jobs.failureDomain.${domain}`
  const translated = t(key)
  return translated === key ? domain : translated
}

export function renderFailureCategoryLabel(category: string | undefined, t: Translate) {
  if (!category) {
    return "-"
  }
  const key = `jobs.failureCategory.${category}`
  const translated = t(key)
  return translated === key ? category : translated
}

export function renderFailureGuidanceActionLabel(action: string, t: Translate) {
  const key = `jobs.failureHelp.action.${action}`
  const translated = t(key)
  return translated === key ? action : translated
}

export function renderDeliveryStatusLabel(status: string | undefined, t: Translate) {
  if (!status) {
    return "-"
  }
  const key = `jobs.delivery.status.${status}`
  const translated = t(key)
  return translated === key ? status : translated
}

export function renderDeliveryChannelLabel(channel: string | undefined, t: Translate, noneValue = "__none__") {
  const normalized = channel ?? noneValue
  const key = normalized === noneValue ? "jobs.delivery.channel.none" : `jobs.delivery.channel.${normalized}`
  const translated = t(key)
  return translated === key ? normalized : translated
}

export function renderDeviceVerificationStatusLabel(status: string | undefined, t: Translate, noneValue = "__none__") {
  const normalized = status ?? noneValue
  const key = normalized === noneValue
    ? "jobs.delivery.deviceVerificationStatus.none"
    : `jobs.delivery.deviceVerificationStatus.${normalized}`
  const translated = t(key)
  return translated === key ? normalized : translated
}

export function renderReleaseFollowUpStatusLabel(status: string | undefined, t: Translate) {
  if (!status) {
    return "-"
  }
  const key = `jobs.delivery.followUpStatus.${status}`
  const translated = t(key)
  return translated === key ? status : translated
}

export function renderStatusContextReason(context: PublicJobStatusContext, t: Translate) {
  const code = context.reason_code
  if (!code) {
    return context.summary ?? "-"
  }
  const key = `jobs.statusContext.reason.${code}`
  const translated = t(key)
  if (translated !== key) {
    return translated
  }
  return context.summary ?? code
}

export function renderStatusContextAction(context: PublicJobStatusContext, t: Translate) {
  return renderSuggestedActionLabel(context.suggested_action, t)
}

export function renderSuggestedActionLabel(action: string | undefined, t: Translate) {
  if (!action) {
    return "-"
  }
  return translateStatusAction(action, t)
}

export function translateStatusAction(action: string, t: Translate) {
  const key = `jobs.statusContext.action.${action}`
  const translated = t(key)
  return translated === key ? action : translated
}

export function isExecutableStatusAction(action?: string): action is ExecutableStatusAction {
  return typeof action === "string" && EXECUTABLE_STATUS_ACTIONS.has(action as ExecutableStatusAction)
}

export function isExecutableNotificationAction(action?: string): action is ExecutableNotificationAction {
  return typeof action === "string" && EXECUTABLE_NOTIFICATION_ACTIONS.has(action as ExecutableNotificationAction)
}

export function canExecuteNotificationAction(item: PublicNotification) {
  if (!isExecutableNotificationAction(item.suggested_action)) {
    return false
  }
  if (item.suggested_action === "inspect_execution_history") {
    return Boolean(item.job_id)
  }
  return true
}

export function canRunDeliverySuggestedAction(context: PublicJobDeliveryContext | undefined) {
  return isExecutableNotificationAction(context?.suggested_action)
}

export function renderRolloutPercent(value?: number) {
  if (typeof value !== "number") {
    return "-"
  }
  return `${value}%`
}