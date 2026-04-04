import { useMutation } from "@tanstack/react-query"
import { toast } from "sonner"

import {
  prepareReview,
  recordDelivery,
  recordDeliveryFollowUp,
  type ArtifactItem,
  type PublicJobRecord,
  type RecordDeliveryFollowUpPayload,
  type RecordDeliveryPayload,
} from "@/api/system"
import {
  buildDeliveryFormState,
  buildFollowUpFormState,
  DELIVERY_CHANNEL_NONE,
  DEVICE_VERIFICATION_NONE,
  getDeliveryValidationMessage,
  getFollowUpValidationMessage,
  needsDeviceVerification,
  parseRolloutPercent,
  splitMultilineField,
  type DeliveryFormState,
  type FollowUpFormState,
} from "@/components/jobs/jobs-delivery-utils"
import { getErrorMessage } from "@/components/jobs/jobs-page-utils"
import type { JobsWorkspaceController } from "@/components/jobs/jobs-workspace-controller"

type Translate = (key: string, options?: Record<string, unknown>) => string

export function useJobsDeliveryFlow({
  t,
  selectedJob,
  latestRunId,
  artifactItems,
  deliveryForm,
  followUpForm,
  refreshAll,
  workspaceController,
  setShowDeliveryForm,
  setShowFollowUpForm,
  setDeliveryForm,
  setFollowUpForm,
}: {
  t: Translate
  selectedJob?: PublicJobRecord
  latestRunId?: string
  artifactItems: ArtifactItem[]
  deliveryForm: DeliveryFormState
  followUpForm: FollowUpFormState
  refreshAll: () => Promise<void>
  workspaceController: JobsWorkspaceController
  setShowDeliveryForm: (show: boolean) => void
  setShowFollowUpForm: (show: boolean) => void
  setDeliveryForm: (form: DeliveryFormState) => void
  setFollowUpForm: (form: FollowUpFormState) => void
}) {
  const prepareReviewMutation = useMutation({
    mutationFn: (runId: string) => prepareReview(runId),
    onSuccess: async () => {
      toast.success(t("jobs.toasts.reviewPrepared"))
      await refreshAll()
      workspaceController.focusTarget("jobs-artifacts")
    },
    onError: (error) => {
      toast.error(getErrorMessage(error, t("jobs.toasts.actionFailed")))
    },
  })

  const recordDeliveryMutation = useMutation({
    mutationFn: (payload: RecordDeliveryPayload) => recordDelivery(payload),
    onSuccess: async (_result, variables) => {
      toast.success(
        t("jobs.toasts.actionCompleted", {
          action: variables.next_action,
        }),
      )
      setShowDeliveryForm(false)
      await refreshAll()
      workspaceController.focusTarget("jobs-delivery-context")
    },
    onError: (error) => {
      toast.error(getErrorMessage(error, t("jobs.toasts.actionFailed")))
    },
  })

  const recordFollowUpMutation = useMutation({
    mutationFn: (payload: RecordDeliveryFollowUpPayload) => recordDeliveryFollowUp(payload),
    onSuccess: async () => {
      toast.success(t("jobs.toasts.followUpRecorded"))
      setShowFollowUpForm(false)
      await refreshAll()
      workspaceController.focusTarget("jobs-delivery-follow-up")
    },
    onError: (error) => {
      toast.error(getErrorMessage(error, t("jobs.toasts.actionFailed")))
    },
  })

  const handleDeliveryFormSubmit = () => {
    if (!selectedJob || !latestRunId) {
      return
    }

    const validationMessage = getDeliveryValidationMessage(deliveryForm, t)
    if (validationMessage) {
      toast.error(validationMessage)
      return
    }

    const rolloutPercent = parseRolloutPercent(deliveryForm.rolloutPercent) ?? 0
    const payload: RecordDeliveryPayload = {
      run_id: latestRunId,
      reviewer_id: deliveryForm.reviewerId.trim(),
      status: deliveryForm.status,
      summary: deliveryForm.summary.trim(),
      release_channel:
        deliveryForm.releaseChannel === DELIVERY_CHANNEL_NONE ? "" : deliveryForm.releaseChannel,
      rollout_percent: rolloutPercent,
      evidence_paths: splitMultilineField(deliveryForm.evidencePaths),
      required_changes: splitMultilineField(deliveryForm.requiredChanges),
      signed_artifact_paths: splitMultilineField(deliveryForm.signedArtifactPaths),
      device_verification_status:
        needsDeviceVerification(deliveryForm.status) &&
        deliveryForm.deviceVerificationStatus !== DEVICE_VERIFICATION_NONE
          ? deliveryForm.deviceVerificationStatus
          : undefined,
      device_verification_summary:
        needsDeviceVerification(deliveryForm.status) &&
        deliveryForm.deviceVerificationStatus !== DEVICE_VERIFICATION_NONE
          ? deliveryForm.deviceVerificationSummary.trim()
          : undefined,
      device_verification_evidence_paths:
        needsDeviceVerification(deliveryForm.status) &&
        deliveryForm.deviceVerificationStatus !== DEVICE_VERIFICATION_NONE
          ? splitMultilineField(deliveryForm.deviceVerificationEvidencePaths)
          : undefined,
      next_action: deliveryForm.status,
    }
    recordDeliveryMutation.mutate(payload)
  }

  const handleFollowUpFormSubmit = () => {
    if (!selectedJob || !latestRunId) {
      return
    }

    const validationMessage = getFollowUpValidationMessage(followUpForm, t)
    if (validationMessage) {
      toast.error(validationMessage)
      return
    }

    recordFollowUpMutation.mutate({
      run_id: latestRunId,
      owner_id: followUpForm.ownerId.trim(),
      status: followUpForm.status,
      summary: followUpForm.summary.trim(),
      evidence_paths: splitMultilineField(followUpForm.evidencePaths),
    })
  }

  const handleCancelDelivery = () => {
    if (!selectedJob) {
      return
    }
    setDeliveryForm(buildDeliveryFormState(selectedJob.delivery_context, artifactItems))
    setShowDeliveryForm(false)
  }

  const handleCancelFollowUp = () => {
    if (!selectedJob) {
      return
    }
    setFollowUpForm(buildFollowUpFormState(selectedJob.delivery_context, artifactItems))
    setShowFollowUpForm(false)
  }

  return {
    prepareReviewMutation,
    recordDeliveryMutation,
    recordFollowUpMutation,
    handleDeliveryFormSubmit,
    handleFollowUpFormSubmit,
    handleCancelDelivery,
    handleCancelFollowUp,
  }
}