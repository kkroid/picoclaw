import { IconLoader2 } from "@tabler/icons-react"

import type { ArtifactItem, PublicNotification } from "@/api/system"
import {
  canExecuteNotificationAction,
  isExecutableNotificationAction,
  renderDeliveryStatusLabel,
  renderDeviceVerificationStatusLabel,
  renderFailureCategoryLabel,
  renderFailureDomainLabel,
  renderReleaseFollowUpStatusLabel,
  translateStatusAction,
} from "@/components/jobs/jobs-action-utils"
import { JobSuggestedActionButton } from "@/components/jobs/jobs-suggested-action-button"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

type Translate = (key: string, options?: Record<string, unknown>) => string

export function JobNotificationRow({
  item,
  t,
  formatDate,
  renderStatusBadge,
  onRunSuggestedAction,
  onAcknowledge,
  pending,
  pendingId,
}: {
  item: PublicNotification
  t: Translate
  formatDate: (value?: string) => string
  renderStatusBadge: (status: string) => React.ReactNode
  onRunSuggestedAction: (item: PublicNotification) => void
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
            {item.status ? renderStatusBadge(item.status) : null}
            {item.delivery_status ? (
              <span className="rounded-md bg-emerald-100 px-2 py-0.5 text-xs font-medium text-emerald-800">
                {renderDeliveryStatusLabel(item.delivery_status, t)}
              </span>
            ) : null}
            {item.device_verification_status ? (
              <span className="rounded-md bg-sky-100 px-2 py-0.5 text-xs font-medium text-sky-800">
                {renderDeviceVerificationStatusLabel(item.device_verification_status, t)}
              </span>
            ) : null}
            {item.release_follow_up_status ? (
              <span className="rounded-md bg-violet-100 px-2 py-0.5 text-xs font-medium text-violet-800">
                {renderReleaseFollowUpStatusLabel(item.release_follow_up_status, t)}
              </span>
            ) : null}
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
            {item.delivery_record_path ? (
              <div className="font-mono break-all whitespace-pre-wrap">
                {item.delivery_record_path}
              </div>
            ) : null}
            {item.release_channel ? <div>{item.release_channel}</div> : null}
            {item.device_verification_status ? (
              <div>{renderDeviceVerificationStatusLabel(item.device_verification_status, t)}</div>
            ) : null}
            {item.release_follow_up_status ? (
              <div>{renderReleaseFollowUpStatusLabel(item.release_follow_up_status, t)}</div>
            ) : null}
            {item.failure_category ? (
              <div>{renderFailureCategoryLabel(item.failure_category, t)}</div>
            ) : null}
            {item.suggested_action ? (
              <div>
                {t("jobs.notifications.suggestedAction", {
                  action: translateStatusAction(item.suggested_action, t),
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
        <div className="flex shrink-0 flex-col gap-2">
          <JobSuggestedActionButton
            t={t}
            actionLabel={translateStatusAction(item.suggested_action ?? "", t)}
            executable={isExecutableNotificationAction(item.suggested_action)}
            disabled={!canExecuteNotificationAction(item)}
            onClick={() => onRunSuggestedAction(item)}
            variant="secondary"
            showManualHint={false}
          />
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
    </div>
  )
}

export function JobArtifactRow({
  item,
  localPath,
  t,
}: {
  item: ArtifactItem
  localPath: string
  t: Translate
}) {
  return (
    <div className="rounded-xl border border-border/60 p-3">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-medium">{item.label || item.artifact_id}</span>
        <span className="rounded-md bg-muted px-2 py-0.5 text-xs">{item.artifact_type}</span>
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
        <div className="font-mono break-all">{localPath}</div>
        {localPath !== item.path ? (
          <div className="font-mono break-all opacity-80">{item.path}</div>
        ) : null}
        {item.description ? <div>{item.description}</div> : null}
      </div>
    </div>
  )
}