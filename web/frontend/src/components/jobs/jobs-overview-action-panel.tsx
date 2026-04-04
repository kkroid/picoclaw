import type { ReactNode } from "react"

import type { PublicJobRecord, PublicNotification } from "@/api/system"
import { JobNotificationRow } from "@/components/jobs/jobs-detail-rows"
import {
  DetailSection,
  EmptyBlock,
  ErrorBlock,
  LoadingBlock,
} from "@/components/jobs/jobs-page-primitives"
import { JobSuggestedActionButton } from "@/components/jobs/jobs-suggested-action-button"
import type { FailureGuidanceCard } from "@/components/jobs/jobs-state-model"
import { Button } from "@/components/ui/button"

type Translate = (key: string, options?: Record<string, unknown>) => string

export function JobOverviewActionPanel({
  t,
  job,
  loading,
  errorMessage,
  selectedJobNotifications,
  failureGuidance,
  statusActionTitle,
  statusActionSummary,
  statusActionLabel,
  statusActionExecutable,
  statusActionPending,
  onRunStatusAction,
  onRunNotificationAction,
  onAcknowledge,
  acknowledgePending,
  acknowledgePendingId,
  formatDate,
  renderStatusBadge,
  renderFailureGuidanceActionLabel,
  onFocusTarget,
}: {
  t: Translate
  job: PublicJobRecord
  loading: boolean
  errorMessage?: string
  selectedJobNotifications: PublicNotification[]
  failureGuidance: FailureGuidanceCard[]
  statusActionTitle: string
  statusActionSummary: string
  statusActionLabel: string
  statusActionExecutable: boolean
  statusActionPending: boolean
  onRunStatusAction: () => void
  onRunNotificationAction: (item: PublicNotification) => void
  onAcknowledge: (notificationId: string) => void
  acknowledgePending: boolean
  acknowledgePendingId?: string
  formatDate: (value?: string) => string
  renderStatusBadge: (status: string) => ReactNode
  renderFailureGuidanceActionLabel: (action: string) => string
  onFocusTarget: (targetId: string) => void
}) {
  return (
    <DetailSection title={t("jobs.notifications.title")}>
      {loading ? (
        <LoadingBlock label={t("labels.loading")} />
      ) : errorMessage ? (
        <ErrorBlock message={errorMessage} />
      ) : (
        <div className="space-y-3">
          {job.status_context ? (
            <div className="rounded-xl border border-border/60 bg-muted/20 p-4">
              <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
                <div className="min-w-0 flex-1 space-y-1">
                  <div className="text-sm font-medium">{statusActionTitle}</div>
                  <div className="text-muted-foreground text-sm/6">{statusActionSummary}</div>
                </div>
                {statusActionExecutable ? (
                  <JobSuggestedActionButton
                    t={t}
                    actionLabel={statusActionLabel}
                    executable
                    disabled={statusActionPending}
                    pending={statusActionPending}
                    onClick={onRunStatusAction}
                  />
                ) : null}
              </div>
            </div>
          ) : null}

          {selectedJobNotifications.map((item) => (
            <JobNotificationRow
              key={item.notification_id}
              item={item}
              t={t}
              formatDate={formatDate}
              renderStatusBadge={renderStatusBadge}
              onRunSuggestedAction={onRunNotificationAction}
              onAcknowledge={onAcknowledge}
              pending={acknowledgePending}
              pendingId={acknowledgePendingId}
            />
          ))}

          {failureGuidance.length > 0 ? (
            <div className="space-y-3 rounded-xl border border-amber-200 bg-amber-50/70 p-4">
              <div className="text-sm font-medium text-amber-950">{t("jobs.detail.failureGuidance")}</div>
              {failureGuidance.slice(0, 2).map((item) => (
                <div key={item.key} className="space-y-2">
                  <div className="text-sm font-medium text-amber-950">{item.title}</div>
                  <div className="text-sm text-amber-900/90">{item.description}</div>
                  {item.actions.length > 0 ? (
                    <div className="flex flex-wrap gap-2">
                      {item.actions.map((action) => (
                        <Button
                          key={`${item.key}-${action.key}`}
                          variant="outline"
                          size="sm"
                          onClick={() => onFocusTarget(action.targetId)}
                        >
                          {renderFailureGuidanceActionLabel(action.key)}
                        </Button>
                      ))}
                    </div>
                  ) : null}
                </div>
              ))}
            </div>
          ) : null}

          {!job.status_context &&
          selectedJobNotifications.length === 0 &&
          failureGuidance.length === 0 ? (
            <EmptyBlock message={t("jobs.notifications.empty")} />
          ) : null}
        </div>
      )}
    </DetailSection>
  )
}