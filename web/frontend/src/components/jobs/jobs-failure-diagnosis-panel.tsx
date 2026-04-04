import type { PublicJobRecord } from "@/api/system"
import { DetailSection, KeyValue } from "@/components/jobs/jobs-page-primitives"
import type {
  FailureDiagnosisSummary,
  FailureGuidanceCard,
} from "@/components/jobs/jobs-state-model"
import { Button } from "@/components/ui/button"

type Translate = (key: string, options?: Record<string, unknown>) => string

export function JobFailureDiagnosisPanel({
  t,
  job,
  failureDiagnosisSummary,
  failureGuidance,
  renderFailureDomainLabel,
  renderFailureCategoryLabel,
  renderFailureGuidanceActionLabel,
  onFocusTarget,
}: {
  t: Translate
  job: PublicJobRecord
  failureDiagnosisSummary?: FailureDiagnosisSummary
  failureGuidance: FailureGuidanceCard[]
  renderFailureDomainLabel: (domain?: string) => string
  renderFailureCategoryLabel: (category?: string) => string
  renderFailureGuidanceActionLabel: (action: string) => string
  onFocusTarget: (targetId: string) => void
}) {
  if (!job.failure_context) {
    return null
  }

  return (
    <DetailSection title={t("jobs.detail.failureContext")}>
      {failureDiagnosisSummary ? (
        <div className="space-y-4">
          <div className="rounded-xl border border-rose-200 bg-rose-50/80 p-4 text-rose-950">
            <div className="text-xs font-medium uppercase tracking-wide opacity-75">
              {t("jobs.detail.failurePanelTitle")}
            </div>
            <div className="mt-2 text-base font-semibold">{failureDiagnosisSummary.summary}</div>
            <div className="mt-3 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
              <KeyValue label={t("jobs.detail.failureStage")} value={failureDiagnosisSummary.stageLabel} />
              <KeyValue
                label={t("jobs.detail.failureImpact")}
                value={failureDiagnosisSummary.impactLabel}
              />
              <KeyValue
                label={t("jobs.detail.retryable")}
                value={
                  job.failure_context.retryable
                    ? t("jobs.detail.booleanTrue")
                    : t("jobs.detail.booleanFalse")
                }
              />
              <KeyValue
                label={t("jobs.detail.nextAction")}
                value={failureDiagnosisSummary.nextActionLabel}
              />
            </div>
            {failureDiagnosisSummary.evidencePaths.length > 0 ? (
              <div className="mt-4 space-y-2">
                <div className="text-xs font-medium uppercase tracking-wide opacity-75">
                  {t("jobs.detail.failureEvidencePriority")}
                </div>
                <div className="rounded-lg bg-background/70 p-3 font-mono text-xs break-all whitespace-pre-wrap text-foreground">
                  {failureDiagnosisSummary.evidencePaths.join("\n")}
                </div>
              </div>
            ) : null}
          </div>

          <details className="rounded-xl border border-border/60 bg-muted/10 p-4">
            <summary className="cursor-pointer list-none text-sm font-medium">
              {t("jobs.detail.failureTechnicalContext")}
            </summary>
            <div className="mt-4 space-y-4">
              <div className="grid gap-3 sm:grid-cols-2">
                <KeyValue
                  label={t("jobs.detail.failureSignature")}
                  value={job.failure_context.failure_signature}
                />
                <KeyValue
                  label={t("jobs.detail.failureDomain")}
                  value={renderFailureDomainLabel(job.failure_context.failure_domain)}
                />
                <KeyValue
                  label={t("jobs.detail.failureCategory")}
                  value={renderFailureCategoryLabel(job.failure_context.failure_category)}
                />
                <KeyValue
                  label={t("jobs.detail.lastError")}
                  value={job.failure_context.last_error_summary}
                />
              </div>
            </div>
          </details>
        </div>
      ) : null}
      {failureGuidance.length > 0 ? (
        <div className="space-y-3 pt-2">
          <div className="text-sm font-medium">{t("jobs.detail.failureGuidance")}</div>
          {failureGuidance.map((item) => (
            <div
              key={item.key}
              className="space-y-3 rounded-xl border border-amber-200 bg-amber-50/70 p-3"
            >
              <div className="space-y-1">
                <div className="text-sm font-medium text-amber-950">{item.title}</div>
                <div className="text-sm text-amber-900/90">{item.description}</div>
              </div>
              {item.evidencePaths.length > 0 ? (
                <div className="space-y-1">
                  <div className="text-xs text-amber-900/80">
                    {t("jobs.detail.failureGuidanceEvidence")}
                  </div>
                  <div className="rounded-lg bg-background/70 p-2 font-mono text-xs break-all whitespace-pre-wrap">
                    {item.evidencePaths.join("\n")}
                  </div>
                </div>
              ) : null}
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
    </DetailSection>
  )
}