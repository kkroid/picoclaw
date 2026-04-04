import type { ReactNode } from "react"

import type { PublicJobRecord } from "@/api/system"
import { EmptyBlock, ErrorBlock, LoadingBlock } from "@/components/jobs/jobs-page-primitives"
import type { JobWorkspaceTab } from "@/components/jobs/jobs-workspace-controller"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"

type Translate = (key: string, options?: Record<string, unknown>) => string

export function JobsDetailPanel({
  t,
  detailTab,
  onChangeTab,
  selectedJob,
  loading,
  errorMessage,
  renderStatusBadge,
  renderStageLabel,
  children,
}: {
  t: Translate
  detailTab: JobWorkspaceTab
  onChangeTab: (tab: JobWorkspaceTab) => void
  selectedJob?: PublicJobRecord
  loading: boolean
  errorMessage?: string
  renderStatusBadge: (status: string) => ReactNode
  renderStageLabel: (phase: string) => string
  children: (selectedJob: PublicJobRecord) => ReactNode
}) {
  return (
    <Card className="border-border/60 bg-card/80" size="sm">
      <CardHeader>
        <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <div>
            <CardTitle>{t("jobs.workspace.detailTitle")}</CardTitle>
            <CardDescription>{t("jobs.workspace.detailDescription")}</CardDescription>
          </div>
          <div className="flex flex-wrap gap-2">
            {(["overview", "execution", "artifacts", "delivery"] as JobWorkspaceTab[]).map((tab) => (
              <Button
                key={tab}
                variant={detailTab === tab ? "default" : "outline"}
                size="sm"
                onClick={() => onChangeTab(tab)}
              >
                {t(`jobs.workspace.tabs.${tab}`)}
              </Button>
            ))}
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        {!selectedJob ? (
          loading ? (
            <LoadingBlock label={t("labels.loading")} />
          ) : errorMessage ? (
            <ErrorBlock message={errorMessage} />
          ) : (
            <EmptyBlock message={t("jobs.detail.empty")} />
          )
        ) : (
          <>
            <div className="flex flex-wrap items-center gap-2">
              <div className="text-base font-semibold">{selectedJob.job_id}</div>
              {renderStatusBadge(selectedJob.status)}
              <span className="text-muted-foreground rounded-md bg-muted px-2 py-0.5 text-xs">
                {renderStageLabel(selectedJob.phase)}
              </span>
            </div>
            {children(selectedJob)}
          </>
        )}
      </CardContent>
    </Card>
  )
}