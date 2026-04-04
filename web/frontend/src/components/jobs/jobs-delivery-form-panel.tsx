import { IconLoader2 } from "@tabler/icons-react"

import { DetailSection, FieldBlock, KeyValue } from "@/components/jobs/jobs-page-primitives"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"

type Translate = (key: string, options?: Record<string, unknown>) => string

type DeliveryFormShape = {
  reviewerId: string
  status: string
  summary: string
  releaseChannel: string
  rolloutPercent: string
  evidencePaths: string
  requiredChanges: string
  signedArtifactPaths: string
  deviceVerificationStatus: string
  deviceVerificationSummary: string
  deviceVerificationEvidencePaths: string
}

type FollowUpFormShape = {
  ownerId: string
  status: string
  summary: string
  evidencePaths: string
}

type ChecklistItem = {
  key: string
  done: boolean
}

export function JobsDeliveryFormPanel({
  t,
  latestRunId,
  reviewBundleReady,
  workflowHint,
  checklistItems,
  prepareReviewPending,
  onPrepareReview,
  showDeliveryForm,
  onToggleDeliveryForm,
  deliveryForm,
  onDeliveryFieldChange,
  suggestedEvidencePaths,
  deviceSuggestedEvidencePaths,
  needsDeviceVerification,
  deliveryStatuses,
  releaseChannels,
  deviceVerificationStatuses,
  renderDeliveryStatusLabel,
  renderDeliveryChannelLabel,
  renderDeviceVerificationStatusLabel,
  recordDeliveryPending,
  onSubmitDelivery,
  onCancelDelivery,
  released,
  followUpWorkflowHint,
  showFollowUpForm,
  onToggleFollowUpForm,
  followUpForm,
  onFollowUpFieldChange,
  followUpStatuses,
  renderReleaseFollowUpStatusLabel,
  followUpSuggestedEvidencePaths,
  recordFollowUpPending,
  onSubmitFollowUp,
  onCancelFollowUp,
}: {
  t: Translate
  latestRunId?: string
  reviewBundleReady: boolean
  workflowHint: string
  checklistItems: ChecklistItem[]
  prepareReviewPending: boolean
  onPrepareReview: () => void
  showDeliveryForm: boolean
  onToggleDeliveryForm: () => void
  deliveryForm: DeliveryFormShape
  onDeliveryFieldChange: (field: keyof DeliveryFormShape, value: string) => void
  suggestedEvidencePaths: string[]
  deviceSuggestedEvidencePaths: string[]
  needsDeviceVerification: boolean
  deliveryStatuses: readonly string[]
  releaseChannels: readonly string[]
  deviceVerificationStatuses: readonly string[]
  renderDeliveryStatusLabel: (status: string) => string
  renderDeliveryChannelLabel: (channel: string) => string
  renderDeviceVerificationStatusLabel: (status: string) => string
  recordDeliveryPending: boolean
  onSubmitDelivery: () => void
  onCancelDelivery: () => void
  released: boolean
  followUpWorkflowHint: string
  showFollowUpForm: boolean
  onToggleFollowUpForm: () => void
  followUpForm: FollowUpFormShape
  onFollowUpFieldChange: (field: keyof FollowUpFormShape, value: string) => void
  followUpStatuses: readonly string[]
  renderReleaseFollowUpStatusLabel: (status: string) => string
  followUpSuggestedEvidencePaths: string[]
  recordFollowUpPending: boolean
  onSubmitFollowUp: () => void
  onCancelFollowUp: () => void
}) {
  return (
    <DetailSection title={t("jobs.delivery.title")}>
      <div className="grid gap-3 sm:grid-cols-2">
        <KeyValue label={t("jobs.delivery.latestRunId")} value={latestRunId} />
        <KeyValue
          label={t("jobs.delivery.reviewBundleReady")}
          value={reviewBundleReady ? t("jobs.detail.booleanTrue") : t("jobs.detail.booleanFalse")}
        />
      </div>
      <div className="rounded-lg border border-border/60 bg-muted/30 px-4 py-3 text-sm">
        <div className="font-medium">{t("jobs.delivery.workflowHint.title")}</div>
        <div className="text-muted-foreground mt-1">{workflowHint}</div>
      </div>
      <div className="space-y-2 pt-1">
        <div className="text-sm font-medium">{t("jobs.delivery.checklist.title")}</div>
        <div className="flex flex-wrap gap-2">
          {checklistItems.map((item) => (
            <span
              key={item.key}
              className={item.done ? "rounded-md bg-emerald-100 px-2 py-1 text-xs font-medium text-emerald-800" : "rounded-md bg-amber-100 px-2 py-1 text-xs font-medium text-amber-800"}
            >
              {t(`jobs.delivery.checklist.${item.key}`)}
            </span>
          ))}
        </div>
      </div>
      <div className="flex flex-wrap gap-2 pt-2">
        <Button
          size="sm"
          variant="outline"
          disabled={!latestRunId || prepareReviewPending}
          onClick={onPrepareReview}
        >
          {prepareReviewPending ? <IconLoader2 className="size-4 animate-spin" /> : null}
          {t("jobs.delivery.prepareReview")}
        </Button>
        <Button size="sm" variant="secondary" disabled={!latestRunId} onClick={onToggleDeliveryForm}>
          {showDeliveryForm ? t("jobs.delivery.hideForm") : t("jobs.delivery.recordDecision")}
        </Button>
      </div>

      {showDeliveryForm ? (
        <div className="grid gap-3 pt-3">
          <div className="grid gap-3 sm:grid-cols-2">
            <FieldBlock label={t("jobs.delivery.reviewerId")}>
              <Input
                value={deliveryForm.reviewerId}
                onChange={(event) => onDeliveryFieldChange("reviewerId", event.target.value)}
                placeholder={t("jobs.delivery.placeholders.reviewerId")}
              />
            </FieldBlock>
            <FieldBlock label={t("jobs.delivery.statusLabel")}>
              <Select
                value={deliveryForm.status}
                onValueChange={(value) => onDeliveryFieldChange("status", value)}
              >
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {deliveryStatuses.map((status) => (
                    <SelectItem key={status} value={status}>
                      {renderDeliveryStatusLabel(status)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </FieldBlock>
            <FieldBlock label={t("jobs.detail.releaseChannel")}>
              <Select
                value={deliveryForm.releaseChannel}
                onValueChange={(value) => onDeliveryFieldChange("releaseChannel", value)}
              >
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {releaseChannels.map((channel) => (
                    <SelectItem key={channel || "none"} value={channel}>
                      {renderDeliveryChannelLabel(channel)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </FieldBlock>
            <FieldBlock label={t("jobs.detail.rolloutPercent")}>
              <Input
                type="number"
                min={0}
                max={100}
                value={deliveryForm.rolloutPercent}
                onChange={(event) => onDeliveryFieldChange("rolloutPercent", event.target.value)}
              />
            </FieldBlock>
          </div>
          <FieldBlock label={t("jobs.detail.deliverySummary")}>
            <Textarea
              value={deliveryForm.summary}
              onChange={(event) => onDeliveryFieldChange("summary", event.target.value)}
              rows={3}
              placeholder={t("jobs.delivery.placeholders.summary")}
            />
          </FieldBlock>
          <FieldBlock label={t("jobs.detail.evidencePaths")}>
            {suggestedEvidencePaths.length > 0 ? (
              <div className="mb-2 flex flex-wrap gap-2">
                {suggestedEvidencePaths.map((path) => (
                  <Button
                    key={path}
                    type="button"
                    size="sm"
                    variant="outline"
                    className="h-7 px-2 text-xs"
                    onClick={() => onDeliveryFieldChange("evidencePaths", path)}
                  >
                    {t("jobs.delivery.suggestEvidence", { path })}
                  </Button>
                ))}
              </div>
            ) : null}
            <Textarea
              value={deliveryForm.evidencePaths}
              onChange={(event) => onDeliveryFieldChange("evidencePaths", event.target.value)}
              rows={3}
              placeholder={t("jobs.delivery.placeholders.multiline")}
            />
          </FieldBlock>
          <FieldBlock label={t("jobs.detail.requiredChanges")}>
            <Textarea
              value={deliveryForm.requiredChanges}
              onChange={(event) => onDeliveryFieldChange("requiredChanges", event.target.value)}
              rows={3}
              placeholder={t("jobs.delivery.placeholders.multiline")}
            />
          </FieldBlock>
          <FieldBlock label={t("jobs.detail.signedArtifactPaths")}>
            <Textarea
              value={deliveryForm.signedArtifactPaths}
              onChange={(event) => onDeliveryFieldChange("signedArtifactPaths", event.target.value)}
              rows={3}
              placeholder={t("jobs.delivery.placeholders.multiline")}
            />
          </FieldBlock>
          {needsDeviceVerification ? (
            <div className="rounded-lg border border-border/60 bg-muted/20 p-3">
              <div className="text-sm font-medium">{t("jobs.delivery.deviceVerificationTitle")}</div>
              <div className="mt-3 grid gap-3 sm:grid-cols-2">
                <FieldBlock label={t("jobs.delivery.deviceVerificationStatusLabel")}>
                  <Select
                    value={deliveryForm.deviceVerificationStatus}
                    onValueChange={(value) => onDeliveryFieldChange("deviceVerificationStatus", value)}
                  >
                    <SelectTrigger className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {deviceVerificationStatuses.map((status) => (
                        <SelectItem key={status} value={status}>
                          {renderDeviceVerificationStatusLabel(status)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </FieldBlock>
                <FieldBlock label={t("jobs.delivery.deviceVerificationSummaryLabel")}>
                  <Input
                    value={deliveryForm.deviceVerificationSummary}
                    onChange={(event) => onDeliveryFieldChange("deviceVerificationSummary", event.target.value)}
                    placeholder={t("jobs.delivery.placeholders.deviceVerificationSummary")}
                  />
                </FieldBlock>
              </div>
              <FieldBlock label={t("jobs.delivery.deviceVerificationEvidencePathsLabel")}>
                {deviceSuggestedEvidencePaths.length > 0 ? (
                  <div className="mb-2 flex flex-wrap gap-2">
                    {deviceSuggestedEvidencePaths.map((path) => (
                      <Button
                        key={path}
                        type="button"
                        size="sm"
                        variant="outline"
                        className="h-7 px-2 text-xs"
                        onClick={() => onDeliveryFieldChange("deviceVerificationEvidencePaths", path)}
                      >
                        {t("jobs.delivery.suggestEvidence", { path })}
                      </Button>
                    ))}
                  </div>
                ) : null}
                <Textarea
                  value={deliveryForm.deviceVerificationEvidencePaths}
                  onChange={(event) => onDeliveryFieldChange("deviceVerificationEvidencePaths", event.target.value)}
                  rows={3}
                  placeholder={t("jobs.delivery.placeholders.multiline")}
                />
              </FieldBlock>
            </div>
          ) : null}
          <div className="flex flex-wrap gap-2">
            <Button size="sm" disabled={!latestRunId || recordDeliveryPending} onClick={onSubmitDelivery}>
              {recordDeliveryPending ? <IconLoader2 className="size-4 animate-spin" /> : null}
              {t("jobs.delivery.submitRecord")}
            </Button>
            <Button size="sm" variant="outline" disabled={recordDeliveryPending} onClick={onCancelDelivery}>
              {t("jobs.delivery.cancelEdit")}
            </Button>
          </div>
        </div>
      ) : null}

      {released ? (
        <div id="jobs-delivery-follow-up" className="rounded-lg border border-border/60 bg-muted/10 p-4">
          <div className="text-sm font-medium">{t("jobs.delivery.followUpTitle")}</div>
          <div className="text-muted-foreground mt-1 text-sm">{followUpWorkflowHint}</div>
          <div className="flex flex-wrap gap-2 pt-3">
            <Button size="sm" variant="secondary" disabled={!latestRunId} onClick={onToggleFollowUpForm}>
              {showFollowUpForm ? t("jobs.delivery.hideFollowUpForm") : t("jobs.delivery.recordFollowUp")}
            </Button>
          </div>
          {showFollowUpForm ? (
            <div className="grid gap-3 pt-3">
              <div className="grid gap-3 sm:grid-cols-2">
                <FieldBlock label={t("jobs.delivery.followUpOwnerId")}>
                  <Input
                    value={followUpForm.ownerId}
                    onChange={(event) => onFollowUpFieldChange("ownerId", event.target.value)}
                    placeholder={t("jobs.delivery.placeholders.followUpOwnerId")}
                  />
                </FieldBlock>
                <FieldBlock label={t("jobs.delivery.followUpStatusLabel")}>
                  <Select
                    value={followUpForm.status}
                    onValueChange={(value) => onFollowUpFieldChange("status", value)}
                  >
                    <SelectTrigger className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {followUpStatuses.map((status) => (
                        <SelectItem key={status} value={status}>
                          {renderReleaseFollowUpStatusLabel(status)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </FieldBlock>
              </div>
              <FieldBlock label={t("jobs.delivery.followUpSummaryLabel")}>
                <Textarea
                  value={followUpForm.summary}
                  onChange={(event) => onFollowUpFieldChange("summary", event.target.value)}
                  rows={3}
                  placeholder={t("jobs.delivery.placeholders.followUpSummary")}
                />
              </FieldBlock>
              <FieldBlock label={t("jobs.delivery.followUpEvidencePathsLabel")}>
                {followUpSuggestedEvidencePaths.length > 0 ? (
                  <div className="mb-2 flex flex-wrap gap-2">
                    {followUpSuggestedEvidencePaths.map((path) => (
                      <Button
                        key={path}
                        type="button"
                        size="sm"
                        variant="outline"
                        className="h-7 px-2 text-xs"
                        onClick={() => onFollowUpFieldChange("evidencePaths", path)}
                      >
                        {t("jobs.delivery.suggestEvidence", { path })}
                      </Button>
                    ))}
                  </div>
                ) : null}
                <Textarea
                  value={followUpForm.evidencePaths}
                  onChange={(event) => onFollowUpFieldChange("evidencePaths", event.target.value)}
                  rows={3}
                  placeholder={t("jobs.delivery.placeholders.multiline")}
                />
              </FieldBlock>
              <div className="flex flex-wrap gap-2">
                <Button size="sm" disabled={!latestRunId || recordFollowUpPending} onClick={onSubmitFollowUp}>
                  {recordFollowUpPending ? <IconLoader2 className="size-4 animate-spin" /> : null}
                  {t("jobs.delivery.submitFollowUp")}
                </Button>
                <Button size="sm" variant="outline" disabled={recordFollowUpPending} onClick={onCancelFollowUp}>
                  {t("jobs.delivery.cancelEdit")}
                </Button>
              </div>
            </div>
          ) : null}
        </div>
      ) : null}
    </DetailSection>
  )
}