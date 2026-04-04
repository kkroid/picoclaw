import type { PublicJobRecord } from "@/api/system"
import {
  KeyValue,
  SummaryCard,
} from "@/components/jobs/jobs-page-primitives"
import type { WorkspaceFocusSummary } from "@/components/jobs/jobs-page-state"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { cn } from "@/lib/utils"

type Translate = (key: string, options?: Record<string, unknown>) => string

export function JobsWorkspaceOverviewPanel({
  t,
  runningCount,
  attentionCount,
  pendingNotificationCount,
  failedCount,
  approvalCount,
  completedCount,
  allJobCount,
  visibleJobCount,
  workspaceFocus,
  selectedJob,
  selectedJobActionCount,
  onOpenOverview,
}: {
  t: Translate
  runningCount: number
  attentionCount: number
  pendingNotificationCount: number
  failedCount: number
  approvalCount: number
  completedCount: number
  allJobCount: number
  visibleJobCount: number
  workspaceFocus: WorkspaceFocusSummary
  selectedJob?: PublicJobRecord
  selectedJobActionCount: number
  onOpenOverview: () => void
}) {
  return (
    <>
      <section className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <SummaryCard
          title={t("jobs.summary.running")}
          value={String(runningCount)}
          detail={t("jobs.summary.visible", { count: visibleJobCount })}
        />
        <SummaryCard
          title={t("jobs.summary.attention")}
          value={String(attentionCount)}
          detail={t("jobs.summary.pendingNotifications", { count: pendingNotificationCount })}
        />
        <SummaryCard
          title={t("jobs.summary.failed")}
          value={String(failedCount)}
          detail={t("jobs.summary.humanApprovals", { count: approvalCount })}
        />
        <SummaryCard
          title={t("jobs.summary.completed")}
          value={String(completedCount)}
          detail={t("jobs.summary.total", { count: allJobCount })}
        />
      </section>

      <Card className="border-border/60 bg-card/80" size="sm">
        <CardHeader>
          <CardTitle>{t("jobs.workspace.focusTitle")}</CardTitle>
          <CardDescription>{t("jobs.workspace.focusDescription")}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div
            className={cn(
              "min-w-0 flex-1 rounded-xl border px-4 py-4",
              workspaceFocus.tone === "running" && "border-sky-200 bg-sky-50 text-sky-950",
              workspaceFocus.tone === "blocked" && "border-amber-200 bg-amber-50 text-amber-950",
              workspaceFocus.tone === "failed" && "border-rose-200 bg-rose-50 text-rose-950",
              workspaceFocus.tone === "completed" &&
                "border-emerald-200 bg-emerald-50 text-emerald-950",
              workspaceFocus.tone === "idle" && "border-border/60 bg-muted/30 text-foreground",
            )}
          >
            <div className="text-xs font-medium uppercase tracking-wide opacity-75">
              {t("jobs.workspace.focusPrimaryAction")}
            </div>
            <div className="mt-2 text-lg font-semibold">{workspaceFocus.headline}</div>
            <div className="mt-2 text-sm/6 opacity-85">{workspaceFocus.detail}</div>
          </div>

          <div className="flex w-full shrink-0 flex-col gap-3 lg:w-72">
            <div className="grid gap-3 sm:grid-cols-3 lg:grid-cols-1">
              <KeyValue
                label={t("jobs.workspace.focusSelectedJob")}
                value={selectedJob?.job_id || t("jobs.workspace.focusNoSelection")}
              />
              <KeyValue
                label={t("jobs.workspace.focusPendingActions")}
                value={String(selectedJobActionCount || pendingNotificationCount)}
              />
              <KeyValue label={t("jobs.summary.failed")} value={String(failedCount)} />
            </div>
            <Button variant="outline" size="sm" disabled={!selectedJob} onClick={onOpenOverview}>
              {t("jobs.workspace.focusOpenOverview")}
            </Button>
          </div>
        </CardContent>
      </Card>
    </>
  )
}