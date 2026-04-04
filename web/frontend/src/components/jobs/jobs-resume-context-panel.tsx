import {
  type PublicNotification,
  type PublicJobRecord,
} from "@/api/system"
import {
  DetailSection,
  KeyValue,
} from "@/components/jobs/jobs-page-primitives"
import { JobSuggestedActionButton } from "@/components/jobs/jobs-suggested-action-button"

type Translate = (key: string, options?: Record<string, unknown>) => string

export function JobResumeContextPanel({
  t,
  job,
  suggestedActionLabel,
  failureDomainLabel,
  failureCategoryLabel,
  executableSuggestedAction,
  onRunSuggestedAction,
}: {
  t: Translate
  job: PublicJobRecord
  suggestedActionLabel: string
  failureDomainLabel: string
  failureCategoryLabel: string
  executableSuggestedAction: boolean
  onRunSuggestedAction: (notification: PublicNotification) => void
}) {
  if (!job.resume_context) {
    return null
  }

  return (
    <DetailSection title={t("jobs.detail.resumeContext")}>
      <div className="grid gap-3 sm:grid-cols-2">
        <KeyValue
          label={t("jobs.detail.resumeAllowed")}
          value={
            job.resume_context.resume_allowed
              ? t("jobs.detail.booleanTrue")
              : t("jobs.detail.booleanFalse")
          }
        />
        <KeyValue
          label={t("jobs.detail.recommendedResumeMode")}
          value={job.resume_context.recommended_resume_mode}
        />
        <KeyValue label={t("jobs.detail.suggestedAction")} value={suggestedActionLabel} />
        <KeyValue label={t("jobs.detail.failureDomain")} value={failureDomainLabel} />
        <KeyValue label={t("jobs.detail.failureCategory")} value={failureCategoryLabel} />
        <KeyValue
          label={t("jobs.detail.requiresHumanConfirmation")}
          value={
            job.resume_context.requires_human_confirmation
              ? t("jobs.detail.booleanTrue")
              : t("jobs.detail.booleanFalse")
          }
        />
        <KeyValue
          label={t("jobs.detail.requiresPreservedWorkspace")}
          value={
            job.resume_context.requires_preserved_workspace
              ? t("jobs.detail.booleanTrue")
              : t("jobs.detail.booleanFalse")
          }
        />
      </div>
      <KeyValue
        label={t("jobs.detail.preservedWorkspacePath")}
        value={job.resume_context.preserved_workspace_path}
        code
      />
      {job.resume_context.suggested_action ? (
        <div className="pt-2">
          <JobSuggestedActionButton
            t={t}
            actionLabel={suggestedActionLabel}
            executable={executableSuggestedAction}
            onClick={() =>
              onRunSuggestedAction({
                notification_id: `resume-${job.job_id}`,
                type: "resume_context_action",
                job_id: job.job_id,
                suggested_action: job.resume_context!.suggested_action,
                created_at: job.updated_at,
                failure_domain: job.resume_context!.failure_domain,
                failure_category: job.resume_context!.failure_category,
              })
            }
          />
        </div>
      ) : null}
    </DetailSection>
  )
}