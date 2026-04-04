import type { PublicJobEvent, PublicJobRecord } from "@/api/system"
import {
  getPendingHumanApprovalCount,
  renderFailureCategoryLabel,
  renderFailureDomainLabel,
  renderStatusContextReason,
} from "@/components/jobs/jobs-action-utils"
import {
  describeExecutionEvent,
  parseAcceptanceCheckSummary,
  renderAcceptanceCheckLabel,
  renderExecutionStageLabel,
  renderRunningStageDetail,
} from "@/components/jobs/jobs-state-model"
import { formatDate } from "@/components/jobs/jobs-page-utils"

type Translate = (key: string, options?: Record<string, unknown>) => string

const RUNNING_JOB_STATUSES = new Set(["running", "running_builder"])

export type JobProgressSummary = {
  headline: string
  detail?: string
  tone: "running" | "blocked" | "failed" | "completed" | "idle"
}

export type WorkspaceFocusSummary = {
  headline: string
  detail: string
  tone: JobProgressSummary["tone"]
}

export function renderStatusLabel(status: string, t: Translate) {
  if (isRunningJobStatus(status)) {
    return t("jobs.status.running")
  }
  const key = `jobs.status.${status}`
  const translated = t(key)
  return translated === key ? status : translated
}

export function isRunningJobStatus(status: string) {
  return RUNNING_JOB_STATUSES.has(status)
}

export function matchesJobSearchQuery(job: PublicJobRecord, query: string) {
  const normalized = query.trim().toLowerCase()
  if (normalized === "") {
    return true
  }
  return [
    job.title,
    job.job_id,
    job.prd_id,
    job.template_id,
    job.status,
    job.phase,
    job.status_context?.summary,
    job.status_context?.suggested_action,
    job.failure_context?.last_error_summary,
    job.failure_context?.failure_domain,
    job.failure_context?.failure_category,
  ]
    .filter((value): value is string => typeof value === "string" && value.trim() !== "")
    .some((value) => value.toLowerCase().includes(normalized))
}

export function compareJobsForList(left: PublicJobRecord, right: PublicJobRecord) {
  const priorityDelta = getJobListPriority(right) - getJobListPriority(left)
  if (priorityDelta !== 0) {
    return priorityDelta
  }

  const updatedDelta = Date.parse(right.updated_at) - Date.parse(left.updated_at)
  if (!Number.isNaN(updatedDelta) && updatedDelta !== 0) {
    return updatedDelta
  }

  return left.job_id.localeCompare(right.job_id)
}

export function getJobAttentionCount(job: PublicJobRecord) {
  let count = getPendingHumanApprovalCount(job)

  if (job.status === "failed") {
    count += 1
  }

  if (job.status_context?.suggested_action) {
    count += 1
  }

  return count
}

export function isAttentionJob(job: PublicJobRecord) {
  return getJobAttentionCount(job) > 0
}

export function getJobListSupportingSummary(
  job: PublicJobRecord,
  progress: JobProgressSummary,
  t: Translate,
) {
  if (job.status === "failed") {
    return job.failure_context?.last_error_summary || progress.detail || t("jobs.list.failedSummary")
  }

  if (getPendingHumanApprovalCount(job) > 0) {
    return (
      job.status_context?.summary ||
      (job.status_context ? renderStatusContextReason(job.status_context, t) : "") ||
      t("jobs.list.awaitingApprovals", {
        count: getPendingHumanApprovalCount(job),
      })
    )
  }

  return progress.detail || job.status_context?.summary || t("jobs.list.idleSummary")
}

export function matchesJobStatusFilter(job: PublicJobRecord, filter: string) {
  if (filter === "all") {
    return true
  }
  if (filter === "running") {
    return isRunningJobStatus(job.status)
  }
  if (filter === "attention") {
    return isAttentionJob(job)
  }
  return job.status === filter
}

export function countJobsForFilter(jobs: PublicJobRecord[], filter: string) {
  return jobs.filter((job) => matchesJobStatusFilter(job, filter)).length
}

export function buildWorkspaceFocusSummary({
  selectedJob,
  progress,
  selectedJobActionCount,
  pendingNotificationCount,
  attentionCount,
  t,
}: {
  selectedJob?: PublicJobRecord
  progress?: JobProgressSummary
  selectedJobActionCount: number
  pendingNotificationCount: number
  attentionCount: number
  t: Translate
}): WorkspaceFocusSummary {
  if (selectedJob && progress) {
    return {
      headline: progress.headline,
      detail:
        selectedJob.status_context?.summary ||
        (selectedJobActionCount > 0
          ? t("jobs.workspace.focusNeedsAttentionDetail", {
              notifications: selectedJobActionCount,
              attention: 1,
            })
          : undefined) ||
        progress.detail ||
        t("jobs.workspace.focusSelectedJobDetail", { jobId: selectedJob.job_id }),
      tone: progress.tone,
    }
  }

  if (pendingNotificationCount > 0 || attentionCount > 0) {
    return {
      headline: t("jobs.workspace.focusNeedsAttention"),
      detail: t("jobs.workspace.focusNeedsAttentionDetail", {
        notifications: pendingNotificationCount,
        attention: attentionCount,
      }),
      tone: "blocked",
    }
  }

  return {
    headline: t("jobs.workspace.focusAllStable"),
    detail: t("jobs.workspace.focusAllStableDetail"),
    tone: "completed",
  }
}

export function summarizeJobProgress(
  job: PublicJobRecord,
  latestEvent: PublicJobEvent | undefined,
  t: Translate,
): JobProgressSummary {
  if (isRunningJobStatus(job.status)) {
    const acceptanceCheck = parseAcceptanceCheckSummary(latestEvent?.summary)
    const currentRoundCheckpointKey = inferCurrentRoundCheckpointKey(job)
    const runningStage = latestEvent?.stage || acceptanceCheck?.stage || job.phase
    const runningHeadline = acceptanceCheck?.checkId
      ? renderAcceptanceCheckLabel(acceptanceCheck.checkId, acceptanceCheck.label, t)
      : currentRoundCheckpointKey
        ? renderAcceptanceCheckLabel(currentRoundCheckpointKey, currentRoundCheckpointKey, t)
      : latestEvent?.type
        ? describeExecutionEvent(latestEvent, t)
        : t("jobs.list.runningDetail", {
            phase: renderExecutionStageLabel(job.phase, t),
          })
    const runningPhaseLabel = renderExecutionStageLabel(job.phase, t)
    return {
      headline:
        runningHeadline ||
        job.status_context?.summary ||
        t("jobs.list.runningDetail", { phase: runningPhaseLabel }),
      detail: [
        renderRunningStageDetail(runningStage, latestEvent?.at, t),
        latestEvent?.status,
        latestEvent?.at ? formatDate(latestEvent.at) : undefined,
      ]
        .filter(Boolean)
        .join(" · "),
      tone: "running",
    }
  }

  if (job.status === "failed") {
    return {
      headline:
        job.failure_context?.last_error_summary ||
        job.status_context?.summary ||
        t("jobs.status.failed"),
      detail: [
        renderFailureDomainLabel(job.failure_context?.failure_domain, t),
        renderFailureCategoryLabel(job.failure_context?.failure_category, t),
      ]
        .filter((value) => value && value !== "-")
        .join(" · "),
      tone: "failed",
    }
  }

  if (job.status === "completed") {
    return {
      headline: latestEvent?.summary || t("jobs.list.completedSummary"),
      detail: job.finished_at ? formatDate(job.finished_at) : undefined,
      tone: "completed",
    }
  }

  const pendingHumanApprovalCount = getPendingHumanApprovalCount(job)
  if (pendingHumanApprovalCount > 0) {
    return {
      headline:
        job.status_context?.summary ||
        t("jobs.list.awaitingApprovals", { count: pendingHumanApprovalCount }),
      detail: job.status_context ? renderStatusContextReason(job.status_context, t) : undefined,
      tone: "blocked",
    }
  }

  return {
    headline: job.status_context?.summary || t("jobs.list.idleSummary"),
    detail: job.phase || undefined,
    tone: "idle",
  }
}

function inferCurrentRoundCheckpointKey(job: PublicJobRecord) {
  const explicitCheckpointKey = job.current_round?.checkpoint_key?.trim()
  if (explicitCheckpointKey) {
    return explicitCheckpointKey
  }
  const currentPhase = job.current_round?.current_phase?.trim()
  if (currentPhase !== "validate") {
    return ""
  }
  return "check-flutter-test"
}

function getJobListPriority(job: PublicJobRecord) {
  let priority = 0

  if (job.status === "failed") {
    priority += 50
  }

  if (getPendingHumanApprovalCount(job) > 0) {
    priority += 30
  }

  if (job.status_context?.suggested_action) {
    priority += 15
  }

  if (isRunningJobStatus(job.status)) {
    priority += 10
  }

  if (job.status === "completed") {
    priority -= 5
  }

  return priority
}