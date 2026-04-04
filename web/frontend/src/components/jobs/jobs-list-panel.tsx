import type { ReactNode } from "react"

import type { PublicJobRecord } from "@/api/system"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { EmptyBlock, ErrorBlock, LoadingBlock } from "@/components/jobs/jobs-page-primitives"
import { cn } from "@/lib/utils"

export type JobListItemViewModel = {
  job: PublicJobRecord
  displayTitle: string
  headline: string
  supportingSummary: string
  attentionCount: number
}

type Translate = (key: string, options?: Record<string, unknown>) => string

function getJobListToneClass(job: PublicJobRecord, attentionCount: number, selected: boolean) {
  if (selected) {
    return "border-primary/40 bg-primary/5"
  }
  if (job.status === "failed") {
    return "border-rose-200 bg-rose-50/70 hover:bg-rose-50"
  }
  if (attentionCount > 0) {
    return "border-amber-200 bg-amber-50/60 hover:bg-amber-50"
  }
  if (job.status === "completed") {
    return "border-emerald-200 bg-emerald-50/60 hover:bg-emerald-50"
  }
  return "border-border/60 hover:bg-muted/40"
}

export function JobListPanel({
  t,
  statusFilters,
  statusFilter,
  onChangeStatusFilter,
  jobSearchQuery,
  onChangeJobSearchQuery,
  jobItems,
  visibleCount,
  selectedJobId,
  items,
  loading,
  errorMessage,
  onSelectJob,
  renderStatusBadge,
  renderStageLabel,
  formatDate,
  countJobsForFilter,
}: {
  t: Translate
  statusFilters: string[]
  statusFilter: string
  onChangeStatusFilter: (filter: string) => void
  jobSearchQuery: string
  onChangeJobSearchQuery: (value: string) => void
  jobItems: PublicJobRecord[]
  visibleCount: number
  selectedJobId: string
  items: JobListItemViewModel[]
  loading: boolean
  errorMessage?: string
  onSelectJob: (jobId: string) => void
  renderStatusBadge: (status: string) => ReactNode
  renderStageLabel: (phase: string | undefined) => string
  formatDate: (value?: string) => string
  countJobsForFilter: (filter: string) => number
}) {
  return (
    <Card className="border-border/60 bg-card/80" size="sm">
      <CardHeader>
        <div className="flex flex-col gap-2 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <CardTitle>{t("jobs.list.title")}</CardTitle>
            <CardDescription>
              {t("jobs.list.description")} {t("jobs.list.resultSummary", {
                visible: visibleCount,
                total: jobItems.length,
              })}
            </CardDescription>
          </div>
        </div>
        <div className="rounded-2xl border border-border/60 bg-muted/15 p-2">
          <div className="flex flex-col gap-2 lg:flex-row lg:items-center">
            <div className="min-w-0 flex-1">
              <Input
                value={jobSearchQuery}
                onChange={(event) => onChangeJobSearchQuery(event.target.value)}
                placeholder={t("jobs.list.searchPlaceholder")}
                className="h-8 border-border/50 bg-background/80"
              />
            </div>
            <div className="flex gap-2 overflow-x-auto pb-1 lg:justify-end lg:pb-0">
              {statusFilters.map((filter) => (
                <Button
                  key={filter}
                  variant={statusFilter === filter ? "default" : "outline"}
                  size="sm"
                  className="h-8 shrink-0 rounded-full px-3"
                  onClick={() => onChangeStatusFilter(filter)}
                >
                  {t(`jobs.filters.${filter}`)}
                  <span className="ml-1 text-[11px] opacity-80">{countJobsForFilter(filter)}</span>
                </Button>
              ))}
            </div>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-2.5">
        {loading ? (
          <LoadingBlock label={t("labels.loading")} />
        ) : errorMessage ? (
          <ErrorBlock message={errorMessage} />
        ) : items.length === 0 ? (
          <EmptyBlock message={t("jobs.list.empty")} />
        ) : (
          items.map(({ job, displayTitle, headline, supportingSummary, attentionCount }) => (
            <button
              key={job.job_id}
              type="button"
              onClick={() => onSelectJob(job.job_id)}
              className={cn(
                "w-full rounded-xl border p-3 text-left transition-colors select-text",
                getJobListToneClass(job, attentionCount, selectedJobId === job.job_id),
              )}
            >
              <div className="flex flex-col gap-2">
                <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                  {renderStatusBadge(job.status)}
                  <span className="text-muted-foreground text-[11px] uppercase tracking-wide">
                    {renderStageLabel(job.phase)}
                  </span>
                  {attentionCount > 0 ? (
                    <span className="rounded-full border border-amber-200 bg-amber-50 px-2 py-0.5 text-[11px] font-medium text-amber-950">
                      {t("jobs.list.pendingActions", { count: attentionCount })}
                    </span>
                  ) : null}
                  <span className="text-muted-foreground ml-auto shrink-0 text-[11px]">
                    {formatDate(job.updated_at)}
                  </span>
                </div>

                <div className="min-w-0 space-y-1">
                  <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
                    <div className="line-clamp-1 text-base font-semibold leading-5">{displayTitle}</div>
                    <div className="text-muted-foreground truncate text-xs">{job.job_id}</div>
                  </div>
                  <div className="line-clamp-1 text-sm font-medium leading-5">{headline}</div>
                  {supportingSummary.trim() !== "" && supportingSummary.trim() !== headline.trim() ? (
                    <div className="text-muted-foreground line-clamp-1 text-xs leading-5">
                      {supportingSummary}
                    </div>
                  ) : null}
                </div>
              </div>
            </button>
          ))
        )}
      </CardContent>
    </Card>
  )
}
