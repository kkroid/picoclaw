import { IconLoader2, IconLock, IconPlayerPlay, IconPower, IconRefresh } from "@tabler/icons-react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { type ReactNode, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import {
  acknowledgeNotification,
  getJob,
  getJobArtifacts,
  getJobEvents,
  getJobs,
  getNotifications,
  getOrchestratorStatus,
  rebuildNotifications,
  startOrchestratorWatch,
  stopOrchestratorWatch,
  type ArtifactItem,
  type OrchestratorStatus,
  type PublicJobEvent,
  type PublicNotification,
  unlockOrchestratorWatch,
} from "@/api/system"
import { PageHeader } from "@/components/page-header"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { cn } from "@/lib/utils"

const STATUS_FILTERS = ["all", "running", "failed", "completed", "cancelled"]
const RUNNING_JOB_STATUSES = new Set(["running", "running_builder"])

export function JobsPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [statusFilter, setStatusFilter] = useState("all")
  const [selectedJobId, setSelectedJobId] = useState("")

  const allJobsQuery = useQuery({
    queryKey: ["appfactory", "jobs", "all"],
    queryFn: () => getJobs(),
    refetchInterval: 10000,
  })

  const jobsQuery = useQuery({
    queryKey: ["appfactory", "jobs", statusFilter],
    queryFn: () =>
      statusFilter === "all" || statusFilter === "running"
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

  const jobItems = jobsQuery.data?.items ?? []
  const effectiveSelectedJobId = jobItems.some(
    (item) => item.job_id === selectedJobId,
  )
    ? selectedJobId
    : (jobItems[0]?.job_id ?? "")

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

  const rebuildNotificationsMutation = useMutation({
    mutationFn: rebuildNotifications,
    onSuccess: async () => {
      toast.success(t("jobs.toasts.notificationsRebuilt"))
      await queryClient.invalidateQueries({
        queryKey: ["appfactory", "notifications"],
      })
    },
    onError: (error) => {
      toast.error(getErrorMessage(error, t("jobs.toasts.actionFailed")))
    },
  })

  const acknowledgeMutation = useMutation({
    mutationFn: acknowledgeNotification,
    onSuccess: async () => {
      toast.success(t("jobs.toasts.notificationAcked"))
      await queryClient.invalidateQueries({
        queryKey: ["appfactory", "notifications"],
      })
    },
    onError: (error) => {
      toast.error(getErrorMessage(error, t("jobs.toasts.actionFailed")))
    },
  })

  const refreshOrchestrator = async () => {
    await queryClient.invalidateQueries({
      queryKey: ["system", "orchestrator-status"],
    })
  }

  const startWatchMutation = useMutation({
    mutationFn: () => startOrchestratorWatch(15),
    onSuccess: async () => {
      toast.success(t("header.orchestrator.toast.started"))
      await refreshOrchestrator()
    },
    onError: (error) => {
      toast.error(getErrorMessage(error, t("header.orchestrator.toast.error")))
    },
  })

  const stopWatchMutation = useMutation({
    mutationFn: stopOrchestratorWatch,
    onSuccess: async () => {
      toast.success(t("header.orchestrator.toast.stopped"))
      await refreshOrchestrator()
    },
    onError: (error) => {
      toast.error(getErrorMessage(error, t("header.orchestrator.toast.error")))
    },
  })

  const unlockWatchMutation = useMutation({
    mutationFn: () => unlockOrchestratorWatch(true),
    onSuccess: async () => {
      toast.success(t("header.orchestrator.toast.unlocked"))
      await refreshOrchestrator()
    },
    onError: (error) => {
      toast.error(getErrorMessage(error, t("header.orchestrator.toast.error")))
    },
  })

  const allJobs = allJobsQuery.data?.items ?? []
  const visibleJobs = jobItems.filter((item) =>
    matchesJobStatusFilter(item.status, statusFilter),
  )
  const notifications = notificationsQuery.data?.items ?? []
  const selectedJob = selectedJobQuery.data
  const jobEvents = eventsQuery.data?.items ?? []
  const artifactItems = artifactsQuery.data?.items ?? []
  const orchestratorBusy =
    orchestratorQuery.isLoading ||
    startWatchMutation.isPending ||
    stopWatchMutation.isPending ||
    unlockWatchMutation.isPending
  const watcherRunning = orchestratorQuery.data?.watch_runner_state === "running"
  const watcherStopping = orchestratorQuery.data?.watch_runner_state === "stopping"
  const lockIssue =
    orchestratorQuery.data?.watch_lock_state === "held_by_other" ||
    orchestratorQuery.data?.watch_lock_state === "stale"

  const runningCount = allJobs.filter((item) => isRunningJobStatus(item.status)).length
  const attentionCount = allJobs.filter(
    (item) => item.status === "failed" || item.human_approvals.length > 0,
  ).length
  const pendingNotificationCount = notifications.filter(
    (item) => !item.acknowledged,
  ).length
  const approvalCount = allJobs.reduce(
    (total, item) => total + item.human_approvals.length,
    0,
  )

  const refreshAll = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ["appfactory", "jobs"] }),
      queryClient.invalidateQueries({ queryKey: ["appfactory", "notifications"] }),
      queryClient.invalidateQueries({ queryKey: ["system", "orchestrator-status"] }),
      queryClient.invalidateQueries({ queryKey: ["appfactory", "job"] }),
      queryClient.invalidateQueries({ queryKey: ["appfactory", "job-events"] }),
      queryClient.invalidateQueries({ queryKey: ["appfactory", "job-artifacts"] }),
    ])
  }

  return (
    <div className="flex h-full flex-col">
      <PageHeader title={t("navigation.jobs")}>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={() => void refreshAll()}>
            <IconRefresh className="size-4" />
            {t("jobs.actions.refresh")}
          </Button>
          <Button
            variant="secondary"
            size="sm"
            disabled={rebuildNotificationsMutation.isPending}
            onClick={() => rebuildNotificationsMutation.mutate()}
          >
            {rebuildNotificationsMutation.isPending ? (
              <IconLoader2 className="size-4 animate-spin" />
            ) : null}
            {t("jobs.actions.rebuildNotifications")}
          </Button>
        </div>
      </PageHeader>

      <div className="flex-1 overflow-auto px-4 py-3 sm:px-6">
        <div className="mx-auto flex w-full max-w-7xl flex-col gap-6 pb-8">
          <p className="text-muted-foreground text-sm">{t("jobs.description")}</p>

          <section className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
            <SummaryCard
              title={t("jobs.summary.total")}
              value={String(allJobs.length)}
              detail={t("jobs.summary.visible", { count: visibleJobs.length })}
            />
            <SummaryCard
              title={t("jobs.summary.running")}
              value={String(runningCount)}
              detail={t("jobs.summary.pendingNotifications", {
                count: pendingNotificationCount,
              })}
            />
            <SummaryCard
              title={t("jobs.summary.attention")}
              value={String(attentionCount)}
              detail={t("jobs.summary.humanApprovals", { count: approvalCount })}
            />
            <SummaryCard
              title={t("jobs.summary.orchestrator")}
              value={renderOrchestratorLabel(orchestratorQuery.data, t)}
              detail={formatDate(orchestratorQuery.data?.updated_at)}
              loading={orchestratorQuery.isLoading}
            />
          </section>

          <Card className="border-border/60 bg-card/80" size="sm">
            <CardHeader>
              <CardTitle>{t("jobs.orchestrator.title")}</CardTitle>
              <CardDescription>
                {renderOrchestratorDetail(orchestratorQuery.data, t)}
              </CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
              <div className="space-y-1 text-sm">
                <div className="flex items-center gap-2">
                  <span
                    className={cn(
                      "inline-flex size-2 rounded-full",
                      getOrchestratorToneClass(orchestratorQuery.data),
                    )}
                  />
                  <span className="font-medium">
                    {renderOrchestratorLabel(orchestratorQuery.data, t)}
                  </span>
                </div>
                {orchestratorQuery.data?.watch_lock_owner_id ? (
                  <div className="text-muted-foreground text-xs">
                    {t("header.orchestrator.menu.lockOwner", {
                      owner: orchestratorQuery.data.watch_lock_owner_id,
                    })}
                  </div>
                ) : null}
                {orchestratorQuery.data?.watch_runner_last_error ? (
                  <div className="text-destructive text-xs">
                    {orchestratorQuery.data.watch_runner_last_error}
                  </div>
                ) : null}
              </div>

              <div className="flex flex-wrap gap-2">
                {watcherRunning || watcherStopping ? (
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={orchestratorBusy}
                    onClick={() => stopWatchMutation.mutate()}
                  >
                    {stopWatchMutation.isPending ? (
                      <IconLoader2 className="size-4 animate-spin" />
                    ) : (
                      <IconPower className="size-4" />
                    )}
                    {t("header.orchestrator.action.stopWatch")}
                  </Button>
                ) : (
                  <Button
                    variant="default"
                    size="sm"
                    disabled={
                      orchestratorBusy ||
                      orchestratorQuery.data?.watch_lock_state === "held_by_other"
                    }
                    onClick={() => startWatchMutation.mutate()}
                  >
                    {startWatchMutation.isPending ? (
                      <IconLoader2 className="size-4 animate-spin" />
                    ) : (
                      <IconPlayerPlay className="size-4" />
                    )}
                    {t("header.orchestrator.action.startWatch")}
                  </Button>
                )}

                <Button
                  variant="outline"
                  size="sm"
                  disabled={orchestratorBusy || !lockIssue}
                  onClick={() => unlockWatchMutation.mutate()}
                >
                  {unlockWatchMutation.isPending ? (
                    <IconLoader2 className="size-4 animate-spin" />
                  ) : (
                    <IconLock className="size-4" />
                  )}
                  {t("header.orchestrator.action.forceUnlock")}
                </Button>

                <Button
                  variant="outline"
                  size="sm"
                  disabled={orchestratorBusy}
                  onClick={() => void refreshOrchestrator()}
                >
                  <IconRefresh className="size-4" />
                  {t("header.orchestrator.action.refresh")}
                </Button>
              </div>
            </CardContent>
          </Card>

          <section className="grid gap-6 xl:grid-cols-[minmax(0,1.1fr)_minmax(340px,0.9fr)]">
            <div className="space-y-6">
              <Card className="border-border/60 bg-card/80" size="sm">
                <CardHeader>
                  <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
                    <div>
                      <CardTitle>{t("jobs.list.title")}</CardTitle>
                      <CardDescription>{t("jobs.list.description")}</CardDescription>
                    </div>
                    <div className="flex flex-wrap gap-2">
                      {STATUS_FILTERS.map((filter) => (
                        <Button
                          key={filter}
                          variant={statusFilter === filter ? "default" : "outline"}
                          size="sm"
                          onClick={() => setStatusFilter(filter)}
                        >
                          {t(`jobs.filters.${filter}`)}
                        </Button>
                      ))}
                    </div>
                  </div>
                </CardHeader>
                <CardContent className="space-y-3">
                  {jobsQuery.isLoading ? (
                    <LoadingBlock label={t("labels.loading")} />
                  ) : jobsQuery.error ? (
                    <ErrorBlock
                      message={getErrorMessage(
                        jobsQuery.error,
                        t("jobs.states.loadErrorJobs"),
                      )}
                    />
                  ) : visibleJobs.length === 0 ? (
                    <EmptyBlock message={t("jobs.list.empty")} />
                  ) : (
                    visibleJobs.map((job) => (
                      <button
                        key={job.job_id}
                        type="button"
                        onClick={() => setSelectedJobId(job.job_id)}
                        className={cn(
                          "w-full rounded-xl border p-4 text-left transition-colors",
                          effectiveSelectedJobId === job.job_id
                            ? "border-primary/40 bg-primary/5"
                            : "border-border/60 hover:bg-muted/40",
                        )}
                      >
                        <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
                          <div className="min-w-0 flex-1 space-y-2">
                            <div className="flex flex-wrap items-center gap-2">
                              <span className="font-semibold">{job.job_id}</span>
                              <StatusBadge status={job.status} t={t} />
                              <span className="text-muted-foreground rounded-md bg-muted px-2 py-0.5 text-xs">
                                {job.phase || t("jobs.detail.none")}
                              </span>
                            </div>
                            <div className="grid gap-1 text-sm sm:grid-cols-2">
                              <KeyValue label={t("jobs.list.prd")} value={job.prd_id} inline />
                              <KeyValue
                                label={t("jobs.list.template")}
                                value={job.template_id}
                                inline
                              />
                              <KeyValue
                                label={t("jobs.list.updatedAt")}
                                value={formatDate(job.updated_at)}
                                inline
                              />
                              <KeyValue
                                label={t("jobs.list.approvals")}
                                value={String(job.human_approvals.length)}
                                inline
                              />
                            </div>
                          </div>
                          <div className="text-muted-foreground shrink-0 text-xs">
                            {t("jobs.list.artifacts", {
                              count: job.artifacts?.length ?? 0,
                            })}
                          </div>
                        </div>
                      </button>
                    ))
                  )}
                </CardContent>
              </Card>

              <Card className="border-border/60 bg-card/80" size="sm">
                <CardHeader>
                  <CardTitle>{t("jobs.events.title")}</CardTitle>
                  <CardDescription>{t("jobs.events.description")}</CardDescription>
                </CardHeader>
                <CardContent className="space-y-3">
                  {effectiveSelectedJobId === "" ? (
                    <EmptyBlock message={t("jobs.detail.empty")} />
                  ) : eventsQuery.isLoading ? (
                    <LoadingBlock label={t("labels.loading")} />
                  ) : eventsQuery.error ? (
                    <ErrorBlock
                      message={getErrorMessage(
                        eventsQuery.error,
                        t("jobs.states.loadErrorEvents"),
                      )}
                    />
                  ) : jobEvents.length === 0 ? (
                    <EmptyBlock message={t("jobs.events.empty")} />
                  ) : (
                    jobEvents.map((event, index) => (
                      <EventRow
                        key={`${event.at}-${event.type}-${index}`}
                        event={event}
                        t={t}
                      />
                    ))
                  )}
                </CardContent>
              </Card>
            </div>

            <div className="space-y-6">
              <Card className="border-border/60 bg-card/80" size="sm">
                <CardHeader>
                  <CardTitle>{t("jobs.notifications.title")}</CardTitle>
                  <CardDescription>{t("jobs.notifications.description")}</CardDescription>
                </CardHeader>
                <CardContent className="space-y-3">
                  {notificationsQuery.isLoading ? (
                    <LoadingBlock label={t("labels.loading")} />
                  ) : notificationsQuery.error ? (
                    <ErrorBlock
                      message={getErrorMessage(
                        notificationsQuery.error,
                        t("jobs.states.loadErrorNotifications"),
                      )}
                    />
                  ) : notifications.length === 0 ? (
                    <EmptyBlock message={t("jobs.notifications.empty")} />
                  ) : (
                    notifications.slice(0, 6).map((item) => (
                      <NotificationRow
                        key={item.notification_id}
                        item={item}
                        t={t}
                        onAcknowledge={(notificationId) =>
                          acknowledgeMutation.mutate(notificationId)
                        }
                        pending={acknowledgeMutation.isPending}
                        pendingId={acknowledgeMutation.variables}
                      />
                    ))
                  )}
                </CardContent>
              </Card>

              <Card className="border-border/60 bg-card/80" size="sm">
                <CardHeader>
                  <CardTitle>{t("jobs.detail.title")}</CardTitle>
                  <CardDescription>{t("jobs.detail.description")}</CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                  {effectiveSelectedJobId === "" ? (
                    <EmptyBlock message={t("jobs.detail.empty")} />
                  ) : selectedJobQuery.isLoading ? (
                    <LoadingBlock label={t("labels.loading")} />
                  ) : selectedJobQuery.error ? (
                    <ErrorBlock
                      message={getErrorMessage(
                        selectedJobQuery.error,
                        t("jobs.states.loadErrorDetail"),
                      )}
                    />
                  ) : selectedJob ? (
                    <>
                      <div className="flex flex-wrap items-center gap-2">
                        <div className="text-base font-semibold">{selectedJob.job_id}</div>
                        <StatusBadge status={selectedJob.status} t={t} />
                        <span className="text-muted-foreground rounded-md bg-muted px-2 py-0.5 text-xs">
                          {selectedJob.phase || t("jobs.detail.none")}
                        </span>
                      </div>

                      <div className="grid gap-3 sm:grid-cols-2">
                        <KeyValue label={t("jobs.detail.prd")} value={selectedJob.prd_id} />
                        <KeyValue
                          label={t("jobs.detail.template")}
                          value={selectedJob.template_id}
                        />
                        <KeyValue
                          label={t("jobs.detail.createdAt")}
                          value={formatDate(selectedJob.created_at)}
                        />
                        <KeyValue
                          label={t("jobs.detail.updatedAt")}
                          value={formatDate(selectedJob.updated_at)}
                        />
                        <KeyValue
                          label={t("jobs.detail.startedAt")}
                          value={formatDate(selectedJob.started_at)}
                        />
                        <KeyValue
                          label={t("jobs.detail.finishedAt")}
                          value={formatDate(selectedJob.finished_at)}
                        />
                      </div>

                      <DetailSection title={t("jobs.detail.runtime")}>
                        <div className="grid gap-3 sm:grid-cols-2">
                          <KeyValue
                            label={t("jobs.detail.builder")}
                            value={selectedJob.runtime?.builder}
                          />
                          <KeyValue
                            label={t("jobs.detail.workerPool")}
                            value={selectedJob.runtime?.worker_pool}
                          />
                          <KeyValue
                            label={t("jobs.detail.networkPolicy")}
                            value={selectedJob.runtime?.network_policy}
                          />
                          <KeyValue
                            label={t("jobs.detail.phase")}
                            value={selectedJob.phase}
                          />
                        </div>
                      </DetailSection>

                      <DetailSection title={t("jobs.detail.budgets")}>
                        <div className="grid gap-3 sm:grid-cols-2">
                          <KeyValue
                            label={t("jobs.detail.iterationBudget")}
                            value={String(selectedJob.budgets.iteration_budget)}
                          />
                          <KeyValue
                            label={t("jobs.detail.tokenBudget")}
                            value={String(selectedJob.budgets.token_budget)}
                          />
                          <KeyValue
                            label={t("jobs.detail.elapsedIterations")}
                            value={String(selectedJob.budgets.elapsed_iterations ?? 0)}
                          />
                          <KeyValue
                            label={t("jobs.detail.consumedTokens")}
                            value={String(selectedJob.budgets.consumed_tokens ?? 0)}
                          />
                        </div>
                      </DetailSection>

                      <DetailSection title={t("jobs.detail.paths")}>
                        <div className="space-y-2">
                          <KeyValue
                            label={t("jobs.detail.workspacePath")}
                            value={selectedJob.workspace_path}
                            code
                          />
                          <KeyValue
                            label={t("jobs.detail.artifactDir")}
                            value={selectedJob.artifact_dir}
                            code
                          />
                          <KeyValue
                            label={t("jobs.detail.builderInputPath")}
                            value={selectedJob.builder_input_path}
                            code
                          />
                          <KeyValue
                            label={t("jobs.detail.builderOutputPath")}
                            value={selectedJob.builder_output_path}
                            code
                          />
                          <KeyValue
                            label={t("jobs.detail.summaryLogPath")}
                            value={selectedJob.logs?.summary_path}
                            code
                          />
                          <KeyValue
                            label={t("jobs.detail.eventLogPath")}
                            value={selectedJob.logs?.event_log_path}
                            code
                          />
                        </div>
                      </DetailSection>

                      {selectedJob.failure_context ? (
                        <DetailSection title={t("jobs.detail.failureContext")}>
                          <div className="grid gap-3 sm:grid-cols-2">
                            <KeyValue
                              label={t("jobs.detail.failureSignature")}
                              value={selectedJob.failure_context.failure_signature}
                            />
                            <KeyValue
                              label={t("jobs.detail.retryable")}
                              value={
                                selectedJob.failure_context.retryable
                                  ? t("jobs.detail.booleanTrue")
                                  : t("jobs.detail.booleanFalse")
                              }
                            />
                          </div>
                          <KeyValue
                            label={t("jobs.detail.lastError")}
                            value={selectedJob.failure_context.last_error_summary}
                          />
                        </DetailSection>
                      ) : null}

                      {selectedJob.resume_context ? (
                        <DetailSection title={t("jobs.detail.resumeContext")}>
                          <div className="grid gap-3 sm:grid-cols-2">
                            <KeyValue
                              label={t("jobs.detail.resumeAllowed")}
                              value={
                                selectedJob.resume_context.resume_allowed
                                  ? t("jobs.detail.booleanTrue")
                                  : t("jobs.detail.booleanFalse")
                              }
                            />
                            <KeyValue
                              label={t("jobs.detail.recommendedResumeMode")}
                              value={selectedJob.resume_context.recommended_resume_mode}
                            />
                            <KeyValue
                              label={t("jobs.detail.nextAction")}
                              value={selectedJob.resume_context.next_action}
                            />
                            <KeyValue
                              label={t("jobs.detail.failureDomain")}
                              value={renderFailureDomainLabel(
                                selectedJob.resume_context.failure_domain,
                                t,
                              )}
                            />
                            <KeyValue
                              label={t("jobs.detail.failureCategory")}
                              value={selectedJob.resume_context.failure_category}
                            />
                            <KeyValue
                              label={t("jobs.detail.requiresHumanConfirmation")}
                              value={
                                selectedJob.resume_context.requires_human_confirmation
                                  ? t("jobs.detail.booleanTrue")
                                  : t("jobs.detail.booleanFalse")
                              }
                            />
                            <KeyValue
                              label={t("jobs.detail.requiresPreservedWorkspace")}
                              value={
                                selectedJob.resume_context.requires_preserved_workspace
                                  ? t("jobs.detail.booleanTrue")
                                  : t("jobs.detail.booleanFalse")
                              }
                            />
                          </div>
                          <KeyValue
                            label={t("jobs.detail.preservedWorkspacePath")}
                            value={selectedJob.resume_context.preserved_workspace_path}
                            code
                          />
                        </DetailSection>
                      ) : null}

                      <DetailSection title={t("jobs.detail.humanApprovals")}>
                        {selectedJob.human_approvals.length === 0 ? (
                          <div className="text-muted-foreground text-sm">
                            {t("jobs.list.noApprovals")}
                          </div>
                        ) : (
                          <div className="flex flex-wrap gap-2">
                            {selectedJob.human_approvals.map((approvalId) => (
                              <span
                                key={approvalId}
                                className="rounded-md bg-amber-100 px-2 py-1 text-xs font-medium text-amber-800"
                              >
                                {approvalId}
                              </span>
                            ))}
                          </div>
                        )}
                      </DetailSection>
                    </>
                  ) : null}
                </CardContent>
              </Card>

              <Card className="border-border/60 bg-card/80" size="sm">
                <CardHeader>
                  <CardTitle>{t("jobs.artifacts.title")}</CardTitle>
                  <CardDescription>{t("jobs.artifacts.description")}</CardDescription>
                </CardHeader>
                <CardContent className="space-y-3">
                  {effectiveSelectedJobId === "" ? (
                    <EmptyBlock message={t("jobs.detail.empty")} />
                  ) : artifactsQuery.isLoading ? (
                    <LoadingBlock label={t("labels.loading")} />
                  ) : artifactsQuery.error ? (
                    <ErrorBlock
                      message={getErrorMessage(
                        artifactsQuery.error,
                        t("jobs.states.loadErrorArtifacts"),
                      )}
                    />
                  ) : artifactItems.length === 0 ? (
                    <EmptyBlock message={t("jobs.artifacts.empty")} />
                  ) : (
                    artifactItems.map((item) => (
                      <ArtifactRow key={item.artifact_id} item={item} t={t} />
                    ))
                  )}
                </CardContent>
              </Card>
            </div>
          </section>
        </div>
      </div>
    </div>
  )
}

function SummaryCard({
  title,
  value,
  detail,
  loading,
}: {
  title: string
  value: string
  detail: string
  loading?: boolean
}) {
  return (
    <Card className="border-border/60 bg-card/80" size="sm">
      <CardHeader>
        <CardDescription>{title}</CardDescription>
        <CardTitle className="text-2xl font-semibold">
          {loading ? <IconLoader2 className="size-5 animate-spin" /> : value}
        </CardTitle>
      </CardHeader>
      <CardContent className="text-muted-foreground text-xs">{detail}</CardContent>
    </Card>
  )
}

function DetailSection({
  title,
  children,
}: {
  title: string
  children: ReactNode
}) {
  return (
    <section className="space-y-2 border-t border-border/60 pt-4 first:border-t-0 first:pt-0">
      <div className="text-sm font-semibold">{title}</div>
      {children}
    </section>
  )
}

function KeyValue({
  label,
  value,
  code,
  inline,
}: {
  label: string
  value?: string
  code?: boolean
  inline?: boolean
}) {
  const content = value && value.trim() !== "" ? value : "-"
  return (
    <div className={cn("space-y-1", inline && "space-y-0")}>
      <div className="text-muted-foreground text-xs">{label}</div>
      <div className={cn("text-sm", code && "font-mono break-all text-xs")}>
        {content}
      </div>
    </div>
  )
}

function LoadingBlock({ label }: { label: string }) {
  return (
    <div className="text-muted-foreground flex items-center gap-2 py-6 text-sm">
      <IconLoader2 className="size-4 animate-spin" />
      {label}
    </div>
  )
}

function ErrorBlock({ message }: { message: string }) {
  return (
    <div className="text-destructive rounded-lg bg-destructive/10 px-4 py-3 text-sm">
      {message}
    </div>
  )
}

function EmptyBlock({ message }: { message: string }) {
  return (
    <div className="text-muted-foreground rounded-lg border border-dashed px-4 py-6 text-sm">
      {message}
    </div>
  )
}

function StatusBadge({
  status,
  t,
}: {
  status: string
  t: (key: string, options?: Record<string, unknown>) => string
}) {
  return (
    <span
      className={cn(
        "rounded-md px-2 py-0.5 text-xs font-medium",
        status === "completed" && "bg-emerald-100 text-emerald-800",
        isRunningJobStatus(status) && "bg-sky-100 text-sky-800",
        status === "failed" && "bg-rose-100 text-rose-800",
        status === "cancelled" && "bg-slate-200 text-slate-700",
        !["completed", "failed", "cancelled"].includes(status) &&
          !isRunningJobStatus(status) &&
          "bg-amber-100 text-amber-800",
      )}
    >
      {renderStatusLabel(status, t)}
    </span>
  )
}

function EventRow({
  event,
  t,
}: {
  event: PublicJobEvent
  t: (key: string, options?: Record<string, unknown>) => string
}) {
  return (
    <div className="rounded-xl border border-border/60 p-3">
      <div className="flex flex-wrap items-center gap-2">
        <span className="rounded-md bg-muted px-2 py-0.5 text-xs font-medium">
          {event.type}
        </span>
        {event.stage ? (
          <span className="text-muted-foreground text-xs">{event.stage}</span>
        ) : null}
        {event.status ? (
          <span className="text-muted-foreground text-xs">{event.status}</span>
        ) : null}
        {event.failure_domain ? (
          <span className="rounded-md bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-800">
            {renderFailureDomainLabel(event.failure_domain, t)}
          </span>
        ) : null}
      </div>
      <div className="mt-2 text-sm font-medium">{event.summary || "-"}</div>
      <div className="text-muted-foreground mt-2 space-y-1 text-xs">
        <div>{formatDate(event.at)}</div>
        {event.snapshot_path ? (
          <div className="font-mono break-all">{event.snapshot_path}</div>
        ) : null}
        {event.failure_category ? <div>{event.failure_category}</div> : null}
        {event.recommended_resume_mode ? <div>{event.recommended_resume_mode}</div> : null}
      </div>
    </div>
  )
}

function NotificationRow({
  item,
  t,
  onAcknowledge,
  pending,
  pendingId,
}: {
  item: PublicNotification
  t: (key: string, options?: Record<string, unknown>) => string
  onAcknowledge: (notificationId: string) => void
  pending: boolean
  pendingId?: string
}) {
  return (
    <div className="rounded-xl border border-border/60 p-3">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 flex-1 space-y-2">
          <div className="flex flex-wrap items-center gap-2">
            <span className="rounded-md bg-muted px-2 py-0.5 text-xs font-medium">
              {item.type}
            </span>
            {item.status ? <StatusBadge status={item.status} t={t} /> : null}
            {item.failure_domain ? (
              <span className="rounded-md bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-800">
                {renderFailureDomainLabel(item.failure_domain, t)}
              </span>
            ) : null}
          </div>
          <div className="text-sm font-medium">{item.summary || item.notification_id}</div>
          <div className="text-muted-foreground space-y-1 text-xs">
            <div>{formatDate(item.created_at)}</div>
            {item.job_id ? <div>{item.job_id}</div> : null}
            {item.failure_category ? <div>{item.failure_category}</div> : null}
            {item.suggested_action ? (
              <div>
                {t("jobs.notifications.suggestedAction", {
                  action: item.suggested_action,
                })}
              </div>
            ) : null}
            {item.links && item.links.length > 0 ? (
              <div className="font-mono break-all whitespace-pre-wrap">
                {item.links.join("\n")}
              </div>
            ) : null}
          </div>
        </div>
        <Button
          variant="outline"
          size="sm"
          disabled={pending || item.acknowledged}
          onClick={() => onAcknowledge(item.notification_id)}
        >
          {pending && pendingId === item.notification_id ? (
            <IconLoader2 className="size-4 animate-spin" />
          ) : null}
          {item.acknowledged
            ? t("jobs.notifications.acknowledged")
            : t("jobs.actions.acknowledge")}
        </Button>
      </div>
    </div>
  )
}

function ArtifactRow({
  item,
  t,
}: {
  item: ArtifactItem
  t: (key: string, options?: Record<string, unknown>) => string
}) {
  return (
    <div className="rounded-xl border border-border/60 p-3">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-medium">{item.label || item.artifact_id}</span>
        <span className="rounded-md bg-muted px-2 py-0.5 text-xs">
          {item.artifact_type}
        </span>
        <span
          className={cn(
            "rounded-md px-2 py-0.5 text-xs font-medium",
            item.produced ? "bg-emerald-100 text-emerald-800" : "bg-rose-100 text-rose-800",
          )}
        >
          {item.produced ? t("jobs.artifacts.produced") : t("jobs.artifacts.missing")}
        </span>
        {item.required ? (
          <span className="rounded-md bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-800">
            {t("jobs.artifacts.required")}
          </span>
        ) : null}
      </div>
      <div className="text-muted-foreground mt-2 space-y-1 text-xs">
        <div className="font-mono break-all">{item.path}</div>
        {item.description ? <div>{item.description}</div> : null}
      </div>
    </div>
  )
}

function renderStatusLabel(
  status: string,
  t: (key: string, options?: Record<string, unknown>) => string,
) {
  if (isRunningJobStatus(status)) {
    return t("jobs.status.running")
  }
  const key = `jobs.status.${status}`
  const translated = t(key)
  return translated === key ? status : translated
}

function isRunningJobStatus(status: string) {
  return RUNNING_JOB_STATUSES.has(status)
}

function matchesJobStatusFilter(status: string, filter: string) {
  if (filter === "all") {
    return true
  }
  if (filter === "running") {
    return isRunningJobStatus(status)
  }
  return status === filter
}

function renderOrchestratorLabel(
  data: OrchestratorStatus | undefined,
  t: (key: string, options?: Record<string, unknown>) => string,
) {
  if (!data) {
    return t("header.orchestrator.status.loading")
  }
  if (data.watch_runner_state === "running") {
    return t("header.orchestrator.status.running")
  }
  if (data.watch_runner_state === "stopping") {
    return t("header.orchestrator.status.stopping")
  }
  if (data.watch_lock_state === "held_by_other") {
    return t("header.orchestrator.status.locked")
  }
  if (data.watch_lock_state === "stale") {
    return t("header.orchestrator.status.stale")
  }
  return t("header.orchestrator.status.idle")
}

function renderOrchestratorDetail(
  data: OrchestratorStatus | undefined,
  t: (key: string, options?: Record<string, unknown>) => string,
) {
  if (!data) {
    return t("header.orchestrator.menu.loading")
  }
  if (data.watch_runner_state === "running") {
    return t("header.orchestrator.menu.runningDetail", {
      interval: data.watch_runner_interval_seconds ?? 15,
    })
  }
  if (data.watch_runner_state === "stopping") {
    return t("header.orchestrator.menu.stoppingDetail")
  }
  if (data.watch_lock_state === "held_by_other") {
    return t("header.orchestrator.menu.lockedDetail")
  }
  if (data.watch_lock_state === "stale") {
    return t("header.orchestrator.menu.staleDetail")
  }
  return t("header.orchestrator.menu.idleDetail")
}

function getOrchestratorToneClass(data: OrchestratorStatus | undefined) {
  if (!data) {
    return "bg-muted-foreground/40"
  }
  if (data.watch_runner_state === "running") {
    return "bg-emerald-500"
  }
  if (data.watch_runner_state === "stopping") {
    return "bg-amber-500"
  }
  if (data.watch_lock_state === "held_by_other") {
    return "bg-amber-500"
  }
  if (data.watch_lock_state === "stale") {
    return "bg-orange-500"
  }
  return "bg-slate-400"
}

function renderFailureDomainLabel(
  domain: string | undefined,
  t: (key: string, options?: Record<string, unknown>) => string,
) {
  if (!domain) {
    return "-"
  }
  const key = `jobs.failureDomain.${domain}`
  const translated = t(key)
  return translated === key ? domain : translated
}

function formatDate(value?: string) {
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

function getErrorMessage(error: unknown, fallback: string) {
  return error instanceof Error ? error.message : fallback
}