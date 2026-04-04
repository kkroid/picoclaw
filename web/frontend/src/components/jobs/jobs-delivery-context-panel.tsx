import type { PublicJobRecord } from "@/api/system"
import { DetailSection, KeyValue, PathList } from "@/components/jobs/jobs-page-primitives"
import { JobSuggestedActionButton } from "@/components/jobs/jobs-suggested-action-button"

type Translate = (key: string, options?: Record<string, unknown>) => string

export function JobsDeliveryContextPanel({
  t,
  job,
  formatDate,
  renderDeliveryStatusLabel,
  renderSuggestedActionLabel,
  renderRolloutPercent,
  renderDeviceVerificationStatusLabel,
  renderReleaseFollowUpStatusLabel,
  executableSuggestedAction,
  onRunSuggestedAction,
  pendingHumanApprovalCount,
  approvalReason,
  approvalNextAction,
}: {
  t: Translate
  job: PublicJobRecord
  formatDate: (value?: string) => string
  renderDeliveryStatusLabel: (status: string) => string
  renderSuggestedActionLabel: (action: string | undefined) => string
  renderRolloutPercent: (value?: number) => string
  renderDeviceVerificationStatusLabel: (status: string) => string
  renderReleaseFollowUpStatusLabel: (status: string) => string
  executableSuggestedAction: boolean
  onRunSuggestedAction: () => void
  pendingHumanApprovalCount: number
  approvalReason?: string
  approvalNextAction?: string
}) {
  const deliveryContext = job.delivery_context

  return (
    <>
      {deliveryContext ? (
        <div id="jobs-delivery-context">
          <DetailSection title={t("jobs.detail.deliveryContext")}>
            <div className="grid gap-3 sm:grid-cols-2">
              <KeyValue
                label={t("jobs.detail.deliveryStatus")}
                value={renderDeliveryStatusLabel(deliveryContext.status)}
              />
              <KeyValue
                label={t("jobs.detail.suggestedAction")}
                value={renderSuggestedActionLabel(deliveryContext.suggested_action)}
              />
              <KeyValue
                label={t("jobs.detail.releaseChannel")}
                value={deliveryContext.release_channel}
              />
              <KeyValue
                label={t("jobs.detail.rolloutPercent")}
                value={renderRolloutPercent(deliveryContext.rollout_percent)}
              />
              <KeyValue
                label={t("jobs.detail.reviewerId")}
                value={deliveryContext.reviewer_id}
              />
              <KeyValue
                label={t("jobs.detail.recordedAt")}
                value={formatDate(deliveryContext.recorded_at)}
              />
            </div>
            <KeyValue
              label={t("jobs.detail.deliverySummary")}
              value={deliveryContext.summary}
            />
            <KeyValue
              label={t("jobs.detail.deliveryRecordPath")}
              value={deliveryContext.delivery_record_path}
              code
            />
            <PathList
              label={t("jobs.detail.evidencePaths")}
              items={deliveryContext.evidence_paths}
            />
            <PathList
              label={t("jobs.detail.signedArtifactPaths")}
              items={deliveryContext.signed_artifact_paths}
            />
            <PathList
              label={t("jobs.detail.requiredChanges")}
              items={deliveryContext.required_changes}
            />
            {deliveryContext.device_verification ? (
              <div className="rounded-lg border border-border/60 bg-muted/20 p-3">
                <div className="text-sm font-medium">{t("jobs.detail.deviceVerification")}</div>
                <div className="mt-3 grid gap-3 sm:grid-cols-2">
                  <KeyValue
                    label={t("jobs.detail.deviceVerificationStatus")}
                    value={renderDeviceVerificationStatusLabel(
                      deliveryContext.device_verification.status ?? "",
                    )}
                  />
                  <KeyValue
                    label={t("jobs.detail.suggestedAction")}
                    value={renderSuggestedActionLabel(
                      deliveryContext.device_verification.suggested_action,
                    )}
                  />
                  <KeyValue
                    label={t("jobs.detail.verifiedAt")}
                    value={formatDate(deliveryContext.device_verification.verified_at)}
                  />
                  <KeyValue
                    label={t("jobs.detail.deviceVerificationRecordPath")}
                    value={deliveryContext.device_verification.record_path}
                    code
                  />
                </div>
                <KeyValue
                  label={t("jobs.detail.deviceVerificationSummary")}
                  value={deliveryContext.device_verification.summary}
                />
                <PathList
                  label={t("jobs.detail.deviceVerificationEvidencePaths")}
                  items={deliveryContext.device_verification.evidence_paths}
                />
              </div>
            ) : null}
            {deliveryContext.release_follow_up ? (
              <div className="rounded-lg border border-border/60 bg-muted/20 p-3">
                <div className="text-sm font-medium">{t("jobs.detail.releaseFollowUp")}</div>
                <div className="mt-3 grid gap-3 sm:grid-cols-2">
                  <KeyValue
                    label={t("jobs.detail.releaseFollowUpStatus")}
                    value={renderReleaseFollowUpStatusLabel(
                      deliveryContext.release_follow_up.status ?? "",
                    )}
                  />
                  <KeyValue
                    label={t("jobs.detail.suggestedAction")}
                    value={renderSuggestedActionLabel(
                      deliveryContext.release_follow_up.suggested_action,
                    )}
                  />
                  <KeyValue
                    label={t("jobs.detail.followUpOwnerId")}
                    value={deliveryContext.release_follow_up.owner_id}
                  />
                  <KeyValue
                    label={t("jobs.detail.followUpUpdatedAt")}
                    value={formatDate(deliveryContext.release_follow_up.updated_at)}
                  />
                  <KeyValue
                    label={t("jobs.detail.releaseFollowUpRecordPath")}
                    value={deliveryContext.release_follow_up.record_path}
                    code
                  />
                </div>
                <KeyValue
                  label={t("jobs.detail.releaseFollowUpSummary")}
                  value={deliveryContext.release_follow_up.summary}
                />
                <PathList
                  label={t("jobs.detail.followUpEvidencePaths")}
                  items={deliveryContext.release_follow_up.evidence_paths}
                />
              </div>
            ) : null}
            <div className="pt-2">
              <JobSuggestedActionButton
                t={t}
                actionLabel={renderSuggestedActionLabel(deliveryContext.suggested_action)}
                executable={executableSuggestedAction}
                onClick={onRunSuggestedAction}
              />
            </div>
          </DetailSection>
        </div>
      ) : null}

      <DetailSection title={t("jobs.detail.humanApprovals")}>
        {pendingHumanApprovalCount === 0 ? (
          <div className="text-muted-foreground text-sm">{t("jobs.list.noApprovals")}</div>
        ) : (
          <div className="space-y-3">
            <div className="rounded-xl border border-amber-200 bg-amber-50 px-3 py-3 text-sm text-amber-900">
              {t("jobs.list.awaitingApprovals", { count: pendingHumanApprovalCount })}
            </div>
            {approvalReason || approvalNextAction ? (
              <div className="grid gap-2 text-sm">
                {approvalReason ? (
                  <div>
                    <span className="text-muted-foreground mr-2">{t("jobs.detail.reason")}</span>
                    <span>{approvalReason}</span>
                  </div>
                ) : null}
                {approvalNextAction ? (
                  <div>
                    <span className="text-muted-foreground mr-2">{t("jobs.detail.nextAction")}</span>
                    <span>{approvalNextAction}</span>
                  </div>
                ) : null}
              </div>
            ) : null}
          </div>
        )}
      </DetailSection>
    </>
  )
}