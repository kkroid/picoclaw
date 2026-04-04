import type { ArtifactItem, PublicJobEvent, PublicJobRecord, PublicNotification } from "@/api/system"
import { JobAdvancedInfoPanel } from "@/components/jobs/jobs-advanced-info-panel"
import { getPendingHumanApprovalCount, isExecutableNotificationAction, isExecutableStatusAction, renderDeliveryChannelLabel, renderDeliveryStatusLabel, renderDeviceVerificationStatusLabel, renderFailureCategoryLabel, renderFailureDomainLabel, renderFailureGuidanceActionLabel, renderReleaseFollowUpStatusLabel, renderRolloutPercent, renderStatusContextAction, renderStatusContextReason, renderSuggestedActionLabel, translateStatusAction, type ExecutableStatusAction } from "@/components/jobs/jobs-action-utils"
import { JobsArtifactsPanel } from "@/components/jobs/jobs-artifacts-panel"
import { JobsDeliveryContextPanel } from "@/components/jobs/jobs-delivery-context-panel"
import { JobsDeliveryFormPanel } from "@/components/jobs/jobs-delivery-form-panel"
import { DELIVERY_RELEASE_CHANNELS, DELIVERY_STATUSES, DEVICE_VERIFICATION_STATUSES, RELEASE_FOLLOW_UP_STATUSES, appendUniqueMultilineItem, hasArtifact, needsDeviceVerification, renderDeliveryWorkflowHint, renderReleaseFollowUpWorkflowHint, type DeliveryFormState, type FollowUpFormState } from "@/components/jobs/jobs-delivery-utils"
import { JobsExecutionPanel } from "@/components/jobs/jobs-execution-panel"
import { JobFailureDiagnosisPanel } from "@/components/jobs/jobs-failure-diagnosis-panel"
import { JobOverviewActionPanel } from "@/components/jobs/jobs-overview-action-panel"
import { runDeliverySuggestedAction } from "@/components/jobs/jobs-page-actions"
import type { JobProgressSummary } from "@/components/jobs/jobs-page-state"
import type { FailureGuidanceCard, FailureDiagnosisSummary, JobExecutionCheckpoint, JobExecutionCheckpointStateSummary, JobExecutionEventStreamSummary, JobExecutionStageSummary } from "@/components/jobs/jobs-state-model"
import { JobLiveProgressPanel } from "@/components/jobs/jobs-live-progress-panel"
import { JobResumeContextPanel } from "@/components/jobs/jobs-resume-context-panel"
import { JobsStatusBadge } from "@/components/jobs/jobs-status-badge"
import { JobStatusContextPanel } from "@/components/jobs/jobs-status-context-panel"
import { getErrorMessage } from "@/components/jobs/jobs-page-utils"
import type { JobWorkspaceTab, JobsWorkspaceController } from "@/components/jobs/jobs-workspace-controller"

type Translate = (key: string, options?: Record<string, unknown>) => string

export function JobsDetailContent({
  t,
  detailTab,
  selectedJob,
  artifactItems,
  jobEvents,
  latestRunId,
  latestJobEvent,
  selectedJobProgress,
  selectedJobExecutionStage,
  selectedJobExecutionBreakdown,
  selectedJobExecutionStateSummary,
  selectedJobExecutionEventStream,
  selectedJobNotifications,
  failureGuidance,
  failureDiagnosisSummary,
  notificationsLoading,
  notificationsError,
  eventsLoading,
  eventsError,
  artifactsLoading,
  artifactsError,
  statusActionPending,
  statusActionJobId,
  onRunStatusAction,
  onRunNotificationAction,
  onAcknowledge,
  acknowledgePending,
  acknowledgePendingId,
  formatDate,
  workspaceController,
  showDeliveryForm,
  setShowDeliveryForm,
  showFollowUpForm,
  setShowFollowUpForm,
  deliveryForm,
  setDeliveryForm,
  followUpForm,
  setFollowUpForm,
  suggestedEvidencePaths,
  deviceSuggestedEvidencePaths,
  followUpSuggestedEvidencePaths,
  deliveryChecklistItems,
  prepareReviewPending,
  onPrepareReview,
  recordDeliveryPending,
  onSubmitDelivery,
  onCancelDelivery,
  recordFollowUpPending,
  onSubmitFollowUp,
  onCancelFollowUp,
}: {
  t: Translate
  detailTab: JobWorkspaceTab
  selectedJob: PublicJobRecord
  artifactItems: ArtifactItem[]
  jobEvents: PublicJobEvent[]
  latestRunId?: string
  latestJobEvent?: PublicJobEvent
  selectedJobProgress?: JobProgressSummary
  selectedJobExecutionStage?: JobExecutionStageSummary
  selectedJobExecutionBreakdown: JobExecutionCheckpoint[]
  selectedJobExecutionStateSummary: JobExecutionCheckpointStateSummary
  selectedJobExecutionEventStream: JobExecutionEventStreamSummary
  selectedJobNotifications: PublicNotification[]
  failureGuidance: FailureGuidanceCard[]
  failureDiagnosisSummary?: FailureDiagnosisSummary
  notificationsLoading: boolean
  notificationsError?: unknown
  eventsLoading: boolean
  eventsError?: unknown
  artifactsLoading: boolean
  artifactsError?: unknown
  statusActionPending: boolean
  statusActionJobId?: string
  onRunStatusAction: (job: PublicJobRecord, action: ExecutableStatusAction) => void
  onRunNotificationAction: (item: PublicNotification) => void
  onAcknowledge: (notificationId: string) => void
  acknowledgePending: boolean
  acknowledgePendingId?: string
  formatDate: (value?: string) => string
  workspaceController: JobsWorkspaceController
  showDeliveryForm: boolean
  setShowDeliveryForm: (value: boolean | ((current: boolean) => boolean)) => void
  showFollowUpForm: boolean
  setShowFollowUpForm: (value: boolean | ((current: boolean) => boolean)) => void
  deliveryForm: DeliveryFormState
  setDeliveryForm: (value: DeliveryFormState | ((current: DeliveryFormState) => DeliveryFormState)) => void
  followUpForm: FollowUpFormState
  setFollowUpForm: (value: FollowUpFormState | ((current: FollowUpFormState) => FollowUpFormState)) => void
  suggestedEvidencePaths: string[]
  deviceSuggestedEvidencePaths: string[]
  followUpSuggestedEvidencePaths: string[]
  deliveryChecklistItems: Array<{ key: string; done: boolean }>
  prepareReviewPending: boolean
  onPrepareReview: () => void
  recordDeliveryPending: boolean
  onSubmitDelivery: () => void
  onCancelDelivery: () => void
  recordFollowUpPending: boolean
  onSubmitFollowUp: () => void
  onCancelFollowUp: () => void
}) {
  return (
    <>
      {detailTab === "overview" ? (
        <JobOverviewActionPanel
          t={t}
          job={selectedJob}
          loading={notificationsLoading}
          errorMessage={
            notificationsError
              ? getErrorMessage(notificationsError, t("jobs.states.loadErrorNotifications"))
              : undefined
          }
          selectedJobNotifications={selectedJobNotifications}
          failureGuidance={failureGuidance}
          statusActionTitle={
            selectedJob.status_context
              ? renderStatusContextAction(selectedJob.status_context, t)
              : t("jobs.detail.none")
          }
          statusActionSummary={
            selectedJob.status_context?.summary ||
            (selectedJob.status_context
              ? renderStatusContextReason(selectedJob.status_context, t)
              : t("jobs.detail.none"))
          }
          statusActionLabel={
            selectedJob.status_context?.suggested_action
              ? translateStatusAction(selectedJob.status_context.suggested_action, t)
              : t("jobs.detail.none")
          }
          statusActionExecutable={Boolean(
            selectedJob.status_context &&
              isExecutableStatusAction(selectedJob.status_context.suggested_action),
          )}
          statusActionPending={statusActionPending && statusActionJobId === selectedJob.job_id}
          onRunStatusAction={() =>
            onRunStatusAction(
              selectedJob,
              selectedJob.status_context!.suggested_action as ExecutableStatusAction,
            )
          }
          onRunNotificationAction={onRunNotificationAction}
          onAcknowledge={onAcknowledge}
          acknowledgePending={acknowledgePending}
          acknowledgePendingId={acknowledgePendingId}
          formatDate={formatDate}
          renderStatusBadge={(status) => <JobsStatusBadge status={status} t={t} />}
          renderFailureGuidanceActionLabel={(action) => renderFailureGuidanceActionLabel(action, t)}
          onFocusTarget={workspaceController.focusTarget}
        />
      ) : null}

      {detailTab === "overview" && selectedJobProgress ? (
        <JobLiveProgressPanel
          t={t}
          progress={selectedJobProgress}
          executionStage={selectedJobExecutionStage}
          latestSignalAt={formatDate(latestJobEvent?.at ?? selectedJob.updated_at)}
          latestSignal={latestJobEvent?.summary || latestJobEvent?.type || t("jobs.detail.none")}
          nextActionLabel={renderSuggestedActionLabel(
            selectedJob.status_context?.suggested_action,
            t,
          )}
          currentRound={selectedJob.current_round}
        />
      ) : null}

      {detailTab === "execution" ? (
        <JobsExecutionPanel
          t={t}
          loading={eventsLoading}
          errorMessage={eventsError ? getErrorMessage(eventsError, t("jobs.states.loadErrorEvents")) : undefined}
          hasBreakdown={selectedJobExecutionBreakdown.length > 0}
          currentStage={selectedJobExecutionStage?.stageLabel}
          currentStep={selectedJobExecutionStage?.stepLabel}
          latestSignalAt={formatDate(latestJobEvent?.at ?? selectedJob.updated_at)}
          stageDetail={selectedJobExecutionStage?.detail}
          currentRound={selectedJob.current_round}
          eventCount={selectedJobExecutionEventStream.totalCount}
          stateSummary={selectedJobExecutionStateSummary}
          checkpoints={selectedJobExecutionBreakdown}
          recentEvents={selectedJobExecutionEventStream.recentItems}
          totalEventCount={selectedJobExecutionEventStream.totalCount}
          allEvents={jobEvents}
          formatDate={formatDate}
          renderDeliveryStatusLabel={(status) => renderDeliveryStatusLabel(status, t)}
          renderDeviceVerificationStatusLabel={(status) => renderDeviceVerificationStatusLabel(status, t)}
          renderReleaseFollowUpStatusLabel={(status) => renderReleaseFollowUpStatusLabel(status, t)}
          renderFailureDomainLabel={(domain) => renderFailureDomainLabel(domain, t)}
          renderFailureCategoryLabel={(category) => renderFailureCategoryLabel(category, t)}
        />
      ) : null}

      {detailTab === "artifacts" ? (
        <JobsArtifactsPanel
          t={t}
          job={selectedJob}
          artifactItems={artifactItems}
          loading={artifactsLoading}
          errorMessage={
            artifactsError ? getErrorMessage(artifactsError, t("jobs.states.loadErrorArtifacts")) : undefined
          }
        />
      ) : null}

      {detailTab === "overview" ? (
        <JobAdvancedInfoPanel t={t} job={selectedJob} formatDate={formatDate} />
      ) : null}

      {detailTab === "overview" && selectedJob.status_context ? (
        <JobStatusContextPanel
          t={t}
          job={selectedJob}
          statusReason={renderStatusContextReason(selectedJob.status_context, t)}
          suggestedAction={renderStatusContextAction(selectedJob.status_context, t)}
          executable={isExecutableStatusAction(selectedJob.status_context.suggested_action)}
          actionLabel={
            selectedJob.status_context.suggested_action
              ? translateStatusAction(selectedJob.status_context.suggested_action, t)
              : t("jobs.detail.none")
          }
          pending={statusActionPending && statusActionJobId === selectedJob.job_id}
          onRunSuggestedAction={() =>
            onRunStatusAction(
              selectedJob,
              selectedJob.status_context!.suggested_action as ExecutableStatusAction,
            )
          }
        />
      ) : null}

      {detailTab === "overview" && selectedJob.failure_context ? (
        <JobFailureDiagnosisPanel
          t={t}
          job={selectedJob}
          failureDiagnosisSummary={failureDiagnosisSummary}
          failureGuidance={failureGuidance}
          renderFailureDomainLabel={(domain) => renderFailureDomainLabel(domain, t)}
          renderFailureCategoryLabel={(category) => renderFailureCategoryLabel(category, t)}
          renderFailureGuidanceActionLabel={(action) => renderFailureGuidanceActionLabel(action, t)}
          onFocusTarget={workspaceController.focusTarget}
        />
      ) : null}

      {detailTab === "overview" && selectedJob.resume_context ? (
        <JobResumeContextPanel
          t={t}
          job={selectedJob}
          suggestedActionLabel={renderSuggestedActionLabel(
            selectedJob.resume_context.suggested_action,
            t,
          )}
          failureDomainLabel={renderFailureDomainLabel(
            selectedJob.resume_context.failure_domain,
            t,
          )}
          failureCategoryLabel={renderFailureCategoryLabel(
            selectedJob.resume_context.failure_category,
            t,
          )}
          executableSuggestedAction={isExecutableNotificationAction(
            selectedJob.resume_context.suggested_action,
          )}
          onRunSuggestedAction={onRunNotificationAction}
        />
      ) : null}

      {detailTab === "delivery" ? (
        <JobsDeliveryContextPanel
          t={t}
          job={selectedJob}
          formatDate={formatDate}
          renderDeliveryStatusLabel={(status) => renderDeliveryStatusLabel(status, t)}
          renderSuggestedActionLabel={(action) => renderSuggestedActionLabel(action, t)}
          renderRolloutPercent={renderRolloutPercent}
          renderDeviceVerificationStatusLabel={(status) => renderDeviceVerificationStatusLabel(status, t)}
          renderReleaseFollowUpStatusLabel={(status) => renderReleaseFollowUpStatusLabel(status, t)}
          executableSuggestedAction={isExecutableNotificationAction(
            selectedJob.delivery_context?.suggested_action,
          )}
          onRunSuggestedAction={() =>
            selectedJob.delivery_context
              ? runDeliverySuggestedAction({
                  job: selectedJob,
                  context: selectedJob.delivery_context,
                  workspaceController,
                })
              : undefined
          }
          pendingHumanApprovalCount={getPendingHumanApprovalCount(selectedJob)}
          approvalReason={
            selectedJob.status_context ? renderStatusContextReason(selectedJob.status_context, t) : undefined
          }
          approvalNextAction={
            selectedJob.status_context ? renderStatusContextAction(selectedJob.status_context, t) : undefined
          }
        />
      ) : null}

      {detailTab === "delivery" ? (
        <JobsDeliveryFormPanel
          t={t}
          latestRunId={latestRunId}
          reviewBundleReady={hasArtifact(artifactItems, "review-bundle")}
          workflowHint={renderDeliveryWorkflowHint(deliveryForm.status, t)}
          checklistItems={deliveryChecklistItems}
          prepareReviewPending={prepareReviewPending}
          onPrepareReview={onPrepareReview}
          showDeliveryForm={showDeliveryForm}
          onToggleDeliveryForm={() => setShowDeliveryForm((current) => !current)}
          deliveryForm={deliveryForm}
          onDeliveryFieldChange={(field, value) => {
            setDeliveryForm((current) => ({
              ...current,
              [field]:
                (field === "evidencePaths" && suggestedEvidencePaths.includes(value)) ||
                (field === "deviceVerificationEvidencePaths" &&
                  deviceSuggestedEvidencePaths.includes(value))
                  ? appendUniqueMultilineItem(current[field], value)
                  : value,
            }))
          }}
          suggestedEvidencePaths={suggestedEvidencePaths}
          deviceSuggestedEvidencePaths={deviceSuggestedEvidencePaths}
          needsDeviceVerification={needsDeviceVerification(deliveryForm.status)}
          deliveryStatuses={DELIVERY_STATUSES}
          releaseChannels={DELIVERY_RELEASE_CHANNELS}
          deviceVerificationStatuses={DEVICE_VERIFICATION_STATUSES}
          renderDeliveryStatusLabel={(status) => renderDeliveryStatusLabel(status, t)}
          renderDeliveryChannelLabel={(channel) => renderDeliveryChannelLabel(channel, t)}
          renderDeviceVerificationStatusLabel={(status) => renderDeviceVerificationStatusLabel(status, t)}
          recordDeliveryPending={recordDeliveryPending}
          onSubmitDelivery={onSubmitDelivery}
          onCancelDelivery={onCancelDelivery}
          released={selectedJob.delivery_context?.status === "released"}
          followUpWorkflowHint={renderReleaseFollowUpWorkflowHint(
            selectedJob.delivery_context?.release_follow_up?.status,
            t,
          )}
          showFollowUpForm={showFollowUpForm}
          onToggleFollowUpForm={() => setShowFollowUpForm((current) => !current)}
          followUpForm={followUpForm}
          onFollowUpFieldChange={(field, value) => {
            setFollowUpForm((current) => ({
              ...current,
              [field]:
                field === "evidencePaths" && followUpSuggestedEvidencePaths.includes(value)
                  ? appendUniqueMultilineItem(current[field], value)
                  : value,
            }))
          }}
          followUpStatuses={RELEASE_FOLLOW_UP_STATUSES}
          renderReleaseFollowUpStatusLabel={(status) => renderReleaseFollowUpStatusLabel(status, t)}
          followUpSuggestedEvidencePaths={followUpSuggestedEvidencePaths}
          recordFollowUpPending={recordFollowUpPending}
          onSubmitFollowUp={onSubmitFollowUp}
          onCancelFollowUp={onCancelFollowUp}
        />
      ) : null}
    </>
  )
}