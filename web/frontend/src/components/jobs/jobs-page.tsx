import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"

import { getAppConfig } from "@/api/config"
import {
  getJob,
  getJobArtifacts,
  getJobEvents,
  getJobs,
  getNotifications,
  getOrchestratorStatus,
  type ArtifactItem,
  type PublicJobEvent,
  type PublicJobRecord,
  type PublicNotification,
} from "@/api/system"
import { PageHeader } from "@/components/page-header"
import {
  JobsCreateDialog,
} from "@/components/jobs/jobs-create-dialog"
import { useJobsCreateFlow } from "@/components/jobs/jobs-create-flow"
import {
  buildDeliveryFormState,
  buildFollowUpFormState,
} from "@/components/jobs/jobs-delivery-utils"
import { JobsDetailContent } from "@/components/jobs/jobs-detail-content"
import { JobsDetailPanel } from "@/components/jobs/jobs-detail-panel"
import { useJobsDeliveryFlow } from "@/components/jobs/jobs-delivery-flow"
import { JobsPageToolbar } from "@/components/jobs/jobs-page-toolbar"
import { useJobsOrchestratorFlow } from "@/components/jobs/jobs-orchestrator-flow"
import { useJobsPageUiState } from "@/components/jobs/jobs-page-ui-state"
import { JobsRuntimeModeBanner } from "@/components/jobs/jobs-runtime-mode-banner"
import {
  countJobsForFilter,
} from "@/components/jobs/jobs-page-state"
import { buildJobsPageViewModel } from "@/components/jobs/jobs-page-view-model"
import {
  refreshAllJobWorkspace,
  refreshOrchestratorStatus,
} from "@/components/jobs/jobs-page-actions"
import { JobListPanel } from "@/components/jobs/jobs-list-panel"
import { JobsOrchestratorPanel } from "@/components/jobs/jobs-orchestrator-panel"
import { JobsWorkspaceOverviewPanel } from "@/components/jobs/jobs-workspace-overview-panel"
import {
  formatDate,
  getBuilderRuntimeMode,
  getErrorMessage,
} from "@/components/jobs/jobs-page-utils"
import { createJobsWorkspaceController } from "@/components/jobs/jobs-workspace-controller"
import { useJobsWorkspaceActions } from "@/components/jobs/jobs-workspace-actions"
import { renderExecutionStageLabel } from "@/components/jobs/jobs-state-model"
import { JobsStatusBadge } from "@/components/jobs/jobs-status-badge"

const STATUS_FILTERS = ["all", "attention", "running", "failed", "completed", "cancelled"]

const EMPTY_JOBS: PublicJobRecord[] = []
const EMPTY_NOTIFICATIONS: PublicNotification[] = []
const EMPTY_EVENTS: PublicJobEvent[] = []
const EMPTY_ARTIFACT_ITEMS: ArtifactItem[] = []

export function JobsPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [statusFilter, setStatusFilter] = useState("all")
  const [jobSearchQuery, setJobSearchQuery] = useState("")

  const allJobsQuery = useQuery({
    queryKey: ["appfactory", "jobs", "all"],
    queryFn: () => getJobs(),
    refetchInterval: 10000,
  })

  const jobsQuery = useQuery({
    queryKey: ["appfactory", "jobs", statusFilter],
    queryFn: () =>
      statusFilter === "all" || statusFilter === "running" || statusFilter === "attention"
        ? getJobs()
        : getJobs(statusFilter),
    refetchInterval: 10000,
  })

  const notificationsQuery = useQuery({
    queryKey: ["appfactory", "notifications"],
    queryFn: () => getNotifications(false),
    refetchInterval: 10000,
  })

  const orchestratorQuery = useQuery({
    queryKey: ["system", "orchestrator-status"],
    queryFn: getOrchestratorStatus,
    refetchInterval: 5000,
  })

  const appConfigQuery = useQuery({
    queryKey: ["system", "app-config"],
    queryFn: getAppConfig,
    refetchInterval: 60000,
  })

  const jobItems = jobsQuery.data?.items ?? EMPTY_JOBS
  const {
    effectiveSelectedJobId,
    detailTab,
    setDetailTab,
    showSystemStatus,
    setShowSystemStatus,
    showCreateJobSheet,
    setShowCreateJobSheet,
    createJobForm,
    setCreateJobForm,
    resetCreateJobForm,
    showDeliveryForm,
    setShowDeliveryForm,
    showFollowUpForm,
    setShowFollowUpForm,
    deliveryForm,
    setDeliveryForm,
    followUpForm,
    setFollowUpForm,
    setSelectedJobId,
  } = useJobsPageUiState({
    jobItems,
  })

  const selectedJobQuery = useQuery({
    queryKey: ["appfactory", "job", effectiveSelectedJobId],
    queryFn: () => getJob(effectiveSelectedJobId),
    enabled: effectiveSelectedJobId !== "",
    refetchInterval: 10000,
  })

  const eventsQuery = useQuery({
    queryKey: ["appfactory", "job-events", effectiveSelectedJobId],
    queryFn: () => getJobEvents(effectiveSelectedJobId),
    enabled: effectiveSelectedJobId !== "",
    refetchInterval: 10000,
  })

  const artifactsQuery = useQuery({
    queryKey: ["appfactory", "job-artifacts", effectiveSelectedJobId],
    queryFn: () => getJobArtifacts(effectiveSelectedJobId),
    enabled: effectiveSelectedJobId !== "",
    refetchInterval: 10000,
  })

  const refreshOrchestrator = async () => refreshOrchestratorStatus(queryClient)

  const workspaceController = createJobsWorkspaceController({
    queryClient,
    setSelectedJobId,
    setDetailTab,
    focusElementById,
  })

  const allJobs = allJobsQuery.data?.items ?? EMPTY_JOBS
  const notifications = notificationsQuery.data?.items ?? EMPTY_NOTIFICATIONS
  const selectedJob = selectedJobQuery.data
  const jobEvents = eventsQuery.data?.items ?? EMPTY_EVENTS
  const artifactItems = artifactsQuery.data?.items ?? EMPTY_ARTIFACT_ITEMS
  const builderRuntimeMode = getBuilderRuntimeMode(appConfigQuery.data)

  const {
    startWatchMutation,
    stopWatchMutation,
    unlockWatchMutation,
    orchestratorBusy,
    watcherRunning,
    watcherStopping,
    lockIssue,
  } = useJobsOrchestratorFlow({
    t,
    data: orchestratorQuery.data,
    loading: orchestratorQuery.isLoading,
    refreshOrchestrator,
  })

  const refreshAll = async () => refreshAllJobWorkspace(queryClient)

  const {
    visibleJobs,
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
  } = buildJobsPageViewModel({
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
  })

  const { createJobMutation, handleCreateJobSubmit } = useJobsCreateFlow({
    t,
    form: createJobForm,
    refreshAll,
    setSelectedJobId,
    setStatusFilter,
    setShowCreateJobSheet,
    setCreateJobForm,
  })

  useEffect(() => {
    setDeliveryForm(
      buildDeliveryFormState(selectedJob?.delivery_context, artifactItems),
    )
    setFollowUpForm(
      buildFollowUpFormState(selectedJob?.delivery_context, artifactItems),
    )
    setShowDeliveryForm(false)
    setShowFollowUpForm(false)
  }, [effectiveSelectedJobId, selectedJob?.delivery_context, artifactItems])

  const {
    prepareReviewMutation,
    recordDeliveryMutation,
    recordFollowUpMutation,
    handleDeliveryFormSubmit,
    handleFollowUpFormSubmit,
    handleCancelDelivery,
    handleCancelFollowUp,
  } = useJobsDeliveryFlow({
    t,
    selectedJob,
    latestRunId,
    artifactItems,
    deliveryForm,
    followUpForm,
    refreshAll,
    workspaceController,
    setShowDeliveryForm,
    setShowFollowUpForm,
    setDeliveryForm,
    setFollowUpForm,
  })

  const {
    rebuildNotificationsMutation,
    acknowledgeMutation,
    statusActionMutation,
    handleNotificationAction,
  } = useJobsWorkspaceActions({
    t,
    queryClient,
    refreshAll,
    refreshOrchestrator,
    startWatch: () => startWatchMutation.mutate(),
    workspaceController,
    watcherRunning,
    watcherStopping,
    watchLockState: orchestratorQuery.data?.watch_lock_state,
  })

  return (
    <div className="flex h-full flex-col">
      <JobsCreateDialog
        open={showCreateJobSheet}
        pending={createJobMutation.isPending}
        form={createJobForm}
        t={t}
        onOpenChange={(open) => {
          if (open) {
            setShowCreateJobSheet(true)
            return
          }
          resetCreateJobForm()
        }}
        onFormChange={setCreateJobForm}
        onSubmit={handleCreateJobSubmit}
        onCancel={resetCreateJobForm}
      />

      <PageHeader title={t("navigation.jobs")}>
        <JobsPageToolbar
          t={t}
          rebuildPending={rebuildNotificationsMutation.isPending}
          onOpenCreate={() => setShowCreateJobSheet(true)}
          onRefresh={() => void refreshAll()}
          onRebuildNotifications={() => rebuildNotificationsMutation.mutate()}
        />
      </PageHeader>

      <div className="flex-1 overflow-auto px-4 py-3 sm:px-6">
        <div className="mx-auto flex w-full max-w-7xl flex-col gap-6 pb-8">
          <p className="text-muted-foreground text-sm">{t("jobs.description")}</p>

          <JobsRuntimeModeBanner t={t} builderRuntimeMode={builderRuntimeMode} />

          <JobsWorkspaceOverviewPanel
            t={t}
            runningCount={runningCount}
            attentionCount={attentionCount}
            pendingNotificationCount={pendingNotificationCount}
            failedCount={failedCount}
            approvalCount={approvalCount}
            completedCount={completedCount}
            allJobCount={allJobs.length}
            visibleJobCount={visibleJobs.length}
            workspaceFocus={workspaceFocus}
            selectedJob={selectedJob}
            selectedJobActionCount={selectedJobActionCount}
            onOpenOverview={workspaceController.openOverview}
          />

          <JobsOrchestratorPanel
            t={t}
            data={orchestratorQuery.data}
            showSystemStatus={showSystemStatus}
            orchestratorBusy={orchestratorBusy}
            watcherRunning={watcherRunning}
            watcherStopping={watcherStopping}
            lockIssue={lockIssue}
            startPending={startWatchMutation.isPending}
            stopPending={stopWatchMutation.isPending}
            unlockPending={unlockWatchMutation.isPending}
            onToggleShowSystemStatus={() => setShowSystemStatus((current) => !current)}
            onStartWatch={() => startWatchMutation.mutate()}
            onStopWatch={() => stopWatchMutation.mutate()}
            onUnlockWatch={() => unlockWatchMutation.mutate()}
            onRefresh={() => void refreshOrchestrator()}
          />

          <section className="grid gap-6 xl:grid-cols-[minmax(0,1.1fr)_minmax(340px,0.9fr)]">
            <div className="space-y-6">
              <JobListPanel
                t={t}
                statusFilters={STATUS_FILTERS}
                statusFilter={statusFilter}
                onChangeStatusFilter={setStatusFilter}
                jobSearchQuery={jobSearchQuery}
                onChangeJobSearchQuery={setJobSearchQuery}
                jobItems={jobItems}
                visibleCount={jobListItems.length}
                selectedJobId={effectiveSelectedJobId}
                items={jobListItems}
                loading={jobsQuery.isLoading}
                errorMessage={
                  jobsQuery.error
                    ? getErrorMessage(jobsQuery.error, t("jobs.states.loadErrorJobs"))
                    : undefined
                }
                onSelectJob={setSelectedJobId}
                renderStatusBadge={(status) => <JobsStatusBadge status={status} t={t} />}
                renderStageLabel={(phase) => renderExecutionStageLabel(phase, t)}
                formatDate={formatDate}
                countJobsForFilter={(filter) => countJobsForFilter(jobItems, filter)}
              />

            </div>

            <div className="space-y-6">
              <JobsDetailPanel
                t={t}
                detailTab={detailTab}
                onChangeTab={setDetailTab}
                selectedJob={effectiveSelectedJobId === "" ? undefined : selectedJob}
                loading={selectedJobQuery.isLoading}
                errorMessage={
                  selectedJobQuery.error
                    ? getErrorMessage(selectedJobQuery.error, t("jobs.states.loadErrorDetail"))
                    : undefined
                }
                renderStatusBadge={(status) => <JobsStatusBadge status={status} t={t} />}
                renderStageLabel={(phase) => renderExecutionStageLabel(phase, t)}
              >
                {(selectedJob) => (
                  <JobsDetailContent
                    t={t}
                    detailTab={detailTab}
                    selectedJob={selectedJob}
                    artifactItems={artifactItems}
                    jobEvents={jobEvents}
                    latestRunId={latestRunId}
                    latestJobEvent={latestJobEvent}
                    selectedJobProgress={selectedJobProgress}
                    selectedJobExecutionStage={selectedJobExecutionStage}
                    selectedJobExecutionBreakdown={selectedJobExecutionBreakdown}
                    selectedJobExecutionStateSummary={selectedJobExecutionStateSummary}
                    selectedJobExecutionEventStream={selectedJobExecutionEventStream}
                    selectedJobNotifications={selectedJobNotifications}
                    failureGuidance={failureGuidance}
                    failureDiagnosisSummary={failureDiagnosisSummary}
                    notificationsLoading={notificationsQuery.isLoading}
                    notificationsError={notificationsQuery.error}
                    eventsLoading={eventsQuery.isLoading}
                    eventsError={eventsQuery.error}
                    artifactsLoading={artifactsQuery.isLoading}
                    artifactsError={artifactsQuery.error}
                    statusActionPending={statusActionMutation.isPending}
                    statusActionJobId={statusActionMutation.variables?.job.job_id}
                    onRunStatusAction={(job, action) =>
                      statusActionMutation.mutate({ job, action })
                    }
                    onRunNotificationAction={handleNotificationAction}
                    onAcknowledge={(notificationId) => acknowledgeMutation.mutate(notificationId)}
                    acknowledgePending={acknowledgeMutation.isPending}
                    acknowledgePendingId={acknowledgeMutation.variables}
                    formatDate={formatDate}
                    workspaceController={workspaceController}
                    showDeliveryForm={showDeliveryForm}
                    setShowDeliveryForm={setShowDeliveryForm}
                    showFollowUpForm={showFollowUpForm}
                    setShowFollowUpForm={setShowFollowUpForm}
                    deliveryForm={deliveryForm}
                    setDeliveryForm={setDeliveryForm}
                    followUpForm={followUpForm}
                    setFollowUpForm={setFollowUpForm}
                    suggestedEvidencePaths={suggestedEvidencePaths}
                    deviceSuggestedEvidencePaths={deviceSuggestedEvidencePaths}
                    followUpSuggestedEvidencePaths={followUpSuggestedEvidencePaths}
                    deliveryChecklistItems={deliveryChecklistItems}
                    prepareReviewPending={prepareReviewMutation.isPending}
                    onPrepareReview={() => {
                      if (latestRunId) {
                        prepareReviewMutation.mutate(latestRunId)
                      }
                    }}
                    recordDeliveryPending={recordDeliveryMutation.isPending}
                    onSubmitDelivery={handleDeliveryFormSubmit}
                    onCancelDelivery={handleCancelDelivery}
                    recordFollowUpPending={recordFollowUpMutation.isPending}
                    onSubmitFollowUp={handleFollowUpFormSubmit}
                    onCancelFollowUp={handleCancelFollowUp}
                  />
                )}
              </JobsDetailPanel>
            </div>
          </section>
        </div>
      </div>
    </div>
  )
}

function focusElementById(id: string) {
  const element = document.getElementById(id)
  if (!element) {
    return
  }

  element.scrollIntoView({
    behavior: "smooth",
    block: "start",
  })

  if (!element.hasAttribute("tabindex")) {
    element.setAttribute("tabindex", "-1")
  }
  element.focus({ preventScroll: true })
}
