import type { ArtifactItem, PublicJobEvent, PublicJobRecord, PublicNotification } from "@/api/system"
import { buildDeliveryChecklistItems, collectDeviceEvidencePaths, collectFollowUpEvidencePaths, collectSuggestedEvidencePaths, getLatestRunId, type DeliveryFormState } from "@/components/jobs/jobs-delivery-utils"
import { buildWorkspaceFocusSummary, compareJobsForList, getJobAttentionCount, getJobListSupportingSummary, isAttentionJob, isRunningJobStatus, matchesJobSearchQuery, matchesJobStatusFilter, summarizeJobProgress } from "@/components/jobs/jobs-page-state"
import { buildExecutionBreakdown, buildExecutionEventStreamSummary, buildFailureDiagnosisSummary, buildFailureGuidance, getLatestJobEvent, summarizeExecutionCheckpointStates, summarizeExecutionStage } from "@/components/jobs/jobs-state-model"
import { getPendingHumanApprovalCount, renderFailureCategoryLabel, renderFailureGuidanceActionLabel, renderSuggestedActionLabel } from "@/components/jobs/jobs-action-utils"

type Translate = (key: string, options?: Record<string, unknown>) => string

export type JobListViewItem = {
  job: PublicJobRecord
  displayTitle: string
  headline: string
  supportingSummary: string
  attentionCount: number
}

export function buildJobsPageViewModel({
  allJobs,
  jobItems,
  notifications,
  selectedJob,
  effectiveSelectedJobId,
  jobEvents,
  artifactItems,
  statusFilter,
  jobSearchQuery,
  deliveryForm,
  t,
}: {
  allJobs: PublicJobRecord[]
  jobItems: PublicJobRecord[]
  notifications: PublicNotification[]
  selectedJob?: PublicJobRecord
  effectiveSelectedJobId: string
  jobEvents: PublicJobEvent[]
  artifactItems: ArtifactItem[]
  statusFilter: string
  jobSearchQuery: string
  deliveryForm: DeliveryFormState
  t: Translate
}) {
  const visibleJobs = jobItems.filter(
    (item) =>
      matchesJobStatusFilter(item, statusFilter) &&
      matchesJobSearchQuery(item, jobSearchQuery),
  )
  const sortedVisibleJobs = [...visibleJobs].sort(compareJobsForList)
  const jobListItems: JobListViewItem[] = sortedVisibleJobs.map((job) => {
    const progress = summarizeJobProgress(job, undefined, t)
    return {
      job,
      displayTitle: job.title?.trim() || job.job_id,
      headline: progress.headline,
      supportingSummary: getJobListSupportingSummary(job, progress, t),
      attentionCount: getJobAttentionCount(job),
    }
  })

  const latestRunId = getLatestRunId(jobEvents)
  const latestJobEvent = getLatestJobEvent(jobEvents)
  const selectedJobProgress = selectedJob
    ? summarizeJobProgress(selectedJob, latestJobEvent, t)
    : undefined
  const selectedJobExecutionStage = selectedJob
    ? summarizeExecutionStage(selectedJob, latestJobEvent, t)
    : undefined
  const selectedJobExecutionBreakdown = selectedJob
    ? buildExecutionBreakdown(selectedJob, jobEvents, t)
    : []
  const selectedJobExecutionStateSummary = summarizeExecutionCheckpointStates(
    selectedJobExecutionBreakdown,
  )
  const selectedJobExecutionEventStream = buildExecutionEventStreamSummary(jobEvents)

  const suggestedEvidencePaths = collectSuggestedEvidencePaths(artifactItems)
  const deviceSuggestedEvidencePaths = collectDeviceEvidencePaths(artifactItems)
  const followUpSuggestedEvidencePaths = collectFollowUpEvidencePaths(artifactItems)
  const deliveryChecklistItems = buildDeliveryChecklistItems(deliveryForm, artifactItems)

  const failureGuidance = selectedJob
    ? buildFailureGuidance(selectedJob, artifactItems, t, {
        renderFailureCategoryLabel: (category) => renderFailureCategoryLabel(category, t),
      })
    : []
  const failureDiagnosisSummary = selectedJob
    ? buildFailureDiagnosisSummary(selectedJob, artifactItems, failureGuidance, t, {
        renderSuggestedActionLabel: (action) => renderSuggestedActionLabel(action, t),
        renderFailureGuidanceActionLabel: (action) =>
          renderFailureGuidanceActionLabel(action, t),
      })
    : undefined

  const runningCount = allJobs.filter((item) => isRunningJobStatus(item.status)).length
  const attentionCount = allJobs.filter((item) => isAttentionJob(item)).length
  const pendingNotificationCount = notifications.filter((item) => !item.acknowledged).length
  const approvalCount = allJobs.reduce(
    (total, item) => total + getPendingHumanApprovalCount(item),
    0,
  )
  const failedCount = allJobs.filter((item) => item.status === "failed").length
  const completedCount = allJobs.filter((item) => item.status === "completed").length
  const selectedJobNotifications = notifications
    .filter(
      (item) =>
        !item.acknowledged &&
        effectiveSelectedJobId !== "" &&
        item.job_id === effectiveSelectedJobId,
    )
    .slice(0, 3)
  const selectedJobActionCount =
    selectedJobNotifications.length +
    (selectedJob?.status_context?.suggested_action ? 1 : 0) +
    failureGuidance.length
  const workspaceFocus = buildWorkspaceFocusSummary({
    selectedJob,
    progress: selectedJobProgress,
    selectedJobActionCount,
    pendingNotificationCount,
    attentionCount,
    t,
  })

  return {
    visibleJobs,
    sortedVisibleJobs,
    jobListItems,
    latestRunId,
    latestJobEvent,
    selectedJobProgress,
    selectedJobExecutionStage,
    selectedJobExecutionBreakdown,
    selectedJobExecutionStateSummary,
    selectedJobExecutionEventStream,
    suggestedEvidencePaths,
    deviceSuggestedEvidencePaths,
    followUpSuggestedEvidencePaths,
    deliveryChecklistItems,
    failureGuidance,
    failureDiagnosisSummary,
    runningCount,
    attentionCount,
    pendingNotificationCount,
    approvalCount,
    failedCount,
    completedCount,
    selectedJobNotifications,
    selectedJobActionCount,
    workspaceFocus,
  }
}