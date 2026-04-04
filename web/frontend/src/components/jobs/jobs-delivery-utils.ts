import type { ArtifactItem, PublicJobDeliveryContext, PublicJobEvent } from "@/api/system"
import { collectProducedArtifactPaths } from "@/components/jobs/jobs-state-model"

type Translate = (key: string, options?: Record<string, unknown>) => string

export const DELIVERY_CHANNEL_NONE = "__none__"
export const DEVICE_VERIFICATION_NONE = "__none__"
export const DELIVERY_STATUSES = [
  "changes_requested",
  "approved_for_signing",
  "signed",
  "staged",
  "released",
] as const
export const DEVICE_VERIFICATION_STATUSES = [
  DEVICE_VERIFICATION_NONE,
  "pending",
  "passed",
  "failed",
] as const
export const DELIVERY_RELEASE_CHANNELS = [
  DELIVERY_CHANNEL_NONE,
  "internal",
  "canary",
  "production",
] as const
export const RELEASE_FOLLOW_UP_STATUSES = ["monitoring", "stable", "issue_detected"] as const

export type DeliveryStatus = (typeof DELIVERY_STATUSES)[number]
export type DeviceVerificationStatus = (typeof DEVICE_VERIFICATION_STATUSES)[number]
export type DeliveryReleaseChannel = (typeof DELIVERY_RELEASE_CHANNELS)[number]
export type ReleaseFollowUpStatus = (typeof RELEASE_FOLLOW_UP_STATUSES)[number]

export type DeliveryFormState = {
  reviewerId: string
  status: DeliveryStatus
  summary: string
  releaseChannel: DeliveryReleaseChannel
  rolloutPercent: string
  evidencePaths: string
  requiredChanges: string
  signedArtifactPaths: string
  deviceVerificationStatus: DeviceVerificationStatus
  deviceVerificationSummary: string
  deviceVerificationEvidencePaths: string
}

export type FollowUpFormState = {
  ownerId: string
  status: ReleaseFollowUpStatus
  summary: string
  evidencePaths: string
}

export type DeliveryChecklistItem = {
  key: string
  done: boolean
}

export function createDefaultDeliveryForm(): DeliveryFormState {
  return {
    reviewerId: "",
    status: "staged",
    summary: "",
    releaseChannel: DELIVERY_CHANNEL_NONE,
    rolloutPercent: "0",
    evidencePaths: "",
    requiredChanges: "",
    signedArtifactPaths: "",
    deviceVerificationStatus: DEVICE_VERIFICATION_NONE,
    deviceVerificationSummary: "",
    deviceVerificationEvidencePaths: "",
  }
}

export function createDefaultFollowUpForm(): FollowUpFormState {
  return {
    ownerId: "",
    status: "monitoring",
    summary: "",
    evidencePaths: "",
  }
}

export function buildDeliveryFormState(
  context: PublicJobDeliveryContext | undefined,
  artifacts: ArtifactItem[],
): DeliveryFormState {
  const suggestedEvidence = collectSuggestedEvidencePaths(artifacts)
  const deviceEvidence = collectDeviceEvidencePaths(artifacts)
  return {
    reviewerId: context?.reviewer_id ?? "",
    status: isDeliveryStatus(context?.status) ? context.status : "staged",
    summary: context?.summary ?? "",
    releaseChannel: isDeliveryReleaseChannel(context?.release_channel)
      ? context.release_channel
      : DELIVERY_CHANNEL_NONE,
    rolloutPercent: String(context?.rollout_percent ?? 0),
    evidencePaths: (context?.evidence_paths ?? suggestedEvidence).join("\n"),
    requiredChanges: (context?.required_changes ?? []).join("\n"),
    signedArtifactPaths: (context?.signed_artifact_paths ?? []).join("\n"),
    deviceVerificationStatus: isDeviceVerificationStatus(context?.device_verification?.status)
      ? context.device_verification.status
      : DEVICE_VERIFICATION_NONE,
    deviceVerificationSummary: context?.device_verification?.summary ?? "",
    deviceVerificationEvidencePaths: (
      context?.device_verification?.evidence_paths ?? deviceEvidence
    ).join("\n"),
  }
}

export function buildFollowUpFormState(
  context: PublicJobDeliveryContext | undefined,
  artifacts: ArtifactItem[],
): FollowUpFormState {
  const suggestedEvidence = collectFollowUpEvidencePaths(artifacts)
  return {
    ownerId: context?.release_follow_up?.owner_id ?? "",
    status: isReleaseFollowUpStatus(context?.release_follow_up?.status)
      ? context.release_follow_up.status
      : "monitoring",
    summary: context?.release_follow_up?.summary ?? "",
    evidencePaths: (context?.release_follow_up?.evidence_paths ?? suggestedEvidence).join("\n"),
  }
}

export function getLatestRunId(events: PublicJobEvent[]) {
  for (let index = events.length - 1; index >= 0; index -= 1) {
    const runId = events[index]?.run_id?.trim()
    if (runId) {
      return runId
    }
  }
  return ""
}

export function collectSuggestedEvidencePaths(artifacts: ArtifactItem[]) {
  return collectProducedArtifactPaths(artifacts, [
    "review-bundle",
    "handoff-checklist",
    "smoke-test-report",
    "build-report",
  ])
}

export function collectDeviceEvidencePaths(artifacts: ArtifactItem[]) {
  return collectProducedArtifactPaths(artifacts, [
    "device-logcat",
    "device-screenshot",
    "smoke-test-report",
    "build-report",
    "app-debug.apk",
  ])
}

export function collectFollowUpEvidencePaths(artifacts: ArtifactItem[]) {
  return collectProducedArtifactPaths(artifacts, [
    "device-logcat",
    "device-screenshot",
    "smoke-test-report",
    "build-report",
    "app-debug.apk",
  ])
}

export function parseRolloutPercent(value: string) {
  if (value.trim() === "") {
    return 0
  }
  const parsed = Number(value)
  if (!Number.isInteger(parsed) || parsed < 0 || parsed > 100) {
    return null
  }
  return parsed
}

export function splitMultilineField(value: string) {
  return value
    .split("\n")
    .map((item) => item.trim())
    .filter((item) => item !== "")
}

export function appendUniqueMultilineItem(value: string, item: string) {
  const items = splitMultilineField(value)
  if (items.includes(item)) {
    return items.join("\n")
  }
  return [...items, item].join("\n")
}

export function hasArtifact(items: ArtifactItem[], artifactId: string) {
  return items.some((item) => item.artifact_id === artifactId && item.produced)
}

export function buildDeliveryChecklistItems(
  form: DeliveryFormState,
  artifacts: ArtifactItem[],
): DeliveryChecklistItem[] {
  const items: DeliveryChecklistItem[] = [
    { key: "reviewBundle", done: hasArtifact(artifacts, "review-bundle") },
    { key: "handoffChecklist", done: hasArtifact(artifacts, "handoff-checklist") },
    { key: "reviewerAssigned", done: form.reviewerId.trim() !== "" },
    { key: "evidenceAttached", done: splitMultilineField(form.evidencePaths).length > 0 },
  ]

  if (form.status === "changes_requested") {
    items.push({
      key: "changesDocumented",
      done: splitMultilineField(form.requiredChanges).length > 0,
    })
  }
  if (["signed", "staged", "released"].includes(form.status)) {
    items.push({
      key: "signedArtifactsAttached",
      done: splitMultilineField(form.signedArtifactPaths).length > 0,
    })
  }
  if (["staged", "released"].includes(form.status)) {
    items.push({
      key: "releaseChannelSelected",
      done: form.releaseChannel !== DELIVERY_CHANNEL_NONE,
    })
  }
  if (form.status === "staged") {
    items.push({
      key: "rolloutDefined",
      done: parseRolloutPercent(form.rolloutPercent) !== null,
    })
  }
  if (needsDeviceVerification(form.status)) {
    items.push({
      key: "deviceVerificationRecorded",
      done: form.deviceVerificationStatus !== DEVICE_VERIFICATION_NONE,
    })
  }

  return items
}

export function getDeliveryValidationMessage(form: DeliveryFormState, t: Translate) {
  if (form.reviewerId.trim() === "") {
    return t("jobs.delivery.validation.reviewerId")
  }
  if (parseRolloutPercent(form.rolloutPercent) === null) {
    return t("jobs.delivery.validation.rolloutPercent")
  }
  if (splitMultilineField(form.evidencePaths).length === 0) {
    return t("jobs.delivery.validation.evidencePaths")
  }
  if (form.status === "changes_requested" && splitMultilineField(form.requiredChanges).length === 0) {
    return t("jobs.delivery.validation.requiredChanges")
  }
  if (["signed", "staged", "released"].includes(form.status) && splitMultilineField(form.signedArtifactPaths).length === 0) {
    return t("jobs.delivery.validation.signedArtifactPaths")
  }
  if (["staged", "released"].includes(form.status) && form.releaseChannel === DELIVERY_CHANNEL_NONE) {
    return t("jobs.delivery.validation.releaseChannel")
  }
  if (needsDeviceVerification(form.status) && form.deviceVerificationStatus === DEVICE_VERIFICATION_NONE) {
    return t("jobs.delivery.validation.deviceVerificationStatus")
  }
  if (
    needsDeviceVerification(form.status) &&
    ["passed", "failed"].includes(form.deviceVerificationStatus) &&
    splitMultilineField(form.deviceVerificationEvidencePaths).length === 0
  ) {
    return t("jobs.delivery.validation.deviceVerificationEvidencePaths")
  }
  return ""
}

export function getFollowUpValidationMessage(form: FollowUpFormState, t: Translate) {
  if (form.ownerId.trim() === "") {
    return t("jobs.delivery.validation.followUpOwnerId")
  }
  if (splitMultilineField(form.evidencePaths).length === 0) {
    return t("jobs.delivery.validation.followUpEvidencePaths")
  }
  return ""
}

export function renderDeliveryWorkflowHint(status: DeliveryStatus, t: Translate) {
  return t(`jobs.delivery.workflowHint.${status}`)
}

export function renderReleaseFollowUpWorkflowHint(status: string | undefined, t: Translate) {
  const normalized = isReleaseFollowUpStatus(status) ? status : "monitoring"
  return t(`jobs.delivery.followUpHint.${normalized}`)
}

export function isDeliveryStatus(value: string | undefined): value is DeliveryStatus {
  return typeof value === "string" && DELIVERY_STATUSES.includes(value as DeliveryStatus)
}

export function isDeliveryReleaseChannel(value: string | undefined): value is DeliveryReleaseChannel {
  return typeof value === "string" && DELIVERY_RELEASE_CHANNELS.includes(value as DeliveryReleaseChannel)
}

export function isDeviceVerificationStatus(
  value: string | undefined,
): value is Exclude<DeviceVerificationStatus, typeof DEVICE_VERIFICATION_NONE> {
  return (
    typeof value === "string" &&
    DEVICE_VERIFICATION_STATUSES.includes(value as DeviceVerificationStatus) &&
    value !== DEVICE_VERIFICATION_NONE
  )
}

export function isReleaseFollowUpStatus(value: string | undefined): value is ReleaseFollowUpStatus {
  return typeof value === "string" && RELEASE_FOLLOW_UP_STATUSES.includes(value as ReleaseFollowUpStatus)
}

export function needsDeviceVerification(status: DeliveryStatus) {
  return status === "staged" || status === "released"
}