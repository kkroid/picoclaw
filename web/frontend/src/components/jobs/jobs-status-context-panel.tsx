import type { PublicJobRecord } from "@/api/system"
import {
  DetailSection,
  KeyValue,
} from "@/components/jobs/jobs-page-primitives"
import { JobSuggestedActionButton } from "@/components/jobs/jobs-suggested-action-button"

type Translate = (key: string, options?: Record<string, unknown>) => string

export function JobStatusContextPanel({
  t,
  job,
  statusReason,
  suggestedAction,
  executable,
  actionLabel,
  pending,
  onRunSuggestedAction,
}: {
  t: Translate
  job: PublicJobRecord
  statusReason: string
  suggestedAction: string
  executable: boolean
  actionLabel: string
  pending: boolean
  onRunSuggestedAction: () => void
}) {
  return (
    <DetailSection title={t("jobs.detail.statusContext")}>
      <div className="grid gap-3 sm:grid-cols-2">
        <KeyValue label={t("jobs.detail.statusReason")} value={statusReason} />
        <KeyValue label={t("jobs.detail.suggestedAction")} value={suggestedAction} />
      </div>
      <KeyValue label={t("jobs.detail.statusSummary")} value={job.status_context?.summary} />
      {executable ? (
        <div className="pt-2">
          <JobSuggestedActionButton
            t={t}
            actionLabel={actionLabel}
            executable
            disabled={pending}
            pending={pending}
            onClick={onRunSuggestedAction}
          />
        </div>
      ) : job.status_context?.suggested_action ? (
        <div className="text-muted-foreground pt-2 text-xs">{t("jobs.detail.manualActionHint")}</div>
      ) : null}
    </DetailSection>
  )
}